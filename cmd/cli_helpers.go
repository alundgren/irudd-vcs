// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"fmt"
	"os"
	"unicode"

	"gitea.dev/modules/log"
	"gitea.dev/modules/private"
	"gitea.dev/modules/setting"

	"github.com/urfave/cli/v3"
)

func setup(ctx context.Context, debug bool) {
	if debug {
		setupConsoleLogger(log.TRACE, false, os.Stderr)
	} else {
		setupConsoleLogger(log.FATAL, false, os.Stderr)
	}
	setting.MustInstalled()
	if _, err := os.Stat(setting.RepoRootPath); err != nil {
		_ = fail(ctx, "Unable to access repository path", "Unable to access repository path %q, err: %v", setting.RepoRootPath, err)
		return
	}
	// no git.InitSimple: these commands only call the private API, and git version is a process per hook
}

// fail prints message to stdout, it's mainly used for git serv and git hook commands.
// The output will be passed to git client and shown to user.
func fail(ctx context.Context, userMessage, logMsgFmt string, args ...any) error {
	if userMessage == "" {
		userMessage = "Internal Server Error (no specific error)"
	}

	// There appears to be a chance to cause a zombie process and failure to read the Exit status
	// if nothing is outputted on stdout.
	_, _ = fmt.Fprintln(os.Stdout, "")
	// add extra empty lines to separate our message from other git errors to get more attention
	_, _ = fmt.Fprintln(os.Stderr, "error:")
	_, _ = fmt.Fprintln(os.Stderr, "error:", userMessage)
	_, _ = fmt.Fprintln(os.Stderr, "error:")

	if logMsgFmt != "" {
		logMsg := fmt.Sprintf(logMsgFmt, args...)
		if !setting.IsProd {
			_, _ = fmt.Fprintln(os.Stderr, "Gitea:", logMsg)
		}
		if unicode.IsPunct(rune(userMessage[len(userMessage)-1])) {
			logMsg = userMessage + " " + logMsg
		} else {
			logMsg = userMessage + ". " + logMsg
		}
		_ = private.SSHLog(ctx, true, logMsg)
	}
	return cli.Exit("", 1)
}

// handleCliResponseExtra handles the extra response from the cli sub-commands
// If there is a user message it will be printed to stdout
// If the command failed it will return an error (the error will be printed by cli framework)
func handleCliResponseExtra(extra private.ResponseExtra) error {
	if extra.UserMsg != "" {
		_, _ = fmt.Fprintln(os.Stdout, extra.UserMsg)
	}
	if extra.HasError() {
		return cli.Exit(extra.Error, 1)
	}
	return nil
}

// getAccessMode maps an SSH git/LFS verb to the access mode it requires, with
// ok=false for an unrecognised verb. Callers MUST reject the request when ok is
