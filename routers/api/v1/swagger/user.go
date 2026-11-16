// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package swagger

import (
	api "gitea.dev/modules/structs"
)

// User
// swagger:response User
type swaggerResponseUser struct {
	// in:body
	Body api.User `json:"body"`
}

// UserList
// swagger:response UserList
type swaggerResponseUserList struct {
	// in:body
	Body []api.User `json:"body"`
}

// EmailList
// swagger:response EmailList
type swaggerResponseEmailList struct {
	// in:body
	Body []api.Email `json:"body"`
}

// UserSettings
// swagger:response UserSettings
type swaggerResponseUserSettings struct {
	// in:body
	Body api.UserSettings `json:"body"`
}
