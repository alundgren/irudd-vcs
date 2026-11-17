// Copyright 2024 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package git

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"

	"gitea.dev/modules/git/gitcmd"
	"gitea.dev/modules/setting"
)

// syncGitConfig only modifies gitconfig, won't change global variables (otherwise there will be data-race problem)
func syncGitConfig(ctx context.Context) (err error) {
	if err = os.MkdirAll(gitcmd.HomeDir(), os.ModePerm); err != nil {
		return fmt.Errorf("unable to prepare git home directory %s, err: %w", gitcmd.HomeDir(), err)
	}
	c := loadGlobalConfig(ctx)

	// Git requires setting user.name and user.email in order to commit changes - old comment: "if they're not set just add some defaults"
	// TODO: need to confirm whether users really need to change these values manually. It seems that these values are dummy only and not really used.
	// If these values are not really used, then they can be set (overwritten) directly without considering about existence.
	for configKey, defaultValue := range map[string]string{
		"user.name":  "Gitea",
		"user.email": "gitea@fake.local",
	} {
		if err := c.setNonExist(configKey, defaultValue); err != nil {
			return err
		}
	}

	// Set git some configurations - these must be set to these values for gitea to work correctly
	if err := c.set("core.quotePath", "false"); err != nil {
		return err
	}

	if err := c.set("receive.advertisePushOptions", "true"); err != nil {
		return err
	}

	if DefaultFeatures().CheckVersionAtLeast("2.18") {
		if err := c.set("core.commitGraph", "true"); err != nil {
			return err
		}
		if err := c.set("gc.writeCommitGraph", "true"); err != nil {
			return err
		}
		if err := c.set("fetch.writeCommitGraph", "true"); err != nil {
			return err
		}
	}

	// AGit flow is not supported: make sure refs/for is not routed to a proc-receive hook
	if err := c.unsetAll("receive.procReceiveRefs", "refs/for"); err != nil {
		return err
	}

	// Due to CVE-2022-24765, git now denies access to git directories which are not owned by current user.
	// However, some docker users and samba users find it difficult to configure their systems correctly,
	// so that Gitea's git repositories are owned by the Gitea user.
	// (Possibly Windows Service users - but ownership in this case should really be set correctly on the filesystem.)
	// See issue: https://github.com/go-gitea/gitea/issues/19455
	// As Gitea now always use its internal git config file, and access to the git repositories is managed through Gitea,
	// it is now safe to set "safe.directory=*" for internal usage only.
	// Although this setting is only supported by some new git versions, it is also tolerated by earlier versions
	if err := c.addNonExist("safe.directory", "*"); err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		if err := c.set("core.longpaths", "true"); err != nil {
			return err
		}
		if setting.Git.DisableCoreProtectNTFS {
			err = c.set("core.protectNTFS", "false")
		} else {
			err = c.unsetAll("core.protectNTFS", "false")
		}
		if err != nil {
			return err
		}
	}

	// By default, partial clones are disabled, enable them from git v2.22
	if !setting.Git.DisablePartialClone && DefaultFeatures().CheckVersionAtLeast("2.22") {
		if err = c.set("uploadpack.allowfilter", "true"); err != nil {
			return err
		}
		err = c.set("uploadpack.allowAnySHA1InWant", "true")
	} else {
		if err = c.unsetAll("uploadpack.allowfilter", "true"); err != nil {
			return err
		}
		err = c.unsetAll("uploadpack.allowAnySHA1InWant", "true")
	}
	if err != nil {
		return err
	}

	// Apply user's git config options last so they take precedence over builtin defaults
	for k, v := range setting.GitConfig.Options {
		if err = c.set(strings.ToLower(k), v); err != nil {
			return err
		}
	}

	GlobalConfig = &GlobalConfigStruct{}
	// HINT: GIT-DIFF-TREE-UI-CONFIG: Git's bug: git-diff-tree loads config with /* no "diff" UI options */ (since 20 years ago).
	// https://github.com/git/git/blame/5d2e7709234afea1b6ddb25cd4f60d3d5fb3c200/builtin/diff-tree.c#L127
	// Although document and manual say that "git-diff-tree" supports "diff.orderfile" option, but it is not actually supported.
	// So we need to apply the diff.orderfile explicitly in our code.
	GlobalConfig.DiffOrderFile, _ = c.get("diff.orderfile")
	return nil
}

// globalConfig reads and changes Gitea's global git config. It holds the values read with one git process,
// since a process per key slows startup down (about 15 ms each on Windows); nil values mean every lookup runs git.
type globalConfig struct {
	ctx    context.Context
	values map[string][]string
}

