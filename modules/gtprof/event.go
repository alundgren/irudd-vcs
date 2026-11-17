// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package gtprof

type EventConfig struct{}

type EventOption interface {
	applyEvent(*EventConfig)
}

func eventConfigFromOptions(options ...EventOption) *EventConfig {
	cfg := &EventConfig{}
	for _, opt := range options {
		opt.applyEvent(cfg)
	}
	return cfg
}
