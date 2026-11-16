// Copyright 2022 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"

	repo_model "gitea.dev/models/repo"
	user_model "gitea.dev/models/user"
)

// CanUserDelete returns true if user could delete the repository, which any signed-in user can
func CanUserDelete(_ context.Context, _ *repo_model.Repository, user *user_model.User) (bool, error) {
	return user != nil && user.ID > 0, nil
}
