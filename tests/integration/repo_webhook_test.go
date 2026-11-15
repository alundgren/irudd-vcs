// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"testing"

	auth_model "gitea.dev/models/auth"
	"gitea.dev/models/perm"
	"gitea.dev/models/repo"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/commitstatus"
	"gitea.dev/modules/git"
	"gitea.dev/modules/gitrepo"
	"gitea.dev/modules/json"
	"gitea.dev/modules/setting"
	api "gitea.dev/modules/structs"
	webhook_module "gitea.dev/modules/webhook"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testAPICreateWebhookForRepo(t *testing.T, session *TestSession, userName, repoName, url, event string, branchFilter ...string) {
	token := getTokenForLoggedInUser(t, session, auth_model.AccessTokenScopeAll)
	var branchFilterString string
	if len(branchFilter) > 0 {
		branchFilterString = branchFilter[0]
	}
	req := NewRequestWithJSON(t, "POST", "/api/v1/repos/"+userName+"/"+repoName+"/hooks", api.CreateHookOption{
		Type: "gitea",
		Config: api.CreateHookOptionConfig{
			"content_type": "json",
			"url":          url,
		},
		Events:       []string{event},
		Active:       true,
		BranchFilter: branchFilterString,
	}).AddTokenAuth(token)
	MakeRequest(t, req, http.StatusCreated)
}

func testAPICreateWebhookForOrg(t *testing.T, session *TestSession, userName, url, event string) {
	token := getTokenForLoggedInUser(t, session, auth_model.AccessTokenScopeAll)
	req := NewRequestWithJSON(t, "POST", "/api/v1/orgs/"+userName+"/hooks", api.CreateHookOption{
		Type: "gitea",
		Config: api.CreateHookOptionConfig{
			"content_type": "json",
			"url":          url,
		},
		Events: []string{event},
		Active: true,
	}).AddTokenAuth(token)
	MakeRequest(t, req, http.StatusCreated)
}

type mockWebhookProvider struct {
	server *httptest.Server
}

func newMockWebhookProvider(callback func(r *http.Request), status int) *mockWebhookProvider {
	m := &mockWebhookProvider{}
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callback(r)
		w.WriteHeader(status)
	}))
	return m
}

func (m *mockWebhookProvider) URL() string {
	if m.server == nil {
		return ""
	}
	return m.server.URL
}

// Close closes the mock webhook http server
func (m *mockWebhookProvider) Close() {
	if m.server != nil {
		m.server.Close()
		m.server = nil
	}
}

func Test_WebhookCreate(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, giteaURL *url.URL) {
		var payloads []api.CreatePayload
		var triggeredEvent string
		provider := newMockWebhookProvider(func(r *http.Request) {
			content, _ := io.ReadAll(r.Body)
			var payload api.CreatePayload
			err := json.Unmarshal(content, &payload)
			assert.NoError(t, err)
			payloads = append(payloads, payload)
			triggeredEvent = string(webhook_module.HookEventCreate)
		}, http.StatusOK)
		defer provider.Close()

		// 1. create a new webhook with special webhook for repo1
		session := loginUser(t, "user2")

		testAPICreateWebhookForRepo(t, session, "user2", "repo1", provider.URL(), "create")

		// 2. trigger the webhook
		testAPICreateBranch(t, session, "user2", "repo1", "master", "master2", http.StatusCreated)

		// 3. validate the webhook is triggered
		assert.Len(t, payloads, 1)
		assert.Equal(t, string(webhook_module.HookEventCreate), triggeredEvent)
		assert.Equal(t, "repo1", payloads[0].Repo.Name)
		assert.Equal(t, "user2/repo1", payloads[0].Repo.FullName)
		assert.Equal(t, "master2", payloads[0].Ref)
		assert.Equal(t, "branch", payloads[0].RefType)
	})
}

