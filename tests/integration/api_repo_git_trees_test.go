// Copyright 2019 The Gitea Authors. All rights reserved.
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
	"github.com/stretchr/testify/require"
)

func TestAPIReposGitTrees(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})         // owner of the repo1 & repo16
	org3 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 3})          // owner of the repo3
	user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})         // owner of neither repos
	repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})   // public repo
	repo3 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 3})   // public repo
	repo16 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 16}) // private repo
	repo1TreeSHA := "65f1bf27bc3bf70f64657658635e66094edbcb4d"
	repo3TreeSHA := "2a47ca4b614a9f5a43abbd5ad851a54a616ffee6"
	repo16TreeSHA := "69554a64c1e6030f051e5c3f94bfbd773cd6a324"
	badSHA := "0000000000000000000000000000000000000000"

	// Login as User2.
	session := loginUser(t, user2.Name)
	token := getTokenForLoggedInUser(t, session, auth_model.AccessTokenScopeReadRepository)

	// Test a public repo that anyone can GET the tree of
	_ = session.MakeRequest(t, NewRequest(t, "GET", "/api/v1/repos/user2/repo1/git/trees/master"), http.StatusOK)

	resp := session.MakeRequest(t, NewRequest(t, "GET", "/api/v1/repos/user2/repo1/git/trees/62fb502a7172d4453f0322a2cc85bddffa57f07a?per_page=1"), http.StatusOK)
	respGitTree := DecodeJSON(t, resp, &api.GitTreeResponse{})
	assert.True(t, respGitTree.Truncated)
	require.Len(t, respGitTree.Entries, 1)
	assert.Equal(t, "File-WoW", respGitTree.Entries[0].Path)

	resp = session.MakeRequest(t, NewRequest(t, "GET", "/api/v1/repos/user2/repo1/git/trees/62fb502a7172d4453f0322a2cc85bddffa57f07a?page=2&per_page=1"), http.StatusOK)
	respGitTree = DecodeJSON(t, resp, &api.GitTreeResponse{})
	assert.False(t, respGitTree.Truncated)
	require.Len(t, respGitTree.Entries, 1)
	assert.Equal(t, "README.md", respGitTree.Entries[0].Path)

	// Tests a private repo with no token so will fail
	for _, ref := range [...]string{
		"master",     // Branch
		repo1TreeSHA, // Tag
	} {
		req := NewRequestf(t, "GET", "/api/v1/repos/%s/%s/git/trees/%s", user2.Name, repo16.Name, ref)
		MakeRequest(t, req, http.StatusUnauthorized)
	}

	// Test using access token for a private repo that the user of the token owns
	req := NewRequestf(t, "GET", "/api/v1/repos/%s/%s/git/trees/%s", user2.Name, repo16.Name, repo16TreeSHA).
		AddTokenAuth(token)
	MakeRequest(t, req, http.StatusOK)

	// Test using bad sha
	req = NewRequestf(t, "GET", "/api/v1/repos/%s/%s/git/trees/%s", user2.Name, repo1.Name, badSHA).AddTokenAuth(token)
	MakeRequest(t, req, http.StatusBadRequest)

	// Test using org repo "org3/repo3" where user2 is a collaborator
	req = NewRequestf(t, "GET", "/api/v1/repos/%s/%s/git/trees/%s", org3.Name, repo3.Name, repo3TreeSHA).
		AddTokenAuth(token)
	MakeRequest(t, req, http.StatusOK)

	// Test using org repo "org3/repo3" with no user token
	req = NewRequestf(t, "GET", "/api/v1/repos/%s/%s/git/trees/%s", org3.Name, repo3TreeSHA, repo3.Name)
	MakeRequest(t, req, http.StatusUnauthorized)

	// Login as User4.
	session = loginUser(t, user4.Name)
	token4 := getTokenForLoggedInUser(t, session, auth_model.AccessTokenScopeAll)

	// Test using org repo "org3/repo3" by user4, who owns it like every signed-in user
	req = NewRequestf(t, "GET", "/api/v1/repos/%s/%s/git/trees/%s", org3.Name, repo3.Name, repo3TreeSHA).AddTokenAuth(token4)
	MakeRequest(t, req, http.StatusOK)
}
