// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package doctor

import (
	"slices"
	"testing"

	"gitea.dev/models/db"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unittest"
	"gitea.dev/modules/log"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsistencyCheck(t *testing.T) {
	checks := prepareDBConsistencyChecks()
	idx := slices.IndexFunc(checks, func(check consistencyCheck) bool {
		return check.Name == "Orphaned Releases without existing repository"
	})
	require.NotEqual(t, -1, idx)

	_ = db.TruncateBeans(t.Context(), &repo_model.Release{}, &repo_model.Repository{})

	err := db.Insert(t.Context(), &repo_model.Repository{ID: 1, Name: "repo1", LowerName: "repo1"})
	assert.NoError(t, err)
	err = db.Insert(t.Context(), &repo_model.Release{TagName: "v1", LowerTagName: "v1", RepoID: 1})
	assert.NoError(t, err)
	err = db.Insert(t.Context(), &repo_model.Release{TagName: "v2", LowerTagName: "v2", RepoID: 99999999})
	assert.NoError(t, err)

	unittest.AssertExistsAndLoadBean(t, &repo_model.Release{TagName: "v1"})
	unittest.AssertExistsAndLoadBean(t, &repo_model.Release{TagName: "v2"})

	releaseCheck := checks[idx]
	err = releaseCheck.Run(t.Context(), log.GetManager().GetLogger(log.DEFAULT), true)
	assert.NoError(t, err)

	unittest.AssertExistsAndLoadBean(t, &repo_model.Release{TagName: "v1"})
	unittest.AssertNotExistsBean(t, &repo_model.Release{TagName: "v2"})
}
