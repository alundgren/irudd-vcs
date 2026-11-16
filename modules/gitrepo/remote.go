// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package gitrepo

import (
	"context"

	"gitea.dev/modules/git"
	giturl "gitea.dev/modules/git/url"
	"gitea.dev/modules/util"
)

// GitRemoteGetURL returns the url of a specific remote of the repository.
func GitRemoteGetURL(ctx context.Context, repo Repository, remoteName string) (*giturl.GitURL, error) {
	addr, err := git.GetRemoteAddress(ctx, repoPath(repo), remoteName)
	if err != nil {
		return nil, err
	}
	if addr == "" {
		return nil, util.NewNotExistErrorf("remote '%s' does not exist", remoteName)
	}
	return giturl.ParseGitURL(addr)
}
