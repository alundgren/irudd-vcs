// Copyright 2018 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package issues

import (
	"context"

	"gitea.dev/models/db"
	repo_model "gitea.dev/models/repo"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/container"
	"gitea.dev/modules/log"
	"gitea.dev/modules/setting"
)

// CommentList defines a list of comments
type CommentList []*Comment

// LoadPosters loads posters
func (comments CommentList) LoadPosters(ctx context.Context) error {
	if len(comments) == 0 {
		return nil
	}

	posterIDs := container.FilterSlice(comments, func(c *Comment) (int64, bool) {
		return c.PosterID, c.Poster == nil && c.PosterID > 0
	})

	posterMaps, err := user_model.GetUsersMapByIDs(ctx, posterIDs)
	if err != nil {
		return err
	}

	for _, comment := range comments {
		if comment.Poster == nil {
			comment.Poster = user_model.GetPossibleUserFromMap(comment.PosterID, posterMaps)
		}
	}
	return nil
}

func (comments CommentList) getLabelIDs() []int64 {
	return container.FilterSlice(comments, func(comment *Comment) (int64, bool) {
		return comment.LabelID, comment.LabelID > 0 && comment.Label == nil
	})
}

func (comments CommentList) loadLabels(ctx context.Context) error {
	if len(comments) == 0 {
		return nil
	}

	labelIDs := comments.getLabelIDs()
	if len(labelIDs) == 0 {
		return nil
	}
	commentLabels := make(map[int64]*Label, len(labelIDs))
	left := len(labelIDs)
	for left > 0 {
		limit := min(left, db.DefaultMaxInSize)
		rows, err := db.GetEngine(ctx).
			In("id", labelIDs[:limit]).
			Rows(new(Label))
		if err != nil {
			return err
		}

		for rows.Next() {
			var label Label
			err = rows.Scan(&label)
			if err != nil {
				_ = rows.Close()
				return err
			}
			commentLabels[label.ID] = &label
		}
		_ = rows.Close()
		left -= limit
		labelIDs = labelIDs[limit:]
	}

	for _, comment := range comments {
		comment.Label = commentLabels[comment.LabelID]
	}
	return nil
}

func (comments CommentList) getAssigneeIDs() []int64 {
	return container.FilterSlice(comments, func(comment *Comment) (int64, bool) {
		return comment.AssigneeID, comment.AssigneeID > 0
	})
}

func (comments CommentList) loadAssignees(ctx context.Context) error {
	if len(comments) == 0 {
		return nil
	}

	assigneeIDs := comments.getAssigneeIDs()
	if len(assigneeIDs) == 0 {
		return nil
	}
	assignees := make(map[int64]*user_model.User, len(assigneeIDs))
	left := len(assigneeIDs)
	for left > 0 {
		limit := min(left, db.DefaultMaxInSize)
		rows, err := db.GetEngine(ctx).
			In("id", assigneeIDs[:limit]).
			Rows(new(user_model.User))
		if err != nil {
			return err
		}

		for rows.Next() {
			var user user_model.User
			err = rows.Scan(&user)
			if err != nil {
				rows.Close()
				return err
			}

			assignees[user.ID] = &user
		}
		_ = rows.Close()

		left -= limit
		assigneeIDs = assigneeIDs[limit:]
	}

	for _, comment := range comments {
		comment.Assignee = assignees[comment.AssigneeID]
		if comment.Assignee == nil {
			comment.AssigneeID = user_model.GhostUserID
			comment.Assignee = user_model.NewGhostUser()
		}
	}
	return nil
}

// getIssueIDs returns all the issue ids on this comment list which issue hasn't been loaded
func (comments CommentList) getIssueIDs() []int64 {
	return container.FilterSlice(comments, func(comment *Comment) (int64, bool) {
		return comment.IssueID, comment.Issue == nil
	})
}

// Issues returns all the issues of comments
func (comments CommentList) Issues() IssueList {
	issues := make(map[int64]*Issue, len(comments))
	for _, comment := range comments {
		if comment.Issue != nil {
			if _, ok := issues[comment.Issue.ID]; !ok {
				issues[comment.Issue.ID] = comment.Issue
			}
		}
	}

	issueList := make([]*Issue, 0, len(issues))
	for _, issue := range issues {
		issueList = append(issueList, issue)
	}
	return issueList
}

