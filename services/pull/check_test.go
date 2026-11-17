// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package pull

import (
	"strconv"
	"testing"
	"time"

	"gitea.dev/models/db"
	git_model "gitea.dev/models/git"
	issues_model "gitea.dev/models/issues"
	"gitea.dev/models/unittest"
	"gitea.dev/modules/graceful"
	"gitea.dev/modules/queue"
	"gitea.dev/modules/setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPullRequest_AddToTaskQueue(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	idChan := make(chan int64, 10)
	testHandler := func(items ...string) []string {
		for _, s := range items {
			id, _ := strconv.ParseInt(s, 10, 64)
			idChan <- id
		}
		return nil
	}

	cfg, err := setting.GetQueueSettings(setting.CfgProvider, "pr_patch_checker")
	assert.NoError(t, err)
	prPatchCheckerQueue, err = queue.NewWorkerPoolQueueWithContext(t.Context(), "pr_patch_checker", cfg, testHandler, true)
	assert.NoError(t, err)

	pr := unittest.AssertExistsAndLoadBean(t, &issues_model.PullRequest{ID: 2})
	StartPullRequestCheckImmediately(t.Context(), pr)

	assert.Eventually(t, func() bool {
		pr = unittest.AssertExistsAndLoadBean(t, &issues_model.PullRequest{ID: 2})
		return pr.Status == issues_model.PullRequestStatusChecking
	}, 1*time.Second, 100*time.Millisecond)

	has, err := prPatchCheckerQueue.Has(strconv.FormatInt(pr.ID, 10))
	assert.True(t, has)
	assert.NoError(t, err)

	go prPatchCheckerQueue.Run()

	select {
	case id := <-idChan:
		assert.Equal(t, pr.ID, id)
	case <-time.After(time.Second):
		assert.FailNow(t, "Timeout: nothing was added to pullRequestQueue")
	}

	has, err = prPatchCheckerQueue.Has(strconv.FormatInt(pr.ID, 10))
	assert.False(t, has)
	assert.NoError(t, err)

	pr = unittest.AssertExistsAndLoadBean(t, &issues_model.PullRequest{ID: 2})
	assert.Equal(t, issues_model.PullRequestStatusChecking, pr.Status)

	prPatchCheckerQueue.ShutdownWait(time.Second)
	prPatchCheckerQueue = nil
}

func TestCheckSigningRequirementsHeadCommits(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	pr := unittest.AssertExistsAndLoadBean(t, &issues_model.PullRequest{ID: 2})
	require.NoError(t, pr.LoadBaseRepo(ctx))
	require.NoError(t, pr.LoadHeadRepo(ctx))

	check := func() error {
		return checkSigningRequirements(ctx, pr)
	}

	// No protected branch rule on the base branch: the check must pass.
	require.NoError(t, check())

	// Protected branch without RequireSignedCommits: the check must still pass.
	require.NoError(t, git_model.UpdateProtectBranch(ctx, pr.BaseRepo, &git_model.ProtectedBranch{
		RepoID:               pr.BaseRepoID,
		RuleName:             pr.BaseBranch,
		RequireSignedCommits: false,
	}, git_model.WhitelistOptions{}))
	require.NoError(t, check())

	// With RequireSignedCommits enabled: the test fixture commits have no signatures,
	// so the check must report ErrHeadCommitsNotAllVerified.
	pb, err := git_model.GetFirstMatchProtectedBranchRule(ctx, pr.BaseRepoID, pr.BaseBranch)
	require.NoError(t, err)
	require.NotNil(t, pb)
	pb.RequireSignedCommits = true
	require.NoError(t, git_model.UpdateProtectBranch(ctx, pr.BaseRepo, pb, git_model.WhitelistOptions{}))
	require.ErrorIs(t, check(), ErrHeadCommitsNotAllVerified)
}

func TestMarkPullRequestAsMergeable(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	prPatchCheckerQueue = queue.CreateUniqueQueue(graceful.GetManager().ShutdownContext(), "pr_patch_checker", func(items ...string) []string { return nil })
	go prPatchCheckerQueue.Run()
	defer func() {
		prPatchCheckerQueue.ShutdownWait(time.Second)
		prPatchCheckerQueue = nil
	}()

	ctx := t.Context()
	_, _ = db.GetEngine(ctx).ID(2).Update(&issues_model.PullRequest{Status: issues_model.PullRequestStatusChecking})
	pr := unittest.AssertExistsAndLoadBean(t, &issues_model.PullRequest{ID: 2})
	require.False(t, pr.HasMerged)
	require.Equal(t, issues_model.PullRequestStatusChecking, pr.Status)

	markPullRequestAsMergeable(ctx, pr)
	pr = unittest.AssertExistsAndLoadBean(t, &issues_model.PullRequest{ID: 2})
	require.Equal(t, issues_model.PullRequestStatusMergeable, pr.Status)
}

func TestFinishPullRequestCheck(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	// the queue never checks it, as when its task still waits
	prPatchCheckerQueue = queue.CreateUniqueQueue(graceful.GetManager().ShutdownContext(), "pr_patch_checker", func(items ...string) []string { return items })
	defer func() { prPatchCheckerQueue = nil }()

	ctx := t.Context()
	pr := unittest.AssertExistsAndLoadBean(t, &issues_model.PullRequest{ID: 2})
	require.True(t, markPullRequestStatusAsChecking(ctx, pr))
	AddPullRequestToCheckQueue(pr.ID)

	pr, err := FinishPullRequestCheck(ctx, pr)
	require.NoError(t, err)
	assert.Equal(t, issues_model.PullRequestStatusMergeable, pr.Status)
	// the queued task saves the status when it checks again
	assert.Equal(t, issues_model.PullRequestStatusChecking, unittest.AssertExistsAndLoadBean(t, &issues_model.PullRequest{ID: 2}).Status)

	// a pull that is not checking stays as it is
	same, err := FinishPullRequestCheck(ctx, pr)
	require.NoError(t, err)
	assert.Same(t, pr, same)
}
