// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package gitrepo

import (
	"os"
	"path/filepath"
	"testing"

	"gitea.dev/modules/git"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetDefaultBranch(t *testing.T) {
	repoDir := filepath.Join(t.TempDir(), "repo.git")
	require.NoError(t, git.InitRepository(t.Context(), repoDir, true, git.Sha1ObjectFormat.Name()))
	repo := &mockRepository{path: repoDir}
	readHead := func() string {
		head, err := os.ReadFile(filepath.Join(repoDir, "HEAD"))
		require.NoError(t, err)
		return string(head)
	}

	require.NoError(t, SetDefaultBranch(t.Context(), repo, "dev"))
	assert.Equal(t, "ref: refs/heads/dev\n", readHead())
	name, err := GetDefaultBranch(t.Context(), repo)
	require.NoError(t, err)
	assert.Equal(t, "dev", name)

	// already set: HEAD is left as it is
	info, err := os.Stat(filepath.Join(repoDir, "HEAD"))
	require.NoError(t, err)
	require.NoError(t, SetDefaultBranch(t.Context(), repo, "dev"))
	again, err := os.Stat(filepath.Join(repoDir, "HEAD"))
	require.NoError(t, err)
	assert.Equal(t, info.ModTime(), again.ModTime())

	require.NoError(t, SetDefaultBranch(t.Context(), repo, "main"))
	assert.Equal(t, "ref: refs/heads/main\n", readHead())
}
