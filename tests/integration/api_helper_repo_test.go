// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

// API ports of the helpers that used to drive the removed web forms (fork, branch, editor, pulls)

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	issues_model "gitea.dev/models/issues"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	api "gitea.dev/modules/structs"
	"gitea.dev/modules/util"
	"gitea.dev/services/forms"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testRepoFork forks ownerName/repoName into forkOwnerName (the session user or one of its orgs)
func testRepoFork(t *testing.T, session *TestSession, ownerName, repoName, forkOwnerName, forkRepoName, _ string) *httptest.ResponseRecorder {
	t.Helper()
	forkOwner := unittest.AssertExistsAndLoadBean(t, &user_model.User{Name: forkOwnerName})

	req := NewRequestf(t, "GET", "/api/v1/repos/%s/%s", forkOwnerName, forkRepoName)
	session.MakeRequest(t, req, http.StatusNotFound)

	opts := &api.CreateForkOption{Name: &forkRepoName}
	if forkOwner.IsOrganization() {
		opts.Organization = &forkOwnerName
	}
	req = NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/forks", ownerName, repoName), opts)
	resp := session.MakeRequest(t, req, http.StatusAccepted)
	fork := DecodeJSON(t, resp, &api.Repository{})
	assert.Equal(t, forkOwnerName+"/"+forkRepoName, fork.FullName)

	req = NewRequestf(t, "GET", "/api/v1/repos/%s/%s", forkOwnerName, forkRepoName)
	return session.MakeRequest(t, req, http.StatusOK)
}

// testCreateBranch creates a branch from oldRefSubURL ("branch/x", "tag/x" or "commit/x"), expectedStatus is an API status
func testCreateBranch(t testing.TB, session *TestSession, user, repo, oldRefSubURL, newBranchName string, expectedStatus int) {
	t.Helper()
	_, oldRef, _ := strings.Cut(oldRefSubURL, "/")
	req := NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/branches", user, repo), &api.CreateBranchRepoOption{
		BranchName: newBranchName,
		OldRefName: oldRef,
	})
	session.MakeRequest(t, req, expectedStatus)
}

func testDeleteBranch(t *testing.T, session *TestSession, ownerName, repoName, branchName string) {
	t.Helper()
	req := NewRequestf(t, "DELETE", "/api/v1/repos/%s/%s/branches/%s", ownerName, repoName, url.PathEscape(branchName))
	session.MakeRequest(t, req, http.StatusNoContent)
}

func testDeleteRepository(t *testing.T, session *TestSession, ownerName, repoName string) {
	t.Helper()
	req := NewRequestf(t, "DELETE", "/api/v1/repos/%s/%s", ownerName, repoName)
	session.MakeRequest(t, req, http.StatusNoContent)
}

// testAPIChangeFile writes content to filePath on branch (or on newBranch created from branch) and checks the result
func testAPIChangeFile(t *testing.T, session *TestSession, user, repo, branch, newBranch, filePath, content string) {
	t.Helper()
	contentsURL := fmt.Sprintf("/api/v1/repos/%s/%s/contents/%s", user, repo, util.PathEscapeSegments(filePath))
	req := NewRequest(t, "GET", contentsURL+"?ref="+url.QueryEscape(branch))
	resp := session.MakeRequest(t, req, NoExpectedStatus)
	var sha string
	if resp.Code == http.StatusOK {
		sha = DecodeJSON(t, resp, &api.ContentsResponse{}).SHA
	}
	req = NewRequestWithJSON(t, "PUT", contentsURL, &api.UpdateFileOptions{
		FileOptions: api.FileOptions{
			BranchName:    branch,
			NewBranchName: util.Iif(newBranch == branch, "", newBranch),
		},
		SHA:           sha,
		ContentBase64: base64.StdEncoding.EncodeToString([]byte(content)),
	})
	resp = session.MakeRequest(t, req, NoExpectedStatus)
	require.Contains(t, []int{http.StatusOK, http.StatusCreated}, resp.Code, "change file failed: %s", resp.Body.String())

	targetBranch := util.IfZero(newBranch, branch)
	req = NewRequestf(t, "GET", "/api/v1/repos/%s/%s/raw/%s?ref=%s", user, repo, util.PathEscapeSegments(filePath), url.QueryEscape(targetBranch))
	resp = session.MakeRequest(t, req, http.StatusOK)
	assert.Equal(t, content, resp.Body.String())
}

func testEditFile(t *testing.T, session *TestSession, user, repo, branch, filePath, newContent string) {
	t.Helper()
	testAPIChangeFile(t, session, user, repo, branch, "", filePath, newContent)
}

func testEditFileToNewBranch(t *testing.T, session *TestSession, user, repo, branch, targetBranch, filePath, newContent string) {
	t.Helper()
	testAPIChangeFile(t, session, user, repo, branch, targetBranch, filePath, newContent)
}

