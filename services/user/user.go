// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package user

import (
	"context"
	"fmt"
	"time"

	"gitea.dev/models/db"
	repo_model "gitea.dev/models/repo"
	system_model "gitea.dev/models/system"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/log"
	"gitea.dev/modules/util"
	repo_service "gitea.dev/services/repository"
)

// DeleteUser completely and permanently deletes everything of a user,
// but issues/comments/pulls will be kept and shown as someone has been deleted,
// unless the user is younger than USER_DELETE_WITH_COMMENTS_MAX_DAYS.
func DeleteUser(ctx context.Context, u *user_model.User, purge bool) error {
	if u.IsOrganization() {
		return fmt.Errorf("%s is an organization not a user", u.Name)
	}

	if purge {
		// Disable the user first
		// NOTE: This is deliberately not within a transaction as it must disable the user immediately to prevent any further action by the user to be purged.
		if err := user_model.UpdateUserCols(ctx, &user_model.User{
			ID:              u.ID,
			IsActive:        false,
			IsAdmin:         false,
			ProhibitLogin:   true,
			MaxRepoCreation: 0,
		}, "is_active", "is_admin", "prohibit_login", "max_repo_creation"); err != nil {
			return fmt.Errorf("unable to disable user: %s[%d] prior to purge. UpdateUserCols: %w", u.Name, u.ID, err)
		}

		// Delete all repos belonging to this user
		// Now this is not within a transaction because there are internal transactions within the DeleteRepository
		// BUT: the db will still be consistent even if a number of repos have already been deleted.
		// And in fact we want to capture any repositories that are being created in other transactions in the meantime
		//
		// An alternative option here would be write a DeleteAllRepositoriesForUserID function which would delete all of the repos
		// but such a function would likely get out of date
		err := repo_service.DeleteOwnerRepositoriesDirectly(ctx, u)
		if err != nil {
			return err
		}
	}

	if err := db.WithTx(ctx, func(ctx context.Context) error {
		// Note: A user owns any repository
		//	cannot perform delete operation. This causes a race with the purge above
		//  however consistency requires that we ensure that this is the case

		// Check ownership of repository.
		count, err := repo_model.CountRepositories(ctx, repo_model.CountRepositoryOptions{OwnerID: u.ID})
		if err != nil {
			return fmt.Errorf("GetRepositoryCount: %w", err)
		} else if count > 0 {
			return repo_model.ErrUserOwnRepos{UID: u.ID}
		}

		if err := deleteUser(ctx, u, purge); err != nil {
			return fmt.Errorf("DeleteUser: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	// Note: There are something just cannot be roll back, so just keep error logs of those operations.
	path := user_model.UserPath(u.Name)
	if err := util.RemoveAll(path); err != nil {
		err = fmt.Errorf("failed to RemoveAll %s: %w", path, err)
		_ = system_model.CreateNotice(ctx, system_model.NoticeTask, fmt.Sprintf("delete user '%s': %v", u.Name, err))
	}

	return nil
}

// DeleteInactiveUsers deletes all inactive users and their email addresses.
func DeleteInactiveUsers(ctx context.Context, olderThan time.Duration) error {
	inactiveUsers, err := user_model.GetInactiveUsers(ctx, olderThan)
	if err != nil {
		return err
	}

	// FIXME: should only update authorized_keys file once after all deletions.
	for _, u := range inactiveUsers {
		if err = DeleteUser(ctx, u, false); err != nil {
			// Ignore inactive users that were ever active but then were set inactive by admin
			if repo_model.IsErrUserOwnRepos(err) {
				log.Warn("Inactive user %q has repositories, skipping deletion: %v", u.Name, err)
				continue
			}
			select {
			case <-ctx.Done():
				return db.ErrCancelledf("when deleting inactive user %q", u.Name)
			default:
				return err
			}
		}
	}
	return nil // TODO: there could be still inactive users left, and the number would increase gradually
}