func Test_WebhookDelete(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, giteaURL *url.URL) {
		var payloads []api.DeletePayload
		var triggeredEvent string
		provider := newMockWebhookProvider(func(r *http.Request) {
			content, _ := io.ReadAll(r.Body)
			var payload api.DeletePayload
			err := json.Unmarshal(content, &payload)
			assert.NoError(t, err)
			payloads = append(payloads, payload)
			triggeredEvent = "delete"
		}, http.StatusOK)
		defer provider.Close()

		// 1. create a new webhook with special webhook for repo1
		session := loginUser(t, "user2")

		testAPICreateWebhookForRepo(t, session, "user2", "repo1", provider.URL(), "delete")

		// 2. trigger the webhook
		testAPICreateBranch(t, session, "user2", "repo1", "master", "master2", http.StatusCreated)
		testAPIDeleteBranch(t, "master2", http.StatusNoContent)

		// 3. validate the webhook is triggered
		assert.Equal(t, "delete", triggeredEvent)
		assert.Len(t, payloads, 1)
		assert.Equal(t, "repo1", payloads[0].Repo.Name)
		assert.Equal(t, "user2/repo1", payloads[0].Repo.FullName)
		assert.Equal(t, "master2", payloads[0].Ref)
		assert.Equal(t, "branch", payloads[0].RefType)
	})
}

func Test_WebhookFork(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, giteaURL *url.URL) {
		var payloads []api.ForkPayload
		var triggeredEvent string
		provider := newMockWebhookProvider(func(r *http.Request) {
			content, _ := io.ReadAll(r.Body)
			var payload api.ForkPayload
			err := json.Unmarshal(content, &payload)
			assert.NoError(t, err)
			payloads = append(payloads, payload)
			triggeredEvent = "fork"
		}, http.StatusOK)
		defer provider.Close()

		// 1. create a new webhook with special webhook for repo1
		session := loginUser(t, "user1")

		testAPICreateWebhookForRepo(t, session, "user2", "repo1", provider.URL(), "fork")

		// 2. trigger the webhook
		testRepoFork(t, session, "user2", "repo1", "user1", "repo1-fork", "master")

		// 3. validate the webhook is triggered
		assert.Equal(t, "fork", triggeredEvent)
		assert.Len(t, payloads, 1)
		assert.Equal(t, "repo1-fork", payloads[0].Repo.Name)
		assert.Equal(t, "user1/repo1-fork", payloads[0].Repo.FullName)
		assert.Equal(t, "repo1", payloads[0].Forkee.Name)
		assert.Equal(t, "user2/repo1", payloads[0].Forkee.FullName)
	})
}

