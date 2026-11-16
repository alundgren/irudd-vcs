// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package common

import (
	"fmt"
	"net/http"

	user_model "gitea.dev/models/user"
	"gitea.dev/modules/httpcache"
	"gitea.dev/modules/log"
	"gitea.dev/modules/setting"
	"gitea.dev/modules/web/middleware"
	"gitea.dev/modules/web/routing"
)

// renderPanicErrorPage responds with a plain 500 for the recovered panic value, it handles the stack trace, and it never panics
func renderPanicErrorPage(w http.ResponseWriter, req *http.Request, recovered any) {
	combinedErr := fmt.Errorf("%v\n%s", recovered, log.Stack(2))
	log.Error("PANIC: %v", combinedErr)

	defer func() {
		if err := recover(); err != nil {
			log.Error("Panic occurs again when rendering error page: %v. Stack:\n%s", combinedErr, log.Stack(2))
		}
	}()

	routing.UpdatePanicError(req.Context(), combinedErr)

	plainMsg := "Internal Server Error"
	ctxData := middleware.GetContextData(req.Context())
	// This recovery handler could be called without Gitea's web context, so we shouldn't touch that context too much.
	user, _ := ctxData[middleware.ContextDataKeySignedUser].(*user_model.User)
	if !setting.IsProd || (user != nil && user.IsAdmin) {
		plainMsg = "PANIC: " + combinedErr.Error()
	}

	httpcache.SetCacheControlInHeader(w.Header(), &httpcache.CacheControlOptions{})
	w.Header().Set("Content-Type", "text/plain;charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write([]byte(plainMsg))
}
