// Copyright 2022 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package organization

import (
	"context"

	"gitea.dev/models/db"
	"gitea.dev/modules/timeutil"
)

// TeamInvite represents an invite to a team
type TeamInvite struct {
	ID          int64              `xorm:"pk autoincr"`
	Token       string             `xorm:"UNIQUE(token) INDEX NOT NULL DEFAULT ''"`
	InviterID   int64              `xorm:"NOT NULL DEFAULT 0"`
	OrgID       int64              `xorm:"INDEX NOT NULL DEFAULT 0"`
	TeamID      int64              `xorm:"UNIQUE(team_mail) INDEX NOT NULL DEFAULT 0"`
	Email       string             `xorm:"UNIQUE(team_mail) NOT NULL DEFAULT ''"`
	CreatedUnix timeutil.TimeStamp `xorm:"INDEX created"`
	UpdatedUnix timeutil.TimeStamp `xorm:"INDEX updated"`
}

func GetInvitesByTeamID(ctx context.Context, teamID int64) ([]*TeamInvite, error) {
	invites := make([]*TeamInvite, 0, 10)
	return invites, db.GetEngine(ctx).
		Where("team_id=?", teamID).
		Find(&invites)
}