func Test_WebhookIssueComment(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, giteaURL *url.URL) {
		var payloads []api.IssueCommentPayload
		var triggeredEvent string
		provider := newMockWebhookProvider(func(r *http.Request) {
			content, _ := io.ReadAll(r.Body)
			var payload api.IssueCommentPayload
			err := json.Unmarshal(content, &payload)
			assert.NoError(t, err)
			payloads = append(payloads, payload)
			triggeredEvent = "issue_comment"
		}, http.StatusOK)
		defer provider.Close()

		// 1. create a new webhook with special webhook for repo1
		session := loginUser(t, "user2")

		testAPICreateWebhookForRepo(t, session, "user2", "repo1", provider.URL(), "issue_comment")

		t.Run("create comment", func(t *testing.T) {
			// 2. trigger the webhook
			issueURL := testNewIssue(t, session, "user2", "repo1", "Title2", "Description2")
			testIssueAddComment(t, session, issueURL, "issue title2 comment1", "")

			// 3. validate the webhook is triggered
			assert.Equal(t, "issue_comment", triggeredEvent)
			assert.Len(t, payloads, 1)
			assert.EqualValues(t, "created", payloads[0].Action)
			assert.Equal(t, "repo1", payloads[0].Issue.Repo.Name)
			assert.Equal(t, "user2/repo1", payloads[0].Issue.Repo.FullName)
			assert.Equal(t, "Title2", payloads[0].Issue.Title)
			assert.Equal(t, "Description2", payloads[0].Issue.Body)
			assert.Equal(t, "issue title2 comment1", payloads[0].Comment.Body)
		})

		t.Run("update comment", func(t *testing.T) {
			payloads = make([]api.IssueCommentPayload, 0, 2)
			triggeredEvent = ""

			// 2. trigger the webhook
			issueURL := testNewIssue(t, session, "user2", "repo1", "Title3", "Description3")
			commentID := testIssueAddComment(t, session, issueURL, "issue title3 comment1", "")
			modifiedContent := "issue title2 comment1 - modified"
			req := NewRequestWithJSON(t, "PATCH", fmt.Sprintf("/api/v1/repos/%s/%s/issues/comments/%d", "user2", "repo1", commentID), &api.EditIssueCommentOption{
				Body: modifiedContent,
			})
			session.MakeRequest(t, req, http.StatusOK)

			// 3. validate the webhook is triggered
			assert.Equal(t, "issue_comment", triggeredEvent)
			assert.Len(t, payloads, 2)
			assert.EqualValues(t, "edited", payloads[1].Action)
			assert.Equal(t, "repo1", payloads[1].Issue.Repo.Name)
			assert.Equal(t, "user2/repo1", payloads[1].Issue.Repo.FullName)
			assert.Equal(t, "Title3", payloads[1].Issue.Title)
			assert.Equal(t, "Description3", payloads[1].Issue.Body)
			assert.Equal(t, modifiedContent, payloads[1].Comment.Body)
		})

		t.Run("Update comment with no content change", func(t *testing.T) {
			payloads = make([]api.IssueCommentPayload, 0, 2)
			triggeredEvent = ""
			commentContent := "issue title3 comment1"

			// 2. trigger the webhook
			issueURL := testNewIssue(t, session, "user2", "repo1", "Title3", "Description3")
			commentID := testIssueAddComment(t, session, issueURL, commentContent, "")

			payloads = make([]api.IssueCommentPayload, 0, 2)
			triggeredEvent = ""
			req := NewRequestWithJSON(t, "PATCH", fmt.Sprintf("/api/v1/repos/%s/%s/issues/comments/%d", "user2", "repo1", commentID), &api.EditIssueCommentOption{
				Body: commentContent,
			})
			session.MakeRequest(t, req, http.StatusOK)

			// 3. validate the webhook is not triggered because no content change
			assert.Empty(t, triggeredEvent)
			assert.Empty(t, payloads)
		})
	})
}

func Test_WebhookPush(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, giteaURL *url.URL) {
		var payloads []api.PushPayload
		var triggeredEvent string
		provider := newMockWebhookProvider(func(r *http.Request) {
			content, _ := io.ReadAll(r.Body)
			var payload api.PushPayload
			err := json.Unmarshal(content, &payload)
			assert.NoError(t, err)
			payloads = append(payloads, payload)
			triggeredEvent = "push"
		}, http.StatusOK)
		defer provider.Close()

		// 1. create a new webhook with special webhook for repo1
		session := loginUser(t, "user2")

		testAPICreateWebhookForRepo(t, session, "user2", "repo1", provider.URL(), "push")

		// 2. trigger the webhook
		testCreateFile(t, session, "user2", "repo1", "master", "", "test_webhook_push.md", "# a test file for webhook push")

		// 3. validate the webhook is triggered
		assert.Equal(t, "push", triggeredEvent)
		assert.Len(t, payloads, 1)
		assert.Equal(t, "repo1", payloads[0].Repo.Name)
		assert.Equal(t, "user2/repo1", payloads[0].Repo.FullName)
		assert.Len(t, payloads[0].Commits, 1)
		assert.Equal(t, []string{"test_webhook_push.md"}, payloads[0].Commits[0].Added)
	})
}

