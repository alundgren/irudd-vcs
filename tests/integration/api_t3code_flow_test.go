// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	auth_model "gitea.dev/models/auth"
	"gitea.dev/modules/commitstatus"
	"gitea.dev/modules/json"
	api "gitea.dev/modules/structs"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAPIT3codeFlow walks the calls t3code's Forgejo/Gitea provider makes against a repository,
// using token auth only, as one pull request lifecycle.
func TestAPIT3codeFlow(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, u *url.URL) {
		const (
			owner      = "user2"
			reviewer   = "user4"
			repoName   = "t3code-flow"
			baseBranch = "main"
			headBranch = "t3/feature"
			statusName = "t3/ci"
		)
		token := getUserToken(t, owner, auth_model.AccessTokenScopeWriteRepository, auth_model.AccessTokenScopeWriteIssue, auth_model.AccessTokenScopeWriteUser)
		reviewerToken := getUserToken(t, reviewer, auth_model.AccessTokenScopeWriteRepository, auth_model.AccessTokenScopeWriteIssue)
		repoAPI := fmt.Sprintf("/api/v1/repos/%s/%s", owner, repoName)

		call := func(t *testing.T, tok, method, path string, body any, status int) *httptest.ResponseRecorder {
			t.Helper()
			req := NewRequest(t, method, path)
			if body != nil {
				req = NewRequestWithJSON(t, method, path, body)
			}
			return MakeRequest(t, req.AddTokenAuth(tok), status)
		}
		getPull := func(t *testing.T, index int64) *api.PullRequest {
			t.Helper()
			return DecodeJSON(t, call(t, token, "GET", fmt.Sprintf("%s/pulls/%d", repoAPI, index), nil, http.StatusOK), &api.PullRequest{})
		}

		// viewer and user search
		viewer := DecodeJSON(t, call(t, token, "GET", "/api/v1/user", nil, http.StatusOK), &api.User{})
		assert.Equal(t, owner, viewer.UserName)
		var search struct {
			OK   bool        `json:"ok"`
			Data []*api.User `json:"data"`
		}
		DecodeJSON(t, call(t, token, "GET", "/api/v1/users/search?q="+reviewer+"&limit=50", nil, http.StatusOK), &search)
		assert.True(t, search.OK)
		require.NotEmpty(t, search.Data)
		assert.Equal(t, reviewer, search.Data[0].UserName)

		// create the repository; the reviewer needs no grant, every signed-in user owns every repository
		repo := DecodeJSON(t, call(t, token, "POST", "/api/v1/user/repos", &api.CreateRepoOption{
			Name:          repoName,
			AutoInit:      true,
			Readme:        "Default",
			DefaultBranch: baseBranch,
		}, http.StatusCreated), &api.Repository{})
		require.Equal(t, baseBranch, repo.DefaultBranch)
		assert.NotEmpty(t, repo.CloneURL)

		// push a branch over HTTP git, authenticating with the token
		gitURL := *u
		gitURL.Path = fmt.Sprintf("/%s/%s.git", owner, repoName)
		gitURL.User = url.UserPassword(owner, token)
		dstPath := t.TempDir()
		doGitClone(dstPath, &gitURL)(t)
		doGitCreateBranch(dstPath, headBranch)(t)
		doGitCheckoutWriteFileCommit(localGitAddCommitOptions{
			LocalRepoPath:   dstPath,
			CheckoutBranch:  headBranch,
			TreeFilePath:    "feature.txt",
			TreeFileContent: "line one\nline two\n",
		})(t)
		doGitPushTestRepository(dstPath, "origin", headBranch)(t)

		// require a status check on the base branch; t3code reads the policy from the branch endpoint
		call(t, token, "POST", repoAPI+"/branch_protections", &api.CreateBranchProtectionOption{
			RuleName:            baseBranch,
			EnablePush:          true,
			EnableStatusCheck:   true,
			StatusCheckContexts: []string{statusName},
		}, http.StatusCreated)
		resp := call(t, token, "GET", repoAPI+"/branches/"+baseBranch, nil, http.StatusOK)
		var rawBranch map[string]any
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &rawBranch))
		for _, key := range []string{"protected", "enable_status_check", "status_check_contexts", "required_approvals", "user_can_push", "user_can_merge", "effective_branch_protection_name"} {
			assert.Contains(t, rawBranch, key)
		}
		branch := DecodeJSON(t, resp, &api.Branch{})
		assert.True(t, branch.Protected)
		assert.True(t, branch.EnableStatusCheck)
		assert.Equal(t, []string{statusName}, branch.StatusCheckContexts)
		assert.Equal(t, baseBranch, branch.EffectiveBranchProtectionName)

		// file contents and the head commit
		head := DecodeJSON(t, call(t, token, "GET", repoAPI+"/branches/"+headBranch, nil, http.StatusOK), &api.Branch{})
		headSHA := head.Commit.ID
		require.NotEmpty(t, headSHA)
		contents := DecodeJSON(t, call(t, token, "GET", repoAPI+"/contents/feature.txt?ref="+url.QueryEscape(headBranch), nil, http.StatusOK), &api.ContentsResponse{})
		assert.Equal(t, "file", contents.Type)
		require.NotNil(t, contents.Content)
		content, err := base64.StdEncoding.DecodeString(*contents.Content)
		require.NoError(t, err)
		assert.Equal(t, "line one\nline two\n", string(content))
		commit := DecodeJSON(t, call(t, token, "GET", repoAPI+"/git/commits/"+headSHA+"?files=false&verification=false", nil, http.StatusOK), &api.Commit{})
		assert.Equal(t, headSHA, commit.SHA)
		assert.Len(t, commit.Parents, 1)
		assert.Contains(t, commit.RepoCommit.Message, "update feature.txt")
		resp = call(t, token, "GET", repoAPI+"/git/commits/"+headSHA+".diff", nil, http.StatusOK)
		assert.Contains(t, resp.Body.String(), "+line two")

		// open the pull request
		pr := DecodeJSON(t, call(t, token, "POST", repoAPI+"/pulls", &api.CreatePullRequestOption{
			Head:  owner + ":" + headBranch,
			Base:  baseBranch,
			Title: "Add feature",
			Body:  "First draft",
		}, http.StatusCreated), &api.PullRequest{})
		require.NotZero(t, pr.Index)
		assert.Equal(t, headBranch, pr.Head.Ref)
		assert.Equal(t, headSHA, pr.Head.Sha)
		assert.Equal(t, baseBranch, pr.Base.Ref)
		assert.Equal(t, api.StateOpen, pr.State)
		assert.False(t, pr.HasMerged)
		assert.True(t, strings.HasSuffix(pr.HTMLURL, fmt.Sprintf("/%s/%s/pulls/%d", owner, repoName, pr.Index)), pr.HTMLURL)
		pullAPI := fmt.Sprintf("%s/pulls/%d", repoAPI, pr.Index)
		issueAPI := fmt.Sprintf("%s/issues/%d", repoAPI, pr.Index)

		// list with state and base filters, and look up by head
		pulls := DecodeJSON(t, call(t, token, "GET", repoAPI+"/pulls?state=open&base_branch="+baseBranch+"&sort=recentupdate&limit=50&page=1", nil, http.StatusOK), []*api.PullRequest{})
		require.Len(t, pulls, 1)
		assert.Equal(t, pr.Index, pulls[0].Index)
		pulls = DecodeJSON(t, call(t, token, "GET", repoAPI+"/pulls?state=closed&base_branch="+baseBranch, nil, http.StatusOK), []*api.PullRequest{})
		assert.Empty(t, pulls)
		pulls = DecodeJSON(t, call(t, token, "GET", repoAPI+"/pulls?state=open&base_branch=missing", nil, http.StatusOK), []*api.PullRequest{})
		assert.Empty(t, pulls)
		// t3code looks PRs up by head branch and falls back to a full scan if the filter is ignored
		pulls = DecodeJSON(t, call(t, token, "GET", repoAPI+"/pulls?state=open&head="+url.QueryEscape(headBranch)+"&sort=recentupdate&limit=50&page=1", nil, http.StatusOK), []*api.PullRequest{})
		require.Len(t, pulls, 1)
		assert.Equal(t, headBranch, pulls[0].Head.Ref)
		pulls = DecodeJSON(t, call(t, token, "GET", repoAPI+"/pulls?state=open&head="+url.QueryEscape(owner+":"+headBranch), nil, http.StatusOK), []*api.PullRequest{})
		require.Len(t, pulls, 1)
		assert.Equal(t, pr.Index, pulls[0].Index)
		assert.Empty(t, DecodeJSON(t, call(t, token, "GET", repoAPI+"/pulls?state=open&head="+baseBranch, nil, http.StatusOK), []*api.PullRequest{}))
		assert.Empty(t, DecodeJSON(t, call(t, token, "GET", repoAPI+"/pulls?state=open&head="+url.QueryEscape("no-such-user:"+headBranch), nil, http.StatusOK), []*api.PullRequest{}))
		byHead := DecodeJSON(t, call(t, token, "GET", repoAPI+"/pulls/"+baseBranch+"/"+headBranch, nil, http.StatusOK), &api.PullRequest{})
		assert.Equal(t, pr.Index, byHead.Index)

		// get and edit title/body
		assert.Equal(t, "Add feature", getPull(t, pr.Index).Title)
		edited := DecodeJSON(t, call(t, token, "PATCH", pullAPI, &api.EditPullRequestOption{
			Title: "Add t3 feature",
			Body:  new("Ready for review"),
		}, http.StatusCreated), &api.PullRequest{})
		assert.Equal(t, "Add t3 feature", edited.Title)
		assert.Equal(t, "Ready for review", edited.Body)

		// diff and commits
		resp = call(t, token, "GET", pullAPI+".diff", nil, http.StatusOK)
		assert.Contains(t, resp.Body.String(), "diff --git a/feature.txt b/feature.txt")
		assert.Contains(t, resp.Body.String(), "+line two")
		commits := DecodeJSON(t, call(t, token, "GET", pullAPI+"/commits?files=false&verification=false", nil, http.StatusOK), []*api.Commit{})
		require.Len(t, commits, 1)
		assert.Equal(t, headSHA, commits[0].SHA)

		// commit statuses
		postStatus := func(t *testing.T, sha string, state commitstatus.CommitStatusState) {
			t.Helper()
			status := DecodeJSON(t, call(t, token, "POST", repoAPI+"/statuses/"+sha, &api.CreateStatusOption{
				State:       state,
				TargetURL:   "http://ci.example.com/run/1",
				Description: "t3 ci " + string(state),
				Context:     statusName,
			}, http.StatusCreated), &api.CommitStatus{})
			assert.Equal(t, state, status.State)
		}
		postStatus(t, headSHA, commitstatus.CommitStatusPending)
		postStatus(t, headSHA, commitstatus.CommitStatusSuccess)
		statuses := DecodeJSON(t, call(t, token, "GET", repoAPI+"/statuses/"+headSHA+"?sort=recentupdate&limit=50&page=1", nil, http.StatusOK), []*api.CommitStatus{})
		require.Len(t, statuses, 2)
		// newest first even when both were posted within the same second
		assert.Equal(t, commitstatus.CommitStatusSuccess, statuses[0].State)
		assert.Equal(t, commitstatus.CommitStatusPending, statuses[1].State)
		assert.Equal(t, statusName, statuses[0].Context)
		combined := DecodeJSON(t, call(t, token, "GET", repoAPI+"/commits/"+headSHA+"/status", nil, http.StatusOK), &api.CombinedStatus{})
		assert.Equal(t, commitstatus.CommitStatusSuccess, combined.State)
		assert.Equal(t, 1, combined.TotalCount)
		require.Len(t, combined.Statuses, 1)
		assert.Equal(t, commitstatus.CommitStatusSuccess, combined.Statuses[0].State)

		// add and remove a label on the pull request issue
		label := DecodeJSON(t, call(t, token, "POST", repoAPI+"/labels", &api.CreateLabelOption{Name: "t3", Color: "#00aabb"}, http.StatusCreated), &api.Label{})
		labels := DecodeJSON(t, call(t, token, "POST", issueAPI+"/labels", &api.IssueLabelsOption{Labels: []any{label.ID}}, http.StatusOK), []*api.Label{})
		require.Len(t, labels, 1)
		assert.Equal(t, label.ID, labels[0].ID)
		require.Len(t, getPull(t, pr.Index).Labels, 1)
		call(t, token, "DELETE", fmt.Sprintf("%s/labels/%d", issueAPI, label.ID), nil, http.StatusNoContent)
		assert.Empty(t, getPull(t, pr.Index).Labels)

		// assignees
		candidates := DecodeJSON(t, call(t, token, "GET", repoAPI+"/assignees", nil, http.StatusOK), []*api.User{})
		candidateNames := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			candidateNames = append(candidateNames, candidate.UserName)
		}
		assert.Subset(t, candidateNames, []string{owner, reviewer}) // every active user can be assigned
		edited = DecodeJSON(t, call(t, token, "PATCH", pullAPI, &api.EditPullRequestOption{Assignees: []string{owner, reviewer}}, http.StatusCreated), &api.PullRequest{})
		assert.Len(t, edited.Assignees, 2)

		// issue comments, their permalinks, and a reaction
		comment := DecodeJSON(t, call(t, token, "POST", issueAPI+"/comments", &api.CreateIssueCommentOption{Body: "Pushed the feature"}, http.StatusCreated), &api.Comment{})
		assert.Equal(t, fmt.Sprintf("%s#issuecomment-%d", pr.HTMLURL, comment.ID), comment.HTMLURL)
		comments := DecodeJSON(t, call(t, token, "GET", issueAPI+"/comments", nil, http.StatusOK), []*api.Comment{})
		require.Len(t, comments, 1)
		assert.Equal(t, comment.ID, comments[0].ID)
		assert.True(t, strings.HasSuffix(comments[0].HTMLURL, fmt.Sprintf("#issuecomment-%d", comment.ID)), comments[0].HTMLURL)
		commentReactionsAPI := fmt.Sprintf("%s/issues/comments/%d/reactions", repoAPI, comment.ID)
		call(t, reviewerToken, "POST", commentReactionsAPI, &api.EditReactionOption{Reaction: "+1"}, http.StatusCreated)
		reactions := DecodeJSON(t, call(t, token, "GET", commentReactionsAPI, nil, http.StatusOK), []*api.Reaction{})
		require.Len(t, reactions, 1)
		assert.Equal(t, "+1", reactions[0].Reaction)
		assert.Equal(t, reviewer, reactions[0].User.UserName)
		assert.Empty(t, DecodeJSON(t, call(t, token, "GET", issueAPI+"/reactions", nil, http.StatusOK), []*api.Reaction{}))

		// request a review
		requested := DecodeJSON(t, call(t, token, "POST", pullAPI+"/requested_reviewers", &api.PullReviewRequestOptions{Reviewers: []string{reviewer}}, http.StatusCreated), []*api.PullReview{})
		require.Len(t, requested, 1)
		assert.Equal(t, api.ReviewStateRequestReview, requested[0].State)
		requestedReviewers := getPull(t, pr.Index).RequestedReviewers
		require.Len(t, requestedReviewers, 1)
		assert.Equal(t, reviewer, requestedReviewers[0].UserName)

		// the reviewer submits a review with an inline comment
		review := DecodeJSON(t, call(t, reviewerToken, "POST", pullAPI+"/reviews", &api.CreatePullReviewOptions{
			Event:    api.ReviewStateComment,
			Body:     "One nit",
			CommitID: headSHA,
			Comments: []api.CreatePullReviewComment{{Path: "feature.txt", Body: "Typo here?", NewLineNum: 2}},
		}, http.StatusOK), &api.PullReview{})
		assert.Equal(t, api.ReviewStateComment, review.State)
		assert.Equal(t, 1, review.CodeCommentsCount)
		assert.Equal(t, headSHA, review.CommitID)
		reviews := DecodeJSON(t, call(t, token, "GET", pullAPI+"/reviews?limit=50&page=1", nil, http.StatusOK), []*api.PullReview{})
		var foundReview bool
		for _, r := range reviews {
			if r.ID == review.ID {
				foundReview = true
				assert.Equal(t, reviewer, r.Reviewer.UserName)
				assert.Equal(t, "One nit", r.Body)
			}
		}
		assert.True(t, foundReview)
		// t3code takes the issue comment ID from the review's html_url to react to the review
		reviewCommentID := regexp.MustCompile(`#issuecomment-([1-9][0-9]*)$`).FindStringSubmatch(review.HTMLURL)
		require.Len(t, reviewCommentID, 2, review.HTMLURL)
		assert.True(t, strings.HasPrefix(review.HTMLURL, pr.HTMLURL+"#"), review.HTMLURL)
		call(t, token, "POST", fmt.Sprintf("%s/issues/comments/%s/reactions", repoAPI, reviewCommentID[1]), &api.EditReactionOption{Reaction: "eyes"}, http.StatusCreated)
		reviewComments := DecodeJSON(t, call(t, token, "GET", fmt.Sprintf("%s/reviews/%d/comments", pullAPI, review.ID), nil, http.StatusOK), []*api.PullReviewComment{})
		require.Len(t, reviewComments, 1)
		assert.Equal(t, "feature.txt", reviewComments[0].Path)
		assert.Equal(t, "Typo here?", reviewComments[0].Body)
		assert.EqualValues(t, 2, reviewComments[0].LineNum)
		assert.Equal(t, headSHA, reviewComments[0].CommitID)

		// move the base ahead, then bring the head branch up to date
		call(t, token, "POST", repoAPI+"/contents/base.txt", &api.CreateFileOptions{
			FileOptions:   api.FileOptions{BranchName: baseBranch, Message: "base change"},
			ContentBase64: base64.StdEncoding.EncodeToString([]byte("base\n")),
		}, http.StatusCreated)
		require.Eventually(t, func() bool {
			return getPull(t, pr.Index).Mergeable
		}, 10*time.Second, 100*time.Millisecond)
		call(t, token, "POST", pullAPI+"/update?style=merge", nil, http.StatusOK)
		updatedHead := getPull(t, pr.Index).Head.Sha
		assert.NotEqual(t, headSHA, updatedHead)
		resp = call(t, token, "GET", repoAPI+"/contents/base.txt?ref="+url.QueryEscape(headBranch), nil, http.StatusOK)
		baseFile := DecodeJSON(t, resp, &api.ContentsResponse{})
		require.NotNil(t, baseFile.Content)
		assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("base\n")), *baseFile.Content)

		// the required check has not run on the new head yet
		mergeForm := map[string]any{"Do": "merge", "delete_branch_after_merge": true}
		call(t, token, "POST", pullAPI+"/merge", mergeForm, http.StatusMethodNotAllowed)
		postStatus(t, updatedHead, commitstatus.CommitStatusSuccess)
		require.Eventually(t, func() bool {
			req := NewRequestWithJSON(t, "POST", pullAPI+"/merge", mergeForm).AddTokenAuth(token)
			return MakeRequest(t, req, NoExpectedStatus).Code == http.StatusOK
		}, 10*time.Second, 100*time.Millisecond)

		merged := getPull(t, pr.Index)
		assert.True(t, merged.HasMerged)
		assert.Equal(t, api.StateClosed, merged.State)
		require.NotNil(t, merged.MergedCommitID)
		assert.NotEmpty(t, *merged.MergedCommitID)
		require.NotNil(t, merged.MergedBy)
		assert.Equal(t, owner, merged.MergedBy.UserName)
		call(t, token, "GET", pullAPI+"/merge", nil, http.StatusNoContent)
		call(t, token, "GET", repoAPI+"/branches/"+headBranch, nil, http.StatusNotFound)
		pulls = DecodeJSON(t, call(t, token, "GET", repoAPI+"/pulls?state=closed&base_branch="+baseBranch, nil, http.StatusOK), []*api.PullRequest{})
		require.Len(t, pulls, 1)
		assert.True(t, pulls[0].HasMerged)
	})
}
