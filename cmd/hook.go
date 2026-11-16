// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"gitea.dev/modules/git"
	"gitea.dev/modules/git/gitcmd"
	"gitea.dev/modules/log"
	"gitea.dev/modules/private"
	repo_module "gitea.dev/modules/repository"
	"gitea.dev/modules/setting"

	"github.com/urfave/cli/v3"
)

const (
	hookBatchSize = 500
)

func newHookCommand() *cli.Command {
	return &cli.Command{
		Name:        "hook",
		Usage:       "(internal) Should only be called by Git",
		Hidden:      true, // internal commands shouldn't be visible
		Description: "Delegate commands to corresponding Git hooks",
		Before:      PrepareConsoleLoggerLevel(log.FATAL),
		Commands: []*cli.Command{
			newHookPreReceiveCommand(),
			newHookUpdateCommand(),
			newHookPostReceiveCommand(),
		},
	}
}

func newHookPreReceiveCommand() *cli.Command {
	return &cli.Command{
		Name:        "pre-receive",
		Usage:       "Delegate pre-receive Git hook",
		Description: "This command should only be called by Git",
		Action:      runHookPreReceive,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name: "debug",
			},
		},
	}
}

func newHookUpdateCommand() *cli.Command {
	return &cli.Command{
		Name:        "update",
		Usage:       "Delegate update Git hook",
		Description: "This command should only be called by Git",
		Action:      runHookUpdate,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name: "debug",
			},
		},
	}
}

func newHookPostReceiveCommand() *cli.Command {
	return &cli.Command{
		Name:        "post-receive",
		Usage:       "Delegate post-receive Git hook",
		Description: "This command should only be called by Git",
		Action:      runHookPostReceive,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name: "debug",
			},
		},
	}
}

type delayWriter struct {
	internal io.Writer
	buf      *bytes.Buffer
	timer    *time.Timer
}

func newDelayWriter(internal io.Writer, delay time.Duration) *delayWriter {
	timer := time.NewTimer(delay)
	return &delayWriter{
		internal: internal,
		buf:      &bytes.Buffer{},
		timer:    timer,
	}
}

func (d *delayWriter) Write(p []byte) (n int, err error) {
	if d.buf != nil {
		select {
		case <-d.timer.C:
			_, err := d.internal.Write(d.buf.Bytes())
			if err != nil {
				return 0, err
			}
			d.buf = nil
			return d.internal.Write(p)
		default:
			return d.buf.Write(p)
		}
	}
	return d.internal.Write(p)
}

func (d *delayWriter) WriteString(s string) (n int, err error) {
	if d.buf != nil {
		select {
		case <-d.timer.C:
			_, err := d.internal.Write(d.buf.Bytes())
			if err != nil {
				return 0, err
			}
			d.buf = nil
			return d.internal.Write([]byte(s))
		default:
			return d.buf.WriteString(s)
		}
	}
	return d.internal.Write([]byte(s))
}

func (d *delayWriter) Close() error {
	stopped := d.timer.Stop()
	if stopped || d.buf == nil {
		return nil
	}
	_, err := d.internal.Write(d.buf.Bytes())
	d.buf = nil
	return err
}

func parseGitHookCommitRefLine(line string) (oldCommitID, newCommitID string, refFullName git.RefName, ok bool) {
	fields := strings.Split(line, " ")
	if len(fields) != 3 {
		return "", "", "", false
	}
	return fields[0], fields[1], git.RefName(fields[2]), true
}

