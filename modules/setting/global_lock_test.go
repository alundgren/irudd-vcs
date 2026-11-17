// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadGlobalLockConfig(t *testing.T) {
	t.Run("DefaultGlobalLockConfig", func(t *testing.T) {
		iniStr := ``
		cfg, err := NewConfigProviderFromData(iniStr)
		assert.NoError(t, err)

		loadGlobalLockFrom(cfg)
		assert.Equal(t, "memory", GlobalLock.ServiceType)
	})
}
