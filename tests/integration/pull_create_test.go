// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"testing"

	auth_model "gitea.dev/models/auth"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unittest"
	api "gitea.dev/modules/structs"
	"gitea.dev/tests"

	"github.com/stretchr/testify/assert"
)

func TestPullCreate(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, u *url.URL) {
		session := loginUser(t, "user1")
		testRepoFork(t, session, "user2", "repo1", "user1", "repo1", "")
		testEditFile(t, session, "user1", "repo1", "master", "README.md", "Hello, World (Edited)\n")
		repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{OwnerName: "user2", Name: "repo1"})
		assert.Equal(t, 3, repo1.NumPulls)
		assert.Equal(t, 3, repo1.NumOpenPulls)
		resp := testPullCreate(t, session, "user1", "repo1", false, "master", "master", "This is a pull title")

		repo1 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{OwnerName: "user2", Name: "repo1"})
		assert.Equal(t, 4, repo1.NumPulls)
		assert.Equal(t, 4, repo1.NumOpenPulls)

		// check the pull URL
		url := testPullURL(t, resp)
		assert.Regexp(t, "^/user2/repo1/pulls/[0-9]*$", url)

		// test create the pull request again and it should fail now
		testPullCreateFailure(t, session, "user2", "repo1", "master", "user1", "repo1", "master", "This is a pull title", http.StatusConflict)

		// check .diff can be accessed and matches performed change
		req := NewRequest(t, "GET", "/api/v1/repos"+url+".diff")
		resp = session.MakeRequest(t, req, http.StatusOK)
		assert.Regexp(t, `\+Hello, World \(Edited\)`, resp.Body)
		assert.Regexp(t, "^diff", resp.Body)
		assert.NotRegexp(t, "diff.*diff", resp.Body) // not two diffs, just one

		// check .patch can be accessed and matches performed change
		req = NewRequest(t, "GET", "/api/v1/repos"+url+".patch")
		resp = session.MakeRequest(t, req, http.StatusOK)
		assert.Regexp(t, `\+Hello, World \(Edited\)`, resp.Body)
		assert.Regexp(t, "diff", resp.Body)
		assert.Regexp(t, `Subject: \[PATCH\] Update README.md`, resp.Body)
		assert.NotRegexp(t, "diff.*diff", resp.Body) // not two diffs, just one
	})
}

func TestPullBranchDelete(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, u *url.URL) {
		defer tests.PrepareTestEnv(t)()

		session := loginUser(t, "user1")
		testRepoFork(t, session, "user2", "repo1", "user1", "repo1", "")
		testCreateBranch(t, session, "user1", "repo1", "branch/master", "master1", http.StatusCreated)
		testEditFile(t, session, "user1", "repo1", "master1", "README.md", "Hello, World (Edited)\n")
		resp := testPullCreate(t, session, "user1", "repo1", false, "master", "master1", "This is a pull title")

		// check the pull URL
		url := "/api/v1/repos" + testPullURL(t, resp)
		assert.Regexp(t, "^/api/v1/repos/user2/repo1/pulls/[0-9]*$", url)
		req := NewRequest(t, "GET", url)
		session.MakeRequest(t, req, http.StatusOK)

		// delete head branch and confirm the pull is still readable
		testDeleteBranch(t, session, "user1", "repo1", "master1")
		req = NewRequest(t, "GET", url)
		session.MakeRequest(t, req, http.StatusOK)

		// delete head repository and confirm the pull is still readable
		testDeleteRepository(t, session, "user1", "repo1")
		req = NewRequest(t, "GET", url)
		session.MakeRequest(t, req, http.StatusOK)
	})
}

/*
Setup:
The base repository is: user2/repo1
Fork repository to: user1/repo1
Push extra commit to: user2/repo1, which changes README.md
Create a PR on user1/repo1

Test checks:
Check if pull request can be created from base to the fork repository.
*/
func TestPullCreatePrFromBaseToFork(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, u *url.URL) {
		sessionFork := loginUser(t, "user1")
		testRepoFork(t, sessionFork, "user2", "repo1", "user1", "repo1", "")

		// Edit base repository
		sessionBase := loginUser(t, "user2")
		testEditFile(t, sessionBase, "user2", "repo1", "master", "README.md", "Hello, World (Edited)\n")

		// Create a PR
		resp := testPullCreateDirectly(t, sessionFork, createPullRequestOptions{
			BaseRepoOwner: "user1",
			BaseRepoName:  "repo1",
			BaseBranch:    "master",
			HeadRepoOwner: "user2",
			HeadRepoName:  "repo1",
			HeadBranch:    "master",
			Title:         "This is a pull title",
		})
		// check the pull URL
		url := testPullURL(t, resp)
		assert.Regexp(t, "^/user1/repo1/pulls/[0-9]*$", url)
	})
}

