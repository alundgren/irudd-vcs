// Copyright 2014 The Gogs Authors. All rights reserved.
// Copyright 2020 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package issues

import (
	"context"
	"fmt"
	"html/template"
	"regexp"
	"slices"

	"gitea.dev/models/db"
	repo_model "gitea.dev/models/repo"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/container"
	"gitea.dev/modules/log"
	api "gitea.dev/modules/structs"
	"gitea.dev/modules/timeutil"
	"gitea.dev/modules/util"

	"xorm.io/builder"
)

// ErrIssueNotExist represents a "IssueNotExist" kind of error.
type ErrIssueNotExist struct {
	ID     int64
	RepoID int64
	Index  int64
}

// IsErrIssueNotExist checks if an error is a ErrIssueNotExist.
func IsErrIssueNotExist(err error) bool {
	_, ok := err.(ErrIssueNotExist)
	return ok
}

func (err ErrIssueNotExist) Error() string {
	return fmt.Sprintf("issue does not exist [id: %d, repo_id: %d, index: %d]", err.ID, err.RepoID, err.Index)
}

func (err ErrIssueNotExist) Unwrap() error {
	return util.ErrNotExist
}

var ErrIssueAlreadyChanged = util.NewInvalidArgumentErrorf("the issue is already changed")

// Issue represents an issue or pull request of repository.
type Issue struct {
	ID               int64                  `xorm:"pk autoincr"`
	RepoID           int64                  `xorm:"INDEX UNIQUE(repo_index)"`
	Repo             *repo_model.Repository `xorm:"-"`
	Index            int64                  `xorm:"UNIQUE(repo_index)"` // Index in one repository.
	PosterID         int64                  `xorm:"INDEX"`
	Poster           *user_model.User       `xorm:"-"`
	OriginalAuthor   string
	OriginalAuthorID int64         `xorm:"index"`
	Title            string        `xorm:"name"`
	Content          string        `xorm:"LONGTEXT"`
	RenderedContent  template.HTML `xorm:"-"`
	ContentVersion   int           `xorm:"NOT NULL DEFAULT 0"`
	Labels           []*Label      `xorm:"-"`
	isLabelsLoaded   bool          `xorm:"-"`
	Priority         int
	AssigneeID       int64            `xorm:"-"`
	Assignee         *user_model.User `xorm:"-"`
	isAssigneeLoaded bool             `xorm:"-"`
	IsClosed         bool             `xorm:"INDEX"`
	IsRead           bool             `xorm:"-"`
	IsPull           bool             `xorm:"INDEX"` // Indicates whether is a pull request or not.
	PullRequest      *PullRequest     `xorm:"-"`
	NumComments      int

	// TODO: RemoveIssueRef: see "repo/issue/branch_selector_field.tmpl"
	Ref string

	DeadlineUnix timeutil.TimeStamp `xorm:"INDEX"`

	CreatedUnix timeutil.TimeStamp `xorm:"INDEX created"`
	UpdatedUnix timeutil.TimeStamp `xorm:"INDEX updated"`
	ClosedUnix  timeutil.TimeStamp `xorm:"INDEX"`

	Comments  CommentList        `xorm:"-"`
	Reactions ReactionList       `xorm:"-"`
	Assignees []*user_model.User `xorm:"-"`

	// IsLocked limits commenting abilities to users on an issue
	// with write access
	IsLocked bool `xorm:"NOT NULL DEFAULT false"`

	// For view issue page.
	ShowRole RoleDescriptor `xorm:"-"`
}

var (
	issueTasksPat     = regexp.MustCompile(`(^\s*[-*]\s\[[\sxX]\]\s.)|(\n\s*[-*]\s\[[\sxX]\]\s.)`)
	issueTasksDonePat = regexp.MustCompile(`(^\s*[-*]\s\[[xX]\]\s.)|(\n\s*[-*]\s\[[xX]\]\s.)`)
)

// IssueIndex represents the issue index table
type IssueIndex db.ResourceIndex

func init() {
	db.RegisterModel(new(Issue))
	db.RegisterModel(new(IssueIndex))
}

// IsOverdue checks if the issue is overdue
func (issue *Issue) IsOverdue() bool {
	if issue.IsClosed {
		return issue.ClosedUnix >= issue.DeadlineUnix
	}
	return timeutil.TimeStampNow() >= issue.DeadlineUnix
}

