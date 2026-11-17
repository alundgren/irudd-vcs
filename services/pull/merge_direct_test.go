// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package pull

import (
	"testing"

	"gitea.dev/modules/git/gitcmd"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStripCommitMessage(t *testing.T) {
	for _, message := range []string{
		"Merge pull request 'x' (#1) from a into main",
		"subject\n\nbody",
		"\n\n  subject  \t\r\n\n\n\nbody line \n# not a comment here\n\n\n",
		"subject\r\n\r\nbody\r\n",
		"  leading spaces stay\n\t\n",
		"   \n\t\n",
		"",
		"\vvertical\f",
	} {
		// git stripspace cleans up as git commit does for --message
		want, _, err := gitcmd.NewCommand("stripspace").WithStdinBytes([]byte(message)).RunStdString(t.Context())
		require.NoError(t, err)
		assert.Equal(t, want, stripCommitMessage(message), "%q", message)
	}
}
