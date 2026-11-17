// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package pull

import (
	"context"
	"fmt"

	issues_model "gitea.dev/models/issues"
	repo_model "gitea.dev/models/repo"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/container"
	"gitea.dev/modules/git"
	"gitea.dev/modules/git/gitcmd"
	"gitea.dev/modules/log"
	"gitea.dev/modules/setting"
)

// doMergeStyleSquash gets a commit author signature for squash commits
func getAuthorSignatureSquash(ctx *mergeContext) (*git.Signature, error) {
	gitRepo, err := git.OpenRepository(ctx, ctx.tmpBasePath)
	if err != nil {
		log.Error("%-v Unable to open base repository: %v", ctx.pr, err)
		return nil, err
	}
	defer gitRepo.Close()
	return squashAuthorSignature(ctx, ctx.pr, gitRepo, git.RefNameFromBranch(tmpRepoTrackingBranch), git.RefNameHead)
}

// squashAuthorSignature gets the author of a squash commit from the commits of the head that are not in the base
func squashAuthorSignature(ctx context.Context, pr *issues_model.PullRequest, gitRepo *git.Repository, headRef, baseRef git.RefName) (*git.Signature, error) {
	if err := pr.Issue.LoadPoster(ctx); err != nil {
		log.Error("%-v Issue[%d].LoadPoster: %v", pr, pr.Issue.ID, err)
		return nil, err
	}

	// Try to get a signature from the same user in one of the commits, as the
	// poster email might be private or commits might have a different signature
	// than the primary email address of the poster.
	commits, err := gitRepo.CommitsBetween(headRef, baseRef, -1)
	if err != nil {
		log.Error("%-v Unable to get commits between: head and tracking branch: %v", pr, err)
		return nil, err
	}

	uniqueEmails := make(container.Set[string])
	for _, commit := range commits {
		if commit.Author != nil && uniqueEmails.Add(commit.Author.Email) {
			commitUser, _ := user_model.GetUserByEmail(ctx, commit.Author.Email)
			if commitUser != nil && commitUser.ID == pr.Issue.Poster.ID {
				return commit.Author, nil
			}
		}
	}

	return pr.Issue.Poster.NewGitSig(), nil
}

// doMergeStyleSquash squashes the tracking branch on the current HEAD (=base)
func doMergeStyleSquash(ctx *mergeContext, message string) error {
	sig, err := getAuthorSignatureSquash(ctx)
	if err != nil {
		return fmt.Errorf("getAuthorSignatureSquash: %w", err)
	}

	cmdMerge := gitcmd.NewCommand("merge", "--squash").AddDynamicArguments(tmpRepoTrackingBranch)
	if err := runMergeCommand(ctx, repo_model.MergeStyleSquash, cmdMerge); err != nil {
		log.Error("%-v Unable to merge --squash tracking into base: %v", ctx.pr, err)
		return err
	}

	if setting.Repository.PullRequest.AddCoCommitterTrailers && ctx.committer.String() != sig.String() {
		message = AddCommitMessageTailer(message, git.CoAuthoredByTrailer, sig.String())
	}
	cmdCommit := gitcmd.NewCommand("commit").
		AddOptionFormat("--author='%s <%s>'", sig.Name, sig.Email).
		AddOptionFormat("--message=%s", message).
		AddArguments("--allow-empty")
	addCommitSigningOptions(cmdCommit)
	if err := ctx.PrepareGitCmd(cmdCommit).RunWithStderr(ctx); err != nil {
		log.Error("git commit %-v: %v\n%s\n%s", ctx.pr, err, ctx.outbuf.String(), err.Stderr())
		return fmt.Errorf("git commit [%s:%s -> %s:%s]: %w\n%s\n%s", ctx.pr.HeadRepo.FullName(), ctx.pr.HeadBranch, ctx.pr.BaseRepo.FullName(), ctx.pr.BaseBranch, err, ctx.outbuf.String(), err.Stderr())
	}
	ctx.outbuf.Reset()
	return nil
}
