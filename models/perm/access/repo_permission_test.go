// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package access

import (
	"testing"

	perm_model "gitea.dev/models/perm"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unit"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHasAnyUnitAccess(t *testing.T) {
	perm := Permission{}
	assert.False(t, perm.HasAnyUnitAccess())

	perm = Permission{
		units: []*repo_model.RepoUnit{{Type: unit.TypeReleases}},
	}
	assert.False(t, perm.HasAnyUnitAccess())

	perm = Permission{
		AccessMode: perm_model.AccessModeRead,
		units:      []*repo_model.RepoUnit{{Type: unit.TypeReleases}},
	}
	assert.True(t, perm.HasAnyUnitAccess())

	perm = Permission{
		unitsMode: map[unit.Type]perm_model.AccessMode{unit.TypeReleases: perm_model.AccessModeRead},
	}
	assert.True(t, perm.HasAnyUnitAccess())
}

func TestUnitAccessMode(t *testing.T) {
	perm := Permission{
		AccessMode: perm_model.AccessModeNone,
	}
	assert.Equal(t, perm_model.AccessModeNone, perm.UnitAccessMode(unit.TypeReleases), "no unit, no map, use AccessMode")

	perm = Permission{
		AccessMode: perm_model.AccessModeRead,
		units: []*repo_model.RepoUnit{
			{Type: unit.TypeReleases},
		},
	}
	assert.Equal(t, perm_model.AccessModeRead, perm.UnitAccessMode(unit.TypeReleases), "only unit, no map, use AccessMode")

	perm = Permission{
		AccessMode: perm_model.AccessModeAdmin,
		unitsMode: map[unit.Type]perm_model.AccessMode{
			unit.TypeReleases: perm_model.AccessModeRead,
		},
	}
	assert.Equal(t, perm_model.AccessModeAdmin, perm.UnitAccessMode(unit.TypeReleases), "no unit, only map, admin overrides map")

	perm = Permission{
		AccessMode: perm_model.AccessModeNone,
		unitsMode: map[unit.Type]perm_model.AccessMode{
			unit.TypeReleases: perm_model.AccessModeRead,
		},
	}
	assert.Equal(t, perm_model.AccessModeRead, perm.UnitAccessMode(unit.TypeReleases), "no unit, only map, use map")

	perm = Permission{
		AccessMode: perm_model.AccessModeNone,
		units: []*repo_model.RepoUnit{
			{Type: unit.TypeReleases},
		},
		unitsMode: map[unit.Type]perm_model.AccessMode{
			unit.TypeReleases: perm_model.AccessModeRead,
		},
	}
	assert.Equal(t, perm_model.AccessModeRead, perm.UnitAccessMode(unit.TypeReleases), "has unit, and map, use map")
}

func TestGetRepoPermission(t *testing.T) {
	require.NoError(t, unittest.PrepareTestDatabase())
	ctx := t.Context()

	privateRepo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 2, IsPrivate: true})
	otherOwnersRepo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 3})
	user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})

	t.Run("AnonymousGetsNothing", func(t *testing.T) {
		for _, doer := range []*user_model.User{nil, user_model.NewGhostUser()} {
			perm, err := GetIndividualUserRepoPermission(ctx, privateRepo, doer)
			require.NoError(t, err)
			assert.False(t, perm.HasAnyUnitAccess())
			assert.False(t, perm.CanRead(unit.TypeCode))
		}
	})

	t.Run("SignedInUserOwnsEveryRepo", func(t *testing.T) {
		for _, repo := range []*repo_model.Repository{privateRepo, otherOwnersRepo} {
			perm, err := GetIndividualUserRepoPermission(ctx, repo, user)
			require.NoError(t, err)
			assert.True(t, perm.IsOwner())
			assert.True(t, perm.CanWrite(unit.TypeCode))
		}
	})

	t.Run("DoerMatchesIndividual", func(t *testing.T) {
		doerPerm, err := GetDoerRepoPermission(ctx, privateRepo, user)
		require.NoError(t, err)
		individualPerm, err := GetIndividualUserRepoPermission(ctx, privateRepo, user)
		require.NoError(t, err)
		assert.Equal(t, individualPerm, doerPerm)
	})
}
