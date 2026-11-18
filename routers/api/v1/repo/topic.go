// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"net/http"

	api "gitea.dev/modules/structs"
	"gitea.dev/modules/util"
	"gitea.dev/services/context"
)

// ListTopics returns the topics stored on the repository
func ListTopics(ctx *context.APIContext) {
	// swagger:operation GET /repos/{owner}/{repo}/topics repository repoListTopics
	// ---
	// summary: Get list of topics that a repository has
	// produces:
	//   - application/json
	// parameters:
	// - name: owner
	//   in: path
	//   description: owner of the repo
	//   type: string
	//   required: true
	// - name: repo
	//   in: path
	//   description: name of the repo
	//   type: string
	//   required: true
	// responses:
	//   "200":
	//     "$ref": "#/responses/TopicNames"
	//   "404":
	//     "$ref": "#/responses/notFound"

	// kept read-only because tea reads it after creating, editing or showing a repository
	ctx.JSON(http.StatusOK, api.TopicName{TopicNames: util.SliceNilAsEmpty(ctx.Repo.Repository.Topics)})
}
