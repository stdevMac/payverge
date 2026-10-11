package scan

import (
	"os"
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

func TestLocalArtifactStoreKeepsTraversalKeysInsideRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "space-scans")
	store, err := NewLocalArtifactStore(root)
	if err != nil {
		t.Fatalf("NewLocalArtifactStore: %v", err)
	}
	loc, err := store.Put("../../escape/../scan.json", []byte("x"), "application/json")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if loc != "local://"+filepath.Join("escape", "scan.json") {
		t.Fatalf("location = %q", loc)
	}
	if _, err := os.Stat(filepath.Join(root, "escape", "scan.json")); err != nil {
		t.Fatalf("file not written under root: %v", err)
	}
}

func TestLocalArtifactStoreRefusesSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(base, "outside")
	if err := os.Mkdir(outside, 0o750); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("outside the store"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "space-scans")
	store, err := NewLocalArtifactStore(root)
	if err != nil {
		t.Fatalf("NewLocalArtifactStore: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Put("linked/new.json", []byte("x"), "application/json"); err == nil {
		t.Fatal("Put wrote through a symlinked directory")
	}
	if _, err := os.Stat(filepath.Join(outside, "new.json")); !os.IsNotExist(err) {
		t.Fatalf("file created outside root: %v", err)
	}
	if data, err := store.Get("local://linked/secret.txt"); err == nil {
		t.Fatalf("Get read outside root: %q", data)
	}
	if err := store.Delete("local://linked/secret.txt"); err == nil {
		t.Fatal("Delete through a symlink escape returned nil")
	}
	if _, err := os.Stat(secret); err != nil {
		t.Fatalf("file outside root was removed: %v", err)
	}
}
