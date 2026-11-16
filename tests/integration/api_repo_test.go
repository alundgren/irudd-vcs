// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	auth_model "gitea.dev/models/auth"
	"gitea.dev/models/db"
	access_model "gitea.dev/models/perm/access"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/setting"
	api "gitea.dev/modules/structs"
	"gitea.dev/modules/test"
	"gitea.dev/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"xorm.io/builder"
)

func TestAPIUserReposNotLogin(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})

	req := NewRequestf(t, "GET", "/api/v1/users/%s/repos", user.Name)
	MakeRequest(t, req, http.StatusUnauthorized)
}

func TestAPISearchRepo(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	const keyword = "test"

	req := NewRequestf(t, "GET", "/api/v1/repos/search?q=%s", keyword)
	MakeRequest(t, req, http.StatusUnauthorized)

	req = NewRequestf(t, "GET", "/api/v1/repos/search?q=%s&private=false", keyword).AddTokenAuth(getUserToken(t, "user2", auth_model.AccessTokenScopeReadRepository))
	resp := MakeRequest(t, req, http.StatusOK)

	body := DecodeJSON(t, resp, &api.SearchResults{})
	assert.NotEmpty(t, body.Data)
	for _, repo := range body.Data {
		assert.Contains(t, repo.Name, keyword)
		assert.False(t, repo.Private)
	}

	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 15})
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 16})
	org3 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 18})
	user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 20})
	orgUser := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 17})

	defer test.MockVariableValue(&setting.API.DefaultPagingNum, 10)()

	// Map of expected results, where key is user for login
	type expectedResults map[*user_model.User]struct {
		count           int
		repoOwnerID     int64
		repoName        string
		includesPrivate bool
	}

	// anonymous requests are rejected before searching, so only signed-in users are listed;
	// an owner filter lists exactly that owner's repos, private ones included, for any signed-in user
	testCases := []struct {
		name, requestURL string
		expectedResults
	}{
		{
			name: "RepositoriesMax50", requestURL: "/api/v1/repos/search?limit=50&private=false", expectedResults: expectedResults{
				user:  {count: 36},
				user2: {count: 36},
			},
		},
		{
			name: "RepositoriesMax10", requestURL: "/api/v1/repos/search?limit=10&private=false", expectedResults: expectedResults{
				user:  {count: 10},
				user2: {count: 10},
			},
		},
		{
			name: "RepositoriesDefault", requestURL: "/api/v1/repos/search?default&private=false", expectedResults: expectedResults{
				user:  {count: 10},
				user2: {count: 10},
			},
		},
		{
			name: "RepositoriesByName", requestURL: fmt.Sprintf("/api/v1/repos/search?q=%s&private=false", "big_test_"), expectedResults: expectedResults{
				user:  {count: 7, repoName: "big_test_"},
				user2: {count: 7, repoName: "big_test_"},
			},
		},
		{
			name: "RepositoriesByName", requestURL: fmt.Sprintf("/api/v1/repos/search?q=%s&private=false", "user2/big_test_"), expectedResults: expectedResults{
				user2: {count: 2, repoName: "big_test_"},
			},
		},
		{
			name: "RepositoriesAccessibleAndRelatedToUser", requestURL: fmt.Sprintf("/api/v1/repos/search?uid=%d", user.ID), expectedResults: expectedResults{
				user:  {count: 4, repoOwnerID: user.ID, includesPrivate: true},
				user2: {count: 4, repoOwnerID: user.ID, includesPrivate: true},
			},
		},
		{
			name: "RepositoriesAccessibleAndRelatedToUser2", requestURL: fmt.Sprintf("/api/v1/repos/search?uid=%d", user2.ID), expectedResults: expectedResults{
				user:  {count: 2, repoOwnerID: user2.ID, includesPrivate: true},
				user2: {count: 2, repoOwnerID: user2.ID, includesPrivate: true},
				user4: {count: 2, repoOwnerID: user2.ID, includesPrivate: true},
			},
		},
		{
			name: "RepositoriesAccessibleAndRelatedToUser3", requestURL: fmt.Sprintf("/api/v1/repos/search?uid=%d", org3.ID), expectedResults: expectedResults{
				user:  {count: 0},
				user2: {count: 0},
				org3:  {count: 0},
			},
		},
		{
			name: "RepositoriesOwnedByOrganization", requestURL: fmt.Sprintf("/api/v1/repos/search?uid=%d", orgUser.ID), expectedResults: expectedResults{
				user:  {count: 2, repoOwnerID: orgUser.ID, includesPrivate: true},
				user2: {count: 2, repoOwnerID: orgUser.ID, includesPrivate: true},
			},
		},
		{name: "RepositoriesAccessibleAndRelatedToUser4", requestURL: fmt.Sprintf("/api/v1/repos/search?uid=%d", user4.ID), expectedResults: expectedResults{
			user:  {count: 4, repoOwnerID: user4.ID, includesPrivate: true},
			user4: {count: 4, repoOwnerID: user4.ID, includesPrivate: true},
		}},
		{name: "RepositoriesAccessibleAndRelatedToUser4/SearchModeSource", requestURL: fmt.Sprintf("/api/v1/repos/search?uid=%d&mode=%s", user4.ID, "source"), expectedResults: expectedResults{
			user:  {count: 0},
			user4: {count: 0},
		}},
		{name: "RepositoriesAccessibleAndRelatedToUser4/SearchModeFork", requestURL: fmt.Sprintf("/api/v1/repos/search?uid=%d&mode=%s", user4.ID, "fork"), expectedResults: expectedResults{
			user:  {count: 2, includesPrivate: true},
			user4: {count: 2, includesPrivate: true},
		}},
		{name: "RepositoriesAccessibleAndRelatedToUser4/SearchModeFork/Exclusive", requestURL: fmt.Sprintf("/api/v1/repos/search?uid=%d&mode=%s&exclusive=1", user4.ID, "fork"), expectedResults: expectedResults{
			user:  {count: 2, includesPrivate: true},
			user4: {count: 2, includesPrivate: true},
		}},
		{name: "RepositoriesAccessibleAndRelatedToUser4/SearchModeMirror", requestURL: fmt.Sprintf("/api/v1/repos/search?uid=%d&mode=%s", user4.ID, "mirror"), expectedResults: expectedResults{
			user:  {count: 2, includesPrivate: true},
			user4: {count: 2, includesPrivate: true},
		}},
		{name: "RepositoriesAccessibleAndRelatedToUser4/SearchModeMirror/Exclusive", requestURL: fmt.Sprintf("/api/v1/repos/search?uid=%d&mode=%s&exclusive=1", user4.ID, "mirror"), expectedResults: expectedResults{
			user:  {count: 2, includesPrivate: true},
			user4: {count: 2, includesPrivate: true},
		}},
		{name: "RepositoriesAccessibleAndRelatedToUser4/SearchModeCollaborative", requestURL: fmt.Sprintf("/api/v1/repos/search?uid=%d&mode=%s", user4.ID, "collaborative"), expectedResults: expectedResults{
			user:  {count: 0},
			user4: {count: 0},
		}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			for userToLogin, expected := range testCase.expectedResults {
				var testName string
				var userID int64
				var token string
				testName = fmt.Sprintf("LoggedUser%d", userToLogin.ID)
				session := loginUser(t, userToLogin.Name)
				token = getTokenForLoggedInUser(t, session, auth_model.AccessTokenScopeReadRepository)
				userID = userToLogin.ID

				t.Run(testName, func(t *testing.T) {
					request := NewRequest(t, "GET", testCase.requestURL).
						AddTokenAuth(token)
					response := MakeRequest(t, request, http.StatusOK)

					body := DecodeJSON(t, response, &api.SearchResults{})

					repoNames := make([]string, 0, len(body.Data))
					for _, repo := range body.Data {
						repoNames = append(repoNames, fmt.Sprintf("%d:%s:%t", repo.ID, repo.FullName, repo.Private))
					}
					assert.Len(t, repoNames, expected.count)
					for _, repo := range body.Data {
						r := getRepo(t, repo.ID)
						hasAccess, err := access_model.HasAnyUnitAccess(t.Context(), userID, r)
						assert.NoError(t, err, "Error when checking if User: %d has access to %s: %v", userID, repo.FullName, err)
						assert.True(t, hasAccess, "User: %d does not have access to %s", userID, repo.FullName)

						assert.NotEmpty(t, repo.Name)
						assert.Equal(t, repo.Name, r.Name)

						if len(expected.repoName) > 0 {
							assert.Contains(t, repo.Name, expected.repoName)
						}

						if expected.repoOwnerID > 0 {
							assert.Equal(t, expected.repoOwnerID, repo.Owner.ID)
						}

						if !expected.includesPrivate {
							assert.False(t, repo.Private, "User: %d not expecting private repository: %s", userID, repo.FullName)
						}
					}
				})
			}
		})
	}
}

