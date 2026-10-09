package security

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// devPluginKeyFile is where a development instance keeps its generated
// PLUGIN_SECRET_KEY, relative to the data directory (config.DataDir()).
const devPluginKeyFile = "secrets/plugin_secret_key"

// PluginKeySource says where the active plugin encryption key came from.
type PluginKeySource string

const (
	// PluginKeyFromEnv: PLUGIN_SECRET_KEY is set (always the case in production).
	PluginKeyFromEnv PluginKeySource = "env"
	// PluginKeyGenerated: development generated a new random key and persisted it.
	PluginKeyGenerated PluginKeySource = "generated"
	// PluginKeyLoaded: development reused the key persisted on an earlier boot.
	PluginKeyLoaded PluginKeySource = "file"
	// PluginKeyEphemeral: development could not persist a key; this process
	// uses a random in-memory key, so credentials saved now will not decrypt
	// after a restart.
	PluginKeyEphemeral PluginKeySource = "ephemeral"
)

// DevPluginKeyStatus describes the outcome of EnsureDevPluginSecretKey. It
// never carries key material.
type DevPluginKeyStatus struct {
	Source PluginKeySource
	// Path is the key file (generated / file / ephemeral attempts).
	Path string
	// PersistErr explains why an ephemeral key was used.
	PersistErr error
}

var (
	devPluginKeyMu sync.Mutex
	devPluginKey   []byte
)

// EnsureDevPluginSecretKey resolves the plugin encryption key once at boot.
//
// Production must set PLUGIN_SECRET_KEY: there is no fallback, so an unset
// key is an error. Outside production an unset key is replaced by a random
// 32-byte key generated on first boot and persisted with 0600 permissions at
// <dataDir>/secrets/plugin_secret_key, then reused on every later boot. It is
// never a constant: every instance gets its own key.
//
// A key file that exists but cannot be read or decoded is an error rather
// than being regenerated, because replacing it would orphan every credential
// already encrypted with it. When the directory is not writable, the process
// falls back to an ephemeral in-memory key and reports why in PersistErr.
func EnsureDevPluginSecretKey(production bool, dataDir string) (DevPluginKeyStatus, error) {
	if strings.TrimSpace(os.Getenv("PLUGIN_SECRET_KEY")) != "" {
		return DevPluginKeyStatus{Source: PluginKeyFromEnv}, nil
	}
	if production {
		return DevPluginKeyStatus{}, errors.New("PLUGIN_SECRET_KEY is required in production (generate one with: openssl rand -base64 32)")
	}
	if strings.TrimSpace(dataDir) == "" {
		dataDir = "data"
	}
	path := filepath.Join(dataDir, filepath.FromSlash(devPluginKeyFile))

	devPluginKeyMu.Lock()
	defer devPluginKeyMu.Unlock()

	key, err := readDevPluginKey(path)
	switch {
	case err == nil:
		devPluginKey = key
		return DevPluginKeyStatus{Source: PluginKeyLoaded, Path: path}, nil
	case !errors.Is(err, fs.ErrNotExist):
		return DevPluginKeyStatus{Path: path}, err
	}

	key, err = writeDevPluginKey(path)
	if err == nil {
		devPluginKey = key
		return DevPluginKeyStatus{Source: PluginKeyGenerated, Path: path}, nil
	}
	if errors.Is(err, fs.ErrExist) {
		// Another process created it between the read and the write.
		key, readErr := readDevPluginKey(path)
		if readErr != nil {
			return DevPluginKeyStatus{Path: path}, readErr
		}
		devPluginKey = key
		return DevPluginKeyStatus{Source: PluginKeyLoaded, Path: path}, nil
	}

	ephemeral, genErr := randomPluginKey()
	if genErr != nil {
		return DevPluginKeyStatus{Path: path}, genErr
	}
	devPluginKey = ephemeral
	return DevPluginKeyStatus{Source: PluginKeyEphemeral, Path: path, PersistErr: err}, nil
}

func readDevPluginKey(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("development plugin key %s is not a regular file", path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		// Tighten permissions left loose by a copy or restore.
		_ = os.Chmod(path, 0o600)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read development plugin key %s: %w", path, err)
	}
	key, err := decodePluginKey(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("development plugin key %s is corrupt (%v); restore it or set PLUGIN_SECRET_KEY — deleting it makes stored plugin credentials unreadable", path, err)
	}
	return key, nil
}

func writeDevPluginKey(path string) ([]byte, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	key, err := randomPluginKey()
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(key) + "\n"
	if _, err := f.WriteString(encoded); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return key, nil
}

func randomPluginKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate plugin secret key: %w", err)
	}
	return key, nil
}

// developmentPluginKey returns the key installed by EnsureDevPluginSecretKey,
// or — for processes that never called it (tests, one-off tools) — a random
// per-process key generated on first use.
func developmentPluginKey() ([]byte, error) {
	devPluginKeyMu.Lock()
	defer devPluginKeyMu.Unlock()
	if devPluginKey == nil {
		key, err := randomPluginKey()
		if err != nil {
			return nil, err
		}
		devPluginKey = key
		log.Println("WARNING: PLUGIN_SECRET_KEY is not set; using an ephemeral per-process plugin encryption key (plugin credentials saved now will not decrypt after a restart)")
	}
	return devPluginKey, nil
}

// resetDevelopmentPluginKey clears the cached development key (tests only).
func resetDevelopmentPluginKey() {
	devPluginKeyMu.Lock()
	devPluginKey = nil
	devPluginKeyMu.Unlock()
}

func decodePluginKey(raw string) ([]byte, error) {
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding} {
		decoded, err := encoding.DecodeString(raw)
		if err == nil && len(decoded) == 32 {
			return decoded, nil
		}
	}
	if len([]byte(raw)) == 32 {
		return []byte(raw), nil
	}
	// 64 hex characters (openssl rand -hex 32), decision D-2. Checked last:
	// no 64-character hex string is a valid 32-byte base64 or raw key, so
	// existing keys keep their exact derivation.
	if len(raw) == 64 {
		if decoded, err := hex.DecodeString(raw); err == nil {
			return decoded, nil
		}
	}
	return nil, errors.New("PLUGIN_SECRET_KEY must be 32 bytes, base64-encoded 32 bytes or 64 hex characters (generate one with: openssl rand -hex 32)")
}
