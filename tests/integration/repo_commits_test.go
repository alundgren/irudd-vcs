// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"sync"
	"testing"

	auth_model "gitea.dev/models/auth"
	"gitea.dev/models/db"
	git_model "gitea.dev/models/git"
	"gitea.dev/modules/commitstatus"
	"gitea.dev/modules/json"
	"gitea.dev/modules/setting"
	api "gitea.dev/modules/structs"
	"gitea.dev/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testAPIBranchCommitID returns the head commit of a branch via the API
func testAPIBranchCommitID(t *testing.T, session *TestSession, owner, repo, branch string) string {
	req := NewRequestf(t, "GET", "/api/v1/repos/%s/%s/branches/%s", owner, repo, branch)
	resp := session.MakeRequest(t, req, http.StatusOK)
	apiBranch := DecodeJSON(t, resp, &api.Branch{})
	require.NotNil(t, apiBranch.Commit)
	require.NotEmpty(t, apiBranch.Commit.ID)
	return apiBranch.Commit.ID
}

func TestRepoCommitsWithStatus(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	session := loginUser(t, "user2")
	ctx := NewAPITestContext(t, "user2", "repo1", auth_model.AccessTokenScopeWriteRepository)

	requestCommitStatuses := func(t *testing.T, linkList, linkCombined string) (statuses []*api.CommitStatus, status api.CombinedStatus) {
		assert.NoError(t, json.Unmarshal(session.MakeRequest(t, NewRequest(t, "GET", linkList), http.StatusOK).Body.Bytes(), &statuses))
		assert.NoError(t, json.Unmarshal(session.MakeRequest(t, NewRequest(t, "GET", linkCombined), http.StatusOK).Body.Bytes(), &status))
		return statuses, status
	}

	testRefMaster := func(t *testing.T, state commitstatus.CommitStatusState) {
		_ = db.TruncateBeans(t.Context(), &git_model.CommitStatus{})

		commitID := testAPIBranchCommitID(t, session, "user2", "repo1", "master")

		// Call API to add status for commit
		doAPICreateCommitStatusTest(ctx, commitID, state, "testci")(t)

		testRepoCommitsWithStatus := func(t *testing.T, linkList, linkCombined string, state commitstatus.CommitStatusState) {
			statuses, status := requestCommitStatuses(t, linkList, linkCombined)
			require.Len(t, statuses, 1)
			require.NotNil(t, status)

			assert.Equal(t, state, statuses[0].State)
			assert.Equal(t, setting.AppURL+"api/v1/repos/user2/repo1/statuses/"+commitID, statuses[0].URL)
			assert.Equal(t, "http://test.ci/", statuses[0].TargetURL)
			assert.Empty(t, statuses[0].Description)
			assert.Equal(t, "testci", statuses[0].Context)

			assert.Len(t, status.Statuses, 1)
			assert.Equal(t, statuses[0], status.Statuses[0])
			assert.Equal(t, commitID, status.SHA)
			combinedState := state
			if state == commitstatus.CommitStatusError || state == commitstatus.CommitStatusWarning {
				combinedState = commitstatus.CommitStatusFailure // the combined state reports these as failure
			}
			assert.Equal(t, combinedState, status.State)
		}
		// By SHA
		testRepoCommitsWithStatus(t, "/api/v1/repos/user2/repo1/commits/"+commitID+"/statuses", "/api/v1/repos/user2/repo1/commits/"+commitID+"/status", state)
		// By short SHA
		testRepoCommitsWithStatus(t, "/api/v1/repos/user2/repo1/commits/"+commitID[:7]+"/statuses", "/api/v1/repos/user2/repo1/commits/"+commitID[:7]+"/status", state)
		// By Ref
		testRepoCommitsWithStatus(t, "/api/v1/repos/user2/repo1/commits/master/statuses", "/api/v1/repos/user2/repo1/commits/master/status", state)
		// Tag "v1.1" points to master
		testRepoCommitsWithStatus(t, "/api/v1/repos/user2/repo1/commits/v1.1/statuses", "/api/v1/repos/user2/repo1/commits/v1.1/status", state)
	}

	t.Run("pending", func(t *testing.T) { testRefMaster(t, "pending") })
	t.Run("success", func(t *testing.T) { testRefMaster(t, "success") })
	t.Run("error", func(t *testing.T) { testRefMaster(t, "error") })
	t.Run("failure", func(t *testing.T) { testRefMaster(t, "failure") })
	t.Run("warning", func(t *testing.T) { testRefMaster(t, "warning") })
	t.Run("BranchWithSlash", func(t *testing.T) {
		_ = db.TruncateBeans(t.Context(), &git_model.CommitStatus{})

		linkList, linkCombined := "/api/v1/repos/user2/repo1/commits/feature%2F1/statuses", "/api/v1/repos/user2/repo1/commits/feature/1/status"
		statuses, status := requestCommitStatuses(t, linkList, linkCombined)
		assert.Empty(t, statuses)
		assert.Empty(t, status.Statuses)
		doAPICreateCommitStatusTest(ctx, "feature/1", commitstatus.CommitStatusSuccess, "testci")(t)
		statuses, status = requestCommitStatuses(t, linkList, linkCombined)
		assert.NotEmpty(t, statuses)
		assert.NotEmpty(t, status.Statuses)
	})
}

func TestRepoCommitsStatusParallel(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	session := loginUser(t, "user2")
	commitID := testAPIBranchCommitID(t, session, "user2", "repo1", "master")

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Go(func() {
			t.Run(fmt.Sprintf("ParallelCreateStatus_%d", i), func(t *testing.T) {
				ctx := NewAPITestContext(t, "user2", "repo1", auth_model.AccessTokenScopeWriteRepository)
				doAPICreateCommitStatusTest(ctx, commitID, commitstatus.CommitStatusPending, "testci")(t)
			})
		})
	}
	wg.Wait()
}

func TestRepoCommitsStatusMultiple(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	session := loginUser(t, "user2")
	commitID := testAPIBranchCommitID(t, session, "user2", "repo1", "master")

	// Call API to add status for commit
	ctx := NewAPITestContext(t, "user2", "repo1", auth_model.AccessTokenScopeWriteRepository)
	t.Run("CreateStatus", doAPICreateCommitStatusTest(ctx, commitID, commitstatus.CommitStatusSuccess, "testci"))
	t.Run("CreateStatus", doAPICreateCommitStatusTest(ctx, commitID, commitstatus.CommitStatusSuccess, "other_context"))

	req := NewRequestf(t, "GET", "/api/v1/repos/user2/repo1/commits/%s/status", commitID)
	resp := session.MakeRequest(t, req, http.StatusOK)
	status := DecodeJSON(t, resp, &api.CombinedStatus{})
	assert.Len(t, status.Statuses, 2)
	assert.Equal(t, commitstatus.CommitStatusSuccess, status.State)
}
