// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package web

import (
	"net/http"

	"gitea.dev/modules/log"
	"gitea.dev/modules/reqctx"
	"gitea.dev/modules/setting"
	"gitea.dev/modules/web"
	"gitea.dev/modules/web/routing"
	"gitea.dev/modules/web/types"
	"gitea.dev/routers/common"
	"gitea.dev/routers/web/repo"
	auth_service "gitea.dev/services/auth"
	"gitea.dev/services/context"

	chi_middleware "github.com/go-chi/chi/v5/middleware"
	"github.com/klauspost/compress/gzhttp"
)

var GzipMinSize = 1400 // min size to compress for the body size of response

type AuthMiddleware struct {
	AllowOAuth2       types.PreMiddlewareProvider
	AllowBasic        types.PreMiddlewareProvider
	MiddlewareHandler func(*context.Context)
}

// newWebAuthMiddleware authenticates non-API routes (git over HTTP, LFS) by token or basic auth; there are no browser sessions
func newWebAuthMiddleware() *AuthMiddleware {
	type keyAllowOAuth2 struct{}
	type keyAllowBasic struct{}
	webAuth := &AuthMiddleware{}

	middlewareSetContextValue := func(key, val any) types.PreMiddlewareProvider {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				dataStore := reqctx.GetRequestDataStore(r.Context())
				dataStore.SetContextValue(key, val)
				next.ServeHTTP(w, r)
			})
		}
	}

	webAuth.AllowBasic = middlewareSetContextValue(keyAllowBasic{}, true)
	webAuth.AllowOAuth2 = middlewareSetContextValue(keyAllowOAuth2{}, true)

	webAuth.MiddlewareHandler = func(ctx *context.Context) {
		group := auth_service.NewGroup()
		if ctx.GetContextValue(keyAllowOAuth2{}) == true {
			group.Add(&auth_service.OAuth2{})
		}
		if ctx.GetContextValue(keyAllowBasic{}) == true {
			group.Add(&auth_service.Basic{})
		}

		ar, err := common.AuthShared(ctx.Base, group)
		if err != nil {
			log.Error("Failed to verify user: %v", err)
			ctx.HTTPError(http.StatusUnauthorized, "Failed to authenticate user")
			return
		}
		ctx.Doer = ar.Doer
		ctx.IsSigned = ar.Doer != nil
	}
	return webAuth
}

// checkSignedInUser rejects users who may not log in
func checkSignedInUser(ctx *context.Context) {
	if !ctx.IsSigned {
		return
	}
	if !ctx.Doer.IsActive || ctx.Doer.ProhibitLogin {
		log.Info("Failed authentication attempt for %s from %s", ctx.Doer.Name, ctx.RemoteAddr())
		ctx.HTTPError(http.StatusForbidden, "This account is prohibited from signing in")
	}
}

// Routes returns the non-API routes: git over HTTP, LFS and the API spec
func Routes() *web.Router {
	routes := web.NewRouter()

	// GetHead allows a HEAD request redirect to GET if HEAD method is not defined for that route
	routes.BeforeRouting(chi_middleware.GetHead)

	var mid []any
	if setting.EnableGzip {
		// random jitter is recommended by: https://pkg.go.dev/github.com/klauspost/compress/gzhttp#readme-breach-mitigation
		wrapper, err := gzhttp.NewWrapper(gzhttp.RandomJitter(32, 0, false), gzhttp.MinSize(GzipMinSize), gzhttp.CompressionLevel(6))
		if err != nil {
			log.Fatal("gzhttp.NewWrapper failed: %v", err)
		}
		mid = append(mid, wrapper)
	}

	webAuth := newWebAuthMiddleware()
	mid = append(mid, context.Contexter(), webAuth.MiddlewareHandler)

	webRoutes := web.NewRouter()
	webRoutes.AfterRouting(mid...)
	registerWebRoutes(webRoutes, webAuth)

	routes.Mount("", webRoutes)
	return routes
}

func registerWebRoutes(m *web.Router, webAuth *AuthMiddleware) {
	lfsServerEnabled := func(ctx *context.Context) {
		if !setting.LFS.StartServer {
			ctx.HTTPError(http.StatusNotFound)
			return
		}
	}

	// git lfs uses its own jwt key, and it handles the token & auth by itself, it conflicts with the general "OAuth2" auth method
	// pattern: "/{username}/{reponame}/{lfs-paths}": git-lfs support, see also addOwnerRepoGitHTTPRouters
	common.AddOwnerRepoGitLFSRoutes(m, lfsServerEnabled, webAuth.AllowBasic, repo.CorsHandler(), checkSignedInUser)

	// pattern: "/{username}/{reponame}/{git-paths}": git http support
	addOwnerRepoGitHTTPRouters(m, repo.HTTPGitEnabledHandler, webAuth.AllowBasic, webAuth.AllowOAuth2, repo.CorsHandler(), checkSignedInUser, context.UserAssignmentWeb())

	if setting.API.EnableSwagger {
		m.Get("/swagger.v1.json", SwaggerV1Json)
		m.Get("/openapi3.v1.json", OpenAPI3Json)
	}

	m.NotFound(func(w http.ResponseWriter, req *http.Request) {
		defer routing.RecordFuncInfo(req.Context(), routing.GetFuncInfo(http.NotFound, "WebNotFound"))()
		http.NotFound(w, req)
	})
}
