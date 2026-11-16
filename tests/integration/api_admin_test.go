// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"testing"

	auth_model "gitea.dev/models/auth"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/glob"
	"gitea.dev/modules/json"
	"gitea.dev/modules/setting"
	api "gitea.dev/modules/structs"
	"gitea.dev/modules/test"
	"gitea.dev/tests"

	"github.com/stretchr/testify/assert"
)

func TestAPIListUsers(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	adminUsername := "user1"
	token := getUserToken(t, adminUsername, auth_model.AccessTokenScopeReadAdmin)

	req := NewRequest(t, "GET", "/api/v1/admin/users").
		AddTokenAuth(token)
	resp := MakeRequest(t, req, http.StatusOK)
	users := DecodeJSON(t, resp, []api.User{})

	found := false
	for _, user := range users {
		if user.UserName == adminUsername {
			found = true
		}
	}
	assert.True(t, found)
	numberOfUsers := unittest.GetCount(t, &user_model.User{}, "type = 0")
	assert.Len(t, users, numberOfUsers)
}

func TestAPIListUsersNotLoggedIn(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	req := NewRequest(t, "GET", "/api/v1/admin/users")
	MakeRequest(t, req, http.StatusUnauthorized)
}

func TestAPIListUsersNonAdmin(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	nonAdminUsername := "user2"
	token := getUserToken(t, nonAdminUsername, auth_model.AccessTokenScopeAll)
	req := NewRequest(t, "GET", "/api/v1/admin/users").
		AddTokenAuth(token)
	MakeRequest(t, req, http.StatusForbidden)
}

func TestAPICreateUserInvalidEmail(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	adminUsername := "user1"
	token := getUserToken(t, adminUsername, auth_model.AccessTokenScopeWriteAdmin)
	req := NewRequestWithValues(t, "POST", "/api/v1/admin/users", map[string]string{
		"email":                "invalid_email@domain.com\r\n",
		"full_name":            "invalid user",
		"login_name":           "invalidUser",
		"must_change_password": "true",
		"password":             "password",
		"send_notify":          "true",
		"source_id":            "0",
		"username":             "invalidUser",
	}).AddTokenAuth(token)
	MakeRequest(t, req, http.StatusUnprocessableEntity)
}

func TestAPICreateAndDeleteUser(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	adminUsername := "user1"
	token := getUserToken(t, adminUsername, auth_model.AccessTokenScopeWriteAdmin)

	req := NewRequestWithValues(
		t,
		"POST",
		"/api/v1/admin/users",
		map[string]string{
			"email":                "deleteme@domain.com",
			"full_name":            "delete me",
			"login_name":           "deleteme",
			"must_change_password": "true",
			"password":             "password",
			"send_notify":          "true",
			"source_id":            "0",
			"username":             "deleteme",
		},
	).AddTokenAuth(token)
	MakeRequest(t, req, http.StatusCreated)

	req = NewRequest(t, "DELETE", "/api/v1/admin/users/deleteme").
		AddTokenAuth(token)
	MakeRequest(t, req, http.StatusNoContent)
}

func TestAPIEditUser(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	adminUsername := "user1"
	token := getUserToken(t, adminUsername, auth_model.AccessTokenScopeWriteAdmin)
	urlStr := "/api/v1/admin/users/" + "user2"

	fullNameToChange := "Full Name User 2"
	req := NewRequestWithValues(t, "PATCH", urlStr, map[string]string{
		// required
		"login_name": "user2",
		"source_id":  "0",
		// to change
		"full_name": fullNameToChange,
	}).AddTokenAuth(token)
	MakeRequest(t, req, http.StatusOK)
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{LoginName: "user2"})
	assert.Equal(t, fullNameToChange, user2.FullName)

	req = NewRequestWithJSON(t, "PATCH", urlStr, api.EditUserOption{SourceID: 0, Email: new("")}).AddTokenAuth(token)
	resp := MakeRequest(t, req, http.StatusBadRequest)

	errMap := make(map[string]any)
	json.Unmarshal(resp.Body.Bytes(), &errMap)
	assert.Equal(t, "e-mail invalid [email: ]", errMap["message"].(string))

	user2 = unittest.AssertExistsAndLoadBean(t, &user_model.User{LoginName: "user2"})
	assert.False(t, user2.IsRestricted)
	req = NewRequestWithJSON(t, "PATCH", urlStr, api.EditUserOption{Restricted: new(true)}).AddTokenAuth(token)
	MakeRequest(t, req, http.StatusOK)
	user2 = unittest.AssertExistsAndLoadBean(t, &user_model.User{LoginName: "user2"})
	assert.True(t, user2.IsRestricted)
}

