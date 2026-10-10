package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

// Extraction pages are read and deleted only through the protected StorageKey.
// A row carrying just the legacy FilePath column must not make the worker read
// or delete local files.
func TestMenuExtractionAssetsIgnoreLegacyFilePath(t *testing.T) {
	dir := t.TempDir()
	page := filepath.Join(dir, "page.jpg")
	require.NoError(t, os.WriteFile(page, []byte("not for the extractor"), 0o600))
	images := []database.MenuExtractionImage{{FilePath: page, PageOrder: 0, MIMEType: "image/jpeg"}}

	_, err := loadMenuExtractionInputs(images)
	require.ErrorContains(t, err, "has no storage identity")

	cleanupMenuExtractionAssets(images)
	_, err = os.Stat(page)
	require.NoError(t, err, "cleanup removed a legacy local file")
	_, err = os.Stat(dir)
	require.NoError(t, err, "cleanup removed the legacy file's directory")
}
