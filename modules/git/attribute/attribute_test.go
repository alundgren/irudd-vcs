// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package attribute

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Attribute(t *testing.T) {
	assert.Empty(t, Attribute("").ToString().Value())
	assert.Empty(t, Attribute("unspecified").ToString().Value())
	assert.Equal(t, "python", Attribute("python").ToString().Value())
	assert.Equal(t, "Java", Attribute("Java").ToString().Value())

	attributes := Attributes{
		m: map[string]Attribute{
			"linguist-generated":     "true",
			"linguist-documentation": "false",
			"linguist-detectable":    "set",
			"linguist-language":      "Python",
			"gitlab-language":        "Java",
			"filter":                 "unspecified",
			"test":                   "",
		},
	}

	assert.Empty(t, attributes.Get("test").ToString().Value())
	assert.Empty(t, attributes.Get("filter").ToString().Value())
	assert.Equal(t, "Python", attributes.Get("linguist-language").ToString().Value())
	assert.Equal(t, "Java", attributes.Get("gitlab-language").ToString().Value())
	assert.True(t, attributes.Get("linguist-generated").ToBool().Value())
	assert.False(t, attributes.Get("linguist-documentation").ToBool().Value())
	assert.True(t, attributes.Get("linguist-detectable").ToBool().Value())
}
