// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

//go:build !gogit

package git

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// batchPoolMaxIdle bounds the idle cat-file processes, one per repository
const batchPoolMaxIdle = 32

// batchPoolSentinel is an object name that never resolves, so cat-file answers "<name> missing"
const batchPoolSentinel = "0000000000000000000000000000000000000000"

// batchPool keeps the cat-file --batch-command process of a closed Repository for a while: starting
// one costs about 10 ms on Windows, and API requests often open the same repository one after another.
type batchPool struct {
	mu      sync.Mutex
	ctx     context.Context // nil while disabled
	idle    time.Duration
	root    string                  // only repositories under it are kept, not temporary ones that are removed right after
	entries map[string]*pooledBatch // by repository path
	gen     uint64                  // bumped by ReleaseBatches, so batches in use then are not kept
}

type pooledBatch struct {
	batch *catFileBatchCommand
	timer *time.Timer
}

// pooledBatches maps the batches that newBatch created to the generation they were created in
var pooledBatches sync.Map

var catFileBatchPool = &batchPool{entries: map[string]*pooledBatch{}}

// EnableBatchPool lets closed repositories under root keep their cat-file process for idle; processes end
// with ctx. Unit tests leave it disabled, since they remove repositories without ReleaseBatches.
func EnableBatchPool(ctx context.Context, idle time.Duration, root string) {
	p := catFileBatchPool
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ctx, p.idle, p.root = ctx, idle, filepath.Clean(root)+string(filepath.Separator)
	context.AfterFunc(ctx, func() { ReleaseBatches("") })
}

// ReleaseBatches ends the idle cat-file processes of the repositories under dir, all of them for "".
// Windows cannot delete or rename files that a process holds open.
func ReleaseBatches(dir string) {
	p := catFileBatchPool
	p.mu.Lock()
	p.gen++ // a batch in use now may have the old files open, and a new repository may take the path
	var closing []*catFileBatchCommand
	prefix := filepath.Clean(dir) + string(filepath.Separator)
	for path, e := range p.entries {
		if dir == "" || path == filepath.Clean(dir) || strings.HasPrefix(path, prefix) {
			e.timer.Stop()
			delete(p.entries, path)
			closing = append(closing, e.batch)
		}
	}
	p.mu.Unlock()
	for _, b := range closing {
		p.closeBatch(b)
	}
}

// newBatch returns a batch that may outlive ctx through the pool, or nil while the pool is disabled
func (p *batchPool) newBatch(repoPath string) *catFileBatchCommand {
	p.mu.Lock()
	poolCtx, gen := p.ctx, p.gen
	if !strings.HasPrefix(filepath.Clean(repoPath), p.root) {
		p.mu.Unlock()
		return nil
	}
	if e, ok := p.entries[repoPath]; ok {
		e.timer.Stop()
		delete(p.entries, repoPath)
		p.mu.Unlock()
		if e.batch.batch.closeFunc.Load() != nil {
			return e.batch
		}
		pooledBatches.Delete(e.batch) // the process ended while idle
	} else {
		p.mu.Unlock()
	}
	if poolCtx == nil || poolCtx.Err() != nil || !DefaultFeatures().SupportCatFileBatchCommand {
		return nil
	}
	b, err := newCatFileBatchCommand(poolCtx, repoPath)
	if err != nil {
		return nil
	}
	b.getBatch() // started now, so a kill from another goroutine finds the process
	pooledBatches.Store(b, gen)
	return b
}

func (p *batchPool) closeBatch(b *catFileBatchCommand) {
	pooledBatches.Delete(b)
	b.Close()
}

// put keeps b for the next Repository of repoPath, or closes it
func (p *batchPool) put(repoPath string, b *catFileBatchCommand) {
	if !b.inSync() {
		p.closeBatch(b)
		return
	}
	gen, _ := pooledBatches.Load(b)
	p.mu.Lock()
	if p.ctx == nil || p.ctx.Err() != nil || gen != p.gen || len(p.entries) >= batchPoolMaxIdle {
		p.mu.Unlock()
		p.closeBatch(b)
		return
	}
	old := p.entries[repoPath]
	e := &pooledBatch{batch: b}
	e.timer = time.AfterFunc(p.idle, func() {
		p.mu.Lock()
		expired := p.entries[repoPath] == e // not taken or replaced meanwhile
		if expired {
			delete(p.entries, repoPath)
		}
		p.mu.Unlock()
		if expired {
			p.closeBatch(b)
		}
	})
	p.entries[repoPath] = e
	p.mu.Unlock()
	if old != nil {
		old.timer.Stop()
		p.closeBatch(old.batch)
	}
}

// inSync checks that nothing is left unread, so the next user starts at a response boundary
func (b *catFileBatchCommand) inSync() bool {
	if b.batch == nil || b.batch.closeFunc.Load() == nil {
		return false
	}
	rd := b.batch.respReader
	if rd.Buffered() > 1 {
		return false
	}
	if _, err := b.batch.reqWriter.Write([]byte("info " + batchPoolSentinel + "\n")); err != nil {
		return false
	}
	line, err := rd.ReadString('\n')
	if err == nil && line == "\n" { // the LF that ends the content of the last object
		line, err = rd.ReadString('\n')
	}
	return err == nil && line == batchPoolSentinel+" missing\n" && rd.Buffered() == 0
}
