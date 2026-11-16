// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"testing"

	auth_model "gitea.dev/models/auth"
	"gitea.dev/tests"

	"github.com/stretchr/testify/assert"
)

func TestAPIAuth(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	token := getUserToken(t, "user2", auth_model.AccessTokenScopeAll)

	// a token works as the basic auth password or username, like git clients send it
	MakeRequest(t, NewRequestf(t, "GET", "/api/v1/user").AddBasicAuth("user2", token), http.StatusOK)
	MakeRequest(t, NewRequestf(t, "GET", "/api/v1/user").AddBasicAuth(token, ""), http.StatusOK)

	// passwords are not accepted
	req := NewRequestf(t, "GET", "/api/v1/user").AddBasicAuth("user2", "password")
	resp := MakeRequest(t, req, http.StatusUnauthorized)
	assert.Contains(t, resp.Body.String(), `{"message":"invalid token"`)

	req = NewRequestf(t, "GET", "/api/v1/users/user2/repos").AddTokenAuth("Bearer wrong_token")
	resp = MakeRequest(t, req, http.StatusUnauthorized)
	assert.Contains(t, resp.Body.String(), `{"message":"invalid token"`)

	// anonymous requests only get the version
	MakeRequest(t, NewRequest(t, "GET", "/api/v1/version"), http.StatusOK)
	for _, path := range []string{"/api/v1/user", "/api/v1/users/user2", "/api/v1/repos/user2/repo1", "/api/v1/repos/search", "/api/v1/settings/api"} {
		resp = MakeRequest(t, NewRequest(t, "GET", path), http.StatusUnauthorized)
		assert.Contains(t, resp.Body.String(), `"token is required"`, path)
	}
}
