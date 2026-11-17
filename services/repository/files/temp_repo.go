// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package files

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	repo_model "gitea.dev/models/repo"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/git"
	"gitea.dev/modules/git/gitcmd"
	"gitea.dev/modules/gitrepo"
	"gitea.dev/modules/log"
	"gitea.dev/modules/private"
	repo_module "gitea.dev/modules/repository"
	"gitea.dev/modules/util"
)

// TemporaryUploadRepository is a type to wrap our upload repositories as a shallow clone
type TemporaryUploadRepository struct {
	repo     *repo_model.Repository
	gitRepo  *git.Repository
	basePath string
	cleanup  func()
}

// NewTemporaryUploadRepository creates a new temporary upload repository
func NewTemporaryUploadRepository(repo *repo_model.Repository) (*TemporaryUploadRepository, error) {
	basePath, cleanup, err := repo_module.CreateTemporaryPath("upload")
	if err != nil {
		return nil, err
	}
	t := &TemporaryUploadRepository{repo: repo, basePath: basePath, cleanup: cleanup}
	return t, nil
}

// Close the repository cleaning up all files
func (t *TemporaryUploadRepository) Close() {
	// must stop the repo access before removal, otherwise Windows can't remove the directory occupied by other processes
	t.gitRepo.Close()
	if t.cleanup != nil {
		t.cleanup()
	}
}

// Clone the base repository to our path and set branch as the HEAD
func (t *TemporaryUploadRepository) Clone(ctx context.Context, branch string, bare bool) error {
	if err := gitrepo.CloneRepoToLocal(ctx, t.repo, t.basePath, git.CloneRepoOptions{
		Bare:   bare,
		Branch: branch,
		Shared: true,
	}); err != nil {
		stderr := err.Error()
		if matched, _ := regexp.MatchString(".*Remote branch .* not found in upstream origin.*", stderr); matched {
			return git.ErrBranchNotExist{
				Name: branch,
			}
		} else if matched, _ := regexp.MatchString(".* repository .* does not exist.*", stderr); matched {
			return repo_model.ErrRepoNotExist{
				ID:        t.repo.ID,
				UID:       t.repo.OwnerID,
				OwnerName: t.repo.OwnerName,
				Name:      t.repo.Name,
			}
		}
		return fmt.Errorf("Clone: %w %s", err, stderr)
	}
	gitRepo, err := git.OpenRepository(ctx, t.basePath)
	if err != nil {
		return err
	}
	t.gitRepo = gitRepo
	return nil
}

// Init the repository
func (t *TemporaryUploadRepository) Init(ctx context.Context, objectFormatName string) error {
	if err := git.InitRepository(ctx, t.basePath, false, objectFormatName); err != nil {
		return err
	}
	gitRepo, err := git.OpenRepository(ctx, t.basePath)
	if err != nil {
		return err
	}
	t.gitRepo = gitRepo
	return nil
}

// SetDefaultIndex sets the git index to our HEAD
func (t *TemporaryUploadRepository) SetDefaultIndex(ctx context.Context) error {
	if err := gitcmd.NewCommand("read-tree", "HEAD").WithDir(t.basePath).RunWithStderr(ctx); err != nil {
		return fmt.Errorf("SetDefaultIndex: %w", err)
	}
	return nil
}

// LsFiles checks if the given filename arguments are in the index
func (t *TemporaryUploadRepository) LsFiles(ctx context.Context, filenames ...string) ([]string, error) {
	stdOut := new(bytes.Buffer)
	if err := gitcmd.NewCommand("ls-files", "-z").AddDashesAndList(filenames...).
		WithDir(t.basePath).
		WithStdoutBuffer(stdOut).
		RunWithStderr(ctx); err != nil {
		return nil, fmt.Errorf("unable to run git ls-files for temporary repo of: %s, error: %w", t.repo.FullName(), err)
	}

	fileList := make([]string, 0, len(filenames))
	for line := range bytes.SplitSeq(stdOut.Bytes(), []byte{'\000'}) {
		fileList = append(fileList, string(line))
	}

	return fileList, nil
}

func (t *TemporaryUploadRepository) RemoveRecursivelyFromIndex(ctx context.Context, path string) error {
	_, _, err := gitcmd.NewCommand("rm", "--cached", "-r").
		AddDynamicArguments(path).
		WithDir(t.basePath).
		RunStdBytes(ctx)
	return err
}

