// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package issue

import (
	issues_model "gitea.dev/models/issues"
	user_model "gitea.dev/models/user"
)

type ReviewRequestNotifier struct {
	Comment  *issues_model.Comment
	IsAdd    bool
	Reviewer *user_model.User
}
