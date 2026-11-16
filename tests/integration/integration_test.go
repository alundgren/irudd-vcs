// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"bytes"
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"gitea.dev/models/auth"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/graceful"
	"gitea.dev/modules/json"
	"gitea.dev/modules/log"
	"gitea.dev/modules/setting"
	"gitea.dev/modules/testlogger"
	"gitea.dev/modules/web"
	"gitea.dev/routers"
	"gitea.dev/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testWebRoutes *web.Router

func testMain(m *testing.M) int {
	defer log.GetManager().Close()

	managerCtx, cancel := context.WithCancel(context.Background())
	graceful.InitManager(managerCtx)
	defer cancel()

	err := tests.InitIntegrationTest()
	if err != nil {
		return testlogger.MainErrorf("InitTest error: %v", err)
	}
	testWebRoutes = routers.NormalRoutes()

	err = unittest.InitFixtures(
		unittest.FixturesOptions{
			Dir: filepath.Join(setting.GetGiteaTestSourceRoot(), "models/fixtures/"),
		},
	)
	if err != nil {
		return testlogger.MainErrorf("InitFixtures: %v", err)
	}

	// FIXME: the console logger is deleted by mistake, so if there is any `log.Fatal`, developers won't see any error message.
	// Instead, "No tests were found",  last nonsense log is "According to the configuration, subsequent logs will not be printed to the console"
	exitCode := m.Run()

	return exitCode
}

func TestMain(m *testing.M) {
	// -test.list must skip InitIntegrationTest, which requires a database.
	flag.Parse()
	if flag.Lookup("test.list").Value.String() != "" {
		os.Exit(m.Run())
	}
	os.Exit(testMain(m))
}

// TestSession sends requests as a user with an all-scope token, the server has no sessions or passwords
type TestSession struct {
	userName string
	token    string
}

func (s *TestSession) MakeRequest(t testing.TB, rw *RequestWrapper, expectedStatus int) *httptest.ResponseRecorder {
	t.Helper()
	if s != nil && s.userName != "" && rw.Header.Get("Authorization") == "" {
		if s.token == "" {
			s.token = getTokenForLoggedInUser(t, s, auth.AccessTokenScopeAll)
		}
		rw.SetBasicAuth(s.userName, s.token) // like git clients, so the git and LFS routes accept it too
	}
	return MakeRequest(t, rw, expectedStatus)
}

// gitUserInfo is the userinfo of a git remote URL, with an all-scope token as the password
func gitUserInfo(t testing.TB, userName string) *url.Userinfo {
	return url.UserPassword(userName, getUserToken(t, userName, auth.AccessTokenScopeAll))
}

func emptyTestSession(t testing.TB) *TestSession {
	t.Helper()
	return &TestSession{}
}

func getUserToken(t testing.TB, userName string, scope ...auth.AccessTokenScope) string {
	return getTokenForLoggedInUser(t, loginUser(t, userName), scope...)
}

func loginUser(t testing.TB, userName string) *TestSession {
	t.Helper()
	return &TestSession{userName: userName}
}

// token has to be unique this counter take care of
var tokenCounter atomic.Int64

// getTokenForLoggedInUser returns a token for a logged-in user.
// It is created directly because the API refuses scope-less tokens, which some tests rely on.
func getTokenForLoggedInUser(t testing.TB, session *TestSession, scopes ...auth.AccessTokenScope) string {
	t.Helper()
	scopeNames := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scopeNames = append(scopeNames, string(scope))
	}
	scope, err := auth.AccessTokenScope(strings.Join(scopeNames, ",")).Normalize()
	require.NoError(t, err)
	u := unittest.AssertExistsAndLoadBean(t, &user_model.User{LowerName: strings.ToLower(session.userName)})
	token := &auth.AccessToken{
		UID:   u.ID,
		Name:  fmt.Sprintf("api-testing-token-%d", tokenCounter.Add(1)),
		Scope: scope,
	}
	require.NoError(t, auth.NewAccessToken(t.Context(), token))
	return token.Token
}

type RequestWrapper struct {
	*http.Request
}

// AddBasicAuth sends a token as the basic auth password, like git clients do
func (req *RequestWrapper) AddBasicAuth(username, token string) *RequestWrapper {
	req.Request.SetBasicAuth(username, token)
	return req
}

func (req *RequestWrapper) AddTokenAuth(token string) *RequestWrapper {
	if token == "" {
		return req
	}
	if !strings.HasPrefix(token, "Bearer ") {
		token = "Bearer " + token
	}
	req.Request.Header.Set("Authorization", token)
	return req
}

