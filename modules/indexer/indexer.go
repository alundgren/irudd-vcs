// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package indexer

type SearchModeType string

const (
	SearchModeExact  SearchModeType = "exact"
	SearchModeWords  SearchModeType = "words"
	SearchModeFuzzy  SearchModeType = "fuzzy"
	SearchModeRegexp SearchModeType = "regexp"
)
