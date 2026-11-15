// Copyright 2020 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package git

import (
	"context"
	"fmt"

	"gitea.dev/models/db"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/modules/lfs"
	"gitea.dev/modules/log"
	"gitea.dev/modules/setting"
	"gitea.dev/modules/timeutil"
	"gitea.dev/modules/util"

	"xorm.io/builder"
)

// ErrLFSLockNotExist represents a "LFSLockNotExist" kind of error.
type ErrLFSLockNotExist struct {
	ID     int64
	RepoID int64
	Path   string
}

// IsErrLFSLockNotExist checks if an error is a ErrLFSLockNotExist.
func IsErrLFSLockNotExist(err error) bool {
	_, ok := err.(ErrLFSLockNotExist)
	return ok
}

func (err ErrLFSLockNotExist) Error() string {
	return fmt.Sprintf("lfs lock does not exist [id: %d, rid: %d, path: %s]", err.ID, err.RepoID, err.Path)
}

func (err ErrLFSLockNotExist) Unwrap() error {
	return util.ErrNotExist
}

// ErrLFSLockAlreadyExist represents a "LFSLockAlreadyExist" kind of error.
type ErrLFSLockAlreadyExist struct {
	RepoID int64
	Path   string
}

// IsErrLFSLockAlreadyExist checks if an error is a ErrLFSLockAlreadyExist.
func IsErrLFSLockAlreadyExist(err error) bool {
	_, ok := err.(ErrLFSLockAlreadyExist)
	return ok
}

func (err ErrLFSLockAlreadyExist) Error() string {
	return fmt.Sprintf("lfs lock already exists [rid: %d, path: %s]", err.RepoID, err.Path)
}

func (err ErrLFSLockAlreadyExist) Unwrap() error {
	return util.ErrAlreadyExist
}

// LFSMetaObject stores metadata for LFS tracked files.
type LFSMetaObject struct {
	ID           int64 `xorm:"pk autoincr"`
	lfs.Pointer  `xorm:"extends"`
	RepositoryID int64              `xorm:"UNIQUE(s) INDEX NOT NULL"`
	CreatedUnix  timeutil.TimeStamp `xorm:"created"`
	UpdatedUnix  timeutil.TimeStamp `xorm:"INDEX updated"`
}

func init() {
	db.RegisterModel(new(LFSMetaObject))
}

// ErrLFSObjectNotExist is returned from lfs models functions in order
// to differentiate between database and missing object errors.
var ErrLFSObjectNotExist = db.ErrNotExist{Resource: "LFS Meta object"}

