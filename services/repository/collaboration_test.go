// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"testing"

	"gitea.dev/models/perm"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"

	"github.com/stretchr/testify/assert"
)

func TestRepository_AddCollaborator(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	repo3 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 3})
	user4 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})
	testSuccess := func(repo *repo_model.Repository, user *user_model.User) {
		assert.NoError(t, repo.LoadOwner(t.Context()))
		assert.NoError(t, AddOrUpdateCollaborator(t.Context(), repo, user, perm.AccessModeWrite))
		unittest.CheckConsistencyFor(t, repo, user)
	}
	testSuccess(repo1, user4)
	testSuccess(repo1, user4)
	testSuccess(repo3, user4)

	assert.Error(t, AddOrUpdateCollaborator(t.Context(), repo1, user4, perm.AccessModeOwner))
	assert.NoError(t, AddOrUpdateCollaborator(t.Context(), repo1, user4, perm.AccessModeAdmin))
}

func TestRepository_DeleteCollaboration(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 15})
	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 22})

	assert.NoError(t, repo.LoadOwner(t.Context()))
	assert.NoError(t, DeleteCollaboration(t.Context(), repo, user))
	unittest.AssertNotExistsBean(t, &repo_model.Collaboration{RepoID: repo.ID, UserID: user.ID})

	assert.NoError(t, DeleteCollaboration(t.Context(), repo, user))
	unittest.AssertNotExistsBean(t, &repo_model.Collaboration{RepoID: repo.ID, UserID: user.ID})

	unittest.CheckConsistencyFor(t, &repo_model.Repository{ID: repo.ID})
}
