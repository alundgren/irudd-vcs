// Copyright 2014 The Gogs Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package structs

import (
	"strings"
	"time"
)

// ObjectFormatName is the git hash algorithm used by a repository.
// swagger:enum ObjectFormatName
type ObjectFormatName string

const (
	ObjectFormatSHA1   ObjectFormatName = "sha1"
	ObjectFormatSHA256 ObjectFormatName = "sha256"
)

// Permission represents a set of permissions
type Permission struct {
	Admin bool `json:"admin"` // Admin indicates if the user is an administrator of the repository.
	Push  bool `json:"push"`  // Push indicates if the user can push code to the repository.
	Pull  bool `json:"pull"`  // Pull indicates if the user can pull code from the repository.
}

// Repository represents a repository
type Repository struct {
	ID          int64  `json:"id"`
	Owner       *User  `json:"owner"`
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Empty       bool   `json:"empty"`
	Private     bool   `json:"private"`
	Fork        bool   `json:"fork"`
	Template    bool   `json:"template"`
	// the original repository if this repository is a fork, otherwise null
	Parent              *Repository `json:"parent,omitempty"`
	Mirror              bool        `json:"mirror"`
	Size                int         `json:"size"`
	HTMLURL             string      `json:"html_url"`
	URL                 string      `json:"url"`
	Link                string      `json:"link"`
	SSHURL              string      `json:"ssh_url"`
	CloneURL            string      `json:"clone_url"`
	OriginalURL         string      `json:"original_url"`
	Website             string      `json:"website"`
	Stars               int         `json:"stars_count"`
	Forks               int         `json:"forks_count"`
	Watchers            int         `json:"watchers_count"`
	BranchCount         int         `json:"branch_count"`
	OpenIssues          int         `json:"open_issues_count"`
	OpenPulls           int         `json:"open_pr_counter"`
	Releases            int         `json:"release_counter"`
	DefaultBranch       string      `json:"default_branch"`
	DefaultTargetBranch string      `json:"default_target_branch,omitempty"`
	Archived            bool        `json:"archived"`
	// swagger:strfmt date-time
	Created time.Time `json:"created_at"`
	// swagger:strfmt date-time
	Updated                       time.Time   `json:"updated_at"`
	ArchivedAt                    time.Time   `json:"archived_at"`
	Permissions                   *Permission `json:"permissions,omitempty"`
	HasCode                       bool        `json:"has_code"`
	HasIssues                     bool        `json:"has_issues"`
	HasPullRequests               bool        `json:"has_pull_requests"`
	HasReleases                   bool        `json:"has_releases"`
	IgnoreWhitespaceConflicts     bool        `json:"ignore_whitespace_conflicts"`
	AllowMerge                    bool        `json:"allow_merge_commits"`
	AllowRebase                   bool        `json:"allow_rebase"`
	AllowRebaseMerge              bool        `json:"allow_rebase_explicit"`
	AllowSquash                   bool        `json:"allow_squash_merge"`
	AllowFastForwardOnly          bool        `json:"allow_fast_forward_only_merge"`
	AllowMergeUpdate              bool        `json:"allow_merge_update"`
	AllowRebaseUpdate             bool        `json:"allow_rebase_update"`
	AllowManualMerge              bool        `json:"allow_manual_merge"`
	AutodetectManualMerge         bool        `json:"autodetect_manual_merge"`
	DefaultDeleteBranchAfterMerge bool        `json:"default_delete_branch_after_merge"`
	DefaultMergeStyle             string      `json:"default_merge_style"`
	DefaultUpdateStyle            string      `json:"default_update_style"`
	DefaultAllowMaintainerEdit    bool        `json:"default_allow_maintainer_edit"`
	AvatarURL                     string      `json:"avatar_url"`
	Internal                      bool        `json:"internal"`
	MirrorInterval                string      `json:"mirror_interval"`
	// ObjectFormatName of the underlying git repository
	ObjectFormatName ObjectFormatName `json:"object_format_name"`
	// swagger:strfmt date-time
	MirrorUpdated time.Time `json:"mirror_updated"`
	// swagger:strfmt date-time
	MirrorLastSyncAt time.Time     `json:"mirror_last_sync_at"`
	RepoTransfer     *RepoTransfer `json:"repo_transfer,omitempty"`
	Topics           []string      `json:"topics"`
}

