package scan

import (
	"path/filepath"
	"testing"
)

func TestNewLocalArtifactStoreDefaultsUnderDataDir(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("DATA_DIR", dataDir)

	store, err := NewLocalArtifactStore("")
	if err != nil {
		t.Fatalf("NewLocalArtifactStore: %v", err)
	}
	if want := filepath.Join(dataDir, "space-scans"); store.Root != want {
		t.Fatalf("Root = %q, want %q", store.Root, want)
	}
	loc, err := store.Put("abc.jpg", []byte("x"), "image/jpeg")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := store.Get(loc)
	if err != nil || string(got) != "x" {
		t.Fatalf("Get = %q, %v", got, err)
	}
}
