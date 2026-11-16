// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"

	"gitea.dev/modules/graceful"
	repo_service "gitea.dev/services/repository"

	"github.com/urfave/cli/v3"
)

func newRegenerateHooksCommand() *cli.Command {
	return &cli.Command{
		Name:   "hooks",
		Usage:  "Regenerate git-hooks",
		Action: runRegenerateHooks,
	}
}

func runRegenerateHooks(ctx context.Context, _ *cli.Command) error {
	if err := initDB(ctx); err != nil {
		return err
	}
	return repo_service.SyncRepositoryHooks(graceful.GetManager().ShutdownContext())
}
