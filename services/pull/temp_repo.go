// Copyright 2019 The Gitea Authors.
// All rights reserved.
// SPDX-License-Identifier: MIT

package pull

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	git_model "gitea.dev/models/git"
	issues_model "gitea.dev/models/issues"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/modules/git"
	"gitea.dev/modules/git/gitcmd"
	"gitea.dev/modules/gitrepo"
	"gitea.dev/modules/log"
	repo_module "gitea.dev/modules/repository"
)

// Temporary repos created here use standard branch names to help simplify
// merging code
const (
	tmpRepoBaseBranch     = "base"     // equivalent to pr.BaseBranch
	tmpRepoTrackingBranch = "tracking" // equivalent to pr.HeadBranch
	tmpRepoStagingBranch  = "staging"  // this is used for a working branch
)

type prTmpRepoContext struct {
	context.Context
	tmpBasePath string
	pr          *issues_model.PullRequest
	outbuf      *bytes.Buffer // we keep these around to help reduce needless buffer recreation, any use should be preceded by a Reset and preferably after use
}

// PrepareGitCmd prepares a git command with the correct directory, environment, and output buffers
// This function can only be called with gitcmd.Run()
// Do NOT use it with gitcmd.RunStd*() functions, otherwise it will panic
func (ctx *prTmpRepoContext) PrepareGitCmd(cmd *gitcmd.Command) *gitcmd.Command {
	ctx.outbuf.Reset()
	return cmd.WithDir(ctx.tmpBasePath).WithStdoutBuffer(ctx.outbuf)
}

