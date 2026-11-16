// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"context"
	"fmt"
	"strings"

	"gitea.dev/models/db"
	"gitea.dev/models/unit"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/container"
	"gitea.dev/modules/optional"
	"gitea.dev/modules/structs"
	"gitea.dev/modules/util"

	"xorm.io/builder"
)

// RepositoryListDefaultPageSize is the default number of repositories
// to load in memory when running administrative tasks on all (or almost
// all) of them.
// The number should be low enough to avoid filling up all RAM with
// repository data...
const RepositoryListDefaultPageSize = 64

// RepositoryList contains a list of repositories
type RepositoryList []*Repository

func (repos RepositoryList) Len() int {
	return len(repos)
}

func (repos RepositoryList) Less(i, j int) bool {
	return repos[i].FullName() < repos[j].FullName()
}

func (repos RepositoryList) Swap(i, j int) {
	repos[i], repos[j] = repos[j], repos[i]
}

// ValuesRepository converts a repository map to a list
// FIXME: Remove in favor of maps.values when MIN_GO_VERSION >= 1.18
func ValuesRepository(m map[int64]*Repository) []*Repository {
	values := make([]*Repository, 0, len(m))
	for _, v := range m {
		values = append(values, v)
	}
	return values
}

func (repos RepositoryList) LoadUnits(ctx context.Context) error {
	if len(repos) == 0 {
		return nil
	}

	// Load units.
	units := make([]*RepoUnit, 0, len(repos)*6)
	if err := db.GetEngine(ctx).
		In("repo_id", repos.IDs()).
		Find(&units); err != nil {
		return fmt.Errorf("find units: %w", err)
	}

	unitsMap := make(map[int64][]*RepoUnit, len(repos))
	for _, unit := range units {
		if !unit.Type.UnitGlobalDisabled() {
			unitsMap[unit.RepoID] = append(unitsMap[unit.RepoID], unit)
		}
	}

	for _, repo := range repos {
		repo.Units = unitsMap[repo.ID]
	}

	return nil
}

func (repos RepositoryList) IDs() []int64 {
	repoIDs := make([]int64, len(repos))
	for i := range repos {
		repoIDs[i] = repos[i].ID
	}
	return repoIDs
}

func (repos RepositoryList) LoadOwners(ctx context.Context) error {
	if len(repos) == 0 {
		return nil
	}

	userIDs := container.FilterSlice(repos, func(repo *Repository) (int64, bool) {
		return repo.OwnerID, true
	})

	// Load owners.
	users := make(map[int64]*user_model.User, len(userIDs))
	if err := db.GetEngine(ctx).
		Where("id > 0").
		In("id", userIDs).
		Find(&users); err != nil {
		return fmt.Errorf("find users: %w", err)
	}
	for i := range repos {
		repos[i].Owner = users[repos[i].OwnerID]
	}
	return nil
}

// LoadAttributes loads the attributes for the given RepositoryList
func (repos RepositoryList) LoadAttributes(ctx context.Context) error {
	return repos.LoadOwners(ctx)
}

// SearchRepoOptions holds the search options
type SearchRepoOptions struct {
	db.ListOptions
	Actor           *user_model.User
	Keyword         string
	OwnerID         int64
	PriorityOwnerID int64
	OrderBy         db.SearchOrderBy
	Private         bool  // Include private repositories in results
	StarredByID     int64 // stars were removed, so a set value matches no repository
	AllPublic       bool  // Include also all public repositories of users and public organisations
	AllLimited      bool  // Include also all public repositories of limited organisations
	// None -> include public and private
	// True -> include just private
	// False -> include just public
	IsPrivate optional.Option[bool]
	// None -> include collaborative AND non-collaborative
	// True -> include just collaborative
	// False -> include just non-collaborative
	Collaborate optional.Option[bool]
	// None -> include forks AND non-forks
	// True -> include just forks
	// False -> include just non-forks
	Fork optional.Option[bool]
	// If Fork option is True, you can use this option to limit the forks of a special repo by repo id.
	ForkFrom int64
	// None -> include templates AND non-templates
	// True -> include just templates
	// False -> include just non-templates
	Template optional.Option[bool]
	// None -> include mirrors AND non-mirrors
	// True -> include just mirrors
	// False -> include just non-mirrors
	Mirror optional.Option[bool]
	// None -> include archived AND non-archived
	// True -> include just archived
	// False -> include just non-archived
	Archived optional.Option[bool]
	// only search topic name; topics were removed, so it matches no repository
	TopicOnly bool
	// include description in keyword search
	IncludeDescription bool
	// LowerNames represents valid lower names to restrict to
	LowerNames []string
	// When specified true, apply some filters over the conditions:
	// - Don't show forks, when opts.Fork is OptionalBoolNone.
	// - Do not display repositories that don't have a description, an icon and topics.
	OnlyShowRelevant bool
}

