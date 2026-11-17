// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package git

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func enableTestBatchPool(t *testing.T, idle time.Duration) {
	root, err := filepath.Abs(testReposDir)
	require.NoError(t, err)
	// a context that never ends: cancelling it would run ReleaseBatches later, in whichever test runs then
	EnableBatchPool(context.Background(), idle, root)
	t.Cleanup(func() {
		ReleaseBatches("")
		catFileBatchPool.mu.Lock()
		catFileBatchPool.ctx = nil
		catFileBatchPool.mu.Unlock()
	})
}

func pooledBatchOf(repoPath string) *catFileBatchCommand {
	catFileBatchPool.mu.Lock()
	defer catFileBatchPool.mu.Unlock()
	if e := catFileBatchPool.entries[repoPath]; e != nil {
		return e.batch
	}
	return nil
}

func TestCatFileBatchPool(t *testing.T) {
	enableTestBatchPool(t, time.Minute)
	repoPath, err := filepath.Abs(filepath.Join(testReposDir, "repo1_bare"))
	require.NoError(t, err)
	const head = "ce064814f4a0d337b333e646ece456cd39fab612"

	use := func(t *testing.T, ctx context.Context, fn func(batch CatFileBatch)) *catFileBatchCommand {
		repo, err := OpenRepository(ctx, repoPath)
		require.NoError(t, err)
		batch, done, err := repo.CatFileBatch(ctx)
		require.NoError(t, err)
		fn(batch)
		done()
		b := repo.catFileBatchCloser.(*catFileBatchCommand)
		require.NoError(t, repo.Close())
		return b
	}
	queryHead := func(batch CatFileBatch) {
		info, err := batch.QueryInfo(head)
		require.NoError(t, err)
		assert.Equal(t, "commit", info.Type)
	}

	t.Run("Reuse", func(t *testing.T) {
		first := use(t, t.Context(), queryHead)
		assert.Same(t, first, pooledBatchOf(repoPath))
		second := use(t, t.Context(), queryHead)
		assert.Same(t, first, second, "the next repository takes the pooled process")
	})

	t.Run("ContentFullyRead", func(t *testing.T) {
		b := use(t, t.Context(), func(batch CatFileBatch) {
			info, rd, err := batch.QueryContent(head)
			require.NoError(t, err)
			_, err = rd.Discard(int(info.Size)) // the LF after the content stays unread, as callers leave it
			require.NoError(t, err)
		})
		assert.Same(t, b, pooledBatchOf(repoPath))
		use(t, t.Context(), queryHead)
	})

	t.Run("ContentLeftUnread", func(t *testing.T) {
		use(t, t.Context(), func(batch CatFileBatch) {
			_, _, err := batch.QueryContent(head)
			require.NoError(t, err)
		})
		assert.Nil(t, pooledBatchOf(repoPath), "a process with unread output is not kept")
		use(t, t.Context(), queryHead)
	})

	t.Run("ContextEnded", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		use(t, ctx, func(batch CatFileBatch) {
			queryHead(batch)
			cancel()
		})
		assert.Nil(t, pooledBatchOf(repoPath), "the context ended the process")
		use(t, t.Context(), queryHead)
	})

	t.Run("OutsideRoot", func(t *testing.T) {
		tmpRepo := filepath.Join(t.TempDir(), "tmp.git")
		require.NoError(t, InitRepository(t.Context(), tmpRepo, true, Sha1ObjectFormat.Name()))
		repo, err := OpenRepository(t.Context(), tmpRepo)
		require.NoError(t, err)
		_, err = repo.GetRefCommitID("refs/heads/master")
		require.Error(t, err)
		require.NoError(t, repo.Close())
		assert.Nil(t, pooledBatchOf(tmpRepo), "a temporary repository is removed right after, so its process ends with it")
	})

	t.Run("Release", func(t *testing.T) {
		use(t, t.Context(), queryHead)
		require.NotNil(t, pooledBatchOf(repoPath))
		ReleaseBatches(filepath.Dir(repoPath))
		assert.Nil(t, pooledBatchOf(repoPath))

		// a batch in use while its repository is released is not kept either
		use(t, t.Context(), func(batch CatFileBatch) {
			queryHead(batch)
			ReleaseBatches(repoPath)
		})
		assert.Nil(t, pooledBatchOf(repoPath))
	})
}

func TestCatFileBatchPoolIdle(t *testing.T) {
	enableTestBatchPool(t, 500*time.Millisecond) // the pool must still hold it right after Close on a busy machine
	repoPath, err := filepath.Abs(filepath.Join(testReposDir, "repo1_bare"))
	require.NoError(t, err)

	repo, err := OpenRepository(t.Context(), repoPath)
	require.NoError(t, err)
	_, err = repo.GetRefCommitID("refs/heads/master")
	require.NoError(t, err)
	require.NoError(t, repo.Close())
	require.NotNil(t, pooledBatchOf(repoPath))
	assert.Eventually(t, func() bool { return pooledBatchOf(repoPath) == nil }, 5*time.Second, 20*time.Millisecond)
}
