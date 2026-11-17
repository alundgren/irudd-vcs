// Copyright 2014 The Gogs Authors. All rights reserved.
// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package auth

import (
	"net/http"

	user_model "gitea.dev/models/user"
	"gitea.dev/modules/auth/httpauth"
	"gitea.dev/modules/log"
	"gitea.dev/modules/util"
)

var _ Method = &OAuth2{}

// OAuth2 implements the Auth interface and authenticates requests
// (API requests only) by looking for an OAuth token in the "Authorization" header.
type OAuth2 struct{}

// Name represents the name of auth method
func (o *OAuth2) Name() string {
	return "oauth2"
}

// parseToken returns the token from request, and a boolean value
// representing whether the token exists or not
func parseToken(req *http.Request) (string, bool) {
	// a token in the query string would end up in logs and proxies, so only the header is accepted
	if util.HasCredentialQuery(req.URL.RawQuery) {
		log.Warn("API token sent in the query string is ignored, send it in the Authorization header")
	}
	if auHead := req.Header.Get("Authorization"); auHead != "" {
		parsed, ok := httpauth.ParseAuthorizationHeader(auHead)
		if ok && parsed.BearerToken != nil {
			return parsed.BearerToken.Token, true
		}
	}
	return "", false
}

// Verify extracts the user ID from the OAuth token in the "Authorization" header and returns the corresponding user object for that ID.
// If verification is successful returns an existing user object.
// Returns nil if verification fails.
func (o *OAuth2) Verify(req *http.Request, w http.ResponseWriter, store DataStore) (*user_model.User, error) {
	token, ok := parseToken(req)
	if !ok {
		return nil, nil //nolint:nilnil // the auth method is not applicable
	}

	return userFromAccessToken(req.Context(), store, token)
}
