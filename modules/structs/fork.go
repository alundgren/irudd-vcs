// Copyright 2016 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package structs

// CreateForkOption options for creating a fork
type CreateForkOption struct {
	// organization name; there are no organizations, so setting it fails with 422
	Organization *string `json:"organization"`
	// name of the forked repository
	Name *string `json:"name"`
}
