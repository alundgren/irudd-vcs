// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package gitrepo

import (
	"context"

	"gitea.dev/modules/git"
)

// Push pushes from one managed repository to another managed repository.
func Push(ctx context.Context, fromRepo, toRepo Repository, opts git.PushOptions) error {
	opts.Remote = repoPath(toRepo)
	return git.Push(ctx, repoPath(fromRepo), opts)
}

// PushFromLocal pushes from a local path to a managed repository.
func PushFromLocal(ctx context.Context, fromLocalPath string, toRepo Repository, opts git.PushOptions) error {
	opts.Remote = repoPath(toRepo)
	return git.Push(ctx, fromLocalPath, opts)
}
