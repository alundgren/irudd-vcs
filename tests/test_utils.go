// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package tests

import (
	"path/filepath"
	"testing"

	"gitea.dev/models/unittest"
	"gitea.dev/modules/git"
	"gitea.dev/modules/graceful"
	"gitea.dev/modules/setting"
	"gitea.dev/modules/storage"
	"gitea.dev/modules/testlogger"
	"gitea.dev/modules/util"
	"gitea.dev/routers"

	"github.com/stretchr/testify/assert"
)

func InitIntegrationTest() error {
	testlogger.Init()

	err := setting.PrepareIntegrationTestConfig()
	if err != nil {
		return err
	}

	setting.SetupGiteaTestEnv()
	setting.Repository.DefaultBranch = "master" // many test code still assume that default branch is called "master"

	if err := git.InitFull(); err != nil {
		return err
	}

	setting.Database.SlowQueryThreshold = 0
	setting.LoadDBSetting()
	cleanupDb, err := unittest.ResetTestDatabase()
	if err != nil {
		return err
	}
	_ = cleanupDb // no clean up yet (not really needed at the moment)

	if err := storage.Init(); err != nil {
		return err
	}

	routers.InitWebInstalled(graceful.GetManager().HammerContext())
	return nil
}

func PrepareGitRepoDirectory(t testing.TB) {
	if !assert.NotEmpty(t, setting.RepoRootPath) {
		return
	}
	assert.NoError(t, unittest.SyncDirs(filepath.Join(setting.GetGiteaTestSourceRoot(), "tests/gitea-repositories-meta"), setting.RepoRootPath))
}

func PrepareLFSStorage(t testing.TB) {
	// load LFS object fixtures
	// (LFS storage can be on any of several backends, including remote servers, so init it with the storage API)
	lfsFixtures, err := storage.NewStorage(setting.LocalStorageType, &setting.Storage{
		Path: filepath.Join(filepath.Dir(setting.AppPath), "tests/gitea-lfs-meta"),
	})
	assert.NoError(t, err)
	assert.NoError(t, storage.Clean(storage.LFS))
	assert.NoError(t, lfsFixtures.IterateObjects("", func(path string, _ storage.Object) error {
		_, err := storage.Copy(storage.LFS, path, lfsFixtures, path)
		return err
	}))
}

func PrepareTestEnv(t testing.TB, skip ...int) func() {
	t.Helper()
	deferFn := PrintCurrentTest(t, util.OptionalArg(skip)+1)

	// load database fixtures
	assert.NoError(t, unittest.LoadFixtures())

	// do not add more Prepare* functions here, only call necessary ones in the related test functions
	PrepareGitRepoDirectory(t)
	PrepareLFSStorage(t)
	return deferFn
}

func PrintCurrentTest(t testing.TB, skip ...int) func() {
	t.Helper()
	return testlogger.PrintCurrentTest(t, util.OptionalArg(skip)+1)
}
