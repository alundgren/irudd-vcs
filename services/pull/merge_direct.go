// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package pull

import (
	"context"
	"os"
	"strings"
	"time"

	issues_model "gitea.dev/models/issues"
	repo_model "gitea.dev/models/repo"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/git"
	"gitea.dev/modules/git/gitcmd"
	"gitea.dev/modules/gitrepo"
	"gitea.dev/modules/log"
	"gitea.dev/modules/private"
	repo_module "gitea.dev/modules/repository"
	"gitea.dev/modules/setting"
)

// mergeInRepository makes the merge or squash commit of a pull within one repository in that repository, without the
// temporary repository, its checkout and the push back, which take 300-400 ms on Windows. It applies only when
// a push would run nothing besides the generated hooks, which do nothing for it without ref rules. ok is false
// when it does not apply or git merge could report something, such as a conflict, and the caller merges as before.
func mergeInRepository(ctx context.Context, pr *issues_model.PullRequest, doer *user_model.User, mergeStyle repo_model.MergeStyle, expectedHeadCommitID, message string, pushTrigger repo_module.PushTrigger) (mergeCommitID string, ok bool, err error) {
	if (mergeStyle != repo_model.MergeStyleMerge && mergeStyle != repo_model.MergeStyleSquash) || setting.LFS.StartServer || !git.DefaultFeatures().SupportGitMergeTree ||
		pr.Flow != issues_model.PullRequestFlowGithub || pr.HeadRepoID != pr.BaseRepoID {
		return "", false, nil
	}
	if err := pr.LoadBaseRepo(ctx); err != nil || pr.LoadHeadRepo(ctx) != nil || pr.HeadRepo == nil {
		return "", false, nil //nolint:nilerr // the temporary repository reports it
	}
	repo := pr.BaseRepo
	noRefRules := repo_module.NoRefRulesEnvironment(ctx, doer, repo)
	if len(noRefRules) == 0 || !gitrepo.HasOnlyGiteaHooks(repo) {
		return "", false, nil
	}
	gitRepo, err := gitrepo.OpenRepository(ctx, repo)
	if err != nil {
		return "", false, nil //nolint:nilerr // the temporary repository reports it
	}
	defer gitRepo.Close()
	baseCommit, err := gitRepo.GetBranchCommit(pr.BaseBranch)
	if err != nil {
		return "", false, nil //nolint:nilerr // the temporary repository reports it
	}
	headCommitID, err := gitRepo.GetBranchCommitID(pr.HeadBranch)
	if err != nil {
		return "", false, nil //nolint:nilerr // the temporary repository reports it
	}
	if expectedHeadCommitID != "" && headCommitID != expectedHeadCommitID {
		return "", true, ErrSHADoesNotMatch{GivenSHA: expectedHeadCommitID, CurrentSHA: headCommitID}
	}
	baseCommitID := baseCommit.ID.String()

	// merge-tree merges as git merge does, and fails for conflicts and unrelated histories
	stdout, _, runErr := gitrepo.RunCmdString(ctx, repo, gitcmd.NewCommand("merge-tree", "--write-tree", "--no-messages").
		AddDynamicArguments(baseCommitID, headCommitID))
	treeID := strings.TrimSpace(stdout)
	// the tree of the base also comes from a head within it, for which git merge makes no merge commit
	if runErr != nil || treeID == "" || treeID == baseCommit.Tree.ID.String() {
		return "", false, nil
	}

	committer := doer.NewGitSig()
	author := committer
	cmdCommit := gitcmd.NewCommand("commit-tree", "--no-gpg-sign", "-F", "-").AddOptionValues("-p", baseCommitID)
	if mergeStyle == repo_model.MergeStyleMerge {
		cmdCommit.AddOptionValues("-p", headCommitID)
	} else {
		if err := pr.LoadIssue(ctx); err != nil {
			return "", false, nil //nolint:nilerr // the temporary repository reports it
		}
		if author, err = squashAuthorSignature(ctx, pr, gitRepo, git.RefName(headCommitID), git.RefName(baseCommitID)); err != nil {
			return "", false, nil //nolint:nilerr // the temporary repository reports it
		}
		if setting.Repository.PullRequest.AddCoCommitterTrailers && committer.String() != author.String() {
			message = AddCommitMessageTailer(message, git.CoAuthoredByTrailer, author.String())
		}
	}
	if message = stripCommitMessage(message); message == "" {
		return "", false, nil
	}
	commitTimeStr := time.Now().Format(time.RFC3339)
	stdout, _, runErr = gitrepo.RunCmdString(ctx, repo, cmdCommit.AddDynamicArguments(treeID).
		WithEnv(append(os.Environ(),
			"GIT_AUTHOR_NAME="+author.Name,
			"GIT_AUTHOR_EMAIL="+author.Email,
			"GIT_AUTHOR_DATE="+commitTimeStr,
			"GIT_COMMITTER_NAME="+committer.Name,
			"GIT_COMMITTER_EMAIL="+committer.Email,
			"GIT_COMMITTER_DATE="+commitTimeStr,
		)).WithStdinBytes([]byte(message)))
	if runErr != nil {
		log.Error("%-v commit-tree: %v", pr, runErr)
		return "", false, nil
	}
	mergeCommitID = strings.TrimSpace(stdout)

	// as for a push, the branch only moves from the commit that was merged
	if runErr := gitrepo.RunCmdWithStderr(ctx, repo, gitcmd.NewCommand("update-ref").
		AddDynamicArguments(git.BranchPrefix+pr.BaseBranch, mergeCommitID, baseCommitID)); runErr != nil {
		return "", true, &git.ErrPushOutOfDate{StdErr: runErr.Stderr(), Err: runErr}
	}

	headUser := doer
	if err := pr.HeadRepo.LoadOwner(ctx); err == nil {
		headUser = pr.HeadRepo.Owner
	} else {
		log.Warn("Can't find user: %d for head repository in %-v - defaulting to doer: %s - %v", pr.HeadRepo.OwnerID, pr, doer.Name, err)
	}
	env := append(repo_module.FullPushingEnvironment(headUser, doer, repo, pr.ID, pr.Index), repo_module.EnvPushTrigger+"="+string(pushTrigger))
	// as with the hook, a failure here does not fail the merge
	if err := private.ServerPostReceive(ctx, env, baseCommitID, mergeCommitID, git.RefNameFromBranch(pr.BaseBranch)); err != nil {
		log.Error("post-receive of the merge of %-v: %v", pr, err)
	}
	return mergeCommitID, true, nil
}

// stripCommitMessage cleans up a message as git commit does for one given with --message
func stripCommitMessage(message string) string {
	var b strings.Builder
	empties := 0
	for line := range strings.SplitSeq(message, "\n") {
		if line = strings.TrimRight(line, " \t\r"); line == "" { // the characters git takes for whitespace
			empties++
			continue
		}
		if empties > 0 && b.Len() > 0 {
			b.WriteByte('\n')
		}
		empties = 0
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
