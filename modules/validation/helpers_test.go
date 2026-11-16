// Copyright 2018 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_IsValidURL(t *testing.T) {
	cases := []struct {
		description string
		url         string
		valid       bool
	}{
		{
			description: "Empty URL",
			url:         "",
			valid:       false,
		},
		{
			description: "Loopback IPv4 URL",
			url:         "http://127.0.1.1:5678/",
			valid:       true,
		},
		{
			description: "Loopback IPv6 URL",
			url:         "https://[::1]/",
			valid:       true,
		},
		{
			description: "Missing semicolon after schema",
			url:         "http//meh/",
			valid:       false,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.description, func(t *testing.T) {
			assert.Equal(t, testCase.valid, IsValidURL(testCase.url))
		})
	}
}

func TestIsValidUsername(t *testing.T) {
	tests := []struct {
		arg  string
		want bool
	}{
		{arg: "a", want: true},
		{arg: "abc", want: true},
		{arg: "0.b-c", want: true},
		{arg: "a.b-c_d", want: true},
		{arg: "", want: false},
		{arg: ".abc", want: false},
		{arg: "abc.", want: false},
		{arg: "a..bc", want: false},
		{arg: "a...bc", want: false},
		{arg: "a.-bc", want: false},
		{arg: "a._bc", want: false},
		{arg: "a_-bc", want: false},
		{arg: "a/bc", want: false},
		{arg: "☁️", want: false},
		{arg: "-", want: false},
		{arg: "--diff", want: false},
		{arg: "-im-here", want: false},
		{arg: "a space", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			assert.Equalf(t, tt.want, IsValidUsername(tt.arg), "IsValidUsername(%v)", tt.arg)
		})
	}
}
