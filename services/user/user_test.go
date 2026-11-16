// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package user

import (
	"strings"
	"testing"
	"time"

	"gitea.dev/models/db"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/timeutil"

	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	unittest.MainTest(m)
}

func TestDeleteUser(t *testing.T) {
	test := func(userID int64) {
		assert.NoError(t, unittest.PrepareTestDatabase())
		user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: userID})

		ownedRepos := make([]*repo_model.Repository, 0, 10)
		assert.NoError(t, db.GetEngine(t.Context()).Find(&ownedRepos, &repo_model.Repository{OwnerID: userID}))
		if len(ownedRepos) > 0 {
			err := DeleteUser(t.Context(), user, false)
			assert.Error(t, err)
			assert.True(t, repo_model.IsErrUserOwnRepos(err))
			return
		}

		assert.NoError(t, DeleteUser(t.Context(), user, false))
		unittest.AssertNotExistsBean(t, &user_model.User{ID: userID})
		unittest.CheckConsistencyFor(t, &user_model.User{}, &repo_model.Repository{})
	}
	test(2)
	test(4)
	test(8)
	test(11)

	org := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 3})
	assert.Error(t, DeleteUser(t.Context(), org, false))

	t.Run("CleanupOrphanedTables", func(t *testing.T) {
		assert.NoError(t, unittest.PrepareTestDatabase())

		// delete users
		user24 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 24})
		assert.NoError(t, DeleteUser(t.Context(), user24, true))

		user32 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 32})
		assert.NoError(t, DeleteUser(t.Context(), user32, true))

		user2 := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 2})
		assert.NoError(t, DeleteUser(t.Context(), user2, true))

		// assert they do not exist after deletion
		unittest.AssertNotExistsBean(t, &user_model.User{ID: 24})
		unittest.AssertNotExistsBean(t, &user_model.User{ID: 32})
		unittest.AssertNotExistsBean(t, &user_model.User{ID: 2})
		unittest.AssertNotExistsBean(t, &user_model.EmailAddress{UID: 2})
	})
}

func TestPurgeUser(t *testing.T) {
	test := func(userID int64) {
		assert.NoError(t, unittest.PrepareTestDatabase())
		user := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: userID})

		err := DeleteUser(t.Context(), user, true)
		assert.NoError(t, err)

		unittest.AssertNotExistsBean(t, &user_model.User{ID: userID})
		unittest.CheckConsistencyFor(t, &user_model.User{}, &repo_model.Repository{})
	}
	test(2)
	test(4)
	test(8)
	test(11)

	org := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 3})
	assert.Error(t, DeleteUser(t.Context(), org, false))
}

func TestCreateUser(t *testing.T) {
	user := &user_model.User{
		Name:    "GiteaBot",
		Email:   "GiteaBot@gitea.io",
		IsAdmin: false,
	}

	assert.NoError(t, user_model.CreateUser(t.Context(), user, &user_model.Meta{}))

	assert.NoError(t, DeleteUser(t.Context(), user, false))
}

func TestDeleteInactiveUsers(t *testing.T) {
	addUser := func(name, email string, createdUnix timeutil.TimeStamp, active bool) {
		inactiveUser := &user_model.User{Name: name, LowerName: strings.ToLower(name), Email: email, CreatedUnix: createdUnix, IsActive: active}
		_, err := db.GetEngine(t.Context()).NoAutoTime().Insert(inactiveUser)
		assert.NoError(t, err)
		inactiveUserEmail := &user_model.EmailAddress{UID: inactiveUser.ID, IsPrimary: true, Email: email, LowerEmail: strings.ToLower(email), IsActivated: active}
		err = db.Insert(t.Context(), inactiveUserEmail)
		assert.NoError(t, err)
	}
	addUser("user-inactive-10", "user-inactive-10@test.com", timeutil.TimeStampNow().Add(-600), false)
	addUser("user-inactive-5", "user-inactive-5@test.com", timeutil.TimeStampNow().Add(-300), false)
	addUser("user-active-10", "user-active-10@test.com", timeutil.TimeStampNow().Add(-600), true)
	addUser("user-active-5", "user-active-5@test.com", timeutil.TimeStampNow().Add(-300), true)
	unittest.AssertExistsAndLoadBean(t, &user_model.User{Name: "user-inactive-10"})
	unittest.AssertExistsAndLoadBean(t, &user_model.EmailAddress{Email: "user-inactive-10@test.com"})
	assert.NoError(t, DeleteInactiveUsers(t.Context(), 8*time.Minute))
	unittest.AssertNotExistsBean(t, &user_model.User{Name: "user-inactive-10"})
	unittest.AssertNotExistsBean(t, &user_model.EmailAddress{Email: "user-inactive-10@test.com"})
	unittest.AssertExistsAndLoadBean(t, &user_model.User{Name: "user-inactive-5"})
	unittest.AssertExistsAndLoadBean(t, &user_model.User{Name: "user-active-10"})
	unittest.AssertExistsAndLoadBean(t, &user_model.User{Name: "user-active-5"})
}
