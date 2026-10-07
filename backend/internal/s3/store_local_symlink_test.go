package s3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLocalStoreSymlinkEscapes: a symlink planted in the store tree (restored
// backup, another process with volume access) must never let the local driver
// read, serve, overwrite or delete a file outside its root. This includes the
// protected store next to the public one.
func TestLocalStoreSymlinkEscapes(t *testing.T) {
	ctx := context.Background()
	storageDir := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("outside the store"), 0o600))

	pub, err := NewLocalStore(filepath.Join(storageDir, "public"))
	require.NoError(t, err)
	prot, err := NewLocalStore(filepath.Join(storageDir, "protected"))
	require.NoError(t, err)
	require.NoError(t, prot.Put(ctx, "businesses/1/ledger/receipt.pdf", strings.NewReader("protected bytes"), PutOptions{ContentType: "application/pdf"}))

	objects := filepath.Join(pub.Root(), localObjectsDir)
	// Final component is a symlink to a file outside the store.
	require.NoError(t, os.Symlink(secret, filepath.Join(objects, "leak.png")))
	// Relative symlink from public into the protected store.
	require.NoError(t, os.Symlink(filepath.Join("..", "..", "protected", localObjectsDir, "businesses", "1", "ledger", "receipt.pdf"), filepath.Join(objects, "receipt.png")))
	// Directory component is a symlink to a directory outside the store.
	require.NoError(t, os.Symlink(outside, filepath.Join(objects, "biz")))

	for _, key := range []string{"leak.png", "receipt.png", "biz/secret.txt"} {
		_, err := pub.Get(ctx, key)
		require.ErrorIs(t, err, ErrNotFound, "Get %s", key)
		_, err = pub.Stat(ctx, key)
		require.ErrorIs(t, err, ErrNotFound, "Stat %s", key)
		exists, err := pub.Exists(ctx, key)
		require.NoError(t, err, "Exists %s", key)
		require.False(t, exists, "Exists %s", key)
	}

	// Writes through a symlinked directory are refused and land nowhere.
	require.Error(t, pub.Put(ctx, "biz/new.png", strings.NewReader("x"), PutOptions{}))
	_, err = os.Stat(filepath.Join(outside, "new.png"))
	require.ErrorIs(t, err, os.ErrNotExist, "Put wrote through a symlinked directory")
	entries, err := os.ReadDir(filepath.Join(pub.Root(), localTmpDir))
	require.NoError(t, err)
	require.Empty(t, entries, "a refused Put must not leave temp files")

	// Deletes never remove the link target.
	require.ErrorIs(t, pub.Delete(ctx, "biz/secret.txt"), ErrNotFound)
	require.ErrorIs(t, pub.Delete(ctx, "leak.png"), ErrNotFound)
	raw, err := os.ReadFile(secret)
	require.NoError(t, err)
	require.Equal(t, "outside the store", string(raw))
	_, err = prot.Stat(ctx, "businesses/1/ledger/receipt.pdf")
	require.NoError(t, err, "the protected object must survive")
}

// TestLocalStoreIgnoresSymlinkedSidecar: a meta sidecar that is a symlink out
// of the store is ignored, so it cannot change the served content type.
func TestLocalStoreIgnoresSymlinkedSidecar(t *testing.T) {
	ctx := context.Background()
	store, err := NewLocalStore(t.TempDir())
	require.NoError(t, err)
	body := []byte("png bytes")
	require.NoError(t, store.Put(ctx, "a/photo.png", bytes.NewReader(body), PutOptions{ContentType: "image/png"}))

	forged := filepath.Join(t.TempDir(), "forged.json")
	require.NoError(t, os.WriteFile(forged, []byte(`{"content_type":"text/html","sha256":"00000000000000000000000000000000","size":9}`), 0o600))
	sidecar := filepath.Join(store.Root(), localMetaDir, "a", "photo.png.json")
	require.NoError(t, os.Remove(sidecar))
	require.NoError(t, os.Symlink(forged, sidecar))

	info, err := store.Stat(ctx, "a/photo.png")
	require.NoError(t, err)
	require.Equal(t, "image/png", info.ContentType, "content type must come from the extension, not the out-of-store sidecar")
	require.True(t, strings.HasPrefix(info.ETag, `W/"`), "the forged sidecar ETag must not be used, got %s", info.ETag)
}

// TestServeMediaRefusesSymlinkEscape: GET /media answers 404 (not the outside
// file, not a 5xx) for a key that resolves through an escaping symlink.
func TestServeMediaRefusesSymlinkEscape(t *testing.T) {
	store, err := NewLocalStore(t.TempDir())
	require.NoError(t, err)
	secret := filepath.Join(t.TempDir(), "secret.png")
	require.NoError(t, os.WriteFile(secret, []byte("outside the store"), 0o600))
	require.NoError(t, os.Symlink(secret, filepath.Join(store.Root(), localObjectsDir, "leak.png")))

	t.Cleanup(SetStores(store, nil))
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		w := httptest.NewRecorder()
		ServeMedia(w, httptest.NewRequest(method, "/media/leak.png", nil), "leak.png")
		require.Equal(t, http.StatusNotFound, w.Code, method)
		got, _ := io.ReadAll(w.Body)
		require.False(t, bytes.Contains(got, []byte("outside the store")), method)
	}
}

func TestEscapesRootIgnoresOrdinaryErrors(t *testing.T) {
	require.False(t, escapesRoot(nil))
	require.False(t, escapesRoot(os.ErrNotExist))
	require.False(t, escapesRoot(&os.PathError{Op: "open", Path: "x", Err: errors.New("permission denied")}))
}
