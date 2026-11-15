// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gitea.dev/models/db"
	git_model "gitea.dev/models/git"
	issues_model "gitea.dev/models/issues"
	access_model "gitea.dev/models/perm/access"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unit"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/cache"
	"gitea.dev/modules/git"
	"gitea.dev/modules/gitrepo"
	"gitea.dev/modules/graceful"
	"gitea.dev/modules/log"
	"gitea.dev/modules/optional"
	"gitea.dev/modules/queue"
	repo_module "gitea.dev/modules/repository"
	"gitea.dev/modules/reqctx"
	"gitea.dev/modules/timeutil"
	"gitea.dev/modules/util"
	notify_service "gitea.dev/services/notify"
	release_service "gitea.dev/services/release"
)

// CreateNewBranch creates a new repository branch
func CreateNewBranch(ctx context.Context, doer *user_model.User, repo *repo_model.Repository, oldBranchName, branchName string) (err error) {
	branch, err := git_model.GetBranch(ctx, repo.ID, oldBranchName)
	if err != nil {
		return err
	}

	return CreateNewBranchFromCommit(ctx, doer, repo, branch.CommitID, branchName)
}

func getDivergenceCacheKey(repoID int64, branchName string) string {
	return fmt.Sprintf("%d-%s", repoID, branchName)
}

func DelDivergenceFromCache(repoID int64, branchName string) error {
	return cache.GetCache().Delete(getDivergenceCacheKey(repoID, branchName))
}

// DelRepoDivergenceFromCache deletes all divergence caches of a repository
func DelRepoDivergenceFromCache(ctx context.Context, repoID int64) error {
	dbBranches, err := db.Find[git_model.Branch](ctx, git_model.FindBranchOptions{
		RepoID:      repoID,
		ListOptions: db.ListOptionsAll,
	})
	if err != nil {
		return err
	}
	for i := range dbBranches {
		if err := DelDivergenceFromCache(repoID, dbBranches[i].Name); err != nil {
			log.Error("DelDivergenceFromCache: %v", err)
		}
	}
	return nil
}

// checkBranchName validates branch name with existing repository branches
func checkBranchName(ctx context.Context, repo *repo_model.Repository, name string) error {
	_, err := gitrepo.WalkReferences(ctx, repo, func(_, refName string) error {
		branchRefName := strings.TrimPrefix(refName, git.BranchPrefix)
		switch {
		case branchRefName == name:
			return git_model.ErrBranchAlreadyExists{
				BranchName: name,
			}
		// If branchRefName like "a/b" but we want to create a branch named a then we have a conflict
		case strings.HasPrefix(branchRefName, name+"/"):
			return git_model.ErrBranchNameConflict{
				BranchName: branchRefName,
			}
			// Conversely if branchRefName like "a" but we want to create a branch named "a/b" then we also have a conflict
		case strings.HasPrefix(name, branchRefName+"/"):
			return git_model.ErrBranchNameConflict{
				BranchName: branchRefName,
			}
		case refName == git.TagPrefix+name:
			return release_service.ErrTagAlreadyExists{
				TagName: name,
			}
		}
		return nil
	})
	return err
}