func TestCreatePullRequestFromNestedOrgForks(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, _ *url.URL) {
		session := loginUser(t, "user1")
		token := getTokenForLoggedInUser(t, session, auth_model.AccessTokenScopeWriteRepository, auth_model.AccessTokenScopeWriteOrganization)

		const (
			baseOrg     = "test-fork-org1"
			midForkOrg  = "test-fork-org2"
			leafForkOrg = "test-fork-org3"
			repoName    = "test-fork-repo"
			patchBranch = "teabot-patch-1"
		)

		createOrg := func(name string) {
			req := NewRequestWithJSON(t, "POST", "/api/v1/orgs", &api.CreateOrgOption{
				UserName:   name,
				Visibility: "public",
			}).AddTokenAuth(token)
			MakeRequest(t, req, http.StatusCreated)
		}

		createOrg(baseOrg)
		createOrg(midForkOrg)
		createOrg(leafForkOrg)

		req := NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/orgs/%s/repos", baseOrg), &api.CreateRepoOption{
			Name:          repoName,
			AutoInit:      true,
			DefaultBranch: "main",
			Private:       false,
			Readme:        "Default",
		}).AddTokenAuth(token)
		resp := MakeRequest(t, req, http.StatusCreated)
		baseRepo := DecodeJSON(t, resp, &api.Repository{})
		assert.Equal(t, "main", baseRepo.DefaultBranch)

		forkIntoOrg := func(srcOrg, dstOrg string) api.Repository {
			req := NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/forks", srcOrg, repoName), &api.CreateForkOption{
				Organization: new(dstOrg),
			}).AddTokenAuth(token)
			resp := MakeRequest(t, req, http.StatusAccepted)
			forkRepo := DecodeJSON(t, resp, &api.Repository{})
			assert.NotNil(t, forkRepo.Owner)
			if forkRepo.Owner != nil {
				assert.Equal(t, dstOrg, forkRepo.Owner.UserName)
			}
			return *forkRepo
		}

		forkIntoOrg(baseOrg, midForkOrg)
		forkIntoOrg(midForkOrg, leafForkOrg)

		req = NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/contents/%s", leafForkOrg, repoName, "patch-from-org3.txt"), &api.CreateFileOptions{
			FileOptions: api.FileOptions{
				BranchName:    "main",
				NewBranchName: patchBranch,
				Message:       "create patch from org3",
			},
			ContentBase64: base64.StdEncoding.EncodeToString([]byte("patch content")),
		}).AddTokenAuth(token)
		MakeRequest(t, req, http.StatusCreated)

		prPayload := map[string]string{
			"head":  fmt.Sprintf("%s:%s", leafForkOrg, patchBranch),
			"base":  "main",
			"title": "test creating pull from test-fork-org3 to test-fork-org1",
		}
		req = NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/pulls", baseOrg, repoName), prPayload).AddTokenAuth(token)
		resp = MakeRequest(t, req, http.StatusCreated)
		pr := DecodeJSON(t, resp, &api.PullRequest{})
		assert.Equal(t, prPayload["title"], pr.Title)
		if assert.NotNil(t, pr.Head) {
			assert.Equal(t, patchBranch, pr.Head.Ref)
			if assert.NotNil(t, pr.Head.Repository) {
				assert.Equal(t, fmt.Sprintf("%s/%s", leafForkOrg, repoName), pr.Head.Repository.FullName)
			}
		}
		if assert.NotNil(t, pr.Base) {
			assert.Equal(t, "main", pr.Base.Ref)
			if assert.NotNil(t, pr.Base.Repository) {
				assert.Equal(t, fmt.Sprintf("%s/%s", baseOrg, repoName), pr.Base.Repository.FullName)
			}
		}
	})
}

func TestPullCreateParallel(t *testing.T) {
	onGiteaRun(t, func(t *testing.T, u *url.URL) {
		sessionFork := loginUser(t, "user1")
		testRepoFork(t, sessionFork, "user2", "repo1", "user1", "repo1", "")

		repo1 := unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{OwnerName: "user2", Name: "repo1"})
		assert.Equal(t, 3, repo1.NumPulls)
		assert.Equal(t, 3, repo1.NumOpenPulls)

		var wg sync.WaitGroup
		for i := range 5 {
			wg.Go(func() {
				branchName := fmt.Sprintf("new-branch-%d", i)
				testEditFileToNewBranch(t, sessionFork, "user1", "repo1", "master", branchName, "README.md", fmt.Sprintf("Hello, World (Edited) %d\n", i))

				// Create a PR
				resp := testPullCreateDirectly(t, sessionFork, createPullRequestOptions{
					BaseRepoOwner: "user2",
					BaseRepoName:  "repo1",
					BaseBranch:    "master",
					HeadRepoOwner: "user1",
					HeadRepoName:  "repo1",
					HeadBranch:    branchName,
					Title:         fmt.Sprintf("This is a pull title %d", i),
				})
				// check the pull URL
				url := testPullURL(t, resp)
				assert.Regexp(t, "^/user2/repo1/pulls/[0-9]*$", url)
			})
		}
		wg.Wait()

		repo1 = unittest.AssertExistsAndLoadBean(t, &repo_model.Repository{OwnerName: "user2", Name: "repo1"})
		assert.Equal(t, 8, repo1.NumPulls)
		assert.Equal(t, 8, repo1.NumOpenPulls)
	})
}
