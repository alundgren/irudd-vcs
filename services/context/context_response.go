// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package context

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"

	user_model "gitea.dev/models/user"
	"gitea.dev/modules/log"
	"gitea.dev/modules/setting"
	"gitea.dev/modules/structs"
	"gitea.dev/modules/util"
)

// RedirectToUser redirect to a differently-named user
func RedirectToUser(ctx *Base, doer *user_model.User, userName string, redirectUserID int64) {
	user, err := user_model.GetUserByID(ctx, redirectUserID)
	if err != nil {
		if user_model.IsErrUserNotExist(err) {
			ctx.HTTPError(http.StatusNotFound, "user does not exist")
		} else {
			ctx.HTTPError(http.StatusInternalServerError, "unable to get user")
		}
		return
	}

	// Handle Visibility
	if user.Visibility != structs.VisibleTypePublic && doer == nil {
		// We must be signed in to see limited or private organizations
		ctx.HTTPError(http.StatusNotFound, "user does not exist")
		return
	}

	redirectPath := strings.Replace(
		ctx.Req.URL.EscapedPath(),
		url.PathEscape(userName),
		url.PathEscape(user.Name),
		1,
	)
	if ctx.Req.URL.RawQuery != "" {
		redirectPath += "?" + ctx.Req.URL.RawQuery
	}
	ctx.Redirect(path.Join(setting.AppSubURL, redirectPath), http.StatusTemporaryRedirect)
}

// NotFound responds with a plain 404 and logs the given error, if any.
func (ctx *Context) NotFound(logErr error) {
	ctx.notFoundInternal("", logErr)
}

func (ctx *Context) notFoundInternal(logMsg string, logErr error) {
	if logErr != nil {
		log.Log(2, log.DEBUG, "%s: %v", logMsg, logErr)
	}
	ctx.plainTextInternal(3, http.StatusNotFound, []byte("Not found.\n"))
}

// ServerError responds with a plain 500 and logs the given error, if any.
// If the error is controlled by our error system, a 404 is sent instead.
func (ctx *Context) ServerError(logMsg string, logErr error) {
	if errors.Is(logErr, util.ErrNotExist) {
		ctx.notFoundInternal(logMsg, logErr)
		return
	}
	ctx.serverErrorInternal(logMsg, logErr)
}

func (ctx *Context) serverErrorInternal(logMsg string, logErr error) {
	if logErr != nil {
		log.ErrorWithSkip(2, "%s: %v", logMsg, logErr)
		if _, ok := logErr.(*net.OpError); ok || errors.Is(logErr, &net.OpError{}) {
			// This is an error within the underlying connection
			// and further rendering will not work so just return
			return
		}
	}
	ctx.plainTextInternal(3, http.StatusInternalServerError, []byte("Internal Server Error\n"))
}

// NotFoundOrServerError use error check function to determine if the error
// is about not found. It responds with 404 status code for not found error,
// or error context description for logging purpose of 500 server error.
// TODO: remove the "errCheck" and use util.ErrNotFound to check
func (ctx *Context) NotFoundOrServerError(logMsg string, errCheck func(error) bool, logErr error) {
	if errCheck(logErr) {
		ctx.notFoundInternal(logMsg, logErr)
		return
	}
	ctx.serverErrorInternal(logMsg, logErr)
}
