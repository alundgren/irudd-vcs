// Copyright 2016 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package structs

import (
	"time"
)

// Release represents a repository release
type Release struct {
	// The unique identifier of the release
	ID int64 `json:"id"`
	// The name of the git tag associated with the release
	TagName string `json:"tag_name"`
	// The target commitish for the release
	Target string `json:"target_commitish"`
	// The display title of the release
	Title string `json:"name"`
	// The release notes or description
	Note string `json:"body"`
	// The API URL of the release
	URL string `json:"url"`
	// The HTML URL to view the release
	HTMLURL string `json:"html_url"`
	// Whether the release is a draft
	IsDraft bool `json:"draft"`
	// Whether the release is a prerelease
	IsPrerelease bool `json:"prerelease"`
	// swagger:strfmt date-time
	CreatedAt time.Time `json:"created_at"`
	// swagger:strfmt date-time
	PublishedAt time.Time `json:"published_at"`
	// The user who published the release
	Publisher *User `json:"author"`
}
