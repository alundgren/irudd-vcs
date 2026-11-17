// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package cmd

import (
	"testing"

	auth_model "gitea.dev/models/auth"
	"gitea.dev/models/db"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminUserCreate(t *testing.T) {
	reset := func() {
		require.NoError(t, db.TruncateBeans(t.Context(), &user_model.User{}))
		require.NoError(t, db.TruncateBeans(t.Context(), &user_model.EmailAddress{}))
		require.NoError(t, db.TruncateBeans(t.Context(), &auth_model.AccessToken{}))
	}

	createUser := func(name string, args ...string) error {
		return microcmdUserCreate().Run(t.Context(), append([]string{"create", "--username", name, "--email", name + "@gitea.local"}, args...))
	}

	t.Run("SingleOwner", func(t *testing.T) {
		reset()
		require.NoError(t, createUser("u"))
		u := unittest.AssertExistsAndLoadBean(t, &user_model.User{LowerName: "u"})
		assert.True(t, u.IsAdmin, "the owner is the site admin")
		assert.Equal(t, user_model.UserTypeIndividual, u.Type)

		assert.ErrorContains(t, createUser("u2"), "there can only be one user")
		assert.Equal(t, 1, unittest.GetCount(t, &user_model.User{}))
	})

	t.Run("AccessToken", func(t *testing.T) {
		// the default token has "all" access
		reset()
		assert.NoError(t, createUser("u"))
		assert.Equal(t, 1, unittest.GetCount(t, &auth_model.AccessToken{}))
		accessToken := unittest.AssertExistsAndLoadBean(t, &auth_model.AccessToken{Name: "gitea-admin"})
		hasScopes, err := accessToken.Scope.HasScope(auth_model.AccessTokenScopeWriteAdmin, auth_model.AccessTokenScopeWriteRepository)
		assert.NoError(t, err)
		assert.True(t, hasScopes)

		// name & scopes
		reset()
		assert.NoError(t, createUser("u", "--access-token-name", "new-token-name", "--access-token-scopes", "read:issue,read:user"))
		accessToken = unittest.AssertExistsAndLoadBean(t, &auth_model.AccessToken{Name: "new-token-name"})
		hasScopes, err = accessToken.Scope.HasScope(auth_model.AccessTokenScopeReadIssue, auth_model.AccessTokenScopeReadUser)
		assert.NoError(t, err)
		assert.True(t, hasScopes)
		hasScopes, err = accessToken.Scope.HasScope(auth_model.AccessTokenScopeWriteAdmin, auth_model.AccessTokenScopeWriteRepository)
		assert.NoError(t, err)
		assert.False(t, hasScopes)

		// empty permission
		reset()
		err = createUser("u", "--access-token-scopes", "public-only")
		assert.Equal(t, 0, unittest.GetCount(t, &user_model.User{}))
		assert.Equal(t, 0, unittest.GetCount(t, &auth_model.AccessToken{}))
		assert.ErrorContains(t, err, "access token does not have any permission")
	})

	t.Run("UserFields", func(t *testing.T) {
		reset()
		assert.NoError(t, createUser("u-FullNameWithSpace", "--fullname", "First O'Middle Last"))
		unittest.AssertExistsAndLoadBean(t, &user_model.User{
			Name:      "u-FullNameWithSpace",
			LowerName: "u-fullnamewithspace",
			FullName:  "First O'Middle Last",
			Email:     "u-FullNameWithSpace@gitea.local",
		})
	})
}
