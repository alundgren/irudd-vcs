// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package release

import (
	"context"
	"errors"

	"gitea.dev/modules/graceful"
	"gitea.dev/modules/log"
	"gitea.dev/modules/queue"
	repo_module "gitea.dev/modules/repository"
)

type TagSyncOptions struct {
	RepoID int64
}

// tagSyncQueue represents a queue to handle tag sync jobs.
var tagSyncQueue *queue.WorkerPoolQueue[*TagSyncOptions]

func handlerTagSync(items ...*TagSyncOptions) []*TagSyncOptions {
	for _, opts := range items {
		err := repo_module.SyncRepoTags(graceful.GetManager().ShutdownContext(), opts.RepoID)
		if err != nil {
			log.Error("syncRepoTags [%d] failed: %v", opts.RepoID, err)
		}
	}
	return nil
}

func initTagSyncQueue(ctx context.Context) error {
	tagSyncQueue = queue.CreateUniqueQueue(ctx, "tag_sync", handlerTagSync)
	if tagSyncQueue == nil {
		return errors.New("unable to create tag_sync queue")
	}
	go graceful.GetManager().RunWithCancel(tagSyncQueue)

	return nil
}
