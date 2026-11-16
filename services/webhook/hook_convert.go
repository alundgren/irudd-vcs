// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package webhook

import (
	"fmt"

	webhook_model "gitea.dev/models/webhook"
	api "gitea.dev/modules/structs"
)

// ToHook convert models.Webhook to api.Hook
// This function is not part of the convert package to prevent an import cycle
func ToHook(repoLink string, w *webhook_model.Webhook) (*api.Hook, error) {
	config := map[string]string{
		"url":          w.URL,
		"content_type": w.ContentType.Name(),
	}
	return &api.Hook{
		ID:     w.ID,
		Name:   w.Name,
		Type:   w.Type,
		URL:    fmt.Sprintf("%s/settings/hooks/%d", repoLink, w.ID),
		Active: w.IsActive,
		Config: config,
		Events: w.EventsArray(),
		// the stored authorization header is a secret and must never be returned by the API,
		// consistent with the webhook secret which is also omitted from the response
		AuthorizationHeader: "",
		Updated:             w.UpdatedUnix.AsTime(),
		Created:             w.CreatedUnix.AsTime(),
		BranchFilter:        w.BranchFilter,
	}, nil
}