func (req *RequestWrapper) SetHeader(name, value string) *RequestWrapper {
	req.Request.Header.Set(name, value)
	return req
}

func NewRequest(t testing.TB, method, urlStr string) *RequestWrapper {
	t.Helper()
	return NewRequestWithBody(t, method, urlStr, nil)
}

func NewRequestf(t testing.TB, method, urlFormat string, args ...any) *RequestWrapper {
	t.Helper()
	return NewRequest(t, method, fmt.Sprintf(urlFormat, args...))
}

func NewRequestWithValues(t testing.TB, method, urlStr string, values map[string]string) *RequestWrapper {
	t.Helper()
	urlValues := url.Values{}
	for key, value := range values {
		urlValues[key] = []string{value}
	}
	return NewRequestWithURLValues(t, method, urlStr, urlValues)
}

func NewRequestWithURLValues(t testing.TB, method, urlStr string, urlValues url.Values) *RequestWrapper {
	t.Helper()
	return NewRequestWithBody(t, method, urlStr, strings.NewReader(urlValues.Encode())).
		SetHeader("Content-Type", "application/x-www-form-urlencoded")
}

func NewRequestWithJSON(t testing.TB, method, urlStr string, v any) *RequestWrapper {
	t.Helper()

	jsonBytes, err := json.Marshal(v)
	assert.NoError(t, err)
	return NewRequestWithBody(t, method, urlStr, bytes.NewBuffer(jsonBytes)).
		SetHeader("Content-Type", "application/json")
}

func NewRequestWithBody(t testing.TB, method, urlStr string, body io.Reader) *RequestWrapper {
	t.Helper()
	if !strings.HasPrefix(urlStr, "http:") && !strings.HasPrefix(urlStr, "https:") && !strings.HasPrefix(urlStr, "/") {
		t.Fatalf("invalid url str: %s", urlStr)
	}
	req, err := http.NewRequest(method, urlStr, body)
	assert.NoError(t, err)
	if req.URL.User != nil {
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(req.URL.User.String())))
	}
	req.RequestURI = req.URL.Path
	if req.URL.RawQuery != "" {
		req.RequestURI += "?" + req.URL.RawQuery
	}
	return &RequestWrapper{req}
}

const NoExpectedStatus = 0

func MakeRequest(t testing.TB, rw *RequestWrapper, expectedStatus int) *httptest.ResponseRecorder {
	t.Helper()
	req := rw.Request
	recorder := httptest.NewRecorder()
	if req.RemoteAddr == "" {
		req.RemoteAddr = "test-mock:12345"
	}
	// Ensure unknown contentLength is seen as -1
	if req.Body != nil && req.ContentLength == 0 {
		req.ContentLength = -1
	}
	testWebRoutes.ServeHTTP(recorder, req)
	if expectedStatus != NoExpectedStatus {
		if expectedStatus != recorder.Code {
			logUnexpectedResponse(t, recorder)
			// don't use "require" which exits the test case and makes "wait group" wait forever
			assert.Equal(t, expectedStatus, recorder.Code, "Request: %s %s", req.Method, req.URL.String())
		}
	}
	return recorder
}

// logUnexpectedResponse logs the contents of an unexpected response.
func logUnexpectedResponse(t testing.TB, recorder *httptest.ResponseRecorder) {
	t.Helper()
	respBytes := recorder.Body.Bytes()
	if len(respBytes) == 0 {
		return
	} else if len(respBytes) < 500 {
		// if body is short, just log the whole thing
		t.Log("Response: ", string(respBytes))
		return
	}
	t.Log("Response length: ", len(respBytes))
}

// DecodeJSON decodes then response as JSON into typed variable and return it
// HINT: don't use it on existing variable (reuse existing variable):
// if the existing var already contains some values but the new input doesn't, then it leads to wrong test result in edge cases.
// For slice decoding, use: v := DecodeJSON(t, resp, []T{})
func DecodeJSON[T any](t testing.TB, resp *httptest.ResponseRecorder, v T) (ret T) {
	t.Helper()

	// FIXME: JSON-KEY-CASE: for testing purpose only, because many structs don't provide `json` tags, they just use capitalized field names
	decoder := json.NewDecoderCaseInsensitive(resp.Body)
	// don't use "require" which exits the test case and makes "wait group" wait forever
	assert.NoError(t, decoder.Decode(&v))
	return v
}