func (opts *SearchRepoOptions) ApplyPublicOnly(publicOnly bool) {
	if publicOnly {
		opts.Private = false
		opts.AllLimited = false
	}
}

// UserOwnedRepoCond returns user ownered repositories
func UserOwnedRepoCond(userID int64) builder.Cond {
	return builder.Eq{
		"repository.owner_id": userID,
	}
}

// SearchRepositoryCondition creates a query condition according search repository options
func SearchRepositoryCondition(opts SearchRepoOptions) builder.Cond {
	cond := builder.NewCond()

	if opts.Private {
		if opts.Actor != nil && !opts.Actor.IsAdmin && opts.Actor.ID != opts.OwnerID {
			// OK we're in the context of a User
			cond = cond.And(AccessibleRepositoryCondition(opts.Actor, unit.TypeInvalid))
		}
	} else {
		// Not looking at private organisations and users
		// We should be able to see all non-private repositories that
		// isn't in a private or limited organisation.
		cond = cond.And(
			builder.Eq{"is_private": false},
			builder.NotIn("owner_id", builder.Select("id").From("`user`").Where(
				builder.Or(builder.Eq{"visibility": structs.VisibleTypeLimited}, builder.Eq{"visibility": structs.VisibleTypePrivate}),
			)))
	}

	if opts.IsPrivate.Has() {
		cond = cond.And(builder.Eq{"is_private": opts.IsPrivate.Value()})
	}

	if opts.Template.Has() {
		cond = cond.And(builder.Eq{"is_template": opts.Template.Value()})
	}

	// Restrict to starred repositories
	if opts.StarredByID > 0 {
		cond = cond.And(builder.Expr("1 = 0"))
	}

	// Restrict repositories to those the OwnerID owns or contributes to as per opts.Collaborate
	if opts.OwnerID > 0 {
		var accessCond builder.Cond = builder.Eq{"owner_id": opts.OwnerID}
		if opts.Collaborate.Value() {
			accessCond = builder.Expr("1 = 0") // there are no collaborators
		}

		if opts.AllPublic {
			accessCond = accessCond.Or(builder.Eq{"is_private": false}.And(builder.In("owner_id", builder.Select("`user`.id").From("`user`").Where(builder.Eq{"`user`.visibility": structs.VisibleTypePublic}))))
		}

		if opts.AllLimited {
			accessCond = accessCond.Or(builder.Eq{"is_private": false}.And(builder.In("owner_id", builder.Select("`user`.id").From("`user`").Where(builder.Eq{"`user`.visibility": structs.VisibleTypeLimited}))))
		}

		cond = cond.And(accessCond)
	}

	if opts.Keyword != "" {
		keywordCond := builder.Expr("1 = 0")
		if !opts.TopicOnly {
			// separate keyword
			likes := builder.NewCond()
			for v := range strings.SplitSeq(opts.Keyword, ",") {
				likes = likes.Or(builder.Like{"lower_name", strings.ToLower(v)})

				// If the string looks like "org/repo", match against that pattern too
				if strings.Count(opts.Keyword, "/") == 1 {
					pieces := strings.Split(opts.Keyword, "/")
					ownerName := pieces[0]
					repoName := pieces[1]
					likes = likes.Or(builder.And(builder.Like{"owner_name", strings.ToLower(ownerName)}, builder.Like{"lower_name", strings.ToLower(repoName)}))
				}

				if opts.IncludeDescription {
					likes = likes.Or(builder.Like{"LOWER(description)", strings.ToLower(v)})
				}
			}
			keywordCond = likes
		}
		cond = cond.And(keywordCond)
	}

	if opts.Fork.Has() || opts.OnlyShowRelevant {
		if opts.OnlyShowRelevant && !opts.Fork.Has() {
			cond = cond.And(builder.Eq{"is_fork": false})
		} else {
			cond = cond.And(builder.Eq{"is_fork": opts.Fork.Value()})

			if opts.ForkFrom > 0 && opts.Fork.Value() {
				cond = cond.And(builder.Eq{"fork_id": opts.ForkFrom})
			}
		}
	}

	if opts.Mirror.Has() {
		cond = cond.And(builder.Eq{"is_mirror": opts.Mirror.Value()})
	}

	if opts.Actor != nil && opts.Actor.IsRestricted {
		cond = cond.And(AccessibleRepositoryCondition(opts.Actor, unit.TypeInvalid))
	}

	if opts.Archived.Has() {
		cond = cond.And(builder.Eq{"is_archived": opts.Archived.Value()})
	}

	if opts.OnlyShowRelevant {
		// Only show a repo that has at least a topic, an icon, or a description
		subQueryCond := builder.NewCond()

		// Topic checking. Topics are present.
		subQueryCond = subQueryCond.Or(builder.And(builder.Neq{"topics": "null"}, builder.Neq{"topics": "[]"}))

		// Description checking. Description not empty
		subQueryCond = subQueryCond.Or(builder.Neq{"description": ""})

		// Repo has a avatar
		subQueryCond = subQueryCond.Or(builder.Neq{"avatar": ""})

		// Always hide repo's that are empty
		subQueryCond = subQueryCond.And(builder.Eq{"is_empty": false})

		cond = cond.And(subQueryCond)
	}

	return cond
}

