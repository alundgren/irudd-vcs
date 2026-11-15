// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_getStorageInheritNameSectionTypeForLFS(t *testing.T) {
	iniStr := `
	[storage]
	STORAGE_TYPE = minio
	`
	cfg, err := NewConfigProviderFromData(iniStr)
	assert.NoError(t, err)
	assert.NoError(t, loadLFSFrom(cfg))

	assert.EqualValues(t, "minio", LFS.Storage.Type)
	assert.Equal(t, "lfs/", LFS.Storage.MinioConfig.BasePath)

	iniStr = `
[server]
LFS_CONTENT_PATH = path_ignored
[lfs]
PATH = path_used
`
	cfg, err = NewConfigProviderFromData(iniStr)
	assert.NoError(t, err)
	assert.NoError(t, loadLFSFrom(cfg))

	assert.EqualValues(t, "local", LFS.Storage.Type)
	assert.Contains(t, LFS.Storage.Path, "path_used")

	iniStr = `
[server]
LFS_CONTENT_PATH = deprecatedpath
`
	cfg, err = NewConfigProviderFromData(iniStr)
	assert.NoError(t, err)
	assert.NoError(t, loadLFSFrom(cfg))

	assert.EqualValues(t, "local", LFS.Storage.Type)
	assert.Contains(t, LFS.Storage.Path, "deprecatedpath")

	iniStr = `
[storage.lfs]
STORAGE_TYPE = minio
`
	cfg, err = NewConfigProviderFromData(iniStr)
	assert.NoError(t, err)
	assert.NoError(t, loadLFSFrom(cfg))

	assert.EqualValues(t, "minio", LFS.Storage.Type)
	assert.Equal(t, "lfs/", LFS.Storage.MinioConfig.BasePath)

	iniStr = `
[lfs]
STORAGE_TYPE = my_minio

[storage.my_minio]
STORAGE_TYPE = minio
`
	cfg, err = NewConfigProviderFromData(iniStr)
	assert.NoError(t, err)
	assert.NoError(t, loadLFSFrom(cfg))

	assert.EqualValues(t, "minio", LFS.Storage.Type)
	assert.Equal(t, "lfs/", LFS.Storage.MinioConfig.BasePath)

	iniStr = `
[lfs]
STORAGE_TYPE = my_minio
MINIO_BASE_PATH = my_lfs/

[storage.my_minio]
STORAGE_TYPE = minio
`
	cfg, err = NewConfigProviderFromData(iniStr)
	assert.NoError(t, err)
	assert.NoError(t, loadLFSFrom(cfg))

	assert.EqualValues(t, "minio", LFS.Storage.Type)
	assert.Equal(t, "my_lfs/", LFS.Storage.MinioConfig.BasePath)
}

func Test_LFSStorage1(t *testing.T) {
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
}

func Test_LFSServerConfigs(t *testing.T) {
	iniStr := `
[server]
LFS_MAX_BATCH_SIZE = 100
`
	cfg, err := NewConfigProviderFromData(iniStr)
	assert.NoError(t, err)

	assert.NoError(t, loadLFSFrom(cfg))
	assert.Equal(t, 100, LFS.MaxBatchSize)
}
