// Copyright 2018 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

// Package cmd provides subcommands to the gitea binary - such as "web" or
// "admin".
package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"gitea.dev/models/db"
	"gitea.dev/modules/log"
	"gitea.dev/modules/setting"

	"github.com/urfave/cli/v3"
)

func initDB(ctx context.Context) error {
	setting.MustInstalled()
	setting.LoadDBSetting()
	setting.InitSQLLoggersForCli(log.INFO)

	if err := db.InitEngine(ctx); err != nil {
		return fmt.Errorf("unable to initialize the database using the configuration in %q. Error: %w", setting.CustomConf, err)
	}
	return nil
}

func installSignals() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		// install notify
		signalChannel := make(chan os.Signal, 1)

		signal.Notify(
			signalChannel,
			syscall.SIGINT,
			syscall.SIGTERM,
		)
		select {
		case <-signalChannel:
		case <-ctx.Done():
		}
		cancel()
		signal.Reset()
	}()

	return ctx, cancel
}

func setupConsoleLogger(level log.Level, colorize bool, out io.Writer) {
	if out != os.Stdout && out != os.Stderr {
		panic("setupConsoleLogger can only be used with os.Stdout or os.Stderr")
	}

	writeMode := log.WriterMode{
		Level:        level,
		Colorize:     colorize,
		WriterOption: log.WriterConsoleOption{Stderr: out == os.Stderr},
	}
	writer := log.NewEventWriterConsole("console-default", writeMode)
	log.GetManager().GetLogger(log.DEFAULT).ReplaceAllWriters(writer)
}

func globalBool(c *cli.Command, name string) bool {
	for _, ctx := range c.Lineage() {
		if ctx.Bool(name) {
			return true
		}
	}
	return false
}

// PrepareConsoleLoggerLevel by default, use INFO level for console logger, but some sub-commands (for git/ssh protocol) shouldn't output any log to stdout.
// Any log appears in git stdout pipe will break the git protocol, eg: client can't push and hangs forever.
func PrepareConsoleLoggerLevel(defaultLevel log.Level) func(context.Context, *cli.Command) (context.Context, error) {
	return func(ctx context.Context, c *cli.Command) (context.Context, error) {
		if setting.InstallLock {
			// During config loading, there might also be logs (for example: deprecation warnings).
			// It must make sure that console logger is set up before config is loaded.
			log.Error("Config is loaded before console logger is setup, it will cause bugs. Please fix it. CustomConf=%s", setting.CustomConf)
			return nil, errors.New("console logger must be setup before config is loaded")
		}
		level := defaultLevel
		if globalBool(c, "quiet") {
			level = log.FATAL
		}
		if globalBool(c, "debug") || globalBool(c, "verbose") {
			level = log.TRACE
		}
		log.SetupStderrLogger(log.DEFAULT, "console-stderr", level)
		return ctx, nil
	}
}

func isValidDefaultSubCommand(cmd *cli.Command) (string, bool) {
	// Dirty patch for urfave/cli's strange design.
	// "./gitea bad-cmd" should not start the web server.
	args := cmd.Args().Slice()
	if len(args) != 0 {
		return args[0], false
	}
	return "", true
}
