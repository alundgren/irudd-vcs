// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import "gitea.dev/models/db"

// OrderByMap represents all possible search order
var OrderByMap = map[string]map[string]db.SearchOrderBy{
	"asc": {
		"alpha":    "owner_name ASC, name ASC",
		"created":  db.SearchOrderByOldest,
		"updated":  db.SearchOrderByLeastUpdated,
		"size":     "size ASC",
		"git_size": "git_size ASC",
		"lfs_size": "lfs_size ASC",
		"id":       db.SearchOrderByID,
		"stars":    db.SearchOrderByStars,
		"forks":    db.SearchOrderByForks,
	},
	"desc": {
		"alpha":    "owner_name DESC, name DESC",
		"created":  db.SearchOrderByNewest,
		"updated":  db.SearchOrderByRecentUpdated,
		"size":     "size DESC",
		"git_size": "git_size DESC",
		"lfs_size": "lfs_size DESC",
		"id":       db.SearchOrderByIDReverse,
		"stars":    db.SearchOrderByStarsReverse,
		"forks":    db.SearchOrderByForksReverse,
	},
}
