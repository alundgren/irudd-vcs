// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"

	"gitea.dev/models/db"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unit"
)

// UpdateRepositoryUnits updates a repository's units
func UpdateRepositoryUnits(ctx context.Context, repo *repo_model.Repository, units []repo_model.RepoUnit, deleteUnitTypes []unit.Type) (err error) {
	return db.WithTx(ctx, func(ctx context.Context) error {
		// Delete existing settings of units before adding again
		for _, u := range units {
			deleteUnitTypes = append(deleteUnitTypes, u.Type)
		}

		if _, err = db.GetEngine(ctx).Where("repo_id = ?", repo.ID).In("type", deleteUnitTypes).Delete(new(repo_model.RepoUnit)); err != nil {
			return err
		}

		if len(units) > 0 {
			if err = db.Insert(ctx, units); err != nil {
				return err
			}
		}

		return nil
	})
}
