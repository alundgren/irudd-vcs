// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package issue

import (
	"context"

	issues_model "gitea.dev/models/issues"
	"gitea.dev/models/perm"
	access_model "gitea.dev/models/perm/access"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unit"
	user_model "gitea.dev/models/user"
	notify_service "gitea.dev/services/notify"
)

// ReviewRequest add or remove a review request from a user for this PR, and make comment for it.
func ReviewRequest(ctx context.Context, issue *issues_model.Issue, doer *user_model.User, permDoer *access_model.Permission, reviewer *user_model.User, isAdd bool) (comment *issues_model.Comment, err error) {
	err = isValidReviewRequest(ctx, reviewer, doer, isAdd, issue, permDoer)
	if err != nil {
		return nil, err
	}

	if isAdd {
		comment, err = issues_model.AddReviewRequest(ctx, issue, reviewer, doer)
	} else {
		comment, err = issues_model.RemoveReviewRequest(ctx, issue, reviewer, doer)
	}

	if err != nil {
		return nil, err
	}

	if comment != nil {
		notify_service.PullRequestReviewRequest(ctx, doer, issue, reviewer, isAdd, comment)
	}

	return comment, err
}

// isValidReviewRequest Check permission for ReviewRequest
func isValidReviewRequest(ctx context.Context, reviewer, doer *user_model.User, isAdd bool, issue *issues_model.Issue, permDoer *access_model.Permission) error {
	if reviewer.IsOrganization() {
		return issues_model.ErrNotValidReviewRequest{
			Reason: "Organization can't be added as reviewer",
			UserID: doer.ID,
			RepoID: issue.Repo.ID,
		}
	}
	if doer.IsOrganization() {
		return issues_model.ErrNotValidReviewRequest{
			Reason: "Organization can't be doer to add reviewer",
			UserID: doer.ID,
			RepoID: issue.Repo.ID,
		}
	}

	permReviewer, err := access_model.GetIndividualUserRepoPermission(ctx, issue.Repo, reviewer)
	if err != nil {
		return err
	}

	if permDoer == nil {
		permDoer = new(access_model.Permission)
		*permDoer, err = access_model.GetDoerRepoPermission(ctx, issue.Repo, doer)
		if err != nil {
			return err
		}
	}

	lastReview, err := issues_model.GetReviewByIssueIDAndUserID(ctx, issue.ID, reviewer.ID)
	if err != nil && !issues_model.IsErrReviewNotExist(err) {
		return err
	}

	canDoerChangeReviewRequests := CanDoerChangeReviewRequests(ctx, doer, issue.Repo, issue.PosterID)

	if isAdd {
		if !permReviewer.CanAccessAny(perm.AccessModeRead, unit.TypePullRequests) {
			return issues_model.ErrNotValidReviewRequest{
				Reason: "Reviewer can't read",
				UserID: doer.ID,
				RepoID: issue.Repo.ID,
			}
		}

		if reviewer.ID == issue.PosterID && issue.OriginalAuthorID == 0 {
			return issues_model.ErrNotValidReviewRequest{
				Reason: "poster of pr can't be reviewer",
				UserID: doer.ID,
				RepoID: issue.Repo.ID,
			}
		}

		if canDoerChangeReviewRequests {
			return nil
		}

		if doer.ID == issue.PosterID && issue.OriginalAuthorID == 0 && lastReview != nil && lastReview.Type != issues_model.ReviewTypeRequest {
			return nil
		}

		return issues_model.ErrNotValidReviewRequest{
			Reason: "Doer can't choose reviewer",
			UserID: doer.ID,
			RepoID: issue.Repo.ID,
		}
	}

	if canDoerChangeReviewRequests {
		return nil
	}

	if lastReview != nil && lastReview.Type == issues_model.ReviewTypeRequest && lastReview.ReviewerID == doer.ID {
		return nil
	}

	return issues_model.ErrNotValidReviewRequest{
		Reason: "Doer can't remove reviewer",
		UserID: doer.ID,
		RepoID: issue.Repo.ID,
	}
}

func ReviewRequestNotify(ctx context.Context, issue *issues_model.Issue, doer *user_model.User, reviewNotifiers []*ReviewRequestNotifier) {
	for _, reviewNotifier := range reviewNotifiers {
		if reviewNotifier.Reviewer != nil {
			notify_service.PullRequestReviewRequest(ctx, issue.Poster, issue, reviewNotifier.Reviewer, reviewNotifier.IsAdd, reviewNotifier.Comment)
		}
	}
}

// CanDoerChangeReviewRequests returns if the doer can add/remove review requests of a PR,
// which any signed-in user can on a repository that isn't archived
func CanDoerChangeReviewRequests(_ context.Context, doer *user_model.User, repo *repo_model.Repository, _ int64) bool {
	return !repo.IsArchived && doer != nil && doer.ID > 0
}
