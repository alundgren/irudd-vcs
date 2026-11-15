// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"encoding/base64"
	"net/http"
	"testing"

	auth_model "gitea.dev/models/auth"
	"gitea.dev/modules/setting"
	api "gitea.dev/modules/structs"
	"gitea.dev/tests"

	"github.com/stretchr/testify/assert"
)

func TestEmptyRepoAddFileByAPI(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	session := loginUser(t, "user30")
	token := getTokenForLoggedInUser(t, session, auth_model.AccessTokenScopeWriteRepository)

	req := NewRequestWithJSON(t, "POST", "/api/v1/repos/user30/empty/contents/new-file.txt", &api.CreateFileOptions{
		FileOptions: api.FileOptions{
			NewBranchName: "new_branch",
			Message:       "init",
		},
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("newly-added-api-file")),
	}).AddTokenAuth(token)

	resp := MakeRequest(t, req, http.StatusCreated)
	fileResponse := DecodeJSON(t, resp, &api.FileResponse{})
	expectedHTMLURL := setting.AppURL + "user30/empty/src/branch/new_branch/new-file.txt"
	assert.Equal(t, expectedHTMLURL, *fileResponse.Content.HTMLURL)

	req = NewRequest(t, "GET", "/api/v1/repos/user30/empty/raw/new-file.txt?ref=new_branch")
	resp = session.MakeRequest(t, req, http.StatusOK)
	assert.Contains(t, resp.Body.String(), "newly-added-api-file")

	req = NewRequest(t, "GET", "/api/v1/repos/user30/empty").
		AddTokenAuth(token)
	resp = session.MakeRequest(t, req, http.StatusOK)
	apiRepo := DecodeJSON(t, resp, &api.Repository{})
	assert.Equal(t, "new_branch", apiRepo.DefaultBranch)
}
