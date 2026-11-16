// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package gitrepo

import (
	"context"

	"gitea.dev/modules/git"
)

// CloneRepoToLocal clones a managed repository to a local path.
func CloneRepoToLocal(ctx context.Context, fromRepo Repository, toLocalPath string, opts git.CloneRepoOptions) error {
	return git.Clone(ctx, repoPath(fromRepo), toLocalPath, opts)
}

func Clone(ctx context.Context, fromRepo, toRepo Repository, opts git.CloneRepoOptions) error {
	return git.Clone(ctx, repoPath(fromRepo), repoPath(toRepo), opts)
}
