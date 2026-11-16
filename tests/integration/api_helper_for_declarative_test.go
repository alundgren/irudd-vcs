// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"gitea.dev/models/auth"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/modules/json"
	"gitea.dev/modules/queue"
	api "gitea.dev/modules/structs"
	"gitea.dev/services/forms"

	"github.com/stretchr/testify/assert"
)

type APITestContext struct {
	Reponame     string
	Session      *TestSession
	Token        string
	Username     string
	ExpectedCode int
}

func NewAPITestContext(t *testing.T, username, reponame string, scope ...auth.AccessTokenScope) APITestContext {
	session := loginUser(t, username)
	if len(scope) == 0 {
		// FIXME: legacy logic: no scope means all
		scope = []auth.AccessTokenScope{auth.AccessTokenScopeAll}
	}
	token := getTokenForLoggedInUser(t, session, scope...)
	return APITestContext{
		Session:  session,
		Token:    token,
		Username: username,
		Reponame: reponame,
	}
}

func (ctx APITestContext) GitPath() string {
	return fmt.Sprintf("%s/%s.git", ctx.Username, ctx.Reponame)
}

func doAPICreateRepository(ctx APITestContext, empty bool, callback ...func(*testing.T, api.Repository)) func(*testing.T) {
	return func(t *testing.T) {
		createRepoOption := &api.CreateRepoOption{
			AutoInit:    !empty,
			Description: "Temporary repo",
			Name:        ctx.Reponame,
			Private:     true,
			Template:    true,
			Gitignores:  "",
			License:     "WTFPL",
			Readme:      "Default",
		}
		req := NewRequestWithJSON(t, "POST", "/api/v1/user/repos", createRepoOption).
			AddTokenAuth(ctx.Token)
		if ctx.ExpectedCode != 0 {
			ctx.Session.MakeRequest(t, req, ctx.ExpectedCode)
			return
		}
		resp := ctx.Session.MakeRequest(t, req, http.StatusCreated)

		repository := DecodeJSON(t, resp, &api.Repository{})
		if len(callback) > 0 {
			callback[0](t, *repository)
		}
	}
}

func doAPIEditRepository(ctx APITestContext, editRepoOption *api.EditRepoOption, callback ...func(*testing.T, api.Repository)) func(*testing.T) {
	return func(t *testing.T) {
		req := NewRequestWithJSON(t, "PATCH", fmt.Sprintf("/api/v1/repos/%s/%s", url.PathEscape(ctx.Username), url.PathEscape(ctx.Reponame)), editRepoOption).
			AddTokenAuth(ctx.Token)
		if ctx.ExpectedCode != 0 {
			ctx.Session.MakeRequest(t, req, ctx.ExpectedCode)
			return
		}
		resp := ctx.Session.MakeRequest(t, req, http.StatusOK)

		repository := DecodeJSON(t, resp, &api.Repository{})
		if len(callback) > 0 {
			callback[0](t, *repository)
		}
	}
}

func doAPIForkRepository(ctx APITestContext, username string, callback ...func(*testing.T, api.Repository)) func(*testing.T) {
	return func(t *testing.T) {
		createForkOption := &api.CreateForkOption{}
		req := NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/forks", username, ctx.Reponame), createForkOption).
			AddTokenAuth(ctx.Token)
		if ctx.ExpectedCode != 0 {
			ctx.Session.MakeRequest(t, req, ctx.ExpectedCode)
			return
		}
		resp := ctx.Session.MakeRequest(t, req, http.StatusAccepted)
		repository := DecodeJSON(t, resp, &api.Repository{})
		if len(callback) > 0 {
			callback[0](t, *repository)
		}
	}
}

func doAPIGetRepository(ctx APITestContext, callback ...func(*testing.T, api.Repository)) func(*testing.T) {
	return func(t *testing.T) {
		req := NewRequest(t, "GET", fmt.Sprintf("/api/v1/repos/%s/%s", ctx.Username, ctx.Reponame)).
			AddTokenAuth(ctx.Token)
		if ctx.ExpectedCode != 0 {
			ctx.Session.MakeRequest(t, req, ctx.ExpectedCode)
			return
		}
		resp := ctx.Session.MakeRequest(t, req, http.StatusOK)

		repository := DecodeJSON(t, resp, &api.Repository{})
		if len(callback) > 0 {
			callback[0](t, *repository)
		}
	}
}

func doAPIDeleteRepository(ctx APITestContext) func(*testing.T) {
	return func(t *testing.T) {
		req := NewRequest(t, "DELETE", fmt.Sprintf("/api/v1/repos/%s/%s", ctx.Username, ctx.Reponame)).
			AddTokenAuth(ctx.Token)
		if ctx.ExpectedCode != 0 {
			ctx.Session.MakeRequest(t, req, ctx.ExpectedCode)
			return
		}
		ctx.Session.MakeRequest(t, req, http.StatusNoContent)
	}
}

