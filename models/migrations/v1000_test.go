// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package migrations

import (
	"testing"

	"gitea.dev/models/migrations/migrationtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"xorm.io/xorm/names"
)

func TestMain(m *testing.M) {
	migrationtest.MainTest(m)
}

func TestDropSingleOwnerLeftovers(t *testing.T) {
	// the user table and two of the dropped tables as they were before the migration
	type User struct {
		ID                 int64  `xorm:"pk autoincr"`
		LowerName          string `xorm:"UNIQUE NOT NULL"`
		Name               string `xorm:"UNIQUE NOT NULL"`
		Passwd             string `xorm:"NOT NULL"`
		PasswdHashAlgo     string `xorm:"NOT NULL DEFAULT 'argon2'"`
		Salt               string `xorm:"VARCHAR(32)"`
		MustChangePassword bool   `xorm:"NOT NULL DEFAULT false"`
		LoginType          int
		LoginSource        int64 `xorm:"NOT NULL DEFAULT 0"`
		LoginName          string
		IsRestricted       bool `xorm:"NOT NULL DEFAULT false"`
		IsAdmin            bool
		NumTeams           int
	}
	type Team struct {
		ID   int64 `xorm:"pk autoincr"`
		Name string
	}
	type LoginSource struct {
		ID   int64 `xorm:"pk autoincr"`
		Name string
	}

	x, deferable := migrationtest.PrepareTestEnv(t, 0, new(User), new(Team), new(LoginSource))
	defer deferable()
	require.NotNil(t, x)
	x.SetMapper(names.GonicMapper{})

	_, err := x.Insert(&User{LowerName: "owner", Name: "Owner", Passwd: "hash", LoginName: "owner", IsRestricted: true, IsAdmin: true})
	require.NoError(t, err)

	require.NoError(t, dropSingleOwnerLeftovers(t.Context(), x))

	tables := migrationtest.LoadTableSchemasMap(t, x)
	assert.NotContains(t, tables, "team")
	assert.NotContains(t, tables, "login_source")
	require.Contains(t, tables, "user")
	assert.ElementsMatch(t, []string{"id", "lower_name", "name", "is_admin"}, tables["user"].ColumnsSeq())

	rows, err := x.NewSession().QueryString("SELECT `name`, `is_admin` FROM `user`")
	require.NoError(t, err)
	assert.Equal(t, []map[string]string{{"name": "Owner", "is_admin": "1"}}, rows)
}
