// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	auth_model "gitea.dev/models/auth"
	"gitea.dev/models/db"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/optional"
	"gitea.dev/modules/setting"

	"github.com/urfave/cli/v3"
	"xorm.io/builder"
)

func microcmdUserCreate() *cli.Command {
	return &cli.Command{
		Name:   "create",
		Usage:  "Create the owner account and an access token for it",
		Action: runCreateUser,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "username",
				Usage:    "Username",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "email",
				Usage:    "User email address",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "access-token-name",
				Usage: `Name of the generated access token`,
				Value: "gitea-admin",
			},
			&cli.StringFlag{
				Name:  "access-token-scopes",
				Usage: `Scopes of the generated access token, comma separated. Examples: "all", "public-only,read:issue", "write:repository,write:user"`,
				Value: "all",
			},
			&cli.StringFlag{
				Name:  "fullname",
				Usage: `The full, human-readable name of the user`,
			},
		},
	}
}

func runCreateUser(ctx context.Context, c *cli.Command) error {
	// this command highly depends on the many setting options (visibility, etc.), so it must have a full setting load first
	// duplicate setting loading should be safe at the moment, but it should be refactored & improved in the future.
	setting.LoadSettings()

	if !setting.IsInTesting {
		// FIXME: need to refactor the "initDB" related code later
		// it doesn't make sense to call it in (almost) every command action function
		if err := initDB(ctx); err != nil {
			return err
		}
	}

	accessTokenName := strings.TrimSpace(c.String("access-token-name"))
	if accessTokenName == "" {
		return errors.New("access-token-name cannot be empty")
	}
	accessTokenScope, err := auth_model.AccessTokenScope(c.String("access-token-scopes")).Normalize()
	if err != nil {
		return fmt.Errorf("invalid access token scope provided: %w", err)
	}
	if !accessTokenScope.HasPermissionScope() {
		return errors.New("access token does not have any permission")
	}

	// the server has a single owner, who signs in with access tokens only
	hasOwner, err := db.GetEngine(ctx).Where(builder.Eq{"type": user_model.UserTypeIndividual}).Exist(&user_model.User{})
	if err != nil {
		return err
	}
	if hasOwner {
		return errors.New("the owner account already exists, there can only be one user")
	}

	u := &user_model.User{
		Name:       c.String("username"),
		Email:      c.String("email"),
		IsAdmin:    true,
		Type:       user_model.UserTypeIndividual,
		Visibility: setting.Service.DefaultUserVisibilityMode,
		FullName:   c.String("fullname"),
	}
	if err := user_model.CreateUser(ctx, u, &user_model.Meta{}, &user_model.CreateUserOverwriteOptions{IsActive: optional.Some(true)}); err != nil {
		return fmt.Errorf("CreateUser: %w", err)
	}
	fmt.Printf("New user '%s' has been successfully created!\n", u.Name)

	t := &auth_model.AccessToken{Name: accessTokenName, UID: u.ID, Scope: accessTokenScope}
	if err := auth_model.NewAccessToken(ctx, t); err != nil {
		return err
	}
	fmt.Printf("Access token was successfully created... %s\n", t.Token)
	return nil
}
