// Copyright 2014 The Gogs Authors. All rights reserved.
// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package git

import (
	"os"
	"path/filepath"
	"strings"

	"gitea.dev/modules/git/gitcmd"
)

// ObjectType git object type
type ObjectType string

const (
	// ObjectCommit commit object type
	ObjectCommit ObjectType = "commit"
	// ObjectTree tree object type
	ObjectTree ObjectType = "tree"
	// ObjectBlob blob object type
	ObjectBlob ObjectType = "blob"
	// ObjectTag tag object type
	ObjectTag ObjectType = "tag"
	// ObjectBranch branch object type
	ObjectBranch ObjectType = "branch"
)

// Bytes returns the byte array for the Object Type
func (o ObjectType) Bytes() []byte {
	return []byte(o)
}

func (repo *Repository) GetObjectFormat() (ObjectFormat, error) {
	if repo != nil && repo.objectFormat != nil {
		return repo.objectFormat, nil
	}

	if repo != nil {
		if format := objectFormatFromBareConfig(repo.Path); format != nil {
			repo.objectFormat = format
			return format, nil
		}
	}

	str, err := repo.hashObjectBytes(nil, false)
	if err != nil {
		return nil, err
	}
	hash, err := NewIDFromString(str)
	if err != nil {
		return nil, err
	}

	repo.objectFormat = hash.Type()

	return repo.objectFormat, nil
}

// objectFormatFromBareConfig reads extensions.objectformat from a bare repository's config, which saves
// spawning git on every request. It returns nil when repoPath is not a bare repository or the value is unknown.
func objectFormatFromBareConfig(repoPath string) ObjectFormat {
	for _, dir := range []string{"objects", "refs"} {
		if st, err := os.Stat(filepath.Join(repoPath, dir)); err != nil || !st.IsDir() {
			return nil
		}
	}
	data, err := os.ReadFile(filepath.Join(repoPath, "config"))
	if err != nil {
		return nil
	}
	section := ""
	for line := range strings.Lines(string(data)) {
		line = strings.TrimSpace(line)
		if header, ok := strings.CutPrefix(line, "["); ok {
			name, _, _ := strings.Cut(header, "]")
			section = strings.ToLower(strings.TrimSpace(name))
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if section == "extensions" && ok && strings.EqualFold(strings.TrimSpace(key), "objectformat") {
			return ObjectFormatFromName(strings.ToLower(strings.Trim(strings.TrimSpace(value), "\"")))
		}
	}
	return Sha1ObjectFormat // git's default without the extension
}

// HashObjectBytes returns hash for the content
func (repo *Repository) HashObjectBytes(buf []byte) (ObjectID, error) {
	idStr, err := repo.hashObjectBytes(buf, true)
	if err != nil {
		return nil, err
	}
	return NewIDFromString(idStr)
}

func (repo *Repository) hashObjectBytes(buf []byte, save bool) (string, error) {
	var cmd *gitcmd.Command
	if save {
		cmd = gitcmd.NewCommand("hash-object", "-w", "--stdin")
	} else {
		cmd = gitcmd.NewCommand("hash-object", "--stdin")
	}
	stdout, _, err := cmd.
		WithDir(repo.Path).
		WithStdinBytes(buf).
		RunStdString(repo.Ctx)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout), nil
}
