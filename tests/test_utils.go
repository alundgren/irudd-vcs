// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitea.dev/cmd"
	"gitea.dev/models/unittest"
	"gitea.dev/modules/git"
	"gitea.dev/modules/gitrepo"
	"gitea.dev/modules/graceful"
	"gitea.dev/modules/log"
	"gitea.dev/modules/setting"
	"gitea.dev/modules/storage"
	"gitea.dev/modules/testlogger"
	"gitea.dev/modules/util"
	"gitea.dev/routers"

	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli/v3"
)

// fixtureReposPath holds the fixture repositories with the current hooks, which PrepareGitRepoDirectory copies
var fixtureReposPath string

// RunAsGitea runs the gitea command when the hooks of a test repository start this binary as setting.AppPath, so
// they run the code under test instead of a separately built gitea. A test binary gets no arguments without "-".
func RunAsGitea() bool {
	if len(os.Args) < 2 || strings.HasPrefix(os.Args[1], "-") {
		return false
	}
	cli.OsExiter = func(code int) {
		log.GetManager().Close()
		os.Exit(code)
	}
	_ = cmd.RunMainApp(cmd.NewMainApp(cmd.AppVersion{Version: "test"}), os.Args...) // as main does
	log.GetManager().Close()
	return true
}

// initGitHome starts each test process with the git home that git.InitFull makes, so global config that a test
// sets, such as git lfs install, does not reach the next test. It restores a saved copy: writing the config
// anew takes a git process per setting.
func initGitHome() error {
	home := setting.Git.HomePath
	saved := home + "-gitconfig"
	if err := os.RemoveAll(home); err != nil {
		return err
	}
	if data, err := os.ReadFile(saved); err == nil {
		if err = os.MkdirAll(home, os.ModePerm); err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(home, ".gitconfig"), data, 0o644); err != nil {
			return err
		}
	}
	if err := git.InitFull(); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(home, ".gitconfig"))
	if err != nil {
		return err
	}
	return os.WriteFile(saved, data, 0o644)
}

func InitIntegrationTest() error {
	testlogger.Init()

	err := setting.PrepareIntegrationTestConfig()
	if err != nil {
		return err
	}

	setting.SetupGiteaTestEnv()
	setting.Repository.DefaultBranch = "master" // many test code still assume that default branch is called "master"
	// the hooks run this binary as gitea, see RunAsGitea
	if setting.AppPath, err = os.Executable(); err != nil {
		return err
	}

	// a git command in a missing test repository must not find and change the source checkout
	if err := os.Setenv("GIT_CEILING_DIRECTORIES", setting.GetGiteaTestSourceRoot()); err != nil {
		return err
	}

	if err := initGitHome(); err != nil {
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
	if err := prepareFixtureRepos(); err != nil {
		return err
	}

	routers.InitWebInstalled(graceful.GetManager().HammerContext())
	return nil
}

// prepareFixtureRepos copies the fixture repositories once and gives them the hooks that new repositories get
func prepareFixtureRepos() error {
	fixtureReposPath = filepath.Join(setting.AppWorkPath, "fixture-repositories")
	if err := unittest.SyncDirs(filepath.Join(setting.GetGiteaTestSourceRoot(), "tests/gitea-repositories-meta"), fixtureReposPath); err != nil {
		return err
	}
	hookDirs, err := filepath.Glob(filepath.Join(fixtureReposPath, "*", "*.git", "hooks"))
	if err != nil {
		return err
	}
	for _, hookDir := range hookDirs {
		if err := gitrepo.CreateDelegateHooksInDir(hookDir); err != nil {
			return err
		}
	}
	return nil
}

func PrepareGitRepoDirectory(t testing.TB) {
	if !assert.NotEmpty(t, setting.RepoRootPath) {
		return
	}
	assert.NoError(t, unittest.SyncDirs(fixtureReposPath, setting.RepoRootPath))
}

func PrepareLFSStorage(t testing.TB) {
	// load LFS object fixtures
	// (LFS storage can be on any of several backends, including remote servers, so init it with the storage API)
	lfsFixtures, err := storage.NewStorage(setting.LocalStorageType, &setting.Storage{
		Path: filepath.Join(setting.GetGiteaTestSourceRoot(), "tests/gitea-lfs-meta"),
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
