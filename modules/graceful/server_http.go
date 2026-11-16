// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package graceful

import (
	"context"
	"net"
	"net/http"
)

func newHTTPServer(network, address, name string, handler http.Handler) (*Server, ServeFunction) {
	server := NewServer(network, address, name)
	protocols := http.Protocols{}
	protocols.SetHTTP1(true)
	protocols.SetHTTP2(true)            // HTTP/2 can only be used when Gitea is configured to use TLS
	protocols.SetUnencryptedHTTP2(true) // Allow HTTP/2 without TLS, in case Gitea is behind a reverse proxy
	httpServer := http.Server{
		Protocols:   &protocols,
		Handler:     handler,
		BaseContext: func(net.Listener) context.Context { return GetManager().HammerContext() },
	}
	server.OnShutdown = func() {
		httpServer.SetKeepAlivesEnabled(false)
	}
	return server, httpServer.Serve
}

// HTTPListenAndServe listens on the provided network address and then calls Serve
// to handle requests on incoming connections.
func HTTPListenAndServe(network, address, name string, handler http.Handler) error {
	server, lHandler := newHTTPServer(network, address, name, handler)
	return server.ListenAndServe(lHandler)
}
