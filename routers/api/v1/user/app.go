// Copyright 2014 The Gogs Authors. All rights reserved.
// Copyright 2018 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package user

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	auth_model "gitea.dev/models/auth"
	"gitea.dev/models/db"
	api "gitea.dev/modules/structs"
	"gitea.dev/modules/web"
	"gitea.dev/routers/api/v1/utils"
	"gitea.dev/services/context"
)

// ListAccessTokens list all the access tokens
func ListAccessTokens(ctx *context.APIContext) {
	// swagger:operation GET /users/{username}/tokens user userGetTokens
	// ---
	// summary: List the authenticated user's access tokens
	// produces:
	// - application/json
	// parameters:
	// - name: username
	//   in: path
	//   description: username of to user whose access tokens are to be listed
	//   type: string
	//   required: true
	// - name: page
	//   in: query
	//   description: page number of results to return (1-based)
	//   type: integer
	// - name: limit
	//   in: query
	//   description: page size of results
	//   type: integer
	// responses:
	//   "200":
	//     "$ref": "#/responses/AccessTokenList"
	//   "403":
	//     "$ref": "#/responses/forbidden"

	opts := auth_model.ListAccessTokensOptions{UserID: ctx.ContextUser.ID, ListOptions: utils.GetListOptions(ctx)}

	tokens, count, err := db.FindAndCount[auth_model.AccessToken](ctx, opts)
	if err != nil {
		ctx.APIErrorInternal(err)
		return
	}

	apiTokens := make([]*api.AccessToken, len(tokens))
	for i := range tokens {
		apiTokens[i] = &api.AccessToken{
			ID:             tokens[i].ID,
			Name:           tokens[i].Name,
			TokenLastEight: tokens[i].TokenLastEight,
			Scopes:         tokens[i].Scope.StringSlice(),
			Created:        tokens[i].CreatedUnix.AsTime(),
			Updated:        tokens[i].UpdatedUnix.AsTime(),
		}
	}

	ctx.SetTotalCountHeader(count)
	ctx.JSON(http.StatusOK, &apiTokens)
}

// CreateAccessToken create access tokens
func CreateAccessToken(ctx *context.APIContext) {
	// swagger:operation POST /users/{username}/tokens user userCreateToken
	// ---
	// summary: Create an access token
	// consumes:
	// - application/json
	// produces:
	// - application/json
	// parameters:
	// - name: username
	//   in: path
	//   description: username of the user whose token is to be created
	//   required: true
	//   type: string
	// - name: body
	//   in: body
	//   schema:
	//     "$ref": "#/definitions/CreateAccessTokenOption"
	// responses:
	//   "201":
	//     "$ref": "#/responses/AccessToken"
	//   "400":
	//     "$ref": "#/responses/error"
	//   "403":
	//     "$ref": "#/responses/forbidden"

	form := web.GetForm(ctx).(*api.CreateAccessTokenOption)

	t := &auth_model.AccessToken{
		UID:  ctx.ContextUser.ID,
		Name: form.Name,
	}

	exist, err := auth_model.AccessTokenByNameExists(ctx, t)
	if err != nil {
		ctx.APIErrorInternal(err)
		return
	}
	if exist {
		ctx.APIError(http.StatusBadRequest, "access token name has been used already")
		return
	}

	scope, err := auth_model.AccessTokenScope(strings.Join(form.Scopes, ",")).Normalize()
	if err != nil {
		ctx.APIError(http.StatusBadRequest, fmt.Sprintf("invalid access token scope provided: %v", err))
		return
	}
	if scope == "" {
		ctx.APIError(http.StatusBadRequest, "access token must have a scope")
		return
	}
	t.Scope = scope

	// a token-authenticated request must not mint a token with a broader scope than its own
	if ctx.Data["IsApiToken"] == true {
		apiTokenScope, ok := ctx.Data["ApiTokenScope"].(auth_model.AccessTokenScope)
		if !ok {
			ctx.APIError(http.StatusForbidden, "the authenticating token has no scope")
			return
		}
		hasScope, err := apiTokenScope.CanCreateChildScope(scope)
		if err != nil {
			ctx.APIErrorInternal(err)
			return
		}
		if !hasScope {
			ctx.APIError(http.StatusForbidden, "cannot create an access token with a broader scope than the authenticating token")
			return
		}
		// a public-only token must not mint a token that drops the public-only restriction
		if t.Scope, err = t.Scope.EnforcePublicOnlyFrom(apiTokenScope); err != nil {
			ctx.APIErrorInternal(err)
			return
		}
	}

	if err := auth_model.NewAccessToken(ctx, t); err != nil {
		ctx.APIErrorInternal(err)
		return
	}
	ctx.JSON(http.StatusCreated, &api.AccessToken{
		Name:           t.Name,
		Token:          t.Token,
		ID:             t.ID,
		TokenLastEight: t.TokenLastEight,
		Scopes:         t.Scope.StringSlice(),
	})
}

// DeleteAccessToken delete access tokens
func DeleteAccessToken(ctx *context.APIContext) {
	// swagger:operation DELETE /users/{username}/tokens/{token} user userDeleteAccessToken
	// ---
	// summary: delete an access token
	// produces:
	// - application/json
	// parameters:
	// - name: username
	//   in: path
	//   description: username of the user whose token is to be deleted
	//   type: string
	//   required: true
	// - name: token
	//   in: path
	//   description: token to be deleted, identified by ID and if not available by name
	//   type: string
	//   required: true
	// responses:
	//   "204":
	//     "$ref": "#/responses/empty"
	//   "403":
	//     "$ref": "#/responses/forbidden"
	//   "404":
	//     "$ref": "#/responses/notFound"
	//   "422":
	//     "$ref": "#/responses/error"

	token := ctx.PathParam("id")
	tokenID, _ := strconv.ParseInt(token, 0, 64)

	if tokenID == 0 {
		tokens, err := db.Find[auth_model.AccessToken](ctx, auth_model.ListAccessTokensOptions{
			Name:   token,
			UserID: ctx.ContextUser.ID,
		})
		if err != nil {
			ctx.APIErrorInternal(err)
			return
		}

		switch len(tokens) {
		case 0:
			ctx.APIErrorNotFound()
			return
		case 1:
			tokenID = tokens[0].ID
		default:
			ctx.APIError(http.StatusUnprocessableEntity, fmt.Sprintf("multiple matches for token name '%s'", token))
			return
		}
	}

	if err := auth_model.DeleteAccessTokenByID(ctx, tokenID, ctx.ContextUser.ID); err != nil {
		ctx.APIErrorAuto(err)
		return
	}

	ctx.Status(http.StatusNoContent)
}
