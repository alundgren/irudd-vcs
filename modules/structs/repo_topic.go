// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package structs

// TopicName a list of repo topic names
type TopicName struct {
	// List of topic names
	TopicNames []string `json:"topics"`
}