func Test_WebhookPushDevBranch(t *testing.T) {
	var payloads []api.PushPayload
	var triggeredEvent string
	provider := newMockWebhookProvider(func(r *http.Request) {
		content, _ := io.ReadAll(r.Body)
		var payload api.PushPayload
		err := json.Unmarshal(content, &payload)
		assert.NoError(t, err)
		payloads = append(payloads, payload)
		triggeredEvent = "push"
	}, http.StatusOK)
	defer provider.Close()

	onGiteaRun(t, func(t *testing.T, giteaURL *url.URL) {
		// 1. create a new webhook with special webhook for repo1
		session := loginUser(t, "user2")

		// only for dev branch
		testAPICreateWebhookForRepo(t, session, "user2", "repo1", provider.URL(), "push", "develop")

		// 2. this should not trigger the webhook
		testCreateFile(t, session, "user2", "repo1", "master", "", "test_webhook_push.md", "# a test file for webhook push")
		assert.Empty(t, triggeredEvent)
		assert.Empty(t, payloads)

		repo1 := unittest.AssertExistsAndLoadBean(t, &repo.Repository{ID: 1})
		gitRepo, err := gitrepo.OpenRepository(t.Context(), repo1)
		assert.NoError(t, err)
		defer gitRepo.Close()

		beforeCommitID, err := gitRepo.GetBranchCommitID("develop")
		assert.NoError(t, err)

		// 3. trigger the webhook
		testCreateFile(t, session, "user2", "repo1", "develop", "", "test_webhook_push.md", "# a test file for webhook push")

		afterCommitID, err := gitRepo.GetBranchCommitID("develop")
		assert.NoError(t, err)

		// 4. validate the webhook is triggered
		assert.Equal(t, "push", triggeredEvent)
		assert.Len(t, payloads, 1)
		assert.Equal(t, "refs/heads/develop", payloads[0].Ref)
		assert.Equal(t, beforeCommitID, payloads[0].Before)
		assert.Equal(t, afterCommitID, payloads[0].After)
		assert.Equal(t, "repo1", payloads[0].Repo.Name)
		assert.Equal(t, "develop", payloads[0].Branch())
		assert.Equal(t, "user2/repo1", payloads[0].Repo.FullName)
		assert.Len(t, payloads[0].Commits, 1)
		assert.Equal(t, afterCommitID, payloads[0].Commits[0].ID)
		assert.Equal(t, setting.AppURL+"user2/repo1/compare/"+beforeCommitID+"..."+afterCommitID, payloads[0].CompareURL)
		assert.Equal(t, []string{"test_webhook_push.md"}, payloads[0].Commits[0].Added)
		assert.Empty(t, payloads[0].Commits[0].Removed)
	})
}

func Test_WebhookPushToNewBranch(t *testing.T) {
	var payloads []api.PushPayload
	var triggeredEvent string
	provider := newMockWebhookProvider(func(r *http.Request) {
		content, _ := io.ReadAll(r.Body)
		var payload api.PushPayload
		err := json.Unmarshal(content, &payload)
		assert.NoError(t, err)
		payloads = append(payloads, payload)
		triggeredEvent = "push"
	}, http.StatusOK)
	defer provider.Close()

	onGiteaRun(t, func(t *testing.T, giteaURL *url.URL) {
		// 1. create a new webhook with special webhook for repo1
		session := loginUser(t, "user2")

		// only for dev branch
		testAPICreateWebhookForRepo(t, session, "user2", "repo1", provider.URL(), "push", "new_branch")

		repo1 := unittest.AssertExistsAndLoadBean(t, &repo.Repository{ID: 1})
		gitRepo, err := gitrepo.OpenRepository(t.Context(), repo1)
		assert.NoError(t, err)
		defer gitRepo.Close()

		beforeCommitID, err := gitRepo.GetBranchCommitID("master")
		assert.NoError(t, err)

		// 2. trigger the webhook
		testCreateFile(t, session, "user2", "repo1", "master", "new_branch", "test_webhook_push.md", "# a new push from new branch")

		afterCommitID, err := gitRepo.GetBranchCommitID("new_branch")
		assert.NoError(t, err)
		emptyCommitID := git.Sha1ObjectFormat.EmptyObjectID().String()

		// 4. validate the webhook is triggered
		assert.Equal(t, "push", triggeredEvent)
		assert.Len(t, payloads, 1)
		assert.Equal(t, "refs/heads/new_branch", payloads[0].Ref)
		assert.Equal(t, emptyCommitID, payloads[0].Before)
		assert.Equal(t, afterCommitID, payloads[0].After)
		assert.Equal(t, "repo1", payloads[0].Repo.Name)
		assert.Equal(t, "new_branch", payloads[0].Branch())
		assert.Equal(t, "user2/repo1", payloads[0].Repo.FullName)
		assert.Len(t, payloads[0].Commits, 1)
		assert.Equal(t, afterCommitID, payloads[0].Commits[0].ID)
		assert.Equal(t, setting.AppURL+"user2/repo1/compare/"+beforeCommitID+"..."+afterCommitID, payloads[0].CompareURL)
		assert.Equal(t, []string{"test_webhook_push.md"}, payloads[0].Commits[0].Added)
		assert.Empty(t, payloads[0].Commits[0].Removed)
	})
}

