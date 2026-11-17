// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package dump

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"gitea.dev/modules/log"
	"gitea.dev/modules/setting"
	"gitea.dev/modules/timeutil"
)

var SupportedOutputTypes = []string{"zip", "tar", "tar.gz"}

// PrepareFileNameAndType prepares the output file name and type, if the type is not supported, it returns an empty "outType"
func PrepareFileNameAndType(argFile, argType string) (outFileName, outType string) {
	if argFile == "" && argType == "" {
		outType = SupportedOutputTypes[0]
		outFileName = fmt.Sprintf("gitea-dump-%d.%s", timeutil.TimeStampNow(), outType)
	} else if argFile == "" {
		outType = argType
		outFileName = fmt.Sprintf("gitea-dump-%d.%s", timeutil.TimeStampNow(), outType)
	} else if argType == "" {
		if filepath.Ext(outFileName) == "" {
			outType = SupportedOutputTypes[0]
			outFileName = argFile
		} else {
			for _, t := range SupportedOutputTypes {
				if strings.HasSuffix(argFile, "."+t) {
					outFileName = argFile
					outType = t
				}
			}
		}
	} else {
		outFileName, outType = argFile, argType
	}
	if !slices.Contains(SupportedOutputTypes, outType) {
		return "", ""
	}
	return outFileName, outType
}

func IsSubdir(upper, lower string) (bool, error) {
	if relPath, err := filepath.Rel(upper, lower); err != nil {
		return false, err
	} else if relPath == "." || !strings.HasPrefix(relPath, ".") {
		return true, nil
	}
	return false, nil
}

type Dumper struct {
	Verbose bool

	ctx  context.Context
	zipw *zip.Writer
	tarw *tar.Writer
	gzw  *gzip.Writer

	globalExcludeAbsPaths []string
}

// NewDumper writes zip, tar or tar.gz with the standard library; other formats need an external tool on the output
func NewDumper(ctx context.Context, format string, output io.Writer) (*Dumper, error) {
	d := &Dumper{ctx: ctx}
	switch format {
	case "zip":
		d.zipw = zip.NewWriter(output)
	case "tar":
		d.tarw = tar.NewWriter(output)
	case "tar.gz":
		d.gzw = gzip.NewWriter(output)
		d.tarw = tar.NewWriter(d.gzw)
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
	return d, nil
}

// addEntry writes one entry; open is not called for directories
func (dumper *Dumper) addEntry(name string, info fs.FileInfo, open func() (io.ReadCloser, error)) error {
	if err := dumper.ctx.Err(); err != nil {
		return err
	}
	var w io.Writer
	if dumper.zipw != nil {
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return fmt.Errorf("zip header for %s: %w", name, err)
		}
		hdr.Name, hdr.Method = name, zip.Deflate
		if info.IsDir() {
			hdr.Name, hdr.Method = strings.TrimSuffix(name, "/")+"/", zip.Store // the slash marks a directory
		}
		if w, err = dumper.zipw.CreateHeader(hdr); err != nil {
			return fmt.Errorf("zip entry for %s: %w", name, err)
		}
	} else {
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("tar header for %s: %w", name, err)
		}
		hdr.Name = name
		if err = dumper.tarw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("tar entry for %s: %w", name, err)
		}
		if hdr.Typeflag != tar.TypeReg {
			return nil
		}
		w = dumper.tarw
	}
	if info.IsDir() {
		return nil
	}
	r, err := open()
	if err != nil {
		return err
	}
	defer r.Close()
	if _, err = io.Copy(w, r); err != nil {
		return fmt.Errorf("copy %s: %w", name, err)
	}
	return nil
}

// AddFileByPath adds a file by its filesystem path
func (dumper *Dumper) AddFileByPath(filePath, absPath string) error {
	if dumper.Verbose {
		log.Info("Adding local file %s", filePath)
	}

	fileInfo, err := os.Stat(absPath)
	if err != nil {
		return err
	}
	return dumper.addEntry(filePath, fileInfo, func() (io.ReadCloser, error) { return os.Open(absPath) })
}

// AddFileByReader adds a file's contents from a Reader
func (dumper *Dumper) AddFileByReader(r io.Reader, info os.FileInfo, customName string) error {
	if dumper.Verbose {
		log.Info("Adding storage file %s", customName)
	}
	return dumper.addEntry(customName, info, func() (io.ReadCloser, error) { return io.NopCloser(r), nil })
}

func (dumper *Dumper) Close() error {
	if dumper.zipw != nil {
		return dumper.zipw.Close()
	}
	err := dumper.tarw.Close()
	if dumper.gzw != nil {
		err = errors.Join(err, dumper.gzw.Close())
	}
	return err
}

func (dumper *Dumper) normalizeFilePath(absPath string) string {
	absPath = filepath.Clean(absPath)
	if setting.IsWindows {
		absPath = strings.ToLower(absPath)
	}
	return absPath
}

func (dumper *Dumper) GlobalExcludeAbsPath(absPaths ...string) {
	for _, absPath := range absPaths {
		dumper.globalExcludeAbsPaths = append(dumper.globalExcludeAbsPaths, dumper.normalizeFilePath(absPath))
	}
}

func (dumper *Dumper) shouldExclude(absPath string, excludes []string) bool {
	norm := dumper.normalizeFilePath(absPath)
	return slices.Contains(dumper.globalExcludeAbsPaths, norm) || slices.Contains(excludes, norm)
}

func (dumper *Dumper) AddRecursiveExclude(insidePath, absPath string, excludes []string) error {
	excludes = slices.Clone(excludes)
	for i := range excludes {
		excludes[i] = dumper.normalizeFilePath(excludes[i])
	}
	return dumper.addFileOrDir(insidePath, absPath, excludes)
}

func (dumper *Dumper) addFileOrDir(insidePath, absPath string, excludes []string) error {
	absPath, err := filepath.Abs(absPath)
	if err != nil {
		return err
	}
	dir, err := os.Open(absPath)
	if err != nil {
		return err
	}
	defer dir.Close()

	files, err := dir.Readdir(0)
	if err != nil {
		return err
	}
	for _, file := range files {
		currentAbsPath := filepath.Join(absPath, file.Name())
		if dumper.shouldExclude(currentAbsPath, excludes) {
			continue
		}

		currentInsidePath := path.Join(insidePath, file.Name())
		if file.IsDir() {
			if err := dumper.AddFileByPath(currentInsidePath, currentAbsPath); err != nil {
				return err
			}
			if err = dumper.addFileOrDir(currentInsidePath, currentAbsPath, excludes); err != nil {
				return err
			}
		} else {
			// only copy regular files and symlink regular files, skip non-regular files like socket/pipe/...
			shouldAdd := file.Mode().IsRegular()
			if !shouldAdd && file.Mode()&os.ModeSymlink == os.ModeSymlink {
				target, err := filepath.EvalSymlinks(currentAbsPath)
				if err != nil {
					return err
				}
				targetStat, err := os.Stat(target)
				if err != nil {
					return err
				}
				shouldAdd = targetStat.Mode().IsRegular()
			}
			if shouldAdd {
				if err = dumper.AddFileByPath(currentInsidePath, currentAbsPath); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
