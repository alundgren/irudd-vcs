// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"
	"strings"

	"gitea.dev/models/db"
	repo_model "gitea.dev/models/repo"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/gitrepo"
	"gitea.dev/modules/globallock"
	"gitea.dev/modules/log"
	notify_service "gitea.dev/services/notify"
)

func getRepoWorkingLockKey(repoID int64) string {
	return fmt.Sprintf("repo_working_%d", repoID)
}

// isRepositoryModelOrDirExist returns true if the repository with given name under user has already existed.
func isRepositoryModelOrDirExist(ctx context.Context, u *user_model.User, repoName string) (bool, error) {
	has, err := repo_model.IsRepositoryModelExist(ctx, u, repoName)
	if err != nil {
		return false, err
	}
	repo := repo_model.StorageRepo(repo_model.RelativePath(u.Name, repoName))
	isExist, err := gitrepo.IsRepositoryExist(ctx, repo)
	return has || isExist, err
}

// changeRepositoryName changes all corresponding setting from old repository name to new one.
func changeRepositoryName(ctx context.Context, repo *repo_model.Repository, newRepoName string) (err error) {
	oldRepoName := repo.Name
	newRepoName = strings.ToLower(newRepoName)
	if err = repo_model.IsUsableRepoName(newRepoName); err != nil {
		return err
	}

	if err := repo.LoadOwner(ctx); err != nil {
		return err
	}

	has, err := isRepositoryModelOrDirExist(ctx, repo.Owner, newRepoName)
	if err != nil {
		return fmt.Errorf("IsRepositoryExist: %w", err)
	} else if has {
		return repo_model.ErrRepoAlreadyExist{
			Uname: repo.OwnerName,
			Name:  newRepoName,
		}
	}

	if err = gitrepo.RenameRepository(ctx, repo,
		repo_model.StorageRepo(repo_model.RelativePath(repo.OwnerName, newRepoName))); err != nil {
		return fmt.Errorf("rename repository directory: %w", err)
	}

	return db.WithTx(ctx, func(ctx context.Context) error {
		return repo_model.NewRedirect(ctx, repo.Owner.ID, repo.ID, oldRepoName, newRepoName)
	})
}

// ChangeRepositoryName changes all corresponding setting from old repository name to new one.
func ChangeRepositoryName(ctx context.Context, doer *user_model.User, repo *repo_model.Repository, newRepoName string) error {
	log.Trace("ChangeRepositoryName: %s/%s -> %s", doer.Name, repo.Name, newRepoName)

	oldRepoName := repo.Name

	// Change repository directory name. We must lock the local copy of the
	// repo so that we can automatically rename the repo path and updates the
	// local copy's origin accordingly.

	releaser, err := globallock.Lock(ctx, getRepoWorkingLockKey(repo.ID))
	if err != nil {
		log.Error("lock.Lock(): %v", err)
		return fmt.Errorf("lock.Lock: %w", err)
	}
	defer releaser()

	if err := changeRepositoryName(ctx, repo, newRepoName); err != nil {
		return err
	}
	releaser()

	repo.Name = newRepoName
	notify_service.RenameRepository(ctx, doer, repo, oldRepoName)

	return nil
}
