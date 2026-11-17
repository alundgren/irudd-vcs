// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package gitrepo

import (
	"context"
	"fmt"
	"strings"

	"gitea.dev/modules/git"
	"gitea.dev/modules/git/gitcmd"
	"gitea.dev/modules/util"

	lru "github.com/hashicorp/golang-lru/v2"
)

// mergeBases maps two commit IDs to their merge base, "" if none, which the IDs fix since replace objects
// are disabled; the PR commits API asks for it on every request
var mergeBases, _ = lru.New[string, string](4096)

// MergeBase checks and returns merge base of two commits.
func MergeBase(ctx context.Context, repo Repository, baseCommitID, headCommitID string) (string, error) {
	cacheable := git.IsFullCommitID(baseCommitID) && git.IsFullCommitID(headCommitID)
	key := baseCommitID + " " + headCommitID
	if mergeBase, ok := mergeBases.Get(key); ok && cacheable {
		if mergeBase == "" {
			return "", util.NewNotExistErrorf("merge-base for %s and %s doesn't exist", baseCommitID, headCommitID)
		}
		return mergeBase, nil
	}
	mergeBase, stderr, err := RunCmdString(ctx, repo, gitcmd.NewCommand("merge-base").
		AddDashesAndList(baseCommitID, headCommitID))
	if err != nil {
		if gitcmd.IsErrorExitCode(err, 1) && strings.TrimSpace(stderr) == "" {
			if cacheable {
				mergeBases.Add(key, "")
			}
			return "", util.NewNotExistErrorf("merge-base for %s and %s doesn't exist", baseCommitID, headCommitID)
		}
		return "", fmt.Errorf("get merge-base of %s and %s failed: %w", baseCommitID, headCommitID, err)
	}
	mergeBase = strings.TrimSpace(mergeBase)
	if cacheable {
		mergeBases.Add(key, mergeBase)
	}
	return mergeBase, nil
}