// LoadRepo loads issue's repository
func (issue *Issue) LoadRepo(ctx context.Context) (err error) {
	if issue.Repo == nil && issue.RepoID != 0 {
		issue.Repo, err = repo_model.GetRepositoryByID(ctx, issue.RepoID)
		if err != nil {
			return fmt.Errorf("getRepositoryByID [%d]: %w", issue.RepoID, err)
		}
	}
	return nil
}

// LoadPoster loads poster
func (issue *Issue) LoadPoster(ctx context.Context) (err error) {
	if issue.Poster != nil {
		return nil
	}
	issue.PosterID, issue.Poster, err = user_model.GetPossibleUserByID(ctx, issue.PosterID)
	return err
}

// LoadPullRequest loads pull request info
func (issue *Issue) LoadPullRequest(ctx context.Context) (err error) {
	if issue.IsPull {
		if issue.PullRequest == nil && issue.ID != 0 {
			issue.PullRequest, err = GetPullRequestByIssueID(ctx, issue.ID)
			if err != nil {
				if IsErrPullRequestNotExist(err) {
					return err
				}
				return fmt.Errorf("getPullRequestByIssueID [%d]: %w", issue.ID, err)
			}
		}
		if issue.PullRequest != nil {
			issue.PullRequest.Issue = issue
		}
	}
	return nil
}

func (issue *Issue) loadComments(ctx context.Context) (err error) {
	return issue.loadCommentsByType(ctx, CommentTypeUndefined)
}

// LoadDiscussComments loads discuss comments
func (issue *Issue) LoadDiscussComments(ctx context.Context) error {
	return issue.loadCommentsByType(ctx, CommentTypeComment)
}

func (issue *Issue) loadCommentsByType(ctx context.Context, tp CommentType) (err error) {
	if issue.Comments != nil {
		return nil
	}
	issue.Comments, err = FindComments(ctx, &FindCommentsOptions{
		IssueID: issue.ID,
		Type:    tp,
	})
	for _, comment := range issue.Comments {
		comment.Issue = issue
	}
	return err
}

func (issue *Issue) loadReactions(ctx context.Context) (err error) {
	if issue.Reactions != nil {
		return nil
	}
	reactions, _, err := FindReactions(ctx, FindReactionsOptions{
		IssueID: issue.ID,
	})
	if err != nil {
		return err
	}
	if err = issue.LoadRepo(ctx); err != nil {
		return err
	}
	// Load reaction user data
	if _, err := reactions.LoadUsers(ctx, issue.Repo); err != nil {
		return err
	}

	// Cache comments to map
	comments := make(map[int64]*Comment)
	for _, comment := range issue.Comments {
		comments[comment.ID] = comment
	}
	// Add reactions either to issue or comment
	for _, react := range reactions {
		if react.CommentID == 0 {
			issue.Reactions = append(issue.Reactions, react)
		} else if comment, ok := comments[react.CommentID]; ok {
			comment.Reactions = append(comment.Reactions, react)
		}
	}
	return nil
}

// LoadAttributes loads the attribute of this issue.
func (issue *Issue) LoadAttributes(ctx context.Context) (err error) {
	if err = issue.LoadRepo(ctx); err != nil {
		return err
	}

	if err = issue.LoadPoster(ctx); err != nil {
		return err
	}

	if err = issue.LoadLabels(ctx); err != nil {
		return err
	}

	if err = issue.LoadAssignees(ctx); err != nil {
		return err
	}

	if err = issue.LoadPullRequest(ctx); err != nil && !IsErrPullRequestNotExist(err) {
		// It is possible pull request is not yet created.
		return err
	}

	if err = issue.loadComments(ctx); err != nil {
		return err
	}

	if err = issue.Comments.LoadAttributes(ctx); err != nil {
		return err
	}
	return issue.loadReactions(ctx)
}

func (issue *Issue) ResetAttributesLoaded() {
	issue.isLabelsLoaded = false
	issue.isAssigneeLoaded = false
}

// GetIsRead load the `IsRead` field of the issue
func (issue *Issue) GetIsRead(ctx context.Context, userID int64) error {
	issueUser, has, err := db.Get[IssueUser](ctx, builder.Eq{"issue_id": issue.ID, "uid": userID})
	if err != nil {
		return err
	} else if !has {
		issue.IsRead = false
		return nil
	}
	issue.IsRead = issueUser.IsRead
	return nil
}

// APIURL returns the absolute APIURL to this issue.
func (issue *Issue) APIURL(ctx context.Context) string {
	if issue.Repo == nil {
		err := issue.LoadRepo(ctx)
		if err != nil {
			log.Error("Issue[%d].APIURL(): %v", issue.ID, err)
			return ""
		}
	}
	return fmt.Sprintf("%s/issues/%d", issue.Repo.APIURL(), issue.Index)
}

