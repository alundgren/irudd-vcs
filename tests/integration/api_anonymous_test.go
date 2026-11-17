// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gitea.dev/modules/json"
	"gitea.dev/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAPIAnonymousAccess checks every route in the served API spec: only the version and the
// specs are anonymous, everything else needs the owner's token
func TestAPIAnonymousAccess(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	resp := MakeRequest(t, NewRequest(t, "GET", "/swagger.v1.json"), http.StatusOK)
	MakeRequest(t, NewRequest(t, "GET", "/openapi3.v1.json"), http.StatusOK)
	var spec struct {
		BasePath string                    `json:"basePath"`
		Paths    map[string]map[string]any `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &spec))
	require.Greater(t, len(spec.Paths), 80, "the spec should list the API routes")

	params := regexp.MustCompile(`\{[^}]+\}`)
	value := func(param string) string {
		switch param {
		case "{owner}", "{username}":
			return "user2"
		case "{repo}":
			return "repo1"
		case "{sha}", "{ref}", "{branch}", "{name}", "{tag}", "{filepath}", "{basehead}", "{archive}", "{diffType}":
			return "master"
		}
		return "1"
	}

	paths := make([]string, 0, len(spec.Paths))
	for path := range spec.Paths {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	checked := 0
	for _, path := range paths {
		url := spec.BasePath + params.ReplaceAllStringFunc(path, value)
		for method := range spec.Paths[path] {
			method = strings.ToUpper(method)
			if method == "PARAMETERS" {
				continue
			}
			if path == "/version" {
				MakeRequest(t, NewRequest(t, method, url), http.StatusOK)
				continue
			}
			resp := MakeRequest(t, NewRequest(t, method, url), NoExpectedStatus)
			assert.Equal(t, http.StatusUnauthorized, resp.Code, "%s %s", method, url)
			checked++
		}
	}
	assert.Greater(t, checked, 120, "every route should have been checked")

	// git over HTTP and LFS need the token too
	for _, req := range []*RequestWrapper{
		NewRequest(t, "GET", "/user2/repo1.git/info/refs?service=git-upload-pack"),
		NewRequestWithBody(t, "POST", "/user2/repo1.git/git-upload-pack", strings.NewReader("0000")),
		NewRequest(t, "GET", "/user2/repo1.git/HEAD"),
		NewRequestWithJSON(t, "POST", "/user2/repo1.git/info/lfs/objects/batch", map[string]any{"operation": "download", "objects": []any{}}).
			SetHeader("Accept", "application/vnd.git-lfs+json"),
	} {
		resp := MakeRequest(t, req, NoExpectedStatus)
		assert.Equal(t, http.StatusUnauthorized, resp.Code, "%s %s", req.Method, req.URL)
	}
}
