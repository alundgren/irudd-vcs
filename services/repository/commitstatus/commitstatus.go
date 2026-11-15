// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package commitstatus

import (
	"context"
	"crypto/sha256"
	"fmt"

	"gitea.dev/models/db"
	git_model "gitea.dev/models/git"
	repo_model "gitea.dev/models/repo"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/cache"
	"gitea.dev/modules/git"
	"gitea.dev/modules/gitrepo"
	"gitea.dev/modules/log"
	repo_module "gitea.dev/modules/repository"
	"gitea.dev/services/notify"
)

func getCacheKey(repoID int64, brancheName string) string {
	hashBytes := sha256.Sum256(fmt.Appendf(nil, "%d:%s", repoID, brancheName))
	return fmt.Sprintf("commit_status:%x", hashBytes)
}

func deleteCommitStatusCache(repoID int64, branchName string) error {
	c := cache.GetCache()
	return c.Delete(getCacheKey(repoID, branchName))
}

// CreateCommitStatus creates a new CommitStatus given a bunch of parameters
// NOTE: All text-values will be trimmed from whitespaces.
// Requires: Repo, Creator, SHA
func CreateCommitStatus(ctx context.Context, repo *repo_model.Repository, creator *user_model.User, sha string, status *git_model.CommitStatus) error {
	// confirm that commit is exist
	gitRepo, closer, err := gitrepo.RepositoryFromContextOrOpen(ctx, repo)
	if err != nil {
		return fmt.Errorf("OpenRepository[%s]: %w", repo.RelativePath(), err)
	}
	defer closer.Close()

	objectFormat := git.ObjectFormatFromName(repo.ObjectFormatName)

	commit, err := gitRepo.GetCommit(sha)
	if err != nil {
		return fmt.Errorf("GetCommit[%s]: %w", sha, err)
	}
	if len(sha) != objectFormat.FullLength() {
		// use complete commit sha
		sha = commit.ID.String()
	}

	if err := db.WithTx(ctx, func(ctx context.Context) error {
		if err := git_model.NewCommitStatus(ctx, git_model.NewCommitStatusOptions{
			Repo:         repo,
			Creator:      creator,
			SHA:          commit.ID,
			CommitStatus: status,
		}); err != nil {
			return fmt.Errorf("NewCommitStatus[repo_id: %d, user_id: %d, sha: %s]: %w", repo.ID, creator.ID, sha, err)
		}

		return git_model.UpdateCommitStatusSummary(ctx, repo.ID, commit.ID.String())
	}); err != nil {
		return err
	}

	notify.CreateCommitStatus(ctx, repo, repo_module.CommitToPushCommit(commit), creator, status)

	defaultBranchCommit, err := gitRepo.GetBranchCommit(repo.DefaultBranch)
	if err != nil {
		return fmt.Errorf("GetBranchCommit[%s]: %w", repo.DefaultBranch, err)
	}

	if commit.ID.String() == defaultBranchCommit.ID.String() { // since one commit status updated, the combined commit status should be invalid
		if err := deleteCommitStatusCache(repo.ID, repo.DefaultBranch); err != nil {
			log.Error("deleteCommitStatusCache[%d:%s] failed: %v", repo.ID, repo.DefaultBranch, err)
		}
	}

	return nil
}