var repoCache = make(map[int64]*repo_model.Repository)

func getRepo(t *testing.T, repoID int64) *repo_model.Repository {
	if _, ok := repoCache[repoID]; !ok {
		repoCache[repoID] = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repoID})
	}
	return repoCache[repoID]
}

func TestAPIViewRepo(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	req := NewRequest(t, "GET", "/api/v1/repos/user2/repo1")
	resp := MakeRequest(t, req, http.StatusOK)
	repo := DecodeJSON(t, resp, &api.Repository{})
	assert.EqualValues(t, 1, repo.ID)
	assert.Equal(t, "repo1", repo.Name)
	assert.Equal(t, 2, repo.Releases)
	assert.Equal(t, 1, repo.OpenIssues)
	assert.Equal(t, 3, repo.OpenPulls)

	req = NewRequest(t, "GET", "/api/v1/repos/user12/repo10")
	resp = MakeRequest(t, req, http.StatusOK)
	repo = DecodeJSON(t, resp, &api.Repository{})
	assert.EqualValues(t, 10, repo.ID)
	assert.Equal(t, "repo10", repo.Name)
	assert.Equal(t, 1, repo.OpenPulls)
	assert.Equal(t, 1, repo.Forks)

	req = NewRequest(t, "GET", "/api/v1/repos/user5/repo4")
	resp = MakeRequest(t, req, http.StatusOK)
	repo = DecodeJSON(t, resp, &api.Repository{})
	assert.EqualValues(t, 4, repo.ID)
	assert.Equal(t, "repo4", repo.Name)
	assert.Equal(t, 1, repo.Stars)
}

