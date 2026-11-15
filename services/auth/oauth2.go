// Copyright 2014 The Gogs Authors. All rights reserved.
// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"errors"
	"net/http"

	auth_model "gitea.dev/models/auth"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/auth/httpauth"
	"gitea.dev/modules/log"
	"gitea.dev/modules/setting"
	"gitea.dev/modules/timeutil"
	"gitea.dev/modules/util"
)

var _ Method = &OAuth2{}

// OAuth2 implements the Auth interface and authenticates requests
// (API requests only) by looking for an OAuth token in query parameters or the
// "Authorization" header.
type OAuth2 struct{}

// Name represents the name of auth method
func (o *OAuth2) Name() string {
	return "oauth2"
}

// parseToken returns the token from request, and a boolean value
// representing whether the token exists or not
func parseToken(req *http.Request) (string, bool) {
	_ = req.ParseForm()
	if !setting.DisableQueryAuthToken {
		// Check token.
		if token := req.Form.Get("token"); token != "" {
			return token, true
		}
		// Check access token.
		if token := req.Form.Get("access_token"); token != "" {
			return token, true
		}
	} else if req.Form.Get("token") != "" || req.Form.Get("access_token") != "" {
		log.Warn("API token sent in query string but DISABLE_QUERY_AUTH_TOKEN=true")
	}

	// check header token
	if auHead := req.Header.Get("Authorization"); auHead != "" {
		parsed, ok := httpauth.ParseAuthorizationHeader(auHead)
		if ok && parsed.BearerToken != nil {
			return parsed.BearerToken.Token, true
		}
	}
	return "", false
}

// userFromToken returns the user corresponding to the OAuth token.
// It will set 'IsApiToken' to true if the token is an API token and
// set 'ApiTokenScope' to the scope of the access token (TODO: this behavior should be fixed, don't set ctx.Data)
func (o *OAuth2) userFromToken(ctx context.Context, tokenSHA string, store DataStore) (*user_model.User, error) {
	t, err := auth_model.GetAccessTokenBySHA(ctx, tokenSHA)
	if err != nil {
		return nil, err
	}

	t.UpdatedUnix = timeutil.TimeStampNow()
	if err = auth_model.UpdateAccessToken(ctx, t); err != nil {
		log.Error("UpdateAccessToken: %v", err)
	}
	store.GetData()["IsApiToken"] = true
	store.GetData()["ApiTokenScope"] = t.Scope
	return user_model.GetUserByID(ctx, t.UID)
}

// Verify extracts the user ID from the OAuth token in the query parameters
// or the "Authorization" header and returns the corresponding user object for that ID.
// If verification is successful returns an existing user object.
// Returns nil if verification fails.
func (o *OAuth2) Verify(req *http.Request, w http.ResponseWriter, store DataStore) (*user_model.User, error) {
	token, ok := parseToken(req)
	if !ok {
		return nil, nil //nolint:nilnil // the auth method is not applicable
	}

	user, err := o.userFromToken(req.Context(), token, store)
	if err != nil && !errors.Is(err, util.ErrNotExist) {
		log.Error("userFromToken: %v", err) // the callers might ignore the error, so log it here
	}
	return user, err
}
