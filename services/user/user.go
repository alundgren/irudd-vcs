// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package user

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"gitea.dev/models/db"
	repo_model "gitea.dev/models/repo"
	system_model "gitea.dev/models/system"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/log"
	"gitea.dev/modules/util"
	repo_service "gitea.dev/services/repository"
)

// RenameUser renames a user
func RenameUser(ctx context.Context, u *user_model.User, newUserName string, doer *user_model.User) error {
	if newUserName == u.Name {
		return nil
	}

	// Non-local users are not allowed to change their own username, but admins are
	isExternalUser := !u.IsOrganization() && !u.IsLocal()
	if isExternalUser && !doer.IsAdmin {
		return user_model.ErrUserIsNotLocal{UID: u.ID, Name: u.Name}
	}

	if err := user_model.IsUsableUsername(newUserName); err != nil {
		return err
	}

	onlyCapitalization := strings.EqualFold(newUserName, u.Name)
	oldUserName := u.Name

	if onlyCapitalization {
		u.Name = newUserName
		if err := user_model.UpdateUserCols(ctx, u, "name"); err != nil {
			u.Name = oldUserName
			return err
		}
		return repo_model.UpdateRepositoryOwnerNames(ctx, u.ID, newUserName)
	}

	ctx, committer, err := db.TxContext(ctx)
	if err != nil {
		return err
	}
	defer committer.Close()

	isExist, err := user_model.IsUserExist(ctx, u.ID, newUserName)
	if err != nil {
		return err
	}
	if isExist {
		return user_model.ErrUserAlreadyExist{
			Name: newUserName,
		}
	}

	if err = repo_model.UpdateRepositoryOwnerName(ctx, oldUserName, newUserName); err != nil {
		return err
	}

	if err = user_model.NewUserRedirect(ctx, u.ID, oldUserName, newUserName); err != nil {
		return err
	}

	u.Name = newUserName
	u.LowerName = strings.ToLower(newUserName)
	if err := user_model.UpdateUserCols(ctx, u, "name", "lower_name"); err != nil {
		u.Name = oldUserName
		u.LowerName = strings.ToLower(oldUserName)
		return err
	}

	// Do not fail if directory does not exist
	if err = util.Rename(user_model.UserPath(oldUserName), user_model.UserPath(newUserName)); err != nil && !os.IsNotExist(err) {
		u.Name = oldUserName
		u.LowerName = strings.ToLower(oldUserName)
		return fmt.Errorf("rename user directory: %w", err)
	}

	if err = committer.Commit(); err != nil {
		u.Name = oldUserName
		u.LowerName = strings.ToLower(oldUserName)
		if err2 := util.Rename(user_model.UserPath(newUserName), user_model.UserPath(oldUserName)); err2 != nil && !os.IsNotExist(err2) {
			log.Error("Unable to rollback directory change during failed username change from: %s to: %s. DB Error: %v. Filesystem Error: %v", oldUserName, newUserName, err, err2)
			return fmt.Errorf("failed to rollback directory change during failed username change from: %s to: %s. DB Error: %w. Filesystem Error: %v", oldUserName, newUserName, err, err2)
		}
		return err
	}
	return nil
}

// DeleteUser completely and permanently deletes everything of a user,
// but issues/comments/pulls will be kept and shown as someone has been deleted,
// unless the user is younger than USER_DELETE_WITH_COMMENTS_MAX_DAYS.
func DeleteUser(ctx context.Context, u *user_model.User, purge bool) error {
	if u.IsOrganization() {
		return fmt.Errorf("%s is an organization not a user", u.Name)
	}

	if u.IsActive && user_model.IsLastAdminUser(ctx, u) {
		return user_model.ErrDeleteLastAdminUser{UID: u.ID}
	}

	if purge {
		// Disable the user first
		// NOTE: This is deliberately not within a transaction as it must disable the user immediately to prevent any further action by the user to be purged.
		if err := user_model.UpdateUserCols(ctx, &user_model.User{
			ID:              u.ID,
			IsActive:        false,
			IsRestricted:    true,
			IsAdmin:         false,
			ProhibitLogin:   true,
			Passwd:          "",
			Salt:            "",
			PasswdHashAlgo:  "",
			MaxRepoCreation: 0,
		}, "is_active", "is_restricted", "is_admin", "prohibit_login", "max_repo_creation", "passwd", "salt", "passwd_hash_algo"); err != nil {
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
