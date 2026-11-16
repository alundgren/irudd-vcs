// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo_test

import (
	"testing"

	"gitea.dev/models/db"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"xorm.io/builder"
)

func TestUserRepo(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	t.Run("GetIssuePostersWithSearch", testUserRepoGetIssuePostersWithSearch)
	t.Run("Assignees", testUserRepoAssignees)
}

func testUserRepoAssignees(t *testing.T) {
	ctx := t.Context()
	activeUsers := func() int {
		n, err := db.GetEngine(ctx).Where(builder.Eq{"type": user_model.UserTypeIndividual, "is_active": true}).Count(&user_model.User{})
		require.NoError(t, err)
		return int(n)
	}

	// every active user can be assigned, whoever owns the repository
	repo2 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2})
	users, err := repo_model.GetRepoAssignees(ctx, repo2)
	require.NoError(t, err)
	assert.Len(t, users, activeUsers())

	// do not return deactivated users
	require.NoError(t, user_model.UpdateUserCols(ctx, &user_model.User{ID: 15, IsActive: false}, "is_active"))
	users, err = repo_model.GetRepoAssignees(ctx, repo2)
	require.NoError(t, err)
	assert.Len(t, users, activeUsers())
	for _, u := range users {
		assert.NotEqual(t, int64(15), u.ID)
	}
}

func testUserRepoGetIssuePostersWithSearch(t *testing.T) {
	repo2 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2})

	users, err := repo_model.GetIssuePostersWithSearch(t.Context(), repo2, false, "USER")
	require.NoError(t, err)
	require.Len(t, users, 1)
	assert.Equal(t, "user2", users[0].Name)

	users, err = repo_model.GetIssuePostersWithSearch(t.Context(), repo2, false, "TW%O")
	require.NoError(t, err)
	require.Len(t, users, 1)
	assert.Equal(t, "user2", users[0].Name)
}

func repoIDs(repos []*repo_model.Repository) []int64 {
	ids := make([]int64, len(repos))
	for i, r := range repos {
		ids[i] = r.ID
	}
	return ids
}
