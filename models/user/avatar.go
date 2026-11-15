// Copyright 2020 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package user

import "context"

// AvatarLinkWithSize returns an empty link: avatars are not served by this backend
func (u *User) AvatarLinkWithSize(_ context.Context, _ int) string {
	return ""
}

// AvatarLink returns an empty link: avatars are not served by this backend
func (u *User) AvatarLink(ctx context.Context) string {
	return u.AvatarLinkWithSize(ctx, 0)
}