// loadGlobalConfig reads all global values at once, and falls back to a lookup per key if that fails
func loadGlobalConfig(ctx context.Context) *globalConfig {
	c := &globalConfig{ctx: ctx}
	stdout, _, err := gitcmd.NewCommand("config", "--global", "--list", "-z").RunStdString(ctx)
	if err != nil {
		if !gitcmd.IsErrorExitCode(err, 128) { // 128: there is no global config file yet
			return c
		}
		stdout = ""
	}
	c.values = map[string][]string{}
	for entry := range strings.SplitSeq(stdout, "\x00") {
		if key, value, ok := strings.Cut(entry, "\n"); ok || key != "" {
			c.values[key] = append(c.values[key], value)
		}
	}
	return c
}

// configKeyCanonical lower-cases the section and the name like "git config --list", the subsection keeps its case
func configKeyCanonical(key string) string {
	first, last := strings.IndexByte(key, '.'), strings.LastIndexByte(key, '.')
	if first < 0 || first == last {
		return strings.ToLower(key)
	}
	return strings.ToLower(key[:first]) + key[first:last] + strings.ToLower(key[last:])
}

// lookup returns the values of key, or runs "git config --get" if the values were not read
func (c *globalConfig) lookup(key string) (last string, exist bool, err error) {
	if c.values != nil {
		values := c.values[configKeyCanonical(key)]
		if len(values) == 0 {
			return "", false, nil
		}
		return values[len(values)-1], true, nil
	}
	stdout, _, err := gitcmd.NewCommand("config", "--global", "--get").AddDynamicArguments(key).RunStdString(c.ctx)
	if gitcmd.IsErrorExitCode(err, 1) {
		return "", false, nil
	} else if err != nil {
		return "", false, fmt.Errorf("failed to get git config %s, err: %w", key, err)
	}
	return strings.TrimRight(stdout, "\r\n"), true, nil
}

func (c *globalConfig) get(key string) (string, error) {
	value, _, err := c.lookup(key)
	return value, err
}

func (c *globalConfig) set(key, value string) error {
	currValue, _, err := c.lookup(key)
	if err != nil {
		return err
	}
	if strings.TrimSpace(currValue) == value {
		return nil
	}

	if _, _, err = gitcmd.NewCommand("config", "--global").
		AddDynamicArguments(key, value).
		RunStdString(c.ctx); err != nil {
		return fmt.Errorf("failed to set git global config %s, err: %w", key, err)
	}
	if c.values != nil {
		c.values[configKeyCanonical(key)] = []string{value}
	}
	return nil
}

func (c *globalConfig) setNonExist(key, value string) error {
	if _, exist, err := c.lookup(key); err != nil || exist {
		return err
	}
	if _, _, err := gitcmd.NewCommand("config", "--global").AddDynamicArguments(key, value).RunStdString(c.ctx); err != nil {
		return fmt.Errorf("failed to set git global config %s, err: %w", key, err)
	}
	if c.values != nil {
		c.values[configKeyCanonical(key)] = []string{value}
	}
	return nil
}

// addNonExist adds the value unless one contains it, like the unanchored value pattern of "git config --get"
func (c *globalConfig) addNonExist(key, value string) error {
	if c.values != nil {
		for _, v := range c.values[configKeyCanonical(key)] {
			if strings.Contains(v, value) {
				return nil
			}
		}
	} else {
		_, _, err := gitcmd.NewCommand("config", "--global", "--get").AddDynamicArguments(key, regexp.QuoteMeta(value)).RunStdString(c.ctx)
		if err == nil {
			return nil // already exist
		} else if !gitcmd.IsErrorExitCode(err, 1) {
			return fmt.Errorf("failed to get git config %s, err: %w", key, err)
		}
	}
	if _, _, err := gitcmd.NewCommand("config", "--global", "--add").AddDynamicArguments(key, value).RunStdString(c.ctx); err != nil {
		return fmt.Errorf("failed to add git global config %s, err: %w", key, err)
	}
	if c.values != nil {
		c.values[configKeyCanonical(key)] = append(c.values[configKeyCanonical(key)], value)
	}
	return nil
}

func (c *globalConfig) unsetAll(key, value string) error {
	if _, exist, err := c.lookup(key); err != nil || !exist {
		return err
	}
	if _, _, err := gitcmd.NewCommand("config", "--global", "--unset-all").AddDynamicArguments(key, regexp.QuoteMeta(value)).RunStdString(c.ctx); err != nil {
		return fmt.Errorf("failed to unset git global config %s, err: %w", key, err)
	}
	if c.values != nil {
		canonical := configKeyCanonical(key)
		kept := c.values[canonical][:0]
		for _, v := range c.values[canonical] {
			if !strings.Contains(v, value) {
				kept = append(kept, v)
			}
		}
		c.values[canonical] = kept
	}
	return nil
}
