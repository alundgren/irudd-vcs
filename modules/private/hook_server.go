// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package private

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"gitea.dev/modules/git"
	"gitea.dev/modules/log"
	"gitea.dev/modules/repository"
	"gitea.dev/modules/util"
)

const serverPushBatchSize = 500 // as the hook command batches

// RunServerPush runs a push from the server with the post-receive hook writing its refs to a file, and then
// sends the refs to the post-receive handler as the hook would: starting gitea for it costs 40-60 ms on Windows.
// Hooks from before this ignore the file and start gitea, which leaves the file empty.
func RunServerPush(ctx context.Context, env []string, push func(env []string) error) error {
	f, err := os.CreateTemp("", "gitea-post-receive-")
	if err != nil {
		log.Warn("post-receive file for a server push: %v", err)
		return push(env)
	}
	file := f.Name()
	_ = f.Close()
	defer func() { _ = util.Remove(file) }()

	pushErr := push(append(env, repository.EnvPostReceiveFile+"="+filepath.ToSlash(file)))
	// git runs post-receive for the refs a failed push did update, so these are handled too, and as with
	// the hook a failure here does not fail the push
	if err := serverPostReceiveFile(ctx, env, file); err != nil {
		log.Error("post-receive of a server push: %v", err)
	}
	return pushErr
}

// ServerPostReceive sends a ref that the server updated without a push to the post-receive handler, as RunServerPush does
func ServerPostReceive(ctx context.Context, env []string, oldCommitID, newCommitID string, refName git.RefName) error {
	return serverPostReceive(ctx, env, strings.NewReader(oldCommitID+" "+newCommitID+" "+refName.String()+"\n"))
}

func serverPostReceiveFile(ctx context.Context, env []string, file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	return serverPostReceive(ctx, env, f)
}

func serverPostReceive(ctx context.Context, env []string, refs io.Reader) error {
	getEnv := func(key string) string {
		for _, kv := range slices.Backward(env) { // the last one wins, as in exec
			if k, v, ok := strings.Cut(kv, "="); ok && k == key {
				return v
			}
		}
		return ""
	}
	// the hook command does nothing for these
	if isInternal, _ := strconv.ParseBool(getEnv(repository.EnvIsInternal)); isInternal || getEnv("SSH_ORIGINAL_COMMAND") == "" {
		return nil
	}
	pusherID, _ := strconv.ParseInt(getEnv(repository.EnvPusherID), 10, 64)
	prID, _ := strconv.ParseInt(getEnv(repository.EnvPRID), 10, 64)
	opts := HookOptions{
		UserName:       getEnv(repository.EnvPusherName),
		UserID:         pusherID,
		PullRequestID:  prID,
		PushTrigger:    repository.PushTrigger(getEnv(repository.EnvPushTrigger)),
		GitPushOptions: map[string]string{}, // a server push sets none
	}
	repoUser, repoName := getEnv(repository.EnvRepoUsername), getEnv(repository.EnvRepoName)

	send := func() error {
		if len(opts.RefFullNames) == 0 {
			return nil
		}
		if _, extra := HookPostReceive(ctx, repoUser, repoName, opts); extra.HasError() {
			return fmt.Errorf("%s: %w", extra.UserMsg, extra.Error)
		}
		opts.OldCommitIDs, opts.NewCommitIDs, opts.RefFullNames = nil, nil, nil
		return nil
	}
	scanner := bufio.NewScanner(refs)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), " ")
		if len(fields) != 3 {
			continue
		}
		opts.OldCommitIDs = append(opts.OldCommitIDs, fields[0])
		opts.NewCommitIDs = append(opts.NewCommitIDs, fields[1])
		opts.RefFullNames = append(opts.RefFullNames, git.RefName(fields[2]))
		if len(opts.RefFullNames) >= serverPushBatchSize {
			if err := send(); err != nil {
				return err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return send()
}
