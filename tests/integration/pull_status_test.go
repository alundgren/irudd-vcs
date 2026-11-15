// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	auth_model "gitea.dev/models/auth"
	git_model "gitea.dev/models/git"
	"gitea.dev/models/issues"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unittest"
	"gitea.dev/modules/commitstatus"
	"gitea.dev/modules/setting"
	api "gitea.dev/modules/structs"
	"gitea.dev/modules/test"
	"gitea.dev/services/pull"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPullCreate_CommitStatus(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, u *url.URL) {
		session := loginUser(t, "user1")
		testRepoFork(t, session, "user2", "repo1", "user1", "repo1", "")
		testEditFileToNewBranch(t, session, "user1", "repo1", "master", "status1", "README.md", "status1")

		testPullCreateDirectly(t, session, createPullRequestOptions{
			BaseRepoOwner: "user1",
			BaseRepoName:  "repo1",
			BaseBranch:    "master",
			HeadBranch:    "status1",
			Title:         "pull request from status1",
		})

		// Request pull request commits
		req := NewRequest(t, "GET", "/api/v1/repos/user1/repo1/pulls/1/commits")
		resp := session.MakeRequest(t, req, http.StatusOK)
		commits := DecodeJSON(t, resp, []*api.Commit{})
		require.Len(t, commits, 1)
		commitID := commits[0].SHA

		statusList := []commitstatus.CommitStatusState{
			commitstatus.CommitStatusPending,
			commitstatus.CommitStatusError,
			commitstatus.CommitStatusFailure,
			commitstatus.CommitStatusWarning,
			commitstatus.CommitStatusSuccess,
		}

		// the combined state reports error and warning as failure
		combinedStates := map[commitstatus.CommitStatusState]commitstatus.CommitStatusState{
			commitstatus.CommitStatusPending: commitstatus.CommitStatusPending,
			commitstatus.CommitStatusError:   commitstatus.CommitStatusFailure,
			commitstatus.CommitStatusFailure: commitstatus.CommitStatusFailure,
			commitstatus.CommitStatusWarning: commitstatus.CommitStatusFailure,
			commitstatus.CommitStatusSuccess: commitstatus.CommitStatusSuccess,
		}

		testCtx := NewAPITestContext(t, "user1", "repo1", auth_model.AccessTokenScopeWriteRepository)

		// Update commit status, and check if the combined status is updated as well
		for _, status := range statusList {
			// Call API to add status for commit
			t.Run("CreateStatus", doAPICreateCommitStatusTest(testCtx, commitID, status, "testci"))

			req = NewRequestf(t, "GET", "/api/v1/repos/user1/repo1/commits/%s/status", commitID)
			resp = session.MakeRequest(t, req, http.StatusOK)
			combined := DecodeJSON(t, resp, &api.CombinedStatus{})
			assert.Equal(t, combinedStates[status], combined.State)
			if assert.Len(t, combined.Statuses, 1) {
				assert.Equal(t, status, combined.Statuses[0].State)
			}
		}

		repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{OwnerName: "user1", Name: "repo1"})
		css := unittest.AssertExistsAndLoadBean(t, &git_model.CommitStatusSummary{RepoID: repo1.ID, SHA: commitID})
		assert.Equal(t, commitstatus.CommitStatusSuccess, css.State)
	})
}

func doAPICreateCommitStatusTest(ctx APITestContext, ref string, state commitstatus.CommitStatusState, statusContext string) func(*testing.T) {
	return func(t *testing.T) {
		link := fmt.Sprintf("/api/v1/repos/%s/%s/statuses/%s", ctx.Username, ctx.Reponame, url.PathEscape(ref))
		req := NewRequestWithJSON(t, http.MethodPost, link, api.CreateStatusOption{
			State:     state,
			TargetURL: "http://test.ci/",
			Context:   statusContext,
		}).AddTokenAuth(ctx.Token)
		ctx.Session.MakeRequest(t, req, ctx.ExpectedCode)
	}
}

func testWaitPullStatus(t *testing.T, repoID, index int64, status issues.PullRequestStatus) *issues.PullRequest {
	var pr *issues.PullRequest
	require.Eventually(t, func() bool {
		pr = unittest.AssertExistsAndLoadBean(t, &issues.PullRequest{BaseRepoID: repoID, Index: index})
		return pr.Status == status
	}, 5*time.Second, 50*time.Millisecond)
	return pr
}

