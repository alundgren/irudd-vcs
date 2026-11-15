// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package files

import (
	"fmt"
)

// ErrCommitIDDoesNotMatch represents a "CommitIDDoesNotMatch" kind of error.
type ErrCommitIDDoesNotMatch struct {
	GivenCommitID   string
	CurrentCommitID string
}

// IsErrCommitIDDoesNotMatch checks if an error is a ErrCommitIDDoesNotMatch.
func IsErrCommitIDDoesNotMatch(err error) bool {
	_, ok := err.(ErrCommitIDDoesNotMatch)
	return ok
}

func (err ErrCommitIDDoesNotMatch) Error() string {
	return fmt.Sprintf("file CommitID does not match [given: %s, expected: %s]", err.GivenCommitID, err.CurrentCommitID)
}
