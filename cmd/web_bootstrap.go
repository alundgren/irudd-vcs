// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"fmt"
	"os"

	"gitea.dev/modules/generate"
	"gitea.dev/modules/log"
	"gitea.dev/modules/setting"

	"github.com/urfave/cli/v3"
)

// bootstrapConfig replaces the removed install page: on first start, when the config file
// doesn't exist yet, it writes a minimal installed config and reloads the settings.
// An existing config file is never modified here.
func bootstrapConfig(cmd *cli.Command) error {
	if setting.InstallLock {
		return nil
	}
	if _, err := os.Stat(setting.CustomConf); err == nil {
		return fmt.Errorf("INSTALL_LOCK is not set in %q: irudd-vcs has no install page, set [security] INSTALL_LOCK = true", setting.CustomConf)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("unable to check config file %q: %w", setting.CustomConf, err)
	}

	secretKey, err := generate.NewSecretKey()
	if err != nil {
		return fmt.Errorf("unable to generate SECRET_KEY: %w", err)
	}
	cfg, err := setting.NewConfigProviderFromFile(setting.CustomConf)
	if err != nil {
		return err
	}
	cfg.Section("").Key("WORK_PATH").SetValue(setting.AppWorkPath)
	cfg.Section("security").Key("INSTALL_LOCK").SetValue("true")
	cfg.Section("security").Key("SECRET_KEY").SetValue(secretKey)
	if err = cfg.Save(); err != nil { // creates the directory and restricts the file to 0600
		return fmt.Errorf("unable to write config file %q: %w", setting.CustomConf, err)
	}

	prepareWorkPathAndCustomConf(cmd) // reload, also generates and saves INTERNAL_TOKEN and LFS_JWT_SECRET
	if !setting.InstallLock {
		return fmt.Errorf("INSTALL_LOCK is still not set after writing %q", setting.CustomConf)
	}
	if err := os.MkdirAll(setting.AppDataPath, os.ModePerm); err != nil {
		return fmt.Errorf("unable to create APP_DATA_PATH %q: %w", setting.AppDataPath, err)
	}

	log.Info("Created config file %q", setting.CustomConf)
	log.Info("Create the first admin with: gitea admin user create --admin --username NAME --email EMAIL --random-password")
	return nil
}
