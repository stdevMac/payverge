package security

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/config"
)

func devPluginKeyTestEnv(t *testing.T) {
	t.Helper()
	t.Setenv("PLUGIN_SECRET_KEY", "")
	t.Setenv("ENV", "development")
	t.Setenv("APP_ENV", "")
	config.SetProductionModeOverride(false)
	resetDevelopmentPluginKey()
	t.Cleanup(resetDevelopmentPluginKey)
}

func currentDevKey(t *testing.T) []byte {
	t.Helper()
	key, err := pluginSecretKey()
	if err != nil {
		t.Fatalf("pluginSecretKey() error = %v", err)
	}
	return append([]byte(nil), key...)
}

func TestEnsureDevPluginSecretKey_GeneratesPersistsAndReuses(t *testing.T) {
	devPluginKeyTestEnv(t)
	dataDir := t.TempDir()

	status, err := EnsureDevPluginSecretKey(false, dataDir)
	if err != nil {
		t.Fatalf("EnsureDevPluginSecretKey() error = %v", err)
	}
	if status.Source != PluginKeyGenerated {
		t.Fatalf("Source = %q, want %q", status.Source, PluginKeyGenerated)
	}
	wantPath := filepath.Join(dataDir, "secrets", "plugin_secret_key")
	if status.Path != wantPath {
		t.Fatalf("Path = %q, want %q", status.Path, wantPath)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(wantPath)
		if err != nil {
			t.Fatalf("stat key file: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("key file perm = %o, want 600", perm)
		}
		dirInfo, err := os.Stat(filepath.Dir(wantPath))
		if err != nil {
			t.Fatalf("stat key dir: %v", err)
		}
		if perm := dirInfo.Mode().Perm(); perm != 0o700 {
			t.Fatalf("key dir perm = %o, want 700", perm)
		}
	}
	first := currentDevKey(t)
	if len(first) != 32 {
		t.Fatalf("key length = %d, want 32", len(first))
	}

	// Encrypt with the persisted key, then simulate a restart.
	ciphertext, err := EncryptSecret("mp_access_token_value")
	if err != nil {
		t.Fatalf("EncryptSecret() error = %v", err)
	}
	resetDevelopmentPluginKey()

	status, err = EnsureDevPluginSecretKey(false, dataDir)
	if err != nil {
		t.Fatalf("second EnsureDevPluginSecretKey() error = %v", err)
	}
	if status.Source != PluginKeyLoaded {
		t.Fatalf("Source after restart = %q, want %q", status.Source, PluginKeyLoaded)
	}
	if !bytes.Equal(first, currentDevKey(t)) {
		t.Fatal("restart produced a different key; persisted credentials would be unreadable")
	}
	plaintext, err := DecryptSecret(ciphertext)
	if err != nil || plaintext != "mp_access_token_value" {
		t.Fatalf("DecryptSecret() after restart = (%q, %v)", plaintext, err)
	}
}

func TestEnsureDevPluginSecretKey_IsNeverTheRetiredConstant(t *testing.T) {
	devPluginKeyTestEnv(t)
	legacy := sha256.Sum256([]byte("payverge-local-plugin-secret-key"))

	if _, err := EnsureDevPluginSecretKey(false, t.TempDir()); err != nil {
		t.Fatalf("EnsureDevPluginSecretKey() error = %v", err)
	}
	a := currentDevKey(t)

	resetDevelopmentPluginKey()
	if _, err := EnsureDevPluginSecretKey(false, t.TempDir()); err != nil {
		t.Fatalf("EnsureDevPluginSecretKey() error = %v", err)
	}
	b := currentDevKey(t)

	if bytes.Equal(a, legacy[:]) || bytes.Equal(b, legacy[:]) {
		t.Fatal("development key equals the retired public constant")
	}
	if bytes.Equal(a, b) {
		t.Fatal("two instances generated the same development key")
	}
}

func TestPluginSecretKey_UnconfiguredProcessUsesEphemeralKey(t *testing.T) {
	devPluginKeyTestEnv(t)
	legacy := sha256.Sum256([]byte("payverge-local-plugin-secret-key"))

	key := currentDevKey(t)
	if bytes.Equal(key, legacy[:]) {
		t.Fatal("unconfigured development process fell back to the retired public constant")
	}
	if !bytes.Equal(key, currentDevKey(t)) {
		t.Fatal("ephemeral key must be stable within one process")
	}
}

func TestEnsureDevPluginSecretKey_ProductionRequiresEnv(t *testing.T) {
	devPluginKeyTestEnv(t)
	dataDir := t.TempDir()

	if _, err := EnsureDevPluginSecretKey(true, dataDir); err == nil {
		t.Fatal("EnsureDevPluginSecretKey(production) error = nil, want error")
	}
	if _, err := os.Stat(filepath.Join(dataDir, "secrets")); !os.IsNotExist(err) {
		t.Fatal("production must not generate a key file")
	}
}

func TestEnsureDevPluginSecretKey_EnvWins(t *testing.T) {
	devPluginKeyTestEnv(t)
	t.Setenv("PLUGIN_SECRET_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	dataDir := t.TempDir()

	for _, production := range []bool{false, true} {
		status, err := EnsureDevPluginSecretKey(production, dataDir)
		if err != nil {
			t.Fatalf("EnsureDevPluginSecretKey(%v) error = %v", production, err)
		}
		if status.Source != PluginKeyFromEnv {
			t.Fatalf("Source = %q, want env", status.Source)
		}
	}
	if _, err := os.Stat(filepath.Join(dataDir, "secrets")); !os.IsNotExist(err) {
		t.Fatal("an env key must not generate a key file")
	}
	if key := currentDevKey(t); !bytes.Equal(key, bytes.Repeat([]byte{7}, 32)) {
		t.Fatal("PLUGIN_SECRET_KEY was not used")
	}
}

func TestEnsureDevPluginSecretKey_CorruptFileIsAnError(t *testing.T) {
	devPluginKeyTestEnv(t)
	dataDir := t.TempDir()
	path := filepath.Join(dataDir, "secrets", "plugin_secret_key")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not-a-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := EnsureDevPluginSecretKey(false, dataDir)
	if err == nil {
		t.Fatal("corrupt key file accepted; regenerating would orphan stored credentials")
	}
	if strings.Contains(err.Error(), "not-a-key") {
		t.Fatal("error echoes key file contents")
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "not-a-key\n" {
		t.Fatal("corrupt key file was overwritten")
	}
}

func TestEnsureDevPluginSecretKey_TightensLoosePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions")
	}
	devPluginKeyTestEnv(t)
	dataDir := t.TempDir()
	path := filepath.Join(dataDir, "secrets", "plugin_secret_key")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	encoded := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))
	if err := os.WriteFile(path, []byte(encoded+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}

	status, err := EnsureDevPluginSecretKey(false, dataDir)
	if err != nil || status.Source != PluginKeyLoaded {
		t.Fatalf("EnsureDevPluginSecretKey() = (%v, %v)", status.Source, err)
	}
	info, _ := os.Stat(path)
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("perm = %o, want 600", perm)
	}
	if !bytes.Equal(currentDevKey(t), bytes.Repeat([]byte{3}, 32)) {
		t.Fatal("persisted key not loaded")
	}
}

func TestEnsureDevPluginSecretKey_UnwritableDirFallsBackToEphemeral(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX permissions enforced for a non-root user")
	}
	devPluginKeyTestEnv(t)
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })

	status, err := EnsureDevPluginSecretKey(false, filepath.Join(parent, "data"))
	if err != nil {
		t.Fatalf("EnsureDevPluginSecretKey() error = %v", err)
	}
	if status.Source != PluginKeyEphemeral || status.PersistErr == nil {
		t.Fatalf("status = %+v, want ephemeral with PersistErr", status)
	}
	if len(currentDevKey(t)) != 32 {
		t.Fatal("ephemeral key not installed")
	}
}
