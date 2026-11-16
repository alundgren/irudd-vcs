// Copyright 2014 The Gogs Authors. All rights reserved.
// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package auth

import (
	"errors"
)

type ErrUserAuthMessage string

func (e ErrUserAuthMessage) Error() string {
	return string(e)
}

func ErrAsUserAuthMessage(err error) (string, bool) {
	var msg ErrUserAuthMessage
	if errors.As(err, &msg) {
		return msg.Error(), true
	}
	return "", false
}
