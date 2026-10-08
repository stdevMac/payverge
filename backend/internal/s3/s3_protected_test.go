package s3

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteFileProtected_RejectsEmptyKey(t *testing.T) {
	err := DeleteFileProtected("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestDeleteFileProtected_RequiresInitializedClient(t *testing.T) {
	// When the protected client is nil (unit-test default), callers must get a
	// clear configuration error rather than a nil-pointer panic.
	restore := SetStores(PublicStore(), nil)
	t.Cleanup(restore)

	err := DeleteFileProtected("ai/menu-extraction/1/1/page.png")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}

func TestExtractKeyFromURL_StripsHost(t *testing.T) {
	key, err := extractKeyFromURL("https://protected.example/ai/menu-extraction/1/2/page.webp")
	require.NoError(t, err)
	assert.Equal(t, "ai/menu-extraction/1/2/page.webp", key)
}
