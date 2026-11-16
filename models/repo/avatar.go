// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import "context"

// AvatarLink returns an empty link: avatars are not served by this backend
func (repo *Repository) AvatarLink(_ context.Context) string {
	return ""
}
