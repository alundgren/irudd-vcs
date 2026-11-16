// Copyright 2016 The Gogs Authors. All rights reserved.
// Copyright 2018 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package structs

// Team represents a team in an organization. There are no teams, it keeps the type of
// requested_reviewers_teams, which is always empty
type Team struct {
	// The unique identifier of the team
	ID int64 `json:"id"`
	// The name of the team
	Name string `json:"name"`
	// The description of the team
	Description string `json:"description"`
}
