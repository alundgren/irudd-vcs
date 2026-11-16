// Copyright 2022 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"testing"

	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unittest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMakeRepoPrivate(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	assert.False(t, repo.IsPrivate)

	assert.NoError(t, MakeRepoPrivate(t.Context(), repo, true))

	updatedRepo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID})
	assert.True(t, updatedRepo.IsPrivate)
}

// TestUpdateRepositoryVisibilityChange covers the shared updateRepository helper (used by the API EditRepo path)
func TestUpdateRepositoryVisibilityChange(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	assert.False(t, repo.IsPrivate)

	repo.IsPrivate = true
	require.NoError(t, updateRepository(t.Context(), repo, true))

	updatedRepo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repo.ID})
	assert.True(t, updatedRepo.IsPrivate)
}