// HTMLURL returns the absolute URL to this issue.
func (issue *Issue) HTMLURL(ctx context.Context) string {
	var path string
	if issue.IsPull {
		path = "pulls"
	} else {
		path = "issues"
	}
	return fmt.Sprintf("%s/%s/%d", issue.Repo.HTMLURL(ctx), path, issue.Index)
}

// Link returns the issue's relative URL.
func (issue *Issue) Link() string {
	var path string
	if issue.IsPull {
		path = "pulls"
	} else {
		path = "issues"
	}
	return fmt.Sprintf("%s/%s/%d", issue.Repo.Link(), path, issue.Index)
}

// DiffURL returns the absolute URL to this diff
func (issue *Issue) DiffURL() string {
	if issue.IsPull {
		return fmt.Sprintf("%s/pulls/%d.diff", issue.Repo.APIURL(), issue.Index) // the web diff route is gone
	}
	return ""
}

// PatchURL returns the absolute URL to this patch
func (issue *Issue) PatchURL() string {
	if issue.IsPull {
		return fmt.Sprintf("%s/pulls/%d.patch", issue.Repo.APIURL(), issue.Index) // the web patch route is gone
	}
	return ""
}

// State returns string representation of issue status.
func (issue *Issue) State() api.StateType {
	if issue.IsClosed {
		return api.StateClosed
	}
	return api.StateOpen
}

// HashTag returns unique hash tag for issue.
func (issue *Issue) HashTag() string {
	return fmt.Sprintf("issue-%d", issue.ID)
}

// IsPoster returns true if given user by ID is the poster.
func (issue *Issue) IsPoster(uid int64) bool {
	return issue.OriginalAuthorID == 0 && issue.PosterID == uid
}

// GetTasks returns the amount of tasks in the issues content
func (issue *Issue) GetTasks() int {
	return len(issueTasksPat.FindAllStringIndex(issue.Content, -1))
}

// GetTasksDone returns the amount of completed tasks in the issues content
func (issue *Issue) GetTasksDone() int {
	return len(issueTasksDonePat.FindAllStringIndex(issue.Content, -1))
}

// GetLastEventTimestamp returns the last user visible event timestamp, either the creation of this issue or the close.
func (issue *Issue) GetLastEventTimestamp() timeutil.TimeStamp {
	if issue.IsClosed {
		return issue.ClosedUnix
	}
	return issue.CreatedUnix
}

// GetLastEventLabel returns the localization label for the current issue.
func (issue *Issue) GetLastEventLabel() string {
	if issue.IsClosed {
		if issue.IsPull && issue.PullRequest.HasMerged {
			return "repo.pulls.merged_by"
		}
		return "repo.issues.closed_by"
	}
	return "repo.issues.opened_by"
}

// GetLastComment return last comment for the current issue.
func (issue *Issue) GetLastComment(ctx context.Context) (*Comment, error) {
	var c Comment
	exist, err := db.GetEngine(ctx).Where("type = ?", CommentTypeComment).
		And("issue_id = ?", issue.ID).Desc("created_unix").Get(&c)
	if err != nil {
		return nil, err
	}
	if !exist {
		return nil, nil //nolint:nilnil // return nil to indicate that the object does not exist
	}
	return &c, nil
}

// GetLastEventLabelFake returns the localization label for the current issue without providing a link in the username.
func (issue *Issue) GetLastEventLabelFake() string {
	if issue.IsClosed {
		if issue.IsPull && issue.PullRequest.HasMerged {
			return "repo.pulls.merged_by_fake"
		}
		return "repo.issues.closed_by_fake"
	}
	return "repo.issues.opened_by_fake"
}

// GetIssueByIndex returns raw issue without loading attributes by index in a repository.
func GetIssueByIndex(ctx context.Context, repoID, index int64) (*Issue, error) {
	if index < 1 {
		return nil, ErrIssueNotExist{}
	}
	issue, has, err := db.Get[Issue](ctx, builder.Eq{"repo_id": repoID, "`index`": index})
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrIssueNotExist{0, repoID, index}
	}
	return issue, nil
}

// GetIssueWithAttrsByIndex returns issue by index in a repository.
func GetIssueWithAttrsByIndex(ctx context.Context, repoID, index int64) (*Issue, error) {
	issue, err := GetIssueByIndex(ctx, repoID, index)
	if err != nil {
		return nil, err
	}
	return issue, issue.LoadAttributes(ctx)
}

