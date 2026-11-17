// Copyright 2020 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package nosql

import (
	"context"
	"sync"

	"gitea.dev/modules/process"

	"github.com/syndtr/goleveldb/leveldb"
)

var manager *Manager

// Manager is the nosql connection manager
type Manager struct {
	ctx      context.Context
	finished process.FinishedFunc
	mutex    sync.Mutex

	LevelDBConnections map[string]*levelDBHolder
}

type levelDBHolder struct {
	name  []string
	count int64
	db    *leveldb.DB
}

func init() {
	_ = GetManager()
}

// GetManager returns a Manager and initializes one as singleton is there's none yet
func GetManager() *Manager {
	if manager == nil {
		ctx, _, finished := process.GetManager().AddTypedContext(context.Background(), "Service: NoSQL", process.SystemProcessType, false)
		manager = &Manager{
			ctx:                ctx,
			finished:           finished,
			LevelDBConnections: make(map[string]*levelDBHolder),
		}
	}
	return manager
}
