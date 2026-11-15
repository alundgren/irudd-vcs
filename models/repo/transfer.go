// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"context"
	"fmt"

	"gitea.dev/models/db"
	"gitea.dev/models/organization"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/log"
	"gitea.dev/modules/timeutil"
	"gitea.dev/modules/util"

	"xorm.io/builder"
)

// ErrNoPendingRepoTransfer is an error type for repositories without a pending
// transfer request
type ErrNoPendingRepoTransfer struct {
	RepoID int64
}

func (err ErrNoPendingRepoTransfer) Error() string {
	return fmt.Sprintf("repository doesn't have a pending transfer [repo_id: %d]", err.RepoID)
}

// IsErrNoPendingTransfer is an error type when a repository has no pending
// transfers
func IsErrNoPendingTransfer(err error) bool {
	_, ok := err.(ErrNoPendingRepoTransfer)
	return ok
}

func (err ErrNoPendingRepoTransfer) Unwrap() error {
	return util.ErrNotExist
}

// RepoTransfer is used to manage repository transfers
type RepoTransfer struct { //nolint:revive // export stutter
	ID          int64 `xorm:"pk autoincr"`
	DoerID      int64
	Doer        *user_model.User `xorm:"-"`
	RecipientID int64
	Recipient   *user_model.User `xorm:"-"`
	RepoID      int64
	Repo        *Repository `xorm:"-"`
	TeamIDs     []int64
	Teams       []*organization.Team `xorm:"-"`

	CreatedUnix timeutil.TimeStamp `xorm:"INDEX NOT NULL created"`
	UpdatedUnix timeutil.TimeStamp `xorm:"INDEX NOT NULL updated"`
}

func init() {
	db.RegisterModel(new(RepoTransfer))
}

func (r *RepoTransfer) LoadRecipient(ctx context.Context) error {
	if r.Recipient == nil {
		u, err := user_model.GetUserByID(ctx, r.RecipientID)
		if err != nil {
			return err
		}
		r.Recipient = u
	}

	return nil
}

func (r *RepoTransfer) LoadRepo(ctx context.Context) error {
	if r.Repo == nil {
		repo, err := GetRepositoryByID(ctx, r.RepoID)
		if err != nil {
			return err
		}
		r.Repo = repo
	}

	return nil
}

// LoadAttributes fetches the transfer recipient from the database
func (r *RepoTransfer) LoadAttributes(ctx context.Context) error {
	if err := r.LoadRecipient(ctx); err != nil {
		return err
	}

	if r.Recipient.IsOrganization() && r.Teams == nil {
		teamsMap, err := organization.GetTeamsByIDs(ctx, r.TeamIDs)
		if err != nil {
			return err
		}
		for _, team := range teamsMap {
			r.Teams = append(r.Teams, team)
		}
	}

	if err := r.LoadRepo(ctx); err != nil {
		return err
	}

	if r.Doer == nil {
		u, err := user_model.GetUserByID(ctx, r.DoerID)
		if err != nil {
			return err
		}
		r.Doer = u
	}

	return nil
}

// CanUserAcceptOrRejectTransfer checks if the user has the rights to accept/decline a repo transfer.
// For user, it checks if it's himself
// For organizations, it checks if the user is able to create repos
func (r *RepoTransfer) CanUserAcceptOrRejectTransfer(ctx context.Context, u *user_model.User) bool {
	if err := r.LoadAttributes(ctx); err != nil {
		log.Error("LoadAttributes: %v", err)
		return false
	}

	if !r.Recipient.IsOrganization() {
		return r.RecipientID == u.ID
	}

	allowed, err := organization.CanCreateOrgRepo(ctx, r.RecipientID, u.ID)
	if err != nil {
		log.Error("CanCreateOrgRepo: %v", err)
		return false
	}

	return allowed
}

type PendingRepositoryTransferOptions struct {
	RepoID      int64
	SenderID    int64
	RecipientID int64
}

func (opts *PendingRepositoryTransferOptions) ToConds() builder.Cond {
	cond := builder.NewCond()
	if opts.RepoID != 0 {
		cond = cond.And(builder.Eq{"repo_id": opts.RepoID})
	}
	if opts.SenderID != 0 {
		cond = cond.And(builder.Eq{"doer_id": opts.SenderID})
	}
	if opts.RecipientID != 0 {
		cond = cond.And(builder.Eq{"recipient_id": opts.RecipientID})
	}
	return cond
}

func GetPendingRepositoryTransfers(ctx context.Context, opts *PendingRepositoryTransferOptions) ([]*RepoTransfer, error) {
	transfers := make([]*RepoTransfer, 0, 10)
	return transfers, db.GetEngine(ctx).
		Where(opts.ToConds()).
		Find(&transfers)
}

// GetPendingRepositoryTransfer fetches the most recent and ongoing transfer
// process for the repository
func GetPendingRepositoryTransfer(ctx context.Context, repo *Repository) (*RepoTransfer, error) {
	transfers, err := GetPendingRepositoryTransfers(ctx, &PendingRepositoryTransferOptions{RepoID: repo.ID})
	if err != nil {
		return nil, err
	}

	if len(transfers) != 1 {
		return nil, ErrNoPendingRepoTransfer{RepoID: repo.ID}
	}

	return transfers[0], nil
}
