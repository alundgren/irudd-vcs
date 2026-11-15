// Copyright 2021 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package unittest

import (
	"context"
	"reflect"
	"strconv"
	"strings"

	"gitea.dev/models/db"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"xorm.io/builder"
)

const (
	// these const values are copied from `models` package to prevent from cycle-import
	modelsUserTypeOrganization = 1
	modelsCommentTypeComment   = 0
)

var consistencyCheckMap = make(map[string]func(t TestingT, bean any))

// CheckConsistencyFor test that all matching database entries are consistent
func CheckConsistencyFor(t TestingT, beansToCheck ...any) {
	for _, bean := range beansToCheck {
		sliceType := reflect.SliceOf(reflect.TypeOf(bean))
		sliceValue := reflect.MakeSlice(sliceType, 0, 10)

		ptrToSliceValue := reflect.New(sliceType)
		ptrToSliceValue.Elem().Set(sliceValue)

		assert.NoError(t, db.GetEngine(context.TODO()).Table(bean).Find(ptrToSliceValue.Interface()))
		sliceValue = ptrToSliceValue.Elem()

		for i := 0; i < sliceValue.Len(); i++ {
			entity := sliceValue.Index(i).Interface()
			checkForConsistency(t, entity)
		}
	}
}

func checkForConsistency(t TestingT, bean any) {
	tb, err := GetXORMEngine().TableInfo(bean)
	assert.NoError(t, err)
	f := consistencyCheckMap[tb.Name]
	require.NotNil(t, f, "unknown bean type: %#v", bean)
	f(t, bean)
}

func init() {
	parseBool := func(v string) bool {
		b, _ := strconv.ParseBool(v)
		return b
	}
	parseInt := func(v string) int {
		i, _ := strconv.Atoi(v)
		return i
	}

	checkForUserConsistency := func(t TestingT, bean any) {
		user := reflectionWrap(bean)
		AssertCountByCond(t, "repository", builder.Eq{"owner_id": user.int("ID")}, user.int("NumRepos"))
		AssertCountByCond(t, "org_user", builder.Eq{"org_id": user.int("ID")}, user.int("NumMembers"))
		AssertCountByCond(t, "team", builder.Eq{"org_id": user.int("ID")}, user.int("NumTeams"))
		if user.int("Type") != modelsUserTypeOrganization {
			assert.Equal(t, 0, user.int("NumMembers"), "Unexpected number of members for user id: %d", user.int("ID"))
			assert.Equal(t, 0, user.int("NumTeams"), "Unexpected number of teams for user id: %d", user.int("ID"))
		}
	}

	checkForRepoConsistency := func(t TestingT, bean any) {
		repo := reflectionWrap(bean)
		assert.Equal(t, repo.str("LowerName"), strings.ToLower(repo.str("Name")), "repo: %+v", repo)
		AssertCountByCond(t, "repository", builder.Eq{"fork_id": repo.int("ID")}, repo.int("NumForks"))
		if repo.bool("IsFork") {
			AssertExistsAndLoadMap(t, "repository", builder.Eq{"id": repo.int("ForkID")})
		}

		actual := GetCountByCond(t, "issue", builder.Eq{"is_pull": false, "repo_id": repo.int("ID")})
		assert.EqualValues(t, repo.int("NumIssues"), actual,
			"Unexpected number of issues for repo id: %d", repo.int("ID"))

		actual = GetCountByCond(t, "issue", builder.Eq{"is_pull": false, "is_closed": true, "repo_id": repo.int("ID")})
		assert.EqualValues(t, repo.int("NumClosedIssues"), actual,
			"Unexpected number of closed issues for repo id: %d", repo.int("ID"))

		actual = GetCountByCond(t, "issue", builder.Eq{"is_pull": true, "repo_id": repo.int("ID")})
		assert.EqualValues(t, repo.int("NumPulls"), actual,
			"Unexpected number of pulls for repo id: %d", repo.int("ID"))

		actual = GetCountByCond(t, "issue", builder.Eq{"is_pull": true, "is_closed": true, "repo_id": repo.int("ID")})
		assert.EqualValues(t, repo.int("NumClosedPulls"), actual,
			"Unexpected number of closed pulls for repo id: %d", repo.int("ID"))
	}

	checkForIssueConsistency := func(t TestingT, bean any) {
		issue := reflectionWrap(bean)
		typeComment := modelsCommentTypeComment
		actual := GetCountByCond(t, "comment", builder.Eq{"`type`": typeComment, "issue_id": issue.int("ID")})
		assert.EqualValues(t, issue.int("NumComments"), actual, "Unexpected number of comments for issue id: %d", issue.int("ID"))
		if issue.bool("IsPull") {
			prRow := AssertExistsAndLoadMap(t, "pull_request", builder.Eq{"issue_id": issue.int("ID")})
			assert.Equal(t, parseInt(prRow["index"]), issue.int("Index"), "Unexpected index for issue id: %d", issue.int("ID"))
		}
	}

	checkForPullRequestConsistency := func(t TestingT, bean any) {
		pr := reflectionWrap(bean)
		issueRow := AssertExistsAndLoadMap(t, "issue", builder.Eq{"id": pr.int("IssueID")})
		assert.True(t, parseBool(issueRow["is_pull"]))
		assert.Equal(t, parseInt(issueRow["index"]), pr.int("Index"), "Unexpected index for pull request id: %d", pr.int("ID"))
	}

	checkForLabelConsistency := func(t TestingT, bean any) {
		label := reflectionWrap(bean)
		issueLabels, err := db.GetEngine(context.TODO()).Table("issue_label").
			Where(builder.Eq{"label_id": label.int("ID")}).
			Query()
		assert.NoError(t, err)

		assert.Len(t, issueLabels, label.int("NumIssues"), "Unexpected number of issue for label id: %d", label.int("ID"))

		issueIDs := make([]int, len(issueLabels))
		for i, issueLabel := range issueLabels {
			issueIDs[i], _ = strconv.Atoi(string(issueLabel["issue_id"]))
		}

		expected := int64(0)
		if len(issueIDs) > 0 {
			expected = GetCountByCond(t, "issue", builder.In("id", issueIDs).And(builder.Eq{"is_closed": true}))
		}
		assert.EqualValues(t, expected, label.int("NumClosedIssues"), "Unexpected number of closed issues for label id: %d", label.int("ID"))
	}

	checkForTeamConsistency := func(t TestingT, bean any) {
		team := reflectionWrap(bean)
		AssertCountByCond(t, "team_user", builder.Eq{"team_id": team.int("ID")}, team.int("NumMembers"))
		AssertCountByCond(t, "team_repo", builder.Eq{"team_id": team.int("ID")}, team.int("NumRepos"))
	}

	consistencyCheckMap["user"] = checkForUserConsistency
	consistencyCheckMap["repository"] = checkForRepoConsistency
	consistencyCheckMap["issue"] = checkForIssueConsistency
	consistencyCheckMap["pull_request"] = checkForPullRequestConsistency
	consistencyCheckMap["label"] = checkForLabelConsistency
	consistencyCheckMap["team"] = checkForTeamConsistency
}
