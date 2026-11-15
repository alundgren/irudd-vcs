// Copyright 2022 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"context"
	"strings"

	"gitea.dev/models/db"
	"gitea.dev/models/organization"
	"gitea.dev/models/perm"
	"gitea.dev/models/unit"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/container"

	"xorm.io/builder"
)

// GetRepoAssignees returns all users that have write access and can be assigned to issues or pull-requests of the repository,
func GetRepoAssignees(ctx context.Context, repo *Repository) (_ []*user_model.User, err error) {
	if err = repo.LoadOwner(ctx); err != nil {
		return nil, err
	}

	e := db.GetEngine(ctx)
	userIDs := make([]int64, 0, 10)
	if err = e.Table("access").
		Where("repo_id = ? AND mode >= ?", repo.ID, perm.AccessModeWrite).
		Select("user_id").
		Find(&userIDs); err != nil {
		return nil, err
	}

	uniqueUserIDs := make(container.Set[int64])
	uniqueUserIDs.AddMultiple(userIDs...)

	if repo.Owner.IsOrganization() {
		// issues and pull requests both need "assignee list"
		additionalUserIDs, err := organization.GetTeamUserIDsWithAccessToAnyRepoUnit(ctx, repo.OwnerID, repo.ID, perm.AccessModeRead, unit.TypeIssues, unit.TypePullRequests)
		if err != nil {
			return nil, err
		}
		uniqueUserIDs.AddMultiple(additionalUserIDs...)
	}

	// Leave a seat for owner itself to append later, but if owner is an organization
	// and just waste 1 unit is cheaper than re-allocate memory once.
	users := make([]*user_model.User, 0, len(uniqueUserIDs)+1)
	if len(uniqueUserIDs) > 0 {
		if err = e.In("id", uniqueUserIDs.Values()).
			Where(builder.Eq{"`user`.is_active": true}).
			OrderBy(user_model.GetOrderByName()).
			Find(&users); err != nil {
			return nil, err
		}
	}
	if !repo.Owner.IsOrganization() && !uniqueUserIDs.Contains(repo.OwnerID) {
		users = append(users, repo.Owner)
	}

	return users, nil
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
