// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"testing"

	auth_model "gitea.dev/models/auth"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	api "gitea.dev/modules/structs"
	"gitea.dev/tests"

	"github.com/stretchr/testify/require"
)

func TestAPIRepositoryCreationTokenScopes(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{Name: "user2"})

	t.Run("organization creation requires repository scope", func(t *testing.T) {
		orgOnlyToken := getUserToken(t, user.Name, auth_model.AccessTokenScopeWriteOrganization)
		req := NewRequestWithJSON(t, "POST", "/api/v1/orgs/org3/repos", &api.CreateRepoOption{Name: "missing-repository-scope"}).
			AddTokenAuth(orgOnlyToken)
		MakeRequest(t, req, http.StatusForbidden)

		orgRepoToken := getUserToken(t, user.Name,
			auth_model.AccessTokenScopeWriteOrganization,
			auth_model.AccessTokenScopeWriteRepository,
		)
		req = NewRequestWithJSON(t, "POST", "/api/v1/orgs/org3/repos", &api.CreateRepoOption{Name: "repository-scope-allowed"}).
			AddTokenAuth(orgRepoToken)
		resp := MakeRequest(t, req, http.StatusCreated)
		createdRepo := DecodeJSON(t, resp, &api.Repository{})
		require.Equal(t, "org3/repository-scope-allowed", createdRepo.FullName)
	})
}
