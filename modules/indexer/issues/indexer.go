// Copyright 2018 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package issues

import (
	"context"

	db_model "gitea.dev/models/db"
	"gitea.dev/modules/indexer/issues/db"
	"gitea.dev/modules/indexer/issues/internal"
)

// SearchOptions indicates the options for searching issues
type SearchOptions = internal.SearchOptions

const (
	SortByCreatedDesc  = internal.SortByCreatedDesc
	SortByUpdatedDesc  = internal.SortByUpdatedDesc
	SortByCommentsDesc = internal.SortByCommentsDesc
	SortByDeadlineDesc = internal.SortByDeadlineDesc
	SortByCreatedAsc   = internal.SortByCreatedAsc
	SortByUpdatedAsc   = internal.SortByUpdatedAsc
	SortByCommentsAsc  = internal.SortByCommentsAsc
	SortByDeadlineAsc  = internal.SortByDeadlineAsc
)

// SearchIssues search issues by options, directly against the database.
func SearchIssues(ctx context.Context, opts *SearchOptions) ([]int64, int64, error) {
	result, err := db.Search(ctx, opts)
	if err != nil {
		return nil, 0, err
	}
	return SearchResultToIDSlice(result), result.Total, nil
}

func SearchResultToIDSlice(result *internal.SearchResult) []int64 {
	ret := make([]int64, 0, len(result.Hits))
	for _, hit := range result.Hits {
		ret = append(ret, hit.ID)
	}
	return ret
}

// CountIssues counts issues by options. It is a shortcut of SearchIssues(ctx, opts) but only returns the total count.
func CountIssues(ctx context.Context, opts *SearchOptions) (int64, error) {
	opts = opts.Copy(func(options *SearchOptions) { options.Paginator = &db_model.ListOptions{PageSize: 0} })

	_, total, err := SearchIssues(ctx, opts)
	return total, err
}