func runHookPreReceive(ctx context.Context, c *cli.Command) error {
	if isInternal, _ := strconv.ParseBool(os.Getenv(repo_module.EnvIsInternal)); isInternal {
		return nil
	}

	setup(ctx, c.Bool("debug"))

	if len(os.Getenv("SSH_ORIGINAL_COMMAND")) == 0 {
		if setting.OnlyAllowPushIfGiteaEnvironmentSet {
			return fail(ctx, `Rejecting changes as Gitea environment not set.
If you are pushing over SSH you must push with a key managed by
Gitea or set your environment appropriately.`, "")
		}
		return nil
	}

	// the environment is set by serv command
	username := os.Getenv(repo_module.EnvRepoUsername)
	reponame := os.Getenv(repo_module.EnvRepoName)
	userID, _ := strconv.ParseInt(os.Getenv(repo_module.EnvPusherID), 10, 64)
	prID, _ := strconv.ParseInt(os.Getenv(repo_module.EnvPRID), 10, 64)

	hookOptions := private.HookOptions{
		UserID:                          userID,
		GitAlternativeObjectDirectories: os.Getenv(private.GitAlternativeObjectDirectories),
		GitObjectDirectory:              os.Getenv(private.GitObjectDirectory),
		GitQuarantinePath:               os.Getenv(private.GitQuarantinePath),
		GitPushOptions:                  pushOptions(),
		PullRequestID:                   prID,
	}

	scanner := bufio.NewScanner(os.Stdin)

	oldCommitIDs := make([]string, hookBatchSize)
	newCommitIDs := make([]string, hookBatchSize)
	refFullNames := make([]git.RefName, hookBatchSize)
	count := 0
	total := 0
	lastline := 0

	out := io.Discard
	if setting.Git.VerbosePush {
		if setting.Git.VerbosePushDelay > 0 {
			dWriter := newDelayWriter(os.Stdout, setting.Git.VerbosePushDelay)
			defer dWriter.Close()
			out = dWriter
		} else {
			out = os.Stdout
		}
	}

	for scanner.Scan() {
		oldCommitID, newCommitID, refFullName, ok := parseGitHookCommitRefLine(scanner.Text())
		if !ok {
			continue
		}

		total++
		lastline++

		// check every ref, not only branches and tags, so the server can reject writes to refs like refs/pull/*
		{
			oldCommitIDs[count] = oldCommitID
			newCommitIDs[count] = newCommitID
			refFullNames[count] = refFullName
			count++
			fmt.Fprintf(out, "*")

			if count >= hookBatchSize {
				fmt.Fprintf(out, " Checking %d references\n", count)

				hookOptions.OldCommitIDs = oldCommitIDs
				hookOptions.NewCommitIDs = newCommitIDs
				hookOptions.RefFullNames = refFullNames
				extra := private.HookPreReceive(ctx, username, reponame, hookOptions)
				if extra.HasError() {
					return fail(ctx, extra.UserMsg, "HookPreReceive(batch) failed: %v", extra.Error)
				}
				count = 0
				lastline = 0
			}
		}
		if lastline >= hookBatchSize {
			fmt.Fprintf(out, "\n")
			lastline = 0
		}
	}
	if err := scanner.Err(); err != nil {
		return fail(ctx, "Hook failed: stdin read error", "scanner error: %v", err)
	}

	if count > 0 {
		hookOptions.OldCommitIDs = oldCommitIDs[:count]
		hookOptions.NewCommitIDs = newCommitIDs[:count]
		hookOptions.RefFullNames = refFullNames[:count]

		fmt.Fprintf(out, " Checking %d references\n", count)

		extra := private.HookPreReceive(ctx, username, reponame, hookOptions)
		if extra.HasError() {
			return fail(ctx, extra.UserMsg, "HookPreReceive(last) failed: %v", extra.Error)
		}
	} else if lastline > 0 {
		fmt.Fprintf(out, "\n")
	}

	fmt.Fprintf(out, "Checked %d references in total\n", total)
	return nil
}

// runHookUpdate avoid to do heavy operations on update hook because it will be
// invoked for every ref update which does not like pre-receive and post-receive
func runHookUpdate(_ context.Context, c *cli.Command) error {
	if isInternal, _ := strconv.ParseBool(os.Getenv(repo_module.EnvIsInternal)); isInternal {
		return nil
	}

	// Update is empty and is kept only for backwards compatibility
	if len(os.Args) < 3 {
		return nil
	}
	refName := git.RefName(os.Args[len(os.Args)-3])
	if refName.IsPull() {
		// ignore update to refs/pull/xxx/head, so we don't need to output any information
		os.Exit(1)
	}
	return nil
}

