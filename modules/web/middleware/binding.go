// Copyright 2014 The Gogs Authors. All rights reserved.
// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package middleware

import (
	"gitea.dev/modules/util"

	"gitea.com/go-chi/binding"
)

func init() {
	binding.SetNameMapper(util.ToSnakeCase)
}
