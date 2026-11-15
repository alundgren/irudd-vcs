// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"

	"gitea.dev/models/db"
	"gitea.dev/modules/timeutil"

	"xorm.io/builder"
)

type AuthToken struct { //nolint:revive // export stutter
	ID          string `xorm:"pk"`
	TokenHash   string
	UserID      int64              `xorm:"INDEX"`
	ExpiresUnix timeutil.TimeStamp `xorm:"INDEX"`
}

func init() {
	db.RegisterModel(new(AuthToken))
}

func DeleteAuthTokensByUserID(ctx context.Context, uid int64) error {
	_, err := db.GetEngine(ctx).Where(builder.Eq{"user_id": uid}).Delete(&AuthToken{})
	return err
}
