// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegisterModelInitFunc(t *testing.T) {
	models, initFuncs := registeredModels, registeredInitFuncs
	defer func() { registeredModels, registeredInitFuncs = models, initFuncs }()
	registeredInitFuncs = nil

	type withInit struct{ ID int64 }
	type withoutInit struct{ ID int64 }
	RegisterModel(new(withInit), func() error { return nil })
	RegisterModel(new(withoutInit))
	assert.Len(t, registeredModels, len(models)+2)
	assert.Len(t, registeredInitFuncs, 1, "the first init func is kept even with none registered before")
}