func testCreateFile(t *testing.T, session *TestSession, user, repo, baseBranchName, newBranchName, filePath, content string) {
	t.Helper()
	testAPIChangeFile(t, session, user, repo, baseBranchName, newBranchName, filePath, content)
}

// testPullURL returns the "/{owner}/{repo}/pulls/{index}" path of a pull created through the API
func testPullURL(t *testing.T, resp *httptest.ResponseRecorder) string {
	t.Helper()
	pr := DecodeJSON(t, resp, &api.PullRequest{})
	u, err := url.Parse(pr.HTMLURL)
	require.NoError(t, err)
	return u.Path
}

// testPullCreate creates a pull from user/repo:sourceBranch into its parent repository, or into user/repo itself when toSelf is set
func testPullCreate(t *testing.T, session *TestSession, user, repo string, toSelf bool, targetBranch, sourceBranch, title string) *httptest.ResponseRecorder {
	t.Helper()
	headRepo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{OwnerName: user, Name: repo})
	baseRepo := headRepo
	if !toSelf && headRepo.IsFork {
		baseRepo = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: headRepo.ForkID})
	}
	return testPullCreateDirectly(t, session, createPullRequestOptions{
		BaseRepoOwner: baseRepo.OwnerName,
		BaseRepoName:  baseRepo.Name,
		BaseBranch:    targetBranch,
		HeadRepoOwner: util.Iif(baseRepo.ID == headRepo.ID, "", user),
		HeadRepoName:  util.Iif(baseRepo.ID == headRepo.ID, "", repo),
		HeadBranch:    sourceBranch,
		Title:         title,
	})
}

type createPullRequestOptions struct {
	BaseRepoOwner string
	BaseRepoName  string
	BaseBranch    string
	HeadRepoOwner string
	HeadRepoName  string
	HeadBranch    string
	Title         string
	ReviewerIDs   string // comma-separated list of user IDs
}

func (opts createPullRequestOptions) IsValid() bool {
	return opts.BaseRepoOwner != "" && opts.BaseRepoName != "" && opts.BaseBranch != "" &&
		opts.HeadBranch != "" && opts.Title != ""
}

func (opts createPullRequestOptions) headCompare() string {
	if opts.HeadRepoOwner == "" {
		return opts.HeadBranch
	}
	if opts.HeadRepoName == "" {
		return fmt.Sprintf("%s:%s", opts.HeadRepoOwner, opts.HeadBranch)
	}
	return fmt.Sprintf("%s/%s:%s", opts.HeadRepoOwner, opts.HeadRepoName, opts.HeadBranch)
}

func testPullCreateRequest(t *testing.T, session *TestSession, opts createPullRequestOptions, expectedStatus int) *httptest.ResponseRecorder {
	t.Helper()
	if !opts.IsValid() {
		t.Fatal("Invalid pull request options")
	}
	var reviewers []string
	for idStr := range strings.SplitSeq(opts.ReviewerIDs, ",") {
		if idStr == "" {
			continue
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		require.NoError(t, err)
		reviewers = append(reviewers, unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: id}).Name)
	}
	req := NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/pulls", opts.BaseRepoOwner, opts.BaseRepoName), &api.CreatePullRequestOption{
		Head:      opts.headCompare(),
		Base:      opts.BaseBranch,
		Title:     opts.Title,
		Reviewers: reviewers,
	})
	return session.MakeRequest(t, req, expectedStatus)
}

func testPullCreateDirectly(t *testing.T, session *TestSession, opts createPullRequestOptions) *httptest.ResponseRecorder {
	t.Helper()
	return testPullCreateRequest(t, session, opts, http.StatusCreated)
}

func testPullCreateFailure(t *testing.T, session *TestSession, baseRepoOwner, baseRepoName, baseBranch, headRepoOwner, headRepoName, headBranch, title string, expectedStatus int) *httptest.ResponseRecorder {
	t.Helper()
	return testPullCreateRequest(t, session, createPullRequestOptions{
		BaseRepoOwner: baseRepoOwner,
		BaseRepoName:  baseRepoName,
		BaseBranch:    baseBranch,
		HeadRepoOwner: headRepoOwner,
		HeadRepoName:  headRepoName,
		HeadBranch:    headBranch,
		Title:         title,
	}, expectedStatus)
}

type MergeOptions struct {
	Style        repo_model.MergeStyle
	HeadCommitID string
	DeleteBranch bool
}

func testPullMerge(t *testing.T, session *TestSession, user, repo, pullNum string, mergeOptions MergeOptions) *httptest.ResponseRecorder {
	t.Helper()
	form := &forms.MergePullRequestForm{
		Do:                     string(mergeOptions.Style),
		HeadCommitID:           mergeOptions.HeadCommitID,
		DeleteBranchAfterMerge: &mergeOptions.DeleteBranch,
	}
	var resp *httptest.ResponseRecorder
	require.Eventually(t, func() bool {
		req := NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%s/merge", user, repo, pullNum), form)
		resp = session.MakeRequest(t, req, NoExpectedStatus)
		return resp.Code == http.StatusOK
	}, 5*time.Second, 50*time.Millisecond, "Timed out waiting for pull merge to succeed")

	pullNumInt, err := strconv.ParseInt(pullNum, 10, 64)
	assert.NoError(t, err)
	repository, err := repo_model.GetRepositoryByOwnerAndName(t.Context(), user, repo)
	assert.NoError(t, err)
	pull, err := issues_model.GetPullRequestByIndex(t.Context(), repository.ID, pullNumInt)
	assert.NoError(t, err)
	assert.True(t, pull.HasMerged)

	return resp
}

