// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package gitrepo

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateDelegateHooksPermissions(t *testing.T) {
	hookDir := t.TempDir()
	existingHookDir := filepath.Join(hookDir, "post-receive.d")
	require.NoError(t, os.MkdirAll(existingHookDir, 0o777))
	require.NoError(t, os.Chmod(existingHookDir, 0o777))

	require.NoError(t, createDelegateHooks(hookDir))

	hookNames, _, _ := getHookTemplates()
	for _, hookName := range hookNames {
		for _, path := range []string{
			filepath.Join(hookDir, hookName),
			filepath.Join(hookDir, hookName+".d"),
			filepath.Join(hookDir, hookName+".d", "gitea"),
		} {
			info, err := os.Stat(path)
			require.NoError(t, err)
			require.Equal(t, os.FileMode(0o755), info.Mode().Perm(), path)
		}
	}
}

// hookShell finds the shell that git runs hooks with, avoiding the WSL bash on Windows
func hookShell(t *testing.T) string {
	if runtime.GOOS != "windows" {
		sh, err := exec.LookPath("sh")
		if err != nil {
			t.Skip("no sh")
		}
		return sh
	}
	out, err := exec.Command("git", "--exec-path").Output()
	if err != nil {
		t.Skip("no git")
	}
	// <git>/mingw64/libexec/git-core
	sh := filepath.Join(strings.TrimSpace(string(out)), "..", "..", "..", "bin", "sh.exe")
	if _, err := os.Stat(sh); err != nil {
		t.Skip("no sh next to git")
	}
	return sh
}

func TestHookDispatcher(t *testing.T) {
	sh := hookShell(t)
	const refs = "old new refs/heads/main\n"

	run := func(t *testing.T, hookName string, hooks map[string]string, args ...string) (string, int) {
		gitDir := t.TempDir()
		hookDir := filepath.Join(gitDir, "hooks")
		require.NoError(t, os.MkdirAll(filepath.Join(hookDir, hookName+".d"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(hookDir, hookName), []byte(hookDispatcherTpl(hookName != "update")), 0o755))
		for name, body := range hooks {
			require.NoError(t, os.WriteFile(filepath.Join(hookDir, hookName+".d", name), []byte("#!/bin/sh\n"+body+"\n"), 0o755))
		}
		cmd := exec.Command(sh, append([]string{filepath.ToSlash(filepath.Join(hookDir, hookName))}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_DIR="+filepath.ToSlash(gitDir))
		cmd.Stdin = strings.NewReader(refs)
		out, err := cmd.Output()
		code := 0
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			code = exitErr.ExitCode()
		} else {
			require.NoError(t, err)
		}
		return strings.ReplaceAll(string(out), "\r", ""), code
	}

	t.Run("SourcesGitea", func(t *testing.T) {
		out, code := run(t, "pre-receive", map[string]string{"gitea": `echo "gitea $hookname"; exec cat`})
		assert.Equal(t, "gitea pre-receive\n"+refs, out) // $hookname is only set when sourced
		assert.Zero(t, code)
	})
	t.Run("GiteaWithoutExec", func(t *testing.T) {
		out, code := run(t, "post-receive", map[string]string{"gitea": "cat; false"})
		assert.Equal(t, refs, out) // runs once and keeps its status
		assert.Equal(t, 1, code)
	})
	t.Run("UpdateArgs", func(t *testing.T) {
		out, code := run(t, "update", map[string]string{"gitea": `exec echo "update $1 $2 $3"`}, "refs/heads/main", "a", "b")
		assert.Equal(t, "update refs/heads/main a b\n", out)
		assert.Zero(t, code)
	})
	t.Run("SingleCustom", func(t *testing.T) {
		out, code := run(t, "pre-receive", map[string]string{"pre-receive": `echo "custom $hookname"; cat; exit 2`})
		assert.Equal(t, "custom \n"+refs, out) // exec'd, not sourced
		assert.Equal(t, 2, code)
	})
	t.Run("Several", func(t *testing.T) {
		out, code := run(t, "pre-receive", map[string]string{"gitea": "echo gitea; cat", "pre-receive": "echo custom; cat; exit 4"})
		assert.Equal(t, "gitea\n"+refs+"custom\n"+refs, out)
		assert.Equal(t, 4, code)
	})
	t.Run("SeveralUpdate", func(t *testing.T) {
		out, code := run(t, "update", map[string]string{"gitea": `echo "gitea $1 $2 $3"`, "update": `echo "custom $1 $2 $3"`}, "r", "a", "b")
		assert.Equal(t, "gitea r a b\ncustom r a b\n", out)
		assert.Zero(t, code)
	})
	t.Run("None", func(t *testing.T) {
		out, code := run(t, "pre-receive", nil)
		assert.Empty(t, out)
		assert.Zero(t, code)
	})
}
