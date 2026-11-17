// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package pull

import (
	"bytes"
	"testing"

	issues_model "gitea.dev/models/issues"
	"gitea.dev/models/unittest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownloadDiffOrPatchCached(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	pr := unittest.AssertExistsAndLoadBean(t, &issues_model.PullRequest{ID: 2})

	for _, kind := range []struct{ patch, binary bool }{{false, false}, {true, false}, {false, true}} {
		prDiffCache.Purge()
		var first, second bytes.Buffer
		require.NoError(t, DownloadDiffOrPatch(t.Context(), pr, &first, kind.patch, kind.binary))
		assert.Equal(t, 1, prDiffCache.Len(), "the diff of two commit IDs is cached")
		require.NoError(t, DownloadDiffOrPatch(t.Context(), pr, &second, kind.patch, kind.binary))
		assert.NotEmpty(t, first.String())
		assert.Equal(t, first.String(), second.String())
	}

	// a short merge base is not a commit ID, so the diff is not cached
	prDiffCache.Purge()
	short := *pr
	short.MergeBase = pr.MergeBase[:10]
	var out bytes.Buffer
	require.NoError(t, DownloadDiffOrPatch(t.Context(), &short, &out, false, false))
	assert.Zero(t, prDiffCache.Len())

	// a diff over the size limit is written but not cached
	buf := &cappedBuffer{w: &out, limit: 4}
	_, err := buf.Write([]byte("12345"))
	require.NoError(t, err)
	assert.True(t, buf.over)
}
