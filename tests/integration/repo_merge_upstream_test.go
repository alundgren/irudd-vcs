// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	auth_model "gitea.dev/models/auth"
	"gitea.dev/models/db"
	git_model "gitea.dev/models/git"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unittest"
	user_model "gitea.dev/models/user"
	api "gitea.dev/modules/structs"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoMergeUpstream(t *testing.T) {
	onGiteaRun(t, func(*testing.T, *url.URL) {
		forkUser := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: 4})

		baseRepo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{ID: 1})
		baseUser := unittest.AssertExistsAndLoadBean(t, &user_model.User{ID: baseRepo.OwnerID})

		session := loginUser(t, forkUser.Name)
		token := getTokenForLoggedInUser(t, session, auth_model.AccessTokenScopeWriteRepository)

		checkFileContent := func(branch, exp string) {
			req := NewRequest(t, "GET", fmt.Sprintf("/api/v1/repos/%s/test-repo-fork/raw/new-file.txt?ref=%s", forkUser.Name, branch)).AddTokenAuth(token)
			resp := MakeRequest(t, req, http.StatusOK)
			require.Equal(t, exp, resp.Body.String())
		}

		// create a fork
		req := NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/forks", baseUser.Name, baseRepo.Name), &api.CreateForkOption{
			Name: new("test-repo-fork"),
		}).AddTokenAuth(token)
		MakeRequest(t, req, http.StatusAccepted)
		forkRepo := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{OwnerID: forkUser.ID, Name: "test-repo-fork"})

		// create fork-branch
		testCreateBranch(t, session, forkUser.Name, "test-repo-fork", "branch/master", "fork-branch", http.StatusCreated)

		t.Run("HeadBeforeBase", func(t *testing.T) {
			// add a file in base repo
			sessionBaseUser := loginUser(t, baseUser.Name)
			require.NoError(t, createOrReplaceFileInBranch(baseUser, baseRepo, "new-file.txt", "master", "test-content-1"))

			// if the fork-branch name also exists in the base repo, then that branch is used instead of the default branch
			testCreateBranch(t, sessionBaseUser, baseUser.Name, baseRepo.Name, "branch/master", "fork-branch", http.StatusCreated)

			// sync the fork
			req = NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/test-repo-fork/merge-upstream", forkUser.Name), &api.MergeUpstreamRequest{
				Branch: "fork-branch",
			}).AddTokenAuth(token)
			MakeRequest(t, req, http.StatusOK)
			checkFileContent("fork-branch", "test-content-1")

			// delete the "fork-branch" from the base repo
			testDeleteBranch(t, sessionBaseUser, baseUser.Name, baseRepo.Name, "fork-branch")
		})

		t.Run("BaseChangeAfterHeadChange", func(t *testing.T) {
			// update the files: base first, head later, and check the prompt
			require.NoError(t, createOrReplaceFileInBranch(baseUser, baseRepo, "new-file.txt", "master", "test-content-2"))
			require.NoError(t, createOrReplaceFileInBranch(forkUser, forkRepo, "new-file-other.txt", "fork-branch", "test-content-other"))

			// make sure the base branch's update time is before the fork, to make it test the complete logic
			baseBranch := unittest.AssertExistsAndLoadBean(t, &git_model.Branch{RepoID: baseRepo.ID, Name: "master"})
			forkBranch := unittest.AssertExistsAndLoadBean(t, &git_model.Branch{RepoID: forkRepo.ID, Name: "fork-branch"})
			_, err := db.GetEngine(t.Context()).ID(forkBranch.ID).Update(&git_model.Branch{UpdatedUnix: baseBranch.UpdatedUnix + 1})
			require.NoError(t, err)

			// and do the merge-upstream by API
			req = NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/test-repo-fork/merge-upstream", forkUser.Name), &api.MergeUpstreamRequest{
				Branch: "fork-branch",
			}).AddTokenAuth(token)
			resp := MakeRequest(t, req, http.StatusOK)
			checkFileContent("fork-branch", "test-content-2")

			mergeResp := DecodeJSON(t, resp, &api.MergeUpstreamResponse{})
			assert.Equal(t, "merge", mergeResp.MergeStyle)
		})

		t.Run("FastForwardOnly", func(t *testing.T) {
			// Create a clean branch for fast-forward testing
			testCreateBranch(t, session, forkUser.Name, "test-repo-fork", "branch/master", "ff-test-branch", http.StatusCreated)

			// Add content to base repository that can be fast-forwarded
			require.NoError(t, createOrReplaceFileInBranch(baseUser, baseRepo, "ff-test.txt", "master", "ff-content-1"))

			// ff_only=true with fast-forward possible (should succeed)
			req = NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/test-repo-fork/merge-upstream", forkUser.Name), &api.MergeUpstreamRequest{
				Branch: "ff-test-branch",
				FfOnly: true,
			}).AddTokenAuth(token)
			resp := MakeRequest(t, req, http.StatusOK)

			mergeResp := DecodeJSON(t, resp, &api.MergeUpstreamResponse{})
			assert.Equal(t, "fast-forward", mergeResp.MergeStyle)

			// ff_only=true when fast-forward is not possible (should fail)
			require.NoError(t, createOrReplaceFileInBranch(baseUser, baseRepo, "another-file.txt", "master", "more-content"))

			req = NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/test-repo-fork/merge-upstream", forkUser.Name), &api.MergeUpstreamRequest{
				Branch: "fork-branch",
				FfOnly: true,
			}).AddTokenAuth(token)
			MakeRequest(t, req, http.StatusBadRequest)
		})

		t.Run("BasePrivateStillSyncs", func(t *testing.T) {
			// add a new commit to the base repo, then make the base repo private
			require.NoError(t, createOrReplaceFileInBranch(baseUser, baseRepo, "secret.txt", "master", "private-content"))
			baseRepo.IsPrivate = true
			_, err := db.GetEngine(t.Context()).ID(baseRepo.ID).Cols("is_private").Update(baseRepo)
			require.NoError(t, err)
			// the fork owner still reads the private base repo, every signed-in user owns every repository
			req = NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/test-repo-fork/merge-upstream", forkUser.Name), &api.MergeUpstreamRequest{
				Branch: "fork-branch",
			}).AddTokenAuth(token)
			MakeRequest(t, req, http.StatusOK)
		})
	})
}
