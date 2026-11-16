// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package integration

import (
	"context"
	"fmt"
	"io"
	mathRand "math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	auth_model "gitea.dev/models/auth"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/modules/git"
	"gitea.dev/modules/git/gitcmd"
	"gitea.dev/modules/lfs"
	"gitea.dev/modules/setting"
	api "gitea.dev/modules/structs"
	"gitea.dev/tests"

	"github.com/stretchr/testify/assert"
)

const (
	testFileSizeSmall = 10
	testFileSizeLarge = 10 * 1024 * 1024 // 10M
)

func TestGitGeneral(t *testing.T) {
	onGiteaRun(t, testGitGeneral)
}

func testGitGeneral(t *testing.T, u *url.URL) {
	username := "user2"
	baseAPITestContext := NewAPITestContext(t, username, "repo1", auth_model.AccessTokenScopeWriteRepository, auth_model.AccessTokenScopeWriteUser)

	u.Path = baseAPITestContext.GitPath()

	forkedUserCtx := NewAPITestContext(t, "user4", "repo1", auth_model.AccessTokenScopeWriteRepository, auth_model.AccessTokenScopeWriteUser)

	t.Run("HTTP", func(t *testing.T) {
		defer tests.PrintCurrentTest(t)()
		ensureAnonymousClone(t, u)
		httpContext := baseAPITestContext
		httpContext.Reponame = "repo-tmp-17"
		forkedUserCtx.Reponame = httpContext.Reponame

		dstPath := t.TempDir()

		t.Run("CreateRepoInDifferentUser", doAPICreateRepository(forkedUserCtx, false))

		t.Run("ForkFromDifferentUser", doAPIForkRepository(httpContext, forkedUserCtx.Username))

		u.Path = httpContext.GitPath()
		u.User = gitUserInfo(t, username)

		t.Run("Clone", doGitClone(dstPath, u))

		dstPath2 := t.TempDir()

		t.Run("Partial Clone", doPartialGitClone(dstPath2, u))

		pushedFilesStandard := standardCommitAndPushTest(t, dstPath, testFileSizeSmall, testFileSizeLarge)
		pushedFilesLFS := lfsCommitAndPushTest(t, dstPath, testFileSizeSmall, testFileSizeLarge)
		rawTest(t, &httpContext, pushedFilesStandard[0], pushedFilesStandard[1], pushedFilesLFS[0], pushedFilesLFS[1])
		mediaTest(t, &httpContext, pushedFilesStandard[0], pushedFilesStandard[1], pushedFilesLFS[0], pushedFilesLFS[1])

		t.Run("CreateProtectedBranch", doCreateProtectedBranch(&httpContext, dstPath))
		t.Run("BranchProtectMerge", doBranchProtectPRMerge(&httpContext, dstPath))
		t.Run("CreatePRAndSetManuallyMerged", doCreatePRAndSetManuallyMerged(httpContext, httpContext, dstPath, "master", "test-manually-merge"))
		t.Run("MergeFork", func(t *testing.T) {
			defer tests.PrintCurrentTest(t)()
			t.Run("CreatePRAndMerge", doMergeFork(httpContext, forkedUserCtx, "master", httpContext.Username+":master"))
			rawTest(t, &forkedUserCtx, pushedFilesStandard[0], pushedFilesStandard[1], pushedFilesLFS[0], pushedFilesLFS[1])
			mediaTest(t, &forkedUserCtx, pushedFilesStandard[0], pushedFilesStandard[1], pushedFilesLFS[0], pushedFilesLFS[1])
		})

		t.Run("PushCreate", doPushCreate(httpContext, u))
	})
}

// ensureAnonymousClone checks that cloning without a token is refused, even for a public repository
func ensureAnonymousClone(t *testing.T, u *url.URL) {
	t.Run("CloneAnonymous", doGitCloneFail(u))
}

func standardCommitAndPushTest(t *testing.T, dstPath string, sizes ...int) (pushedFiles []string) {
	t.Run("CommitAndPushStandard", func(t *testing.T) {
		defer tests.PrintCurrentTest(t)()
		pushedFiles = commitAndPushTest(t, dstPath, "data-file-", sizes...)
	})
	return pushedFiles
}