// SyncBranchesToDB sync the branch information in the database.
// It will check whether the branches of the repository have never been synced before.
// If so, it will sync all branches of the repository.
// Otherwise, it will sync the branches that need to be updated.
func SyncBranchesToDB(ctx context.Context, repoID, pusherID int64, branchNames, commitIDs []string, getCommit func(commitID string) (*git.Commit, error)) error {
	// Some designs that make the code look strange but are made for performance optimization purposes:
	// 1. Sync branches in a batch to reduce the number of DB queries.
	// 2. Lazy load commit information since it may be not necessary.
	// 3. Exit early if synced all branches of git repo when there's no branch in DB.
	// 4. Check the branches in DB if they are already synced.
	//
	// If the user pushes many branches at once, the Git hook will call the internal API in batches, rather than all at once.
	// See https://github.com/go-gitea/gitea/blob/cb52b17f92e2d2293f7c003649743464492bca48/cmd/hook.go#L27
	// For the first batch, it will hit optimization 3.
	// For other batches, it will hit optimization 4.

	if len(branchNames) != len(commitIDs) {
		return errors.New("branchNames and commitIDs length not match")
	}

	return db.WithTx(ctx, func(ctx context.Context) error {
		branches, err := git_model.GetBranches(ctx, repoID, branchNames, true)
		if err != nil {
			return fmt.Errorf("git_model.GetBranches: %v", err)
		}

		if len(branches) == 0 {
			// if user haven't visit UI but directly push to a branch after upgrading from 1.20 -> 1.21,
			// we cannot simply insert the branch but need to check we have branches or not
			hasBranch, err := db.Exist[git_model.Branch](ctx, git_model.FindBranchOptions{
				RepoID:          repoID,
				IsDeletedBranch: optional.Some(false),
			}.ToConds())
			if err != nil {
				return err
			}
			if !hasBranch {
				if _, err = repo_module.SyncRepoBranches(ctx, repoID, pusherID); err != nil {
					return fmt.Errorf("repo_module.SyncRepoBranches %d failed: %v", repoID, err)
				}
				return nil
			}
		}

		branchMap := make(map[string]*git_model.Branch, len(branches))
		for _, branch := range branches {
			branchMap[branch.Name] = branch
		}

		newBranches := make([]*git_model.Branch, 0, len(branchNames))

		for i, branchName := range branchNames {
			commitID := commitIDs[i]
			branch, exist := branchMap[branchName]
			if exist && branch.CommitID == commitID && !branch.IsDeleted {
				continue
			}

			commit, err := getCommit(commitID)
			if err != nil {
				return fmt.Errorf("get commit of %s failed: %v", branchName, err)
			}

			if exist {
				if _, err := git_model.UpdateBranch(ctx, repoID, pusherID, branchName, commit); err != nil {
					return fmt.Errorf("git_model.UpdateBranch %d:%s failed: %v", repoID, branchName, err)
				}
				continue
			}

			// if database have branches but not this branch, it means this is a new branch
			newBranches = append(newBranches, &git_model.Branch{
				RepoID:        repoID,
				Name:          branchName,
				CommitID:      commit.ID.String(),
				CommitMessage: commit.MessageTitle(),
				PusherID:      pusherID,
				CommitTime:    timeutil.TimeStamp(commit.Committer.When.Unix()),
			})
		}

		if len(newBranches) > 0 {
			return db.Insert(ctx, newBranches)
		}
		return nil
	})
}

// CreateNewBranchFromCommit creates a new repository branch
func CreateNewBranchFromCommit(ctx context.Context, doer *user_model.User, repo *repo_model.Repository, commitID, branchName string) (err error) {
	err = repo.MustNotBeArchived()
	if err != nil {
		return err
	}

	// Check if branch name can be used
	if err := checkBranchName(ctx, repo, branchName); err != nil {
		return err
	}

	if err := gitrepo.Push(ctx, repo, repo, git.PushOptions{
		Branch: fmt.Sprintf("%s:%s%s", commitID, git.BranchPrefix, branchName),
		Env:    repo_module.PushingEnvironment(doer, repo),
	}); err != nil {
		if git.IsErrPushOutOfDate(err) || git.IsErrPushRejected(err) {
			return err
		}
		return fmt.Errorf("push: %w", err)
	}
	return nil
}

