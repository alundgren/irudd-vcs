// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package queue

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gitea.dev/modules/nosql"
	"gitea.dev/modules/setting"

	"gitea.com/lunny/levelqueue"
	"github.com/syndtr/goleveldb/leveldb"
)

// baseLevelQueuePushPoper is the common interface for levelqueue.Queue and levelqueue.UniqueQueue
type baseLevelQueuePushPoper interface {
	RPush(data []byte) error
	LPop() ([]byte, error)
	Len() int64
}

type baseLevelQueueCommonImpl struct {
	length       int
	internalFunc func() baseLevelQueuePushPoper
	mu           *sync.Mutex
	pushed       chan struct{} // wakes a waiting PopItem, which otherwise polls with a backoff of up to 2 seconds
}

func (q *baseLevelQueueCommonImpl) PushItem(ctx context.Context, data []byte) error {
	return backoffErr(ctx, backoffBegin, backoffUpper, time.After(pushBlockTime), func() (retry bool, err error) {
		if q.mu != nil {
			q.mu.Lock()
			defer q.mu.Unlock()
		}

		cnt := int(q.internalFunc().Len())
		if cnt >= q.length {
			return true, nil
		}
		retry, err = false, q.internalFunc().RPush(data)
		if err == levelqueue.ErrAlreadyInQueue {
			err = ErrAlreadyInQueue
		}
		if err == nil && q.pushed != nil {
			select {
			case q.pushed <- struct{}{}:
			default: // a wake-up is pending already
			}
		}
		return retry, err
	})
}

func (q *baseLevelQueueCommonImpl) PopItem(ctx context.Context) ([]byte, error) {
	pop := func() (retry bool, data []byte, err error) {
		if q.mu != nil {
			q.mu.Lock()
			defer q.mu.Unlock()
		}

		data, err = q.internalFunc().LPop()
		if err == levelqueue.ErrNotFound {
			return true, nil, nil
		}
		if err != nil {
			return false, nil, err
		}
		return false, data, nil
	}
	if q.pushed == nil {
		return backoffRetErr(ctx, backoffBegin, backoffUpper, infiniteTimerC, pop)
	}
	// as backoffRetErr, but a push ends the wait, so an item is not left for up to the backoff limit
	d := backoffBegin
	for {
		retry, data, err := pop()
		if err != nil || !retry {
			return data, err
		}
		timer := time.NewTimer(d)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-q.pushed:
			timer.Stop()
			d = backoffBegin
		case <-timer.C:
			d = min(d*2, backoffUpper)
		}
	}
}

func baseLevelQueueCommon(cfg *BaseConfig, mu *sync.Mutex, pushed chan struct{}, internalFunc func() baseLevelQueuePushPoper) *baseLevelQueueCommonImpl {
	return &baseLevelQueueCommonImpl{length: cfg.Length, mu: mu, pushed: pushed, internalFunc: internalFunc}
}

func levelDBConn(cfg *BaseConfig) (string, error) {
	if cfg.ConnStr == "" { // use data dir as conn str
		if !filepath.IsAbs(cfg.DataFullDir) {
			return "", fmt.Errorf("invalid leveldb data dir (not absolute): %q", cfg.DataFullDir)
		}
		return cfg.DataFullDir, nil
	}
	if !strings.HasPrefix(cfg.ConnStr, "leveldb://") {
		return "", fmt.Errorf("invalid leveldb connection string: %q", cfg.ConnStr)
	}
	return cfg.ConnStr, nil
}

func prepareLevelDB(cfg *BaseConfig) (conn string, db *leveldb.DB, err error) {
	if conn, err = levelDBConn(cfg); err != nil {
		return "", nil, err
	}
	for range 10 {
		if db, err = nosql.GetManager().GetLevelDB(conn); err == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}
	return conn, db, err
}

// OpenLevelDBEarly starts opening the level database of the common queue settings, which takes tens of
// milliseconds, so that it overlaps with other startup work. Call release once the queues are created
// and hold their own references.
func OpenLevelDBEarly() (release func()) {
	cfg, err := setting.GetQueueSettings(setting.CfgProvider, "")
	if err != nil || cfg.Type != "level" {
		return func() {}
	}
	conn, err := levelDBConn(toBaseConfig("", cfg))
	if err != nil {
		return func() {} // the queues report it
	}
	opened := make(chan bool, 1)
	go func() {
		_, err := nosql.GetManager().GetLevelDB(conn) // a failure is retried and reported by the queues
		opened <- err == nil
	}()
	return func() {
		if <-opened {
			_ = nosql.GetManager().CloseLevelDB(conn)
		}
	}
}
