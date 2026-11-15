// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package git

import (
	"context"

	"gitea.dev/modules/git/gitcmd"
)

// GetRemoteAddress returns remote url of git repository in the repoPath with special remote name
func GetRemoteAddress(ctx context.Context, repoPath, remoteName string) (string, error) {
	cmd := gitcmd.NewCommand("remote", "get-url").AddDynamicArguments(remoteName)
	result, _, err := cmd.WithDir(repoPath).RunStdString(ctx)
	if err != nil {
		return "", err
	}

	if len(result) > 0 {
		result = result[:len(result)-1]
	}
	return result, nil
}
