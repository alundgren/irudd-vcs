// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package routing

import (
	"context"

	"gitea.dev/modules/gtprof"
	"gitea.dev/modules/reqctx"
)

type contextKeyType struct{}

var contextKey contextKeyType

func getRequestRecord(ctx context.Context) *requestRecord {
	record, _ := ctx.Value(contextKey).(*requestRecord)
	return record
}

// RecordFuncInfo records a func info into context
func RecordFuncInfo(ctx context.Context, funcInfo *FuncInfo) (end func()) {
	end = func() {}
	if reqCtx := reqctx.FromContext(ctx); reqCtx != nil {
		var traceSpan *gtprof.TraceSpan
		traceSpan, end = gtprof.GetTracer().StartInContext(reqCtx, "http.func")
		traceSpan.SetAttributeString("func", funcInfo.shortName)
	}
	if record := getRequestRecord(ctx); record != nil {
		record.lock.Lock()
		record.funcInfo = funcInfo
		record.lock.Unlock()
	}
	return end
}

// UpdatePanicError updates a context's error info, a panic may be recovered by other middlewares, but we still need to know that.
func UpdatePanicError(ctx context.Context, err error) {
	record := getRequestRecord(ctx)
	if record == nil {
		return
	}

	record.lock.Lock()
	record.panicError = err
	record.lock.Unlock()
}
