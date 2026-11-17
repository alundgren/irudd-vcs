// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package globallock

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocker(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		locker := NewMemoryLocker()
		testLocker(t, locker)
		testMemoryLocker(t, locker.(*memoryLocker))
	})
}

func testLocker(t *testing.T, locker Locker) {
	t.Run("lock", func(t *testing.T) {
		parentCtx := t.Context()
		release, err := locker.Lock(parentCtx, "test")
		defer release()

		assert.NoError(t, err)

		func() {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			release, err := locker.Lock(ctx, "test")
			defer release()

			assert.Error(t, err)
		}()

		release()

		func() {
			release, err := locker.Lock(t.Context(), "test")
			defer release()

			assert.NoError(t, err)
		}()
	})

	t.Run("try lock", func(t *testing.T) {
		parentCtx := t.Context()
		ok, release, err := locker.TryLock(parentCtx, "test")
		defer release()

		assert.True(t, ok)
		assert.NoError(t, err)

		func() {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			ok, release, err := locker.TryLock(ctx, "test")
			defer release()

			assert.False(t, ok)
			assert.NoError(t, err)
		}()

		release()

		func() {
			ok, release, _ := locker.TryLock(t.Context(), "test")
			defer release()

			assert.True(t, ok)
		}()
	})

	t.Run("wait and acquired", func(t *testing.T) {
		ctx := t.Context()
		release, err := locker.Lock(ctx, "test")
		require.NoError(t, err)

		wg := &sync.WaitGroup{}
		wg.Go(func() {
			started := time.Now()
			release, err := locker.Lock(t.Context(), "test") // should be blocked for seconds
			defer release()
			assert.Greater(t, time.Since(started), time.Second)
			assert.NoError(t, err)
		})

		time.Sleep(2 * time.Second)
		release()

		wg.Wait()
	})

	t.Run("multiple release", func(t *testing.T) {
		ctx := t.Context()

		release1, err := locker.Lock(ctx, "test")
		require.NoError(t, err)

		release1()

		release2, err := locker.Lock(ctx, "test")
		defer release2()
		require.NoError(t, err)

		// Call release1 again,
		// it should not panic or block,
		// and it shouldn't affect the other lock
		release1()

		ok, release3, err := locker.TryLock(ctx, "test")
		defer release3()
		require.NoError(t, err)
		// It should be able to acquire the lock;
		// otherwise, it means the lock has been released by release1
		assert.False(t, ok)
	})
}

// testMemoryLocker does specific tests for memoryLocker
func testMemoryLocker(t *testing.T, locker *memoryLocker) {
	// nothing to do
}
