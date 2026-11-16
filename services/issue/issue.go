// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package issue

import (
	"context"
	"fmt"

	"gitea.dev/models/db"
	issues_model "gitea.dev/models/issues"
	repo_model "gitea.dev/models/repo"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/git"
	"gitea.dev/modules/gitrepo"
	"gitea.dev/modules/log"
	notify_service "gitea.dev/services/notify"
)

// NewIssue creates new issue with labels for repository.
func NewIssue(ctx context.Context, repo *repo_model.Repository, issue *issues_model.Issue, labelIDs, assigneeIDs []int64) error {
	if err := issue.LoadPoster(ctx); err != nil {
		return err
	}

	assigneeCommentMap := make(map[int64]*issues_model.Comment)
	assignees := make(map[int64]*user_model.User)
	if err := db.WithTx(ctx, func(ctx context.Context) error {
		if err := issues_model.NewIssue(ctx, repo, issue, labelIDs); err != nil {
			return err
		}
		for _, assigneeID := range assigneeIDs {
			assignee, err := user_model.GetUserByID(ctx, assigneeID)
			if err != nil {
				log.Error("GetUserByID: %v", err)
				continue
			}
			assignees[assigneeID] = assignee
			comment, err := AddAssigneeIfNotAssigned(ctx, issue, issue.Poster, assignee)
			if err != nil {
				return err
			}
			assigneeCommentMap[assigneeID] = comment
		}
		return nil
	}); err != nil {
		return err
	}

	mentions, err := issues_model.FindAndUpdateIssueMentions(ctx, issue, issue.Poster, issue.Content)
	if err != nil {
		return err
	}

	notify_service.NewIssue(ctx, issue, mentions)
	if len(issue.Labels) > 0 {
		notify_service.IssueChangeLabels(ctx, issue.Poster, issue, issue.Labels, nil)
	}

	if len(assigneeIDs) > 0 {
		for _, assignee := range assignees {
			notify_service.IssueChangeAssignee(ctx, issue.Poster, issue, assignee, false, assigneeCommentMap[assignee.ID])
		}
	}

	return nil
}

// ChangeTitle changes the title of this issue, as the given user.
func ChangeTitle(ctx context.Context, issue *issues_model.Issue, doer *user_model.User, title string) error {
	oldTitle := issue.Title
	issue.Title = title

	if oldTitle == title {
		return nil
	}

	if err := issue.LoadRepo(ctx); err != nil {
		return err
	}

	if err := issues_model.ChangeIssueTitle(ctx, issue, doer, oldTitle); err != nil {
		return err
	}

	var reviewNotifiers []*ReviewRequestNotifier
	if issue.IsPull && issues_model.HasWorkInProgressPrefix(oldTitle) && !issues_model.HasWorkInProgressPrefix(title) {
		if err := issue.LoadPullRequest(ctx); err != nil {
			return err
		}

		var err error
		reviewNotifiers, err = PullRequestCodeOwnersReview(ctx, issue.PullRequest)
		if err != nil {
			log.Error("PullRequestCodeOwnersReview: %v", err)
		}
	}

	notify_service.IssueChangeTitle(ctx, doer, issue, oldTitle)
	ReviewRequestNotify(ctx, issue, issue.Poster, reviewNotifiers)

	return nil
}

// ChangeIssueRef changes the branch of this issue, as the given user.
func ChangeIssueRef(ctx context.Context, issue *issues_model.Issue, doer *user_model.User, ref string) error {
	oldRef := issue.Ref
	issue.Ref = ref

	if err := issues_model.ChangeIssueRef(ctx, issue, doer, oldRef); err != nil {
		return err
	}

	notify_service.IssueChangeRef(ctx, doer, issue, oldRef)

	return nil
}

// DeleteIssue deletes an issue
func DeleteIssue(ctx context.Context, doer *user_model.User, issue *issues_model.Issue) error {
	// load issue before deleting it
	if err := issue.LoadAttributes(ctx); err != nil {
		return err
	}
	if err := issue.LoadPullRequest(ctx); err != nil {
		return err
	}

	// delete entries in database
	if err := deleteIssue(ctx, issue); err != nil {
		return err
	}

	// delete pull request related git data
	if issue.IsPull {
		if err := issue.PullRequest.LoadBaseRepo(ctx); err != nil {
			return err
		}
		if err := gitrepo.RemoveRef(ctx, issue.PullRequest.BaseRepo, issue.PullRequest.GetGitHeadRefName()); err != nil {
			return err
		}
	}

	notify_service.DeleteIssue(ctx, doer, issue)

	return nil
}

// GetRefEndNamesAndURLs retrieves the ref end names (e.g. refs/heads/branch-name -> branch-name)
// and their respective URLs.
func GetRefEndNamesAndURLs(issues []*issues_model.Issue, repoLink string) (map[int64]string, map[int64]string) {
	issueRefEndNames := make(map[int64]string, len(issues))
	issueRefURLs := make(map[int64]string, len(issues))
	for _, issue := range issues {
		if issue.Ref != "" {
			ref := git.RefName(issue.Ref)
			issueRefEndNames[issue.ID] = ref.ShortName()
			issueRefURLs[issue.ID] = repoLink + "/src/" + ref.RefWebLinkPath()
		}
	}
	return issueRefEndNames, issueRefURLs
}

// deleteIssue deletes the issue
func deleteIssue(ctx context.Context, issue *issues_model.Issue) error {
	return db.WithTx(ctx, func(ctx context.Context) error {
		if _, err := db.GetEngine(ctx).ID(issue.ID).NoAutoCondition().Delete(issue); err != nil {
			return err
		}

		if err := issues_model.DecrRepoIssueNumbers(ctx, issue.RepoID, issue.IsPull, true, issue.IsClosed); err != nil {
			return err
		}

		// delete all database data still assigned to this issue
		return db.DeleteBeans(ctx,
			&issues_model.ContentHistory{IssueID: issue.ID},
			&issues_model.Comment{IssueID: issue.ID},
			&issues_model.IssueLabel{IssueID: issue.ID},
			&issues_model.IssueAssignees{IssueID: issue.ID},
			&issues_model.IssueUser{IssueID: issue.ID},
			&issues_model.Reaction{IssueID: issue.ID},
			&issues_model.PullRequest{IssueID: issue.ID},
			&issues_model.Comment{RefIssueID: issue.ID},
		)
	})
}

// DeleteOrphanedIssues delete issues without a repo
func DeleteOrphanedIssues(ctx context.Context) error {
	return db.WithTx(ctx, func(ctx context.Context) error {
		repoIDs, err := issues_model.GetOrphanedIssueRepoIDs(ctx)
		if err != nil {
			return err
		}
		for i := range repoIDs {
			if err := DeleteIssuesByRepoID(ctx, repoIDs[i]); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteIssuesByRepoID deletes issues by repositories id
func DeleteIssuesByRepoID(ctx context.Context, repoID int64) error {
	for {
		issues := make([]*issues_model.Issue, 0, db.DefaultMaxInSize)
		if err := db.GetEngine(ctx).
			Where("repo_id = ?", repoID).
			OrderBy("id").
			Limit(db.DefaultMaxInSize).
			Find(&issues); err != nil {
			return err
		}

		if len(issues) == 0 {
			break
		}

		for _, issue := range issues {
			if err := deleteIssue(ctx, issue); err != nil {
				return fmt.Errorf("deleteIssue [issue_id: %d]: %w", issue.ID, err)
			}
		}
	}

	return nil
}