// LoadIssues loads issues of comments
func (comments CommentList) LoadIssues(ctx context.Context) error {
	if len(comments) == 0 {
		return nil
	}

	issueIDs := comments.getIssueIDs()
	issues := make(map[int64]*Issue, len(issueIDs))
	left := len(issueIDs)
	for left > 0 {
		limit := min(left, db.DefaultMaxInSize)
		rows, err := db.GetEngine(ctx).
			In("id", issueIDs[:limit]).
			Rows(new(Issue))
		if err != nil {
			return err
		}

		for rows.Next() {
			var issue Issue
			err = rows.Scan(&issue)
			if err != nil {
				rows.Close()
				return err
			}

			issues[issue.ID] = &issue
		}
		_ = rows.Close()

		left -= limit
		issueIDs = issueIDs[limit:]
	}

	for _, comment := range comments {
		if comment.Issue == nil {
			comment.Issue = issues[comment.IssueID]
		}
	}
	return nil
}

func (comments CommentList) getReviewIDs() []int64 {
	return container.FilterSlice(comments, func(comment *Comment) (int64, bool) {
		return comment.ReviewID, comment.ReviewID > 0
	})
}

func (comments CommentList) loadReviews(ctx context.Context) error {
	if len(comments) == 0 {
		return nil
	}

	reviewIDs := comments.getReviewIDs()
	if len(reviewIDs) == 0 {
		return nil
	}
	reviews := make(map[int64]*Review, len(reviewIDs))
	if err := db.GetEngine(ctx).In("id", reviewIDs).Find(&reviews); err != nil {
		return err
	}

	for _, comment := range comments {
		comment.Review = reviews[comment.ReviewID]
		if comment.Review == nil {
			// review request which has been replaced by actual reviews doesn't exist in database anymore, so don't log errors for them.
			if comment.ReviewID > 0 && comment.Type != CommentTypeReviewRequest {
				log.Error("comment with review id [%d] but has no review record", comment.ReviewID)
			}
			continue
		}

		// If the comment dismisses a review, we need to load the reviewer to show whose review has been dismissed.
		// Otherwise, the reviewer is the poster of the comment, so we don't need to load it.
		if comment.Type == CommentTypeDismissReview {
			if err := comment.Review.LoadReviewer(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

// loadResolveDoers bulk-loads the resolve doer for all code comments that have one.
func (comments CommentList) loadResolveDoers(ctx context.Context) error {
	resolveDoerIDs := container.FilterSlice(comments, func(c *Comment) (int64, bool) {
		return c.ResolveDoerID, c.ResolveDoerID != 0 && c.Type == CommentTypeCode
	})
	if len(resolveDoerIDs) == 0 {
		return nil
	}

	userMaps, err := user_model.GetUsersMapByIDs(ctx, resolveDoerIDs)
	if err != nil {
		return err
	}

	for _, comment := range comments {
		if comment.ResolveDoerID == 0 || comment.Type != CommentTypeCode {
			continue
		}
		if u, ok := userMaps[comment.ResolveDoerID]; ok {
			comment.ResolveDoer = u
		} else {
			comment.ResolveDoer = user_model.NewGhostUser()
		}
	}
	return nil
}

// loadReactions bulk-loads reactions for all comments in the list.
func (comments CommentList) loadReactions(ctx context.Context, repo *repo_model.Repository) error {
	if len(comments) == 0 {
		return nil
	}

	commentIDs := container.FilterSlice(comments, func(c *Comment) (int64, bool) {
		return c.ID, c.Reactions == nil
	})
	if len(commentIDs) == 0 {
		return nil
	}

	var allReactions ReactionList
	if err := db.GetEngine(ctx).
		Where("`comment_id` > 0").
		In("comment_id", commentIDs).
		In("`type`", setting.UI.Reactions).
		Asc("issue_id", "comment_id", "created_unix", "id").
		Find(&allReactions); err != nil {
		return err
	}

	if _, err := allReactions.LoadUsers(ctx, repo); err != nil {
		return err
	}

	reactByComment := make(map[int64]ReactionList, len(commentIDs))
	for _, r := range allReactions {
		reactByComment[r.CommentID] = append(reactByComment[r.CommentID], r)
	}

	for _, comment := range comments {
		if comment.Reactions == nil {
			comment.Reactions = reactByComment[comment.ID]
		}
	}
	return nil
}

// LoadAttributes loads attributes of the comments
func (comments CommentList) LoadAttributes(ctx context.Context) (err error) {
	if err = comments.LoadPosters(ctx); err != nil {
		return err
	}

	if err = comments.loadLabels(ctx); err != nil {
		return err
	}

	if err = comments.loadAssignees(ctx); err != nil {
		return err
	}

	if err = comments.loadReviews(ctx); err != nil {
		return err
	}

	return comments.LoadIssues(ctx)
}
