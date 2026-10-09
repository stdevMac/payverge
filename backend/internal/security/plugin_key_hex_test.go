package security

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
)

// Decision D-2: a 64-character hex key (openssl rand -hex 32) decodes to its
// 32 bytes, and every format accepted before keeps its exact derivation.
func TestDecodePluginKey_AcceptsHexAndKeepsExistingFormats(t *testing.T) {
	raw := bytes.Repeat([]byte{0xab, 0x01, 0x7f, 0xee}, 8)

	for _, key := range []string{hex.EncodeToString(raw), strings.ToUpper(hex.EncodeToString(raw))} {
		got, err := decodePluginKey(key)
		if err != nil {
			t.Fatalf("hex key rejected: %v", err)
		}
		if !bytes.Equal(got, raw) {
			t.Fatalf("hex key decoded to %x, want %x", got, raw)
		}
	}

	b64 := base64.StdEncoding.EncodeToString(raw)
	if got, err := decodePluginKey(b64); err != nil || !bytes.Equal(got, raw) {
		t.Fatalf("base64 key derivation changed: %x, %v", got, err)
	}
	literal := "0123456789abcdef0123456789abcdef" // 32 raw bytes
	if got, err := decodePluginKey(literal); err != nil || string(got) != literal {
		t.Fatalf("raw 32-byte key derivation changed: %q, %v", got, err)
	}

	for _, bad := range []string{hex.EncodeToString(raw)[:62], hex.EncodeToString(raw) + "00", strings.Repeat("zz", 32)} {
		if _, err := decodePluginKey(bad); err == nil {
			t.Fatalf("decodePluginKey(%q) accepted a malformed key", bad)
		}
	}
}
