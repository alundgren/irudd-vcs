// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package pull

import (
	"context"

	"gitea.dev/models/db"
	repo_model "gitea.dev/models/repo"
	user_model "gitea.dev/models/user"

	"xorm.io/builder"
)

// GetReviewers get all users can be requested to review: every active user except the poster
func GetReviewers(ctx context.Context, _ *repo_model.Repository, _, posterID int64) ([]*user_model.User, error) {
	users := make([]*user_model.User, 0, 1)
	return users, db.GetEngine(ctx).
		Where(builder.Eq{"`user`.is_active": true, "`user`.type": user_model.UserTypeIndividual}).
		And(builder.Neq{"`user`.id": posterID}).
		OrderBy(user_model.GetOrderByName()).
		Find(&users)
}