func Test_WebhookIssue(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, giteaURL *url.URL) {
		var payloads []api.IssuePayload
		var triggeredEvent string
		provider := newMockWebhookProvider(func(r *http.Request) {
			content, _ := io.ReadAll(r.Body)
			var payload api.IssuePayload
			err := json.Unmarshal(content, &payload)
			assert.NoError(t, err)
			payloads = append(payloads, payload)
			triggeredEvent = "issues"
		}, http.StatusOK)
		defer provider.Close()

		// 1. create a new webhook with special webhook for repo1
		session := loginUser(t, "user2")

		testAPICreateWebhookForRepo(t, session, "user2", "repo1", provider.URL(), "issues")

		// 2. trigger the webhook
		testNewIssue(t, session, "user2", "repo1", "Title1", "Description1")

		// 3. validate the webhook is triggered
		assert.Equal(t, "issues", triggeredEvent)
		assert.Len(t, payloads, 1)
		assert.EqualValues(t, "opened", payloads[0].Action)
		assert.Equal(t, "repo1", payloads[0].Issue.Repo.Name)
		assert.Equal(t, "user2/repo1", payloads[0].Issue.Repo.FullName)
		assert.Equal(t, "Title1", payloads[0].Issue.Title)
		assert.Equal(t, "Description1", payloads[0].Issue.Body)
		assert.Positive(t, payloads[0].Issue.Created.Unix())
		assert.Positive(t, payloads[0].Issue.Updated.Unix())
	})
}

func Test_WebhookIssueDelete(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, giteaURL *url.URL) {
		var payloads []api.IssuePayload
		var triggeredEvent string
		provider := newMockWebhookProvider(func(r *http.Request) {
			content, _ := io.ReadAll(r.Body)
			var payload api.IssuePayload
			err := json.Unmarshal(content, &payload)
			assert.NoError(t, err)
			payloads = append(payloads, payload)
			triggeredEvent = "issue"
		}, http.StatusOK)
		defer provider.Close()

		// 1. create a new webhook with special webhook for repo1
		session := loginUser(t, "user2")
		testAPICreateWebhookForRepo(t, session, "user2", "repo1", provider.URL(), "issues")
		issueURL := testNewIssue(t, session, "user2", "repo1", "Title1", "Description1")

		// 2. trigger the webhook
		testIssueDelete(t, session, issueURL)

		// 3. validate the webhook is triggered
		assert.Equal(t, "issue", triggeredEvent)
		require.Len(t, payloads, 2)
		assert.EqualValues(t, "deleted", payloads[1].Action)
		assert.Equal(t, "repo1", payloads[1].Issue.Repo.Name)
		assert.Equal(t, "user2/repo1", payloads[1].Issue.Repo.FullName)
		assert.Equal(t, "Title1", payloads[1].Issue.Title)
		assert.Equal(t, "Description1", payloads[1].Issue.Body)
	})
}

