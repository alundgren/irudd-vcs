// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"net/http"
	"net/url"
	"testing"

	"gitea.dev/models/db"
	git_model "gitea.dev/models/git"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/git/gitcmd"
	api "gitea.dev/modules/structs"
	"gitea.dev/services/release"
	"gitea.dev/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateNewTagProtected(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
	owner := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: repo.OwnerID})

	t.Run("Code", func(t *testing.T) {
		defer tests.PrintCurrentTest(t)()

		err := release.CreateNewTag(t.Context(), owner, repo, "master", "t-first", "first tag")
		assert.NoError(t, err)

		err = release.CreateNewTag(t.Context(), owner, repo, "master", "v-2", "second tag")
		assert.Error(t, err)
		assert.True(t, release.IsErrProtectedTagName(err))

		err = release.CreateNewTag(t.Context(), owner, repo, "master", "v-1.1", "third tag")
		assert.NoError(t, err)
	})

	t.Run("Git", func(t *testing.T) {
		onGiteaRun(t, func(t *testing.T, u *url.URL) {
			httpContext := NewAPITestContext(t, owner.Name, repo.Name)

			dstPath := t.TempDir()

			u.Path = httpContext.GitPath()
			u.User = url.UserPassword(owner.Name, userPassword)

			doGitClone(dstPath, u)(t)

			_, _, err := gitcmd.NewCommand("tag", "v-2").WithDir(dstPath).RunStdString(t.Context())
			assert.NoError(t, err)

			_, _, err = gitcmd.NewCommand("push", "--tags").WithDir(dstPath).RunStdString(t.Context())
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "Tag v-2 is protected")
		})
	})

	t.Run("GitTagForce", func(t *testing.T) {
		onGiteaRun(t, func(t *testing.T, u *url.URL) {
			httpContext := NewAPITestContext(t, owner.Name, repo.Name)

			dstPath := t.TempDir()

			u.Path = httpContext.GitPath()
			u.User = url.UserPassword(owner.Name, userPassword)

			doGitClone(dstPath, u)(t)

			_, _, err := gitcmd.NewCommand("tag", "v-1.1", "-m", "force update", "--force").
				WithDir(dstPath).
				RunStdString(t.Context())
			require.NoError(t, err)

			_, _, err = gitcmd.NewCommand("push", "--tags").WithDir(dstPath).RunStdString(t.Context())
			require.NoError(t, err)

			_, _, err = gitcmd.NewCommand("tag", "v-1.1", "-m", "force update v2", "--force").WithDir(dstPath).RunStdString(t.Context())
			require.NoError(t, err)

			_, _, err = gitcmd.NewCommand("push", "--tags").WithDir(dstPath).RunStdString(t.Context())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "the tag already exists in the remote")

			_, _, err = gitcmd.NewCommand("push", "--tags", "--force").WithDir(dstPath).RunStdString(t.Context())
			require.NoError(t, err)
			req := NewRequestf(t, "GET", "/api/v1/repos/%s/tags/v-1.1", repo.FullName())
			resp := MakeRequest(t, req, http.StatusOK)
			assert.Contains(t, DecodeJSON(t, resp, &api.Tag{}).Message, "force update v2")
		})
	})

	// Cleanup
	releases, err := db.Find[repo_model.Release](t.Context(), repo_model.FindReleasesOptions{
		IncludeTags: true,
		TagNames:    []string{"v-1", "v-1.1"},
		RepoID:      repo.ID,
	})
	assert.NoError(t, err)

	for _, release := range releases {
		_, err = db.DeleteByID[repo_model.Release](t.Context(), release.ID)
		assert.NoError(t, err)
	}

	protectedTags, err := git_model.GetProtectedTags(t.Context(), repo.ID)
	assert.NoError(t, err)

	for _, protectedTag := range protectedTags {
		err = git_model.DeleteProtectedTag(t.Context(), protectedTag)
		assert.NoError(t, err)
	}
}
