// Copyright 2014 The Gogs Authors. All rights reserved.
// Copyright 2020 The Gitea Authors.
// SPDX-License-Identifier: MIT

package context

import (
	"gitea.dev/models/organization"
)

// Organization contains organization context
type Organization struct {
	IsOwner          bool
	IsMember         bool
	IsTeamMember     bool // Is member of team.
	IsTeamAdmin      bool // In owner team or team that has admin permission level.
	Organization     *organization.Organization
	OrgLink          string
	CanCreateOrgRepo bool

	Team  *organization.Team
	Teams []*organization.Team
}
