// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package gitrepo

import (
	"strings"
	"testing"

	"gitea.dev/modules/git/gitcmd"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeBaseCached(t *testing.T) {
	repo := &mockRepository{path: "repo1_bare"}
	revParse := func(ref string) string {
		id, _, err := RunCmdString(t.Context(), repo, gitcmd.NewCommand("rev-parse").AddDynamicArguments(ref))
		require.NoError(t, err)
		return strings.TrimSpace(id)
	}
	master, branch2 := revParse("master"), revParse("branch2")

	_, err := MergeBase(t.Context(), repo, "master", "branch2")
	require.NoError(t, err)
	assert.False(t, mergeBases.Contains("master branch2"), "refs move, so they are not cached")

	expected, err := MergeBase(t.Context(), repo, master, branch2)
	require.NoError(t, err)
	assert.True(t, mergeBases.Contains(master+" "+branch2))
	cached, err := MergeBase(t.Context(), repo, master, branch2)
	require.NoError(t, err)
	assert.Equal(t, expected, cached)
	assert.Equal(t, revParse("branch2~1"), cached)
}
