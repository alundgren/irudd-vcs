// Copyright 2018 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	auth_model "gitea.dev/models/auth"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	"gitea.dev/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type permission struct {
	category auth_model.AccessTokenScopeCategory
	level    auth_model.AccessTokenScopeLevel
}

type requiredScopeTestCase struct {
	url                 string
	method              string
	requiredPermissions []permission
}

func (c *requiredScopeTestCase) Name() string {
	return fmt.Sprintf("%v %v", c.method, c.url)
}

// TestAPIDeniesPermissionBasedOnTokenScope tests that API routes forbid access
// when the correct token scope is not included.
func TestAPIDeniesPermissionBasedOnTokenScope(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// We'll assert that each endpoint, when fetched with a token with all
	// scopes *except* the ones specified, a forbidden status code is returned.
	//
	// This is to protect against endpoints having their access check copied
	// from other endpoints and not updated.
	//
	// Test cases are in alphabetical order by URL.
	testCases := []requiredScopeTestCase{
		{
			"/api/v1/admin/emails",
			"GET",
			[]permission{
				{
					auth_model.AccessTokenScopeCategoryAdmin,
					auth_model.Read,
				},
			},
		},
		{
			"/api/v1/admin/users",
			"GET",
			[]permission{
				{
					auth_model.AccessTokenScopeCategoryAdmin,
					auth_model.Read,
				},
			},
		},
		{
			"/api/v1/admin/users",
			"POST",
			[]permission{
				{
					auth_model.AccessTokenScopeCategoryAdmin,
					auth_model.Write,
				},
			},
		},
		{
			"/api/v1/admin/users/user2",
			"PATCH",
			[]permission{
				{
					auth_model.AccessTokenScopeCategoryAdmin,
					auth_model.Write,
				},
			},
		},
		{
			"/api/v1/admin/users/user2/orgs",
			"GET",
			[]permission{
				{
					auth_model.AccessTokenScopeCategoryAdmin,
					auth_model.Read,
				},
			},
		},
		{
			"/api/v1/admin/users/user2/orgs",
			"POST",
			[]permission{
				{
					auth_model.AccessTokenScopeCategoryAdmin,
					auth_model.Write,
				},
			},
		},
		{
			"/api/v1/admin/orgs",
			"GET",
			[]permission{
				{
					auth_model.AccessTokenScopeCategoryAdmin,
					auth_model.Read,
				},
			},
		},
		{
			"/api/v1/org/org1/repos",
			"POST",
			[]permission{
				{
					auth_model.AccessTokenScopeCategoryOrganization,
					auth_model.Write,
				},
				{
					auth_model.AccessTokenScopeCategoryRepository,
					auth_model.Write,
				},
			},
		},
		{
			"/api/v1/repos/user1/repo1",
			"GET",
			[]permission{
				{
					auth_model.AccessTokenScopeCategoryRepository,
					auth_model.Read,
				},
			},
		},
		{
			"/api/v1/repos/user1/repo1",
			"PATCH",
			[]permission{
				{
					auth_model.AccessTokenScopeCategoryRepository,
					auth_model.Write,
				},
			},
		},
		{
			"/api/v1/repos/user1/repo1",
			"DELETE",
			[]permission{
				{
					auth_model.AccessTokenScopeCategoryRepository,
					auth_model.Write,
				},
			},
		},
		{
			"/api/v1/repos/user1/repo1/branches",
			"GET",
			[]permission{
				{
					auth_model.AccessTokenScopeCategoryRepository,
					auth_model.Read,
				},
			},
		},
		{
			"/api/v1/repos/user1/repo1/issues",
			"GET",
			[]permission{
				{
					auth_model.AccessTokenScopeCategoryIssue,
					auth_model.Read,
				},
			},
		},
		{
			"/api/v1/repos/user1/repo1/media/foo",
			"GET",
			[]permission{
				{
					auth_model.AccessTokenScopeCategoryRepository,
					auth_model.Read,
				},
			},
		},
		{
			"/api/v1/repos/user1/repo1/raw/foo",
			"GET",
			[]permission{
				{
					auth_model.AccessTokenScopeCategoryRepository,
					auth_model.Read,
				},
			},
		},
		{
			"/api/v1/repos/user1/repo1/teams",
			"GET",
			[]permission{
				{
					auth_model.AccessTokenScopeCategoryRepository,
					auth_model.Read,
				},
			},
		},
		{
			"/api/v1/repos/user1/repo1/teams/team1",
			"PUT",
			[]permission{
				{
					auth_model.AccessTokenScopeCategoryRepository,
					auth_model.Write,
				},
			},
		},
		// Private repo
		{
			"/api/v1/repos/user2/repo2",
			"GET",
			[]permission{
				{
					auth_model.AccessTokenScopeCategoryRepository,
					auth_model.Read,
				},
			},
		},
		// Private repo
		{
			"/api/v1/repos/user2/repo2",
			"GET",
			[]permission{
				{
					auth_model.AccessTokenScopeCategoryRepository,
					auth_model.Read,
				},
			},
		},
		{
			"/api/v1/user",
			"GET",
			[]permission{
				{
					auth_model.AccessTokenScopeCategoryUser,
					auth_model.Read,
				},
			},
		},
		{
			"/api/v1/user/emails",
			"GET",
			[]permission{
				{
					auth_model.AccessTokenScopeCategoryUser,
					auth_model.Read,
				},
			},
		},
		{
			"/api/v1/user/emails",
			"POST",
			[]permission{
				{
					auth_model.AccessTokenScopeCategoryUser,
					auth_model.Write,
				},
			},
		},
		{
			"/api/v1/user/emails",
			"DELETE",
			[]permission{
				{
					auth_model.AccessTokenScopeCategoryUser,
					auth_model.Write,
				},
			},
		},
		{
			"/api/v1/users/search",
			"GET",
			[]permission{
				{
					auth_model.AccessTokenScopeCategoryUser,
					auth_model.Read,
				},
			},
		},
		// Private user
		{
			"/api/v1/users/user31",
			"GET",
			[]permission{
				{
					auth_model.AccessTokenScopeCategoryUser,
					auth_model.Read,
				},
			},
		},
	}

	// User needs to be admin so that we can verify that tokens without admin
	// scopes correctly deny access.
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 1})
	assert.True(t, user.IsAdmin, "User needs to be admin")

	for _, testCase := range testCases {
		runTestCase(t, &testCase, user)
	}
}