// SearchRepository returns repositories based on search options,
// it returns results in given range and number of total results.
func SearchRepository(ctx context.Context, opts SearchRepoOptions) (RepositoryList, int64, error) {
	cond := SearchRepositoryCondition(opts)
	return SearchRepositoryByCondition(ctx, opts, cond, true)
}

// CountRepository counts repositories based on search options,
func CountRepository(ctx context.Context, opts SearchRepoOptions) (int64, error) {
	return db.GetEngine(ctx).Where(SearchRepositoryCondition(opts)).Count(new(Repository))
}

// SearchRepositoryByCondition search repositories by condition
func SearchRepositoryByCondition(ctx context.Context, opts SearchRepoOptions, cond builder.Cond, loadAttributes bool) (RepositoryList, int64, error) {
	sess, count, err := searchRepositoryByCondition(ctx, opts, cond)
	if err != nil {
		return nil, 0, err
	}

	defaultSize := 50
	if opts.PageSize > 0 {
		defaultSize = opts.PageSize
	}
	repos := make(RepositoryList, 0, defaultSize)
	if err := sess.Find(&repos); err != nil {
		return nil, 0, fmt.Errorf("Repo: %w", err)
	}

	if opts.PageSize <= 0 {
		count = int64(len(repos))
	}

	if loadAttributes {
		if err := repos.LoadAttributes(ctx); err != nil {
			return nil, 0, fmt.Errorf("LoadAttributes: %w", err)
		}
	}

	return repos, count, nil
}

