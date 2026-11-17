// Copyright 2015 The Gogs Authors. All rights reserved.
// Copyright 2016 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

// Package v1 Gitea API
//
// This documentation describes the Gitea API.
//
//	Schemes: https, http
//	License: MIT http://opensource.org/licenses/MIT
//
//	Consumes:
//	- application/json
//
//	Produces:
//	- application/json
//
//	Security:
//	- BasicAuth :
//	- AuthorizationHeaderToken :
//
//	SecurityDefinitions:
//	BasicAuth:
//	     type: basic
//	AuthorizationHeaderToken:
//	     type: apiKey
//	     name: Authorization
//	     in: header
//	     description: API tokens must be prepended with "token" followed by a space.
//
// swagger:meta
package v1

import (
	"fmt"
	"net/http"
	"strings"

	auth_model "gitea.dev/models/auth"
	access_model "gitea.dev/models/perm/access"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unit"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/log"
	"gitea.dev/modules/setting"
	api "gitea.dev/modules/structs"
	"gitea.dev/modules/util"
	"gitea.dev/modules/web"
	"gitea.dev/routers/api/v1/admin"
	"gitea.dev/routers/api/v1/misc"
	"gitea.dev/routers/api/v1/repo"
	"gitea.dev/routers/api/v1/settings"
	"gitea.dev/routers/api/v1/token"
	"gitea.dev/routers/api/v1/user"
	"gitea.dev/routers/common"
	"gitea.dev/services/auth"
	"gitea.dev/services/context"
	"gitea.dev/services/forms"

	_ "gitea.dev/routers/api/v1/swagger" // for swagger generation

	"gitea.com/go-chi/binding"
	chi_middleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

func repoAssignment() func(ctx *context.APIContext) {
	return func(ctx *context.APIContext) {
		userName := ctx.PathParam("username")
		repoName := ctx.PathParam("reponame")

		var (
			owner *user_model.User
			err   error
		)

		// Check if the user is the same as the repository owner.
		if ctx.IsSigned && strings.EqualFold(ctx.Doer.LowerName, userName) {
			owner = ctx.Doer
		} else {
			owner, err = user_model.GetUserByName(ctx, userName)
			if err != nil {
				if user_model.IsErrUserNotExist(err) {
					if redirectUserID, err := user_model.LookupUserRedirect(ctx, userName); err == nil {
						context.RedirectToUser(ctx.Base, ctx.Doer, userName, redirectUserID)
					} else if user_model.IsErrUserRedirectNotExist(err) {
						ctx.APIErrorNotFound()
					} else {
						ctx.APIErrorInternal(err)
					}
				} else {
					ctx.APIErrorInternal(err)
				}
				return
			}
		}
		ctx.Repo.Owner = owner
		ctx.ContextUser = owner

		// Get repository.
		repo, err := repo_model.GetRepositoryByName(ctx, owner.ID, repoName)
		if err != nil {
			if repo_model.IsErrRepoNotExist(err) {
				redirectRepoID, err := repo_model.LookupRedirect(ctx, owner.ID, repoName)
				if err == nil {
					context.RedirectToRepo(ctx.Base, redirectRepoID)
				} else if repo_model.IsErrRedirectNotExist(err) {
					ctx.APIErrorNotFound()
				} else {
					ctx.APIErrorInternal(err)
				}
			} else {
				ctx.APIErrorInternal(err)
			}
			return
		}

		repo.Owner = owner
		ctx.Repo.Repository = repo

		ctx.Repo.Permission, err = access_model.GetDoerRepoPermission(ctx, repo, ctx.Doer)
		if err != nil {
			ctx.APIErrorInternal(err)
			return
		}

		if !ctx.Repo.Permission.HasAnyUnitAccess() {
			ctx.APIErrorNotFound()
			return
		}

		if !ctx.TokenCanAccessRepo(repo) {
			ctx.APIErrorNotFound()
			return
		}
	}
}

func checkTokenPublicOnly() func(ctx *context.APIContext) {
	return func(ctx *context.APIContext) {
		if !ctx.PublicOnly {
			return
		}

		requiredScopeCategories, ok := ctx.Data["requiredScopeCategories"].([]auth_model.AccessTokenScopeCategory)
		if !ok || len(requiredScopeCategories) == 0 {
			return
		}

		for _, category := range requiredScopeCategories {
			switch category {
			case auth_model.AccessTokenScopeCategoryRepository:
				if !ctx.TokenCanAccessRepo(ctx.Repo.Repository) {
					ctx.APIError(http.StatusForbidden, "token scope is limited to public repos")
					return
				}
			case auth_model.AccessTokenScopeCategoryIssue:
				if !ctx.TokenCanAccessRepo(ctx.Repo.Repository) {
					ctx.APIError(http.StatusForbidden, "token scope is limited to public issues")
					return
				}
			case auth_model.AccessTokenScopeCategoryUser:
				if ctx.ContextUser != nil && ctx.ContextUser.IsTokenAccessAllowed() && !ctx.ContextUser.Visibility.IsPublic() {
					ctx.APIError(http.StatusForbidden, "token scope is limited to public users")
					return
				}
			case auth_model.AccessTokenScopeCategoryActivityPub:
				if ctx.ContextUser != nil && ctx.ContextUser.IsTokenAccessAllowed() && !ctx.ContextUser.Visibility.IsPublic() {
					ctx.APIError(http.StatusForbidden, "token scope is limited to public activitypub")
					return
				}
			case auth_model.AccessTokenScopeCategoryNotification:
				if !ctx.TokenCanAccessRepo(ctx.Repo.Repository) {
					ctx.APIError(http.StatusForbidden, "token scope is limited to public notifications")
					return
				}
			}
		}
	}
}

func rejectPublicOnly() func(ctx *context.APIContext) {
	return func(ctx *context.APIContext) {
		if !ctx.PublicOnly {
			return
		}

		ctx.APIError(http.StatusForbidden, "this endpoint is not available for public-only tokens")
	}
}

func contextAuthenticatedUser() func(ctx *context.APIContext) {
	return func(ctx *context.APIContext) {
		ctx.ContextUser = ctx.Doer
	}
}

// if a token is being used for auth, we check that it contains the required scope
// if a token is not being used, reqToken will enforce other sign in methods
func tokenRequiresScopes(requiredScopeCategories ...auth_model.AccessTokenScopeCategory) func(ctx *context.APIContext) {
	return func(ctx *context.APIContext) {
		// no scope required
		if len(requiredScopeCategories) == 0 {
			return
		}

		// Need OAuth2 token to be present.
		scope, scopeExists := ctx.Data["ApiTokenScope"].(auth_model.AccessTokenScope)
		if ctx.Data["IsApiToken"] != true || !scopeExists {
			return
		}

		// use the http method to determine the access level
		requiredScopeLevel := auth_model.Read
		if ctx.Req.Method == http.MethodPost || ctx.Req.Method == http.MethodPut || ctx.Req.Method == http.MethodPatch || ctx.Req.Method == http.MethodDelete {
			requiredScopeLevel = auth_model.Write
		}

		// get the required scope for the given access level and category
		requiredScopes := auth_model.GetRequiredScopes(requiredScopeLevel, requiredScopeCategories...)
		allow, err := scope.HasScope(requiredScopes...)
		if err != nil {
			ctx.APIError(http.StatusForbidden, "checking scope failed: "+err.Error())
			return
		}

		if !allow {
			ctx.APIError(http.StatusForbidden, fmt.Sprintf("token does not have at least one of required scope(s), required=%v, token scope=%v", requiredScopes, scope))
			return
		}

		ctx.Data["requiredScopeCategories"] = requiredScopeCategories

		// check if scope only applies to public resources
		publicOnly, err := scope.PublicOnly()
		if err != nil {
			ctx.APIError(http.StatusForbidden, "parsing public resource scope failed: "+err.Error())
			return
		}

		// assign to true so that those searching should only filter public repositories/users/organizations
		ctx.PublicOnly = publicOnly
	}
}

// Contexter middleware already checks token for user sign in process.
func reqToken() func(ctx *context.APIContext) {
	return func(ctx *context.APIContext) {
		// if a real user is signed in, or the user is from a Actions task, we are good
		if ctx.IsSigned {
			return
		}
		ctx.APIError(http.StatusUnauthorized, "token is required")
	}
}

// reqSiteAdmin user should be the site admin
func reqSiteAdmin() func(ctx *context.APIContext) {
	return func(ctx *context.APIContext) {
		if !ctx.IsUserSiteAdmin() {
			ctx.APIError(http.StatusForbidden, "user should be the site admin")
			return
		}
	}
}

func reqGitHook() func(ctx *context.APIContext) {
	return func(ctx *context.APIContext) {
		if !ctx.Doer.CanEditGitHook() {
			ctx.APIError(http.StatusForbidden, "must be allowed to edit Git hooks")
			return
		}
	}
}

// reqWebhooksEnabled requires webhooks to be enabled by admin.
func reqWebhooksEnabled() func(ctx *context.APIContext) {
	return func(ctx *context.APIContext) {
		if setting.DisableWebhooks {
			ctx.APIError(http.StatusForbidden, "webhooks disabled by administrator")
			return
		}
	}
}

func mustAllowPulls(ctx *context.APIContext) {
	if !(ctx.Repo.Repository.CanEnablePulls() && ctx.Repo.Permission.CanRead(unit.TypePullRequests)) {
		if ctx.Repo.Repository.CanEnablePulls() && log.IsTrace() {
			if ctx.IsSigned {
				log.Trace("Permission Denied: User %-v cannot read %-v in Repo %-v\n"+
					"User in Repo has Permissions: %-+v",
					ctx.Doer,
					unit.TypePullRequests,
					ctx.Repo.Repository,
					ctx.Repo.Permission)
			} else {
				log.Trace("Permission Denied: Anonymous user cannot read %-v in Repo %-v\n"+
					"Anonymous user in Repo has Permissions: %-+v",
					unit.TypePullRequests,
					ctx.Repo.Repository,
					ctx.Repo.Permission)
			}
		}
		ctx.APIErrorNotFound()
		return
	}
}

func mustEnableIssuesOrPulls(ctx *context.APIContext) {
	if !ctx.Repo.Permission.CanRead(unit.TypeIssues) &&
		!(ctx.Repo.Repository.CanEnablePulls() && ctx.Repo.Permission.CanRead(unit.TypePullRequests)) {
		if ctx.Repo.Repository.CanEnablePulls() && log.IsTrace() {
			if ctx.IsSigned {
				log.Trace("Permission Denied: User %-v cannot read %-v and %-v in Repo %-v\n"+
					"User in Repo has Permissions: %-+v",
					ctx.Doer,
					unit.TypeIssues,
					unit.TypePullRequests,
					ctx.Repo.Repository,
					ctx.Repo.Permission)
			} else {
				log.Trace("Permission Denied: Anonymous user cannot read %-v and %-v in Repo %-v\n"+
					"Anonymous user in Repo has Permissions: %-+v",
					unit.TypeIssues,
					unit.TypePullRequests,
					ctx.Repo.Repository,
					ctx.Repo.Permission)
			}
		}
		ctx.APIErrorNotFound()
		return
	}
}

// FIXME: for consistency, maybe most mustNotBeArchived checks should be replaced with mustEnableEditor
func mustNotBeArchived(ctx *context.APIContext) {
	if ctx.Repo.Repository.IsArchived {
		ctx.APIError(http.StatusLocked, "repo is archived")
		return
	}
}

func mustEnableEditor(ctx *context.APIContext) {
	if !ctx.Repo.Repository.CanEnableEditor() {
		ctx.APIError(http.StatusLocked, "repo is not allowed to edit")
		return
	}
}

// bind binding an obj to a func(ctx *context.APIContext)
func bind[T any](_ T) any {
	return func(ctx *context.APIContext) {
		theObj := new(T) // create a new form obj for every request but not use obj directly
		errs := binding.Bind(ctx.Req, theObj)
		if len(errs) > 0 {
			ctx.APIError(http.StatusUnprocessableEntity, fmt.Sprintf("%s: %s", errs[0].FieldNames, errs[0].Error()))
			return
		}
		web.SetForm(ctx, theObj)
	}
}

func buildAuthGroup() *auth.Group {
	group := auth.NewGroup(
		&auth.OAuth2{}, // token auth (PATs)
		&auth.Basic{},
	)
	return group
}

func apiAuth(authMethod auth.Method) func(*context.APIContext) {
	return func(ctx *context.APIContext) {
		ar, err := common.AuthShared(ctx.Base, authMethod)
		if err != nil {
			msg, ok := auth.ErrAsUserAuthMessage(err)
			msg = util.Iif(ok, msg, "invalid token")
			ctx.APIError(http.StatusUnauthorized, msg)
			return
		}
		ctx.Doer = ar.Doer
		ctx.IsSigned = ar.Doer != nil
	}
}

// checkSignedInUser rejects tokens of users who may not log in
func checkSignedInUser(ctx *context.APIContext) {
	if ctx.IsSigned && (!ctx.Doer.IsActive || ctx.Doer.ProhibitLogin) {
		log.Info("Failed authentication attempt for %s from %s", ctx.Doer.Name, ctx.RemoteAddr())
		ctx.APIError(http.StatusForbidden, "This account is prohibited from signing in")
	}
}

// checkQueryAuthToken tells a client that sends its token in the query string why it is not signed in
func checkQueryAuthToken(ctx *context.APIContext) {
	if util.HasCredentialQuery(ctx.Req.URL.RawQuery) {
		ctx.Resp.Header().Set("X-Gitea-Warning", "token and access_token query parameters are not accepted, send the token in the Authorization header")
	}
}

// Routes registers all v1 APIs routes to web application.
func Routes() *web.Router {
	m := web.NewRouter()

	// redirect HEAD requests to GET if no HEAD handler is defined (RFC 9110 §9.3.2)
	m.BeforeRouting(chi_middleware.GetHead)

	if setting.CORSConfig.Enabled {
		m.BeforeRouting(cors.Handler(cors.Options{
			AllowedOrigins:   setting.CORSConfig.AllowDomain,
			AllowedMethods:   setting.CORSConfig.Methods,
			AllowCredentials: setting.CORSConfig.AllowCredentials,
			AllowedHeaders:   append([]string{"Authorization"}, setting.CORSConfig.Headers...),
			MaxAge:           int(setting.CORSConfig.MaxAge.Seconds()),
		}))
	}

	m.AfterRouting(context.APIContexter())
	m.AfterRouting(checkQueryAuthToken)

	m.AfterRouting(apiAuth(buildAuthGroup()))
	m.AfterRouting(checkSignedInUser)

	// the only anonymous endpoint, so clients and health checks can probe the server before authenticating
	m.Get("/version", misc.Version)

	m.Group("", func() {
		m.Group("/settings", func() {
			m.Get("/api", settings.GetGeneralAPISettings)
			m.Get("/repository", settings.GetGeneralRepoSettings)
		})

		// Token introspection and deletion endpoint
		m.Combo("/token").
			Get(reqToken(), token.GetCurrentToken).
			Delete(reqToken(), token.DeleteCurrentToken)

		// Users (requires user scope)
		m.Group("/users", func() {
			m.Get("/search", user.Search)

			m.Group("/{username}", func() {
				m.Get("", user.GetInfo)
				m.Get("/repos", tokenRequiresScopes(auth_model.AccessTokenScopeCategoryRepository), user.ListUserRepos)
			}, context.UserAssignmentAPI(), checkTokenPublicOnly())
		}, tokenRequiresScopes(auth_model.AccessTokenScopeCategoryUser))

		// Users (requires user scope)
		m.Group("/user", func() {
			m.Get("", user.GetAuthenticatedUser)
			m.Group("/settings", func() {
				m.Get("", user.GetUserSettings)
				m.Patch("", bind(api.UserSettingsOptions{}), user.UpdateUserSettings)
			}, rejectPublicOnly())
			// Email addresses are always private account data.
			m.Combo("/emails", rejectPublicOnly()).
				Get(user.ListEmails).
				Post(bind(api.CreateEmailOption{}), user.AddEmail).
				Delete(bind(api.DeleteEmailOption{}), user.DeleteEmail)

			// (repo scope)
			m.Combo("/repos", tokenRequiresScopes(auth_model.AccessTokenScopeCategoryRepository)).Get(user.ListMyRepos).
				Post(rejectPublicOnly(), bind(api.CreateRepoOption{}), repo.Create)

			m.Group("/hooks", func() {
				m.Combo("").Get(user.ListHooks).
					Post(bind(api.CreateHookOption{}), user.CreateHook)
				m.Combo("/{id}").Get(user.GetHook).
					Patch(bind(api.EditHookOption{}), user.EditHook).
					Delete(user.DeleteHook)
			}, reqWebhooksEnabled(), rejectPublicOnly())
		}, tokenRequiresScopes(auth_model.AccessTokenScopeCategoryUser), reqToken(), contextAuthenticatedUser(), checkTokenPublicOnly())

		// requires repo scope
		// FIXME: Don't expose repository id outside of the system
		m.Combo("/repositories/{id}", reqToken(), tokenRequiresScopes(auth_model.AccessTokenScopeCategoryRepository)).Get(repo.GetByID)

		// Repos (requires repo scope)
		m.Group("/repos", func() {
			m.Get("/search", repo.Search)

			m.Group("/{username}/{reponame}", func() {
				m.Get("/compare/*", repo.CompareDiff)

				m.Combo("").Get(repo.Get).
					Delete(reqToken(), repo.Delete).
					Patch(reqToken(), bind(api.EditRepoOption{}), repo.Edit)

				m.Group("/hooks/git", func() {
					m.Combo("").Get(repo.ListGitHooks)
					m.Group("/{id}", func() {
						m.Combo("").Get(repo.GetGitHook).
							Patch(bind(api.EditGitHookOption{}), repo.EditGitHook).
							Delete(repo.DeleteGitHook)
					})
				}, reqToken(), reqGitHook(), context.ReferencesGitRepo(true))
				m.Group("/hooks", func() {
					m.Combo("").Get(repo.ListHooks).
						Post(bind(api.CreateHookOption{}), repo.CreateHook)
					m.Group("/{id}", func() {
						m.Combo("").Get(repo.GetHook).
							Patch(bind(api.EditHookOption{}), repo.EditHook).
							Delete(repo.DeleteHook)
						m.Post("/tests", context.ReferencesGitRepo(), context.RepoRefForAPI, repo.TestHook)
					})
				}, reqToken(), reqWebhooksEnabled())
				m.Get("/assignees", reqToken(), repo.GetAssignees)
				m.Get("/assignees/{assignee}", reqToken(), repo.CheckRepoIssueAssignee)
				m.Get("/reviewers", reqToken(), repo.GetReviewers)
				m.Get("/raw/*", context.ReferencesGitRepo(), context.RepoRefForAPI, repo.GetRawFile)
				m.Get("/media/*", context.ReferencesGitRepo(), context.RepoRefForAPI, repo.GetRawFileOrLFS)
				m.Combo("/forks").Get(repo.ListForks).
					Post(reqToken(), bind(api.CreateForkOption{}), repo.CreateFork)
				m.Post("/merge-upstream", reqToken(), mustNotBeArchived, bind(api.MergeUpstreamRequest{}), repo.MergeUpstream)
				m.Group("/branches", func() {
					m.Get("", repo.ListBranches)
					m.Get("/*", repo.GetBranch)
					m.Delete("/*", reqToken(), mustNotBeArchived, repo.DeleteBranch)
					m.Post("", reqToken(), mustNotBeArchived, bind(api.CreateBranchRepoOption{}), repo.CreateBranch)
					m.Put("/*", reqToken(), mustNotBeArchived, bind(api.UpdateBranchRepoOption{}), repo.UpdateBranch)
					m.Patch("/*", reqToken(), mustNotBeArchived, bind(api.RenameBranchRepoOption{}), repo.RenameBranch)
				}, context.ReferencesGitRepo())
				m.Group("/branch_protections", func() {
					m.Get("", repo.ListBranchProtections)
					m.Post("", bind(api.CreateBranchProtectionOption{}), mustNotBeArchived, repo.CreateBranchProtection)
					m.Group("/*", func() {
						m.Get("", repo.GetBranchProtection)
						m.Patch("", bind(api.EditBranchProtectionOption{}), mustNotBeArchived, repo.EditBranchProtection)
						m.Delete("", mustNotBeArchived, repo.DeleteBranchProtection)
					})
					m.Post("/priority", bind(api.UpdateBranchProtectionPriories{}), mustNotBeArchived, repo.UpdateBranchProtectionPriories)
				}, reqToken())
				m.Group("/tags", func() {
					m.Get("", repo.ListTags)
					m.Get("/*", repo.GetTag)
					m.Post("", reqToken(), mustNotBeArchived, bind(api.CreateTagOption{}), repo.CreateTag)
					m.Delete("/*", reqToken(), mustNotBeArchived, repo.DeleteTag)
				}, context.ReferencesGitRepo(true))

				m.Get("/editorconfig/{filename}", context.ReferencesGitRepo(), context.RepoRefForAPI, repo.GetEditorconfig)
				m.Group("/pulls", func() {
					m.Combo("").Get(repo.ListPullRequests).
						Post(reqToken(), mustNotBeArchived, bind(api.CreatePullRequestOption{}), repo.CreatePullRequest)
					m.Post("/comments/{id}/resolve", reqToken(), mustNotBeArchived, repo.ResolvePullReviewComment)
					m.Post("/comments/{id}/unresolve", reqToken(), mustNotBeArchived, repo.UnresolvePullReviewComment)
					m.Group("/{index}", func() {
						m.Combo("").Get(repo.GetPullRequest).
							Patch(reqToken(), bind(api.EditPullRequestOption{}), repo.EditPullRequest)
						m.Get(".{diffType:diff|patch}", repo.DownloadPullDiffOrPatch)
						m.Post("/update", reqToken(), repo.UpdatePullRequest)
						m.Get("/commits", repo.GetPullRequestCommits)
						m.Get("/files", repo.GetPullRequestFiles)
						m.Combo("/merge").Get(repo.IsPullRequestMerged).
							Post(reqToken(), mustNotBeArchived, bind(forms.MergePullRequestForm{}), repo.MergePullRequest)
						m.Group("/reviews", func() {
							m.Combo("").
								Get(repo.ListPullReviews).
								Post(reqToken(), bind(api.CreatePullReviewOptions{}), repo.CreatePullReview)
							m.Group("/{id}", func() {
								m.Combo("").
									Get(repo.GetPullReview).
									Delete(reqToken(), repo.DeletePullReview).
									Post(reqToken(), bind(api.SubmitPullReviewOptions{}), repo.SubmitPullReview)
								m.Combo("/comments").
									Get(repo.GetPullReviewComments)
								m.Post("/dismissals", reqToken(), bind(api.DismissPullReviewOptions{}), repo.DismissPullReview)
								m.Post("/undismissals", reqToken(), repo.UnDismissPullReview)
							})
						})
						m.Combo("/requested_reviewers", reqToken()).
							Delete(bind(api.PullReviewRequestOptions{}), repo.DeleteReviewRequests).
							Post(bind(api.PullReviewRequestOptions{}), repo.CreateReviewRequests)
						m.Post("/comments/{id}/replies", reqToken(), mustNotBeArchived, bind(api.CreatePullReviewCommentReplyOptions{}), repo.CreatePullReviewCommentReply)
					})
					m.Get("/{base}/*", repo.GetPullRequestByBaseHead)
				}, mustAllowPulls, context.ReferencesGitRepo())
				m.Group("/statuses", func() { // "/statuses/{sha}" only accepts commit ID
					m.Combo("/{sha}").Get(repo.GetCommitStatuses).
						Post(reqToken(), bind(api.CreateStatusOption{}), repo.NewCommitStatus)
				})
				m.Group("/commits", func() {
					m.Get("", context.ReferencesGitRepo(), repo.GetAllCommits)
					m.PathGroup("/*", func(g *web.RouterPathGroup) {
						// Mis-configured reverse proxy might decode the `%2F` to slash ahead, so we need to support both formats (escaped, unescaped) here.
						// It also matches GitHub's behavior
						g.MatchPath("GET", "/<ref:*>/status", repo.GetCombinedCommitStatusByRef)
						g.MatchPath("GET", "/<ref:*>/statuses", repo.GetCommitStatusesByRef)
						g.MatchPath("GET", "/<sha>/pull", repo.GetCommitPullRequest)
					})
				})
				m.Group("/git", func() {
					m.Group("/commits", func() {
						m.Get("/{sha}", repo.GetSingleCommit)
						m.Get("/{sha}.{diffType:diff|patch}", repo.DownloadCommitDiffOrPatch)
					})
					m.Get("/refs", repo.GetGitAllRefs)
					m.Get("/refs/*", repo.GetGitRefs)
					m.Get("/trees/{sha}", repo.GetTree)
					m.Get("/blobs/{sha}", repo.GetBlob)
					m.Get("/tags/{sha}", repo.GetAnnotatedTag)
					m.Get("/notes/{sha}", repo.GetNote)
				}, context.ReferencesGitRepo(true))
				m.Post("/diffpatch", mustEnableEditor, reqToken(), bind(api.ApplyDiffPatchFileOptions{}), repo.ReqChangeRepoFileOptionsAndCheck, repo.ApplyDiffPatch)
				m.Group("/contents", func() {
					m.Get("", repo.GetContentsList)
					m.Get("/*", repo.GetContents)
					m.Group("", func() {
						// "change file" operations, need permission to write to the target branch provided by the form
						m.Post("", bind(api.ChangeFilesOptions{}), repo.ReqChangeRepoFileOptionsAndCheck, repo.ChangeFiles)
						m.Group("/*", func() {
							m.Post("", bind(api.CreateFileOptions{}), repo.ReqChangeRepoFileOptionsAndCheck, repo.CreateFile)
							m.Put("", bind(api.UpdateFileOptions{}), repo.ReqChangeRepoFileOptionsAndCheck, repo.UpdateFile)
							m.Delete("", bind(api.DeleteFileOptions{}), repo.ReqChangeRepoFileOptionsAndCheck, repo.DeleteFile)
						})
					}, mustEnableEditor, reqToken())
				}, context.ReferencesGitRepo())
				m.Group("/contents-ext", func() {
					m.Get("", repo.GetContentsExt)
					m.Get("/*", repo.GetContentsExt)
				}, context.ReferencesGitRepo())
				m.Combo("/file-contents", context.ReferencesGitRepo()).
					Get(repo.GetFileContentsGet).
					Post(bind(api.GetFilesOptions{}), repo.GetFileContentsPost) // the POST method requires "write" permission, so we also support "GET" method above
			}, repoAssignment(), checkTokenPublicOnly())
		}, tokenRequiresScopes(auth_model.AccessTokenScopeCategoryRepository))

		// Issue (requires issue scope)
		m.Group("/repos", func() {
			m.Get("/issues/search", repo.SearchIssues)

			m.Group("/{username}/{reponame}", func() {
				m.Group("/issues", func() {
					m.Combo("").Get(repo.ListIssues).
						Post(reqToken(), mustNotBeArchived, bind(api.CreateIssueOption{}), repo.CreateIssue)
					m.Group("/comments", func() {
						m.Get("", repo.ListRepoIssueComments)
						m.Group("/{id}", func() {
							m.Combo("").
								Get(repo.GetIssueComment).
								Patch(mustNotBeArchived, reqToken(), bind(api.EditIssueCommentOption{}), repo.EditIssueComment).
								Delete(reqToken(), repo.DeleteIssueComment)
							m.Combo("/reactions").
								Get(repo.GetIssueCommentReactions).
								Post(reqToken(), bind(api.EditReactionOption{}), repo.PostIssueCommentReaction).
								Delete(reqToken(), bind(api.EditReactionOption{}), repo.DeleteIssueCommentReaction)
						})
					})
					m.Group("/{index}", func() {
						m.Combo("").Get(repo.GetIssue).
							Patch(reqToken(), bind(api.EditIssueOption{}), repo.EditIssue).
							Delete(reqToken(), context.ReferencesGitRepo(), repo.DeleteIssue)
						m.Combo("/assignees").
							Post(reqToken(), mustNotBeArchived, bind(api.IssueAssigneesOption{}), repo.AddIssueAssignees).
							Delete(reqToken(), mustNotBeArchived, bind(api.IssueAssigneesOption{}), repo.DeleteIssueAssignees)
						m.Get("/assignees/{assignee}", repo.CheckIssueAssignee)
						m.Group("/comments", func() {
							m.Combo("").Get(repo.ListIssueComments).
								Post(reqToken(), mustNotBeArchived, bind(api.CreateIssueCommentOption{}), repo.CreateIssueComment)
							m.Combo("/{id}", reqToken()).Patch(bind(api.EditIssueCommentOption{}), repo.EditIssueCommentDeprecated).
								Delete(repo.DeleteIssueCommentDeprecated)
						})
						m.Get("/timeline", repo.ListIssueCommentsAndTimeline)
						m.Group("/labels", func() {
							m.Combo("").Get(repo.ListIssueLabels).
								Post(reqToken(), bind(api.IssueLabelsOption{}), repo.AddIssueLabels).
								Put(reqToken(), bind(api.IssueLabelsOption{}), repo.ReplaceIssueLabels).
								Delete(reqToken(), repo.ClearIssueLabels)
							m.Delete("/{id}", reqToken(), repo.DeleteIssueLabel)
						})
						m.Combo("/deadline").Post(reqToken(), bind(api.EditDeadlineOption{}), repo.UpdateIssueDeadline)
						m.Combo("/reactions").
							Get(repo.GetIssueReactions).
							Post(reqToken(), bind(api.EditReactionOption{}), repo.PostIssueReaction).
							Delete(reqToken(), bind(api.EditReactionOption{}), repo.DeleteIssueReaction)
					})
				}, mustEnableIssuesOrPulls)
				m.Group("/labels", func() {
					m.Combo("").Get(repo.ListLabels).
						Post(reqToken(), bind(api.CreateLabelOption{}), repo.CreateLabel)
					m.Combo("/{id}").Get(repo.GetLabel).
						Patch(reqToken(), bind(api.EditLabelOption{}), repo.EditLabel).
						Delete(reqToken(), repo.DeleteLabel)
				})
			}, repoAssignment(), checkTokenPublicOnly())
		}, tokenRequiresScopes(auth_model.AccessTokenScopeCategoryIssue))

		m.Group("/admin", func() {
			m.Group("/unadopted", func() {
				m.Get("", admin.ListUnadoptedRepositories)
				m.Post("/{username}/{reponame}", admin.AdoptRepository)
				m.Delete("/{username}/{reponame}", admin.DeleteUnadoptedRepository)
			})
			m.Group("/hooks", func() {
				m.Combo("").Get(admin.ListHooks).
					Post(bind(api.CreateHookOption{}), admin.CreateHook)
				m.Combo("/{id}").Get(admin.GetHook).
					Patch(bind(api.EditHookOption{}), admin.EditHook).
					Delete(admin.DeleteHook)
			})
		}, tokenRequiresScopes(auth_model.AccessTokenScopeCategoryAdmin), reqToken(), reqSiteAdmin())
	}, reqToken())

	return m
}