// RemoveFilesFromIndex removes the given files from the index
func (t *TemporaryUploadRepository) RemoveFilesFromIndex(ctx context.Context, filenames ...string) error {
	objFmt, err := t.gitRepo.GetObjectFormat()
	if err != nil {
		return fmt.Errorf("unable to get object format for temporary repo: %q, error: %w", t.repo.FullName(), err)
	}
	stdIn := new(bytes.Buffer)
	for _, file := range filenames {
		if file != "" {
			// man git-update-index: input syntax (1): mode SP sha1 TAB path
			// mode=0 means "remove from index", then hash part "does not matter as long as it is well formatted."
			_, _ = fmt.Fprintf(stdIn, "0 %s\t%s\x00", objFmt.EmptyObjectID(), file)
		}
	}

	if err := gitcmd.NewCommand("update-index", "--remove", "-z", "--index-info").
		WithDir(t.basePath).
		WithStdinBytes(stdIn.Bytes()).
		RunWithStderr(ctx); err != nil {
		return fmt.Errorf("unable to update-index for temporary repo: %q, error: %w", t.repo.FullName(), err)
	}
	return nil
}

// HashObjectAndWrite writes the provided content to the object db and returns its hash
func (t *TemporaryUploadRepository) HashObjectAndWrite(ctx context.Context, content io.Reader) (string, error) {
	stdOut := new(bytes.Buffer)
	if err := gitcmd.NewCommand("hash-object", "-w", "--stdin").
		WithDir(t.basePath).
		WithStdoutBuffer(stdOut).
		WithStdinCopy(content).
		RunWithStderr(ctx); err != nil {
		return "", fmt.Errorf("unable to hash-object to temporary repo: %s, error: %w", t.repo.FullName(), err)
	}

	return strings.TrimSpace(stdOut.String()), nil
}

// AddObjectToIndex adds the provided object hash to the index with the provided mode and path
func (t *TemporaryUploadRepository) AddObjectToIndex(ctx context.Context, mode, objectHash, objectPath string) error {
	cmd := gitcmd.NewCommand("update-index", "--add", "--replace", "--cacheinfo").
		AddDynamicArguments(mode + "," + objectHash + "," + objectPath).WithDir(t.basePath)
	if err := cmd.RunWithStderr(ctx); err != nil {
		if matched, _ := regexp.MatchString(".*Invalid path '.*", err.Stderr()); matched {
			return ErrFilePathInvalid{
				Message: objectPath,
				Path:    objectPath,
			}
		}
		return fmt.Errorf("unable to add object to index at %s in temporary repo %s, error: %w", objectPath, t.repo.FullName(), err)
	}
	return nil
}

// WriteTree writes the current index as a tree to the object db and returns its hash
func (t *TemporaryUploadRepository) WriteTree(ctx context.Context) (string, error) {
	stdout, _, err := gitcmd.NewCommand("write-tree").WithDir(t.basePath).RunStdString(ctx)
	if err != nil {
		log.Error("Unable to write tree in temporary repo: %s(%s): Error: %v", t.repo.FullName(), t.basePath, err)
		return "", fmt.Errorf("Unable to write-tree in temporary repo for: %s Error: %w", t.repo.FullName(), err)
	}
	return strings.TrimSpace(stdout), nil
}

// GetLastCommit gets the last commit ID SHA of the repo
func (t *TemporaryUploadRepository) GetLastCommit(ctx context.Context) (string, error) {
	return t.GetLastCommitByRef(ctx, "HEAD")
}

// GetLastCommitByRef gets the last commit ID SHA of the repo by ref
func (t *TemporaryUploadRepository) GetLastCommitByRef(ctx context.Context, ref string) (string, error) {
	if ref == "" {
		ref = "HEAD"
	}
	stdout, _, err := gitcmd.NewCommand("rev-parse").AddDynamicArguments(ref).WithDir(t.basePath).RunStdString(ctx)
	if err != nil {
		log.Error("Unable to get last ref for %s in temporary repo: %s(%s): Error: %v", ref, t.repo.FullName(), t.basePath, err)
		return "", fmt.Errorf("Unable to rev-parse %s in temporary repo for: %s Error: %w", ref, t.repo.FullName(), err)
	}
	return strings.TrimSpace(stdout), nil
}

