// Copyright 2020 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// loadLFSStorage loads the LFS storage, the only feature storage left
func loadLFSStorage(rootCfg ConfigProvider) (*Storage, error) {
	if err := loadLFSFrom(rootCfg); err != nil {
		return nil, err
	}
	return LFS.Storage, nil
}

// namedStorageLoader loads storage name with [secName] as override section, like the removed feature loaders did
func namedStorageLoader(secName, name string) func(rootCfg ConfigProvider) (*Storage, error) {
	return func(rootCfg ConfigProvider) (*Storage, error) {
		sec, _ := rootCfg.GetSection(secName)
		return getStorage(rootCfg, name, "", sec)
	}
}

type testLocalStoragePathCase struct {
	loader       func(rootCfg ConfigProvider) (*Storage, error)
	expectedPath string
}

var testIniAbsPathRe = regexp.MustCompile(`(?m)^(PATH\s*=\s*)/`)

// testAbsPath makes unix-style absolute test paths absolute on Windows too, which needs a volume
func testAbsPath(p string) string {
	if runtime.GOOS == "windows" && strings.HasPrefix(p, "/") {
		return "C:" + p
	}
	return p
}

func testLocalStoragePath(t *testing.T, appDataPath, iniStr string, cases []testLocalStoragePathCase) {
	if runtime.GOOS == "windows" {
		iniStr = testIniAbsPathRe.ReplaceAllString(iniStr, "${1}C:/")
	}
	cfg, err := NewConfigProviderFromData(iniStr)
	assert.NoError(t, err)
	AppDataPath = testAbsPath(appDataPath)
	for _, c := range cases {
		storage, err := c.loader(cfg)
		assert.NoError(t, err)

		assert.EqualValues(t, "local", storage.Type)
		assert.True(t, filepath.IsAbs(storage.Path))
		assert.Equal(t, filepath.Clean(testAbsPath(c.expectedPath)), filepath.Clean(storage.Path))
	}
}

func Test_getStorageInheritStorageTypeLocal(t *testing.T) {
	testLocalStoragePath(t, "/appdata", `
[storage]
STORAGE_TYPE = local
`, []testLocalStoragePathCase{
		{loadLFSStorage, "/appdata/lfs"},
		{namedStorageLoader("other", "others"), "/appdata/others"},
	})
}

func Test_getStorageInheritStorageTypeLocalPath(t *testing.T) {
	testLocalStoragePath(t, "/appdata", `
[storage]
STORAGE_TYPE = local
PATH = /data/gitea
`, []testLocalStoragePathCase{
		{loadLFSStorage, "/data/gitea/lfs"},
		{namedStorageLoader("other", "others"), "/data/gitea/others"},
	})
}

func Test_getStorageInheritStorageTypeLocalRelativePath(t *testing.T) {
	testLocalStoragePath(t, "/appdata", `
[storage]
STORAGE_TYPE = local
PATH = storages
`, []testLocalStoragePathCase{
		{loadLFSStorage, "/appdata/storages/lfs"},
		{namedStorageLoader("other", "others"), "/appdata/storages/others"},
	})
}

func Test_getStorageInheritStorageTypeLocalPathOverride(t *testing.T) {
	testLocalStoragePath(t, "/appdata", `
[storage]
STORAGE_TYPE = local
PATH = /data/gitea

[lfs]
PATH = /data/gitea/the-lfs-dir
`, []testLocalStoragePathCase{
		{loadLFSStorage, "/data/gitea/the-lfs-dir"},
		{namedStorageLoader("other", "others"), "/data/gitea/others"},
	})
}

func Test_getStorageInheritStorageTypeLocalPathOverrideEmpty(t *testing.T) {
	testLocalStoragePath(t, "/appdata", `
[storage]
STORAGE_TYPE = local
PATH = /data/gitea

[lfs]
`, []testLocalStoragePathCase{
		{loadLFSStorage, "/data/gitea/lfs"},
		{namedStorageLoader("other", "others"), "/data/gitea/others"},
	})
}