// RenameBranch rename a branch
func RenameBranch(ctx context.Context, repo *repo_model.Repository, doer *user_model.User, from, to string) (string, error) {
	err := repo.MustNotBeArchived()
	if err != nil {
		return "", err
	}

	if from == to {
		return "target_exist", nil
	}

	if exist, _ := git_model.IsBranchExist(ctx, repo.ID, to); exist {
		return "target_exist", nil
	}

	fromBranch, err := git_model.GetBranch(ctx, repo.ID, from)
	if err != nil {
		if git_model.IsErrBranchNotExist(err) {
			return "from_not_exist", nil
		}
		return "", err
	}

	perm, err := access_model.GetDoerRepoPermission(ctx, repo, doer)
	if err != nil {
		return "", err
	}

	isDefault := from == repo.DefaultBranch
	if isDefault && !perm.IsAdmin() {
		return "", repo_model.ErrUserDoesNotHaveAccessToRepo{
			UserID:   doer.ID,
			RepoName: repo.LowerName,
		}
	}

	// If from == rule name, admins are allowed to modify them.
	if protectedBranch, err := git_model.GetProtectedBranchRuleByName(ctx, repo.ID, from); err != nil {
		return "", err
	} else if protectedBranch != nil && !perm.IsAdmin() {
		return "", repo_model.ErrUserDoesNotHaveAccessToRepo{
			UserID:   doer.ID,
			RepoName: repo.LowerName,
		}
	}

	// We also need to check if "to" matches with a protected branch rule.
	rule, err := git_model.GetFirstMatchProtectedBranchRule(ctx, repo.ID, to)
	if err != nil {
		return "", err
	}
	if rule != nil && !rule.CanUserPush(ctx, doer) {
		return "", git_model.ErrBranchIsProtected
	}

	if err := git_model.RenameBranch(ctx, repo, from, to, func(ctx context.Context, isDefault bool) error {
		err2 := gitrepo.RenameBranch(ctx, repo, from, to)
		if err2 != nil {
			return err2
		}

		if isDefault {
			err2 = gitrepo.SetDefaultBranch(ctx, repo, to)
			if err2 != nil {
				return err2
			}
		}

		return nil
	}); err != nil {
		return "", err
	}

	notify_service.DeleteRef(ctx, doer, repo, git.RefNameFromBranch(from))
	notify_service.CreateRef(ctx, doer, repo, git.RefNameFromBranch(to), fromBranch.CommitID)

	return "", nil
}

// UpdateBranch moves a branch reference to the provided commit. permission check should be done before calling this function.
func UpdateBranch(ctx context.Context, repo *repo_model.Repository, gitRepo *git.Repository, doer *user_model.User, branchName, newCommitID, expectedOldCommitID string, force bool) error {
	branch, err := git_model.GetBranch(ctx, repo.ID, branchName)
	if err != nil {
		return err
	}
	if branch.IsDeleted {
		return git_model.ErrBranchNotExist{
			BranchName: branchName,
		}
	}

	if expectedOldCommitID != "" {
		expectedID, err := gitRepo.ConvertToGitID(expectedOldCommitID)
		if err != nil {
			return fmt.Errorf("ConvertToGitID(old): %w", err)
		}
		if expectedID.String() != branch.CommitID {
			return util.NewInvalidArgumentErrorf("branch commit does not match [expected: %s, given: %s]", expectedID.String(), branch.CommitID)
		}
	}

	newID, err := gitRepo.ConvertToGitID(newCommitID)
	if err != nil {
		return fmt.Errorf("ConvertToGitID(new): %w", err)
	}
	newCommit, err := gitRepo.GetCommit(newID.String())
	if err != nil {
		return err
	}

	if newCommit.ID.String() == branch.CommitID {
		return nil
	}

	isForcePush, err := newCommit.IsForcePush(branch.CommitID)
	if err != nil {
		return err
	}
	if isForcePush && !force {
		return util.NewInvalidArgumentErrorf("Force push %s need a confirm force parameter", branchName)
	}

	pushOpts := git.PushOptions{
		Branch: fmt.Sprintf("%s:%s%s", newCommit.ID.String(), git.BranchPrefix, branchName),
		Env:    repo_module.PushingEnvironment(doer, repo),
		Force:  isForcePush || force,
	}

	if expectedOldCommitID != "" {
		pushOpts.ForceWithLease = fmt.Sprintf("%s:%s", git.BranchPrefix+branchName, branch.CommitID)
	}

	// branch protection will be checked in the pre received hook, so that we don't need any check here
	return gitrepo.Push(ctx, repo, repo, pushOpts)
}