func Test_WebhookIssueAssign(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, giteaURL *url.URL) {
		var payloads []api.PullRequestPayload
		var triggeredEvent string
		provider := newMockWebhookProvider(func(r *http.Request) {
			content, _ := io.ReadAll(r.Body)
			var payload api.PullRequestPayload
			err := json.Unmarshal(content, &payload)
			assert.NoError(t, err)
			payloads = append(payloads, payload)
			triggeredEvent = "pull_request_assign"
		}, http.StatusOK)
		defer provider.Close()

		user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
		repo1 := unittest.AssertExistsAndLoadBean(t, &repo.Repository{ID: 1})

		// 1. create a new webhook with special webhook for repo1
		session := loginUser(t, "user2")

		testAPICreateWebhookForRepo(t, session, "user2", "repo1", provider.URL(), "pull_request_assign")

		// 2. trigger the webhook, issue 2 is a pull request
		testIssueAssign(t, session, repo1.Link(), 2, user2.ID)

		// 3. validate the webhook is triggered
		assert.Equal(t, "pull_request_assign", triggeredEvent)
		assert.Len(t, payloads, 1)
		assert.EqualValues(t, "assigned", payloads[0].Action)
		assert.Equal(t, "repo1", payloads[0].PullRequest.Base.Repository.Name)
		assert.Equal(t, "user2/repo1", payloads[0].PullRequest.Base.Repository.FullName)
		assert.Equal(t, "issue2", payloads[0].PullRequest.Title)
		assert.Equal(t, "content for the second issue", payloads[0].PullRequest.Body)
		assert.Equal(t, user2.ID, payloads[0].PullRequest.Assignee.ID)
	})
}

func Test_WebhookPullRequest(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, giteaURL *url.URL) {
		var payloads []api.PullRequestPayload
		var triggeredEvent string
		provider := newMockWebhookProvider(func(r *http.Request) {
			content, _ := io.ReadAll(r.Body)
			var payload api.PullRequestPayload
			err := json.Unmarshal(content, &payload)
			assert.NoError(t, err)
			payloads = append(payloads, payload)
			triggeredEvent = "pull_request"
		}, http.StatusOK)
		defer provider.Close()

		testCtx := NewAPITestContext(t, "user2", "repo1", auth_model.AccessTokenScopeAll)
		// add user4 as collabrator so that it can be a reviewer
		doAPIAddCollaborator(testCtx, "user4", perm.AccessModeWrite)(t)

		// 1. create a new webhook with special webhook for repo1
		sessionUser2 := loginUser(t, "user2")
		sessionUser4 := loginUser(t, "user4")

		// ignore the possible review_requested event to keep the test deterministic
		testAPICreateWebhookForRepo(t, sessionUser2, "user2", "repo1", provider.URL(), "pull_request_only")

		testAPICreateBranch(t, sessionUser2, "user2", "repo1", "master", "master2", http.StatusCreated)
		// 2. trigger the webhook
		repo1 := unittest.AssertExistsAndLoadBean(t, &repo.Repository{ID: 1})
		testPullCreateDirectly(t, sessionUser4, createPullRequestOptions{
			BaseRepoOwner: repo1.OwnerName,
			BaseRepoName:  repo1.Name,
			BaseBranch:    repo1.DefaultBranch,
			HeadRepoOwner: "",
			HeadRepoName:  "",
			HeadBranch:    "master2",
			Title:         "first pull request",
			ReviewerIDs:   "2", // add user2 as reviewer
		})

		// 3. validate the webhook is triggered
		assert.Equal(t, "pull_request", triggeredEvent)
		require.Len(t, payloads, 1)
		assert.Equal(t, "repo1", payloads[0].PullRequest.Base.Repository.Name)
		assert.Equal(t, "user2/repo1", payloads[0].PullRequest.Base.Repository.FullName)
		assert.Equal(t, "repo1", payloads[0].PullRequest.Head.Repository.Name)
		assert.Equal(t, "user2/repo1", payloads[0].PullRequest.Head.Repository.FullName)
		assert.Equal(t, 0, *payloads[0].PullRequest.Additions)
		assert.Equal(t, 0, *payloads[0].PullRequest.ChangedFiles)
		assert.Equal(t, 0, *payloads[0].PullRequest.Deletions)
		assert.Len(t, payloads[0].PullRequest.RequestedReviewers, 1)
		assert.Equal(t, int64(2), payloads[0].PullRequest.RequestedReviewers[0].ID)
	})
}

