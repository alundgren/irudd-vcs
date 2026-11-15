// Copyright 2020 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package convert

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	issues_model "gitea.dev/models/issues"
	repo_model "gitea.dev/models/repo"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/label"
	"gitea.dev/modules/log"
	"gitea.dev/modules/setting"
	api "gitea.dev/modules/structs"
	"gitea.dev/modules/util"
)

// ToAPIIssue converts an Issue to API format
// it assumes some fields assigned with values:
// Required - Poster, Labels,
// Optional - Assignee, PullRequest
func ToAPIIssue(ctx context.Context, doer *user_model.User, issue *issues_model.Issue) *api.Issue {
	if err := issue.LoadPoster(ctx); err != nil {
		return &api.Issue{}
	}
	if err := issue.LoadRepo(ctx); err != nil {
		return &api.Issue{}
	}

	apiIssue := &api.Issue{
		ID:             issue.ID,
		Index:          issue.Index,
		Poster:         ToUser(ctx, issue.Poster, doer),
		Title:          issue.Title,
		Body:           issue.Content,
		Ref:            issue.Ref,
		State:          issue.State(),
		IsLocked:       issue.IsLocked,
		Comments:       issue.NumComments,
		Created:        issue.CreatedUnix.AsTime(),
		Updated:        issue.UpdatedUnix.AsTime(),
		ContentVersion: issue.ContentVersion,
	}

	if issue.Repo != nil {
		if err := issue.Repo.LoadOwner(ctx); err != nil {
			return &api.Issue{}
		}
		apiIssue.URL = issue.APIURL(ctx)
		apiIssue.HTMLURL = issue.HTMLURL(ctx)
		if err := issue.LoadLabels(ctx); err != nil {
			return &api.Issue{}
		}
		apiIssue.Labels = util.SliceNilAsEmpty(ToLabelList(issue.Labels, issue.Repo, issue.Repo.Owner))
		apiIssue.Repo = &api.RepositoryMeta{
			ID:       issue.Repo.ID,
			Name:     issue.Repo.Name,
			Owner:    issue.Repo.OwnerName,
			FullName: issue.Repo.FullName(),
		}
	}

	if issue.ClosedUnix != 0 {
		apiIssue.Closed = issue.ClosedUnix.AsTimePtr()
	}

	if err := issue.LoadAssignees(ctx); err != nil {
		return &api.Issue{}
	}
	if len(issue.Assignees) > 0 {
		for _, assignee := range issue.Assignees {
			apiIssue.Assignees = append(apiIssue.Assignees, ToUser(ctx, assignee, nil))
		}
		apiIssue.Assignee = ToUser(ctx, issue.Assignees[0], nil) // For compatibility, we're keeping the first assignee as `apiIssue.Assignee`
	}
	if issue.IsPull {
		if err := issue.LoadPullRequest(ctx); err != nil {
			return &api.Issue{}
		}
		if issue.PullRequest != nil {
			apiIssue.PullRequest = &api.PullRequestMeta{
				HasMerged:        issue.PullRequest.HasMerged,
				IsWorkInProgress: issue.PullRequest.IsWorkInProgress(ctx),
			}
			if issue.PullRequest.HasMerged {
				apiIssue.PullRequest.Merged = issue.PullRequest.MergedUnix.AsTimePtr()
			}
			// Add pr's html url
			apiIssue.PullRequest.HTMLURL = issue.HTMLURL(ctx)
		}
	}
	if issue.DeadlineUnix != 0 {
		apiIssue.Deadline = issue.DeadlineUnix.AsTimePtr()
	}

	return apiIssue
}

// ToAPIIssueList converts an IssueList to API format
func ToAPIIssueList(ctx context.Context, doer *user_model.User, il issues_model.IssueList) []*api.Issue {
	result := make([]*api.Issue, len(il))
	for i := range il {
		result[i] = ToAPIIssue(ctx, doer, il[i])
	}
	return result
}

// ToLabel converts Label to API format
func ToLabel(label *issues_model.Label, repo *repo_model.Repository, org *user_model.User) *api.Label {
	result := &api.Label{
		ID:          label.ID,
		Name:        label.Name,
		Exclusive:   label.Exclusive,
		Color:       strings.TrimLeft(label.Color, "#"),
		Description: label.Description,
		IsArchived:  label.IsArchived(),
	}

	labelBelongsToRepo := label.BelongsToRepo()

	// calculate URL
	if labelBelongsToRepo && repo != nil {
		result.URL = fmt.Sprintf("%s/labels/%d", repo.APIURL(), label.ID)
	} else { // BelongsToOrg
		if org != nil {
			result.URL = fmt.Sprintf("%sapi/v1/orgs/%s/labels/%d", setting.AppURL, url.PathEscape(org.Name), label.ID)
		} else {
			log.Error("ToLabel did not get org to calculate url for label with id '%d'", label.ID)
		}
	}

	if labelBelongsToRepo && repo == nil {
		log.Error("ToLabel did not get repo to calculate url for label with id '%d'", label.ID)
	}

	return result
}

// ToLabelList converts list of Label to API format
func ToLabelList(labels []*issues_model.Label, repo *repo_model.Repository, org *user_model.User) []*api.Label {
	result := make([]*api.Label, len(labels))
	for i := range labels {
		result[i] = ToLabel(labels[i], repo, org)
	}
	return result
}

// ToLabelTemplate converts Label to API format
func ToLabelTemplate(label *label.Label) *api.LabelTemplate {
	result := &api.LabelTemplate{
		Name:        label.Name,
		Exclusive:   label.Exclusive,
		Color:       strings.TrimLeft(label.Color, "#"),
		Description: label.Description,
	}

	return result
}

// ToLabelTemplateList converts list of Label to API format
func ToLabelTemplateList(labels []*label.Label) []*api.LabelTemplate {
	result := make([]*api.LabelTemplate, len(labels))
	for i := range labels {
		result[i] = ToLabelTemplate(labels[i])
	}
	return result
}
