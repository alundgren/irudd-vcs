// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package db

import (
	"xorm.io/xorm/schemas"
)

// DumpDatabase dumps all data from database as SQLite SQL to file system.
func DumpDatabase(filePath string) error {
	var tbs []*schemas.Table
	for _, t := range registeredModels {
		t, err := xormEngine.TableInfo(t)
		if err != nil {
			return err
		}
		tbs = append(tbs, t)
	}

	type Version struct {
		ID      int64 `xorm:"pk autoincr"`
		Version int64
	}
	t, err := xormEngine.TableInfo(&Version{})
	if err != nil {
		return err
	}
	tbs = append(tbs, t)

	return xormEngine.DumpTablesToFile(tbs, filePath)
}
