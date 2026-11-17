// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package graceful

import (
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPServerReadHeaderTimeout(t *testing.T) {
	oldTimeout := readHeaderTimeout
	readHeaderTimeout = 200 * time.Millisecond
	defer func() { readHeaderTimeout = oldTimeout }()

	_, serve := newHTTPServer("tcp", "127.0.0.1:0", "test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_, _ = w.Write(body)
	}))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	go func() { _ = serve(listener) }()

	t.Run("IncompleteHeaders", func(t *testing.T) {
		conn, err := net.Dial("tcp", listener.Addr().String())
		require.NoError(t, err)
		defer conn.Close()
		_, err = conn.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n"))
		require.NoError(t, err)
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
		_, err = io.ReadAll(conn)
		assert.NoError(t, err, "the server should close the connection before the client deadline")
	})

	t.Run("SlowBody", func(t *testing.T) {
		// the body may take longer than the header timeout, as a long push does
		conn, err := net.Dial("tcp", listener.Addr().String())
		require.NoError(t, err)
		defer conn.Close()
		_, err = conn.Write([]byte("POST / HTTP/1.1\r\nHost: x\r\nContent-Length: 4\r\nConnection: close\r\n\r\nab"))
		require.NoError(t, err)
		time.Sleep(3 * readHeaderTimeout)
		_, err = conn.Write([]byte("cd"))
		require.NoError(t, err)
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
		resp, err := io.ReadAll(conn)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(string(resp), "HTTP/1.1 200 OK"), string(resp))
		assert.True(t, strings.HasSuffix(string(resp), "abcd"), string(resp))
	})
}
