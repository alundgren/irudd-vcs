// Copyright 2020 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package middleware

import (
	"context"

	"gitea.dev/modules/reqctx"
)

const ContextDataKeySignedUser = "SignedUser"

func GetContextData(c context.Context) reqctx.ContextData {
	if rc := reqctx.GetRequestDataStore(c); rc != nil {
		return rc.GetData()
	}
	return nil
}