// testParseIssueURL splits "/{owner}/{repo}/{issues|pulls}/{index}"
func testParseIssueURL(t *testing.T, issueURL string) (owner, repo, index string) {
	t.Helper()
	elems := strings.Split(strings.TrimPrefix(issueURL, "/"), "/")
	require.Len(t, elems, 4, "invalid issue URL %q", issueURL)
	return elems[0], elems[1], elems[3]
}

// testNewIssue creates an issue and returns its "/{owner}/{repo}/issues/{index}" path
func testNewIssue(t *testing.T, session *TestSession, user, repo, title, content string) string {
	t.Helper()
	req := NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/issues", user, repo), &api.CreateIssueOption{
		Title: title,
		Body:  content,
	})
	resp := session.MakeRequest(t, req, http.StatusCreated)
	issue := DecodeJSON(t, resp, &api.Issue{})
	assert.Equal(t, title, issue.Title)
	assert.Equal(t, content, issue.Body)
	return fmt.Sprintf("/%s/%s/issues/%d", user, repo, issue.Index)
}

func testIssueDelete(t *testing.T, session *TestSession, issueURL string) {
	t.Helper()
	owner, repo, index := testParseIssueURL(t, issueURL)
	req := NewRequestf(t, "DELETE", "/api/v1/repos/%s/%s/issues/%s", owner, repo, index)
	session.MakeRequest(t, req, http.StatusNoContent)
}

func testIssueAssign(t *testing.T, session *TestSession, repoLink string, issueID, assigneeID int64) {
	t.Helper()
	issue := unittest.AssertExistsAndLoadBean(t, &issues_model.Issue{ID: issueID})
	assignee := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: assigneeID})
	req := NewRequestWithJSON(t, "PATCH", fmt.Sprintf("/api/v1/repos%s/issues/%d", repoLink, issue.Index), &api.EditIssueOption{
		Assignees: []string{assignee.Name},
	})
	session.MakeRequest(t, req, http.StatusCreated)
}

func testIssueClose(t *testing.T, session *TestSession, owner, repo, issueNumber string) *httptest.ResponseRecorder {
	t.Helper()
	req := NewRequestWithJSON(t, "PATCH", fmt.Sprintf("/api/v1/repos/%s/%s/issues/%s", owner, repo, issueNumber), &api.EditIssueOption{
		State: new(string(api.StateClosed)),
	})
	return session.MakeRequest(t, req, http.StatusCreated)
}

// testIssueAddComment adds a comment, status "close" or "reopen" also changes the issue state, it returns the comment ID
func testIssueAddComment(t *testing.T, session *TestSession, issueURL, content, status string) int64 {
	t.Helper()
	owner, repo, index := testParseIssueURL(t, issueURL)
	req := NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/issues/%s/comments", owner, repo, index), &api.CreateIssueCommentOption{
		Body: content,
	})
	resp := session.MakeRequest(t, req, http.StatusCreated)
	comment := DecodeJSON(t, resp, &api.Comment{})
	assert.Equal(t, content, comment.Body)

	if status != "" {
		state := util.Iif(status == "close", api.StateClosed, api.StateOpen)
		req = NewRequestWithJSON(t, "PATCH", fmt.Sprintf("/api/v1/repos/%s/%s/issues/%s", owner, repo, index), &api.EditIssueOption{
			State: new(string(state)),
		})
		session.MakeRequest(t, req, http.StatusCreated)
	}
	return comment.ID
}

// testCreatePullToDefaultBranch creates a pull into the default branch of baseRepo and returns its index
func testCreatePullToDefaultBranch(t *testing.T, session *TestSession, baseRepo, headRepo *repo_model.Repository, headBranch, title string) string {
	t.Helper()
	opts := createPullRequestOptions{
		BaseRepoOwner: baseRepo.OwnerName,
		BaseRepoName:  baseRepo.Name,
		BaseBranch:    baseRepo.DefaultBranch,
		HeadBranch:    headBranch,
		Title:         title,
	}
	if baseRepo.ID != headRepo.ID {
		opts.HeadRepoOwner, opts.HeadRepoName = headRepo.OwnerName, headRepo.Name
	}
	resp := testPullCreateDirectly(t, session, opts)
	pr := DecodeJSON(t, resp, &api.PullRequest{})
	return strconv.FormatInt(pr.Index, 10)
}
