// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package db

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gitea.dev/modules/setting"
)

type ConnOptions struct {
	Type setting.DatabaseType

	SQLitePath        string
	SQLiteBusyTimeout int
	SQLiteJournalMode string
}

type SQLiteConnStrOptions struct {
	FilePath string
	// how long a concurrent query can wait for others (milliseconds),
	// if timeout is reached, the error is something like "database is locked (SQLITE_BUSY)"
	BusyTimeout int
	JournalMode string
}

func GlobalConnOptions() ConnOptions {
	return ConnOptions{
		Type: setting.Database.Type,

		SQLitePath:        setting.Database.Path,
		SQLiteBusyTimeout: setting.Database.SQLiteBusyTimeout,
		SQLiteJournalMode: setting.Database.SQLiteJournalMode,
	}
}

const sqlDriverSQLite3 = "sqlite3" // although database type also has "sqlite3", they are different, for different purposes

var makeSQLiteConnStr func(opts SQLiteConnStrOptions) (string, string, error)

func registerSQLiteConnStrMaker(fn func(opts SQLiteConnStrOptions) (string, string, error)) {
	if makeSQLiteConnStr != nil {
		panic("another sqlite3 driver has been registered")
	}
	makeSQLiteConnStr = fn
}

func ConnStr(opts ConnOptions) (string, string, error) {
	if !opts.Type.IsSQLite3() {
		return "", "", fmt.Errorf("unsupported database type: %q, only %q is supported", opts.Type, setting.DatabaseTypeSQLite3)
	}
	if makeSQLiteConnStr == nil {
		return "", "", errors.New(`this Gitea binary was not built with SQLite3 support, get an official release or rebuild with correct "-tags"`)
	}
	if opts.SQLitePath == "" {
		return "", "", errors.New("sqlite3 database path cannot be empty")
	}
	if err := os.MkdirAll(filepath.Dir(opts.SQLitePath), os.ModePerm); err != nil {
		return "", "", fmt.Errorf("failed to create directories: %w", err)
	}
	return makeSQLiteConnStr(SQLiteConnStrOptions{
		FilePath:    opts.SQLitePath,
		JournalMode: opts.SQLiteJournalMode,
		BusyTimeout: opts.SQLiteBusyTimeout,
	})
}
