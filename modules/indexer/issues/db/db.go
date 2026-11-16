// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"strings"

	"gitea.dev/models/db"
	issue_model "gitea.dev/models/issues"
	"gitea.dev/modules/indexer"
	"gitea.dev/modules/indexer/issues/internal"
	"gitea.dev/modules/util"

	"xorm.io/builder"
)

func buildMatchQuery(mode indexer.SearchModeType, colName, keyword string) builder.Cond {
	if mode == indexer.SearchModeExact {
		return db.BuildCaseInsensitiveLike(colName, keyword)
	}

	// match words
	cond := builder.NewCond()
	fields := strings.Fields(keyword)
	if len(fields) == 0 {
		return builder.Expr("1=1")
	}
	for _, field := range fields {
		if field == "" {
			continue
		}
		cond = cond.And(db.BuildCaseInsensitiveLike(colName, field))
	}
	return cond
}

// Search searches for issues
func Search(ctx context.Context, options *internal.SearchOptions) (*internal.SearchResult, error) {
	cond := builder.NewCond()

	if options.Keyword != "" {
		repoCond := builder.In("repo_id", options.RepoIDs)
		if len(options.RepoIDs) == 1 {
			repoCond = builder.Eq{"repo_id": options.RepoIDs[0]}
		}
		subQuery := builder.Select("id").From("issue").Where(repoCond)
		searchMode := util.IfZero(options.SearchMode, indexer.SearchModeExact)
		cond = builder.Or(
			buildMatchQuery(searchMode, "issue.name", options.Keyword),
			buildMatchQuery(searchMode, "issue.content", options.Keyword),
			builder.In("issue.id", builder.Select("issue_id").
				From("comment").
				Where(builder.And(
					builder.Eq{"type": issue_model.CommentTypeComment},
					builder.In("issue_id", subQuery),
					buildMatchQuery(searchMode, "content", options.Keyword),
				)),
			),
		)

		if options.IsKeywordNumeric() {
			cond = cond.Or(
				builder.Eq{"`index`": options.Keyword},
			)
		}
	}

	opt, err := ToDBOptions(ctx, options)
	if err != nil {
		return nil, err
	}

	// If pagesize == 0, return total count only. It's a special case for search count.
	if options.Paginator != nil && options.Paginator.PageSize == 0 {
		total, err := issue_model.CountIssues(ctx, opt, cond)
		if err != nil {
			return nil, err
		}
		return &internal.SearchResult{
			Total: total,
		}, nil
	}

	return findWithIssueOptions(ctx, opt, cond)
}

func findWithIssueOptions(ctx context.Context, opt *issue_model.IssuesOptions, otherConds ...builder.Cond) (*internal.SearchResult, error) {
	ids, total, err := issue_model.IssueIDs(ctx, opt, otherConds...)
	if err != nil {
		return nil, err
	}

	hits := make([]internal.Match, 0, len(ids))
	for _, id := range ids {
		hits = append(hits, internal.Match{
			ID: id,
		})
	}
	return &internal.SearchResult{
		Total: total,
		Hits:  hits,
	}, nil
}
