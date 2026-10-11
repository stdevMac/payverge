package s3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// runStoreConformance is the contract every Store driver must satisfy.
func runStoreConformance(t *testing.T, newStore func(t *testing.T) Store) {
	ctx := context.Background()

	t.Run("put get stat round trip", func(t *testing.T) {
		s := newStore(t)
		body := []byte("\x89PNG fake image bytes")
		key := "businesses/7/menu_items/0123456789abcdef_dish.png"
		require.NoError(t, s.Put(ctx, key, bytes.NewReader(body), PutOptions{
			ContentType:        "image/png",
			ContentDisposition: `inline; filename="dish.png"`,
			Metadata:           map[string]string{"ai-generated": "true"},
		}))

		obj, err := s.Get(ctx, key)
		require.NoError(t, err)
		got, err := io.ReadAll(obj.Body)
		require.NoError(t, obj.Body.Close())
		require.NoError(t, err)
		require.Equal(t, body, got)
		require.Equal(t, key, obj.Key)
		require.Equal(t, int64(len(body)), obj.Size)
		require.Equal(t, "image/png", obj.ContentType)
		require.Equal(t, `inline; filename="dish.png"`, obj.ContentDisposition)
		require.Equal(t, "true", obj.Metadata["ai-generated"])
		require.NotEmpty(t, obj.ETag)
		require.False(t, obj.ModTime.IsZero())

		info, err := s.Stat(ctx, key)
		require.NoError(t, err)
		require.Equal(t, obj.ETag, info.ETag)
		require.Equal(t, int64(len(body)), info.Size)
		require.Equal(t, "image/png", info.ContentType)

		ok, err := s.Exists(ctx, key)
		require.NoError(t, err)
		require.True(t, ok)
	})

	t.Run("missing objects map to ErrNotFound", func(t *testing.T) {
		s := newStore(t)
		_, err := s.Get(ctx, "nope/missing.png")
		require.ErrorIs(t, err, ErrNotFound)
		_, err = s.Stat(ctx, "nope/missing.png")
		require.ErrorIs(t, err, ErrNotFound)
		ok, err := s.Exists(ctx, "nope/missing.png")
		require.NoError(t, err)
		require.False(t, ok)
		require.NoError(t, s.Delete(ctx, "nope/missing.png"), "deleting a missing object is not an error")
	})

	t.Run("overwrite replaces bytes and etag", func(t *testing.T) {
		s := newStore(t)
		key := "logos/brand.png"
		require.NoError(t, s.Put(ctx, key, strings.NewReader("one"), PutOptions{ContentType: "image/png"}))
		first, err := s.Stat(ctx, key)
		require.NoError(t, err)
		require.NoError(t, s.Put(ctx, key, strings.NewReader("second"), PutOptions{ContentType: "image/png"}))
		obj, err := s.Get(ctx, key)
		require.NoError(t, err)
		got, _ := io.ReadAll(obj.Body)
		_ = obj.Body.Close()
		require.Equal(t, "second", string(got))
		require.NotEqual(t, first.ETag, obj.ETag)
	})

	t.Run("delete removes the object", func(t *testing.T) {
		s := newStore(t)
		key := "fiscal-receipts/1/2/0123456789abcdef.pdf"
		require.NoError(t, s.Put(ctx, key, strings.NewReader("%PDF-1.4"), PutOptions{ContentType: "application/pdf"}))
		require.NoError(t, s.Delete(ctx, key))
		ok, err := s.Exists(ctx, key)
		require.NoError(t, err)
		require.False(t, ok)
		_, err = s.Get(ctx, key)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("traversal and malformed keys are rejected", func(t *testing.T) {
		s := newStore(t)
		for _, key := range []string{"", "../escape.png", "a/../../escape.png", "/etc/passwd", "a/./b.png", "a//b.png", `a\..\b.png`, "a/\x00b.png", "a/..", ".."} {
			err := s.Put(ctx, key, strings.NewReader("x"), PutOptions{})
			require.ErrorIs(t, err, ErrInvalidKey, "Put(%q)", key)
			_, err = s.Get(ctx, key)
			require.ErrorIs(t, err, ErrInvalidKey, "Get(%q)", key)
			require.ErrorIs(t, s.Delete(ctx, key), ErrInvalidKey, "Delete(%q)", key)
		}
	})

	t.Run("empty body is a valid object", func(t *testing.T) {
		s := newStore(t)
		require.NoError(t, s.Put(ctx, "empty/file.txt", nil, PutOptions{ContentType: "text/plain"}))
		info, err := s.Stat(ctx, "empty/file.txt")
		require.NoError(t, err)
		require.Zero(t, info.Size)
	})
}

func TestLocalStoreConformance(t *testing.T) {
	runStoreConformance(t, func(t *testing.T) Store {
		s, err := NewLocalStore(t.TempDir())
		require.NoError(t, err)
		return s
	})
}

func TestS3StoreConformance(t *testing.T) {
	runStoreConformance(t, func(t *testing.T) Store {
		return newFakeS3Store(t, newFakeS3(t), "public-bucket", "")
	})
}

func TestS3StoreSharedBucketPrefixAndReservation(t *testing.T) {
	ctx := context.Background()
	fake := newFakeS3(t)
	pub := newFakeS3Store(t, fake, "shared", "", SharedBucketProtectedPrefix)
	prot := newFakeS3Store(t, fake, "shared", SharedBucketProtectedPrefix)

	require.NoError(t, prot.Put(ctx, "fiscal-receipts/1/receipt.pdf", strings.NewReader("%PDF"), PutOptions{}))
	require.True(t, fake.has("/shared/protected/fiscal-receipts/1/receipt.pdf"), "protected keys live under protected/ in a shared bucket")

	// The public store can neither read nor overwrite the protected key space.
	_, err := pub.Get(ctx, "protected/fiscal-receipts/1/receipt.pdf")
	require.ErrorIs(t, err, ErrInvalidKey)
	require.ErrorIs(t, pub.Put(ctx, "protected/evil.png", strings.NewReader("x"), PutOptions{}), ErrInvalidKey)
	require.ErrorIs(t, pub.Delete(ctx, "protected/fiscal-receipts/1/receipt.pdf"), ErrInvalidKey)
	require.True(t, fake.has("/shared/protected/fiscal-receipts/1/receipt.pdf"))

	// Legacy S3 keys with spaces stay addressable on the S3 driver.
	require.NoError(t, pub.Put(ctx, "businesses/1/Menu Photo.png", strings.NewReader("x"), PutOptions{}))
	ok, err := pub.Exists(ctx, "businesses/1/Menu Photo.png")
	require.NoError(t, err)
	require.True(t, ok)
}

func TestLocalStoreSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	first, err := NewLocalStore(root)
	require.NoError(t, err)
	key := "businesses/3/menu_items/fedcba9876543210_milanesa.webp"
	require.NoError(t, first.Put(ctx, key, strings.NewReader("webp-bytes"), PutOptions{
		ContentType: "image/webp",
		Metadata:    map[string]string{"ai-generated": "true"},
	}))
	before, err := first.Stat(ctx, key)
	require.NoError(t, err)

	// A new process: a fresh Store over the same directory, no shared memory.
	second, err := NewLocalStore(root)
	require.NoError(t, err)
	obj, err := second.Get(ctx, key)
	require.NoError(t, err)
	got, _ := io.ReadAll(obj.Body)
	_ = obj.Body.Close()
	require.Equal(t, "webp-bytes", string(got))
	require.Equal(t, "image/webp", obj.ContentType)
	require.Equal(t, "true", obj.Metadata["ai-generated"])
	require.Equal(t, before.ETag, obj.ETag, "ETag must be stable across restarts (content hash, not inode/mtime)")
	require.False(t, strings.HasPrefix(obj.ETag, "W/"), "a committed sidecar yields a strong ETag")
}

func TestLocalStorePermissionsAndLayout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "store")
	s, err := NewLocalStore(root)
	require.NoError(t, err)
	require.NoError(t, s.Put(ctx, "a/b/c.png", strings.NewReader("x"), PutOptions{ContentType: "image/png"}))

	for _, dir := range []string{root, filepath.Join(root, "objects"), filepath.Join(root, "objects", "a", "b"), filepath.Join(root, "meta", "a", "b"), filepath.Join(root, "tmp")} {
		fi, err := os.Stat(dir)
		require.NoError(t, err, dir)
		require.Equal(t, os.FileMode(0o700), fi.Mode().Perm(), "dir %s", dir)
	}
	for _, file := range []string{filepath.Join(root, "objects", "a", "b", "c.png"), filepath.Join(root, "meta", "a", "b", "c.png.json")} {
		fi, err := os.Stat(file)
		require.NoError(t, err, file)
		require.Equal(t, os.FileMode(0o600), fi.Mode().Perm(), "file %s", file)
	}
	entries, err := os.ReadDir(filepath.Join(root, "tmp"))
	require.NoError(t, err)
	require.Empty(t, entries, "atomic writes must not leave temp files behind")
}

