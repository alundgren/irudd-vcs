// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"strings"
	"time"

	"gitea.dev/modules/glob"
	"gitea.dev/modules/log"
	"gitea.dev/modules/structs"
)

// Service settings
var Service = struct {
	DefaultUserVisibility           string
	DefaultUserVisibilityMode       structs.VisibleType
	AllowedUserVisibilityModes      []string
	AllowedUserVisibilityModesSlice AllowedVisibility `ini:"-"`
	EmailDomainAllowList            []glob.Glob
	EmailDomainBlockList            []glob.Glob
	DefaultKeepEmailPrivate         bool
	DefaultAllowCreateOrganization  bool
	DefaultUserIsRestricted         bool
	NoReplyAddress                  string
	DefaultOrgMemberVisible         bool
	UserDeleteWithCommentsMaxTime   time.Duration
	ValidSiteURLSchemes             []string
}{
	AllowedUserVisibilityModesSlice: []bool{true, true, true},
}

// AllowedVisibility store in a 3 item bool array what is allowed
type AllowedVisibility []bool

// IsAllowedVisibility check if a AllowedVisibility allow a specific VisibleType
func (a AllowedVisibility) IsAllowedVisibility(t structs.VisibleType) bool {
	if int(t) >= len(a) {
		return false
	}
	return a[t]
}

func CompileEmailGlobList(sec ConfigSection, keys ...string) (globs []glob.Glob) {
	for _, key := range keys {
		list := sec.Key(key).Strings(",")
		for _, s := range list {
			if g, err := glob.Compile(s); err == nil {
				globs = append(globs, g)
			} else {
				log.Error("Skip invalid email allow/block list expression %q: %v", s, err)
			}
		}
	}
	return globs
}

func loadServiceFrom(rootCfg ConfigProvider) {
	sec := rootCfg.Section("service")
	if sec.HasKey("EMAIL_DOMAIN_WHITELIST") {
		deprecatedSetting(rootCfg, "service", "EMAIL_DOMAIN_WHITELIST", "service", "EMAIL_DOMAIN_ALLOWLIST", "1.21")
	}
	Service.EmailDomainAllowList = CompileEmailGlobList(sec, "EMAIL_DOMAIN_WHITELIST", "EMAIL_DOMAIN_ALLOWLIST")
	Service.EmailDomainBlockList = CompileEmailGlobList(sec, "EMAIL_DOMAIN_BLOCKLIST")

	Service.DefaultKeepEmailPrivate = sec.Key("DEFAULT_KEEP_EMAIL_PRIVATE").MustBool()
	Service.DefaultAllowCreateOrganization = sec.Key("DEFAULT_ALLOW_CREATE_ORGANIZATION").MustBool(true)
	Service.DefaultUserIsRestricted = sec.Key("DEFAULT_USER_IS_RESTRICTED").MustBool(false)
	Service.NoReplyAddress = sec.Key("NO_REPLY_ADDRESS").MustString("noreply." + Domain)
	modes := sec.Key("ALLOWED_USER_VISIBILITY_MODES").Strings(",")
	if len(modes) != 0 {
		Service.AllowedUserVisibilityModes = []string{}
		Service.AllowedUserVisibilityModesSlice = []bool{false, false, false}
		for _, sMode := range modes {
			if tp, ok := structs.VisibilityModes[sMode]; ok { // remove unsupported modes
				Service.AllowedUserVisibilityModes = append(Service.AllowedUserVisibilityModes, sMode)
				Service.AllowedUserVisibilityModesSlice[tp] = true
			} else {
				log.Warn("ALLOWED_USER_VISIBILITY_MODES %s is unsupported", sMode)
			}
		}
	}

	if len(Service.AllowedUserVisibilityModes) == 0 {
		Service.AllowedUserVisibilityModes = []string{"public", "limited", "private"}
		Service.AllowedUserVisibilityModesSlice = []bool{true, true, true}
	}

	Service.DefaultUserVisibility = sec.Key("DEFAULT_USER_VISIBILITY").String()
	if Service.DefaultUserVisibility == "" {
		Service.DefaultUserVisibility = Service.AllowedUserVisibilityModes[0]
	} else if !Service.AllowedUserVisibilityModesSlice[structs.VisibilityModes[Service.DefaultUserVisibility]] {
		log.Warn("DEFAULT_USER_VISIBILITY %s is wrong or not in ALLOWED_USER_VISIBILITY_MODES, using first allowed", Service.DefaultUserVisibility)
		Service.DefaultUserVisibility = Service.AllowedUserVisibilityModes[0]
	}
	Service.DefaultUserVisibilityMode = structs.VisibilityModes[Service.DefaultUserVisibility]
	Service.DefaultOrgMemberVisible = sec.Key("DEFAULT_ORG_MEMBER_VISIBLE").MustBool()
	Service.UserDeleteWithCommentsMaxTime = sec.Key("USER_DELETE_WITH_COMMENTS_MAX_TIME").MustDuration(0)
	sec.Key("VALID_SITE_URL_SCHEMES").MustString("http,https")
	Service.ValidSiteURLSchemes = sec.Key("VALID_SITE_URL_SCHEMES").Strings(",")
	schemes := make([]string, 0, len(Service.ValidSiteURLSchemes))
	for _, scheme := range Service.ValidSiteURLSchemes {
		scheme = strings.ToLower(strings.TrimSpace(scheme))
		if scheme != "" {
			schemes = append(schemes, scheme)
		}
	}
	Service.ValidSiteURLSchemes = schemes
}