func runHookPostReceive(ctx context.Context, c *cli.Command) error {
	setup(ctx, c.Bool("debug"))

	// First of all run update-server-info no matter what
	if err := gitcmd.NewCommand("update-server-info").RunWithStderr(ctx); err != nil {
		return fmt.Errorf("failed to call 'git update-server-info': %w", err)
	}

	// Now if we're an internal don't do anything else
	if isInternal, _ := strconv.ParseBool(os.Getenv(repo_module.EnvIsInternal)); isInternal {
		return nil
	}

	if len(os.Getenv("SSH_ORIGINAL_COMMAND")) == 0 {
		if setting.OnlyAllowPushIfGiteaEnvironmentSet {
			return fail(ctx, `Rejecting changes as Gitea environment not set.
If you are pushing over SSH you must push with a key managed by
Gitea or set your environment appropriately.`, "")
		}
		return nil
	}

	out := io.Discard
	if setting.Git.VerbosePush {
		if setting.Git.VerbosePushDelay > 0 {
			dWriter := newDelayWriter(os.Stdout, setting.Git.VerbosePushDelay)
			defer dWriter.Close()
			out = dWriter
		} else {
			out = os.Stdout
		}
	}

	// the environment is set by serv command
	repoUser := os.Getenv(repo_module.EnvRepoUsername)
	repoName := os.Getenv(repo_module.EnvRepoName)
	pusherID, _ := strconv.ParseInt(os.Getenv(repo_module.EnvPusherID), 10, 64)
	prID, _ := strconv.ParseInt(os.Getenv(repo_module.EnvPRID), 10, 64)
	pusherName := os.Getenv(repo_module.EnvPusherName)

	hookOptions := private.HookOptions{
		UserName:                        pusherName,
		UserID:                          pusherID,
		GitAlternativeObjectDirectories: os.Getenv(private.GitAlternativeObjectDirectories),
		GitObjectDirectory:              os.Getenv(private.GitObjectDirectory),
		GitQuarantinePath:               os.Getenv(private.GitQuarantinePath),
		GitPushOptions:                  pushOptions(),
		PullRequestID:                   prID,
		PushTrigger:                     repo_module.PushTrigger(os.Getenv(repo_module.EnvPushTrigger)),
	}

	oldCommitIDs := make([]string, 0, hookBatchSize)
	newCommitIDs := make([]string, 0, hookBatchSize)
	refFullNames := make([]git.RefName, 0, hookBatchSize)
	results := make([]private.HookPostReceiveBranchResult, 0)

	defer func() {
		hookPrintResults(results)
	}()

	processBatch := func() error {
		if len(refFullNames) == 0 {
			return nil
		}
		_, _ = fmt.Fprintf(out, " Processing %d references\n", len(refFullNames))
		hookOptions.OldCommitIDs = oldCommitIDs
		hookOptions.NewCommitIDs = newCommitIDs
		hookOptions.RefFullNames = refFullNames
		resp, extra := private.HookPostReceive(ctx, repoUser, repoName, hookOptions)
		if extra.HasError() {
			return fail(ctx, extra.UserMsg, "HookPostReceive failed: %v", extra.Error)
		}
		results = append(results, resp.Results...)
		oldCommitIDs = oldCommitIDs[:0]
		newCommitIDs = newCommitIDs[:0]
		refFullNames = refFullNames[:0]
		return nil
	}

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		oldCommitID, newCommitID, refFullName, ok := parseGitHookCommitRefLine(scanner.Text())
		if !ok {
			continue
		}
		_, _ = fmt.Fprintf(out, ".")

		oldCommitIDs = append(oldCommitIDs, oldCommitID)
		newCommitIDs = append(newCommitIDs, newCommitID)
		refFullNames = append(refFullNames, refFullName)
		if len(refFullNames) >= hookBatchSize {
			// process and start a new batch
			if err := processBatch(); err != nil {
				return err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fail(ctx, "Hook failed: stdin read error", "scanner error: %v", err)
	}
	return processBatch()
}

func hookPrintResults(results []private.HookPostReceiveBranchResult) {
	for _, res := range results {
		hookPrintResult(res.Message, res.Create, res.Branch, res.URL)
	}
}

func hookPrintResult(output, isCreate bool, branch, url string) {
	if !output {
		return
	}
	fmt.Fprintln(os.Stderr, "")
	if isCreate {
		fmt.Fprintf(os.Stderr, "Create a new pull request for '%s':\n", branch)
		fmt.Fprintf(os.Stderr, "  %s\n", url)
	} else {
		fmt.Fprint(os.Stderr, "Visit the existing pull request:\n")
		fmt.Fprintf(os.Stderr, "  %s\n", url)
	}
	fmt.Fprintln(os.Stderr, "")
	_ = os.Stderr.Sync()
}

func pushOptions() map[string]string {
	opts := make(map[string]string)
	if pushCount, err := strconv.Atoi(os.Getenv(private.GitPushOptionCount)); err == nil {
		for idx := range pushCount {
			opt := os.Getenv(fmt.Sprintf("GIT_PUSH_OPTION_%d", idx))
			kv := strings.SplitN(opt, "=", 2)
			if len(kv) == 2 {
				opts[kv[0]] = kv[1]
			}
		}
	}
	return opts
}

// git PKT-Line api
// pktLineType message type of pkt-line
type pktLineType int64

const (
	_ pktLineType = 0 // unknown type
	// flush-pkt "0000"
	pktLineTypeFlush pktLineType = iota
	// data line
	pktLineTypeData
)

// gitPktLine pkt-line api
type gitPktLine struct {
	Type   pktLineType
	Length uint64
	Data   []byte
}

func readPktLine(ctx context.Context, in *bufio.Reader, requestType pktLineType) (*gitPktLine, error) {
	var (
		err error
		r   *gitPktLine
	)

	// read prefix
	lengthBytes := make([]byte, 4)
	for i := range 4 {
		lengthBytes[i], err = in.ReadByte()
		if err != nil {
			return nil, fail(ctx, "Protocol: stdin error", "Pkt-Line: read stdin failed : %v", err)
		}
	}

	r = new(gitPktLine)
	r.Length, err = strconv.ParseUint(string(lengthBytes), 16, 32)
	if err != nil {
		return nil, fail(ctx, "Protocol: format parse error", "Pkt-Line format is wrong :%v", err)
	}

	if r.Length == 0 {
		if requestType == pktLineTypeData {
			return nil, fail(ctx, "Protocol: format data error", "Pkt-Line format is wrong")
		}
		r.Type = pktLineTypeFlush
		return r, nil
	}

	if r.Length <= 4 || r.Length > 65520 || requestType == pktLineTypeFlush {
		return nil, fail(ctx, "Protocol: format length error", "Pkt-Line format is wrong")
	}

	r.Data = make([]byte, r.Length-4)
	for i := range r.Data {
		r.Data[i], err = in.ReadByte()
		if err != nil {
			return nil, fail(ctx, "Protocol: data error", "Pkt-Line: read stdin failed : %v", err)
		}
	}

	r.Type = pktLineTypeData

	return r, nil
}

func writeFlushPktLine(ctx context.Context, out io.Writer) error {
	l, err := out.Write([]byte("0000"))
	if err != nil || l != 4 {
		return fail(ctx, "Protocol: write error", "Pkt-Line response failed: %v", err)
	}
	return nil
}

func writeDataPktLine(ctx context.Context, out io.Writer, data []byte) error {
	hexchar := []byte("0123456789abcdef")
	hex := func(n uint64) byte {
		return hexchar[n&15]
	}

	length := uint64(len(data) + 4)
	tmp := make([]byte, 4)
	tmp[0] = hex(length >> 12)
	tmp[1] = hex(length >> 8)
	tmp[2] = hex(length >> 4)
	tmp[3] = hex(length)

	lr, err := out.Write(tmp)
	if err != nil || lr != 4 {
		return fail(ctx, "Protocol: write error", "Pkt-Line response failed: %v", err)
	}

	lr, err = out.Write(data)
	if err != nil || int(length-4) != lr {
		return fail(ctx, "Protocol: write error", "Pkt-Line response failed: %v", err)
	}

	return nil
}
