// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package pull

import (
	"context"
	"testing"

	"gitea.dev/models/db"
	git_model "gitea.dev/models/git"
	issues_model "gitea.dev/models/issues"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unit"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsUserAllowedToUpdate(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())

	updatePRConfig := func(t *testing.T, repoID int64, update func(*repo_model.PullRequestsConfig)) {
		repoUnit := unittest.AssertExistsAndLoadBean(t, &repo_model.RepoUnit{RepoID: repoID, Type: unit.TypePullRequests})
		update(repoUnit.PullRequestsConfig())
		require.NoError(t, repo_model.UpdateRepoUnitConfig(t.Context(), repoUnit))
	}
	setRepoAllowRebaseUpdate := func(t *testing.T, repoID int64, allow bool) {
		updatePRConfig(t, repoID, func(c *repo_model.PullRequestsConfig) { c.AllowRebaseUpdate = allow })
	}
	setRepoAllowMergeUpdate := func(t *testing.T, repoID int64, allow bool) {
		updatePRConfig(t, repoID, func(c *repo_model.PullRequestsConfig) { c.AllowMergeUpdate = allow })
	}
	checkUserAllowedToUpdate := func(ctx context.Context, pull *issues_model.PullRequest, user *user_model.User) (bool, bool, repo_model.UpdateStyle, error) {
		ret, err := CheckUserAllowedToUpdate(ctx, pull, user)
		return ret.MergeAllowed, ret.RebaseAllowed, ret.DefaultUpdateStyle, err
	}

	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})

	t.Run("RespectsProtectedBranch", func(t *testing.T) {
		pr2 := unittest.AssertExistsAndLoadBean(t, &issues_model.PullRequest{ID: 2})
		protectedBranch := &git_model.ProtectedBranch{
			RepoID:       pr2.HeadRepoID,
			RuleName:     pr2.HeadBranch,
			CanPush:      false,
			CanForcePush: false,
		}
		_, err := db.GetEngine(t.Context()).Insert(protectedBranch)
		require.NoError(t, err)
		defer db.DeleteByBean(t.Context(), protectedBranch)

		pushAllowed, rebaseAllowed, defaultMergeStyle, err := checkUserAllowedToUpdate(t.Context(), pr2, user2)
		assert.NoError(t, err)
		assert.False(t, pushAllowed)
		assert.False(t, rebaseAllowed)
		assert.Equal(t, repo_model.UpdateStyleMerge, defaultMergeStyle)
	})

	t.Run("DisallowRebaseWhenConfigDisabled", func(t *testing.T) {
		pr2 := unittest.AssertExistsAndLoadBean(t, &issues_model.PullRequest{ID: 2})
		setRepoAllowRebaseUpdate(t, pr2.BaseRepoID, false)
		pushAllowed, rebaseAllowed, _, err := checkUserAllowedToUpdate(t.Context(), pr2, user2)
		assert.NoError(t, err)
		assert.True(t, pushAllowed)
		assert.False(t, rebaseAllowed)
	})

	t.Run("DisallowMergeWhenConfigDisabled", func(t *testing.T) {
		pr2 := unittest.AssertExistsAndLoadBean(t, &issues_model.PullRequest{ID: 2})
		setRepoAllowRebaseUpdate(t, pr2.BaseRepoID, true)
		setRepoAllowMergeUpdate(t, pr2.BaseRepoID, false)
		pushAllowed, rebaseAllowed, _, err := checkUserAllowedToUpdate(t.Context(), pr2, user2)
		assert.NoError(t, err)
		assert.False(t, pushAllowed)
		assert.True(t, rebaseAllowed)
		setRepoAllowMergeUpdate(t, pr2.BaseRepoID, true)
	})

	t.Run("ProtectedBranchAllowsPushWithoutRebase", func(t *testing.T) {
		pr2 := unittest.AssertExistsAndLoadBean(t, &issues_model.PullRequest{ID: 2})
		protectedBranch := &git_model.ProtectedBranch{
			RepoID:       pr2.HeadRepoID,
			RuleName:     pr2.HeadBranch,
			CanPush:      true,
			CanForcePush: false,
		}
		_, err := db.GetEngine(t.Context()).Insert(protectedBranch)
		require.NoError(t, err)
		defer db.DeleteByBean(t.Context(), protectedBranch)

		pushAllowed, rebaseAllowed, _, err := checkUserAllowedToUpdate(t.Context(), pr2, user2)
		assert.NoError(t, err)
		assert.True(t, pushAllowed)
		assert.False(t, rebaseAllowed)
	})

	pr3Poster := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 12})

	t.Run("MaintainerEditInheritsPosterPermissions", func(t *testing.T) {
		pr3 := unittest.AssertExistsAndLoadBean(t, &issues_model.PullRequest{ID: 3})
		pr3.AllowMaintainerEdit = true
		protectedBranch := &git_model.ProtectedBranch{
			RepoID:       pr3.HeadRepoID,
			RuleName:     pr3.HeadBranch,
			CanPush:      true,
			CanForcePush: true,
		}
		_, err := db.GetEngine(t.Context()).Insert(protectedBranch)
		require.NoError(t, err)
		defer db.DeleteByBean(t.Context(), protectedBranch)

		pushAllowed, rebaseAllowed, _, err := checkUserAllowedToUpdate(t.Context(), pr3, pr3Poster)
		assert.NoError(t, err)
		assert.True(t, pushAllowed)
		assert.True(t, rebaseAllowed)
	})

	t.Run("MaintainerEditInheritsPosterPermissionsRebaseDisabled", func(t *testing.T) {
		pr3 := unittest.AssertExistsAndLoadBean(t, &issues_model.PullRequest{ID: 3})
		pr3.AllowMaintainerEdit = true
		protectedBranch := &git_model.ProtectedBranch{
			RepoID:       pr3.HeadRepoID,
			RuleName:     pr3.HeadBranch,
			CanPush:      true,
			CanForcePush: true,
		}
		_, err := db.GetEngine(t.Context()).Insert(protectedBranch)
		require.NoError(t, err)
		defer db.DeleteByBean(t.Context(), protectedBranch)

		setRepoAllowRebaseUpdate(t, pr3.BaseRepoID, false)

		pushAllowed, rebaseAllowed, _, err := checkUserAllowedToUpdate(t.Context(), pr3, pr3Poster)
		assert.NoError(t, err)
		assert.True(t, pushAllowed)
		assert.False(t, rebaseAllowed)
	})
}