func Test_WebhookPullRequestDelete(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, giteaURL *url.URL) {
		var payloads []api.PullRequestPayload
		var triggeredEvent string
		provider := newMockWebhookProvider(func(r *http.Request) {
			content, _ := io.ReadAll(r.Body)
			var payload api.PullRequestPayload
			err := json.Unmarshal(content, &payload)
			assert.NoError(t, err)
			payloads = append(payloads, payload)
			triggeredEvent = "pull_request"
		}, http.StatusOK)
		defer provider.Close()

		// 1. create a new webhook with special webhook for repo1
		session := loginUser(t, "user2")
		testAPICreateWebhookForRepo(t, session, "user2", "repo1", provider.URL(), "pull_request")

		testAPICreateBranch(t, session, "user2", "repo1", "master", "master2", http.StatusCreated)

		repo1 := unittest.AssertExistsAndLoadBean(t, &repo.Repository{ID: 1})
		issueURL := testCreatePullToDefaultBranch(t, session, repo1, repo1, "master2", "first pull request")

		// 2. trigger the webhook
		testIssueDelete(t, session, path.Join(repo1.Link(), "pulls", issueURL))

		// 3. validate the webhook is triggered
		assert.Equal(t, "pull_request", triggeredEvent)
		require.Len(t, payloads, 2)
		assert.EqualValues(t, "deleted", payloads[1].Action)
		assert.Equal(t, "repo1", payloads[1].PullRequest.Base.Repository.Name)
		assert.Equal(t, "user2/repo1", payloads[1].PullRequest.Base.Repository.FullName)
		assert.Equal(t, 0, *payloads[1].PullRequest.Additions)
		assert.Equal(t, 0, *payloads[1].PullRequest.ChangedFiles)
		assert.Equal(t, 0, *payloads[1].PullRequest.Deletions)
	})
}

func Test_WebhookPullRequestComment(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, giteaURL *url.URL) {
		var payloads []api.IssueCommentPayload
		var triggeredEvent string
		provider := newMockWebhookProvider(func(r *http.Request) {
			content, _ := io.ReadAll(r.Body)
			var payload api.IssueCommentPayload
			err := json.Unmarshal(content, &payload)
			assert.NoError(t, err)
			payloads = append(payloads, payload)
			triggeredEvent = "pull_request_comment"
		}, http.StatusOK)
		defer provider.Close()

		// 1. create a new webhook with special webhook for repo1
		session := loginUser(t, "user2")

		testAPICreateWebhookForRepo(t, session, "user2", "repo1", provider.URL(), "pull_request_comment")

		// 2. trigger the webhook
		testAPICreateBranch(t, session, "user2", "repo1", "master", "master2", http.StatusCreated)
		repo1 := unittest.AssertExistsAndLoadBean(t, &repo.Repository{ID: 1})
		prID := testCreatePullToDefaultBranch(t, session, repo1, repo1, "master2", "first pull request")

		testIssueAddComment(t, session, "/user2/repo1/pulls/"+prID, "pull title2 comment1", "")

		// 3. validate the webhook is triggered
		assert.Equal(t, "pull_request_comment", triggeredEvent)
		assert.Len(t, payloads, 1)
		assert.EqualValues(t, "created", payloads[0].Action)
		assert.Equal(t, "repo1", payloads[0].Issue.Repo.Name)
		assert.Equal(t, "user2/repo1", payloads[0].Issue.Repo.FullName)
		assert.Equal(t, "first pull request", payloads[0].Issue.Title)
		assert.Empty(t, payloads[0].Issue.Body)
		assert.Equal(t, "pull title2 comment1", payloads[0].Comment.Body)
	})
}

func Test_WebhookRepository(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, giteaURL *url.URL) {
		var payloads []api.RepositoryPayload
		var triggeredEvent string
		provider := newMockWebhookProvider(func(r *http.Request) {
			content, _ := io.ReadAll(r.Body)
			var payload api.RepositoryPayload
			err := json.Unmarshal(content, &payload)
			assert.NoError(t, err)
			payloads = append(payloads, payload)
			triggeredEvent = "repository"
		}, http.StatusOK)
		defer provider.Close()

		// 1. create a new webhook with special webhook for repo1
		session := loginUser(t, "user1")

		testAPICreateWebhookForOrg(t, session, "org3", provider.URL(), "repository")

		// 2. trigger the webhook
		testAPIOrgCreateRepo(t, session, "org3", "repo_new", http.StatusCreated)

		// 3. validate the webhook is triggered
		assert.Equal(t, "repository", triggeredEvent)
		assert.Len(t, payloads, 1)
		assert.EqualValues(t, "created", payloads[0].Action)
		assert.Equal(t, "org3", payloads[0].Organization.UserName)
		assert.Equal(t, "repo_new", payloads[0].Repository.Name)
		assert.Equal(t, "org3/repo_new", payloads[0].Repository.FullName)
	})
}

