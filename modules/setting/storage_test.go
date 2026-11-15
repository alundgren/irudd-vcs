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

func Test_getStorageMultipleName(t *testing.T) {
	iniStr := `
[lfs]
MINIO_BUCKET = gitea-lfs

[other]
MINIO_BUCKET = gitea-other

[storage]
STORAGE_TYPE = minio
MINIO_BUCKET = gitea-storage
`
	cfg, err := NewConfigProviderFromData(iniStr)
	assert.NoError(t, err)

	other, err := namedStorageLoader("other", "others")(cfg)
	assert.NoError(t, err)
	assert.Equal(t, "gitea-other", other.MinioConfig.Bucket)
	assert.Equal(t, "others/", other.MinioConfig.BasePath)

	assert.NoError(t, loadLFSFrom(cfg))
	assert.Equal(t, "gitea-lfs", LFS.Storage.MinioConfig.Bucket)
	assert.Equal(t, "lfs/", LFS.Storage.MinioConfig.BasePath)

	plain, err := namedStorageLoader("plain", "plains")(cfg)
	assert.NoError(t, err)
	assert.Equal(t, "gitea-storage", plain.MinioConfig.Bucket)
	assert.Equal(t, "plains/", plain.MinioConfig.BasePath)
}

func Test_getStorageUseOtherNameAsType(t *testing.T) {
	iniStr := `
[other]
STORAGE_TYPE = lfs

[storage.lfs]
STORAGE_TYPE = minio
MINIO_BUCKET = gitea-storage
`
	cfg, err := NewConfigProviderFromData(iniStr)
	assert.NoError(t, err)

	other, err := namedStorageLoader("other", "others")(cfg)
	assert.NoError(t, err)
	assert.Equal(t, "gitea-storage", other.MinioConfig.Bucket)
	assert.Equal(t, "others/", other.MinioConfig.BasePath)

	assert.NoError(t, loadLFSFrom(cfg))
	assert.Equal(t, "gitea-storage", LFS.Storage.MinioConfig.Bucket)
	assert.Equal(t, "lfs/", LFS.Storage.MinioConfig.BasePath)
}

func Test_getStorageInheritStorageType(t *testing.T) {
	iniStr := `
[storage]
STORAGE_TYPE = minio
`
	cfg, err := NewConfigProviderFromData(iniStr)
	assert.NoError(t, err)

	assert.NoError(t, loadLFSFrom(cfg))
	assert.EqualValues(t, "minio", LFS.Storage.Type)
	assert.Equal(t, "gitea", LFS.Storage.MinioConfig.Bucket)
	assert.Equal(t, "lfs/", LFS.Storage.MinioConfig.BasePath)

	other, err := namedStorageLoader("other", "others")(cfg)
	assert.NoError(t, err)
	assert.EqualValues(t, "minio", other.Type)
	assert.Equal(t, "gitea", other.MinioConfig.Bucket)
	assert.Equal(t, "others/", other.MinioConfig.BasePath)
}

