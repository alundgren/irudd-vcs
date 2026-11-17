// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
