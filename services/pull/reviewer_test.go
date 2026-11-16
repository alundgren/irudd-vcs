// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package pull_test

import (
	"testing"

	"gitea.dev/models/db"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	pull_service "gitea.dev/services/pull"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"xorm.io/builder"
)

func TestRepoGetReviewers(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	n, err := db.GetEngine(ctx).Where(builder.Eq{"type": user_model.UserTypeIndividual, "is_active": true}).Count(&user_model.User{})
	require.NoError(t, err)
	activeUsers := int(n)
	repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

	// every active user can review
	reviewers, err := pull_service.GetReviewers(ctx, repo1, 2, 0)
	require.NoError(t, err)
	assert.Len(t, reviewers, activeUsers)

	// but not the PR poster
	reviewers, err = pull_service.GetReviewers(ctx, repo1, 11, 2)
	require.NoError(t, err)
	assert.Len(t, reviewers, activeUsers-1)
	for _, r := range reviewers {
		assert.NotEqual(t, int64(2), r.ID)
	}
}
