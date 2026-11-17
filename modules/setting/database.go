// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"path/filepath"
	"time"

	"gitea.dev/modules/log"
)

const DefaultSQLiteBusyTimeout = 20 * 1000

// Database holds the database settings
var Database = struct {
	Type DatabaseType
	Name string // only referenced by legacy migrations for other databases, always empty
	Path string

	SQLiteBusyTimeout int
	SQLiteJournalMode string

	LogSQL             bool
	DBConnectRetries   int
	DBConnectBackoff   time.Duration
	MaxIdleConns       int
	MaxOpenConns       int
	ConnMaxLifetime    time.Duration
	IterateBufferSize  int
	AutoMigration      bool
	SlowQueryThreshold time.Duration
}{
	IterateBufferSize:  50,
	SlowQueryThreshold: 5 * time.Second,
}

// LoadDBSetting loads the database settings
func LoadDBSetting() {
	loadDBSetting(CfgProvider)
}

func loadDBSetting(rootCfg ConfigProvider) {
	sec := rootCfg.Section("database")
	Database.Type = DatabaseType(sec.Key("DB_TYPE").MustString(DatabaseTypeSQLite3))
	if !Database.Type.IsSQLite3() {
		log.Fatal("Unsupported [database] DB_TYPE %q: only %q is supported", Database.Type, DatabaseTypeSQLite3)
	}

	Database.Path = sec.Key("PATH").MustString(filepath.Join(AppDataPath, "gitea.db"))

	Database.SQLiteBusyTimeout = sec.Key("SQLITE_TIMEOUT").MustInt(DefaultSQLiteBusyTimeout)
	// mattn driver isn't really affected by this timeout, but other drivers are affected
	// the default value was 500 (0.5s), to avoid breaking existing users, make sure the timeout is long enough (at least, 5 seconds)
	if Database.SQLiteBusyTimeout < 5000 {
		Database.SQLiteBusyTimeout = DefaultSQLiteBusyTimeout
	}
	// WAL spares a rollback journal file per write and lets reads keep their page cache; on Windows that
	// halves first boot and cuts API latency by about a quarter
	Database.SQLiteJournalMode = sec.Key("SQLITE_JOURNAL_MODE").MustString("WAL")

	Database.MaxIdleConns = sec.Key("MAX_IDLE_CONNS").MustInt(2)
	Database.ConnMaxLifetime = sec.Key("CONN_MAX_LIFETIME").MustDuration(0)
	Database.MaxOpenConns = sec.Key("MAX_OPEN_CONNS").MustInt(0)

	Database.IterateBufferSize = sec.Key("ITERATE_BUFFER_SIZE").MustInt(50)
	Database.LogSQL = sec.Key("LOG_SQL").MustBool(false)
	Database.DBConnectRetries = sec.Key("DB_RETRIES").MustInt(10)
	Database.DBConnectBackoff = sec.Key("DB_RETRY_BACKOFF").MustDuration(3 * time.Second)
	Database.AutoMigration = sec.Key("AUTO_MIGRATION").MustBool(true)
	Database.SlowQueryThreshold = sec.Key("SLOW_QUERY_THRESHOLD").MustDuration(Database.SlowQueryThreshold)
}

// DatabaseType FIXME: it is also used directly with "schemas.DBType", so the names must be consistent
type DatabaseType string

const DatabaseTypeSQLite3 = "sqlite3"

func (t DatabaseType) IsSQLite3() bool {
	return t == DatabaseTypeSQLite3
}

// IsMySQL, IsMSSQL and IsPostgreSQL are kept for legacy migrations, they are always false at runtime

func (t DatabaseType) IsMySQL() bool {
	return t == "mysql"
}

func (t DatabaseType) IsMSSQL() bool {
	return t == "mssql"
}

func (t DatabaseType) IsPostgreSQL() bool {
	return t == "postgres"
}