type CommitTreeUserOptions struct {
	ParentCommitID string
	TreeHash       string
	CommitMessage  string
	SignOff        bool

	DoerUser *user_model.User

	AuthorIdentity    *IdentityOptions // if nil, use doer
	AuthorTime        *time.Time       // if nil, use now
	CommitterIdentity *IdentityOptions
	CommitterTime     *time.Time
}

func makeGitUserSignature(doer *user_model.User, identity, other *IdentityOptions) *git.Signature {
	gitSig := &git.Signature{}
	if identity != nil {
		gitSig.Name, gitSig.Email = identity.GitUserName, identity.GitUserEmail
	}
	if other != nil {
		gitSig.Name = util.IfZero(gitSig.Name, other.GitUserName)
		gitSig.Email = util.IfZero(gitSig.Email, other.GitUserEmail)
	}
	if gitSig.Name == "" {
		gitSig.Name = doer.GitName()
	}
	if gitSig.Email == "" {
		gitSig.Email = doer.GetEmail()
	}
	return gitSig
}

// CommitTree creates a commit from a given tree for the user with provided message
func (t *TemporaryUploadRepository) CommitTree(ctx context.Context, opts *CommitTreeUserOptions) (string, error) {
	authorSig := makeGitUserSignature(opts.DoerUser, opts.AuthorIdentity, opts.CommitterIdentity)
	committerSig := makeGitUserSignature(opts.DoerUser, opts.CommitterIdentity, opts.AuthorIdentity)

	authorDate := opts.AuthorTime
	committerDate := opts.CommitterTime
	if authorDate == nil && committerDate == nil {
		authorDate = new(time.Now())
		committerDate = authorDate
	} else if authorDate == nil {
		authorDate = committerDate
	} else if committerDate == nil {
		committerDate = authorDate
	}

	// Because this may call hooks we should pass in the environment
	env := append(os.Environ(),
		"GIT_AUTHOR_NAME="+authorSig.Name,
		"GIT_AUTHOR_EMAIL="+authorSig.Email,
		"GIT_AUTHOR_DATE="+authorDate.Format(time.RFC3339),
		"GIT_COMMITTER_DATE="+committerDate.Format(time.RFC3339),
	)

	messageBytes := new(bytes.Buffer)
	_, _ = messageBytes.WriteString(opts.CommitMessage)
	_, _ = messageBytes.WriteString("\n")

	cmdCommitTree := gitcmd.NewCommand("commit-tree").AddDynamicArguments(opts.TreeHash)
	if opts.ParentCommitID != "" {
		cmdCommitTree.AddOptionValues("-p", opts.ParentCommitID)
	}

	cmdCommitTree.AddArguments("--no-gpg-sign") // commits are never signed

	if opts.SignOff {
		// Signed-off-by
		_, _ = messageBytes.WriteString("\n")
		_, _ = messageBytes.WriteString("Signed-off-by: ")
		_, _ = messageBytes.WriteString(committerSig.String())
	}

	env = append(env,
		"GIT_COMMITTER_NAME="+committerSig.Name,
		"GIT_COMMITTER_EMAIL="+committerSig.Email,
	)

	stdout := new(bytes.Buffer)
	if err := cmdCommitTree.
		WithEnv(env).
		WithDir(t.basePath).
		WithStdoutBuffer(stdout).
		WithStdinBytes(messageBytes.Bytes()).
		RunWithStderr(ctx); err != nil {
		return "", fmt.Errorf("unable to commit-tree in temporary repo: %s Error: %w", t.repo.FullName(), err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// Push the provided commitHash to the repository branch by the provided user
func (t *TemporaryUploadRepository) Push(ctx context.Context, doer *user_model.User, commitHash, branch string, force bool) error {
	// Because calls hooks we need to pass in the environment
	env := append(repo_module.PushingEnvironment(doer, t.repo), repo_module.NoRefRulesEnvironment(ctx, doer, t.repo)...)
	if t.updateRef(ctx, env, strings.TrimSpace(commitHash), strings.TrimSpace(branch), force) {
		return nil
	}
	if err := private.RunServerPush(ctx, env, func(env []string) error {
		return gitrepo.PushFromLocal(ctx, t.basePath, t.repo, git.PushOptions{
			Branch: strings.TrimSpace(commitHash) + ":" + git.BranchPrefix + strings.TrimSpace(branch),
			Env:    env,
			Force:  force,
		})
	}); err != nil {
		if git.IsErrPushOutOfDate(err) {
			return err
		} else if git.IsErrPushRejected(err) {
			return err
		}
		log.Error("Unable to push back to repo from temporary repo: %s (%s)\nError: %v",
			t.repo.FullName(), t.basePath, err)
		return fmt.Errorf("Unable to push back to repo from temporary repo: %s (%s) Error: %v",
			t.repo.FullName(), t.basePath, err)
	}
	return nil
}

// GetBranchCommit Gets the commit object of the given branch
// updateRef moves the branch to the commit without a push, when the push would create the branch, fast-forward it
// or force it and would run no rules or hooks. The new objects are loose in the temporary repository, whose other
// objects are the repository's, so they are linked into the repository first.
func (t *TemporaryUploadRepository) updateRef(ctx context.Context, env []string, commitID, branch string, force bool) bool {
	// UpdateServerRef checks this too, but before the objects are linked
	if !slices.ContainsFunc(env, func(kv string) bool { return strings.HasPrefix(kv, repo_module.EnvNoRefRules+"=") }) {
		return false
	}
	oldCommitID, err := gitrepo.GetBranchCommitID(ctx, t.repo, branch)
	if err != nil {
		if !git.IsErrNotExist(err) {
			return false
		}
		oldCommitID = "" // the push creates the branch
	} else if !force {
		// a fast-forward from the commit the new one was made on; the push checks any other
		if commit, err := t.GetCommit(commitID); err != nil || commit.ParentCount() == 0 || commit.Parents[0].String() != oldCommitID {
			return false
		}
	}
	if err := linkLooseObjects(t.basePath, filepath.Join(t.repo.RepoPath(), "objects")); err != nil {
		log.Debug("objects of %s for %s, pushing instead: %v", t.basePath, t.repo.FullName(), err)
		return false
	}
	return private.UpdateServerRef(ctx, t.repo, env, git.RefNameFromBranch(branch), oldCommitID, commitID)
}

// linkLooseObjects puts the loose objects of a temporary repository into the objects dir of a repository, as git
// names and writes them; packed objects are not handled
func linkLooseObjects(tmpRepoPath, objectsDir string) error {
	srcDir := filepath.Join(tmpRepoPath, ".git", "objects")
	if _, err := os.Stat(srcDir); err != nil {
		srcDir = filepath.Join(tmpRepoPath, "objects") // a bare clone
	}
	if packs, _ := filepath.Glob(filepath.Join(srcDir, "pack", "*.pack")); len(packs) > 0 {
		return errors.New("packed objects")
	}
	dirs, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		if !dir.IsDir() || len(dir.Name()) != 2 { // info and pack
			continue
		}
		files, err := os.ReadDir(filepath.Join(srcDir, dir.Name()))
		if err != nil {
			return err
		}
		if len(files) > 0 {
			if err := os.MkdirAll(filepath.Join(objectsDir, dir.Name()), 0o755); err != nil {
				return err
			}
		}
		for _, file := range files {
			src, dst := filepath.Join(srcDir, dir.Name(), file.Name()), filepath.Join(objectsDir, dir.Name(), file.Name())
			if _, err := os.Stat(dst); err == nil {
				continue
			}
			if err := os.Link(src, dst); err == nil || errors.Is(err, os.ErrExist) {
				continue
			}
			if err := copyObjectFile(src, dst); err != nil {
				return err
			}
		}
	}
	return nil
}

// copyObjectFile writes a temporary file and renames it, as git does, so a reader never sees part of an object
func copyObjectFile(src, dst string) error {
	content, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(dst), "tmp_obj_")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(content)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		_ = os.Chmod(tmp, 0o444)
		err = os.Rename(tmp, dst)
	}
	if err != nil {
		_ = os.Remove(tmp)
		if _, statErr := os.Stat(dst); statErr == nil {
			return nil // written by another
		}
	}
	return err
}

func (t *TemporaryUploadRepository) GetBranchCommit(branch string) (*git.Commit, error) {
	if t.gitRepo == nil {
		return nil, errors.New("repository has not been cloned")
	}
	return t.gitRepo.GetBranchCommit(branch)
}

// GetCommit Gets the commit object of the given commit ID
func (t *TemporaryUploadRepository) GetCommit(commitID string) (*git.Commit, error) {
	if t.gitRepo == nil {
		return nil, errors.New("repository has not been cloned")
	}
	return t.gitRepo.GetCommit(commitID)
}
