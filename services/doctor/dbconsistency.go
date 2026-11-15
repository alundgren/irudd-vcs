// Copyright 2020 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package doctor

import (
	"context"

	"gitea.dev/models/db"
	issues_model "gitea.dev/models/issues"
	"gitea.dev/models/migrations"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/modules/log"
	issue_service "gitea.dev/services/issue"
)

type consistencyCheck struct {
	Name         string
	Counter      func(context.Context) (int64, error)
	Fixer        func(context.Context) (int64, error)
	FixedMessage string
}

func (c *consistencyCheck) Run(ctx context.Context, logger log.Logger, autofix bool) error {
	count, err := c.Counter(ctx)
	if err != nil {
		logger.Critical("Error: %v whilst counting %s", err, c.Name)
		return err
	}
	if count > 0 {
		if autofix {
			var fixed int64
			if fixed, err = c.Fixer(ctx); err != nil {
				logger.Critical("Error: %v whilst fixing %s", err, c.Name)
				return err
			}

			prompt := "Deleted"
			if c.FixedMessage != "" {
				prompt = c.FixedMessage
			}

			if fixed < 0 {
				logger.Info(prompt+" %d %s", count, c.Name)
			} else {
				logger.Info(prompt+" %d/%d %s", fixed, count, c.Name)
			}
		} else {
			logger.Warn("Found %d %s", count, c.Name)
		}
	}
	return nil
}

func asFixer(fn func(ctx context.Context) error) func(ctx context.Context) (int64, error) {
	return func(ctx context.Context) (int64, error) {
		err := fn(ctx)
		return -1, err
	}
}

func genericOrphanCheck(name, subject, refObject, joinCond string) consistencyCheck {
	return consistencyCheck{
		Name: name,
		Counter: func(ctx context.Context) (int64, error) {
			return db.CountOrphanedObjects(ctx, subject, refObject, joinCond)
		},
		Fixer: func(ctx context.Context) (int64, error) {
			err := db.DeleteOrphanedObjects(ctx, subject, refObject, joinCond)
			return -1, err
		},
	}
}

func prepareDBConsistencyChecks() []consistencyCheck {
	consistencyChecks := []consistencyCheck{
		{
			// find labels without existing repo or org
			Name:    "Orphaned Labels without existing repository or organisation",
			Counter: issues_model.CountOrphanedLabels,
			Fixer:   asFixer(issues_model.DeleteOrphanedLabels),
		},
		{
			// find IssueLabels without existing label
			Name:    "Orphaned Issue Labels without existing label",
			Counter: issues_model.CountOrphanedIssueLabels,
			Fixer:   asFixer(issues_model.DeleteOrphanedIssueLabels),
		},
		{
			// find issues without existing repository
			Name:    "Orphaned Issues without existing repository",
			Counter: issues_model.CountOrphanedIssues,
			Fixer:   asFixer(issue_service.DeleteOrphanedIssues),
		},
		// find releases without existing repository
		genericOrphanCheck("Orphaned Releases without existing repository",
			"release", "repository", "`release`.repo_id=repository.id"),
		// find pulls without existing issues
		genericOrphanCheck("Orphaned PullRequests without existing issue",
			"pull_request", "issue", "pull_request.issue_id=issue.id"),
		// find pull requests without base repository
		genericOrphanCheck("Pull request entries without existing base repository",
			"pull_request", "repository", "pull_request.base_repo_id=repository.id"),
		// find null archived repositories
		{
			Name:         "Repositories with is_archived IS NULL",
			Counter:      repo_model.CountNullArchivedRepository,
			Fixer:        repo_model.FixNullArchivedRepository,
			FixedMessage: "Fixed",
		},
		// find label comments with empty labels
		{
			Name:         "Label comments with empty labels",
			Counter:      issues_model.CountCommentTypeLabelWithEmptyLabel,
			Fixer:        issues_model.FixCommentTypeLabelWithEmptyLabel,
			FixedMessage: "Fixed",
		},
		// find label comments with labels from outside the repository
		{
			Name:         "Label comments with labels from outside the repository",
			Counter:      issues_model.CountCommentTypeLabelWithOutsideLabels,
			Fixer:        issues_model.FixCommentTypeLabelWithOutsideLabels,
			FixedMessage: "Removed",
		},
		// find issue_label with labels from outside the repository
		{
			Name:         "IssueLabels with Labels from outside the repository",
			Counter:      issues_model.CountIssueLabelWithOutsideLabels,
			Fixer:        issues_model.FixIssueLabelWithOutsideLabels,
			FixedMessage: "Removed",
		},
	}

	// TODO: function to recalc all counters

	consistencyChecks = append(consistencyChecks,
		// find protected branches without existing repository
		genericOrphanCheck("Protected Branches without existing repository",
			"protected_branch", "repository", "protected_branch.repo_id=repository.id"),
		// find branches without existing repository
		genericOrphanCheck("Branches without existing repository",
			"branch", "repository", "branch.repo_id=repository.id"),
		// find LFS locks without existing repository
		genericOrphanCheck("LFS locks without existing repository",
			"lfs_lock", "repository", "lfs_lock.repo_id=repository.id"),
		// find collaborations without users
		genericOrphanCheck("Collaborations without existing user",
			"collaboration", "user", "collaboration.user_id=`user`.id"),
		// find collaborations without repository
		genericOrphanCheck("Collaborations without existing repository",
			"collaboration", "repository", "collaboration.repo_id=repository.id"),
		// find access without users
		genericOrphanCheck("Access entries without existing user",
			"access", "user", "access.user_id=`user`.id"),
		// find access without repository
		genericOrphanCheck("Access entries without existing repository",
			"access", "repository", "access.repo_id=repository.id"),
		// find redirects without existing user.
		genericOrphanCheck("Orphaned Redirects without existing redirect user",
			"user_redirect", "user", "user_redirect.redirect_user_id=`user`.id"),
	)
	return consistencyChecks
}

func checkDBConsistency(ctx context.Context, logger log.Logger, autofix bool) error {
	// make sure DB version is uptodate
	if err := db.InitEngineWithMigration(ctx, migrations.EnsureUpToDate); err != nil {
		logger.Critical("Model version on the database does not match the current Gitea version. Model consistency will not be checked until the database is upgraded")
		return err
	}
	consistencyChecks := prepareDBConsistencyChecks()
	for _, c := range consistencyChecks {
		if err := c.Run(ctx, logger, autofix); err != nil {
			return err
		}
	}

	return nil
}

func init() {
	Register(&Check{
		Title:     "Check consistency of database",
		Name:      "check-db-consistency",
		IsDefault: false,
		Run:       checkDBConsistency,
		Priority:  3,
	})
}
