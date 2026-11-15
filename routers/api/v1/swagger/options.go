// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package swagger

import (
	api "gitea.dev/modules/structs"
	"gitea.dev/services/forms"
)

// not actually a response, just a hack to get go-swagger to include definitions
// of the various XYZOption structs

// parameterBodies
// swagger:response parameterBodies
type swaggerParameterBodies struct {
	// in:body
	AddCollaboratorOption api.AddCollaboratorOption

	// in:body
	CreateEmailOption api.CreateEmailOption
	// in:body
	DeleteEmailOption api.DeleteEmailOption

	// in:body
	CreateHookOption api.CreateHookOption
	// in:body
	EditHookOption api.EditHookOption

	// in:body
	EditGitHookOption api.EditGitHookOption

	// in:body
	CreateIssueOption api.CreateIssueOption
	// in:body
	EditIssueOption api.EditIssueOption
	// in:body
	EditDeadlineOption api.EditDeadlineOption
	// in:body
	IssueAssigneesOption api.IssueAssigneesOption

	// in:body
	CreateIssueCommentOption api.CreateIssueCommentOption
	// in:body
	EditIssueCommentOption api.EditIssueCommentOption

	// in:body
	IssueLabelsOption api.IssueLabelsOption

	// in:body
	RenameUserOption api.RenameUserOption

	// in:body
	CreateLabelOption api.CreateLabelOption
	// in:body
	EditLabelOption api.EditLabelOption

	// in:body
	CreateOrgOption api.CreateOrgOption
	// in:body
	EditOrgOption api.EditOrgOption

	// in:body
	CreatePullRequestOption api.CreatePullRequestOption
	// in:body
	EditPullRequestOption api.EditPullRequestOption
	// in:body
	MergePullRequestOption forms.MergePullRequestForm

	// in:body
	CreateRepoOption api.CreateRepoOption
	// in:body
	EditRepoOption api.EditRepoOption
	// in:body
	RenameBranchRepoOption api.RenameBranchRepoOption
	// in:body
	CreateForkOption api.CreateForkOption

	// in:body
	CreateStatusOption api.CreateStatusOption

	// in:body
	CreateTeamOption api.CreateTeamOption
	// in:body
	EditTeamOption api.EditTeamOption

	// in:body
	CreateUserOption api.CreateUserOption

	// in:body
	EditUserOption api.EditUserOption

	// in:body
	GetFilesOptions api.GetFilesOptions

	// in:body
	ApplyDiffPatchFileOptions api.ApplyDiffPatchFileOptions

	// in:body
	ChangeFilesOptions api.ChangeFilesOptions

	// in:body
	CreateFileOptions api.CreateFileOptions

	// in:body
	UpdateFileOptions api.UpdateFileOptions

	// in:body
	DeleteFileOptions api.DeleteFileOptions

	// in:body
	CommitDateOptions api.CommitDateOptions

	// in:body
	EditReactionOption api.EditReactionOption

	// in:body
	CreateBranchRepoOption api.CreateBranchRepoOption
	// in:body
	UpdateBranchRepoOption api.UpdateBranchRepoOption

	// in:body
	CreateBranchProtectionOption api.CreateBranchProtectionOption

	// in:body
	EditBranchProtectionOption api.EditBranchProtectionOption

	// in:body
	UpdateBranchProtectionPriories api.UpdateBranchProtectionPriories

	// in:body
	CreatePullReviewOptions api.CreatePullReviewOptions

	// in:body
	CreatePullReviewComment api.CreatePullReviewComment

	// in:body
	CreatePullReviewCommentReplyOptions api.CreatePullReviewCommentReplyOptions

	// in:body
	SubmitPullReviewOptions api.SubmitPullReviewOptions

	// in:body
	DismissPullReviewOptions api.DismissPullReviewOptions

	// in:body
	PullReviewRequestOptions api.PullReviewRequestOptions

	// in:body
	CreateTagOption api.CreateTagOption

	// in:body
	CreateAccessTokenOption api.CreateAccessTokenOption

	// in:body
	UserSettingsOptions api.UserSettingsOptions

	// in:body
	RenameOrgOption api.RenameOrgOption
}
