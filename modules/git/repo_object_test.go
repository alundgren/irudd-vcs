// Copyright 2026 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObjectFormatFromBareConfig(t *testing.T) {
	for _, format := range DefaultFeatures().SupportedObjectFormats {
		t.Run(format.Name(), func(t *testing.T) {
			bare := filepath.Join(t.TempDir(), "repo.git")
			require.NoError(t, InitRepository(t.Context(), bare, true, format.Name()))
			assert.Equal(t, format, objectFormatFromBareConfig(bare))

			repo, err := OpenRepository(t.Context(), bare)
			require.NoError(t, err)
			defer repo.Close()
			got, err := repo.GetObjectFormat()
			require.NoError(t, err)
			assert.Equal(t, format, got)
		})
	}

	t.Run("NotBare", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, InitRepository(t.Context(), dir, false, Sha1ObjectFormat.Name()))
		// a work tree may hold a file named config, so only bare repositories are read
		require.NoError(t, os.WriteFile(filepath.Join(dir, "config"), []byte("[extensions]\nobjectformat = sha256\n"), 0o644))
		assert.Nil(t, objectFormatFromBareConfig(dir))
	})

	t.Run("Unknown", func(t *testing.T) {
		bare := filepath.Join(t.TempDir(), "repo.git")
		require.NoError(t, InitRepository(t.Context(), bare, true, Sha1ObjectFormat.Name()))
		require.NoError(t, os.WriteFile(filepath.Join(bare, "config"), []byte("[core]\n\tbare = true\n[Extensions]\n\tObjectFormat = \"sha512\"\n"), 0o644))
		assert.Nil(t, objectFormatFromBareConfig(bare))
	})
}
