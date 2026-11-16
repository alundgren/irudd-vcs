// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

// Package contexttest provides utilities for testing Web/API contexts with models.
package contexttest

import (
	gocontext "context"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	access_model "gitea.dev/models/perm/access"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/cache"
	git_module "gitea.dev/modules/git"
	"gitea.dev/modules/gitrepo"
	"gitea.dev/modules/reqctx"
	"gitea.dev/modules/web/middleware"
	"gitea.dev/services/context"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mockRequest(t *testing.T, reqPath string) *http.Request {
	method, path, found := strings.Cut(reqPath, " ")
	if !found {
		method = "GET"
		path = reqPath
	}
	requestURL, err := url.Parse(path)
	assert.NoError(t, err)
	req := &http.Request{Method: method, Host: requestURL.Host, URL: requestURL, Form: maps.Clone(requestURL.Query()), Header: http.Header{}}
	req = req.WithContext(reqctx.NewRequestContextForTest(req.Context()))
	return req
}

// MockContext mock context for unit tests
func MockContext(t *testing.T, reqPath string) (*context.Context, *httptest.ResponseRecorder) {
	resp := httptest.NewRecorder()
	req := mockRequest(t, reqPath)
	base := context.NewBaseContext(resp, req)
	base.Data = middleware.GetContextData(req.Context())

	chiCtx := chi.NewRouteContext()
	ctx := context.NewWebContext(base)
	ctx.SetContextValue(chi.RouteCtxKey, chiCtx)
	ctx.Cache = cache.GetCache()
	return ctx, resp
}

// MockAPIContext mock context for unit tests
func MockAPIContext(t *testing.T, reqPath string) (*context.APIContext, *httptest.ResponseRecorder) {
	resp := httptest.NewRecorder()
	req := mockRequest(t, reqPath)
	base := context.NewBaseContext(resp, req)
	base.Data = middleware.GetContextData(req.Context())
	ctx := &context.APIContext{Base: base, Repo: &context.Repository{}}
	chiCtx := chi.NewRouteContext()
	ctx.SetContextValue(chi.RouteCtxKey, chiCtx)
	return ctx, resp
}

func MockPrivateContext(t *testing.T, reqPath string) (*context.PrivateContext, *httptest.ResponseRecorder) {
	resp := httptest.NewRecorder()
	req := mockRequest(t, reqPath)
	base := context.NewBaseContext(resp, req)
	base.Data = middleware.GetContextData(req.Context())
	ctx := &context.PrivateContext{Base: base}
	chiCtx := chi.NewRouteContext()
	ctx.SetContextValue(chi.RouteCtxKey, chiCtx)
	return ctx, resp
}

// LoadRepo load a repo into a test context.
func LoadRepo(t *testing.T, ctx gocontext.Context, repoID int64) {
	var doer *user_model.User
	var repo *context.Repository
	switch ctx := ctx.(type) {
	case *context.Context:
		repo = ctx.Repo
		doer = ctx.Doer
	case *context.APIContext:
		repo = ctx.Repo
		doer = ctx.Doer
	default:
		assert.FailNow(t, "context is not *context.Context or *context.APIContext")
	}

	repo.Repository = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: repoID})
	var err error
	repo.Owner, err = user_model.GetUserByID(ctx, repo.Repository.OwnerID)
	assert.NoError(t, err)
	repo.RepoLink = repo.Repository.Link()
	repo.Permission, err = access_model.GetDoerRepoPermission(ctx, repo.Repository, doer)
	assert.NoError(t, err)
}

// LoadRepoCommit loads a repo's commit into a test context.
func LoadRepoCommit(t *testing.T, ctx gocontext.Context) {
	var repo *context.Repository
	switch ctx := ctx.(type) {
	case *context.Context:
		repo = ctx.Repo
	case *context.APIContext:
		repo = ctx.Repo
	default:
		assert.FailNow(t, "context is not *context.Context or *context.APIContext")
	}

	gitRepo, err := gitrepo.OpenRepository(ctx, repo.Repository)
	require.NoError(t, err)
	t.Cleanup(func() {
		gitRepo.Close()
	})
	if repo.RefFullName == "" {
		repo.RefFullName = git_module.RefNameFromBranch(repo.Repository.DefaultBranch)
	}
	if repo.RefFullName.IsPull() {
		repo.BranchName = repo.RefFullName.ShortName()
	}
	repo.Commit, err = gitRepo.GetCommit(repo.RefFullName.String())
	require.NoError(t, err)
}

// LoadUser load a user into a test context
func LoadUser(t *testing.T, ctx gocontext.Context, userID int64) {
	doer := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: userID})
	switch ctx := ctx.(type) {
	case *context.Context:
		ctx.Doer = doer
		ctx.IsSigned = true
	case *context.APIContext:
		ctx.Doer = doer
		ctx.IsSigned = true
	default:
		assert.FailNow(t, "context is not *context.Context or *context.APIContext")
	}
}

// LoadGitRepo load a git repo into a test context. Requires that ctx.Repo has
// already been populated.
func LoadGitRepo(t *testing.T, ctx gocontext.Context) {
	var repo *context.Repository
	switch ctx := any(ctx).(type) {
	case *context.Context:
		repo = ctx.Repo
	case *context.APIContext:
		repo = ctx.Repo
	default:
		assert.FailNow(t, "context is not *context.Context or *context.APIContext")
	}
	assert.NoError(t, repo.Repository.LoadOwner(ctx))
	var err error
	repo.GitRepo, err = gitrepo.OpenRepository(ctx, repo.Repository)
	assert.NoError(t, err)
}
