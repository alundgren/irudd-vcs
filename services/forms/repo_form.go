// Copyright 2014 The Gogs Authors. All rights reserved.
// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package forms

import (
	"net/http"

	"gitea.dev/modules/json"
	"gitea.dev/modules/util"

	"gitea.com/go-chi/binding"
)

// MergePullRequestForm form for merging Pull Request
// swagger:model MergePullRequestOption
type MergePullRequestForm struct {
	// required: true
	// enum: ["merge","rebase","rebase-merge","squash","fast-forward-only","manually-merged"]
	Do                     string `json:"do" binding:"Required;In(merge,rebase,rebase-merge,squash,fast-forward-only,manually-merged)"`
	MergeTitleField        string `json:"merge_title_field,omitempty"`
	MergeMessageField      string `json:"merge_message_field,omitempty"`
	MergeCommitID          string `json:"merge_commit_id,omitempty"` // only used for manually-merged
	HeadCommitID           string `json:"head_commit_id,omitempty"`
	ForceMerge             bool   `json:"force_merge,omitempty"`
	MergeWhenChecksSucceed bool   `json:"merge_when_checks_succeed,omitempty"`
	DeleteBranchAfterMerge *bool  `json:"delete_branch_after_merge,omitempty"`
}

func (f *MergePullRequestForm) UnmarshalJSON(b []byte) error {
	// This is for backward compatibility, to support both field names like "do" and "Do",
	// because old code doesn't have "json" tag for these fields
	type aux struct {
		Do1                string `json:"do"`
		Do2                string `json:"Do"`
		MergeTitleField1   string `json:"merge_title_field"`
		MergeTitleField2   string `json:"MergeTitleField"`
		MergeMessageField1 string `json:"merge_message_field"`
		MergeMessageField2 string `json:"MergeMessageField"`
		MergeCommitID1     string `json:"merge_commit_id"`
		MergeCommitID2     string `json:"MergeCommitID"`

		HeadCommitID           string `json:"head_commit_id"`
		ForceMerge             bool   `json:"force_merge"`
		MergeWhenChecksSucceed bool   `json:"merge_when_checks_succeed"`
		DeleteBranchAfterMerge *bool  `json:"delete_branch_after_merge"`
	}
	var a aux
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	f.Do = util.IfZero(a.Do1, a.Do2)
	f.MergeTitleField = util.IfZero(a.MergeTitleField1, a.MergeTitleField2)
	f.MergeMessageField = util.IfZero(a.MergeMessageField1, a.MergeMessageField2)
	f.MergeCommitID = util.IfZero(a.MergeCommitID1, a.MergeCommitID2)
	f.HeadCommitID = a.HeadCommitID
	f.ForceMerge = a.ForceMerge
	f.MergeWhenChecksSucceed = a.MergeWhenChecksSucceed
	f.DeleteBranchAfterMerge = a.DeleteBranchAfterMerge
	return nil
}

// Validate validates the fields
func (f *MergePullRequestForm) Validate(req *http.Request, errs binding.Errors) binding.Errors {
	return errs
}
