// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gitea.dev/models/db"
	git_model "gitea.dev/models/git"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unit"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/git"
	"gitea.dev/modules/gitrepo"
	"gitea.dev/modules/log"
	repo_module "gitea.dev/modules/repository"
	"gitea.dev/modules/structs"
	"gitea.dev/modules/util"
	notify_service "gitea.dev/services/notify"

	"xorm.io/builder"
)

// ErrForkAlreadyExist represents a "ForkAlreadyExist" kind of error.
type ErrForkAlreadyExist struct {
	Uname    string
	RepoName string
	ForkName string
}

// IsErrForkAlreadyExist checks if an error is an ErrForkAlreadyExist.
func IsErrForkAlreadyExist(err error) bool {
	_, ok := err.(ErrForkAlreadyExist)
	return ok
}

func (err ErrForkAlreadyExist) Error() string {
	return fmt.Sprintf("repository is already forked by user [uname: %s, repo path: %s, fork path: %s]", err.Uname, err.RepoName, err.ForkName)
}

func (err ErrForkAlreadyExist) Unwrap() error {
	return util.ErrAlreadyExist
}

// ForkRepoOptions contains the fork repository options
type ForkRepoOptions struct {
	BaseRepo     *repo_model.Repository
	Name         string
	Description  string
	SingleBranch string
}

// ForkRepository forks a repository
func ForkRepository(ctx context.Context, doer, owner *user_model.User, opts ForkRepoOptions) (*repo_model.Repository, error) {
	if err := opts.BaseRepo.LoadOwner(ctx); err != nil {
		return nil, err
	}

	// Fork is prohibited, if user has reached maximum limit of repositories
	if !doer.CanForkRepoIn(owner) {
		return nil, repo_model.ErrReachLimitOfRepo{
			Limit: owner.MaxRepoCreation,
		}
	}

	forkedRepo, err := repo_model.GetUserFork(ctx, opts.BaseRepo.ID, owner.ID)
	if err != nil {
		return nil, err
	}
	if forkedRepo != nil {
		return nil, ErrForkAlreadyExist{
			Uname:    owner.Name,
			RepoName: opts.BaseRepo.FullName(),
			ForkName: forkedRepo.FullName(),
		}
	}

	defaultBranch := opts.BaseRepo.DefaultBranch
	if opts.SingleBranch != "" {
		defaultBranch = opts.SingleBranch
	}
	repo := &repo_model.Repository{
		OwnerID:          owner.ID,
		Owner:            owner,
		OwnerName:        owner.Name,
		Name:             opts.Name,
		LowerName:        strings.ToLower(opts.Name),
		Description:      opts.Description,
		DefaultBranch:    defaultBranch,
		IsPrivate:        opts.BaseRepo.IsPrivate || opts.BaseRepo.Owner.Visibility == structs.VisibleTypePrivate,
		IsEmpty:          opts.BaseRepo.IsEmpty,
		IsFork:           true,
		ForkID:           opts.BaseRepo.ID,
		ObjectFormatName: opts.BaseRepo.ObjectFormatName,
		Status:           repo_model.RepositoryBeingMigrated,
	}

	// 1 - Create the repository in the database
	err = db.WithTx(ctx, func(ctx context.Context) error {
		if err = createRepositoryInDB(ctx, doer, owner, repo, true); err != nil {
			return err
		}
		if err = repo_model.IncrementRepoForkNum(ctx, opts.BaseRepo.ID); err != nil {
			return err
		}

		// copy lfs files failure should not be ignored
		return git_model.CopyLFS(ctx, repo, opts.BaseRepo)
	})
	if err != nil {
		return nil, err
	}

	// last - clean up if something goes wrong
	// WARNING: Don't override all later err with local variables
	defer func() {
		if err != nil {
			// we can not use `ctx` because it may be canceled or timed out
			cleanupRepository(repo)
		}
	}()

	// 2 - check whether the repository with the same storage exists
	var isExist bool
	isExist, err = gitrepo.IsRepositoryExist(ctx, repo)
	if err != nil {
		log.Error("Unable to check if %s exists. Error: %v", repo.FullName(), err)
		return nil, err
	}
	if isExist {
		log.Error("Files already exist in %s and we are not going to adopt or delete.", repo.FullName())
		// Don't return directly, we need err in defer to cleanupRepository
		err = repo_model.ErrRepoFilesAlreadyExist{
			Uname: repo.OwnerName,
			Name:  repo.Name,
		}
		return nil, err
	}

	// 3 - Clone the repository
	cloneOpts := git.CloneRepoOptions{
		Bare:    true,
		Timeout: 10 * time.Minute,
	}
	if opts.SingleBranch != "" {
		cloneOpts.SingleBranch = true
		cloneOpts.Branch = opts.SingleBranch
	}
	if err = gitrepo.Clone(ctx, opts.BaseRepo, repo, cloneOpts); err != nil {
		log.Error("Fork Repository (git clone) Failed for %v (from %v):\nError: %v", repo, opts.BaseRepo, err)
		return nil, fmt.Errorf("git clone: %w", err)
	}

	// 4 - Update the git repository
	if err = updateGitRepoAfterCreate(ctx, repo); err != nil {
		return nil, fmt.Errorf("updateGitRepoAfterCreate: %w", err)
	}

	// 5 - Create hooks
	if err = gitrepo.CreateDelegateHooks(ctx, repo); err != nil {
		return nil, fmt.Errorf("createDelegateHooks: %w", err)
	}

	// 6 - Sync the repository branches and tags
	var gitRepo *git.Repository
	gitRepo, err = gitrepo.OpenRepository(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("OpenRepository: %w", err)
	}
	defer gitRepo.Close()

	if _, _, err = repo_module.SyncRepoBranchesWithRepo(ctx, repo, gitRepo, doer.ID); err != nil {
		return nil, fmt.Errorf("SyncRepoBranchesWithRepo: %w", err)
	}
	if _, err = repo_module.SyncReleasesWithTags(ctx, repo, gitRepo); err != nil {
		return nil, fmt.Errorf("Sync releases from git tags failed: %v", err)
	}

	// 7 - Update the repository
	// even if below operations failed, it could be ignored. And they will be retried
	if err = repo_module.UpdateRepoSize(ctx, repo); err != nil {
		log.Error("Failed to update size for repository: %v", err)
		err = nil
	}

	// 8 - update repository status to be ready
	repo.Status = repo_model.RepositoryReady
	if err = repo_model.UpdateRepositoryColsWithAutoTime(ctx, repo, "status"); err != nil {
		return nil, fmt.Errorf("UpdateRepositoryCols: %w", err)
	}

	notify_service.ForkRepository(ctx, doer, opts.BaseRepo, repo)

	return repo, nil
}

type findForksOptions struct {
	db.ListOptions
	RepoID int64
	Doer   *user_model.User
}

func (opts findForksOptions) ToConds() builder.Cond {
	cond := builder.Eq{"fork_id": opts.RepoID}
	if opts.Doer != nil && opts.Doer.IsAdmin {
		return cond
	}
	return cond.And(repo_model.AccessibleRepositoryCondition(opts.Doer, unit.TypeInvalid))
}

// FindForks returns all the forks of the repository
func FindForks(ctx context.Context, repo *repo_model.Repository, doer *user_model.User, listOptions db.ListOptions) ([]*repo_model.Repository, int64, error) {
	return db.FindAndCount[repo_model.Repository](ctx, findForksOptions{
		ListOptions: listOptions,
		RepoID:      repo.ID,
		Doer:        doer,
	})
}
