// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseGitHookCommitRefLine(t *testing.T) {
	oldCommitID, newCommitID, refName, ok := parseGitHookCommitRefLine("a b c")
	assert.True(t, ok)
	assert.Equal(t, "a", oldCommitID)
	assert.Equal(t, "b", newCommitID)
	assert.Equal(t, "c", string(refName))

	_, _, _, ok = parseGitHookCommitRefLine("a\tb\tc")
	assert.False(t, ok)

	_, _, _, ok = parseGitHookCommitRefLine("a b")
	assert.False(t, ok)
}
