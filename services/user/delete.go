// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package user

import (
	"context"
	"fmt"
	"time"

	auth_model "gitea.dev/models/auth"
	"gitea.dev/models/db"
	git_model "gitea.dev/models/git"
	issues_model "gitea.dev/models/issues"
	"gitea.dev/models/organization"
	access_model "gitea.dev/models/perm/access"
	repo_model "gitea.dev/models/repo"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/setting"

	_ "image/jpeg" // Needed for jpeg support
)

// deleteUser deletes models associated to a user.
func deleteUser(ctx context.Context, u *user_model.User, purge bool) (err error) {
	e := db.GetEngine(ctx)

	if err = db.DeleteBeans(ctx,
		&auth_model.AccessToken{UID: u.ID},
		&repo_model.Collaboration{UserID: u.ID},
		&access_model.Access{UserID: u.ID},
		&issues_model.IssueUser{UID: u.ID},
		&user_model.EmailAddress{UID: u.ID},
		&issues_model.Reaction{UserID: u.ID},
		&organization.TeamUser{UID: u.ID},
		&user_model.Setting{UserID: u.ID},
		&user_model.Redirect{RedirectUserID: u.ID},
	); err != nil {
		return fmt.Errorf("deleteBeans: %w", err)
	}

	if purge || (setting.Service.UserDeleteWithCommentsMaxTime != 0 &&
		u.CreatedUnix.AsTime().Add(setting.Service.UserDeleteWithCommentsMaxTime).After(time.Now())) {
		// Delete Comments
		const batchSize = 50
		for {
			comments := make([]*issues_model.Comment, 0, batchSize)
			if err = e.Where("type=? AND poster_id=?", issues_model.CommentTypeComment, u.ID).Limit(batchSize, 0).Find(&comments); err != nil {
				return err
			}
			if len(comments) == 0 {
				break
			}

			for _, comment := range comments {
				if err = issues_model.DeleteComment(ctx, comment); err != nil {
					return err
				}
			}
		}

		// Delete Reactions
		if err = issues_model.DeleteReaction(ctx, &issues_model.ReactionOptions{DoerID: u.ID}); err != nil {
			return err
		}
	}

	// ***** START: Branch Protections *****
	{
		const batchSize = 50
		for start := 0; ; start += batchSize {
			protections := make([]*git_model.ProtectedBranch, 0, batchSize)
			// @perf: We can't filter on DB side by u.ID, as those IDs are serialized as JSON strings.
			//   We could filter down with `WHERE repo_id IN (reposWithPushPermission(u))`,
			//   though that query will be quite complex and tricky to maintain (compare `getRepoAssignees()`).
			// Also, as we didn't update branch protections when removing entries from `access` table,
			//   it's safer to iterate all protected branches.
			if err = e.Limit(batchSize, start).Find(&protections); err != nil {
				return fmt.Errorf("findProtectedBranches: %w", err)
			}
			if len(protections) == 0 {
				break
			}
			for _, p := range protections {
				if err := git_model.RemoveUserIDFromProtectedBranch(ctx, p, u.ID); err != nil {
					return err
				}
			}
		}
	}
	// ***** END: Branch Protections *****

	// Clear assignee.
	if _, err = db.DeleteByBean(ctx, &issues_model.IssueAssignees{AssigneeID: u.ID}); err != nil {
		return fmt.Errorf("clear assignee: %w", err)
	}

	if _, err = db.DeleteByID[user_model.User](ctx, u.ID); err != nil {
		return fmt.Errorf("delete: %w", err)
	}

	return nil
}
