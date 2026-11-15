// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package webhook

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"gitea.dev/models/unittest"
	webhook_model "gitea.dev/models/webhook"
	"gitea.dev/modules/hostmatcher"
	"gitea.dev/modules/setting"
	api "gitea.dev/modules/structs"
	"gitea.dev/modules/util"
	webhook_module "gitea.dev/modules/webhook"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebhookProxy(t *testing.T) {
	oldWebhook := setting.Webhook
	t.Cleanup(func() {
		setting.Webhook = oldWebhook
	})

	setting.Webhook.ProxyURL = "http://localhost:8080"
	setting.Webhook.ProxyURLFixed, _ = url.Parse(setting.Webhook.ProxyURL)
	setting.Webhook.ProxyHosts = []string{"*.discordapp.com", "discordapp.com"}

	allowedHostMatcher := hostmatcher.ParseHostMatchList("webhook.ALLOWED_HOST_LIST", "discordapp.com,s.discordapp.com")

	tests := []struct {
		req     string
		want    string
		wantErr bool
	}{
		{
			req:     "https://discordapp.com/api/webhooks/xxxxxxxxx/xxxxxxxxxxxxxxxxxxx",
			want:    "http://localhost:8080",
			wantErr: false,
		},
		{
			req:     "http://s.discordapp.com/assets/xxxxxx",
			want:    "http://localhost:8080",
			wantErr: false,
		},
		{
			req:     "http://github.com/a/b",
			want:    "",
			wantErr: false,
		},
		{
			req:     "http://www.discordapp.com/assets/xxxxxx",
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.req, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, tt.req, nil)
			require.NoError(t, err)

			u, err := webhookProxy(allowedHostMatcher)(req)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			got := ""
			if u != nil {
				got = u.String()
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWebhookDeliverAuthorizationHeader(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	done := make(chan struct{}, 1)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/webhook", r.URL.Path)
		assert.Equal(t, "Bearer s3cr3t-t0ken", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		done <- struct{}{}
	}))
	t.Cleanup(s.Close)

	hook := &webhook_model.Webhook{
		RepoID:      3,
		URL:         s.URL + "/webhook",
		ContentType: webhook_model.ContentTypeJSON,
		IsActive:    true,
		Type:        webhook_module.GITEA,
	}
	err := hook.SetHeaderAuthorization("Bearer s3cr3t-t0ken")
	assert.NoError(t, err)
	assert.NoError(t, webhook_model.CreateWebhook(t.Context(), hook))

	hookTask := &webhook_model.HookTask{
		HookID:         hook.ID,
		EventType:      webhook_module.HookEventPush,
		PayloadVersion: 2,
	}

	hookTask, err = webhook_model.CreateHookTask(t.Context(), hookTask)
	assert.NoError(t, err)
	assert.NotNil(t, hookTask)

	assert.NoError(t, Deliver(t.Context(), hookTask))
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("waited to long for request to happen")
	}

	assert.True(t, hookTask.IsSucceed)
	assert.Equal(t, "******", hookTask.RequestInfo.Headers["Authorization"])
}

func TestWebhookDeliverHookTask(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	done := make(chan struct{}, 1)
	var version2Body string

	testVersion := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/webhook", r.URL.Path)
		switch testVersion {
		case 1: // Version 1
			assert.Equal(t, "push", r.Header.Get("X-GitHub-Event"))
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			body, err := io.ReadAll(r.Body)
			assert.NoError(t, err)
			assert.Equal(t, `{"data": 42}`, string(body))

		case 2: // Version 2
			assert.Equal(t, "push", r.Header.Get("X-GitHub-Event"))
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			body, err := io.ReadAll(r.Body)
			assert.NoError(t, err)
			assert.JSONEq(t, version2Body, string(body))

		default:
			w.WriteHeader(http.StatusNotFound)
			t.Fatalf("unexpected url path %s", r.URL.Path)
			return
		}
		w.WriteHeader(http.StatusOK)
		done <- struct{}{}
	}))
	t.Cleanup(s.Close)

	hook := &webhook_model.Webhook{
		RepoID:      3,
		IsActive:    true,
		Type:        webhook_module.GITEA,
		URL:         s.URL + "/webhook",
		HTTPMethod:  "POST",
		ContentType: webhook_model.ContentTypeJSON,
	}
	assert.NoError(t, webhook_model.CreateWebhook(t.Context(), hook))

	t.Run("Version 1", func(t *testing.T) {
		testVersion = 1
		hookTask := &webhook_model.HookTask{
			HookID:         hook.ID,
			EventType:      webhook_module.HookEventPush,
			PayloadContent: `{"data": 42}`,
			PayloadVersion: 1,
		}

		hookTask, err := webhook_model.CreateHookTask(t.Context(), hookTask)
		assert.NoError(t, err)
		assert.NotNil(t, hookTask)

		assert.NoError(t, Deliver(t.Context(), hookTask))
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("waited to long for request to happen")
		}

		assert.True(t, hookTask.IsSucceed)
	})

	t.Run("Version 2", func(t *testing.T) {
		p := pushTestPayload()
		data, err := p.JSONPayload()
		assert.NoError(t, err)
		version2Body = string(data) // the gitea type delivers the payload unchanged

		testVersion = 2
		hookTask := &webhook_model.HookTask{
			HookID:         hook.ID,
			EventType:      webhook_module.HookEventPush,
			PayloadContent: string(data),
			PayloadVersion: 2,
		}

		hookTask, err = webhook_model.CreateHookTask(t.Context(), hookTask)
		assert.NoError(t, err)
		assert.NotNil(t, hookTask)

		assert.NoError(t, Deliver(t.Context(), hookTask))
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("waited to long for request to happen")
		}

		assert.True(t, hookTask.IsSucceed)
	})
}

