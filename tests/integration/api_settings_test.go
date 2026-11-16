// Copyright 2020 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"testing"

	"gitea.dev/modules/setting"
	api "gitea.dev/modules/structs"
	"gitea.dev/tests"

	"github.com/stretchr/testify/assert"
)

func TestAPIExposedSettings(t *testing.T) {
	defer tests.PrepareTestEnv(t)()
	ownerSession := loginUser(t, "user2") // adds the token to requests without one

	req := NewRequest(t, "GET", "/api/v1/settings/api")
	resp := ownerSession.MakeRequest(t, req, http.StatusOK)

	apiSettings := DecodeJSON(t, resp, &api.GeneralAPISettings{})
	assert.Equal(t, &api.GeneralAPISettings{
		MaxResponseItems:       setting.API.MaxResponseItems,
		DefaultPagingNum:       setting.API.DefaultPagingNum,
		DefaultGitTreesPerPage: setting.API.DefaultGitTreesPerPage,
		DefaultMaxBlobSize:     setting.API.DefaultMaxBlobSize,
		DefaultMaxResponseSize: setting.API.DefaultMaxResponseSize,
	}, apiSettings)

	req = NewRequest(t, "GET", "/api/v1/settings/repository")
	resp = ownerSession.MakeRequest(t, req, http.StatusOK)

	repo := DecodeJSON(t, resp, &api.GeneralRepoSettings{})
	assert.Equal(t, &api.GeneralRepoSettings{
		MirrorsDisabled:    true, // mirrors, migrations and stars are removed
		HTTPGitDisabled:    setting.Repository.DisableHTTPGit,
		MigrationsDisabled: true,
		StarsDisabled:      true,
		LFSDisabled:        !setting.LFS.StartServer,
	}, repo)
}