// CreateRepoOption options when creating repository
// swagger:model
type CreateRepoOption struct {
	// Name of the repository to create
	//
	// required: true
	// unique: true
	Name string `json:"name" binding:"Required;AlphaDashDot;MaxSize(100)"`
	// Description of the repository to create
	Description string `json:"description" binding:"MaxSize(2048)"`
	// Whether the repository is private
	Private bool `json:"private"`
	// Label-Set to use
	IssueLabels string `json:"issue_labels" binding:"MaxSize(255)"`
	// Whether the repository should be auto-initialized?
	AutoInit bool `json:"auto_init"`
	// Whether the repository is template
	Template bool `json:"template"`
	// Gitignores to use
	Gitignores string `json:"gitignores" binding:"MaxSize(1024)"`
	// License to use
	License string `json:"license" binding:"MaxSize(100)"`
	// Readme of the repository to create
	Readme string `json:"readme" binding:"MaxSize(255)"`
	// DefaultBranch of the repository (used when initializes and in template)
	DefaultBranch string `json:"default_branch" binding:"GitRefName;MaxSize(100)"`
	// TrustModel of the repository
	// enum: ["default","collaborator","committer","collaboratorcommitter"]
	TrustModel string `json:"trust_model"`
	// ObjectFormatName of the underlying git repository, empty string for default (sha1)
	ObjectFormatName ObjectFormatName `json:"object_format_name" binding:"MaxSize(6)"`
}

// EditRepoOption options when editing a repository's properties
// swagger:model
type EditRepoOption struct {
	// name of the repository
	// unique: true
	Name *string `json:"name,omitempty" binding:"OmitEmpty;AlphaDashDot;MaxSize(100);"`
	// a short description of the repository.
	Description *string `json:"description,omitempty" binding:"MaxSize(2048)"`
	// a URL with more information about the repository.
	Website *string `json:"website,omitempty" binding:"MaxSize(1024)"`
	// either `true` to make the repository private or `false` to make it public.
	// Note: you will get a 422 error if the organization restricts changing repository visibility to organization
	// owners and a non-owner tries to change the value of private.
	Private *bool `json:"private,omitempty"`
	// either `true` to make this repository a template or `false` to make it a normal repository
	Template *bool `json:"template,omitempty"`
	// either `true` to enable code for this repository or `false` to disable it.
	HasCode *bool `json:"has_code,omitempty"`
	// either `true` to enable issues for this repository or `false` to disable them.
	HasIssues *bool `json:"has_issues,omitempty"`
	// sets the default branch for this repository.
	DefaultBranch *string `json:"default_branch,omitempty"`
	// either `true` to allow pull requests, or `false` to prevent pull request.
	HasPullRequests *bool `json:"has_pull_requests,omitempty"`
	// either `true` to enable releases unit, or `false` to disable them.
	HasReleases *bool `json:"has_releases,omitempty"`
	// either `true` to ignore whitespace for conflicts, or `false` to not ignore whitespace.
	IgnoreWhitespaceConflicts *bool `json:"ignore_whitespace_conflicts,omitempty"`
	// either `true` to allow merging pull requests with a merge commit, or `false` to prevent merging pull requests with merge commits.
	AllowMerge *bool `json:"allow_merge_commits,omitempty"`
	// either `true` to allow rebase-merging pull requests, or `false` to prevent rebase-merging.
	AllowRebase *bool `json:"allow_rebase,omitempty"`
	// either `true` to allow rebase with explicit merge commits (--no-ff), or `false` to prevent rebase with explicit merge commits.
	AllowRebaseMerge *bool `json:"allow_rebase_explicit,omitempty"`
	// either `true` to allow squash-merging pull requests, or `false` to prevent squash-merging.
	AllowSquash *bool `json:"allow_squash_merge,omitempty"`
	// either `true` to allow fast-forward-only merging pull requests, or `false` to prevent fast-forward-only merging.
	AllowFastForwardOnly *bool `json:"allow_fast_forward_only_merge,omitempty"`
	// either `true` to allow mark pr as merged manually, or `false` to prevent it.
	AllowManualMerge *bool `json:"allow_manual_merge,omitempty"`
	// either `true` to enable AutodetectManualMerge, or `false` to prevent it. Note: In some special cases, misjudgments can occur.
	AutodetectManualMerge *bool `json:"autodetect_manual_merge,omitempty"`
	// either `true` to allow updating pull request branch by merge, or `false` to prevent it.
	AllowMergeUpdate *bool `json:"allow_merge_update,omitempty"`
	// either `true` to allow updating pull request branch by rebase, or `false` to prevent it.
	AllowRebaseUpdate *bool `json:"allow_rebase_update,omitempty"`
	// set to `true` to delete pr branch after merge by default
	DefaultDeleteBranchAfterMerge *bool `json:"default_delete_branch_after_merge,omitempty"`
	// set to a merge style to be used by this repository: "merge", "rebase", "rebase-merge", "squash", or "fast-forward-only".
	DefaultMergeStyle *string `json:"default_merge_style,omitempty"`
	// set to an update style to be used by this repository: "merge" or "rebase".
	DefaultUpdateStyle *string `json:"default_update_style,omitempty"`
	// set to `true` to allow edits from maintainers by default
	DefaultAllowMaintainerEdit *bool `json:"default_allow_maintainer_edit,omitempty"`
	// set to `true` to archive this repository.
	Archived *bool `json:"archived,omitempty"`
	// set to a string like `8h30m0s` to set the mirror interval time
	MirrorInterval *string `json:"mirror_interval,omitempty"`
	// enable prune - remove obsolete remote-tracking references when mirroring
	EnablePrune *bool `json:"enable_prune,omitempty"`
	// authentication username for the remote repository (mirrors)
	MirrorUsername *string `json:"mirror_username,omitempty"`
	// authentication password for the remote repository (mirrors)
	MirrorPassword *string `json:"mirror_password,omitempty"`
	// authentication token for the remote repository (mirrors)
	MirrorToken *string `json:"mirror_token,omitempty"`
}

