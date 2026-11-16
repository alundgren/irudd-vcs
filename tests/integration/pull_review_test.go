// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	repo_model "gitea.dev/models/repo"
	api "gitea.dev/modules/structs"

	"github.com/stretchr/testify/assert"
)

func TestPullView_GivenApproveOrRejectReviewOnClosedPR(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, giteaURL *url.URL) {
		user1Session := loginUser(t, "user1")
		user2Session := loginUser(t, "user2")

		// Have user1 create a fork of repo1.
		testRepoFork(t, user1Session, "user2", "repo1", "user1", "repo1", "")

		t.Run("Submit approve/reject review on merged PR", func(t *testing.T) {
			// Create a merged PR (made by user1) in the upstream repo1.
			testEditFile(t, user1Session, "user1", "repo1", "master", "README.md", "Hello, World (Edited)\n")
			resp := testPullCreate(t, user1Session, "user1", "repo1", false, "master", "master", "This is a pull title")
			elem := strings.Split(testPullURL(t, resp), "/")
			assert.Equal(t, "pulls", elem[3])
			testPullMerge(t, user1Session, elem[1], elem[2], elem[4], MergeOptions{
				Style:        repo_model.MergeStyleMerge,
				DeleteBranch: false,
			})

			// Submit an approve review on the PR.
			testSubmitReview(t, user2Session, "user2", "repo1", elem[4], "", "approve", http.StatusUnprocessableEntity)

			// Submit a reject review on the PR.
			testSubmitReview(t, user2Session, "user2", "repo1", elem[4], "", "reject", http.StatusUnprocessableEntity)
		})

		t.Run("Submit approve/reject review on closed PR", func(t *testing.T) {
			// Created a closed PR (made by user1) in the upstream repo1.
			testEditFileToNewBranch(t, user1Session, "user1", "repo1", "master", "a-test-branch", "README.md", "Hello, World (Edited...again)\n")
			resp := testPullCreate(t, user1Session, "user1", "repo1", false, "master", "a-test-branch", "This is a pull title")
			elem := strings.Split(testPullURL(t, resp), "/")
			assert.Equal(t, "pulls", elem[3])
			testIssueClose(t, user1Session, elem[1], elem[2], elem[4])

			// Submit an approve review on the PR.
			testSubmitReview(t, user2Session, "user2", "repo1", elem[4], "", "approve", http.StatusUnprocessableEntity)

			// Submit a reject review on the PR.
			testSubmitReview(t, user2Session, "user2", "repo1", elem[4], "", "reject", http.StatusUnprocessableEntity)
		})
	})
}

func testSubmitReview(t *testing.T, session *TestSession, owner, repo, pullNumber, commitID, reviewType string, expectedSubmitStatus int) *httptest.ResponseRecorder {
	event := map[string]api.ReviewStateType{
		"approve": api.ReviewStateApproved,
		"reject":  api.ReviewStateRequestChanges,
		"comment": api.ReviewStateComment,
	}[reviewType]
	req := NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%s/reviews", owner, repo, pullNumber), &api.CreatePullReviewOptions{
		CommitID: commitID,
		Body:     "test",
		Event:    event,
	})
	return session.MakeRequest(t, req, expectedSubmitStatus)
}
