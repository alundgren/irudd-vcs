// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package issues

import (
	"context"
	"fmt"

	"gitea.dev/models/db"
	"gitea.dev/modules/log"
	"gitea.dev/modules/timeutil"

	"xorm.io/builder"
)

// ContentHistory save issue/comment content history revisions.
type ContentHistory struct {
	ID             int64 `xorm:"pk autoincr"`
	PosterID       int64
	IssueID        int64              `xorm:"INDEX"`
	CommentID      int64              `xorm:"INDEX"`
	EditedUnix     timeutil.TimeStamp `xorm:"INDEX"`
	ContentText    string             `xorm:"LONGTEXT"`
	IsFirstCreated bool
	IsDeleted      bool
}

// TableName provides the real table name
func (m *ContentHistory) TableName() string {
	return "issue_content_history"
}

func init() {
	db.RegisterModel(new(ContentHistory))
}

// SaveIssueContentHistory save history
func SaveIssueContentHistory(ctx context.Context, posterID, issueID, commentID int64, editTime timeutil.TimeStamp, contentText string, isFirstCreated bool) error {
	ch := &ContentHistory{
		PosterID:       posterID,
		IssueID:        issueID,
		CommentID:      commentID,
		ContentText:    contentText,
		EditedUnix:     editTime,
		IsFirstCreated: isFirstCreated,
	}
	if err := db.Insert(ctx, ch); err != nil {
		log.Error("can not save issue content history. err=%v", err)
		return err
	}
	// We only keep at most 20 history revisions now. It is enough in most cases.
	// If there is a special requirement to keep more, we can consider introducing a new setting option then, but not now.
	KeepLimitedContentHistory(ctx, issueID, commentID, 20)
	return nil
}

// KeepLimitedContentHistory keeps at most `limit` history revisions, it will hard delete out-dated revisions, sorting by revision interval
// we can ignore all errors in this function, so we just log them
func KeepLimitedContentHistory(ctx context.Context, issueID, commentID int64, limit int) {
	type IDEditTime struct {
		ID         int64
		EditedUnix timeutil.TimeStamp
	}

	var res []*IDEditTime
	err := db.GetEngine(ctx).Select("id, edited_unix").Table("issue_content_history").
		Where(builder.Eq{"issue_id": issueID, "comment_id": commentID}).
		OrderBy("edited_unix ASC").
		Find(&res)
	if err != nil {
		log.Error("can not query content history for deletion, err=%v", err)
		return
	}
	if len(res) <= 2 {
		return
	}

	outDatedCount := len(res) - limit
	for outDatedCount > 0 {
		var indexToDelete int
		minEditedInterval := -1
		// find a history revision with minimal edited interval to delete, the first and the last should never be deleted
		for i := 1; i < len(res)-1; i++ {
			editedInterval := int(res[i].EditedUnix - res[i-1].EditedUnix)
			if minEditedInterval == -1 || editedInterval < minEditedInterval {
				minEditedInterval = editedInterval
				indexToDelete = i
			}
		}
		if indexToDelete == 0 {
			break
		}

		// hard delete the found one
		_, err = db.GetEngine(ctx).Delete(&ContentHistory{ID: res[indexToDelete].ID})
		if err != nil {
			log.Error("can not delete out-dated content history, err=%v", err)
			break
		}
		res = append(res[:indexToDelete], res[indexToDelete+1:]...)
		outDatedCount--
	}
}

// HasIssueContentHistory check if a ContentHistory entry exists
func HasIssueContentHistory(dbCtx context.Context, issueID, commentID int64) (bool, error) {
	exists, err := db.GetEngine(dbCtx).Where(builder.Eq{"issue_id": issueID, "comment_id": commentID}).Exist(&ContentHistory{})
	if err != nil {
		return false, fmt.Errorf("can not check issue content history. err: %w", err)
	}
	return exists, err
}