// runTestCase Helper function to run a single test case.
func runTestCase(t *testing.T, testCase *requiredScopeTestCase, user *user_model.User) {
	t.Run(testCase.Name(), func(t *testing.T) {
		defer tests.PrintCurrentTest(t)()

		// Create a token with all scopes NOT required by the endpoint.
		var unauthorizedScopes []auth_model.AccessTokenScope
		for _, category := range auth_model.AllAccessTokenScopeCategories {
			// For permissions, Write > Read > NoAccess.  So we need to
			// find the minimum required, and only grant permission up to but
			// not including the minimum required.
			minRequiredLevel := auth_model.Write
			categoryIsRequired := false
			for _, requiredPermission := range testCase.requiredPermissions {
				if requiredPermission.category != category {
					continue
				}
				categoryIsRequired = true
				if requiredPermission.level < minRequiredLevel {
					minRequiredLevel = requiredPermission.level
				}
			}
			unauthorizedLevel := auth_model.Write
			if categoryIsRequired {
				switch minRequiredLevel {
				case auth_model.Read:
					unauthorizedLevel = auth_model.NoAccess
				case auth_model.Write:
					unauthorizedLevel = auth_model.Read
				default:
					assert.FailNow(t, "Invalid test case", "Unknown access token scope level: %v", minRequiredLevel)
				}
			}

			if unauthorizedLevel == auth_model.NoAccess {
				continue
			}
			categoryUnauthorizedScopes := auth_model.GetRequiredScopes(
				unauthorizedLevel,
				category)
			unauthorizedScopes = append(unauthorizedScopes, categoryUnauthorizedScopes...)
		}

		accessToken := createAPIAccessTokenWithoutCleanUp(t, "test-token", user, unauthorizedScopes)
		defer deleteAPIAccessToken(t, accessToken, user)

		// Request the endpoint.  Verify that permission is denied.
		req := NewRequest(t, testCase.method, testCase.url).
			AddTokenAuth(accessToken.Token)
		MakeRequest(t, req, http.StatusForbidden)
	})
}

// createAPIAccessTokenWithoutCleanUp creates an access token directly, there is no token API.
// The caller is responsible for deleting the token.
func createAPIAccessTokenWithoutCleanUp(t *testing.T, tokenName string, user *user_model.User, scopes []auth_model.AccessTokenScope) *auth_model.AccessToken {
	scopeNames := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scopeNames = append(scopeNames, string(scope))
	}
	scope, err := auth_model.AccessTokenScope(strings.Join(scopeNames, ",")).Normalize()
	require.NoError(t, err)
	token := &auth_model.AccessToken{UID: user.ID, Name: tokenName, Scope: scope}
	require.NoError(t, auth_model.NewAccessToken(t.Context(), token))
	return token
}

// deleteAPIAccessToken deletes an access token and asserts that deletion succeeded.
func deleteAPIAccessToken(t *testing.T, accessToken *auth_model.AccessToken, user *user_model.User) {
	require.NoError(t, auth_model.DeleteAccessTokenByID(t.Context(), accessToken.ID, user.ID))
	unittest.AssertNotExistsBean(t, &auth_model.AccessToken{ID: accessToken.ID})
}
