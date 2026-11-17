// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package pull

import (
	"strings"
	"testing"

	"gitea.dev/modules/git"
	"gitea.dev/modules/git/gitcmd"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendTmpRepoConfig(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, git.InitRepository(t.Context(), dir, false, git.Sha1ObjectFormat.Name()))
	value := `C:\Users\a "b"\repo.git` + "\tx"
	require.NoError(t, appendTmpRepoConfig(dir, "[remote \"origin\"]\n\turl = "+gitConfigValue(value)+"\n[maintenance]\n\tauto = false\n"))

	get := func(key string) string {
		stdout, _, err := gitcmd.NewCommand("config", "--get").AddDynamicArguments(key).WithDir(dir).RunStdString(t.Context())
		require.NoError(t, err)
		return strings.TrimSuffix(stdout, "\n")
	}
	assert.Equal(t, value, get("remote.origin.url"))
	assert.Equal(t, "false", get("maintenance.auto"))
}
