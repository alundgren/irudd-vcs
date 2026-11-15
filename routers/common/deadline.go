// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package common

import (
	"time"

	"gitea.dev/modules/setting"
	"gitea.dev/modules/timeutil"
)

func ParseAPIDeadlineToEndOfDay(t *time.Time) (timeutil.TimeStamp, error) {
	if t == nil || t.IsZero() || t.Unix() == 0 {
		return 0, nil
	}
	deadline := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, setting.DefaultUILocation)
	return timeutil.TimeStamp(deadline.Unix()), nil
}
