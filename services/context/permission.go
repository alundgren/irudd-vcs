// Copyright 2018 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package context

import (
	"context"
	"net/http"

	auth_model "gitea.dev/models/auth"
	repo_model "gitea.dev/models/repo"
)

// isOwnerHidden reports whether repo's owner is not publicly visible (a limited or private owner), so
// the owner's repositories must be hidden from callers that may only reach genuinely public resources.
func isOwnerHidden(ctx context.Context, repo *repo_model.Repository) bool {
	if err := repo.LoadOwner(ctx); err != nil || repo.Owner == nil {
		return true // fail closed if the owner visibility can't be determined
	}
	return !repo.Owner.Visibility.IsPublic()
}

// publicOnlyTokenDeniedRepo reports whether a public-only API token must be denied access to
// repo. A public-only token may only reach genuinely public resources, so it is denied for
// private repos and for repos owned by a non-public (limited or private) owner.
func publicOnlyTokenDeniedRepo(ctx context.Context, repo *repo_model.Repository) bool {
	if repo == nil {
		return false
	}
	return repo.IsPrivate || isOwnerHidden(ctx, repo)
}

// CheckRepoScopedToken checks whether the authenticated API token has repo scope.
func CheckRepoScopedToken(ctx *Context, repo *repo_model.Repository, level auth_model.AccessTokenScopeLevel) {
	CheckTokenScopes(ctx, repo, auth_model.GetRequiredScopes(level, auth_model.AccessTokenScopeCategoryRepository)...)
}

// CheckTokenScopes checks whether the authenticated API token contains any of the given scopes.
func CheckTokenScopes(ctx *Context, repo *repo_model.Repository, scopes ...auth_model.AccessTokenScope) {
	if ctx.Data["IsApiToken"] != true {
		return
	}

	scope, ok := ctx.Data["ApiTokenScope"].(auth_model.AccessTokenScope)
	if !ok {
		return
	}

	publicOnly, err := scope.PublicOnly()
	if err != nil {
		ctx.ServerError("PublicOnly", err)
		return
	}

	if publicOnly && publicOnlyTokenDeniedRepo(ctx, repo) {
		ctx.HTTPError(http.StatusForbidden)
		return
	}

	scopeMatched, err := scope.HasAnyScope(scopes...)
	if err != nil {
		ctx.ServerError("HasAnyScope", err)
		return
	}

	if !scopeMatched {
		ctx.HTTPError(http.StatusForbidden)
	}
}