func Test_getStorageInheritStorageTypeLocalRelativePathOverride(t *testing.T) {
	testLocalStoragePath(t, "/appdata", `
[storage]
STORAGE_TYPE = local
PATH = /data/gitea

[lfs]
PATH = the-lfs-dir
`, []testLocalStoragePathCase{
		{loadLFSStorage, "/data/gitea/the-lfs-dir"},
		{namedStorageLoader("other", "others"), "/data/gitea/others"},
	})
}

func Test_getStorageInheritStorageTypeLocalPathOverride3(t *testing.T) {
	testLocalStoragePath(t, "/appdata", `
[storage.lfs]
STORAGE_TYPE = local
PATH = /data/gitea/lfs-dir
`, []testLocalStoragePathCase{
		{loadLFSStorage, "/data/gitea/lfs-dir"},
		{namedStorageLoader("other", "others"), "/appdata/others"},
	})
}

func Test_getStorageInheritStorageTypeLocalPathOverride3_5(t *testing.T) {
	testLocalStoragePath(t, "/appdata", `
[storage.lfs]
STORAGE_TYPE = local
PATH = a-relative-path
`, []testLocalStoragePathCase{
		{loadLFSStorage, "/appdata/a-relative-path"},
		{namedStorageLoader("other", "others"), "/appdata/others"},
	})
}

func Test_getStorageInheritStorageTypeLocalPathOverride4(t *testing.T) {
	testLocalStoragePath(t, "/appdata", `
[storage.lfs]
STORAGE_TYPE = local
PATH = /data/gitea/lfs-dir

[lfs]
PATH = /tmp/gitea/lfs-dir
`, []testLocalStoragePathCase{
		{loadLFSStorage, "/tmp/gitea/lfs-dir"},
		{namedStorageLoader("other", "others"), "/appdata/others"},
	})
}

func Test_getStorageInheritStorageTypeLocalPathOverride5(t *testing.T) {
	testLocalStoragePath(t, "/appdata", `
[storage.lfs]
STORAGE_TYPE = local
PATH = /data/gitea/lfs-dir

[lfs]
`, []testLocalStoragePathCase{
		{loadLFSStorage, "/data/gitea/lfs-dir"},
		{namedStorageLoader("other", "others"), "/appdata/others"},
	})
}

func Test_getStorageInheritStorageTypeLocalPathOverride72(t *testing.T) {
	testLocalStoragePath(t, "/appdata", `
[lfs]
STORAGE_TYPE = local
PATH = lfs-dir
`, []testLocalStoragePathCase{
		{loadLFSStorage, "/appdata/lfs-dir"},
	})
}

func Test_getStorageConfiguration20(t *testing.T) {
	cfg, err := NewConfigProviderFromData(`
[lfs]
STORAGE_TYPE = my_storage
PATH = lfs-dir
`)
	assert.NoError(t, err)

	assert.Error(t, loadLFSFrom(cfg))
}

func Test_getStorageConfiguration21(t *testing.T) {
	testLocalStoragePath(t, "/appdata", `
[storage.lfs]
`, []testLocalStoragePathCase{
		{loadLFSStorage, "/appdata/lfs"},
	})
}

func Test_getStorageConfiguration22(t *testing.T) {
	testLocalStoragePath(t, "/appdata", `
[storage.lfs]
PATH = lfs-dir
`, []testLocalStoragePathCase{
		{loadLFSStorage, "/appdata/lfs-dir"},
	})
}

func Test_getStorageConfiguration24(t *testing.T) {
	cfg, err := NewConfigProviderFromData(`
[lfs]
STORAGE_TYPE = my_lfs

[storage.my_lfs]
; unsupported, storage type should be defined explicitly
PATH = lfs-dir
`)
	assert.NoError(t, err)
	assert.Error(t, loadLFSFrom(cfg))
}
