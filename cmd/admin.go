// Copyright 2016 The Gogs Authors. All rights reserved.
// Copyright 2016 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package cmd

import (


	"github.com/urfave/cli/v3"
)

func newAdminCommand() *cli.Command {
	return &cli.Command{
		Name:  "admin",
		Usage: "Perform common administrative operations",
		Commands: []*cli.Command{
			newUserCommand(),
			newRegenerateCommand(),
		},
	}
}

func newRegenerateCommand() *cli.Command {
	return &cli.Command{
		Name:  "regenerate",
		Usage: "Regenerate specific files",
		Commands: []*cli.Command{
			newRegenerateHooksCommand(),
		},
	}
}