// CreateBranchRepoOption options when creating a branch in a repository
// swagger:model
type CreateBranchRepoOption struct {
	// Name of the branch to create
	//
	// required: true
	// unique: true
	BranchName string `json:"new_branch_name" binding:"Required;GitRefName;MaxSize(100)"`

	// Deprecated: true
	// Name of the old branch to create from
	//
	// unique: true
	OldBranchName string `json:"old_branch_name" binding:"GitRefName;MaxSize(100)"`

	// Name of the old branch/tag/commit to create from
	//
	// unique: true
	OldRefName string `json:"old_ref_name" binding:"GitRefName;MaxSize(100)"`
}

// RenameBranchRepoOption options when renaming a branch in a repository
// swagger:model
type RenameBranchRepoOption struct {
	// New branch name
	//
	// required: true
	// unique: true
	Name string `json:"name" binding:"Required;GitRefName;MaxSize(100)"`
}

// UpdateBranchRepoOption options when updating a branch reference in a repository
// swagger:model
type UpdateBranchRepoOption struct {
	// New commit SHA (or any ref) the branch should point to
	//
	// required: true
	NewCommitID string `json:"new_commit_id" binding:"Required"`

	// Expected old commit SHA of the branch; if provided it must match the current tip
	OldCommitID string `json:"old_commit_id"`

	// Force update even if the change is not a fast-forward
	Force bool `json:"force"`
}

// GitServiceType represents a git service
type GitServiceType int

// enumerate all GitServiceType
const (
	NotMigrated       GitServiceType = iota // 0 not migrated from external sites
	PlainGitService                         // 1 plain git service
	GithubService                           // 2 github.com
	GiteaService                            // 3 gitea service
	GitlabService                           // 4 gitlab service
	GogsService                             // 5 gogs service
	OneDevService                           // 6 onedev service
	GitBucketService                        // 7 gitbucket service
	CodebaseService                         // 8 codebase service
	CodeCommitService                       // 9 codecommit service
)

// Name represents the service type's name
// WARNING: the name has to be equal to that on goth's library
func (gt GitServiceType) Name() string {
	return strings.ToLower(gt.Title())
}

// Title represents the service type's proper title
func (gt GitServiceType) Title() string {
	switch gt {
	case GithubService:
		return "GitHub"
	case GiteaService:
		return "Gitea"
	case GitlabService:
		return "GitLab"
	case GogsService:
		return "Gogs"
	case OneDevService:
		return "OneDev"
	case GitBucketService:
		return "GitBucket"
	case CodebaseService:
		return "Codebase"
	case CodeCommitService:
		return "CodeCommit"
	case PlainGitService:
		return "Git"
	}
	return ""
}

// TokenAuth represents whether a service type supports token-based auth
func (gt GitServiceType) TokenAuth() bool {
	switch gt {
	case GithubService, GiteaService, GitlabService:
		return true
	}
	return false
}

// RepoTransfer represents a pending repo transfer
type RepoTransfer struct {
	Doer      *User   `json:"doer"`
	Recipient *User   `json:"recipient"`
	Teams     []*Team `json:"teams"`
}
