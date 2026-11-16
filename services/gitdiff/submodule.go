// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package gitdiff

import (
	"context"
	"html/template"

	"gitea.dev/modules/base"
	"gitea.dev/modules/git"
	"gitea.dev/modules/htmlutil"
)

type SubmoduleDiffInfo struct {
	SubmoduleName string
	SubmoduleFile *git.CommitSubmoduleFile // it might be nil if the submodule is not found or unable to parse
	NewRefID      string
	PreviousRefID string
}

func (si *SubmoduleDiffInfo) CommitRefIDLinkHTML(ctx context.Context, commitID string) template.HTML {
	webLink := si.SubmoduleFile.SubmoduleWebLinkTree(ctx, commitID)
	if webLink == nil {
		return htmlutil.HTMLFormat("%s", base.ShortSha(commitID))
	}
	return htmlutil.HTMLFormat(`<a href="%s">%s</a>`, webLink.CommitWebLink, base.ShortSha(commitID))
}

func (si *SubmoduleDiffInfo) CompareRefIDLinkHTML(ctx context.Context) template.HTML {
	webLink := si.SubmoduleFile.SubmoduleWebLinkCompare(ctx, si.PreviousRefID, si.NewRefID)
	if webLink == nil {
		return htmlutil.HTMLFormat("%s...%s", base.ShortSha(si.PreviousRefID), base.ShortSha(si.NewRefID))
	}
	return htmlutil.HTMLFormat(`<a href="%s">%s...%s</a>`, webLink.CommitWebLink, base.ShortSha(si.PreviousRefID), base.ShortSha(si.NewRefID))
}

func (si *SubmoduleDiffInfo) SubmoduleRepoLinkHTML(ctx context.Context) template.HTML {
	webLink := si.SubmoduleFile.SubmoduleWebLinkTree(ctx)
	if webLink == nil {
		return htmlutil.HTMLFormat("%s", si.SubmoduleName)
	}
	return htmlutil.HTMLFormat(`<a href="%s">%s</a>`, webLink.RepoWebLink, si.SubmoduleName)
}
