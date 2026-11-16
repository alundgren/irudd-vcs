// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package user

import (
	"testing"

	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/optional"

	"github.com/stretchr/testify/assert"
)

func TestUpdateUser(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 28})

	opts := &UpdateOptions{
		KeepEmailPrivate:    optional.Some(false),
		FullName:            optional.Some("Changed Name"),
		Website:             optional.Some("https://gitea.com/"),
		Location:            optional.Some("location"),
		Description:         optional.Some("description"),
		KeepActivityPrivate: optional.Some(true),
		Language:            optional.Some("lang"),
		Theme:               optional.Some("theme"),
		DiffViewStyle:       optional.Some("split"),
	}
	assert.NoError(t, UpdateUser(t.Context(), user, opts))

	check := func(user *user_model.User) {
		assert.Equal(t, opts.KeepEmailPrivate.Value(), user.KeepEmailPrivate)
		assert.Equal(t, opts.FullName.Value(), user.FullName)
		assert.Equal(t, opts.Website.Value(), user.Website)
		assert.Equal(t, opts.Location.Value(), user.Location)
		assert.Equal(t, opts.Description.Value(), user.Description)
		assert.Equal(t, opts.KeepActivityPrivate.Value(), user.KeepActivityPrivate)
		assert.Equal(t, opts.Language.Value(), user.Language)
		assert.Equal(t, opts.Theme.Value(), user.Theme)
		assert.Equal(t, opts.DiffViewStyle.Value(), user.DiffViewStyle)
	}
	check(user)
	check(unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 28}))
}