func TestPullCreate_EmptyChangesWithDifferentCommits(t *testing.T) {
	// Merge must continue if commits SHA are different, even if content is same
	// Reason: gitflow and merging master back into develop, where is high possibility, there are no changes
	// but just commit saying "Merge branch". And this meta commit can be also tagged,
	// so we need to have this meta commit also in develop branch.
	onGiteaRun(t, func(t *testing.T, u *url.URL) {
		session := loginUser(t, "user1")
		testRepoFork(t, session, "user2", "repo1", "user1", "repo1", "")
		testEditFileToNewBranch(t, session, "user1", "repo1", "master", "status1", "README.md", "status1")
		testEditFile(t, session, "user1", "repo1", "status1", "README.md", "# repo1\n\nDescription for repo1")

		testPullCreateDirectly(t, session, createPullRequestOptions{
			BaseRepoOwner: "user1",
			BaseRepoName:  "repo1",
			BaseBranch:    "master",
			HeadBranch:    "status1",
			Title:         "pull request from status1",
		})

		// the changes are already on the target branch, it will be an empty commit, but it can be merged
		repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{OwnerName: "user1", Name: "repo1"})
		testWaitPullStatus(t, repo1.ID, 1, issues.PullRequestStatusEmpty)

		req := NewRequest(t, "GET", "/api/v1/repos/user1/repo1/pulls/1")
		resp := session.MakeRequest(t, req, http.StatusOK)
		assert.True(t, DecodeJSON(t, resp, &api.PullRequest{}).Mergeable)

		req = NewRequest(t, "GET", "/api/v1/repos/user1/repo1/pulls/1/commits")
		resp = session.MakeRequest(t, req, http.StatusOK)
		assert.Len(t, DecodeJSON(t, resp, []*api.Commit{}), 2)
	})
}

func TestPullCreate_EmptyChangesWithSameCommits(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, u *url.URL) {
		session := loginUser(t, "user1")

		testCreateBranch(t, session, "user2", "repo1", "branch/master", "empty-pr-branch", http.StatusCreated)
		resp := testPullCreateDirectly(t, session, createPullRequestOptions{
			BaseRepoOwner: "user2",
			BaseRepoName:  "repo1",
			BaseBranch:    "master",
			HeadBranch:    "empty-pr-branch",
			Title:         "empty pr test",
		})
		pr := DecodeJSON(t, resp, &api.PullRequest{})

		// the branch is already included in the target branch, there is nothing to merge
		testWaitPullStatus(t, 1, pr.Index, issues.PullRequestStatusAncestor)

		req := NewRequestf(t, "GET", "/api/v1/repos/user2/repo1/pulls/%d/files", pr.Index)
		resp = session.MakeRequest(t, req, http.StatusOK)
		assert.Empty(t, DecodeJSON(t, resp, []*api.ChangedFile{}))

		req = NewRequestf(t, "GET", "/api/v1/repos/user2/repo1/pulls/%d/commits", pr.Index)
		resp = session.MakeRequest(t, req, http.StatusOK)
		assert.Empty(t, DecodeJSON(t, resp, []*api.Commit{}))
	})
}

func TestPullStatusDelayCheck(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, u *url.URL) {
		defer test.MockVariableValue(&setting.Repository.PullRequest.DelayCheckForInactiveDays, 1)()
		defer test.MockVariableValue(&pull.AddPullRequestToCheckQueue)()

		session := loginUser(t, "user2")

		run := func(t *testing.T, fn func(*testing.T)) (issue3 *issues.Issue, checkedPrID int64) {
			pull.AddPullRequestToCheckQueue = func(prID int64) {
				checkedPrID = prID
			}
			fn(t)
			issue3 = unittest.AssertExistsAndLoadBean(t, &issues.Issue{RepoID: 1, Index: 3})
			_ = issue3.LoadPullRequest(t.Context())
			return issue3, checkedPrID
		}

		// PR issue3 is merageable at the beginning
		issue3, checkedPrID := run(t, func(t *testing.T) {})
		assert.Equal(t, issues.PullRequestStatusMergeable, issue3.PullRequest.Status)
		assert.Zero(t, checkedPrID)

		// when base branch changes, PR status should be updated, but it is inactive for long time, so no real check
		issue3, checkedPrID = run(t, func(t *testing.T) {
			testEditFile(t, session, "user2", "repo1", "master", "README.md", "new content 1")
		})
		assert.Equal(t, issues.PullRequestStatusChecking, issue3.PullRequest.Status)
		assert.Zero(t, checkedPrID)

		// view a PR with status=checking via the API, it starts the real check
		issue3, checkedPrID = run(t, func(t *testing.T) {
			req := NewRequest(t, "GET", "/api/v1/repos/user2/repo1/pulls/3")
			session.MakeRequest(t, req, http.StatusOK)
		})
		assert.Equal(t, issues.PullRequestStatusChecking, issue3.PullRequest.Status)
		assert.Equal(t, issue3.PullRequest.ID, checkedPrID)

		// when base branch changes, still so no real check
		issue3, checkedPrID = run(t, func(t *testing.T) {
			testEditFile(t, session, "user2", "repo1", "master", "README.md", "new content 2")
		})
		assert.Equal(t, issues.PullRequestStatusChecking, issue3.PullRequest.Status)
		assert.Zero(t, checkedPrID)

		// then allow to check PRs without delay, when base branch changes, the PRs will be checked
		setting.Repository.PullRequest.DelayCheckForInactiveDays = -1
		issue3, checkedPrID = run(t, func(t *testing.T) {
			testEditFile(t, session, "user2", "repo1", "master", "README.md", "new content 3")
		})
		assert.Equal(t, issues.PullRequestStatusChecking, issue3.PullRequest.Status)
		assert.Equal(t, issue3.PullRequest.ID, checkedPrID)
	})
}
