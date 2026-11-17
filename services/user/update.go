// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package user

import (
	"context"

	user_model "gitea.dev/models/user"
	"gitea.dev/modules/optional"
)

// UpdateOptions are the user settings a user can change through the API
type UpdateOptions struct {
	KeepEmailPrivate    optional.Option[bool]
	FullName            optional.Option[string]
	Website             optional.Option[string]
	Location            optional.Option[string]
	Description         optional.Option[string]
	Language            optional.Option[string]
	Theme               optional.Option[string]
	DiffViewStyle       optional.Option[string]
	KeepActivityPrivate optional.Option[bool]
}

func UpdateUser(ctx context.Context, u *user_model.User, opts *UpdateOptions) error {
	cols := make([]string, 0, 9)

	if opts.KeepEmailPrivate.Has() {
		u.KeepEmailPrivate = opts.KeepEmailPrivate.Value()
		cols = append(cols, "keep_email_private")
	}
	if opts.FullName.Has() {
		u.FullName = opts.FullName.Value()
		cols = append(cols, "full_name")
	}
	if opts.Website.Has() {
		u.Website = opts.Website.Value()
		cols = append(cols, "website")
	}
	if opts.Location.Has() {
		u.Location = opts.Location.Value()
		cols = append(cols, "location")
	}
	if opts.Description.Has() {
		u.Description = opts.Description.Value()
		cols = append(cols, "description")
	}
	if opts.Language.Has() {
		u.Language = opts.Language.Value()
		cols = append(cols, "language")
	}
	if opts.Theme.Has() {
		u.Theme = opts.Theme.Value()
		cols = append(cols, "theme")
	}
	if opts.DiffViewStyle.Has() {
		u.DiffViewStyle = opts.DiffViewStyle.Value()
		cols = append(cols, "diff_view_style")
	}
	if opts.KeepActivityPrivate.Has() {
		u.KeepActivityPrivate = opts.KeepActivityPrivate.Value()
		cols = append(cols, "keep_activity_private")
	}

	return user_model.UpdateUserCols(ctx, u, cols...)
}