func searchRepositoryByCondition(ctx context.Context, opts SearchRepoOptions, cond builder.Cond) (db.Engine, int64, error) {
	page := opts.Page
	if page <= 0 {
		page = 1
	}

	orderBy := opts.OrderBy
	if len(orderBy) == 0 {
		orderBy = db.SearchOrderByAlphabetically
	}

	args := make([]any, 0)
	if opts.PriorityOwnerID > 0 {
		orderBy = db.SearchOrderBy(fmt.Sprintf("CASE WHEN owner_id = ? THEN 0 ELSE owner_id END, %s", orderBy))
		args = append(args, opts.PriorityOwnerID)
	} else if strings.Count(opts.Keyword, "/") == 1 {
		// With "owner/repo" search times, prioritise results which match the owner field
		orgName := strings.Split(opts.Keyword, "/")[0]
		orderBy = db.SearchOrderBy(fmt.Sprintf("CASE WHEN owner_name LIKE ? THEN 0 ELSE 1 END, %s", orderBy))
		args = append(args, orgName)
	}

	sess := db.GetEngine(ctx)

	var count int64
	if opts.PageSize > 0 {
		var err error
		count, err = sess.
			Where(cond).
			Count(new(Repository))
		if err != nil {
			return nil, 0, fmt.Errorf("Count: %w", err)
		}
	}

	sess = sess.Where(cond).OrderBy(orderBy.String(), args...)
	if opts.PageSize > 0 {
		sess = sess.Limit(opts.PageSize, (page-1)*opts.PageSize)
	}
	return sess, count, nil
}

// SearchRepositoryIDsByCondition search repository IDs by given condition.
func SearchRepositoryIDsByCondition(ctx context.Context, cond builder.Cond) ([]int64, error) {
	repoIDs := make([]int64, 0, 10)
	return repoIDs, db.GetEngine(ctx).
		Table("repository").
		Cols("id").
		Where(cond).
		Find(&repoIDs)
}

// AccessibleRepositoryCondition takes a user a returns a condition for checking if a repository is accessible:
// the signed-in owner sees every repository, anonymous only genuinely public ones
func AccessibleRepositoryCondition(user *user_model.User, _ unit.Type) builder.Cond {
	if user != nil && user.ID > 0 {
		return builder.NewCond()
	}
	return PublicRepoUnderPublicOwnerCond()
}

// SearchRepositoryByName takes keyword and part of repository name to search,
// it returns results in given range and number of total results.
func SearchRepositoryByName(ctx context.Context, opts SearchRepoOptions) (RepositoryList, int64, error) {
	opts.IncludeDescription = false
	return SearchRepository(ctx, opts)
}

// PublicRepoUnderPublicOwnerCond restricts to public repos whose owner is publicly visible: the
// "genuinely public" set a public-only token or an anonymous caller may see (a public repo under a
// limited/private owner is not publicly reachable and must be excluded).
func PublicRepoUnderPublicOwnerCond() builder.Cond {
	return builder.And(
		builder.Eq{"`repository`.is_private": false},
		builder.In("`repository`.owner_id", builder.Select("id").From("`user`").Where(builder.Eq{"visibility": structs.VisibleTypePublic})),
	)
}

// NotPublicRepoUnderPublicOwnerCond complements PublicRepoUnderPublicOwnerCond. Spelled positively so
// the owner subquery hashes the limited/private minority, not every public user.
func NotPublicRepoUnderPublicOwnerCond() builder.Cond {
	return builder.Or(
		builder.Eq{"`repository`.is_private": true},
		builder.In("`repository`.owner_id", builder.Select("id").From("`user`").Where(builder.Neq{"visibility": structs.VisibleTypePublic})),
	)
}

// GetUserRepositories returns a list of repositories of given user.
func GetUserRepositories(ctx context.Context, opts SearchRepoOptions) (RepositoryList, int64, error) {
	if len(opts.OrderBy) == 0 {
		opts.OrderBy = "updated_unix DESC"
	}

	cond := builder.NewCond()
	if opts.Actor == nil {
		return nil, 0, util.NewInvalidArgumentErrorf("GetUserRepositories: Actor is needed but not given")
	}
	cond = cond.And(builder.Eq{"owner_id": opts.Actor.ID})
	if !opts.Private {
		cond = cond.And(builder.Eq{"is_private": false})
	}

	if len(opts.LowerNames) > 0 {
		cond = cond.And(builder.In("lower_name", opts.LowerNames))
	}

	sess := db.GetEngine(ctx)

	count, err := sess.Where(cond).Count(new(Repository))
	if err != nil {
		return nil, 0, fmt.Errorf("Count: %w", err)
	}

	sess = sess.Where(cond).OrderBy(opts.OrderBy.String())
	repos := make(RepositoryList, 0, opts.PageSize)
	db.SetSessionPagination(sess, &opts)
	return repos, count, sess.Find(&repos)
}
