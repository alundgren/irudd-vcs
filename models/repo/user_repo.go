// Copyright 2022 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"context"
	"strings"

	"gitea.dev/models/db"
	user_model "gitea.dev/models/user"

	"xorm.io/builder"
)

// GetRepoAssignees returns all users that can be assigned to issues or pull-requests of the repository:
// every active individual user, since any signed-in user has full access
func GetRepoAssignees(ctx context.Context, _ *Repository) (users []*user_model.User, err error) {
	err = db.GetEngine(ctx).
		Where(builder.Eq{"`user`.is_active": true, "`user`.type": user_model.UserTypeIndividual}).
		OrderBy(user_model.GetOrderByName()).
		Find(&users)
	return users, err
}

// GetIssuePostersWithSearch returns users with limit of 30 whose username started with prefix that have authored an issue/pull request for the given repository
// It searches with the "user.name" and "user.full_name" fields case-insensitively.
func GetIssuePostersWithSearch(ctx context.Context, repo *Repository, isPull bool, search string) ([]*user_model.User, error) {
	users := make([]*user_model.User, 0, 30)

	cond := builder.In("`user`.id",
		builder.Select("poster_id").From("issue").Where(
			builder.Eq{"repo_id": repo.ID}.
				And(builder.Eq{"is_pull": isPull}),
		).GroupBy("poster_id"))

	if search != "" {
		var prefixCond builder.Cond = builder.Like{"lower_name", strings.ToLower(search) + "%"}
		prefixCond = prefixCond.Or(db.BuildCaseInsensitiveLike("full_name", "%"+search+"%"))
		cond = cond.And(prefixCond)
	}

	return users, db.GetEngine(ctx).
		Where(cond).
		Cols("id", "name", "full_name", "avatar", "avatar_email", "use_custom_avatar").
		OrderBy("name").
		Limit(30).
		Find(&users)
}
