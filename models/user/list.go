// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package user

import (
	"context"

	"gitea.dev/models/db"
)

// UserList is a list of user.
// This type provide valuable methods to retrieve information for a group of users efficiently.
type UserList []*User //revive:disable-line:exported

// GetUserIDs returns a slice of user's id
func (users UserList) GetUserIDs() []int64 {
	userIDs := make([]int64, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.ID) // Considering that user id are unique in the list
	}
	return userIDs
}

// GetUsersByIDs returns all resolved users from a list of Ids.
func GetUsersByIDs(ctx context.Context, ids []int64) (UserList, error) {
	ous := make([]*User, 0, len(ids))
	if len(ids) == 0 {
		return ous, nil
	}
	err := db.GetEngine(ctx).In("id", ids).
		Asc("name").
		Find(&ous)
	return ous, err
}
