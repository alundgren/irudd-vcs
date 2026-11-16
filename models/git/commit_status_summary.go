// Copyright 2024 Gitea. All rights reserved.
// SPDX-License-Identifier: MIT

package git

import (
	"context"

	"gitea.dev/models/db"
	"gitea.dev/modules/commitstatus"
	"gitea.dev/modules/setting"
)

// CommitStatusSummary holds the latest commit Status of a single Commit
type CommitStatusSummary struct {
	ID        int64                          `xorm:"pk autoincr"`
	RepoID    int64                          `xorm:"INDEX UNIQUE(repo_id_sha)"`
	SHA       string                         `xorm:"VARCHAR(64) NOT NULL INDEX UNIQUE(repo_id_sha)"`
	State     commitstatus.CommitStatusState `xorm:"VARCHAR(7) NOT NULL"`
	TargetURL string                         `xorm:"TEXT"`
}

func init() {
	db.RegisterModel(new(CommitStatusSummary))
}

func UpdateCommitStatusSummary(ctx context.Context, repoID int64, sha string) error {
	commitStatuses, err := GetLatestCommitStatus(ctx, repoID, sha, db.ListOptionsAll)
	if err != nil {
		return err
	}
	// it guarantees that commitStatuses is not empty because this function is always called after a commit status is created
	if len(commitStatuses) == 0 {
		setting.PanicInDevOrTesting("no commit statuses found for repo %d and sha %s", repoID, sha)
	}
	state := CalcCommitStatus(commitStatuses) // non-empty commitStatuses is guaranteed
	if cnt, err := db.GetEngine(ctx).Where("repo_id=? AND sha=?", repoID, sha).
		Cols("state, target_url").
		Update(&CommitStatusSummary{
			State:     state.State,
			TargetURL: state.TargetURL,
		}); err != nil {
		return err
	} else if cnt == 0 {
		_, err = db.GetEngine(ctx).Insert(&CommitStatusSummary{
			RepoID:    repoID,
			SHA:       sha,
			State:     state.State,
			TargetURL: state.TargetURL,
		})
		return err
	}
	return nil
}
