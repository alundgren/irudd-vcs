// Copyright 2014 The Gogs Authors. All rights reserved.
// Copyright 2020 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package context

import (
	"context"
	"net/http"
	"strings"

	user_model "gitea.dev/models/user"
	"gitea.dev/modules/cache"
	"gitea.dev/modules/httpcache"
	"gitea.dev/modules/setting"
	"gitea.dev/modules/web"
	web_types "gitea.dev/modules/web/types"
)

// Context represents context of a non-API request (git HTTP and LFS).
// ATTENTION: This struct should never be manually constructed in routes/services,
// it has many internal details which should be carefully prepared by the framework.
// If it is abused, it would cause strange bugs like panic/resource-leak.
type Context struct {
	*Base

	Cache cache.StringCache

	Link string // current request URL (without query string)

	Doer        *user_model.User // current signed-in user
	IsSigned    bool
	IsBasicAuth bool

	ContextUser *user_model.User // the user which is being visited, in most cases it differs from Doer

	Repo *Repository
	Org  *Organization
}

func init() {
	web.RegisterResponseStatusProvider[*Base](func(req *http.Request) web_types.ResponseStatusProvider {
		return req.Context().Value(BaseContextKey).(*Base)
	})
	web.RegisterResponseStatusProvider[*Context](func(req *http.Request) web_types.ResponseStatusProvider {
		return req.Context().Value(WebContextKey).(*Context)
	})
}

type webContextKeyType struct{}

var WebContextKey = webContextKeyType{}

func GetWebContext(ctx context.Context) *Context {
	webCtx, _ := ctx.Value(WebContextKey).(*Context)
	return webCtx
}

// ValidateContext is a special context for form validation middleware. It may be different from other contexts.
type ValidateContext struct {
	*Base
}

// GetValidateContext gets a context for middleware form validation
func GetValidateContext(req *http.Request) (ctx *ValidateContext) {
	if ctxAPI, ok := req.Context().Value(apiContextKey).(*APIContext); ok {
		ctx = &ValidateContext{Base: ctxAPI.Base}
	} else if ctxWeb, ok := req.Context().Value(WebContextKey).(*Context); ok {
		ctx = &ValidateContext{Base: ctxWeb.Base}
	} else {
		panic("invalid context, expect either APIContext or Context")
	}
	return ctx
}

func NewWebContext(base *Base) *Context {
	ctx := &Context{
		Base:  base,
		Cache: cache.GetCache(),
		Link:  setting.AppSubURL + strings.TrimSuffix(base.Req.URL.EscapedPath(), "/"),
		Repo:  &Repository{},
		Org:   &Organization{},
	}
	ctx.SetContextValue(WebContextKey, ctx)
	return ctx
}

// Contexter initializes a classic context for a request.
func Contexter() func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
			base := NewBaseContext(resp, req)
			ctx := NewWebContext(base)
			httpcache.SetCacheControlInHeader(ctx.Resp.Header(), &httpcache.CacheControlOptions{})
			next.ServeHTTP(ctx.Resp, ctx.Req)
		})
	}
}