func lfsCommitAndPushTest(t *testing.T, dstPath string, sizes ...int) (pushedFiles []string) {
	t.Run("CommitAndPushLFS", func(t *testing.T) {
		defer tests.PrintCurrentTest(t)()
		prefix := "lfs-data-file-"
		err := gitcmd.NewCommand("lfs").AddArguments("install").WithDir(dstPath).Run(t.Context())
		assert.NoError(t, err)
		_, _, err = gitcmd.NewCommand("lfs").AddArguments("track").AddDynamicArguments(prefix + "*").
			WithDir(dstPath).RunStdString(t.Context())
		assert.NoError(t, err)
		err = gitAddChangesDeprecated(t.Context(), dstPath, false, ".gitattributes")
		assert.NoError(t, err)

		err = gitCommitChangesDeprecated(t.Context(), dstPath, gitCommitChangesOptions{
			Committer: &git.Signature{
				Email: "user2@example.com",
				Name:  "User Two",
				When:  time.Now(),
			},
			Author: &git.Signature{
				Email: "user2@example.com",
				Name:  "User Two",
				When:  time.Now(),
			},
			Message: fmt.Sprintf("Testing commit @ %v", time.Now()),
		})
		assert.NoError(t, err)

		pushedFiles = commitAndPushTest(t, dstPath, prefix, sizes...)
		t.Run("Locks", func(t *testing.T) {
			defer tests.PrintCurrentTest(t)()
			lockTest(t, dstPath)
		})
	})
	return pushedFiles
}

func commitAndPushTest(t *testing.T, dstPath, prefix string, sizes ...int) (pushedFiles []string) {
	for _, size := range sizes {
		t.Run("PushCommit Size-"+strconv.Itoa(size), func(t *testing.T) {
			defer tests.PrintCurrentTest(t)()
			pushedFiles = append(pushedFiles, doCommitAndPush(t, size, dstPath, prefix))
		})
	}
	return pushedFiles
}

func rawTest(t *testing.T, ctx *APITestContext, little, big, littleLFS, bigLFS string) {
	t.Run("Raw", func(t *testing.T) {
		defer tests.PrintCurrentTest(t)()
		username := ctx.Username
		reponame := ctx.Reponame

		session := loginUser(t, username)

		// Request raw paths
		req := NewRequest(t, "GET", path.Join("/api/v1/repos", username, reponame, "raw", little)+"?ref=master")
		resp := session.MakeRequest(t, req, http.StatusOK)
		assert.Equal(t, testFileSizeSmall, resp.Body.Len())

		if setting.LFS.StartServer {
			req = NewRequest(t, "GET", path.Join("/api/v1/repos", username, reponame, "raw", littleLFS)+"?ref=master")
			resp := session.MakeRequest(t, req, http.StatusOK)
			assert.NotEqual(t, testFileSizeSmall, resp.Body.Len())
			assert.LessOrEqual(t, resp.Body.Len(), 1024)
			if resp.Body.Len() != testFileSizeSmall && resp.Body.Len() <= 1024 {
				assert.Contains(t, resp.Body.String(), lfs.MetaFileIdentifier)
			}
		}

		if !testing.Short() {
			req = NewRequest(t, "GET", path.Join("/api/v1/repos", username, reponame, "raw", big)+"?ref=master")
			resp := session.MakeRequest(t, req, http.StatusOK)
			assert.Equal(t, testFileSizeLarge, resp.Body.Len())

			if setting.LFS.StartServer {
				req = NewRequest(t, "GET", path.Join("/api/v1/repos", username, reponame, "raw", bigLFS)+"?ref=master")
				resp := session.MakeRequest(t, req, http.StatusOK)
				assert.NotEqual(t, testFileSizeLarge, resp.Body.Len())
				if resp.Body.Len() != testFileSizeLarge && resp.Body.Len() <= 1024 {
					assert.Contains(t, resp.Body.String(), lfs.MetaFileIdentifier)
				}
			}
		}
	})
}