// GetIssueByID returns an issue by given ID.
func GetIssueByID(ctx context.Context, id int64) (*Issue, error) {
	issue := new(Issue)
	has, err := db.GetEngine(ctx).ID(id).Get(issue)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrIssueNotExist{id, 0, 0}
	}
	return issue, nil
}

// GetIssuesByIDs return issues with the given IDs.
// If keepOrder is true, the order of the returned issues will be the same as the given IDs.
func GetIssuesByIDs(ctx context.Context, issueIDs []int64, keepOrder ...bool) (IssueList, error) {
	issues := make([]*Issue, 0, len(issueIDs))
	if len(issueIDs) == 0 {
		return issues, nil
	}

	if err := db.GetEngine(ctx).In("id", issueIDs).Find(&issues); err != nil {
		return nil, err
	}

	if len(keepOrder) > 0 && keepOrder[0] {
		m := make(map[int64]*Issue, len(issues))
		appended := container.Set[int64]{}
		for _, issue := range issues {
			m[issue.ID] = issue
		}
		issues = issues[:0]
		for _, id := range issueIDs {
			if issue, ok := m[id]; ok && !appended.Contains(id) { // make sure the id is existed and not appended
				appended.Add(id)
				issues = append(issues, issue)
			}
		}
	}

	return issues, nil
}

// GetParticipantIDsByIssue returns all userIDs who are participated in comments of an issue and issue author
func (issue *Issue) GetParticipantIDsByIssue(ctx context.Context) ([]int64, error) {
	if issue == nil {
		return nil, nil
	}
	userIDs := make([]int64, 0, 5)
	if err := db.GetEngine(ctx).Table("comment").Cols("poster_id").
		Where("`comment`.issue_id = ?", issue.ID).
		And("`comment`.type in (?,?,?)", CommentTypeComment, CommentTypeCode, CommentTypeReview).
		And("`user`.is_active = ?", true).
		And("`user`.prohibit_login = ?", false).
		Join("INNER", "`user`", "`user`.id = `comment`.poster_id").
		Distinct("poster_id").
		Find(&userIDs); err != nil {
		return nil, fmt.Errorf("get poster IDs: %w", err)
	}
	if !slices.Contains(userIDs, issue.PosterID) {
		return append(userIDs, issue.PosterID), nil
	}
	return userIDs, nil
}

// RemapExternalUser ExternalUserRemappable interface
func (issue *Issue) RemapExternalUser(externalName string, externalID, userID int64) error {
	issue.OriginalAuthor = externalName
	issue.OriginalAuthorID = externalID
	issue.PosterID = userID
	return nil
}

// GetUserID ExternalUserRemappable interface
func (issue *Issue) GetUserID() int64 { return issue.PosterID }

// GetExternalName ExternalUserRemappable interface
func (issue *Issue) GetExternalName() string { return issue.OriginalAuthor }

// GetExternalID ExternalUserRemappable interface
func (issue *Issue) GetExternalID() int64 { return issue.OriginalAuthorID }

// HasOriginalAuthor returns if an issue was migrated and has an original author.
func (issue *Issue) HasOriginalAuthor() bool {
	return issue.OriginalAuthor != "" && issue.OriginalAuthorID != 0
}

// InsertIssues insert issues to database
func InsertIssues(ctx context.Context, issues ...*Issue) error {
	return db.WithTx(ctx, func(ctx context.Context) error {
		for _, issue := range issues {
			if err := insertIssue(ctx, issue); err != nil {
				return err
			}
		}
		return nil
	})
}

func insertIssue(ctx context.Context, issue *Issue) error {
	sess := db.GetEngine(ctx)
	if _, err := sess.NoAutoTime().Insert(issue); err != nil {
		return err
	}
	issueLabels := make([]IssueLabel, 0, len(issue.Labels))
	for _, label := range issue.Labels {
		issueLabels = append(issueLabels, IssueLabel{
			IssueID: issue.ID,
			LabelID: label.ID,
		})
	}
	if len(issueLabels) > 0 {
		if _, err := sess.Insert(issueLabels); err != nil {
			return err
		}
	}

	for _, reaction := range issue.Reactions {
		reaction.IssueID = issue.ID
	}

	if len(issue.Reactions) > 0 {
		if _, err := sess.Insert(issue.Reactions); err != nil {
			return err
		}
	}

	return nil
}