func TestWebhookDeliverSpecificTypes(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	type hookCase struct {
		gotBody    chan []byte
		httpMethod string // default to POST
	}

	cases := map[string]*hookCase{
		webhook_module.GITEA: {},
		webhook_module.GOGS:  {},
	}

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		typ := strings.Split(r.URL.Path, "/")[1] // URL: "/{webhook_type}/other-path"
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"), r.URL.Path)
		assert.Equal(t, util.IfZero(cases[typ].httpMethod, "POST"), r.Method, "webhook test request %q", r.URL.Path)
		body, _ := io.ReadAll(r.Body) // read request and send it back to the test by testcase's chan
		cases[typ].gotBody <- body
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(s.Close)

	p := pushTestPayload()
	data, err := p.JSONPayload()
	assert.NoError(t, err)

	for typ := range cases {
		cases[typ].gotBody = make(chan []byte, 1)
		t.Run(typ, func(t *testing.T) {
			t.Parallel()
			hook := &webhook_model.Webhook{
				RepoID:      3,
				IsActive:    true,
				Type:        typ,
				URL:         s.URL + "/" + typ,
				ContentType: webhook_model.ContentTypeJSON,
			}
			assert.NoError(t, webhook_model.CreateWebhook(t.Context(), hook))

			hookTask := &webhook_model.HookTask{
				HookID:         hook.ID,
				EventType:      webhook_module.HookEventPush,
				PayloadContent: string(data),
				PayloadVersion: 2,
			}

			hookTask, err := webhook_model.CreateHookTask(t.Context(), hookTask)
			assert.NoError(t, err)
			assert.NotNil(t, hookTask)

			assert.NoError(t, Deliver(t.Context(), hookTask))

			select {
			case gotBody := <-cases[typ].gotBody:
				assert.Equal(t, string(data), string(gotBody), "the remaining hook types deliver the event payload unchanged")
				assert.Equal(t, hookTask.RequestInfo.Body, string(gotBody), "delivered webhook payload doesn't match saved request")
			case <-time.After(5 * time.Second):
				t.Fatal("waited to long for request to happen")
			}

			assert.True(t, hookTask.IsSucceed)
		})
	}
}

func pushTestPayload() *api.PushPayload {
	commit := &api.PayloadCommit{
		ID:      "2020558fe2e34debb818a514715839cabd25e778",
		Message: "commit message",
		URL:     "http://localhost:3000/test/repo/commit/2020558fe2e34debb818a514715839cabd25e778",
		Author: &api.PayloadUser{
			Name:     "user1",
			Email:    "user1@localhost",
			UserName: "user1",
		},
		Committer: &api.PayloadUser{
			Name:     "user1",
			Email:    "user1@localhost",
			UserName: "user1",
		},
	}

	return &api.PushPayload{
		Ref:          "refs/heads/test",
		Before:       "2020558fe2e34debb818a514715839cabd25e777",
		After:        "2020558fe2e34debb818a514715839cabd25e778",
		CompareURL:   "",
		HeadCommit:   commit,
		Commits:      []*api.PayloadCommit{commit, commit},
		TotalCommits: 2,
		Repo: &api.Repository{
			HTMLURL:  "http://localhost:3000/test/repo",
			Name:     "repo",
			FullName: "test/repo",
		},
		Pusher: &api.User{
			UserName: "user1",
		},
		Sender: &api.User{
			UserName: "user1",
		},
	}
}