var ErrBranchIsDefault = util.ErrorWrap(util.ErrPermissionDenied, "branch is default or pull request target")

func CanDeleteBranch(ctx context.Context, repo *repo_model.Repository, branchName string, doer *user_model.User) error {
	unitPRConfig := repo.MustGetUnit(ctx, unit.TypePullRequests).PullRequestsConfig()
	if branchName == repo.DefaultBranch || branchName == unitPRConfig.DefaultTargetBranch {
		return ErrBranchIsDefault
	}

	perm, err := access_model.GetDoerRepoPermission(ctx, repo, doer)
	if err != nil {
		return err
	}
	if !perm.CanWrite(unit.TypeCode) {
		return util.NewPermissionDeniedErrorf("permission denied to access repo %d unit %s", repo.ID, unit.TypeCode.LogString())
	}

	isProtected, err := git_model.IsBranchProtected(ctx, repo.ID, branchName)
	if err != nil {
		return err
	}
	if isProtected {
		return git_model.ErrBranchIsProtected
	}
	return nil
}

func deleteBranchInternal(ctx context.Context, doer *user_model.User, repo *repo_model.Repository, branchName string, branchCommit *git.Commit) (branchExisted bool, err error) {
	activeInDB, err := git_model.IsBranchExist(ctx, repo.ID, branchName)
	if err != nil {
		return false, fmt.Errorf("IsBranchExist: %w", err)
	}

	// process the branch in db
	if activeInDB {
		if err := git_model.MarkBranchAsDeleted(ctx, repo.ID, branchName, doer.ID); err != nil {
			return false, err
		}
	}

	// process the branch in git
	if branchCommit != nil {
		err := gitrepo.DeleteBranch(ctx, repo, branchName, true)
		if err != nil {
			return false, fmt.Errorf("DeleteBranch: %w", err)
		}
		// since the branch existed in git, return branchExisted=true
		branchExisted = true
	} else {
		// the branch didn't exist in git, return activeInDB to indicate whether the branch was active in DB,
		// for consistency with that the user had seen on the web ui or in the branch list API response.
		branchExisted = activeInDB
	}

	return branchExisted, nil
}

// DeleteBranch delete branch
func DeleteBranch(ctx context.Context, doer *user_model.User, repo *repo_model.Repository, gitRepo *git.Repository, branchName string) error {
	err := repo.MustNotBeArchived()
	if err != nil {
		return err
	}

	if err := CanDeleteBranch(ctx, repo, branchName, doer); err != nil {
		return err
	}

	branchCommit, err := gitRepo.GetBranchCommit(branchName)
	// branchCommit can be nil if the branch doesn't exist in git
	if err != nil && !errors.Is(err, util.ErrNotExist) {
		return err
	}

	branchExisted, err := db.WithTx2(ctx, func(ctx context.Context) (bool, error) {
		return deleteBranchInternal(ctx, doer, repo, branchName, branchCommit)
	})
	if err != nil {
		return err
	}

	if !branchExisted {
		return git.ErrBranchNotExist{Name: branchName}
	}

	// Don't return error below this since the deletion has succeeded
	if branchCommit != nil {
		deleteBranchSuccessPostProcess(doer, repo, branchName, branchCommit)
	}
	return nil
}

func deleteBranchSuccessPostProcess(doer *user_model.User, repo *repo_model.Repository, branchName string, branchCommit *git.Commit) {
	objectFormat := git.ObjectFormatFromName(repo.ObjectFormatName)
	if err := PushUpdates(
		&repo_module.PushUpdateOptions{
			RefFullName:  git.RefNameFromBranch(branchName),
			OldCommitID:  branchCommit.ID.String(),
			NewCommitID:  objectFormat.EmptyObjectID().String(),
			PusherID:     doer.ID,
			PusherName:   doer.Name,
			RepoUserName: repo.OwnerName,
			RepoName:     repo.Name,
		}); err != nil {
		log.Error("PushUpdateOptions: %v", err)
	}
}

