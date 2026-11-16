// Copyright 2020 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package base

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncodeSha256(t *testing.T) {
	assert.Equal(t,
		"c3ab8ff13720e8ad9047dd39466b3c8974e592c2fa383d4a3960714caef0c4f2",
		EncodeSha256("foobar"),
	)
}

func TestShortSha(t *testing.T) {
	assert.Equal(t, "veryverylo", ShortSha("veryverylong"))
}

func TestFileSize(t *testing.T) {
	var size int64 = 512
	assert.Equal(t, "512 B", FileSize(size))
	size *= 1024
	assert.Equal(t, "512 KiB", FileSize(size))
	size *= 1024
	assert.Equal(t, "512 MiB", FileSize(size))
	size *= 1024
	assert.Equal(t, "512 GiB", FileSize(size))
	size *= 1024
	assert.Equal(t, "512 TiB", FileSize(size))
	size *= 1024
	assert.Equal(t, "512 PiB", FileSize(size))
	size *= 4
	assert.Equal(t, "2.0 EiB", FileSize(size))
}

func TestStringsToInt64s(t *testing.T) {
	testSuccess := func(input []string, expected []int64) {
		result, err := StringsToInt64s(input)
		assert.NoError(t, err)
		assert.Equal(t, expected, result)
	}
	testSuccess(nil, nil)
	testSuccess([]string{}, []int64{})
	testSuccess([]string{""}, []int64{})
	testSuccess([]string{"-1234"}, []int64{-1234})
	testSuccess([]string{"1", "4", "16", "64", "256"}, []int64{1, 4, 16, 64, 256})

	ints, err := StringsToInt64s([]string{"-1", "a"})
	assert.Empty(t, ints)
	assert.Error(t, err)
}

func TestInt64sToStrings(t *testing.T) {
	assert.Equal(t, []string{}, Int64sToStrings([]int64{}))
	assert.Equal(t,
		[]string{"1", "4", "16", "64", "256"},
		Int64sToStrings([]int64{1, 4, 16, 64, 256}),
	)
}