func mediaTest(t *testing.T, ctx *APITestContext, little, big, littleLFS, bigLFS string) {
	t.Run("Media", func(t *testing.T) {
		defer tests.PrintCurrentTest(t)()

		username := ctx.Username
		reponame := ctx.Reponame

		session := loginUser(t, username)

		// Request media paths
		req := NewRequest(t, "GET", path.Join("/api/v1/repos", username, reponame, "media", little)+"?ref=master")
		resp := session.MakeRequest(t, req, http.StatusOK)
		assert.Equal(t, testFileSizeSmall, resp.Body.Len())

		req = NewRequest(t, "GET", path.Join("/api/v1/repos", username, reponame, "media", littleLFS)+"?ref=master")
		resp = session.MakeRequest(t, req, http.StatusOK)
		assert.Equal(t, testFileSizeSmall, resp.Body.Len())

		if !testing.Short() {
			req = NewRequest(t, "GET", path.Join("/api/v1/repos", username, reponame, "media", big)+"?ref=master")
			resp = session.MakeRequest(t, req, http.StatusOK)
			assert.Equal(t, testFileSizeLarge, resp.Body.Len())

			if setting.LFS.StartServer {
				req = NewRequest(t, "GET", path.Join("/api/v1/repos", username, reponame, "media", bigLFS)+"?ref=master")
				resp = session.MakeRequest(t, req, http.StatusOK)
				assert.Equal(t, testFileSizeLarge, resp.Body.Len())
			}
		}
	})
}

func lockTest(t *testing.T, repoPath string) {
	lockFileTest(t, "README.md", repoPath)
}

func lockFileTest(t *testing.T, filename, repoPath string) {
	_, _, err := gitcmd.NewCommand("lfs").AddArguments("locks").WithDir(repoPath).RunStdString(t.Context())
	assert.NoError(t, err)
	_, _, err = gitcmd.NewCommand("lfs").AddArguments("lock").AddDynamicArguments(filename).WithDir(repoPath).RunStdString(t.Context())
	assert.NoError(t, err)
	_, _, err = gitcmd.NewCommand("lfs").AddArguments("locks").WithDir(repoPath).RunStdString(t.Context())
	assert.NoError(t, err)
	_, _, err = gitcmd.NewCommand("lfs").AddArguments("unlock").AddDynamicArguments(filename).WithDir(repoPath).RunStdString(t.Context())
	assert.NoError(t, err)
}

func doCommitAndPush(t *testing.T, size int, repoPath, prefix string) string {
	name, err := generateCommitWithNewData(t.Context(), size, repoPath, "user2@example.com", "User Two", prefix)
	assert.NoError(t, err)
	_, _, err = gitcmd.NewCommand("push", "origin", "master").WithDir(repoPath).RunStdString(t.Context())
	assert.NoError(t, err)
	return name
}

func generateCommitWithNewData(ctx context.Context, size int, repoPath, email, fullName, prefix string) (string, error) {
	tmpFile, err := os.CreateTemp(repoPath, prefix)
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	var seed [32]byte
	rander := mathRand.NewChaCha8(seed) // for testing only, no need to seed
	_, err = io.CopyN(tmpFile, rander, int64(size))
	if err != nil {
		return "", err
	}
	_ = tmpFile.Close()

	// Commit
	err = gitAddChangesDeprecated(ctx, repoPath, false, filepath.Base(tmpFile.Name()))
	if err != nil {
		return "", err
	}
	err = gitCommitChangesDeprecated(ctx, repoPath, gitCommitChangesOptions{
		Committer: &git.Signature{
			Email: email,
			Name:  fullName,
			When:  time.Now(),
		},
		Author: &git.Signature{
			Email: email,
			Name:  fullName,
			When:  time.Now(),
		},
		Message: fmt.Sprintf("Testing commit @ %v", time.Now()),
	})
	return filepath.Base(tmpFile.Name()), err
}

