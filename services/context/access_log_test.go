// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package context

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"gitea.dev/modules/log"
	"gitea.dev/modules/setting"

	"github.com/stretchr/testify/assert"
)

type testAccessLoggerMock struct {
	logs []string
}

func (t *testAccessLoggerMock) Log(skip int, event *log.Event, format string, v ...any) {
	t.logs = append(t.logs, fmt.Sprintf(format, v...))
}

func (t *testAccessLoggerMock) GetLevel() log.Level {
	return log.INFO
}

type testAccessLoggerResponseWriterMock struct{}

func (t testAccessLoggerResponseWriterMock) Header() http.Header {
	return nil
}

func (t testAccessLoggerResponseWriterMock) Before(f func(ResponseWriter)) {}

func (t testAccessLoggerResponseWriterMock) WriteHeader(statusCode int) {}

func (t testAccessLoggerResponseWriterMock) Write(bytes []byte) (int, error) {
	return 0, nil
}

func (t testAccessLoggerResponseWriterMock) Flush() {}

func (t testAccessLoggerResponseWriterMock) WrittenStatus() int {
	return http.StatusOK
}

func (t testAccessLoggerResponseWriterMock) WrittenSize() int {
	return 123123
}

func TestAccessLogger(t *testing.T) {
	setting.Log.AccessLogTemplate = `{{.Ctx.RemoteHost}} - {{.Identity}} {{.Start.Format "[02/Jan/2006:15:04:05 -0700]" }} "{{.Ctx.Req.Method}} {{.Ctx.Req.URL.RequestURI}} {{.Ctx.Req.Proto}}" {{.ResponseWriter.Status}} {{.ResponseWriter.Size}} "{{.Ctx.Req.Referer}}" "{{.Ctx.Req.UserAgent}}"`
	recorder := newAccessLogRecorder()
	mockLogger := &testAccessLoggerMock{}
	recorder.logger = mockLogger
	req := &http.Request{
		RemoteAddr: "remote-addr",
		Method:     http.MethodGet,
		Proto:      "https",
		URL:        &url.URL{Path: "/path"},
	}
	req.Header = http.Header{}
	req.Header.Add("Referer", "referer")
	req.Header.Add("User-Agent", "user-agent")
	recorder.record(time.Date(2000, 1, 2, 3, 4, 5, 0, time.UTC), &testAccessLoggerResponseWriterMock{}, req)
	assert.Equal(t, []string{`remote-addr - - [02/Jan/2000:03:04:05 +0000] "GET /path https" 200 123123 "referer" "user-agent"`}, mockLogger.logs)

	mockLogger.logs = nil
	req.URL.RawQuery = "page=2&token=secret"
	recorder.record(time.Date(2000, 1, 2, 3, 4, 5, 0, time.UTC), &testAccessLoggerResponseWriterMock{}, req)
	assert.Equal(t, []string{`remote-addr - - [02/Jan/2000:03:04:05 +0000] "GET /path?page=2&token=(masked) https" 200 123123 "referer" "user-agent"`}, mockLogger.logs)
	assert.Equal(t, "page=2&token=secret", req.URL.RawQuery, "the request itself keeps its query")
}

func TestAccessLoggerRequestID(t *testing.T) {
	assert.False(t, isSafeRequestID("\x00"))
	assert.True(t, isSafeRequestID("a b-c"))
}