func TestLocalStoreStrictKeys(t *testing.T) {
	ctx := context.Background()
	s, err := NewLocalStore(t.TempDir())
	require.NoError(t, err)
	for _, key := range []string{"a b.png", "a/.hidden", ".env", "a/%2e%2e/b", "ünicode.png", "a?b.png", "a#b.png", "a:b.png", strings.Repeat("a", maxKeyLen+1)} {
		require.ErrorIs(t, s.Put(ctx, key, strings.NewReader("x"), PutOptions{}), ErrInvalidKey, "Put(%q)", key)
	}
	// Directories are never objects.
	require.NoError(t, s.Put(ctx, "dir/file.png", strings.NewReader("x"), PutOptions{}))
	_, err = s.Get(ctx, "dir")
	require.ErrorIs(t, err, ErrNotFound)
	_, err = s.Stat(ctx, "dir")
	require.ErrorIs(t, err, ErrNotFound)
	require.ErrorIs(t, s.Delete(ctx, "dir"), ErrNotFound)
	// A path through a file is a miss, not an I/O error.
	_, err = s.Get(ctx, "dir/file.png/child.png")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestLocalStoreStaleSidecarFallsBackToWeakETag(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s, err := NewLocalStore(root)
	require.NoError(t, err)
	require.NoError(t, s.Put(ctx, "x/photo.jpg", strings.NewReader("abc"), PutOptions{ContentType: "image/png"}))
	// Simulate a crash between the object rename and the sidecar rename of a
	// later overwrite: object bytes changed, sidecar did not.
	require.NoError(t, os.WriteFile(filepath.Join(root, "objects", "x", "photo.jpg"), []byte("abcdef"), 0o600))
	info, err := s.Stat(ctx, "x/photo.jpg")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(info.ETag, `W/"`), "stale sidecar must not vouch for new bytes: %s", info.ETag)
	require.Equal(t, "image/jpeg", info.ContentType, "falls back to the extension type")
}

func TestLocalStoreCancelledPutLeavesNothing(t *testing.T) {
	root := t.TempDir()
	s, err := NewLocalStore(root)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = s.Put(ctx, "x/a.png", strings.NewReader("abc"), PutOptions{})
	require.True(t, errors.Is(err, context.Canceled), "got %v", err)
	ok, err := s.Exists(context.Background(), "x/a.png")
	require.NoError(t, err)
	require.False(t, ok)
	entries, err := os.ReadDir(filepath.Join(root, "tmp"))
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestNewLocalStoreRejectsUnwritableRoot(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs non-root POSIX permissions")
	}
	parent := t.TempDir()
	require.NoError(t, os.Chmod(parent, 0o500))
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
	_, err := NewLocalStore(filepath.Join(parent, "store"))
	require.Error(t, err)
	_, err = NewLocalStore("  ")
	require.Error(t, err)
}
