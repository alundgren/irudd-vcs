// Copyright 2014 The Gogs Authors. All rights reserved.
// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package auth

import (
	"errors"
	"net/http"

	user_model "gitea.dev/models/user"
	"gitea.dev/modules/auth/httpauth"
	"gitea.dev/modules/util"
)

// Ensure the struct implements the interface.
var (
	_ Method = &Basic{}
)

// BasicMethodName is the constant name of the basic authentication method
const BasicMethodName = "basic"

// Basic implements the Auth interface and authenticates requests (API requests
// only) by looking for Basic authentication data or "x-oauth-basic" token in the "Authorization"
// header.
type Basic struct{}

// Name represents the name of auth method
func (b *Basic) Name() string {
	return BasicMethodName
}

// parseAuthBasic returns the token of a Basic Authorization header: the password, or the username when the
// password is empty or "x-oauth-basic", as git clients send it
func parseAuthBasic(req *http.Request) (token string, ok bool) {
	authHeader := req.Header.Get("Authorization")
	if authHeader == "" {
		return "", false
	}
	parsed, ok := httpauth.ParseAuthorizationHeader(authHeader)
	if !ok || parsed.BasicAuth == nil {
		return "", false
	}
	uname, passwd := parsed.BasicAuth.Username, parsed.BasicAuth.Password
	if passwd == "" || passwd == "x-oauth-basic" {
		return uname, uname != ""
	}
	return passwd, true
}

// Verify extracts and validates Basic data (username and password/token) from the
// "Authorization" header of the request and returns the corresponding user object for that
// name/token on successful validation.
// Returns nil if header is empty or validation fails.
func (b *Basic) Verify(req *http.Request, w http.ResponseWriter, store DataStore) (*user_model.User, error) {
	token, ok := parseAuthBasic(req)
	if !ok {
		return nil, nil //nolint:nilnil // the auth method is not applicable
	}
	u, err := userFromAccessToken(req.Context(), store, token)
	if errors.Is(err, util.ErrNotExist) {
		return nil, ErrUserAuthMessage("invalid token") // there are no passwords, only tokens
	}
	return u, err
}
