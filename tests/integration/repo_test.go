// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"os"
	"testing"

	"gitea.dev/models/db"
	issues_model "gitea.dev/models/issues"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/util"
	repo_service "gitea.dev/services/repository"
	"gitea.dev/tests"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateRepository the test cannot succeed when moved as a unit test
func TestGenerateRepository(t *testing.T) {
	defer tests.PrepareTestEnv(t)()

	// a successful generate from template
	user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
	repo44 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 44})

	tmplRepoLabels := []*issues_model.Label{
		{RepoID: 44, Name: "priority/high", Exclusive: true, ExclusiveOrder: 2, Color: "#ee0000", Description: "desc-high"},
		{RepoID: 44, Name: "priority/low", Exclusive: true, ExclusiveOrder: 1, Color: "#0000ee", Description: "desc-low"},
	}

	require.NoError(t, issues_model.NewLabels(t.Context(), tmplRepoLabels...))

	generatedRepo, err := repo_service.GenerateRepository(t.Context(), user2, user2, repo44, repo_service.GenerateRepoOptions{
		Name:        "generated-from-template-44",
		GitContent:  true,
		IssueLabels: true,
	})
	require.NoError(t, err)
	require.NotNil(t, generatedRepo)

	exist, err := util.IsExist(repo_model.RepoPath(user2.Name, generatedRepo.Name))
	require.NoError(t, err)
	require.True(t, exist)

	unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{OwnerName: user2.Name, Name: generatedRepo.Name})

	generatedLabels, err := issues_model.GetLabelsByRepoID(t.Context(), generatedRepo.ID, "", db.ListOptions{})
	require.NoError(t, err)
	require.Len(t, generatedLabels, len(tmplRepoLabels))
	for i, tmplLabel := range tmplRepoLabels {
		genLabel := generatedLabels[i]
		assert.Equal(t, tmplLabel.Name, genLabel.Name)
		assert.Equal(t, tmplLabel.Exclusive, genLabel.Exclusive)
		assert.Equal(t, tmplLabel.ExclusiveOrder, genLabel.ExclusiveOrder)
		assert.Equal(t, tmplLabel.Color, genLabel.Color)
		assert.Equal(t, tmplLabel.Description, genLabel.Description)
	}

	err = repo_service.DeleteRepositoryDirectly(t.Context(), generatedRepo.ID)
	assert.NoError(t, err)

	// a failed creating because some mock data
	// create the repository directory so that the creation will fail after database record created.
	assert.NoError(t, os.MkdirAll(repo_model.RepoPath(user2.Name, "generated-from-template-44"), os.ModePerm))

	generatedRepo2, err := repo_service.GenerateRepository(t.Context(), user2, user2, repo44, repo_service.GenerateRepoOptions{
		Name:       "generated-from-template-44",
		GitContent: true,
	})
	assert.Nil(t, generatedRepo2)
	assert.Error(t, err)

	// assert the cleanup is successful
	unittest.AssertNotExistsBean(t, &repo_model.Repository{OwnerName: user2.Name, Name: generatedRepo.Name})

	exist, err = util.IsExist(repo_model.RepoPath(user2.Name, generatedRepo.Name))
	assert.NoError(t, err)
	assert.False(t, exist)
}
