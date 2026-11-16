// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package setting

import (
	"os/exec"
	"path/filepath"
	"strings"

	"gitea.dev/modules/log"
)

// Repository settings
var (
	Repository = struct {
		DetectedCharsetsOrder                   []string
		DetectedCharsetScore                    map[string]int `ini:"-"`
		AnsiCharset                             string
		ForcePrivate                            bool
		DefaultPushCreatePrivate                bool
		MaxCreationLimit                        int
		UserMaxCreationLimit                    int
		OrgMaxCreationLimit                     int
		DisableHTTPGit                          bool
		AccessControlAllowOrigin                string
		DefaultCloseIssuesViaCommitsInAnyBranch bool
		EnablePushCreateUser                    bool
		EnablePushCreateOrg                     bool
		DisabledRepoUnits                       []string
		DefaultRepoUnits                        []string
		DefaultForkRepoUnits                    []string
		DefaultTemplateRepoUnits                []string
		DefaultBranch                           string
		AllowForkWithoutMaximumLimit            bool
		AllowForkIntoSameOwner                  bool

		// Pull request settings
		PullRequest struct {
			WorkInProgressPrefixes                   []string
			DefaultMergeStyle                        string
			DefaultMergeMessageCommitsLimit          int
			DefaultMergeMessageSize                  int
			DefaultMergeMessageAllAuthors            bool
			DefaultMergeMessageMaxApprovers          int
			DefaultMergeMessageOfficialApproversOnly bool
			PopulateSquashCommentWithCommitMessages  bool
			AddCoCommitterTrailers                   bool
			RetargetChildrenOnMerge                  bool
			DelayCheckForInactiveDays                int
			DefaultDeleteBranchAfterMerge            bool
		} `ini:"repository.pull-request"`
	}{
		DetectedCharsetsOrder: []string{
			"UTF-8",
			"UTF-16BE",
			"UTF-16LE",
			"UTF-32BE",
			"UTF-32LE",
			"ISO-8859-1",
			"windows-1252",
			"ISO-8859-2",
			"windows-1250",
			"ISO-8859-5",
			"ISO-8859-6",
			"ISO-8859-7",
			"windows-1253",
			"ISO-8859-8-I",
			"windows-1255",
			"ISO-8859-8",
			"windows-1251",
			"windows-1256",
			"KOI8-R",
			"ISO-8859-9",
			"windows-1254",
			"Shift_JIS",
			"GB18030",
			"EUC-JP",
			"EUC-KR",
			"Big5",
			"ISO-2022-JP",
			"ISO-2022-KR",
			"ISO-2022-CN",
			"IBM424_rtl",
			"IBM424_ltr",
			"IBM420_rtl",
			"IBM420_ltr",
		},
		DetectedCharsetScore:                    map[string]int{},
		AnsiCharset:                             "",
		ForcePrivate:                            false,
		DefaultPushCreatePrivate:                true,
		MaxCreationLimit:                        -1,
		UserMaxCreationLimit:                    -1,
		OrgMaxCreationLimit:                     -1,
		DisableHTTPGit:                          false,
		AccessControlAllowOrigin:                "",
		DefaultCloseIssuesViaCommitsInAnyBranch: false,
		EnablePushCreateUser:                    false,
		EnablePushCreateOrg:                     false,
		DisabledRepoUnits:                       []string{},
		DefaultRepoUnits:                        []string{},
		DefaultForkRepoUnits:                    []string{},
		DefaultTemplateRepoUnits:                []string{},
		DefaultBranch:                           "main",
		AllowForkWithoutMaximumLimit:            true,

		// Pull request settings
		PullRequest: struct {
			WorkInProgressPrefixes                   []string
			DefaultMergeStyle                        string
			DefaultMergeMessageCommitsLimit          int
			DefaultMergeMessageSize                  int
			DefaultMergeMessageAllAuthors            bool
			DefaultMergeMessageMaxApprovers          int
			DefaultMergeMessageOfficialApproversOnly bool
			PopulateSquashCommentWithCommitMessages  bool
			AddCoCommitterTrailers                   bool
			RetargetChildrenOnMerge                  bool
			DelayCheckForInactiveDays                int
			DefaultDeleteBranchAfterMerge            bool
		}{
			WorkInProgressPrefixes:                   []string{"WIP:", "[WIP]"},
			DefaultMergeStyle:                        "merge",
			DefaultMergeMessageCommitsLimit:          50,
			DefaultMergeMessageSize:                  5 * 1024,
			DefaultMergeMessageAllAuthors:            false,
			DefaultMergeMessageMaxApprovers:          10,
			DefaultMergeMessageOfficialApproversOnly: true,
			PopulateSquashCommentWithCommitMessages:  false,
			AddCoCommitterTrailers:                   true,
			RetargetChildrenOnMerge:                  true,
			DelayCheckForInactiveDays:                7,
		},
	}
	RepoRootPath string
	ScriptType   = "bash"
)

