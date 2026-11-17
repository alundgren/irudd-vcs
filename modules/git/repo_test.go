// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package git

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRepoIsEmpty(t *testing.T) {
	emptyRepo2Path := filepath.Join(testReposDir, "repo2_empty")
	repo, err := OpenRepository(t.Context(), emptyRepo2Path)
	assert.NoError(t, err)
	defer repo.Close()
	isEmpty, err := repo.IsEmpty()
	assert.NoError(t, err)
	assert.True(t, isEmpty)
}

// TestCloneRefusesRedirects ensures Clone never follows HTTP redirects, so a remote
// cannot redirect to an otherwise-blocked address (SSRF, e.g. during migration).
func TestCloneRefusesRedirects(t *testing.T) {
	t.Skip("FIXME: GIT-CLONE-HTTP-REDIRECT-SSRF: need a complete solution in the future")
	var targetHit atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHit.Store(true)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer target.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusFound)
	}))
	defer redirect.Close()

	err := Clone(t.Context(), redirect.URL, filepath.Join(t.TempDir(), "dst"), CloneRepoOptions{})
	assert.Error(t, err)
	assert.False(t, targetHit.Load(), "git must not follow the redirect to the target")
}

func TestShowPrettyFormatLogToListCached(t *testing.T) {
	repo, err := OpenRepository(t.Context(), filepath.Join(testReposDir, "repo1_bare"))
	assert.NoError(t, err)
	defer repo.Close()
	const from, to = "37991dec2c8e592043f47155ce4808d4580f9123", "ce064814f4a0d337b333e646ece456cd39fab612"

	for range 2 {
		commits, err := repo.ShowPrettyFormatLogToList(t.Context(), from+".."+to)
		assert.NoError(t, err)
		if assert.Len(t, commits, 2) {
			assert.Equal(t, to, commits[0].ID.String())
			assert.Equal(t, "feaf4ba6bc635fec442f46ddd4512416ec43c2c2", commits[1].ID.String())
		}
		assert.True(t, commitRangeLogs.Contains(from+".."+to))
	}

	_, err = repo.ShowPrettyFormatLogToList(t.Context(), from+"..master")
	assert.NoError(t, err)
	assert.False(t, commitRangeLogs.Contains(from+"..master"), "refs move, so they are not cached")
}