// NewLFSMetaObject stores a given populated LFSMetaObject structure in the database
// if it is not already present.
func NewLFSMetaObject(ctx context.Context, repoID int64, p lfs.Pointer) (*LFSMetaObject, error) {
	m, exist, err := db.Get[LFSMetaObject](ctx, builder.Eq{"repository_id": repoID, "oid": p.Oid})
	if err != nil {
		return nil, err
	} else if exist {
		return m, nil
	}

	m = &LFSMetaObject{Pointer: p, RepositoryID: repoID}
	if err = db.Insert(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

// GetLFSMetaObjectByOid selects a LFSMetaObject entry from database by its OID.
// It may return ErrLFSObjectNotExist or a database error. If the error is nil,
// the returned pointer is a valid LFSMetaObject.
func GetLFSMetaObjectByOid(ctx context.Context, repoID int64, oid string) (*LFSMetaObject, error) {
	if len(oid) == 0 {
		return nil, ErrLFSObjectNotExist
	}

	m, has, err := db.Get[LFSMetaObject](ctx, builder.Eq{"repository_id": repoID, "oid": oid})
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrLFSObjectNotExist
	}
	return m, nil
}

// RemoveLFSMetaObjectByOid removes a LFSMetaObject entry from database by its OID.
// It may return ErrLFSObjectNotExist or a database error.
func RemoveLFSMetaObjectByOid(ctx context.Context, repoID int64, oid string) (int64, error) {
	return RemoveLFSMetaObjectByOidFn(ctx, repoID, oid, nil)
}

// RemoveLFSMetaObjectByOidFn removes a LFSMetaObject entry from database by its OID.
// It may return ErrLFSObjectNotExist or a database error. It will run Fn with the current count within the transaction
func RemoveLFSMetaObjectByOidFn(ctx context.Context, repoID int64, oid string, fn func(count int64) error) (int64, error) {
	if len(oid) == 0 {
		return 0, ErrLFSObjectNotExist
	}

	return db.WithTx2(ctx, func(ctx context.Context) (int64, error) {
		m := &LFSMetaObject{Pointer: lfs.Pointer{Oid: oid}, RepositoryID: repoID}
		if _, err := db.DeleteByBean(ctx, m); err != nil {
			return -1, err
		}

		count, err := db.CountByBean(ctx, &LFSMetaObject{Pointer: lfs.Pointer{Oid: oid}})
		if err != nil {
			return count, err
		}

		if fn != nil {
			if err := fn(count); err != nil {
				return count, err
			}
		}

		return count, nil
	})
}

// ExistsLFSObject checks if a provided Oid exists within the DB
func ExistsLFSObject(ctx context.Context, oid string) (bool, error) {
	return db.GetEngine(ctx).Exist(&LFSMetaObject{Pointer: lfs.Pointer{Oid: oid}})
}

// CopyLFS copies LFS data from one repo to another
func CopyLFS(ctx context.Context, newRepo, oldRepo *repo_model.Repository) error {
	var lfsObjects []*LFSMetaObject
	if err := db.GetEngine(ctx).Where("repository_id=?", oldRepo.ID).Find(&lfsObjects); err != nil {
		return err
	}

	for _, v := range lfsObjects {
		v.ID = 0
		v.RepositoryID = newRepo.ID
		if err := db.Insert(ctx, v); err != nil {
			return err
		}
	}

	return nil
}

// GetRepoLFSSize return a repository's lfs files size
func GetRepoLFSSize(ctx context.Context, repoID int64) (int64, error) {
	lfsSize, err := db.GetEngine(ctx).Where("repository_id = ?", repoID).SumInt(new(LFSMetaObject), "size")
	if err != nil {
		return 0, fmt.Errorf("updateSize: GetLFSMetaObjects: %w", err)
	}
	return lfsSize, nil
}

// IterateRepositoryIDsWithLFSMetaObjects iterates across the repositories that have LFSMetaObjects
func IterateRepositoryIDsWithLFSMetaObjects(ctx context.Context, f func(ctx context.Context, repoID, count int64) error) error {
	batchSize := setting.Database.IterateBufferSize
	sess := db.GetEngine(ctx)
	id := int64(0)
	type RepositoryCount struct {
		RepositoryID int64
		Count        int64
	}
	for {
		counts := make([]*RepositoryCount, 0, batchSize)
		sess.Select("repository_id, COUNT(id) AS count").
			Table("lfs_meta_object").
			Where("repository_id > ?", id).
			GroupBy("repository_id").
			OrderBy("repository_id ASC")

		if err := sess.Limit(batchSize, 0).Find(&counts); err != nil {
			return err
		}
		if len(counts) == 0 {
			return nil
		}

		for _, count := range counts {
			if err := f(ctx, count.RepositoryID, count.Count); err != nil {
				return err
			}
		}
		id = counts[len(counts)-1].RepositoryID
	}
}

// IterateLFSMetaObjectsForRepoOptions provides options for IterateLFSMetaObjectsForRepo
type IterateLFSMetaObjectsForRepoOptions struct {
	OlderThan               timeutil.TimeStamp
	UpdatedLessRecentlyThan timeutil.TimeStamp
}

// IterateLFSMetaObjectsForRepo provides a iterator for LFSMetaObjects per Repo
func IterateLFSMetaObjectsForRepo(ctx context.Context, repoID int64, f func(context.Context, *LFSMetaObject, int64) error, opts *IterateLFSMetaObjectsForRepoOptions) error {
	batchSize := setting.Database.IterateBufferSize
	engine := db.GetEngine(ctx)
	type CountLFSMetaObject struct {
		Count         int64
		LFSMetaObject `xorm:"extends"`
	}

	lastID := int64(0)

	for {
		beans := make([]*CountLFSMetaObject, 0, batchSize)
		sess := engine.Table("lfs_meta_object").Select("`lfs_meta_object`.*, COUNT(`l1`.oid) AS `count`").
			Join("INNER", "`lfs_meta_object` AS l1", "`lfs_meta_object`.oid = `l1`.oid").
			Where("`lfs_meta_object`.repository_id = ?", repoID)
		if !opts.OlderThan.IsZero() {
			sess.And("`lfs_meta_object`.created_unix < ?", opts.OlderThan)
		}
		if !opts.UpdatedLessRecentlyThan.IsZero() {
			sess.And("`lfs_meta_object`.updated_unix < ?", opts.UpdatedLessRecentlyThan)
		}
		sess.GroupBy("`lfs_meta_object`.id").
			And("`lfs_meta_object`.id > ?", lastID).
			OrderBy("`lfs_meta_object`.id ASC")

		if err := sess.Limit(batchSize).Find(&beans); err != nil {
			return err
		}
		if len(beans) == 0 {
			return nil
		}

		for _, bean := range beans {
			if err := f(ctx, &bean.LFSMetaObject, bean.Count); err != nil {
				return err
			}
		}
		lastID = beans[len(beans)-1].ID
	}
}

// MarkLFSMetaObject updates the updated time for the provided LFSMetaObject
func MarkLFSMetaObject(ctx context.Context, id int64) error {
	obj := &LFSMetaObject{
		UpdatedUnix: timeutil.TimeStampNow(),
	}
	count, err := db.GetEngine(ctx).ID(id).Update(obj)
	if count != 1 {
		log.Error("Unexpectedly updated %d LFSMetaObjects with ID: %d", count, id)
	}
	return err
}