func Test_getStorageInheritStorageTypeAzureBlob(t *testing.T) {
	iniStr := `
[storage]
STORAGE_TYPE = azureblob
`
	cfg, err := NewConfigProviderFromData(iniStr)
	assert.NoError(t, err)

	assert.NoError(t, loadLFSFrom(cfg))
	assert.EqualValues(t, "azureblob", LFS.Storage.Type)
	assert.Equal(t, "gitea", LFS.Storage.AzureBlobConfig.Container)
	assert.Equal(t, "lfs/", LFS.Storage.AzureBlobConfig.BasePath)

	other, err := namedStorageLoader("other", "others")(cfg)
	assert.NoError(t, err)
	assert.EqualValues(t, "azureblob", other.Type)
	assert.Equal(t, "gitea", other.AzureBlobConfig.Container)
	assert.Equal(t, "others/", other.AzureBlobConfig.BasePath)
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

func Test_getStorageConfiguration23(t *testing.T) {
	cfg, err := NewConfigProviderFromData(`
[lfs]
STORAGE_TYPE = minio
MINIO_ACCESS_KEY_ID = my_access_key
MINIO_SECRET_ACCESS_KEY = my_secret_key
`)
	assert.NoError(t, err)

	_, err = getStorage(cfg, "", "", nil)
	assert.Error(t, err)

	assert.NoError(t, loadLFSFrom(cfg))
	cp := LFS.Storage.ToShadowCopy()
	assert.Equal(t, "******", cp.MinioConfig.AccessKeyID)
	assert.Equal(t, "******", cp.MinioConfig.SecretAccessKey)
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

func Test_getStorageConfiguration25(t *testing.T) {
	cfg, err := NewConfigProviderFromData(`
[lfs]
STORAGE_TYPE = my_lfs

[storage.my_lfs]
; unsupported, storage type should be known type
STORAGE_TYPE = unknown // should be local or minio
PATH = lfs-dir
`)
	assert.NoError(t, err)
	assert.Error(t, loadLFSFrom(cfg))
}

func Test_getStorageConfiguration26(t *testing.T) {
	cfg, err := NewConfigProviderFromData(`
[lfs]
STORAGE_TYPE = minio
MINIO_ACCESS_KEY_ID = my_access_key
MINIO_SECRET_ACCESS_KEY = my_secret_key
; wrong configuration
MINIO_USE_SSL = abc
`)
	assert.NoError(t, err)
	// assert.Error(t, loadLFSFrom(cfg))
	// FIXME: this should return error but now ini package's MapTo() doesn't check type
	assert.NoError(t, loadLFSFrom(cfg))
}

func Test_getStorageConfiguration27(t *testing.T) {
	cfg, err := NewConfigProviderFromData(`
[storage.lfs]
STORAGE_TYPE = minio
MINIO_ACCESS_KEY_ID = my_access_key
MINIO_SECRET_ACCESS_KEY = my_secret_key
MINIO_USE_SSL = true
`)
	assert.NoError(t, err)
	assert.NoError(t, loadLFSFrom(cfg))
	assert.Equal(t, "my_access_key", LFS.Storage.MinioConfig.AccessKeyID)
	assert.Equal(t, "my_secret_key", LFS.Storage.MinioConfig.SecretAccessKey)
	assert.True(t, LFS.Storage.MinioConfig.UseSSL)
	assert.Equal(t, "lfs/", LFS.Storage.MinioConfig.BasePath)
}

func Test_getStorageConfiguration28(t *testing.T) {
	cfg, err := NewConfigProviderFromData(`
[storage]
STORAGE_TYPE = minio
MINIO_ACCESS_KEY_ID = my_access_key
MINIO_SECRET_ACCESS_KEY = my_secret_key
MINIO_USE_SSL = true
MINIO_BASE_PATH = /prefix
`)
	assert.NoError(t, err)
	assert.NoError(t, loadLFSFrom(cfg))
	assert.Equal(t, "my_access_key", LFS.Storage.MinioConfig.AccessKeyID)
	assert.Equal(t, "my_secret_key", LFS.Storage.MinioConfig.SecretAccessKey)
	assert.True(t, LFS.Storage.MinioConfig.UseSSL)
	assert.Equal(t, "/prefix/lfs/", LFS.Storage.MinioConfig.BasePath)

	cfg, err = NewConfigProviderFromData(`
[storage]
STORAGE_TYPE = minio
MINIO_IAM_ENDPOINT = 127.0.0.1
MINIO_USE_SSL = true
MINIO_BASE_PATH = /prefix
`)
	assert.NoError(t, err)
	assert.NoError(t, loadLFSFrom(cfg))
	assert.Equal(t, "127.0.0.1", LFS.Storage.MinioConfig.IamEndpoint)
	assert.True(t, LFS.Storage.MinioConfig.UseSSL)
	assert.Equal(t, "/prefix/lfs/", LFS.Storage.MinioConfig.BasePath)

	cfg, err = NewConfigProviderFromData(`
[storage]
STORAGE_TYPE = minio
MINIO_ACCESS_KEY_ID = my_access_key
MINIO_SECRET_ACCESS_KEY = my_secret_key
MINIO_USE_SSL = true
MINIO_BASE_PATH = /prefix

[lfs]
MINIO_BASE_PATH = /lfs
`)
	assert.NoError(t, err)
	assert.NoError(t, loadLFSFrom(cfg))
	assert.Equal(t, "my_access_key", LFS.Storage.MinioConfig.AccessKeyID)
	assert.Equal(t, "my_secret_key", LFS.Storage.MinioConfig.SecretAccessKey)
	assert.True(t, LFS.Storage.MinioConfig.UseSSL)
	assert.Equal(t, "/lfs", LFS.Storage.MinioConfig.BasePath)

	cfg, err = NewConfigProviderFromData(`
[storage]
STORAGE_TYPE = minio
MINIO_ACCESS_KEY_ID = my_access_key
MINIO_SECRET_ACCESS_KEY = my_secret_key
MINIO_USE_SSL = true
MINIO_BASE_PATH = /prefix

[storage.lfs]
MINIO_BASE_PATH = /lfs
`)
	assert.NoError(t, err)
	assert.NoError(t, loadLFSFrom(cfg))
	assert.Equal(t, "my_access_key", LFS.Storage.MinioConfig.AccessKeyID)
	assert.Equal(t, "my_secret_key", LFS.Storage.MinioConfig.SecretAccessKey)
	assert.True(t, LFS.Storage.MinioConfig.UseSSL)
	assert.Equal(t, "/lfs", LFS.Storage.MinioConfig.BasePath)
}

func Test_getStorageConfiguration29(t *testing.T) {
	cfg, err := NewConfigProviderFromData(`
[lfs]
STORAGE_TYPE = azureblob
AZURE_BLOB_ACCOUNT_NAME = my_account_name
AZURE_BLOB_ACCOUNT_KEY = my_account_key
`)
	assert.NoError(t, err)
	// assert.Error(t, loadLFSFrom(cfg))
	// FIXME: this should return error but now ini package's MapTo() doesn't check type
	assert.NoError(t, loadLFSFrom(cfg))
}

func Test_getStorageConfiguration30(t *testing.T) {
	cfg, err := NewConfigProviderFromData(`
[storage.lfs]
STORAGE_TYPE = azureblob
AZURE_BLOB_ACCOUNT_NAME = my_account_name
AZURE_BLOB_ACCOUNT_KEY = my_account_key
`)
	assert.NoError(t, err)
	assert.NoError(t, loadLFSFrom(cfg))
	assert.Equal(t, "my_account_name", LFS.Storage.AzureBlobConfig.AccountName)
	assert.Equal(t, "my_account_key", LFS.Storage.AzureBlobConfig.AccountKey)
	assert.Equal(t, "lfs/", LFS.Storage.AzureBlobConfig.BasePath)
}

func Test_getStorageConfiguration31(t *testing.T) {
	cfg, err := NewConfigProviderFromData(`
[storage]
STORAGE_TYPE = azureblob
AZURE_BLOB_ACCOUNT_NAME = my_account_name
AZURE_BLOB_ACCOUNT_KEY = my_account_key
AZURE_BLOB_BASE_PATH = /prefix
`)
	assert.NoError(t, err)
	assert.NoError(t, loadLFSFrom(cfg))
	assert.Equal(t, "my_account_name", LFS.Storage.AzureBlobConfig.AccountName)
	assert.Equal(t, "my_account_key", LFS.Storage.AzureBlobConfig.AccountKey)
	assert.Equal(t, "/prefix/lfs/", LFS.Storage.AzureBlobConfig.BasePath)

	cfg, err = NewConfigProviderFromData(`
[storage]
STORAGE_TYPE = azureblob
AZURE_BLOB_ACCOUNT_NAME = my_account_name
AZURE_BLOB_ACCOUNT_KEY = my_account_key
AZURE_BLOB_BASE_PATH = /prefix

[lfs]
AZURE_BLOB_BASE_PATH = /lfs
`)
	assert.NoError(t, err)
	assert.NoError(t, loadLFSFrom(cfg))
	assert.Equal(t, "my_account_name", LFS.Storage.AzureBlobConfig.AccountName)
	assert.Equal(t, "my_account_key", LFS.Storage.AzureBlobConfig.AccountKey)
	assert.Equal(t, "/lfs", LFS.Storage.AzureBlobConfig.BasePath)

	cfg, err = NewConfigProviderFromData(`
[storage]
STORAGE_TYPE = azureblob
AZURE_BLOB_ACCOUNT_NAME = my_account_name
AZURE_BLOB_ACCOUNT_KEY = my_account_key
AZURE_BLOB_BASE_PATH = /prefix

[storage.lfs]
AZURE_BLOB_BASE_PATH = /lfs
`)
	assert.NoError(t, err)
	assert.NoError(t, loadLFSFrom(cfg))
	assert.Equal(t, "my_account_name", LFS.Storage.AzureBlobConfig.AccountName)
	assert.Equal(t, "my_account_key", LFS.Storage.AzureBlobConfig.AccountKey)
	assert.Equal(t, "/lfs", LFS.Storage.AzureBlobConfig.BasePath)
}
