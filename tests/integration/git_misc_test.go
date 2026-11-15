// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"bytes"
	"io"
	"net/url"
	"sync"
	"testing"

	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/gitrepo"
	files_service "gitea.dev/services/repository/files"

	"github.com/stretchr/testify/assert"
)

func TestDataAsyncDoubleRead_Issue29101(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, u *url.URL) {
		user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
		repo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})

		testContent := bytes.Repeat([]byte{'a'}, 10000)
		resp, err := files_service.ChangeRepoFiles(t.Context(), repo, user, &files_service.ChangeRepoFilesOptions{
			Files: []*files_service.ChangeRepoFile{
				{
					Operation:     "create",
					TreePath:      "test.txt",
					ContentReader: bytes.NewReader(testContent),
				},
			},
			OldBranch: repo.DefaultBranch,
			NewBranch: repo.DefaultBranch,
		})
		assert.NoError(t, err)

		sha := resp.Commit.SHA

		gitRepo, err := gitrepo.OpenRepository(t.Context(), repo)
		assert.NoError(t, err)

		commit, err := gitRepo.GetCommit(sha)
		assert.NoError(t, err)

		entry, err := commit.GetTreeEntryByPath("test.txt")
		assert.NoError(t, err)

		b := entry.Blob()
		r1, err := b.DataAsync()
		assert.NoError(t, err)
		defer r1.Close()
		r2, err := b.DataAsync()
		assert.NoError(t, err)
		defer r2.Close()

		var data1, data2 []byte
		wg := sync.WaitGroup{}
		wg.Go(func() {
			data1, _ = io.ReadAll(r1)
			assert.NoError(t, err)
		})
		wg.Go(func() {
			data2, _ = io.ReadAll(r2)
			assert.NoError(t, err)
		})
		wg.Wait()
		assert.Equal(t, testContent, data1)
		assert.Equal(t, testContent, data2)
	})
}