type BranchSyncOptions struct {
	RepoID int64
}

// branchSyncQueue represents a queue to handle branch sync jobs.
var branchSyncQueue *queue.WorkerPoolQueue[*BranchSyncOptions]

func handlerBranchSync(items ...*BranchSyncOptions) []*BranchSyncOptions {
	for _, opts := range items {
		_, err := repo_module.SyncRepoBranches(graceful.GetManager().ShutdownContext(), opts.RepoID, 0)
		if err != nil {
			log.Error("syncRepoBranches [%d] failed: %v", opts.RepoID, err)
		}
	}
	return nil
}

func initBranchSyncQueue(ctx context.Context) error {
	branchSyncQueue = queue.CreateUniqueQueue(ctx, "branch_sync", handlerBranchSync)
	if branchSyncQueue == nil {
		return errors.New("unable to create branch_sync queue")
	}
	go graceful.GetManager().RunWithCancel(branchSyncQueue)

	return nil
}

// BranchDivergingInfo contains the information about the divergence of a head branch to the base branch.
type BranchDivergingInfo struct {
	// whether the base branch contains new commits which are not in the head branch
	BaseHasNewCommits bool

	// behind/after are number of commits that the head branch is behind/after the base branch, it's 0 if it's unable to calculate.
	// there could be a case that BaseHasNewCommits=true while the behind/after are both 0 (unable to calculate).
	HeadCommitsBehind int
	HeadCommitsAhead  int
}

// GetBranchDivergingInfo returns the information about the divergence of a patch branch to the base branch.
func GetBranchDivergingInfo(ctx reqctx.RequestContext, baseRepo *repo_model.Repository, baseBranch string, headRepo *repo_model.Repository, headBranch string) (*BranchDivergingInfo, error) {
	headGitBranch, err := git_model.GetBranch(ctx, headRepo.ID, headBranch)
	if err != nil {
		return nil, err
	}
	if headGitBranch.IsDeleted {
		return nil, git_model.ErrBranchNotExist{
			BranchName: headBranch,
		}
	}
	baseGitBranch, err := git_model.GetBranch(ctx, baseRepo.ID, baseBranch)
	if err != nil {
		return nil, err
	}
	if baseGitBranch.IsDeleted {
		return nil, git_model.ErrBranchNotExist{
			BranchName: baseBranch,
		}
	}

	info := &BranchDivergingInfo{}
	if headGitBranch.CommitID == baseGitBranch.CommitID {
		return info, nil
	}

	// if the fork repo has new commits, this call will fail because they are not in the base repo
	// exit status 128 - fatal: Invalid symmetric difference expression aaaaaaaaaaaa...bbbbbbbbbbbb
	// so at the moment, we first check the update time, then check whether the fork branch has base's head
	diff, err := gitrepo.GetDivergingCommits(ctx, baseRepo, baseGitBranch.CommitID, headGitBranch.CommitID)
	if err != nil {
		info.BaseHasNewCommits = baseGitBranch.UpdatedUnix > headGitBranch.UpdatedUnix
		if headRepo.IsFork && info.BaseHasNewCommits {
			return info, nil
		}
		// if the base's update time is before the fork, check whether the base's head is in the fork
		headGitRepo, err := gitrepo.RepositoryFromRequestContextOrOpen(ctx, headRepo)
		if err != nil {
			return nil, err
		}
		headCommit, err := headGitRepo.GetCommit(headGitBranch.CommitID)
		if err != nil {
			return nil, err
		}
		baseCommitID, err := git.NewIDFromString(baseGitBranch.CommitID)
		if err != nil {
			return nil, err
		}
		hasPreviousCommit, _ := headCommit.HasPreviousCommit(baseCommitID)
		info.BaseHasNewCommits = !hasPreviousCommit
		return info, nil
	}

	info.HeadCommitsBehind, info.HeadCommitsAhead = diff.Behind, diff.Ahead
	info.BaseHasNewCommits = info.HeadCommitsBehind > 0
	return info, nil
}

