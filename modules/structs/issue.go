// Copyright 2016 The Gogs Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package structs

import (
	"time"
)

// StateType issue state type
//
// swagger:enum StateType
type StateType string

const (
	// StateOpen pr is opened
	StateOpen StateType = "open"
	// StateClosed pr is closed
	StateClosed StateType = "closed"
)

// StateAll is a query parameter filter value, not a valid object state.
const StateAll = "all"

// PullRequestMeta PR info if an issue is a PR
type PullRequestMeta struct {
	HasMerged        bool       `json:"merged"`
	Merged           *time.Time `json:"merged_at"`
	IsWorkInProgress bool       `json:"draft"`
	HTMLURL          string     `json:"html_url"`
}

// RepositoryMeta basic repository information
type RepositoryMeta struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Owner    string `json:"owner"`
	FullName string `json:"full_name"`
}

// Issue represents an issue in a repository
// swagger:model
type Issue struct {
	ID               int64    `json:"id"`
	URL              string   `json:"url"`
	HTMLURL          string   `json:"html_url"`
	Index            int64    `json:"number"`
	Poster           *User    `json:"user"`
	OriginalAuthor   string   `json:"original_author"`
	OriginalAuthorID int64    `json:"original_author_id"`
	Title            string   `json:"title"`
	Body             string   `json:"body"`
	Ref              string   `json:"ref"`
	Labels           []*Label `json:"labels"`
	// deprecated
	Assignee  *User     `json:"assignee"`
	Assignees []*User   `json:"assignees"`
	State     StateType `json:"state"`
	IsLocked  bool      `json:"is_locked"`
	Comments  int       `json:"comments"`
	// swagger:strfmt date-time
	Created time.Time `json:"created_at"`
	// swagger:strfmt date-time
	Updated time.Time `json:"updated_at"`
	// swagger:strfmt date-time
	Closed *time.Time `json:"closed_at"`
	// swagger:strfmt date-time
	Deadline *time.Time `json:"due_date"`

	PullRequest *PullRequestMeta `json:"pull_request"`
	Repo        *RepositoryMeta  `json:"repository"`

	// The version of the issue content for optimistic locking
	ContentVersion int `json:"content_version"`
}

// CreateIssueOption options to create one issue
type CreateIssueOption struct {
	// required:true
	Title string `json:"title" binding:"Required"`
	Body  string `json:"body"`
	Ref   string `json:"ref"`
	// deprecated
	Assignee  string   `json:"assignee"`
	Assignees []string `json:"assignees"`
	// swagger:strfmt date-time
	Deadline *time.Time `json:"due_date"`
	// list of label ids
	Labels []int64 `json:"labels"`
	Closed bool    `json:"closed"`
}

// EditIssueOption options for editing an issue
type EditIssueOption struct {
	Title string  `json:"title"`
	Body  *string `json:"body"`
	Ref   *string `json:"ref"`
	// deprecated
	Assignee  *string  `json:"assignee"`
	Assignees []string `json:"assignees"`
	State     *string  `json:"state"`
	// swagger:strfmt date-time
	Deadline       *time.Time `json:"due_date"`
	RemoveDeadline *bool      `json:"unset_due_date"`
	// The current version of the issue content to detect conflicts during editing
	ContentVersion *int `json:"content_version"`
}

// IssueAssigneesOption options for adding/removing issue assignees
type IssueAssigneesOption struct {
	Assignees []string `json:"assignees"`
}

// EditDeadlineOption options for creating a deadline
type EditDeadlineOption struct {
	// required:true
	// swagger:strfmt date-time
	Deadline *time.Time `json:"due_date"`
}

// IssueDeadline represents an issue deadline
// swagger:model
type IssueDeadline struct {
	// swagger:strfmt date-time
	Deadline *time.Time `json:"due_date"`
}
