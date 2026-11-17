// Copyright 2014 The Gogs Authors. All rights reserved.
// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"errors"

	auth_model "gitea.dev/models/auth"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/log"
	"gitea.dev/modules/util"
)

// AccessTokenMethodName is the login method of requests that a token authenticates
const AccessTokenMethodName = "access_token"

type ErrUserAuthMessage string

func (e ErrUserAuthMessage) Error() string {
	return string(e)
}

func ErrAsUserAuthMessage(err error) (string, bool) {
	if msg, ok := errors.AsType[ErrUserAuthMessage](err); ok {
		return msg.Error(), true
	}
	return "", false
}

// userFromAccessToken returns the owner of an access token for every method that accepts one, with a not-exist
// error for an unknown token. It records the token scope in the store for the scope checks.
func userFromAccessToken(ctx context.Context, store DataStore, token string) (*user_model.User, error) {
	t, err := auth_model.GetAccessTokenBySHA(ctx, token)
	if err == nil {
		var u *user_model.User
		if u, err = user_model.GetUserByID(ctx, t.UID); err == nil {
			if err := auth_model.TouchAccessToken(ctx, t); err != nil {
				log.Error("TouchAccessToken: %v", err)
			}
			store.GetData()["LoginMethod"] = AccessTokenMethodName
			store.GetData()["IsApiToken"] = true
			store.GetData()["ApiTokenScope"] = t.Scope
			return u, nil
		}
	}
	if !errors.Is(err, util.ErrNotExist) {
		log.Error("access token verification: %v", err) // the callers might ignore the error, so log it here
	}
	return nil, err
}
