// Copyright 2014 The Gogs Authors. All rights reserved.
// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package context

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"

	git_model "gitea.dev/models/git"
	issues_model "gitea.dev/models/issues"
	access_model "gitea.dev/models/perm/access"
	repo_model "gitea.dev/models/repo"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/git"
	"gitea.dev/modules/log"
	"gitea.dev/modules/setting"

	"github.com/editorconfig/editorconfig-core-go/v2"
)

// Repository contains information to operate a repository
type Repository struct {
	Permission access_model.Permission

	Repository *repo_model.Repository
	Owner      *user_model.User

	RepoLink string
	GitRepo  *git.Repository

	// RefFullName is the full ref name that the user is viewing
	RefFullName git.RefName
	BranchName  string // it is the RefFullName's short name if its type is "branch"
	TreePath    string

	// Commit it is always set to the commit for the branch or tag, or just the commit that the user is viewing
	Commit       *git.Commit
	CommitID     string
	CommitsCount int64
}

// CanWriteToBranch checks if the branch is writable by the user
func (r *Repository) CanWriteToBranch(ctx context.Context, user *user_model.User, branch string) bool {
	return issues_model.CanMaintainerWriteToBranch(ctx, r.Permission, branch, user)
}

func (r *Repository) GetObjectFormat() git.ObjectFormat {
	return git.ObjectFormatFromName(r.Repository.ObjectFormatName)
}

// GetEditorconfig returns the .editorconfig definition if found in the
// HEAD of the default repo branch.
func (r *Repository) GetEditorconfig(optCommit ...*git.Commit) (cfg *editorconfig.Editorconfig, warning, err error) {
	if r.GitRepo == nil {
		return nil, nil, nil
	}

	var commit *git.Commit

	if len(optCommit) != 0 {
		commit = optCommit[0]
	} else {
		commit, err = r.GitRepo.GetBranchCommit(r.Repository.DefaultBranch)
		if err != nil {
			return nil, nil, err
		}
	}
	treeEntry, err := commit.GetTreeEntryByPath(".editorconfig")
	if err != nil {
		return nil, nil, err
	}
	if treeEntry.Blob().Size() >= setting.UI.MaxDisplayFileSize {
		return nil, nil, git.ErrNotExist{ID: "", RelPath: ".editorconfig"}
	}
	reader, err := treeEntry.Blob().DataAsync()
	if err != nil {
		return nil, nil, err
	}
	defer reader.Close()
	return editorconfig.ParseGraceful(reader)
}

// RedirectToRepo redirect to a differently-named repository
func RedirectToRepo(ctx *Base, redirectRepoID int64) {
	ownerName := ctx.PathParam("username")
	previousRepoName := ctx.PathParam("reponame")

	repo, err := repo_model.GetRepositoryByID(ctx, redirectRepoID)
	if err != nil {
		log.Error("GetRepositoryByID: %v", err)
		ctx.HTTPError(http.StatusInternalServerError, "GetRepositoryByID")
		return
	}

	redirectPath := strings.Replace(
		ctx.Req.URL.EscapedPath(),
		url.PathEscape(ownerName)+"/"+url.PathEscape(previousRepoName),
		url.PathEscape(repo.OwnerName)+"/"+url.PathEscape(repo.Name),
		1,
	)
	if ctx.Req.URL.RawQuery != "" {
		redirectPath += "?" + ctx.Req.URL.RawQuery
	}
	// Git client needs a 301 redirect by default to follow the new location
	// It's not documentated in git documentation, but it's the behavior of git client
	ctx.Redirect(path.Join(setting.AppSubURL, redirectPath), http.StatusMovedPermanently)
}

func getRefNameFromPath(repo *Repository, path string, isExist func(string) bool) string {
	refName := ""
	parts := strings.Split(path, "/")
	for i, part := range parts {
		refName = strings.TrimPrefix(refName+"/"+part, "/")
		if isExist(refName) {
			repo.TreePath = strings.Join(parts[i+1:], "/")
			return refName
		}
	}
	return ""
}

func getRefNameLegacy(ctx *Base, repo *Repository, reqPath, extraRef string) (refName string, refType git.RefType, fallbackDefaultBranch bool) {
	reqRefPath := path.Join(extraRef, reqPath)
	reqRefPathParts := strings.Split(reqRefPath, "/")
	if refName := getRefName(ctx, repo, reqRefPath, git.RefTypeBranch); refName != "" {
		return refName, git.RefTypeBranch, false
	}
	if refName := getRefName(ctx, repo, reqRefPath, git.RefTypeTag); refName != "" {
		return refName, git.RefTypeTag, false
	}
	if git.IsStringLikelyCommitID(git.ObjectFormatFromName(repo.Repository.ObjectFormatName), reqRefPathParts[0]) {
		// FIXME: this logic is different from other types. Ideally, it should also try to GetCommit to check if it exists
		repo.TreePath = strings.Join(reqRefPathParts[1:], "/")
		return reqRefPathParts[0], git.RefTypeCommit, false
	}
	// FIXME: the old code falls back to default branch if "ref" doesn't exist, there could be an edge case:
	// "README?ref=no-such" would read the README file from the default branch, but the user might expect a 404
	repo.TreePath = reqPath
	return repo.Repository.DefaultBranch, git.RefTypeBranch, true
}

func getRefName(ctx *Base, repo *Repository, path string, refType git.RefType) string {
	switch refType {
	case git.RefTypeBranch:
		ref := getRefNameFromPath(repo, path, repo.GitRepo.IsBranchExist)
		if len(ref) == 0 {
			// check if ref is HEAD
			parts := strings.Split(path, "/")
			if parts[0] == headRefName {
				repo.TreePath = strings.Join(parts[1:], "/")
				return repo.Repository.DefaultBranch
			}

			// maybe it's a renamed branch
			return getRefNameFromPath(repo, path, func(s string) bool {
				b, exist, err := git_model.FindRenamedBranch(ctx, repo.Repository.ID, s)
				if err != nil {
					log.Error("FindRenamedBranch: %v", err)
					return false
				}

				if !exist {
					return false
				}

				ctx.Data["IsRenamedBranch"] = true
				ctx.Data["RenamedBranchName"] = b.To

				return true
			})
		}

		return ref
	case git.RefTypeTag:
		return getRefNameFromPath(repo, path, repo.GitRepo.IsTagExist)
	case git.RefTypeCommit:
		parts := strings.Split(path, "/")
		if git.IsStringLikelyCommitID(repo.GetObjectFormat(), parts[0], 7) {
			// FIXME: this logic is different from other types. Ideally, it should also try to GetCommit to check if it exists
			repo.TreePath = strings.Join(parts[1:], "/")
			return parts[0]
		}

		if parts[0] == headRefName {
			// HEAD ref points to last default branch commit
			commit, err := repo.GitRepo.GetBranchCommit(repo.Repository.DefaultBranch)
			if err != nil {
				return ""
			}
			repo.TreePath = strings.Join(parts[1:], "/")
			return commit.ID.String()
		}
	default:
		panic(fmt.Sprintf("Unrecognized ref type: %v", refType))
	}
	return ""
}

const headRefName = "HEAD"
