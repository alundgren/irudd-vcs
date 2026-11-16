// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"

	"gitea.dev/models/db"
	git_model "gitea.dev/models/git"
	issues_model "gitea.dev/models/issues"
	repo_model "gitea.dev/models/repo"
	system_model "gitea.dev/models/system"
	user_model "gitea.dev/models/user"
	"gitea.dev/models/webhook"
	"gitea.dev/modules/gitrepo"
	"gitea.dev/modules/graceful"
	"gitea.dev/modules/lfs"
	"gitea.dev/modules/log"
	"gitea.dev/modules/storage"
	issue_service "gitea.dev/services/issue"

	"xorm.io/builder"
)

func deleteDBRepository(ctx context.Context, repoID int64) error {
	if cnt, err := db.GetEngine(ctx).ID(repoID).Delete(&repo_model.Repository{}); err != nil {
		return err
	} else if cnt != 1 {
		return repo_model.ErrRepoNotExist{
			ID:        repoID,
			OwnerName: "",
			Name:      "",
		}
	}
	return nil
}

// DeleteRepository deletes a repository for a user or organization.
// make sure if you call this func to close open sessions (sqlite will otherwise get a deadlock)
func DeleteRepositoryDirectly(ctx context.Context, repoID int64) error {
	ctx, committer, err := db.TxContext(ctx)
	if err != nil {
		return err
	}
	defer committer.Close()
	sess := db.GetEngine(ctx)

	repo := &repo_model.Repository{}
	has, err := sess.ID(repoID).Get(repo)
	if err != nil {
		return err
	} else if !has {
		return repo_model.ErrRepoNotExist{
			ID:        repoID,
			OwnerName: "",
			Name:      "",
		}
	}

	if err := deleteDBRepository(ctx, repoID); err != nil {
		return err
	}

	if _, err := db.GetEngine(ctx).In("hook_id", builder.Select("id").From("webhook").Where(builder.Eq{"webhook.repo_id": repo.ID})).
		Delete(&webhook.HookTask{}); err != nil {
		return err
	}

	if err := db.DeleteBeans(ctx,
		&issues_model.Comment{RefRepoID: repoID},
		&git_model.CommitStatus{RepoID: repoID},
		&git_model.CommitStatusIndex{RepoID: repoID},
		&git_model.CommitStatusSummary{RepoID: repoID},
		&git_model.Branch{RepoID: repoID},
		&git_model.RenamedBranch{RepoID: repoID},
		&git_model.LFSLock{RepoID: repoID},
		&git_model.ProtectedBranch{RepoID: repoID},
		&git_model.ProtectedTag{RepoID: repoID},
		&repo_model.Release{RepoID: repoID},
		&repo_model.Redirect{RedirectRepoID: repoID},
		&repo_model.RepoUnit{RepoID: repoID},
		&webhook.Webhook{RepoID: repoID},
	); err != nil {
		return fmt.Errorf("deleteBeans: %w", err)
	}

	// Delete Labels and related objects
	if err := issues_model.DeleteLabelsByRepoID(ctx, repoID); err != nil {
		return err
	}

	// Delete Pulls and related objects
	if err := issues_model.DeletePullsByBaseRepoID(ctx, repoID); err != nil {
		return err
	}

	// Delete Issues and related objects
	if err = issue_service.DeleteIssuesByRepoID(ctx, repoID); err != nil {
		return err
	}

	// Delete issue index
	if err := db.DeleteResourceIndex(ctx, "issue_index", repoID); err != nil {
		return err
	}

	if repo.IsFork {
		if _, err := db.Exec(ctx, "UPDATE `repository` SET num_forks=num_forks-1 WHERE id=?", repo.ForkID); err != nil {
			return fmt.Errorf("decrease fork count: %w", err)
		}
	}

	if _, err := db.Exec(ctx, "UPDATE `user` SET num_repos=num_repos-1 WHERE id=?", repo.OwnerID); err != nil {
		return err
	}

	// Remove LFS objects
	var lfsObjects []*git_model.LFSMetaObject
	if err = sess.Where("repository_id=?", repoID).Find(&lfsObjects); err != nil {
		return err
	}

	lfsPaths := make([]string, 0, len(lfsObjects))
	for _, v := range lfsObjects {
		count, err := db.CountByBean(ctx, &git_model.LFSMetaObject{Pointer: lfs.Pointer{Oid: v.Oid}})
		if err != nil {
			return err
		}
		if count > 1 {
			continue
		}

		lfsPaths = append(lfsPaths, v.RelativePath())
	}

	if _, err := db.DeleteByBean(ctx, &git_model.LFSMetaObject{RepositoryID: repoID}); err != nil {
		return err
	}

	if repo.NumForks > 0 {
		if _, err = sess.Exec("UPDATE `repository` SET fork_id=0,is_fork=? WHERE fork_id=?", false, repo.ID); err != nil {
			log.Error("reset 'fork_id' and 'is_fork': %v", err)
		}
	}

	if err = committer.Commit(); err != nil {
		return err
	}

	committer.Close()

	// We should always delete the files after the database transaction succeed. If
	// we delete the file but the database rollback, the repository will be broken.

	// Remove repository files.
	if err := gitrepo.DeleteRepository(ctx, repo); err != nil {
		desc := fmt.Sprintf("Delete repository files (%s): %v", repo.FullName(), err)
		if err = system_model.CreateNotice(graceful.GetManager().ShutdownContext(), system_model.NoticeRepository, desc); err != nil {
			log.Error("CreateRepositoryNotice: %v", err)
		}
	}

	// Remove lfs objects
	for _, lfsObj := range lfsPaths {
		system_model.RemoveStorageWithNotice(ctx, storage.LFS, "Delete orphaned LFS file", lfsObj)
	}

	return nil
}

// DeleteOwnerRepositoriesDirectly calls DeleteRepositoryDirectly for all repos of the given owner
func DeleteOwnerRepositoriesDirectly(ctx context.Context, owner *user_model.User) error {
	for {
		repos, _, err := repo_model.GetUserRepositories(ctx, repo_model.SearchRepoOptions{
			ListOptions: db.ListOptions{
				PageSize: repo_model.RepositoryListDefaultPageSize,
				Page:     1,
			},
			Private: true,
			OwnerID: owner.ID,
			Actor:   owner,
		})
		if err != nil {
			return fmt.Errorf("GetUserRepositories: %w", err)
		}
		if len(repos) == 0 {
			break
		}
		for _, repo := range repos {
			if err := DeleteRepositoryDirectly(ctx, repo.ID); err != nil {
				return fmt.Errorf("unable to delete repository %s for %s[%d]. Error: %w", repo.Name, owner.Name, owner.ID, err)
			}
		}
	}
	return nil
}
