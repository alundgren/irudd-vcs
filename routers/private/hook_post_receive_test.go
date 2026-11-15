// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package private

import (
	"testing"

	issues_model "gitea.dev/models/issues"
	"gitea.dev/models/unittest"
	"gitea.dev/modules/private"
	repo_module "gitea.dev/modules/repository"
	"gitea.dev/services/contexttest"

	"github.com/stretchr/testify/assert"
)

func TestHandlePullRequestMerging(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	pr, err := issues_model.GetUnmergedPullRequest(t.Context(), 1, 1, "branch2", "master", issues_model.PullRequestFlowGithub)
	assert.NoError(t, err)
	assert.NoError(t, pr.LoadBaseRepo(t.Context()))

	ctx, resp := contexttest.MockPrivateContext(t, "/")
	hookPostReceiveHandlePullRequestMerging(ctx, &private.HookOptions{
		PullRequestID: pr.ID,
		UserID:        2,
	}, []*repo_module.PushUpdateOptions{
		{NewCommitID: "01234567"},
	})
	assert.Empty(t, resp.Body.String())
	pr, err = issues_model.GetPullRequestByID(t.Context(), pr.ID)
	assert.NoError(t, err)
	assert.True(t, pr.HasMerged)
	assert.Equal(t, "01234567", pr.MergedCommitID)
}
