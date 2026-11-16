// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package issues

import (
	"context"
	"fmt"

	"gitea.dev/modules/log"
)

// LoadRefComment loads comment that created this reference from database
func (c *Comment) LoadRefComment(ctx context.Context) (err error) {
	if c.RefComment != nil {
		return nil
	}
	c.RefComment, err = GetCommentByID(ctx, c.RefCommentID)
	return err
}

// LoadRefIssue loads comment that created this reference from database
func (c *Comment) LoadRefIssue(ctx context.Context) (err error) {
	if c.RefIssue != nil {
		return nil
	}
	c.RefIssue, err = GetIssueByID(ctx, c.RefIssueID)
	if err == nil {
		err = c.RefIssue.LoadRepo(ctx)
	}
	return err
}

// CommentTypeIsRef returns true if CommentType is a reference from another issue
func CommentTypeIsRef(t CommentType) bool {
	return t == CommentTypeCommentRef || t == CommentTypePullRef || t == CommentTypeIssueRef
}

// RefCommentLink returns the relative URL for the comment that created this reference
func (c *Comment) RefCommentLink(ctx context.Context) string {
	// Edge case for when the reference is inside the title or the description of the referring issue
	if c.RefCommentID == 0 {
		return c.RefIssueLink(ctx)
	}
	if err := c.LoadRefComment(ctx); err != nil { // Silently dropping errors :unamused:
		log.Error("LoadRefComment(%d): %v", c.RefCommentID, err)
		return ""
	}
	return c.RefComment.Link(ctx)
}

// RefIssueLink returns the relative URL of the issue where this reference was created
func (c *Comment) RefIssueLink(ctx context.Context) string {
	if err := c.LoadRefIssue(ctx); err != nil { // Silently dropping errors :unamused:
		log.Error("LoadRefIssue(%d): %v", c.RefCommentID, err)
		return ""
	}
	return c.RefIssue.Link()
}

// RefIssueTitle returns the title of the issue where this reference was created
func (c *Comment) RefIssueTitle(ctx context.Context) string {
	if err := c.LoadRefIssue(ctx); err != nil { // Silently dropping errors :unamused:
		log.Error("LoadRefIssue(%d): %v", c.RefCommentID, err)
		return ""
	}
	return c.RefIssue.Title
}

// RefIssueIdent returns the user friendly identity (e.g. "#1234") of the issue where this reference was created
func (c *Comment) RefIssueIdent(ctx context.Context) string {
	if err := c.LoadRefIssue(ctx); err != nil { // Silently dropping errors :unamused:
		log.Error("LoadRefIssue(%d): %v", c.RefCommentID, err)
		return ""
	}
	// FIXME: check this name for cross-repository references (#7901 if it gets merged)
	return fmt.Sprintf("#%d", c.RefIssue.Index)
}

// __________      .__  .__ __________                                     __
// \______   \__ __|  | |  |\______   \ ____  ________ __   ____   _______/  |_
//  |     ___/  |  \  | |  | |       _// __ \/ ____/  |  \_/ __ \ /  ___/\   __\
//  |    |   |  |  /  |_|  |_|    |   \  ___< <_|  |  |  /\  ___/ \___ \  |  |
//  |____|   |____/|____/____/____|_  /\___  >__   |____/  \___  >____  > |__|
//                                  \/     \/   |__|           \/     \/