func TestAPICreateRepoForUser(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	adminUsername := "user1"
	token := getUserToken(t, adminUsername, auth_model.AccessTokenScopeWriteAdmin)

	req := NewRequestWithJSON(
		t,
		"POST",
		fmt.Sprintf("/api/v1/admin/users/%s/repos", adminUsername),
		&api.CreateRepoOption{
			Name: "admincreatedrepo",
		},
	).AddTokenAuth(token)
	MakeRequest(t, req, http.StatusCreated)
}

func TestAPIRenameUser(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	adminUsername := "user1"
	token := getUserToken(t, adminUsername, auth_model.AccessTokenScopeWriteAdmin)
	urlStr := fmt.Sprintf("/api/v1/admin/users/%s/rename", "user2")
	req := NewRequestWithValues(t, "POST", urlStr, map[string]string{
		// required
		"new_name": "User2",
	}).AddTokenAuth(token)
	MakeRequest(t, req, http.StatusNoContent)

	urlStr = fmt.Sprintf("/api/v1/admin/users/%s/rename", "User2")
	req = NewRequestWithValues(t, "POST", urlStr, map[string]string{
		// required
		"new_name": "User2-2-2",
	}).AddTokenAuth(token)
	MakeRequest(t, req, http.StatusNoContent)

	req = NewRequestWithValues(t, "POST", urlStr, map[string]string{
		// required
		"new_name": "user1",
	}).AddTokenAuth(token)
	// the old user name still be used by with a redirect
	MakeRequest(t, req, http.StatusTemporaryRedirect)

	urlStr = fmt.Sprintf("/api/v1/admin/users/%s/rename", "User2-2-2")
	req = NewRequestWithValues(t, "POST", urlStr, map[string]string{
		// required
		"new_name": "user1",
	}).AddTokenAuth(token)
	MakeRequest(t, req, http.StatusUnprocessableEntity)

	req = NewRequestWithValues(t, "POST", urlStr, map[string]string{
		// required
		"new_name": "user2",
	}).AddTokenAuth(token)
	MakeRequest(t, req, http.StatusNoContent)
}

func TestAPICreateUser_NotAllowedEmailDomain(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	defer test.MockVariableValue(&setting.Service.EmailDomainAllowList, []glob.Glob{glob.MustCompile("example.org")})()

	adminUsername := "user1"
	token := getUserToken(t, adminUsername, auth_model.AccessTokenScopeWriteAdmin)

	req := NewRequestWithValues(t, "POST", "/api/v1/admin/users", map[string]string{
		"email":                "allowedUser1@example1.org",
		"login_name":           "allowedUser1",
		"username":             "allowedUser1",
		"password":             "allowedUser1_pass",
		"must_change_password": "true",
	}).AddTokenAuth(token)
	resp := MakeRequest(t, req, http.StatusCreated)
	assert.Equal(t, "the domain of user email allowedUser1@example1.org conflicts with EMAIL_DOMAIN_ALLOWLIST or EMAIL_DOMAIN_BLOCKLIST", resp.Header().Get("X-Gitea-Warning"))

	req = NewRequest(t, "DELETE", "/api/v1/admin/users/allowedUser1").AddTokenAuth(token)
	MakeRequest(t, req, http.StatusNoContent)
}

func TestAPIEditUser_NotAllowedEmailDomain(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	defer test.MockVariableValue(&setting.Service.EmailDomainAllowList, []glob.Glob{glob.MustCompile("example.org")})()

	adminUsername := "user1"
	token := getUserToken(t, adminUsername, auth_model.AccessTokenScopeWriteAdmin)
	urlStr := "/api/v1/admin/users/user2"

	req := NewRequestWithJSON(t, "PATCH", urlStr, api.EditUserOption{Email: new("user2@example1.com")}).AddTokenAuth(token)
	resp := MakeRequest(t, req, http.StatusBadRequest)
	errMap := make(map[string]string)
	assert.NoError(t, json.Unmarshal(resp.Body.Bytes(), &errMap))
	assert.Equal(t, "the domain of user email user2@example1.com conflicts with EMAIL_DOMAIN_ALLOWLIST or EMAIL_DOMAIN_BLOCKLIST", errMap["message"])

	req = NewRequestWithJSON(t, "PATCH", urlStr, api.EditUserOption{Email: new("user2@example.org")}).AddTokenAuth(token)
	MakeRequest(t, req, http.StatusOK)
}
