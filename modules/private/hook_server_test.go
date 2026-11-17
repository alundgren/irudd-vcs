// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package private

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"gitea.dev/modules/git"
	"gitea.dev/modules/json"
	"gitea.dev/modules/repository"
	"gitea.dev/modules/setting"
	"gitea.dev/modules/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunServerPush(t *testing.T) {
	var paths []string
	var received []HookOptions
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var opts HookOptions
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&opts))
		paths = append(paths, r.URL.Path)
		received = append(received, opts)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	defer test.MockVariableValue(&setting.LocalURL, srv.URL+"/")()
	defer test.MockVariableValue(&setting.InternalToken, "token")()

	env := []string{
		repository.EnvRepoUsername + "=alice", repository.EnvRepoName + "=demo",
		repository.EnvPusherID + "=1", repository.EnvPusherName + "=alice", repository.EnvPRID + "=7",
		repository.EnvPushTrigger + "=" + string(repository.PushTriggerPRMergeToBase), "SSH_ORIGINAL_COMMAND=gitea-internal",
	}
	// stands in for git, whose post-receive hook writes the refs it updated
	hook := func(refs string, pushErr error) func(env []string) error {
		return func(env []string) error {
			var file string
			for _, kv := range env {
				if v, ok := strings.CutPrefix(kv, repository.EnvPostReceiveFile+"="); ok {
					file = v
				}
			}
			require.NotEmpty(t, file)
			require.NoError(t, os.WriteFile(file, []byte(refs), 0o600))
			return pushErr
		}
	}

	require.NoError(t, RunServerPush(t.Context(), env, hook("a1 b1 refs/heads/main\na2 b2 refs/tags/v1\n", nil)))
	require.Len(t, received, 1)
	assert.Equal(t, "/api/internal/hook/post-receive/alice/demo", paths[0])
	assert.Equal(t, []string{"a1", "a2"}, received[0].OldCommitIDs)
	assert.Equal(t, []string{"b1", "b2"}, received[0].NewCommitIDs)
	assert.Equal(t, []git.RefName{"refs/heads/main", "refs/tags/v1"}, received[0].RefFullNames)
	assert.EqualValues(t, 1, received[0].UserID)
	assert.Equal(t, "alice", received[0].UserName)
	assert.EqualValues(t, 7, received[0].PullRequestID)
	assert.Equal(t, repository.PushTriggerPRMergeToBase, received[0].PushTrigger)

	// the refs a failed push updated are handled, and the push error stays
	pushErr := errors.New("rejected")
	assert.Equal(t, pushErr, RunServerPush(t.Context(), env, hook("a3 b3 refs/heads/other\n", pushErr)))
	require.Len(t, received, 2)

	// nothing updated, or a hook that started gitea itself, leaves the file empty
	require.NoError(t, RunServerPush(t.Context(), env, hook("", nil)))
	// the hook command does nothing for an internal push
	require.NoError(t, RunServerPush(t.Context(), append(env, repository.EnvIsInternal+"=true"), hook("a4 b4 refs/heads/main\n", nil)))
	assert.Len(t, received, 2)
}
