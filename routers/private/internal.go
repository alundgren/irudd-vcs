// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

// Package private contains all internal routes. The package name "internal" isn't usable because Golang reserves it for disabling cross-package usage.
package private

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"

	"gitea.dev/modules/log"
	"gitea.dev/modules/private"
	"gitea.dev/modules/setting"
	"gitea.dev/modules/web"
	"gitea.dev/services/context"

	"gitea.com/go-chi/binding"
)

func authInternal(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if setting.InternalToken == "" {
			log.Warn(`The INTERNAL_TOKEN setting is missing from the configuration file: %q, internal API can't work.`, setting.CustomConf)
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}

		tokens := req.Header.Get("X-Gitea-Internal-Auth") // TODO: use something like JWT or HMAC to avoid passing the token in the clear
		after, found := strings.CutPrefix(tokens, "Bearer ")
		authSucceeded := found && subtle.ConstantTimeCompare([]byte(after), []byte(setting.InternalToken)) == 1
		if !authSucceeded {
			log.Debug("Forbidden attempt to access internal API")
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, req)
	})
}

// bind binding an obj to a handler
func bind[T any](_ T) any {
	return func(ctx *context.PrivateContext) {
		theObj := new(T) // create a new form obj for every request but not use obj directly
		binding.Bind(ctx.Req, theObj)
		web.SetForm(ctx, theObj)
	}
}

// setRealIP sets RemoteAddr from the trusted X-Real-IP header set by the internal API
// client (see modules/private.NewInternalRequest); the internal API is gated by InternalToken.
// It replaces chi's deprecated middleware.RealIP, which is unsafe on public-facing endpoints.
func setRealIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if ip := req.Header.Get("X-Real-IP"); net.ParseIP(ip) != nil {
			req.RemoteAddr = ip
		}
		next.ServeHTTP(w, req)
	})
}

// Routes registers all internal APIs routes to web application.
// These APIs will be invoked by internal commands for example `gitea serv` and etc.
func Routes() *web.Router {
	r := web.NewRouter()
	r.AfterRouting(context.PrivateContexter())
	r.AfterRouting(authInternal)
	// Log the real ip address of the request from SSH is really helpful for diagnosing sometimes.
	// Since internal API will be sent only from Gitea sub commands and it's under control (checked by InternalToken), we can trust the headers.
	r.AfterRouting(setRealIP)

	r.Get("/dummy", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Post("/ssh/log", bind(private.SSHLogOption{}), SSHLog)
	r.Post("/hook/pre-receive/{owner}/{repo}", RepoAssignment, bind(private.HookOptions{}), HookPreReceive)
	r.Post("/hook/post-receive/{owner}/{repo}", context.OverrideContext(), bind(private.HookOptions{}), HookPostReceive)
	r.Post("/manager/shutdown", Shutdown)
	r.Post("/manager/restart", Restart)
	r.Post("/manager/flush-queues", bind(private.FlushOptions{}), FlushQueues)
	r.Post("/manager/pause-logging", PauseLogging)
	r.Post("/manager/resume-logging", ResumeLogging)
	r.Post("/manager/release-and-reopen-logging", ReleaseReopenLogging)
	r.Post("/manager/set-log-sql", SetLogSQL)
	r.Get("/manager/processes", Processes)

	return r
}
