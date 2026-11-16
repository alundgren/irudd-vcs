// Copyright 2022 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package user

import (
	"strings"
)

const (
	GhostUserID   int64 = -1
	GhostUserName       = "Ghost"
)

// NewGhostUser creates and returns a fake user for someone has deleted their account.
func NewGhostUser() *User {
	return &User{
		ID:        GhostUserID,
		Name:      GhostUserName,
		LowerName: strings.ToLower(GhostUserName),
	}
}

// IsGhost check if user is fake user for a deleted account
func (u *User) IsGhost() bool {
	if u == nil {
		return false
	}
	return u.ID == GhostUserID && u.Name == GhostUserName
}

func GetSystemUserByName(name string) *User {
	if strings.EqualFold(name, GhostUserName) {
		return NewGhostUser()
	}
	return nil
}
