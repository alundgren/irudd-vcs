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

func TestAPIPublicOnlySelfUserRoutes(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	privateUser := unittest.AssertExistsAndLoadBean(t, &user_model.User{Name: "user31"})
	require.True(t, privateUser.Visibility.IsPrivate())

	privateSession := loginUser(t, privateUser.Name)
	privateReadUserToken := getTokenForLoggedInUser(t, privateSession,
		auth_model.AccessTokenScopePublicOnly,
		auth_model.AccessTokenScopeReadUser,
	)
	privateWriteUserToken := getTokenForLoggedInUser(t, privateSession,
		auth_model.AccessTokenScopePublicOnly,
		auth_model.AccessTokenScopeWriteUser,
	)

	t.Run("PrivateProfileForbidden", func(t *testing.T) {
		defer tests.PrintCurrentTest(t)()

		MakeRequest(t, NewRequest(t, "GET", "/api/v1/users/user31").AddTokenAuth(privateReadUserToken), http.StatusForbidden)
		MakeRequest(t, NewRequest(t, "GET", "/api/v1/user").AddTokenAuth(privateReadUserToken), http.StatusForbidden)
	})

	t.Run("PrivateSensitiveSelfRoutesForbidden", func(t *testing.T) {
		defer tests.PrintCurrentTest(t)()

		MakeRequest(t, NewRequest(t, "GET", "/api/v1/user/settings").AddTokenAuth(privateReadUserToken), http.StatusForbidden)
		hideEmail := true
		settingsReq := NewRequestWithJSON(t, "PATCH", "/api/v1/user/settings", &api.UserSettingsOptions{
			HideEmail: &hideEmail,
		}).AddTokenAuth(privateWriteUserToken)
		MakeRequest(t, settingsReq, http.StatusForbidden)

		MakeRequest(t, NewRequest(t, "GET", "/api/v1/user/emails").AddTokenAuth(privateReadUserToken), http.StatusForbidden)
		emailReq := NewRequestWithJSON(t, "POST", "/api/v1/user/emails", &api.CreateEmailOption{
			Emails: []string{"user31-public-only@example.com"},
		}).AddTokenAuth(privateWriteUserToken)
		MakeRequest(t, emailReq, http.StatusForbidden)

		MakeRequest(t, NewRequest(t, "GET", "/api/v1/user/hooks").AddTokenAuth(privateReadUserToken), http.StatusForbidden)
		hookReq := NewRequestWithJSON(t, "POST", "/api/v1/user/hooks", api.CreateHookOption{
			Type: "gitea",
			Config: api.CreateHookOptionConfig{
				"content_type": "json",
				"url":          "http://example.com/",
			},
			Name: "public-only-private-hook",
		}).AddTokenAuth(privateWriteUserToken)
		MakeRequest(t, hookReq, http.StatusForbidden)
	})

	t.Run("PublicRepoRoutesFilterAndRejectMutations", func(t *testing.T) {
		defer tests.PrintCurrentTest(t)()

		publicSession := loginUser(t, "user2")
		fullWriteRepoToken := getTokenForLoggedInUser(t, publicSession,
			auth_model.AccessTokenScopeWriteUser,
			auth_model.AccessTokenScopeWriteRepository,
		)
		publicOnlyReadRepoToken := getTokenForLoggedInUser(t, publicSession,
			auth_model.AccessTokenScopePublicOnly,
			auth_model.AccessTokenScopeReadUser,
			auth_model.AccessTokenScopeReadRepository,
		)
		publicOnlyWriteRepoToken := getTokenForLoggedInUser(t, publicSession,
			auth_model.AccessTokenScopePublicOnly,
			auth_model.AccessTokenScopeWriteUser,
			auth_model.AccessTokenScopeWriteRepository,
		)

		publicRepoName := "public-only-visible-self-repo"
		privateRepoName := "public-only-hidden-self-repo"

		resp := MakeRequest(t, NewRequestWithJSON(t, "POST", "/api/v1/user/repos", &api.CreateRepoOption{
			Name:    publicRepoName,
			Private: false,
		}).AddTokenAuth(fullWriteRepoToken), http.StatusCreated)
		publicRepo := DecodeJSON(t, resp, &api.Repository{})
		require.Equal(t, "user2/"+publicRepoName, publicRepo.FullName)

		resp = MakeRequest(t, NewRequestWithJSON(t, "POST", "/api/v1/user/repos", &api.CreateRepoOption{
			Name:    privateRepoName,
			Private: true,
		}).AddTokenAuth(fullWriteRepoToken), http.StatusCreated)
		privateRepo := DecodeJSON(t, resp, &api.Repository{})
		require.Equal(t, "user2/"+privateRepoName, privateRepo.FullName)

		MakeRequest(t, NewRequest(t, "GET", "/api/v1/repos/user2/"+privateRepoName).AddTokenAuth(publicOnlyReadRepoToken), http.StatusNotFound)

		resp = MakeRequest(t, NewRequest(t, "GET", "/api/v1/user/repos").AddTokenAuth(publicOnlyReadRepoToken), http.StatusOK)
		repos := DecodeJSON(t, resp, []api.Repository{})

		foundPublicRepo := false
		for _, repo := range repos {
			require.NotEqual(t, privateRepo.FullName, repo.FullName)
			if repo.FullName == publicRepo.FullName {
				foundPublicRepo = true
			}
		}
		require.True(t, foundPublicRepo)

		MakeRequest(t, NewRequestWithJSON(t, "POST", "/api/v1/user/repos", &api.CreateRepoOption{
			Name:    "public-only-rejected-self-repo",
			Private: false,
		}).AddTokenAuth(publicOnlyWriteRepoToken), http.StatusForbidden)
	})
}