func loadRepositoryFrom(rootCfg ConfigProvider) {
	var err error
	// Determine and create root git repository path.
	sec := rootCfg.Section("repository")
	Repository.DisableHTTPGit = sec.Key("DISABLE_HTTP_GIT").MustBool()
	// MAX_CREATION_LIMIT is a shortcut that sets the default for the two per-type limits below.
	// USER_/ORG_MAX_CREATION_LIMIT take precedence when explicitly set.
	Repository.MaxCreationLimit = sec.Key("MAX_CREATION_LIMIT").MustInt(-1)
	Repository.UserMaxCreationLimit = sec.Key("USER_MAX_CREATION_LIMIT").MustInt(Repository.MaxCreationLimit)
	Repository.OrgMaxCreationLimit = sec.Key("ORG_MAX_CREATION_LIMIT").MustInt(Repository.MaxCreationLimit)
	Repository.DefaultBranch = sec.Key("DEFAULT_BRANCH").MustString(Repository.DefaultBranch)
	RepoRootPath = sec.Key("ROOT").MustString(filepath.Join(AppDataPath, "gitea-repositories"))
	if !filepath.IsAbs(RepoRootPath) {
		RepoRootPath = filepath.Join(AppWorkPath, RepoRootPath)
	} else {
		RepoRootPath = filepath.Clean(RepoRootPath)
	}

	checkOverlappedPath("[repository].ROOT", RepoRootPath)

	defaultDetectedCharsetsOrder := make([]string, 0, len(Repository.DetectedCharsetsOrder))
	for _, charset := range Repository.DetectedCharsetsOrder {
		defaultDetectedCharsetsOrder = append(defaultDetectedCharsetsOrder, strings.ToLower(strings.TrimSpace(charset)))
	}
	ScriptType = sec.Key("SCRIPT_TYPE").MustString("bash")

	if _, err := exec.LookPath(ScriptType); err != nil {
		log.Warn("SCRIPT_TYPE %q is not on the current PATH. Are you sure that this is the correct SCRIPT_TYPE?", ScriptType)
	}

	if err = sec.MapTo(&Repository); err != nil {
		log.Fatal("Failed to map Repository settings: %v", err)
	} else if err = rootCfg.Section("repository.pull-request").MapTo(&Repository.PullRequest); err != nil {
		log.Fatal("Failed to map Repository.PullRequest settings: %v", err)
	}

	// Handle preferred charset orders
	preferred := make([]string, 0, len(Repository.DetectedCharsetsOrder))
	for _, charset := range Repository.DetectedCharsetsOrder {
		canonicalCharset := strings.ToLower(strings.TrimSpace(charset))
		preferred = append(preferred, canonicalCharset)
		// remove it from the defaults
		for i, charset := range defaultDetectedCharsetsOrder {
			if charset == canonicalCharset {
				defaultDetectedCharsetsOrder = append(defaultDetectedCharsetsOrder[:i], defaultDetectedCharsetsOrder[i+1:]...)
				break
			}
		}
	}

	i := 0
	for _, charset := range preferred {
		// Add the defaults
		if charset == "defaults" {
			for _, charset := range defaultDetectedCharsetsOrder {
				canonicalCharset := strings.ToLower(strings.TrimSpace(charset))
				if _, has := Repository.DetectedCharsetScore[canonicalCharset]; !has {
					Repository.DetectedCharsetScore[canonicalCharset] = i
					i++
				}
			}
			continue
		}
		if _, has := Repository.DetectedCharsetScore[charset]; !has {
			Repository.DetectedCharsetScore[charset] = i
			i++
		}
	}
}
