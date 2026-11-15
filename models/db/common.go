// Copyright 2022 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package db

import (
	"gitea.dev/modules/util"

	"xorm.io/builder"
)

// BuildCaseInsensitiveLike returns a case-insensitive LIKE condition for the given key and value.
// Cast the search value and the database column value to the same case for case-insensitive matching.
// Only cast ASCII chars because SQLite doesn't handle complete Unicode case folding.
func BuildCaseInsensitiveLike(key, value string) builder.Cond {
	// ToLowerASCII is about 7% faster than ToUpperASCII (according to Golang's benchmark)
	return builder.Like{"LOWER(" + key + ")", util.ToLowerASCII(value)}
}

// BuildCaseInsensitiveIn returns a condition to check if the given value is in the given values case-insensitively.
// See BuildCaseInsensitiveLike for more details
func BuildCaseInsensitiveIn(key string, values []string) builder.Cond {
	incaseValues := make([]string, len(values))
	for i, value := range values {
		incaseValues[i] = util.ToLowerASCII(value)
	}
	return builder.In("LOWER("+key+")", incaseValues)
}
