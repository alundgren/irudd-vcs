// Copyright 2020 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package web

import (
	"net/http"

	"gitea.dev/modules/apispec"
	"gitea.dev/services/context"
)

// SwaggerV1Json render swagger v1 json
func SwaggerV1Json(ctx *context.Context) {
	ctx.Resp.Header().Set("Content-Type", "application/json")
	ctx.Resp.WriteHeader(http.StatusOK)
	_, _ = ctx.Resp.Write(apispec.SwaggerV1())
}

// OpenAPI3Json render OpenAPI 3.0 json (auto-converted from Swagger 2.0)
func OpenAPI3Json(ctx *context.Context) {
	ctx.Resp.Header().Set("Content-Type", "application/json")
	ctx.Resp.WriteHeader(http.StatusOK)
	_, _ = ctx.Resp.Write(apispec.OpenAPI3V1())
}
