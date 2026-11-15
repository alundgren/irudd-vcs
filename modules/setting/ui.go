// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"gitea.dev/modules/container"
)

// UI settings, only the ones that still affect API and git behaviour
var UI = struct {
	FeedMaxCommitNum   int // max commits in a push notification (webhook payload)
	CodeCommentLines   int
	ReactionMaxUserNum int
	MaxDisplayFileSize int64
	Reactions          []string
	ReactionsLookup    container.Set[string] `ini:"-"`

	// TODO: DefaultShowFullName is introduced by https://github.com/go-gitea/gitea/pull/6710
	// But there are still many edge cases:
	// * Many places still use "username", not respecting this setting
	// * Many places use "Full Name" if it is not empty, cause inconsistent UI for users who have set their full name but some others don't
	// * Even if DefaultShowFullName=false, many places still need to show the full name
	// For most cases, either "username" or "username (Full Name)" should be used and are good enough.
	// Only in very few cases (e.g.: unimportant lists, narrow layout), "username" or "Full Name" can be used.
	DefaultShowFullName bool
}{
	FeedMaxCommitNum:   5,
	CodeCommentLines:   4,
	ReactionMaxUserNum: 10,
	MaxDisplayFileSize: 8388608,
	Reactions:          []string{`+1`, `-1`, `laugh`, `hooray`, `confused`, `heart`, `rocket`, `eyes`},
}

func loadUIFrom(rootCfg ConfigProvider) {
	mustMapSetting(rootCfg, "ui", &UI)
	sec := rootCfg.Section("ui")
	UI.DefaultShowFullName = sec.Key("DEFAULT_SHOW_FULL_NAME").MustBool(false)

	UI.ReactionsLookup = make(container.Set[string])
	for _, reaction := range UI.Reactions {
		UI.ReactionsLookup.Add(reaction)
	}
}