// createTemporaryRepoForPR creates a temporary repo with "base" for pr.BaseBranch and "tracking" for  pr.HeadBranch
// it also create a second base branch called "original_base"
func createTemporaryRepoForPR(ctx context.Context, pr *issues_model.PullRequest) (prCtx *prTmpRepoContext, cancel context.CancelFunc, err error) {
	if err := pr.LoadHeadRepo(ctx); err != nil {
		log.Error("%-v LoadHeadRepo: %v", pr, err)
		return nil, nil, fmt.Errorf("%v LoadHeadRepo: %w", pr, err)
	} else if pr.HeadRepo == nil {
		log.Error("%-v HeadRepo %d does not exist", pr, pr.HeadRepoID)
		return nil, nil, &repo_model.ErrRepoNotExist{
			ID: pr.HeadRepoID,
		}
	} else if err := pr.LoadBaseRepo(ctx); err != nil {
		log.Error("%-v LoadBaseRepo: %v", pr, err)
		return nil, nil, fmt.Errorf("%v LoadBaseRepo: %w", pr, err)
	} else if pr.BaseRepo == nil {
		log.Error("%-v BaseRepo %d does not exist", pr, pr.BaseRepoID)
		return nil, nil, &repo_model.ErrRepoNotExist{
			ID: pr.BaseRepoID,
		}
	} else if err := pr.HeadRepo.LoadOwner(ctx); err != nil {
		log.Error("%-v HeadRepo.LoadOwner: %v", pr, err)
		return nil, nil, fmt.Errorf("%v HeadRepo.LoadOwner: %w", pr, err)
	} else if err := pr.BaseRepo.LoadOwner(ctx); err != nil {
		log.Error("%-v BaseRepo.LoadOwner: %v", pr, err)
		return nil, nil, fmt.Errorf("%v BaseRepo.LoadOwner: %w", pr, err)
	}

	// Clone base repo.
	tmpBasePath, cleanup, err := repo_module.CreateTemporaryPath("pull")
	if err != nil {
		log.Error("CreateTemporaryPath[%-v]: %v", pr, err)
		return nil, nil, err
	}
	cancel = cleanup

	prCtx = &prTmpRepoContext{
		Context:     ctx,
		tmpBasePath: tmpBasePath,
		pr:          pr,
		outbuf:      &bytes.Buffer{},
	}

	baseRepoPath := pr.BaseRepo.RepoPath()
	headRepoPath := pr.HeadRepo.RepoPath()

	if err := git.InitRepository(ctx, tmpBasePath, false, pr.BaseRepo.ObjectFormatName); err != nil {
		log.Error("Unable to init tmpBasePath for %-v: %v", pr, err)
		cancel()
		return nil, nil, err
	}

	// addCacheRepo adds git alternatives for the cacheRepoPath in the repoPath
	addCacheRepo := func(repoPath, cacheRepoPath string) error {
		p := filepath.Join(repoPath, ".git", "objects", "info", "alternates")
		f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			log.Error("Could not create .git/objects/info/alternates file in %s: %v", repoPath, err)
			return err
		}
		defer f.Close()
		data := filepath.Join(cacheRepoPath, "objects")
		if _, err := fmt.Fprintln(f, data); err != nil {
			log.Error("Could not write to .git/objects/info/alternates file in %s: %v", repoPath, err)
			return err
		}
		return nil
	}

	// Add head repo remote.
	if err := addCacheRepo(tmpBasePath, baseRepoPath); err != nil {
		log.Error("%-v Unable to add base repository to temporary repo [%s -> %s]: %v", pr, pr.BaseRepo.FullName(), tmpBasePath, err)
		cancel()
		return nil, nil, fmt.Errorf("Unable to add base repository to temporary repo [%s -> tmpBasePath]: %w", pr.BaseRepo.FullName(), err)
	}
	if err := addCacheRepo(tmpBasePath, headRepoPath); err != nil {
		log.Error("%-v Unable to add head repository to temporary repo [%s -> %s]: %v", pr, pr.HeadRepo.FullName(), tmpBasePath, err)
		cancel()
		return nil, nil, fmt.Errorf("Unable to add head base repository to temporary repo [%s -> tmpBasePath]: %w", pr.HeadRepo.FullName(), err)
	}

	// The alternates hold every object, so the refs are set directly: a fetch from a local repository
	// runs upload-pack, rev-list, for-each-ref and maintenance, and each process costs 10-20 ms on Windows.
	// The remotes stay for the pushes back; maintenance is off since the repository is thrown away.
	if err := appendTmpRepoConfig(tmpBasePath, fmt.Sprintf("[remote \"origin\"]\n\turl = %s\n\tfetch = %s\n[remote \"head_repo\"]\n\turl = %s\n\tfetch = +refs/heads/*:refs/remotes/head_repo/*\n[maintenance]\n\tauto = false\n[gc]\n\tauto = 0\n",
		gitConfigValue(baseRepoPath), gitConfigValue("+"+git.BranchPrefix+pr.BaseBranch+":refs/remotes/origin/"+pr.BaseBranch), gitConfigValue(headRepoPath))); err != nil {
		log.Error("%-v Unable to add the remotes to temporary repo [%s]: %v", pr, tmpBasePath, err)
		cancel()
		return nil, nil, fmt.Errorf("Unable to add the remotes to temporary repo: %w", err)
	}

	baseCommitID, err := gitrepo.GetBranchCommitID(ctx, pr.BaseRepo, pr.BaseBranch)
	if err != nil {
		log.Error("%-v Unable to fetch origin base branch [%s:%s -> base, original_base in %s]: %v", pr, pr.BaseRepo.FullName(), pr.BaseBranch, tmpBasePath, err)
		cancel()
		return nil, nil, fmt.Errorf("Unable to fetch origin base branch [%s:%s -> base, original_base in tmpBasePath]: %w", pr.BaseRepo.FullName(), pr.BaseBranch, err)
	}

	objectFormat := git.ObjectFormatFromName(pr.BaseRepo.ObjectFormatName)
	// Fetch head branch
	var headBranch string
	if pr.Flow == issues_model.PullRequestFlowGithub {
		headBranch = git.BranchPrefix + pr.HeadBranch
	} else if len(pr.HeadCommitID) == objectFormat.FullLength() { // for not created pull request
		headBranch = pr.HeadCommitID
	} else {
		headBranch = pr.GetGitHeadRefName()
	}
	headCommitID := headBranch
	if headBranch != pr.HeadCommitID {
		headCommitID, err = getRefCommitID(ctx, pr.HeadRepo, headBranch)
	}
	if err == nil {
		// update-ref refuses a commit that is missing, as the fetch did
		err = prCtx.PrepareGitCmd(gitcmd.NewCommand("update-ref", "--stdin")).WithStdinBytes(fmt.Appendf(nil,
			"create %[1]s%[3]s %[4]s\ncreate %[1]soriginal_%[3]s %[4]s\ncreate %[1]s%[2]s %[5]s\n",
			git.BranchPrefix, tmpRepoTrackingBranch, tmpRepoBaseBranch, baseCommitID, headCommitID)).RunWithStderr(ctx)
	}
	if err != nil {
		cancel()
		if exist, _ := git_model.IsBranchExist(ctx, pr.HeadRepo.ID, pr.HeadBranch); !exist {
			return nil, nil, git_model.ErrBranchNotExist{
				BranchName: pr.HeadBranch,
			}
		}
		log.Error("%-v Unable to fetch head_repo head branch [%s:%s -> tracking in %s]: %v:\n%s", pr, pr.HeadRepo.FullName(), pr.HeadBranch, tmpBasePath, err, prCtx.outbuf.String())
		return nil, nil, fmt.Errorf("Unable to fetch head_repo head branch [%s:%s -> tracking in tmpBasePath]: %w\n%s", pr.HeadRepo.FullName(), headBranch, err, prCtx.outbuf.String())
	}

	if err := prCtx.PrepareGitCmd(gitcmd.NewCommand("symbolic-ref").AddDynamicArguments("HEAD", git.BranchPrefix+tmpRepoBaseBranch)).
		RunWithStderr(ctx); err != nil {
		log.Error("%-v Unable to set HEAD as base branch in [%s]: %v\n%s\n%s", pr, tmpBasePath, err, prCtx.outbuf.String(), err.Stderr())
		cancel()
		return nil, nil, fmt.Errorf("Unable to set HEAD as base branch in tmpBasePath: %w\n%s\n%s", err, prCtx.outbuf.String(), err.Stderr())
	}
	prCtx.outbuf.Reset()
	return prCtx, cancel, nil
}

func getRefCommitID(ctx context.Context, repo *repo_model.Repository, refName string) (string, error) {
	gitRepo, err := gitrepo.OpenRepository(ctx, repo)
	if err != nil {
		return "", err
	}
	defer gitRepo.Close()
	return gitRepo.GetRefCommitID(refName)
}

// gitConfigValue quotes a value for a git config file
func gitConfigValue(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`).Replace(s) + `"`
}

// appendTmpRepoConfig adds settings to a temporary repository in one write, where each git config is a process
func appendTmpRepoConfig(repoPath, config string) error {
	f, err := os.OpenFile(filepath.Join(repoPath, ".git", "config"), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(config); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
