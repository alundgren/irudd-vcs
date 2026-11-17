// Copyright 2019 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package git

import (
	"encoding/binary"
	"slices"
	"strings"
	"testing"
	"time"

	"gitea.dev/modules/test"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSignatureFromCommitLine(t *testing.T) {
	tests := []struct {
		line string
		want *Signature
	}{
		{
			line: "a b <c@d.com> 12345 +0100",
			want: &Signature{
				Name:  "a b",
				Email: "c@d.com",
				When:  time.Unix(12345, 0).In(time.FixedZone("", 3600)),
			},
		},
		{
			line: "bad line",
			want: &Signature{Name: "bad line"},
		},
		{
			line: "bad < line",
			want: &Signature{Name: "bad < line"},
		},
		{
			line: "bad > line",
			want: &Signature{Name: "bad > line"},
		},
		{
			line: "bad-line <name@example.com>",
			want: &Signature{Name: "bad-line <name@example.com>"},
		},
	}
	for _, test := range tests {
		got := parseSignatureFromCommitLine(test.line)
		assert.Equal(t, test.want, got)
	}

	t.Run("KeepsOffsetOutsideLocalDST", func(t *testing.T) {
		// a CET zone without history, as Windows provides: time.Parse matches +0100 to it
		defer test.MockVariableValue(&time.Local, cetWithoutHistory(t))()
		// 2024-07-01, when the zone is at +0200 but the commit says +0100
		got := parseSignatureFromCommitLine("a b <c@d.com> 1719792000 +0100")
		_, offset := got.When.Zone()
		assert.Equal(t, 3600, offset)
		assert.Equal(t, "2024-07-01T01:00:00+01:00", got.When.Format(time.RFC3339))
	})
}

// cetWithoutHistory builds a TZif zone that is CET/CEST at every date, back to year 0
func cetWithoutHistory(t *testing.T) *time.Location {
	header := []byte("TZif2" + strings.Repeat("\x00", 15))
	for _, n := range []uint32{0, 0, 0, 0, 1, 4} { // isutcnt, isstdcnt, leapcnt, timecnt, typecnt, charcnt
		header = binary.BigEndian.AppendUint32(header, n)
	}
	block := slices.Concat(header, []byte{0, 0, 0x0e, 0x10, 0, 0}, []byte("CET\x00")) // one type: UTC+3600, not DST, abbreviation 0
	data := slices.Concat(block, block)                                               // the version 1 and version 2 blocks are the same without transitions
	data = append(data, "\nCET-1CEST,M3.5.0,M10.5.0/3\n"...)
	loc, err := time.LoadLocationFromTZData("CET", data)
	require.NoError(t, err)
	return loc
}