func doCreateProtectedBranch(baseCtx *APITestContext, dstPath string) func(t *testing.T) {
	return func(t *testing.T) {
		defer tests.PrintCurrentTest(t)()
		ctx := NewAPITestContext(t, baseCtx.Username, baseCtx.Reponame, auth_model.AccessTokenScopeWriteRepository)

		t.Run("ProtectBranchWithFilePatterns", doProtectBranch(ctx, "release-*", baseCtx.Username, "", "", "config*"))

		// push a new branch without any new commits
		t.Run("CreateProtectedBranch-NoChanges", doGitCreateBranch(dstPath, "release-v1.0"))
		t.Run("PushProtectedBranch-NoChanges", doGitPushTestRepository(dstPath, "origin", "release-v1.0"))
		t.Run("CheckoutMaster-NoChanges", doGitCheckoutBranch(dstPath, "master"))

		// push a new branch with a new unprotected file
		t.Run("CreateProtectedBranch-UnprotectedFile", doGitCreateBranch(dstPath, "release-v2.0"))
		_, err := generateCommitWithNewData(t.Context(), testFileSizeSmall, dstPath, "user2@example.com", "User Two", "abc.txt")
		assert.NoError(t, err)
		t.Run("PushProtectedBranch-UnprotectedFile", doGitPushTestRepository(dstPath, "origin", "release-v2.0"))
		t.Run("CheckoutMaster-UnprotectedFile", doGitCheckoutBranch(dstPath, "master"))

		// push a new branch with a new protected file
		t.Run("CreateProtectedBranch-ProtectedFile", doGitCreateBranch(dstPath, "release-v3.0"))
		_, err = generateCommitWithNewData(t.Context(), testFileSizeSmall, dstPath, "user2@example.com", "User Two", "config")
		assert.NoError(t, err)
		t.Run("PushProtectedBranch-ProtectedFile", doGitPushTestRepositoryFail(dstPath, "origin", "release-v3.0"))
		t.Run("CheckoutMaster-ProtectedFile", doGitCheckoutBranch(dstPath, "master"))
	}
}

