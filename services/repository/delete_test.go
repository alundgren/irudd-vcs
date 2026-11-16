// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repository_test

import (
	"testing"

	"gitea.dev/models/db"
	git_model "gitea.dev/models/git"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	repo_service "gitea.dev/services/repository"

	"github.com/stretchr/testify/assert"
)

func TestDeleteOwnerRepositoriesDirectly(t *testing.T) {
	unittest.PrepareTestEnv(t)

	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})

	assert.NoError(t, repo_service.DeleteOwnerRepositoriesDirectly(t.Context(), user))
}

func TestDeleteRepositoryDirectlyPurgesRepoScopedRows(t *testing.T) {
	unittest.PrepareTestEnv(t)

	// One row per table that repository deletion used to leave behind (#38494).
	assert.NoError(t, db.Insert(t.Context(),
		&git_model.RenamedBranch{RepoID: 1, From: "old-name", To: "new-name"},
		&git_model.CommitStatusSummary{RepoID: 1, SHA: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", State: "success"},
	))
	unittest.AssertExistsAndLoadBean(t, &git_model.CommitStatusIndex{RepoID: 1})

	assert.NoError(t, repo_service.DeleteRepositoryDirectly(t.Context(), 1))

	unittest.AssertNotExistsBean(t, &git_model.RenamedBranch{RepoID: 1})
	unittest.AssertNotExistsBean(t, &git_model.CommitStatusSummary{RepoID: 1})
	unittest.AssertNotExistsBean(t, &git_model.CommitStatusIndex{RepoID: 1})
}
