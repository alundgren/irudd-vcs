// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package indexer

import (
	issue_indexer "gitea.dev/modules/indexer/issues"
	notify_service "gitea.dev/services/notify"
)

// Init initialize the issue indexer, code search and language stats are not supported
func Init() error {
	notify_service.RegisterNotifier(NewNotifier())

	issue_indexer.InitIssueIndexer(false)
	return nil
}
