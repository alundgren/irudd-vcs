// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package git

import (
	"context"
	"errors"
	"fmt"

	repo_model "gitea.dev/models/repo"
	"gitea.dev/modules/git"
	"gitea.dev/modules/gitrepo"
	"gitea.dev/modules/util"
)

// CompareInfo represents needed information for comparing references.
type CompareInfo struct {
	BaseRepo     *repo_model.Repository
	BaseRef      git.RefName
	BaseCommitID string
	HeadRepo     *repo_model.Repository
	HeadGitRepo  *git.Repository
	HeadRef      git.RefName
	HeadCommitID string

	CompareSeparator string

	// CompareBase is the left-side commit ID used for comparing
	// for "...": it is merge base (empty for no merge base)
	// for direct comparison "..": it is base commit ID
	CompareBase string

	Commits []*git.Commit
}

// GetCompareInfo generates and returns compare information between base and head branches of repositories.
// It does its best to fill the fields as many as it can.
// MergeBase can be empty if the base and head are unrelated.
func GetCompareInfo(ctx context.Context, baseRepo, headRepo *repo_model.Repository, headGitRepo *git.Repository, baseRef, headRef git.RefName, directComparison, fileOnly bool) (compareInfo CompareInfo, err error) {
	// the head repository is open, so its cat-file batch resolves refs without a rev-parse process each
	resolve := func(repo *repo_model.Repository, ref git.RefName) (string, error) {
		if repo.ID != headRepo.ID {
			return gitrepo.GetFullCommitID(ctx, repo, ref.String())
		}
		id, err := headGitRepo.ConvertToGitID(ref.String())
		if err != nil {
			return "", err
		}
		return id.String(), nil
	}
	baseCommitID, err1 := resolve(baseRepo, baseRef)
	headCommitID, err2 := resolve(headRepo, headRef)
	compareInfo = CompareInfo{
		BaseRepo:         baseRepo,
		BaseRef:          baseRef,
		BaseCommitID:     baseCommitID,
		HeadRepo:         headRepo,
		HeadGitRepo:      headGitRepo,
		HeadRef:          headRef,
		HeadCommitID:     headCommitID,
		CompareSeparator: util.Iif(directComparison, "..", "..."),
	}
	if err1 != nil || err2 != nil {
		return compareInfo, errors.Join(err1, err2)
	}

	// if they are not the same repository, then we need to fetch the base commit into the head repository
	// because we will use headGitRepo in the following code
	if baseRepo.ID != headRepo.ID {
		exist := headGitRepo.IsReferenceExist(compareInfo.BaseCommitID)
		if !exist {
			if err := gitrepo.FetchRemoteCommit(ctx, headRepo, baseRepo, compareInfo.BaseCommitID); err != nil {
				return compareInfo, fmt.Errorf("FetchRemoteCommit: %w", err)
			}
		}
	}

	if !directComparison {
		compareInfo.CompareBase, err = gitrepo.MergeBase(ctx, headRepo, compareInfo.BaseCommitID, compareInfo.HeadCommitID)
		if err != nil && !errors.Is(err, util.ErrNotExist) {
			return compareInfo, fmt.Errorf("MergeBase: %w", err)
		}
	} else {
		compareInfo.CompareBase = compareInfo.BaseCommitID
	}

	if compareInfo.CompareBase == "" {
		return compareInfo, nil
	}

	// We have a common base - therefore we know that ... should work
	if !fileOnly {
		// In git log/rev-list, the "..." syntax represents the symmetric difference between two references,
		// which is different from the meaning of "..." in git diff (where it implies diffing from the merge base).
		// For listing PR commits, we must use merge-base..head to include only the commits introduced by the head branch.
		// Otherwise, commits newly pushed to the base branch would also be included, which is incorrect.
		compareInfo.Commits, err = headGitRepo.ShowPrettyFormatLogToList(ctx, compareInfo.CompareBase+".."+compareInfo.HeadCommitID)
		if err != nil {
			return compareInfo, fmt.Errorf("ShowPrettyFormatLogToList: %w", err)
		}
	}
	return compareInfo, nil
}
