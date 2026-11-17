// Copyright 2015 The Gogs Authors. All rights reserved.
// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package migrations

import (
	"context"
	"errors"
	"fmt"

	"gitea.dev/models/db"
	"gitea.dev/modules/log"

	"xorm.io/xorm/names"
)

// minDBVersion is the first irudd-vcs schema version. Databases created by Gitea are not supported:
// the legacy Gitea migrations were removed, and Gitea databases carry tables this fork no longer has.
const minDBVersion = 1000

type migration struct {
	idNumber    int64 // DB version is "the last migration's idNumber" + 1
	description string
	migrate     func(context.Context, db.EngineMigration) error
}

// Version describes the version table. Should have only one row with id==1
type Version struct {
	ID      int64 `xorm:"pk autoincr"`
	Version int64 // DB version is "the last migration's idNumber" + 1
}

// prepareMigrationTasks returns the irudd-vcs schema migrations. Add new migrations to the bottom of the list,
// numbered from minDBVersion.
func prepareMigrationTasks() []*migration {
	return []*migration{
		{idNumber: 1000, description: "Drop password, login source, restricted and organization data", migrate: dropSingleOwnerLeftovers},
	}
}

// GetCurrentDBVersion returns the current db version
func GetCurrentDBVersion(x db.EngineMigration) (int64, error) {
	if err := x.Sync(new(Version)); err != nil {
		return -1, fmt.Errorf("sync: %w", err)
	}

	currentVersion := &Version{ID: 1}
	has, err := x.Get(currentVersion)
	if err != nil {
		return -1, fmt.Errorf("get: %w", err)
	}
	if !has {
		return -1, nil
	}
	return currentVersion.Version, nil
}

func calcDBVersion(migrations []*migration) int64 {
	dbVer := int64(minDBVersion + len(migrations))
	if len(migrations) > 0 && migrations[0].idNumber != minDBVersion {
		panic("migrations should start at minDBVersion")
	}
	if len(migrations) > 0 && dbVer != migrations[len(migrations)-1].idNumber+1 {
		panic("migrations are not in order")
	}
	return dbVer
}

// ExpectedDBVersion returns the expected db version
func ExpectedDBVersion() int64 {
	return calcDBVersion(prepareMigrationTasks())
}

// EnsureUpToDate will check if the db is at the correct version
func EnsureUpToDate(ctx context.Context, x db.EngineMigration) error {
	currentDB, err := GetCurrentDBVersion(x)
	if err != nil {
		return err
	}

	if currentDB < 0 {
		return errors.New("database has not been initialized")
	}

	if minDBVersion > currentDB {
		return fmt.Errorf("DB version %d is from Gitea, which irudd-vcs cannot use: start with a new database", currentDB)
	}

	expectedDB := ExpectedDBVersion()

	if currentDB != expectedDB {
		return fmt.Errorf(`current database version %d is not equal to the expected version %d. Please run "gitea [--config /path/to/app.ini] migrate" to update the database version`, currentDB, expectedDB)
	}

	return nil
}

// Migrate database to current version
func Migrate(ctx context.Context, x db.EngineMigration) error {
	migrations := prepareMigrationTasks()
	maxDBVer := calcDBVersion(migrations)

	// Set a new clean the default mapper to GonicMapper as that is the default for Gitea.
	x.SetMapper(names.GonicMapper{})
	if err := x.Sync(new(Version)); err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	currentVersion := &Version{ID: 1}
	has, err := x.Get(currentVersion)
	if err != nil {
		return fmt.Errorf("get: %w", err)
	} else if !has {
		// If the version record does not exist, it is a fresh installation, and we can skip all migrations.
		// XORM model framework will create all tables when initializing.
		currentVersion.ID = 0
		currentVersion.Version = maxDBVer
		if _, err = x.Insert(currentVersion); err != nil {
			return fmt.Errorf("insert: %w", err)
		}
	}

	curDBVer := currentVersion.Version
	if curDBVer < minDBVersion {
		return fmt.Errorf("database version %d is from Gitea, which irudd-vcs cannot use: start with a new database", curDBVer)
	}

	// Downgrading the database version is not supported
	if maxDBVer < curDBVer {
		return fmt.Errorf("database version %d is for a newer irudd-vcs than this one (%d)", curDBVer, maxDBVer)
	}

	for _, m := range migrations[curDBVer-minDBVersion:] {
		log.Info("Migration[%d]: %s", m.idNumber, m.description)
		// Reset the mapper between each migration - migrations are not supposed to depend on each other
		x.SetMapper(names.GonicMapper{})
		if err = m.migrate(ctx, x); err != nil {
			return fmt.Errorf("migration[%d]: %s failed: %w", m.idNumber, m.description, err)
		}
		currentVersion.Version = m.idNumber + 1
		if _, err = x.ID(1).Update(currentVersion); err != nil {
			return err
		}
	}
	return nil
}