func Test_WebhookStatus(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, giteaURL *url.URL) {
		var payloads []api.CommitStatusPayload
		var triggeredEvent string
		provider := newMockWebhookProvider(func(r *http.Request) {
			assert.Contains(t, r.Header["X-Github-Event-Type"], "status", "X-GitHub-Event-Type should contain status")
			assert.Contains(t, r.Header["X-Github-Hook-Installation-Target-Type"], "repository", "X-GitHub-Hook-Installation-Target-Type should contain repository")
			assert.Contains(t, r.Header["X-Gitea-Event-Type"], "status", "X-Gitea-Event-Type should contain status")
			assert.Contains(t, r.Header["X-Gitea-Hook-Installation-Target-Type"], "repository", "X-Gitea-Hook-Installation-Target-Type should contain repository")
			assert.Contains(t, r.Header["X-Gogs-Event-Type"], "status", "X-Gogs-Event-Type should contain status")
			content, _ := io.ReadAll(r.Body)
			var payload api.CommitStatusPayload
			err := json.Unmarshal(content, &payload)
			assert.NoError(t, err)
			payloads = append(payloads, payload)
			triggeredEvent = "status"
		}, http.StatusOK)
		defer provider.Close()

		// 1. create a new webhook with special webhook for repo1
		session := loginUser(t, "user2")

		testAPICreateWebhookForRepo(t, session, "user2", "repo1", provider.URL(), "status")

		repo1 := unittest.AssertExistsAndLoadBean(t, &repo.Repository{ID: 1})

		gitRepo1, err := gitrepo.OpenRepository(t.Context(), repo1)
		assert.NoError(t, err)
		commitID, err := gitRepo1.GetBranchCommitID(repo1.DefaultBranch)
		assert.NoError(t, err)

		// 2. trigger the webhook
		testCtx := NewAPITestContext(t, "user2", "repo1", auth_model.AccessTokenScopeAll)

		// update a status for a commit via API
		doAPICreateCommitStatusTest(testCtx, commitID, commitstatus.CommitStatusSuccess, "testci")(t)

		// 3. validate the webhook is triggered
		assert.Equal(t, "status", triggeredEvent)
		assert.Len(t, payloads, 1)
		assert.Equal(t, commitID, payloads[0].Commit.ID)
		assert.Equal(t, "repo1", payloads[0].Repo.Name)
		assert.Equal(t, "user2/repo1", payloads[0].Repo.FullName)
		assert.Equal(t, "testci", payloads[0].Context)
		assert.Equal(t, commitID, payloads[0].SHA)
	})
}

func Test_WebhookStatus_NoWrongTrigger(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, giteaURL *url.URL) {
		var trigger string
		provider := newMockWebhookProvider(func(r *http.Request) {
			assert.NotContains(t, r.Header["X-Github-Event-Type"], "status", "X-GitHub-Event-Type should not contain status")
			assert.NotContains(t, r.Header["X-Gitea-Event-Type"], "status", "X-Gitea-Event-Type should not contain status")
			assert.NotContains(t, r.Header["X-Gogs-Event-Type"], "status", "X-Gogs-Event-Type should not contain status")
			trigger = "push"
		}, http.StatusOK)
		defer provider.Close()

		// 1. create a new webhook with special webhook for repo1
		session := loginUser(t, "user2")

		// create a push only webhook
		testAPICreateWebhookForRepo(t, session, "user2", "repo1", provider.URL(), "push")

		// 2. trigger the webhook with a push action
		testCreateFile(t, session, "user2", "repo1", "master", "", "test_webhook_push.md", "# a test file for webhook push")

		// 3. validate the webhook is triggered with right event
		assert.Equal(t, "push", trigger)
	})
}
