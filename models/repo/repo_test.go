// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"testing"

	"gitea.dev/models/unittest"
	"gitea.dev/modules/optional"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	countRepospts        = CountRepositoryOptions{OwnerID: 10}
	countReposptsPublic  = CountRepositoryOptions{OwnerID: 10, Private: optional.Some(false)}
	countReposptsPrivate = CountRepositoryOptions{OwnerID: 10, Private: optional.Some(true)}
)

func TestGetRepositoryCount(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	ctx := t.Context()
	count, err1 := CountRepositories(ctx, countRepospts)
	privateCount, err2 := CountRepositories(ctx, countReposptsPrivate)
	publicCount, err3 := CountRepositories(ctx, countReposptsPublic)
	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.NoError(t, err3)
	assert.Equal(t, int64(3), count)
	assert.Equal(t, privateCount+publicCount, count)
}

func TestGetPublicRepositoryCount(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	count, err := CountRepositories(t.Context(), countReposptsPublic)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestGetPrivateRepositoryCount(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	count, err := CountRepositories(t.Context(), countReposptsPrivate)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

func TestRepoAPIURL(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())
	repo := unittest.AssertExistsAndLoadBean(t, &Repository{ID: 10})

	assert.Equal(t, "https://try.gitea.io/api/v1/repos/user12/repo10", repo.APIURL())
}

func TestGetRepositoryByURL(t *testing.T) {
	assert.NoError(t, unittest.PrepareTestDatabase())

	t.Run("InvalidPath", func(t *testing.T) {
		repo, err := GetRepositoryByURL(t.Context(), "something")
		assert.Nil(t, repo)
		assert.Error(t, err)
	})

	testRepo2 := func(t *testing.T, url string) {
		repo, err := GetRepositoryByURL(t.Context(), url)
		require.NoError(t, err)
		assert.EqualValues(t, 2, repo.ID)
		assert.EqualValues(t, 2, repo.OwnerID)
	}

	t.Run("ValidHttpURL", func(t *testing.T) {
		testRepo2(t, "https://try.gitea.io/user2/repo2")
		testRepo2(t, "https://try.gitea.io/user2/repo2.git")
	})

	t.Run("ValidGitSshURL", func(t *testing.T) {
		testRepo2(t, "git+ssh://sshuser@try.gitea.io/user2/repo2")
		testRepo2(t, "git+ssh://sshuser@try.gitea.io/user2/repo2.git")

		testRepo2(t, "git+ssh://try.gitea.io/user2/repo2")
		testRepo2(t, "git+ssh://try.gitea.io/user2/repo2.git")
	})

	t.Run("ValidImplicitSshURL", func(t *testing.T) {
		testRepo2(t, "sshuser@try.gitea.io:user2/repo2")
		testRepo2(t, "sshuser@try.gitea.io:user2/repo2.git")

		testRelax := func(t *testing.T, url string) {
			repo, err := GetRepositoryByURLRelax(t.Context(), url)
			require.NoError(t, err)
			assert.Equal(t, int64(2), repo.ID)
			assert.Equal(t, int64(2), repo.OwnerID)
		}
		// TODO: it doesn't seem to be common git ssh URL, should we really support this?
		testRelax(t, "try.gitea.io:user2/repo2")
		testRelax(t, "try.gitea.io:user2/repo2.git")
	})
}

func TestIsUsableRepoName(t *testing.T) {
	assert.NoError(t, IsUsableRepoName("a"))
	assert.NoError(t, IsUsableRepoName("-1_."))
	assert.NoError(t, IsUsableRepoName(".profile"))

	assert.Error(t, IsUsableRepoName("-"))
	assert.Error(t, IsUsableRepoName("🌞"))
	assert.Error(t, IsUsableRepoName("the/repo"))
	assert.Error(t, IsUsableRepoName("the..repo"))
	assert.Error(t, IsUsableRepoName("foo.wiki"))
	assert.Error(t, IsUsableRepoName("foo.git"))
	assert.Error(t, IsUsableRepoName("foo.RSS"))
}
