// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gitea.dev/models"
	git_model "gitea.dev/models/git"
	system_model "gitea.dev/models/system"
	webhook_model "gitea.dev/models/webhook"
	"gitea.dev/modules/git"
	"gitea.dev/modules/git/gitcmd"
	"gitea.dev/modules/storage"
	repo_service "gitea.dev/services/repository"

	"github.com/urfave/cli/v3"
)

// The maintenance commands replace the removed cron tasks; run them from an external scheduler if needed.
func newMaintenanceCommand() *cli.Command {
	timeoutFlag := &cli.DurationFlag{
		Name:  "timeout",
		Value: time.Minute,
		Usage: "Timeout for the git command on each repository",
	}
	gitArgsFlag := &cli.StringSliceFlag{
		Name:  "git-arg",
		Usage: "Extra argument passed to the git command, can be repeated",
	}
	return &cli.Command{
		Name:  "maintenance",
		Usage: "Run maintenance tasks",
		Commands: []*cli.Command{
			{
				Name:   "fsck",
				Usage:  "Check the health of all repositories with git fsck",
				Flags:  []cli.Flag{timeoutFlag, gitArgsFlag},
				Action: runMaintenanceFsck,
			},
			{
				Name:   "gc",
				Usage:  "Run git gc on all repositories and update their sizes",
				Flags:  []cli.Flag{timeoutFlag, gitArgsFlag},
				Action: runMaintenanceGC,
			},
			{
				Name:   "repo-stats",
				Usage:  "Recalculate repository, user and issue counters",
				Action: runMaintenanceRepoStats,
			},
			{
				Name:  "deleted-branches",
				Usage: "Remove records of branches deleted before the given age",
				Flags: []cli.Flag{
					&cli.DurationFlag{Name: "older-than", Value: 24 * time.Hour, Usage: "Minimum age of the deleted branch records"},
				},
				Action: runMaintenanceDeletedBranches,
			},
			{
				Name:  "hook-tasks",
				Usage: "Remove delivered webhook tasks, by age or by keeping the newest per webhook",
				Flags: []cli.Flag{
					&cli.DurationFlag{Name: "older-than", Value: 7 * 24 * time.Hour, Usage: "Minimum age of the delivered tasks to remove"},
					&cli.IntFlag{Name: "keep-per-webhook", Usage: "Keep this many newest delivered tasks per webhook instead of removing by age"},
				},
				Action: runMaintenanceHookTasks,
			},
			{
				Name:  "missing-repos",
				Usage: "Fix repository records whose git directory is missing",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "reinit", Usage: "Reinitialize an empty git repository for each record"},
					&cli.BoolFlag{Name: "delete", Usage: "Delete each record and its data"},
				},
				Action: runMaintenanceMissingRepos,
			},
			{
				Name:  "notices",
				Usage: "Remove system notices older than the given age",
				Flags: []cli.Flag{
					&cli.DurationFlag{Name: "older-than", Value: 365 * 24 * time.Hour, Usage: "Minimum age of the notices to remove"},
				},
				Action: runMaintenanceNotices,
			},
		},
	}
}

func initMaintenanceGit(ctx context.Context) error {
	if err := initDB(ctx); err != nil {
		return err
	}
	return git.InitFull()
}

func runMaintenanceFsck(ctx context.Context, c *cli.Command) error {
	if err := initMaintenanceGit(ctx); err != nil {
		return err
	}
	// the args come from the admin running the command, so they are trusted
	return repo_service.GitFsckRepos(ctx, c.Duration("timeout"), gitcmd.ToTrustedCmdArgs(c.StringSlice("git-arg")))
}

func runMaintenanceGC(ctx context.Context, c *cli.Command) error {
	if err := initMaintenanceGit(ctx); err != nil {
		return err
	}
	return repo_service.GitGcRepos(ctx, c.Duration("timeout"), gitcmd.ToTrustedCmdArgs(c.StringSlice("git-arg")))
}

func runMaintenanceRepoStats(ctx context.Context, _ *cli.Command) error {
	if err := initDB(ctx); err != nil {
		return err
	}
	return models.CheckRepoStats(ctx)
}

func runMaintenanceDeletedBranches(ctx context.Context, c *cli.Command) error {
	if err := initDB(ctx); err != nil {
		return err
	}
	git_model.RemoveOldDeletedBranches(ctx, c.Duration("older-than"))
	return nil
}

func runMaintenanceHookTasks(ctx context.Context, c *cli.Command) error {
	if err := initDB(ctx); err != nil {
		return err
	}
	if c.IsSet("keep-per-webhook") {
		if c.IsSet("older-than") {
			return errors.New("use either --older-than or --keep-per-webhook")
		}
		return webhook_model.CleanupHookTaskTable(ctx, webhook_model.PerWebhook, 0, c.Int("keep-per-webhook"))
	}
	return webhook_model.CleanupHookTaskTable(ctx, webhook_model.OlderThan, c.Duration("older-than"), 0)
}

func runMaintenanceMissingRepos(ctx context.Context, c *cli.Command) error {
	if c.Bool("reinit") == c.Bool("delete") {
		return errors.New("use exactly one of --reinit or --delete")
	}
	if err := initMaintenanceGit(ctx); err != nil {
		return err
	}
	if c.Bool("reinit") {
		return repo_service.ReinitMissingRepositories(ctx)
	}
	if err := storage.Init(); err != nil { // deleting a repository also removes its stored LFS objects
		return fmt.Errorf("storage.Init: %w", err)
	}
	return repo_service.DeleteMissingRepositories(ctx)
}

func runMaintenanceNotices(ctx context.Context, c *cli.Command) error {
	if err := initDB(ctx); err != nil {
		return err
	}
	return system_model.DeleteOldSystemNotices(ctx, c.Duration("older-than"))
}