func DeleteBranchAfterMerge(ctx context.Context, doer *user_model.User, prID int64, outFullBranchName *string) error {
	pr, err := issues_model.GetPullRequestByID(ctx, prID)
	if err != nil {
		return err
	}
	if err = pr.LoadIssue(ctx); err != nil {
		return err
	}
	if err = pr.LoadBaseRepo(ctx); err != nil {
		return err
	}
	if err := pr.LoadHeadRepo(ctx); err != nil {
		return err
	}
	if pr.HeadRepo == nil {
		// Forked repository has already been deleted
		return util.ErrorWrapTranslatable(util.ErrNotExist, "repo.branch.deletion_failed", "(deleted-repo):"+pr.HeadBranch)
	}
	if err = pr.HeadRepo.LoadOwner(ctx); err != nil {
		return err
	}

	fullBranchName := pr.HeadRepo.FullName() + ":" + pr.HeadBranch
	if outFullBranchName != nil {
		*outFullBranchName = fullBranchName
	}

	errFailedToDelete := func(err error) error {
		return util.ErrorWrapTranslatable(err, "repo.branch.deletion_failed", fullBranchName)
	}

	// Don't clean up unmerged and unclosed PRs and agit PRs
	if !pr.HasMerged && !pr.Issue.IsClosed && pr.Flow != issues_model.PullRequestFlowGithub {
		return errFailedToDelete(util.ErrUnprocessableContent)
	}

	// Don't clean up when there are other PR's that use this branch as head branch.
	exist, err := issues_model.HasUnmergedPullRequestsByHeadInfo(ctx, pr.HeadRepoID, pr.HeadBranch)
	if err != nil {
		return err
	}
	if exist {
		return errFailedToDelete(util.ErrUnprocessableContent)
	}

	if err := CanDeleteBranch(ctx, pr.HeadRepo, pr.HeadBranch, doer); err != nil {
		if errors.Is(err, util.ErrPermissionDenied) {
			return errFailedToDelete(err)
		}
		return err
	}

	gitBaseRepo, gitBaseCloser, err := gitrepo.RepositoryFromContextOrOpen(ctx, pr.BaseRepo)
	if err != nil {
		return err
	}
	defer gitBaseCloser.Close()

	gitHeadRepo, gitHeadCloser, err := gitrepo.RepositoryFromContextOrOpen(ctx, pr.HeadRepo)
	if err != nil {
		return err
	}
	defer gitHeadCloser.Close()

	// Check if branch has no new commits
	headCommitID, err := gitBaseRepo.GetRefCommitID(pr.GetGitHeadRefName())
	if err != nil {
		log.Error("GetRefCommitID: %v", err)
		return errFailedToDelete(err)
	}
	branchCommitID, err := gitHeadRepo.GetBranchCommitID(pr.HeadBranch)
	if err != nil {
		log.Error("GetBranchCommitID: %v", err)
		return errFailedToDelete(err)
	}
	if headCommitID != branchCommitID {
		return util.ErrorWrapTranslatable(util.ErrUnprocessableContent, "repo.branch.delete_branch_has_new_commits", fullBranchName)
	}

	err = DeleteBranch(ctx, doer, pr.HeadRepo, gitHeadRepo, pr.HeadBranch)
	if errors.Is(err, util.ErrPermissionDenied) || errors.Is(err, util.ErrNotExist) {
		return errFailedToDelete(err)
	}
	if err != nil {
		return err
	}

	// intentionally ignore the following error, since the branch has already been deleted successfully
	if err := issues_model.AddDeletePRBranchComment(ctx, doer, pr.BaseRepo, pr.Issue.ID, pr.HeadBranch); err != nil {
		log.Error("AddDeletePRBranchComment: %v", err)
	}
	return nil
}