func doBranchProtectPRMerge(baseCtx *APITestContext, dstPath string) func(t *testing.T) {
	return func(t *testing.T) {
		defer tests.PrintCurrentTest(t)()
		t.Run("CreateBranchProtected", doGitCreateBranch(dstPath, "protected"))
		t.Run("PushProtectedBranch", doGitPushTestRepository(dstPath, "origin", "protected"))

		ctx := NewAPITestContext(t, baseCtx.Username, baseCtx.Reponame, auth_model.AccessTokenScopeWriteRepository)

		// Protect branch without any whitelisting
		t.Run("ProtectBranchNoWhitelist", doProtectBranch(ctx, "protected", "", "", "", ""))

		// Try to push without permissions, which should fail
		t.Run("TryPushWithoutPermissions", func(t *testing.T) {
			_, err := generateCommitWithNewData(t.Context(), testFileSizeSmall, dstPath, "user2@example.com", "User Two", "branch-data-file-")
			assert.NoError(t, err)
			doGitPushTestRepositoryFail(dstPath, "origin", "protected")(t)
		})

		// Set up permissions for normal push but not force push
		t.Run("SetupNormalPushPermissions", doProtectBranch(ctx, "protected", baseCtx.Username, "", "", ""))

		// Normal push should work
		t.Run("NormalPushWithPermissions", func(t *testing.T) {
			_, err := generateCommitWithNewData(t.Context(), testFileSizeSmall, dstPath, "user2@example.com", "User Two", "branch-data-file-")
			assert.NoError(t, err)
			doGitPushTestRepository(dstPath, "origin", "protected")(t)
		})

		// Try to force push without force push permissions, which should fail
		t.Run("ForcePushWithoutForcePermissions", func(t *testing.T) {
			t.Run("CreateDivergentHistory", func(t *testing.T) {
				gitcmd.NewCommand("reset", "--hard", "HEAD~1").WithDir(dstPath).Run(t.Context())
				_, err := generateCommitWithNewData(t.Context(), testFileSizeSmall, dstPath, "user2@example.com", "User Two", "branch-data-file-new")
				assert.NoError(t, err)
			})
			doGitPushTestRepositoryFail(dstPath, "-f", "origin", "protected")(t)
		})

		// Set up permissions for force push but not normal push
		t.Run("SetupForcePushPermissions", doProtectBranch(ctx, "protected", "", baseCtx.Username, "", ""))

		// Try to force push without normal push permissions, which should fail
		t.Run("ForcePushWithoutNormalPermissions", doGitPushTestRepositoryFail(dstPath, "-f", "origin", "protected"))

		// Set up permissions for normal and force push (both are required to force push)
		t.Run("SetupNormalAndForcePushPermissions", doProtectBranch(ctx, "protected", baseCtx.Username, baseCtx.Username, "", ""))

		// Force push should now work
		t.Run("ForcePushWithPermissions", doGitPushTestRepository(dstPath, "-f", "origin", "protected"))

		t.Run("ProtectProtectedBranchNoWhitelist", doProtectBranch(ctx, "protected", "", "", "", ""))
		t.Run("PushToUnprotectedBranch", doGitPushTestRepository(dstPath, "origin", "protected:unprotected"))
		var pr api.PullRequest
		var err error
		t.Run("CreatePullRequest", func(t *testing.T) {
			pr, err = doAPICreatePullRequest(ctx, baseCtx.Username, baseCtx.Reponame, "protected", "unprotected")(t)
			assert.NoError(t, err)
		})
		t.Run("GenerateCommit", func(t *testing.T) {
			_, err := generateCommitWithNewData(t.Context(), testFileSizeSmall, dstPath, "user2@example.com", "User Two", "branch-data-file-")
			assert.NoError(t, err)
		})
		t.Run("PushToUnprotectedBranch", doGitPushTestRepository(dstPath, "origin", "protected:unprotected-2"))
		var pr2 api.PullRequest
		t.Run("CreatePullRequest", func(t *testing.T) {
			pr2, err = doAPICreatePullRequest(ctx, baseCtx.Username, baseCtx.Reponame, "unprotected", "unprotected-2")(t)
			assert.NoError(t, err)
		})
		t.Run("MergePR2", doAPIMergePullRequest(ctx, baseCtx.Username, baseCtx.Reponame, pr2.Index))
		t.Run("MergePR", doAPIMergePullRequest(ctx, baseCtx.Username, baseCtx.Reponame, pr.Index))
		t.Run("PullProtected", doGitPull(dstPath, "origin", "protected"))

		t.Run("ProtectProtectedBranchUnprotectedFilePaths", doProtectBranch(ctx, "protected", "", "", "unprotected-file-*", ""))
		t.Run("GenerateCommit", func(t *testing.T) {
			_, err := generateCommitWithNewData(t.Context(), testFileSizeSmall, dstPath, "user2@example.com", "User Two", "unprotected-file-")
			assert.NoError(t, err)
		})
		t.Run("PushUnprotectedFilesToProtectedBranch", doGitPushTestRepository(dstPath, "origin", "protected"))

		t.Run("ProtectProtectedBranchWhitelist", doProtectBranch(ctx, "protected", baseCtx.Username, "", "", ""))

		t.Run("CheckoutMaster", doGitCheckoutBranch(dstPath, "master"))
		t.Run("CreateBranchForced", doGitCreateBranch(dstPath, "toforce"))
		t.Run("GenerateCommit", func(t *testing.T) {
			_, err := generateCommitWithNewData(t.Context(), testFileSizeSmall, dstPath, "user2@example.com", "User Two", "branch-data-file-")
			assert.NoError(t, err)
		})
		t.Run("FailToForcePushToProtectedBranch", doGitPushTestRepositoryFail(dstPath, "-f", "origin", "toforce:protected"))
		t.Run("MergeProtectedToToforce", doGitMerge(dstPath, "protected"))
		t.Run("PushToProtectedBranch", doGitPushTestRepository(dstPath, "origin", "toforce:protected"))
		t.Run("CheckoutMasterAgain", doGitCheckoutBranch(dstPath, "master"))
	}
}

func doProtectBranch(ctx APITestContext, branch, userToWhitelistPush, userToWhitelistForcePush, unprotectedFilePatterns, protectedFilePatterns string) func(t *testing.T) {
	return doProtectBranchExt(ctx, branch, doProtectBranchOptions{
		UserToWhitelistPush:      userToWhitelistPush,
		UserToWhitelistForcePush: userToWhitelistForcePush,
		UnprotectedFilePatterns:  unprotectedFilePatterns,
		ProtectedFilePatterns:    protectedFilePatterns,
	})
}

type doProtectBranchOptions struct {
	UserToWhitelistPush, UserToWhitelistForcePush, UnprotectedFilePatterns, ProtectedFilePatterns string

	StatusCheckPatterns []string
}

