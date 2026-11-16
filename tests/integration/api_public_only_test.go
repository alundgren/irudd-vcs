// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"testing"

	auth_model "gitea.dev/models/auth"
	api "gitea.dev/modules/structs"
	"gitea.dev/tests"

	"github.com/stretchr/testify/assert"
)

func TestAPIUserReposPublicOnly(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	token := getUserToken(t, "user2", auth_model.AccessTokenScopeReadUser, auth_model.AccessTokenScopeReadRepository, auth_model.AccessTokenScopePublicOnly)
	req := NewRequest(t, "GET", "/api/v1/user/repos").
		AddTokenAuth(token)
	resp := MakeRequest(t, req, http.StatusOK)

	var repos []api.Repository
	DecodeJSON(t, resp, &repos)
	assert.NotEmpty(t, repos)
	for _, repo := range repos {
		assert.False(t, repo.Private)
	}
	assert.NotContains(t, repoNames(repos), "user2/repo2")

	req = NewRequest(t, "GET", "/api/v1/users/user2/repos").
		AddTokenAuth(token)
	resp = MakeRequest(t, req, http.StatusOK)
	DecodeJSON(t, resp, &repos)
	assert.NotEmpty(t, repos)
	for _, repo := range repos {
		assert.False(t, repo.Private)
	}
	assert.NotContains(t, repoNames(repos), "user2/repo2")
}

func repoNames(repos []api.Repository) []string {
	names := make([]string, 0, len(repos))
	for _, repo := range repos {
		names = append(names, repo.FullName)
	}
	return names
}

func TestAPIRepoByIDPublicOnly(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	token := getUserToken(t, "user2", auth_model.AccessTokenScopeReadRepository, auth_model.AccessTokenScopePublicOnly)
	req := NewRequest(t, "GET", "/api/v1/repositories/1").
		AddTokenAuth(token)
	MakeRequest(t, req, http.StatusOK)

	req = NewRequest(t, "GET", "/api/v1/repositories/2").
		AddTokenAuth(token)
	MakeRequest(t, req, http.StatusNotFound)
}

// TestAPIRepoLimitedOwnerPublicOnly ensures a public-only token cannot reach a public repo
// owned by a limited-visibility owner (which is not reachable anonymously).
func TestAPIRepoLimitedOwnerPublicOnly(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// limited_org (limited visibility) owns the non-private repo public_repo_on_limited_org
	const url = "/api/v1/repos/limited_org/public_repo_on_limited_org"

	// a public-only token is confined to genuinely public resources
	publicOnlyToken := getUserToken(t, "user2", auth_model.AccessTokenScopeReadRepository, auth_model.AccessTokenScopePublicOnly)
	req := NewRequest(t, "GET", url).AddTokenAuth(publicOnlyToken)
	MakeRequest(t, req, http.StatusNotFound)

	// a normal token can still reach the limited owner's public repo
	token := getUserToken(t, "user2", auth_model.AccessTokenScopeReadRepository)
	req = NewRequest(t, "GET", url).AddTokenAuth(token)
	MakeRequest(t, req, http.StatusOK)
}
