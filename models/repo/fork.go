// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"context"

	"gitea.dev/models/db"
)

// GetRepositoriesByForkID returns all repositories with given fork ID.
func GetRepositoriesByForkID(ctx context.Context, forkID int64) ([]*Repository, error) {
	repos := make([]*Repository, 0, 10)
	return repos, db.GetEngine(ctx).
		Where("fork_id=?", forkID).
		Find(&repos)
}

// GetUserFork return user forked repository from this repository, if not forked return nil
func GetUserFork(ctx context.Context, repoID, userID int64) (*Repository, error) {
	var forkedRepo Repository
	has, err := db.GetEngine(ctx).Where("fork_id = ?", repoID).And("owner_id = ?", userID).Get(&forkedRepo)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, nil //nolint:nilnil // return nil to indicate that the object does not exist
	}
	return &forkedRepo, nil
}

// IncrementRepoForkNum increment repository fork number
func IncrementRepoForkNum(ctx context.Context, repoID int64) error {
	_, err := db.GetEngine(ctx).Exec("UPDATE `repository` SET num_forks=num_forks+1 WHERE id=?", repoID)
	return err
}
