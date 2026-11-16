// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

// Package apispec embeds the generated Swagger 2.0 and OpenAPI 3.0 specs of the API.
package apispec

import (
	"html/template"
	"strings"
	"sync"

	"gitea.dev/modules/setting"

	_ "embed"
)

var (
	//go:embed v1_json.tmpl
	swaggerV1 string

	//go:embed v1_openapi3_json.tmpl
	openAPI3V1 string
)

// The specs keep the legacy template placeholders so the generators stay unchanged
func render(spec string) []byte {
	return []byte(strings.NewReplacer(
		"{{.SwaggerAppVer}}", template.JSEscapeString(setting.AppVer),
		"{{.SwaggerAppSubUrl}}", setting.AppSubURL, // it is JS-safe
	).Replace(spec))
}

var (
	SwaggerV1  = sync.OnceValue(func() []byte { return render(swaggerV1) })
	OpenAPI3V1 = sync.OnceValue(func() []byte { return render(openAPI3V1) })
)
