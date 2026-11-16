// Copyright 2014 The Gogs Authors. All rights reserved.
// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package structs

import (
	"time"
)

// AccessToken represents an API access token.
// swagger:response AccessToken
type AccessToken struct {
	// The unique identifier of the access token
	ID int64 `json:"id"`
	// The name of the access token
	Name string `json:"name"`
	// The SHA1 hash of the access token
	Token string `json:"sha1"`
	// The last eight characters of the token
	TokenLastEight string `json:"token_last_eight"`
	// The scopes granted to this access token
	Scopes []string `json:"scopes"`
	// The timestamp when the token was created
	Created time.Time `json:"created_at"`
	// The timestamp when the token was last used
	Updated time.Time `json:"last_used_at"`
}
