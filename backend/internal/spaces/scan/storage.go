package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/s3"
)

// ArtifactStore persists scan upload bytes. Production uses protected S3;
// local dev falls back to <DATA_DIR>/space-scans/ when S3 is unavailable.
type ArtifactStore interface {
	// Put stores data and returns a stable key/location for later Get/Delete.
	Put(key string, data []byte, contentType string) (location string, err error)
	// Get loads bytes by key or location returned from Put.
	Get(keyOrLocation string) ([]byte, error)
	// Delete removes the artifact. Missing keys are not errors.
	Delete(keyOrLocation string) error
}

// S3ArtifactStore writes to the protected S3 bucket.
type S3ArtifactStore struct {
	Folder string // e.g. "space-scans"
}

// Put implements ArtifactStore.
func (s S3ArtifactStore) Put(key string, data []byte, contentType string) (string, error) {
	folder := s.Folder
	if folder == "" {
		folder = "space-scans"
	}
	return s3.UploadBytesProtected(data, key, folder, contentType)
}

// Get implements ArtifactStore.
func (s S3ArtifactStore) Get(keyOrLocation string) ([]byte, error) {
	return s3.DownloadFileProtected(keyOrLocation)
}

// Delete implements ArtifactStore.
func (s S3ArtifactStore) Delete(keyOrLocation string) error {
	return s3.DeleteFileProtected(keyOrLocation)
}

// LocalArtifactStore stores files under a root directory (default
// <DATA_DIR>/space-scans, see DefaultLocalArtifactDir).
//
// Every file operation goes through an os.Root opened on Root, so neither a
// crafted key nor a symlink planted in the directory can read, write or delete
// outside it.
type LocalArtifactStore struct {
	Root string
	fsys *os.Root
	mu   sync.Mutex
}

// DefaultLocalArtifactDir is where scan uploads land when no directory is
// given: space-scans under config.DataDir() (DATA_DIR, else ./data, which is
// the writable backend_data volume in the container image). The old default,
// ./.local/space-scans, is not writable by the image's runtime user, so the
// local fallback silently disappeared there.
func DefaultLocalArtifactDir() string {
	return filepath.Join(config.DataDir(), "space-scans")
}

// NewLocalArtifactStore creates a store rooted at dir (created if missing).
// The store keeps the directory open for the life of the process.
func NewLocalArtifactStore(dir string) (*LocalArtifactStore, error) {
	if dir == "" {
		dir = DefaultLocalArtifactDir()
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create local space-scan dir: %w", err)
	}
	fsys, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open local space-scan dir: %w", err)
	}
	return &LocalArtifactStore{Root: dir, fsys: fsys}, nil
}

// Put implements ArtifactStore.
func (l *LocalArtifactStore) Put(key string, data []byte, contentType string) (string, error) {
	_ = contentType
	l.mu.Lock()
	defer l.mu.Unlock()
	clean := sanitizeLocalKey(key)
	if err := l.fsys.MkdirAll(filepath.Dir(clean), 0o750); err != nil {
		return "", err
	}
	if err := l.fsys.WriteFile(clean, data, 0o640); err != nil {
		return "", err
	}
	// Prefix so Get/Delete can detect local paths.
	return "local://" + clean, nil
}

// Get implements ArtifactStore.
func (l *LocalArtifactStore) Get(keyOrLocation string) ([]byte, error) {
	return l.fsys.ReadFile(sanitizeLocalKey(strings.TrimPrefix(keyOrLocation, "local://")))
}

// Delete implements ArtifactStore. A missing file is not an error.
func (l *LocalArtifactStore) Delete(keyOrLocation string) error {
	err := l.fsys.Remove(sanitizeLocalKey(strings.TrimPrefix(keyOrLocation, "local://")))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func sanitizeLocalKey(key string) string {
	key = strings.ReplaceAll(key, "\\", "/")
	key = strings.TrimPrefix(key, "/")
	parts := strings.Split(key, "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || p == "." || p == ".." {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return "unnamed"
	}
	return filepath.Join(out...)
}

// CompositeArtifactStore tries S3 first; on Put failure falls back to local.
type CompositeArtifactStore struct {
	Primary  ArtifactStore
	Fallback ArtifactStore
}

// Put tries primary then fallback.
func (c CompositeArtifactStore) Put(key string, data []byte, contentType string) (string, error) {
	if c.Primary != nil {
		loc, err := c.Primary.Put(key, data, contentType)
		if err == nil {
			return loc, nil
		}
	}
	if c.Fallback != nil {
		return c.Fallback.Put(key, data, contentType)
	}
	return "", fmt.Errorf("no artifact store available")
}

// Get tries primary then fallback based on location prefix.
func (c CompositeArtifactStore) Get(keyOrLocation string) ([]byte, error) {
	if strings.HasPrefix(keyOrLocation, "local://") && c.Fallback != nil {
		return c.Fallback.Get(keyOrLocation)
	}
	if c.Primary != nil {
		data, err := c.Primary.Get(keyOrLocation)
		if err == nil {
			return data, nil
		}
	}
	if c.Fallback != nil {
		return c.Fallback.Get(keyOrLocation)
	}
	return nil, fmt.Errorf("artifact not found")
}

// Delete tries both stores.
func (c CompositeArtifactStore) Delete(keyOrLocation string) error {
	if strings.HasPrefix(keyOrLocation, "local://") && c.Fallback != nil {
		return c.Fallback.Delete(keyOrLocation)
	}
	var firstErr error
	if c.Primary != nil {
		if err := c.Primary.Delete(keyOrLocation); err != nil {
			firstErr = err
		} else {
			return nil
		}
	}
	if c.Fallback != nil {
		return c.Fallback.Delete(keyOrLocation)
	}
	return firstErr
}

// DefaultArtifactStore builds S3 + local fallback store for production/dev.
func DefaultArtifactStore(localDir string) ArtifactStore {
	local, err := NewLocalArtifactStore(localDir)
	if err != nil {
		// Still return S3-only if local mkdir fails.
		return S3ArtifactStore{Folder: "space-scans"}
	}
	return CompositeArtifactStore{
		Primary:  S3ArtifactStore{Folder: "space-scans"},
		Fallback: local,
	}
}
