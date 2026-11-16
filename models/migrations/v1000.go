// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package migrations

import (
	"context"

	"gitea.dev/models/db"
	"gitea.dev/models/migrations/base"
)

// dropSingleOwnerLeftovers drops the password, login source, restricted and organization data
// that the single-owner server no longer has
func dropSingleOwnerLeftovers(_ context.Context, x db.EngineMigration) error {
	sess := x.NewSession()
	defer sess.Close()
	if err := sess.Begin(); err != nil {
		return err
	}
	if err := base.DropTableColumns(sess, "user",
		"passwd", "passwd_hash_algo", "salt", "must_change_password",
		"login_type", "login_source", "login_name", "is_restricted",
		"allow_create_organization", "num_teams", "num_members", "repo_admin_change_team_access",
	); err != nil {
		return err
	}
	for _, table := range []string{"login_source", "team", "team_user", "team_repo", "team_unit", "org_user", "access", "collaboration"} {
		if err := sess.DropTable(table); err != nil {
			return err
		}
	}
	return sess.Commit()
}