func TestAPIGetRepoByIDUnauthorized(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	session := loginUser(t, user.Name)
	token := getTokenForLoggedInUser(t, session, auth_model.AccessTokenScopeReadRepository)
	req := NewRequest(t, "GET", "/api/v1/repositories/2").
		AddTokenAuth(token)
	MakeRequest(t, req, http.StatusNotFound)
}

func TestAPIRepoCreateConflict(t *testing.T) {
	onGiteaRun(t, testAPIRepoCreateConflict)
}

func testAPIRepoCreateConflict(t *testing.T, u *url.URL) {
	username := "user2"
	baseAPITestContext := NewAPITestContext(t, username, "repo1", auth_model.AccessTokenScopeWriteRepository, auth_model.AccessTokenScopeWriteUser)

	u.Path = baseAPITestContext.GitPath()

	t.Run("Existing", func(t *testing.T) {
		httpContext := baseAPITestContext

		httpContext.Reponame = "repo-tmp-17"
		t.Run("CreateRepo", doAPICreateRepository(httpContext, false))

		req := NewRequestWithJSON(t, "POST", "/api/v1/user/repos",
			&api.CreateRepoOption{
				Name: httpContext.Reponame,
			}).
			AddTokenAuth(httpContext.Token)
		resp := httpContext.Session.MakeRequest(t, req, http.StatusConflict)
		respJSON := DecodeJSON(t, resp, map[string]string{})
		assert.Equal(t, "The repository with the same name already exists.", respJSON["message"])
	})
}

func countActiveUsers(t *testing.T) int {
	n, err := db.GetEngine(t.Context()).Where(builder.Eq{"type": user_model.UserTypeIndividual, "is_active": true}).Count(&user_model.User{})
	require.NoError(t, err)
	return int(n)
}

func TestAPIRepoGetReviewers(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	session := loginUser(t, user.Name)
	token := getTokenForLoggedInUser(t, session, auth_model.AccessTokenScopeReadRepository)
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	req := NewRequestf(t, "GET", "/api/v1/repos/%s/%s/reviewers", user.Name, repo.Name).
		AddTokenAuth(token)
	resp := MakeRequest(t, req, http.StatusOK)
	reviewers := DecodeJSON(t, resp, []*api.User{})
	assert.Len(t, reviewers, countActiveUsers(t)) // every active user can review
}

func TestAPIRepoGetAssignees(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	session := loginUser(t, user.Name)
	token := getTokenForLoggedInUser(t, session, auth_model.AccessTokenScopeReadRepository)
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2})

	req := NewRequestf(t, "GET", "/api/v1/repos/%s/%s/assignees", user.Name, repo.Name).
		AddTokenAuth(token)
	resp := MakeRequest(t, req, http.StatusOK)
	assignees := DecodeJSON(t, resp, []*api.User{})
	assert.Len(t, assignees, countActiveUsers(t)) // every active user can be assigned

	assignee := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	req = NewRequestf(t, "GET", "/api/v1/repos/%s/%s/assignees/%s", user.Name, repo.Name, assignee.Name).
		AddTokenAuth(token)
	MakeRequest(t, req, http.StatusNoContent)

	nonAssignee := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 3}) // an organization row
	req = NewRequestf(t, "GET", "/api/v1/repos/%s/%s/assignees/%s", user.Name, repo.Name, nonAssignee.Name).
		AddTokenAuth(token)
	MakeRequest(t, req, http.StatusBadRequest) // organizations cannot be assigned

	req = NewRequestf(t, "GET", "/api/v1/repos/%s/%s/assignees/%s", user.Name, repo.Name, "org3").
		AddTokenAuth(token)
	MakeRequest(t, req, http.StatusBadRequest)
}
