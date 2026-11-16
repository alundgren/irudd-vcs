// Copyright 2020 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package private

// Email structure holds a data for sending general emails
type Email struct {
	Subject string
	Message string
	To      []string
}