func doAPICreatePullRequest(ctx APITestContext, owner, repo, baseBranch, headBranch string) func(*testing.T) (api.PullRequest, error) {
	return func(t *testing.T) (api.PullRequest, error) {
		req := NewRequestWithJSON(t, http.MethodPost, fmt.Sprintf("/api/v1/repos/%s/%s/pulls", owner, repo), &api.CreatePullRequestOption{
			Head:  headBranch,
			Base:  baseBranch,
			Title: fmt.Sprintf("create a pr from %s to %s", headBranch, baseBranch),
		}).AddTokenAuth(ctx.Token)

		expected := http.StatusCreated
		if ctx.ExpectedCode != 0 {
			expected = ctx.ExpectedCode
		}
		resp := ctx.Session.MakeRequest(t, req, expected)

		decoder := json.NewDecoder(resp.Body)
		pr := api.PullRequest{}
		err := decoder.Decode(&pr)
		return pr, err
	}
}

func doAPIGetPullRequest(ctx APITestContext, owner, repo string, index int64) func(*testing.T) (api.PullRequest, error) {
	return func(t *testing.T) (api.PullRequest, error) {
		req := NewRequest(t, http.MethodGet, fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%d", owner, repo, index)).
			AddTokenAuth(ctx.Token)

		expected := http.StatusOK
		if ctx.ExpectedCode != 0 {
			expected = ctx.ExpectedCode
		}
		resp := ctx.Session.MakeRequest(t, req, expected)

		decoder := json.NewDecoder(resp.Body)
		pr := api.PullRequest{}
		err := decoder.Decode(&pr)
		return pr, err
	}
}

func doAPIMergePullRequest(ctx APITestContext, owner, repo string, index int64) func(*testing.T) {
	return func(t *testing.T) {
		urlStr := fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%d/merge", owner, repo, index)

		var req *RequestWrapper
		var resp *httptest.ResponseRecorder

		for range 6 {
			req = NewRequestWithJSON(t, http.MethodPost, urlStr, &forms.MergePullRequestForm{
				MergeMessageField: "doAPIMergePullRequest Merge",
				Do:                string(repo_model.MergeStyleMerge),
			}).AddTokenAuth(ctx.Token)

			resp = ctx.Session.MakeRequest(t, req, NoExpectedStatus)

			if resp.Code != http.StatusMethodNotAllowed {
				break
			}
			err := DecodeJSON(t, resp, &api.APIError{})
			assert.Equal(t, "Please try again later", err.Message)
			queue.GetManager().FlushAll(t.Context(), 5*time.Second)
			<-time.After(1 * time.Second)
		}

		expected := ctx.ExpectedCode
		if expected == 0 {
			expected = http.StatusOK
		}

		if !assert.Equal(t, expected, resp.Code,
			"Request: %s %s", req.Method, req.URL.String()) {
			logUnexpectedResponse(t, resp)
		}
	}
}

func doAPIManuallyMergePullRequest(ctx APITestContext, owner, repo, commitID string, index int64) func(*testing.T) {
	return func(t *testing.T) {
		urlStr := fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%d/merge", owner, repo, index)
		req := NewRequestWithJSON(t, http.MethodPost, urlStr, &forms.MergePullRequestForm{
			Do:            string(repo_model.MergeStyleManuallyMerged),
			MergeCommitID: commitID,
		}).AddTokenAuth(ctx.Token)

		if ctx.ExpectedCode != 0 {
			ctx.Session.MakeRequest(t, req, ctx.ExpectedCode)
			return
		}
		ctx.Session.MakeRequest(t, req, http.StatusOK)
	}
}

func doAPIGetBranch(ctx APITestContext, branch string, callback ...func(*testing.T, api.Branch)) func(*testing.T) {
	return func(t *testing.T) {
		req := NewRequestf(t, "GET", "/api/v1/repos/%s/%s/branches/%s", ctx.Username, ctx.Reponame, branch).
			AddTokenAuth(ctx.Token)
		if ctx.ExpectedCode != 0 {
			ctx.Session.MakeRequest(t, req, ctx.ExpectedCode)
			return
		}
		resp := ctx.Session.MakeRequest(t, req, http.StatusOK)

		branch := DecodeJSON(t, resp, &api.Branch{})
		if len(callback) > 0 {
			callback[0](t, *branch)
		}
	}
}

func doAPICreateFile(ctx APITestContext, treepath string, options *api.CreateFileOptions, callback ...func(*testing.T, api.FileResponse)) func(*testing.T) {
	return func(t *testing.T) {
		req := NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/contents/%s", ctx.Username, ctx.Reponame, treepath), &options).
			AddTokenAuth(ctx.Token)
		if ctx.ExpectedCode != 0 {
			ctx.Session.MakeRequest(t, req, ctx.ExpectedCode)
			return
		}
		resp := ctx.Session.MakeRequest(t, req, http.StatusCreated)

		contents := DecodeJSON(t, resp, &api.FileResponse{})
		if len(callback) > 0 {
			callback[0](t, *contents)
		}
	}
}
