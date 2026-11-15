// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package unittest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitea.dev/models/db"
	"gitea.dev/modules/cache"
	"gitea.dev/modules/git"
	"gitea.dev/modules/setting"
	"gitea.dev/modules/storage"
	"gitea.dev/modules/tempdir"
	"gitea.dev/modules/testlogger"
	"gitea.dev/modules/util"

	"github.com/stretchr/testify/assert"
	"xorm.io/xorm"
	"xorm.io/xorm/names"
)

// TestOptions represents test options
type TestOptions struct {
	FixtureFiles []string
	SetUp        func() error // SetUp will be executed before all tests in this package
	TearDown     func() error // TearDown will be executed after all tests in this package
}

// MainTest a reusable TestMain(..) function for unit tests that need to use a
// test database. Creates the test database, and sets necessary settings.
func MainTest(m *testing.M, testOptsArg ...*TestOptions) {
	os.Exit(mainTest(m, testOptsArg...))
}

func mainTest(m *testing.M, testOptsArg ...*TestOptions) int {
	testOpts := util.OptionalArg(testOptsArg, &TestOptions{})

	tempWorkPath, tempCleanup, err := tempdir.OsTempDir("gitea-test").MkdirTempRandom("unit-test-dir-")
	if err != nil {
		return testlogger.MainErrorf("Failed to create temp dir for unit test: %v", err)
	}
	defer tempCleanup()

	defer setting.MockBuiltinPaths(tempWorkPath, "", "")()
	setting.SetupGiteaTestEnv()

	giteaRoot := setting.GetGiteaTestSourceRoot()
	fixturesOpts := FixturesOptions{Dir: filepath.Join(giteaRoot, "models", "fixtures"), Files: testOpts.FixtureFiles}
	if err := CreateTestEngine(filepath.Join(tempWorkPath, "sqlite-test.db"), fixturesOpts); err != nil {
		return testlogger.MainErrorf("Error creating test database engine: %v", err)
	}

	setting.AppURL = "https://try.gitea.io/"
	setting.Domain = "try.gitea.io"
	setting.RunUser = "runuser"
	setting.Database.Type = "sqlite3"
	setting.Repository.DefaultBranch = "master" // many test code still assume that default branch is called "master"

	if err = cache.Init(); err != nil {
		return testlogger.MainErrorf("cache.Init: %v", err)
	}
	if err = storage.Init(); err != nil {
		return testlogger.MainErrorf("storage.Init: %v", err)
	}
	if err = SyncDirs(filepath.Join(giteaRoot, "tests", "gitea-repositories-meta"), setting.RepoRootPath); err != nil {
		return testlogger.MainErrorf("util.SyncDirs: %v", err)
	}

	if err = git.InitFull(); err != nil {
		return testlogger.MainErrorf("git.Init: %v", err)
	}

	if testOpts.SetUp != nil {
		if err := testOpts.SetUp(); err != nil {
			return testlogger.MainErrorf("set up failed: %v", err)
		}
	}

	exitStatus := m.Run()

	if testOpts.TearDown != nil {
		if err := testOpts.TearDown(); err != nil {
			return testlogger.MainErrorf("tear down failed: %v", err)
		}
	}
	return exitStatus
}

func ResetTestDatabase() (cleanup func(), err error) {
	defer func() {
		if cleanup == nil {
			cleanup = func() {}
		}
	}()

	connOpts := db.GlobalConnOptions()
	if !connOpts.Type.IsSQLite3() {
		return nil, fmt.Errorf("unsupported testing database type: %q", connOpts.Type)
	}
	if !strings.HasSuffix(connOpts.SQLitePath, "-test.db") {
		return nil, errors.New(`testing database file for sqlite3 must end in "-test.db"`)
	}
	_ = os.Remove(connOpts.SQLitePath)
	err = os.MkdirAll(filepath.Dir(connOpts.SQLitePath), os.ModePerm)
	if err != nil {
		return nil, err
	}
	cleanup = func() {
		_ = os.Remove(connOpts.SQLitePath)
		_ = os.Remove(filepath.Dir(connOpts.SQLitePath))
	}
	return cleanup, nil
}

// FixturesOptions fixtures needs to be loaded options
type FixturesOptions struct {
	Dir   string
	Files []string
}

// CreateTestEngine creates a test database and loads the fixture data from fixturesDir
func CreateTestEngine(testSQLiteFile string, opts FixturesOptions) error {
	driver, connStr, err := db.ConnStr(db.ConnOptions{Type: setting.DatabaseTypeSQLite3, SQLitePath: testSQLiteFile, SQLiteBusyTimeout: setting.DefaultSQLiteBusyTimeout})
	if err != nil {
		return err
	}
	x, err := xorm.NewEngine(driver, connStr)
	if err != nil {
		return err
	}
	x.SetMapper(names.GonicMapper{})
	db.SetDefaultEngine(context.Background(), x)

	if err = db.SyncAllTables(); err != nil {
		return err
	}
	switch os.Getenv("GITEA_TEST_LOG_SQL") {
	case "true", "1":
		x.ShowSQL(true)
	}

	return InitFixtures(opts)
}

// PrepareTestDatabase load test fixtures into test database
func PrepareTestDatabase() error {
	return LoadFixtures()
}

// PrepareTestEnv prepares the environment for unit tests. Can only be called
// by tests that use the above MainTest(..) function.
func PrepareTestEnv(t testing.TB) {
	assert.NoError(t, PrepareTestDatabase())
	metaPath := filepath.Join(setting.GetGiteaTestSourceRoot(), "tests", "gitea-repositories-meta")
	assert.NoError(t, SyncDirs(metaPath, setting.RepoRootPath))
}
