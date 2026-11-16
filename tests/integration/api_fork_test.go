// Copyright 2017 The Gogs Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"testing"

	auth_model "gitea.dev/models/auth"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	api "gitea.dev/modules/structs"
	"gitea.dev/tests"

	"github.com/stretchr/testify/assert"
)

func TestAPIFork(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	t.Run("CreateForkNoLogin", testCreateForkNoLogin)
	t.Run("CreateForkIntoOrgRejected", testCreateForkIntoOrgRejected)
	t.Run("APIForkList", testAPIForkList)
	t.Run("GetPrivateReposForks", testGetPrivateReposForks)
}

func testCreateForkNoLogin(t *testing.T) {
	req := NewRequestWithJSON(t, "POST", "/api/v1/repos/user2/repo1/forks", &api.CreateForkOption{})
	MakeRequest(t, req, http.StatusUnauthorized)
}

func testCreateForkIntoOrgRejected(t *testing.T) {
	user4Token := getUserToken(t, "user4", auth_model.AccessTokenScopeWriteRepository)
	org := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 3})
	req := NewRequestWithJSON(t, "POST", "/api/v1/repos/user2/repo1/forks", &api.CreateForkOption{
		Organization: &org.Name,
	}).AddTokenAuth(user4Token)
	MakeRequest(t, req, http.StatusUnprocessableEntity)
}

func testAPIForkList(t *testing.T) {
	user1Token := getUserToken(t, "user1", auth_model.AccessTokenScopeWriteRepository)
	user4Token := getUserToken(t, "user4", auth_model.AccessTokenScopeWriteRepository)
	for _, token := range []string{user1Token, user4Token} {
		req := NewRequestWithJSON(t, "POST", "/api/v1/repos/user2/repo1/forks", &api.CreateForkOption{}).AddTokenAuth(token)
		MakeRequest(t, req, http.StatusAccepted)
	}

	t.Run("Anonymous", func(t *testing.T) {
		defer tests.PrintCurrentTest(t)()

		req := NewRequest(t, "GET", "/api/v1/repos/user2/repo1/forks")
		MakeRequest(t, req, http.StatusUnauthorized)
	})

	t.Run("LoggedIn", func(t *testing.T) {
		defer tests.PrintCurrentTest(t)()

		req := NewRequest(t, "GET", "/api/v1/repos/user2/repo1/forks").AddTokenAuth(user1Token)
		resp := MakeRequest(t, req, http.StatusOK)

		forks := DecodeJSON(t, resp, []*api.Repository{})
		assert.Len(t, forks, 2)
		assert.Equal(t, "2", resp.Header().Get("X-Total-Count"))
	})

	t.Run("RespHeaderLinks", func(t *testing.T) {
		t.Run("Page1", func(t *testing.T) {
			defer tests.PrintCurrentTest(t)()

			req := NewRequest(t, "GET", "/api/v1/repos/user2/repo1/forks?page=1&limit=1").AddTokenAuth(user1Token)
			resp := MakeRequest(t, req, http.StatusOK)
			assert.Equal(t, "2", resp.Header().Get("X-Total-Count"))

			linkHeader := resp.Header().Get("Link")
			assert.NotEmpty(t, linkHeader, "Link header should not be empty")
			assert.Contains(t, linkHeader, `rel="next"`)
			assert.Contains(t, linkHeader, `rel="last"`)
			assert.Contains(t, linkHeader, `/api/v1/repos/user2/repo1/forks?limit=1&page=2>`)

			forks := DecodeJSON(t, resp, []*api.Repository{})
			assert.Len(t, forks, 1)
		})

		t.Run("Page2", func(t *testing.T) {
			defer tests.PrintCurrentTest(t)()

			req := NewRequest(t, "GET", "/api/v1/repos/user2/repo1/forks?page=2&limit=1").AddTokenAuth(user1Token)
			resp := MakeRequest(t, req, http.StatusOK)
			assert.Equal(t, "2", resp.Header().Get("X-Total-Count"))

			forks := DecodeJSON(t, resp, []*api.Repository{})
			assert.Len(t, forks, 1)
		})
	})
}

func testGetPrivateReposForks(t *testing.T) {
	repo2 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2}) // private repository
	user1Token := getUserToken(t, "user1", auth_model.AccessTokenScopeWriteRepository)

	req := NewRequestWithJSON(t, "POST", "/api/v1/repos/"+repo2.FullName()+"/forks", &api.CreateForkOption{
		Name: new("forked-repo"),
	}).AddTokenAuth(user1Token)
	MakeRequest(t, req, http.StatusAccepted)

	req = NewRequest(t, "GET", "/api/v1/repos/"+repo2.FullName()+"/forks").AddTokenAuth(user1Token)
	resp := MakeRequest(t, req, http.StatusOK)

	forks := DecodeJSON(t, resp, []*api.Repository{})
	assert.Len(t, forks, 1)
	assert.Equal(t, "1", resp.Header().Get("X-Total-Count"))
	assert.Equal(t, "forked-repo", forks[0].Name)
	assert.Equal(t, "user1", forks[0].Owner.UserName)
}
