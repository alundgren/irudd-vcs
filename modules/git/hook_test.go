// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomUpdateHookRuns(t *testing.T) {
	gitCmd := func(dir string, args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	bare := filepath.Join(t.TempDir(), "repo.git")
	_, err := gitCmd("", "init", "--bare", bare)
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(filepath.Join(bare, "hooks", "update")))

	hook, err := GetHook(bare, "update")
	require.NoError(t, err)
	hook.Content = "#!/bin/sh\necho \"custom update $1\"\ncase $1 in refs/heads/blocked) exit 1 ;; esac\n"
	require.NoError(t, hook.Update())
	assert.FileExists(t, filepath.Join(bare, "hooks", "update"), "the custom hook needs a dispatcher")

	work := t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"-c", "user.name=u", "-c", "user.email=u@example.invalid", "commit", "--allow-empty", "-m", "c"},
	} {
		_, err := gitCmd(work, args...)
		require.NoError(t, err)
	}
	out, err := gitCmd(work, "push", bare, "HEAD:refs/heads/main", "HEAD:refs/heads/blocked")
	assert.Error(t, err)
	assert.Contains(t, out, "custom update refs/heads/main")
	assert.Contains(t, out, "custom update refs/heads/blocked")
	_, err = gitCmd(bare, "rev-parse", "--verify", "refs/heads/main")
	assert.NoError(t, err)
	_, err = gitCmd(bare, "rev-parse", "--verify", "refs/heads/blocked")
	assert.Error(t, err, "the hook refused the ref")
}
