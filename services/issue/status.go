// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package issue

import (
	"context"

	"gitea.dev/models/db"
	issues_model "gitea.dev/models/issues"
	user_model "gitea.dev/models/user"
	notify_service "gitea.dev/services/notify"
)

// CloseIssue close an issue.
func CloseIssue(ctx context.Context, issue *issues_model.Issue, doer *user_model.User, commitID string) error {
	var comment *issues_model.Comment
	if err := db.WithTx(ctx, func(ctx context.Context) error {
		var err error
		comment, err = issues_model.CloseIssue(ctx, issue, doer)
		return err
	}); err != nil {
		return err
	}

	notify_service.IssueChangeStatus(ctx, doer, commitID, issue, comment, true)

	return nil
}

// ReopenIssue reopen an issue.
func ReopenIssue(ctx context.Context, issue *issues_model.Issue, doer *user_model.User, commitID string) error {
	comment, err := issues_model.ReopenIssue(ctx, issue, doer)
	if err != nil {
		return err
	}

	notify_service.IssueChangeStatus(ctx, doer, commitID, issue, comment, false)

	return nil
}
