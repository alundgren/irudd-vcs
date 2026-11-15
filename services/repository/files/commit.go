// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package files

import (
	"context"

	"gitea.dev/modules/git"
	"gitea.dev/modules/structs"
)

// GetPayloadCommitVerification returns the verification information of a commit
// Signatures are passed through but never verified, there are no stored GPG or SSH keys
func GetPayloadCommitVerification(_ context.Context, commit *git.Commit) *structs.PayloadCommitVerification {
	verification := &structs.PayloadCommitVerification{Reason: "gpg.error.not_signed_commit"}
	if commit.Signature != nil {
		verification.Reason = "signature verification is not supported"
		verification.Signature = commit.Signature.Signature
		verification.Payload = commit.Signature.Payload
	}
	return verification
}
