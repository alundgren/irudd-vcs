// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"gitea.dev/modules/container"
	"gitea.dev/modules/log"
)

// Admin settings
var Admin struct {
	UserDisabledFeatures container.Set[string]
}

var validUserFeatures = container.SetOf(
	UserFeatureManageCredentials,
)

func loadAdminFrom(rootCfg ConfigProvider) {
	sec := rootCfg.Section("admin")
	Admin.UserDisabledFeatures = container.SetOf(sec.Key("USER_DISABLED_FEATURES").Strings(",")...)

	for feature := range Admin.UserDisabledFeatures {
		if !validUserFeatures.Contains(feature) {
			log.Warn("USER_DISABLED_FEATURES contains unknown feature %q", feature)
		}
	}
}

// UserFeatureManageCredentials is the only user feature still enforced by the API (managing email addresses)
const UserFeatureManageCredentials = "manage_credentials"
