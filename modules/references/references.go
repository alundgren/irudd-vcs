// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package references

import (
	"regexp"
)

var (
	// mentionPattern matches all mentions in the form of "@user" or "@org/team"
	mentionPattern = regexp.MustCompile(`(?:\s|^|\(|\[)(@[-\w][-.\w]*?|@[-\w][-.\w]*?/[-\w][-.\w]*?)(?:\s|$|[:,;.?!](\s|$)|'|\)|\])`)
	// spaceTrimmedPattern let's find the trailing space
	spaceTrimmedPattern = regexp.MustCompile(`(?:.*[0-9a-zA-Z-_])\s`)

	actionStrings = []string{
		"none",
		"closes",
		"reopens",
		"neutered",
	}
)

// XRefAction represents the kind of effect a cross reference has once is resolved
// Cross-references are no longer created, the type stays for existing comment rows
type XRefAction int64

const (
	// XRefActionNone means the cross-reference is simply a comment
	XRefActionNone XRefAction = iota // 0
	// XRefActionCloses means the cross-reference should close an issue if it is resolved
	XRefActionCloses // 1
	// XRefActionReopens means the cross-reference should reopen an issue if it is resolved
	XRefActionReopens // 2
	// XRefActionNeutered means the cross-reference will no longer affect the source
	XRefActionNeutered // 3
)

func (a XRefAction) String() string {
	return actionStrings[a]
}

// RefSpan is the position where the reference was found within the parsed text
type RefSpan struct {
	Start int
	End   int
}

// FindAllMentionsMarkdown matches mention patterns in given content and
// returns a list of found unvalidated user names **not including** the @ prefix.
// The content is scanned as plain text, markdown is not stripped first.
func FindAllMentionsMarkdown(content string) []string {
	bcontent := []byte(content)
	locations := FindAllMentionsBytes(bcontent)
	mentions := make([]string, len(locations))
	for i, val := range locations {
		mentions[i] = string(bcontent[val.Start+1 : val.End])
	}
	return mentions
}

// FindAllMentionsBytes matches mention patterns in given content
// and returns a list of locations for the unvalidated user names, including the @ prefix.
func FindAllMentionsBytes(content []byte) []RefSpan {
	// Sadly we can't use FindAllSubmatchIndex because our pattern checks for starting and
	// trailing spaces (\s@mention,\s), so if we get two consecutive references, the space
	// from the second reference will be "eaten" by the first one:
	// ...\s@mention1\s@mention2\s...	--> ...`\s@mention1\s`, (not) `@mention2,\s...`
	ret := make([]RefSpan, 0, 5)
	pos := 0
	for {
		match := mentionPattern.FindSubmatchIndex(content[pos:])
		if match == nil {
			break
		}
		ret = append(ret, RefSpan{Start: match[2] + pos, End: match[3] + pos})
		notrail := spaceTrimmedPattern.FindSubmatchIndex(content[match[2]+pos : match[3]+pos])
		if notrail == nil {
			pos = match[3] + pos
		} else {
			pos = match[3] + pos + notrail[1] - notrail[3]
		}
	}
	return ret
}