func doMergeFork(ctx, baseCtx APITestContext, baseBranch, headBranch string) func(t *testing.T) {
	return func(t *testing.T) {
		defer tests.PrintCurrentTest(t)()
		var pr api.PullRequest
		var err error

		// Create a test pullrequest
		t.Run("CreatePullRequest", func(t *testing.T) {
			pr, err = doAPICreatePullRequest(ctx, baseCtx.Username, baseCtx.Reponame, baseBranch, headBranch)(t)
			assert.NoError(t, err)
		})

		// Ensure the PR page works
		t.Run("EnsureCanSeePull", doEnsureCanSeePull(baseCtx, pr))

		// Then get the diff string
		var diffContent string
		t.Run("GetDiff", func(t *testing.T) {
			req := NewRequest(t, "GET", fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%d.diff", url.PathEscape(baseCtx.Username), url.PathEscape(baseCtx.Reponame), pr.Index))
			resp := ctx.Session.MakeRequest(t, req, http.StatusOK)
			diffContent = resp.Body.String()
		})

		// Now: Merge the PR & make sure that doesn't break the PR page or change its diff
		t.Run("MergePR", doAPIMergePullRequest(baseCtx, baseCtx.Username, baseCtx.Reponame, pr.Index))
		t.Run("EnsureCanSeePull", doEnsureCanSeePull(baseCtx, pr))
		t.Run("CheckPR", func(t *testing.T) {
			oldMergeBase := pr.MergeBase
			pr2, err := doAPIGetPullRequest(baseCtx, baseCtx.Username, baseCtx.Reponame, pr.Index)(t)
			assert.NoError(t, err)
			assert.Equal(t, oldMergeBase, pr2.MergeBase)
		})
		t.Run("EnsurDiffNoChange", doEnsureDiffNoChange(baseCtx, pr, diffContent))

		// Delete the head repository & make sure that doesn't break the PR page or change its diff
		t.Run("DeleteHeadRepository", doAPIDeleteRepository(ctx))
		t.Run("EnsureCanSeePull", doEnsureCanSeePull(baseCtx, pr))
		t.Run("EnsureDiffNoChange", doEnsureDiffNoChange(baseCtx, pr, diffContent))
	}
}

func doCreatePRAndSetManuallyMerged(ctx, baseCtx APITestContext, dstPath, baseBranch, headBranch string) func(t *testing.T) {
	return func(t *testing.T) {
		defer tests.PrintCurrentTest(t)()
		var (
			pr           api.PullRequest
			err          error
			lastCommitID string
		)

		trueBool := true
		falseBool := false

		t.Run("AllowSetManuallyMergedAndSwitchOffAutodetectManualMerge", doAPIEditRepository(baseCtx, &api.EditRepoOption{
			HasPullRequests:       &trueBool,
			AllowManualMerge:      &trueBool,
			AutodetectManualMerge: &falseBool,
		}))

		t.Run("CreateHeadBranch", doGitCreateBranch(dstPath, headBranch))
		t.Run("PushToHeadBranch", doGitPushTestRepository(dstPath, "origin", headBranch))
		t.Run("CreateEmptyPullRequest", func(t *testing.T) {
			pr, err = doAPICreatePullRequest(ctx, baseCtx.Username, baseCtx.Reponame, baseBranch, headBranch)(t)
			assert.NoError(t, err)
		})
		lastCommitID = pr.Base.Sha
		t.Run("ManuallyMergePR", doAPIManuallyMergePullRequest(ctx, baseCtx.Username, baseCtx.Reponame, lastCommitID, pr.Index))
	}
}

func doEnsureCanSeePull(ctx APITestContext, pr api.PullRequest) func(t *testing.T) {
	return func(t *testing.T) {
		req := NewRequest(t, "GET", fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%d", url.PathEscape(ctx.Username), url.PathEscape(ctx.Reponame), pr.Index))
		ctx.Session.MakeRequest(t, req, http.StatusOK)
		req = NewRequest(t, "GET", fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%d/files", url.PathEscape(ctx.Username), url.PathEscape(ctx.Reponame), pr.Index))
		ctx.Session.MakeRequest(t, req, http.StatusOK)
		req = NewRequest(t, "GET", fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%d/commits", url.PathEscape(ctx.Username), url.PathEscape(ctx.Reponame), pr.Index))
		ctx.Session.MakeRequest(t, req, http.StatusOK)
	}
}

func doEnsureDiffNoChange(ctx APITestContext, pr api.PullRequest, diffContent string) func(t *testing.T) {
	return func(t *testing.T) {
		req := NewRequest(t, "GET", fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%d.diff", url.PathEscape(ctx.Username), url.PathEscape(ctx.Reponame), pr.Index))
		resp := ctx.Session.MakeRequest(t, req, http.StatusOK)
		actual := resp.Body.String()
		assert.Equal(t, diffContent, actual)
	}
}

func doPushCreate(ctx APITestContext, u *url.URL) func(t *testing.T) {
	return func(t *testing.T) {
		defer tests.PrintCurrentTest(t)()

		// create a context for a currently non-existent repository
		ctx.Reponame = "repo-tmp-push-create-" + u.Scheme
		u.Path = ctx.GitPath()

		// Create a temporary directory
		tmpDir := t.TempDir()

		// Now create local repository to push as our test and set its origin
		t.Run("InitTestRepository", doGitInitTestRepository(tmpDir))
		t.Run("AddRemote", doGitAddRemote(tmpDir, "origin", u))

		// Disable "Push To Create" and attempt to push
		setting.Repository.EnablePushCreateUser = false
		t.Run("FailToPushAndCreateTestRepository", doGitPushTestRepositoryFail(tmpDir, "origin", "master"))

		// Enable "Push To Create"
		setting.Repository.EnablePushCreateUser = true

		// Assert that cloning from a non-existent repository does not create it and that it definitely wasn't create above
		t.Run("FailToCloneFromNonExistentRepository", doGitCloneFail(u))

		// Then "Push To Create"x
		t.Run("SuccessfullyPushAndCreateTestRepository", doGitPushTestRepository(tmpDir, "origin", "master"))

		// Finally, fetch repo from database and ensure the correct repository has been created
		repo, err := repo_model.GetRepositoryByOwnerAndName(t.Context(), ctx.Username, ctx.Reponame)
		assert.NoError(t, err)
		assert.False(t, repo.IsEmpty)
		assert.True(t, repo.IsPrivate)

		// Now add a remote that is invalid to "Push To Create"
		invalidCtx := ctx
		invalidCtx.Reponame = "invalid/repo-tmp-push-create-" + u.Scheme
		u.Path = invalidCtx.GitPath()
		t.Run("AddInvalidRemote", doGitAddRemote(tmpDir, "invalid", u))

		// Fail to "Push To Create" the invalid
		t.Run("FailToPushAndCreateInvalidTestRepository", doGitPushTestRepositoryFail(tmpDir, "invalid", "master"))
	}
}

func doProtectBranchExt(ctx APITestContext, ruleName string, opts doProtectBranchOptions) func(t *testing.T) {
	// We are going to just use the owner to set the protection.
	return func(t *testing.T) {
		// replace any existing rule, the create call sets every field like the old settings form did
		req := NewRequestf(t, "DELETE", "/api/v1/repos/%s/%s/branch_protections/%s", url.PathEscape(ctx.Username), url.PathEscape(ctx.Reponame), url.PathEscape(ruleName)).
			AddTokenAuth(ctx.Token)
		ctx.Session.MakeRequest(t, req, NoExpectedStatus)

		form := &api.CreateBranchProtectionOption{
			RuleName:                ruleName,
			UnprotectedFilePatterns: opts.UnprotectedFilePatterns,
			ProtectedFilePatterns:   opts.ProtectedFilePatterns,
		}

		if opts.UserToWhitelistPush != "" {
			form.EnablePush = true
			form.EnablePushWhitelist = true
			form.PushWhitelistUsernames = []string{opts.UserToWhitelistPush}
		}

		// the API only allows force push together with push, which is also what force push requires
		if opts.UserToWhitelistForcePush != "" {
			form.EnableForcePush = true
			form.EnableForcePushAllowlist = true
			form.ForcePushAllowlistUsernames = []string{opts.UserToWhitelistForcePush}
		}

		if len(opts.StatusCheckPatterns) > 0 {
			form.EnableStatusCheck = true
			form.StatusCheckContexts = opts.StatusCheckPatterns
		}

		req = NewRequestWithJSON(t, "POST", fmt.Sprintf("/api/v1/repos/%s/%s/branch_protections", url.PathEscape(ctx.Username), url.PathEscape(ctx.Reponame)), form).
			AddTokenAuth(ctx.Token)
		resp := ctx.Session.MakeRequest(t, req, http.StatusCreated)
		assert.Equal(t, ruleName, DecodeJSON(t, resp, &api.BranchProtection{}).RuleName)
	}
}
