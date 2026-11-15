// Copyright 2025 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package attribute

import (
	"gitea.dev/modules/optional"
)

type Attribute string

const (
	Lockable = "lockable"
	Filter   = "filter"
	Diff     = "diff"
)

func (a Attribute) IsUnspecified() bool {
	return a == "" || a == "unspecified"
}

func (a Attribute) ToString() optional.Option[string] {
	if !a.IsUnspecified() {
		return optional.Some(string(a))
	}
	return optional.None[string]()
}

// ToBool converts the attribute value to optional boolean: true if "set"/"true", false if "unset"/"false", none otherwise
func (a Attribute) ToBool() optional.Option[bool] {
	switch a {
	case "set", "true":
		return optional.Some(true)
	case "unset", "false":
		return optional.Some(false)
	}
	return optional.None[bool]()
}

type Attributes struct {
	m map[string]Attribute
}

func NewAttributes() *Attributes {
	return &Attributes{m: make(map[string]Attribute)}
}

func (attrs *Attributes) Get(name string) Attribute {
	if value, has := attrs.m[name]; has {
		return value
	}
	return ""
}
