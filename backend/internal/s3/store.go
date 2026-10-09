package s3

import (
	"context"
	"errors"
	"io"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Driver names accepted by STORAGE_DRIVER.
const (
	DriverLocal = "local"
	DriverS3    = "s3"
)

// MediaPathPrefix is the same-origin path public objects are served under
// (GET /media/<key>, see media.go).
const MediaPathPrefix = "/media/"

// maxKeyLen mirrors the S3 object-key ceiling.
const maxKeyLen = 1024

var (
	// ErrNotFound is returned by Store reads for a missing object.
	ErrNotFound = errors.New("storage: object not found")
	// ErrInvalidKey is returned for keys that fail ValidateKey.
	ErrInvalidKey = errors.New("storage: invalid object key")
	// ErrNotConfigured is returned when the public/protected store is unset.
	ErrNotConfigured = errors.New("storage: not initialized")
)

// PutOptions carries the per-object attributes a driver persists with the
// bytes. Metadata mirrors S3 user metadata (x-amz-meta-*).
type PutOptions struct {
	ContentType        string
	ContentDisposition string
	Metadata           map[string]string
}

// ObjectInfo describes a stored object without its body.
type ObjectInfo struct {
	Key                string
	Size               int64
	ContentType        string
	ContentDisposition string
	// ETag is a quoted HTTP entity tag (strong or W/ weak), ready to send.
	ETag     string
	ModTime  time.Time
	Metadata map[string]string
}

// Object is an open object. Callers must Close Body. The local driver's
// Body also implements io.ReadSeeker (Range support on /media).
type Object struct {
	ObjectInfo
	Body io.ReadCloser
}

// Store is the object-storage backend. Two instances exist per process: the
// public store (served at /media/<key>, or a configured CDN) and the protected
// store (never served directly — only streamed by authorized handlers).
type Store interface {
	Driver() string
	Put(ctx context.Context, key string, body io.Reader, opts PutOptions) error
	Get(ctx context.Context, key string) (*Object, error)
	Stat(ctx context.Context, key string) (*ObjectInfo, error)
	Exists(ctx context.Context, key string) (bool, error)
	// Delete removes the object. A missing object is not an error (S3 semantics).
	Delete(ctx context.Context, key string) error
}

// storageState is the process-wide storage wiring, swapped atomically under
// stateMu so tests (and Init) never race request goroutines.
type storageState struct {
	public    Store
	protected Store
	// publicURL is PUBLIC_URL without a trailing slash ("" = relative URLs).
	publicURL     string
	publicURLHost string
	publicURLPath string
	// cdnBaseURL (S3_PUBLIC_BASE_URL) overrides PublicURL for the s3 driver:
	// "<cdnBaseURL>/<key>" instead of "<publicURL>/media/<key>".
	cdnBaseURL string
	// protectedBaseURL (S3_PROTECTED_BASE_URL) is a legacy cosmetic prefix for
	// protected location strings. It never makes objects readable.
	protectedBaseURL string
	// s3Endpoint/s3Bucket let KeyFromURL strip "/<bucket>/" from path-style
	// legacy URLs.
	s3EndpointHost string
	s3Bucket       string
}

var (
	stateMu sync.RWMutex
	state   storageState
)

func snapshot() storageState {
	stateMu.RLock()
	defer stateMu.RUnlock()
	return state
}

func updateState(fn func(*storageState)) {
	stateMu.Lock()
	defer stateMu.Unlock()
	fn(&state)
}

// PublicStore returns the store backing /media (nil before Init).
func PublicStore() Store { return snapshot().public }

// ProtectedStore returns the store for authorized-only objects (nil before Init).
func ProtectedStore() Store { return snapshot().protected }

// ActiveDriver reports the public store's driver ("" before Init).
func ActiveDriver() string {
	if s := PublicStore(); s != nil {
		return s.Driver()
	}
	return ""
}

// SetStores installs public/protected stores and returns a func restoring the
// previous wiring. Tests use it to point the package wrappers at a temp-dir
// LocalStore; production wiring goes through Init.
func SetStores(public, protected Store) (restore func()) {
	stateMu.Lock()
	prev := state
	state.public = public
	state.protected = protected
	stateMu.Unlock()
	return func() {
		stateMu.Lock()
		state = prev
		stateMu.Unlock()
	}
}

// SetPublicURL sets the absolute origin PublicURL prefixes ("" = relative
// "/media/<key>"). Returns a restore func.
func SetPublicURL(publicURL string) (restore func()) {
	publicURL = normalizePublicURL(publicURL)
	stateMu.Lock()
	prevURL, prevHost, prevPath := state.publicURL, state.publicURLHost, state.publicURLPath
	state.publicURL = publicURL
	state.publicURLHost = hostOf(publicURL)
	state.publicURLPath = pathOf(publicURL)
	stateMu.Unlock()
	return func() {
		stateMu.Lock()
		state.publicURL, state.publicURLHost, state.publicURLPath = prevURL, prevHost, prevPath
		stateMu.Unlock()
	}
}

func hostOf(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}

// PublicURL is the one URL contract for public objects. Every upload path
// returns it and the frontend renders it as-is:
//
//   - s3 driver with S3_PUBLIC_BASE_URL: "<S3_PUBLIC_BASE_URL>/<key>" (CDN).
//   - otherwise: "${PUBLIC_URL}/media/<key>", or "/media/<key>" when no
//     PUBLIC_URL is configured.
//
// Keys are restricted to URL-safe characters (ValidateKey), so no escaping is
// needed.
func PublicURL(key string) string {
	key = strings.TrimLeft(key, "/")
	s := snapshot()
	if s.cdnBaseURL != "" {
		return s.cdnBaseURL + "/" + key
	}
	return s.publicURL + MediaPathPrefix + key
}

// protectedLocation is what protected upload helpers return (and callers
// persist): the bare key, or the legacy cosmetic S3_PROTECTED_BASE_URL form.
func protectedLocation(key string) string {
	if base := snapshot().protectedBaseURL; base != "" {
		return base + "/" + key
	}
	return key
}

// JoinKey builds "<folder>/<name>" the way the legacy helpers did.
func JoinKey(folderPath, name string) string {
	folderPath = strings.Trim(folderPath, "/")
	if folderPath == "" {
		return name
	}
	return folderPath + "/" + name
}

// ValidateKey enforces the strict object-key grammar: non-empty, at most 1024
// bytes, relative, already path.Clean'd, no empty / "." / ".." / dot-leading
// segments, and only the URL-safe ASCII set [A-Za-z0-9] plus - _ . ~ ! ( ) + ,
// = @ and "/" as the separator. The local driver maps keys onto the
// filesystem and enforces it on every call (it is the traversal guard), and
// /media only serves keys that pass it. Every key the upload paths generate
// satisfies it, so new objects are portable across drivers.
func ValidateKey(key string) error {
	if key == "" || len(key) > maxKeyLen {
		return ErrInvalidKey
	}
	for i := 0; i < len(key); i++ {
		if !keyByteAllowed(key[i]) {
			return ErrInvalidKey
		}
	}
	if key[0] == '/' || path.Clean(key) != key {
		return ErrInvalidKey
	}
	for _, seg := range strings.Split(key, "/") {
		if seg == "" || seg[0] == '.' {
			return ErrInvalidKey
		}
	}
	return nil
}

func keyByteAllowed(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return true
	}
	switch b {
	case '/', '-', '_', '.', '~', '!', '(', ')', '+', ',', '=', '@':
		return true
	}
	return false
}

// SafeKeySegment turns free text (an item name, a folder label) into one
// ValidateKey-safe path segment: lowercased ASCII, common Latin diacritics
// folded, everything else collapsed to "_", at most 64 bytes. Never empty.
func SafeKeySegment(s string) string {
	var b strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if folded, ok := latinFold[r]; ok {
			r = folded
		}
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore && b.Len() > 0 {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
		if b.Len() >= 64 {
			break
		}
	}
	out := strings.Trim(b.String(), "_-")
	if len(out) > 64 {
		out = strings.Trim(out[:64], "_-")
	}
	if out == "" {
		return "item"
	}
	return out
}

var latinFold = map[rune]rune{
	'á': 'a', 'à': 'a', 'â': 'a', 'ä': 'a', 'ã': 'a', 'å': 'a',
	'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e',
	'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i',
	'ó': 'o', 'ò': 'o', 'ô': 'o', 'ö': 'o', 'õ': 'o', 'ø': 'o',
	'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u',
	'ñ': 'n', 'ç': 'c', 'ß': 's', 'ý': 'y', 'ÿ': 'y',
}

// validateLooseKey is the S3 driver's key check and KeyFromURL's. It is looser
// than ValidateKey (legacy S3 objects may carry spaces or non-ASCII names) but
// still rejects empty, absolute, backslash, control-character and "."/".."
// segments, so a parsed key can never traverse.
func validateLooseKey(key string) error {
	if key == "" || len(key) > maxKeyLen || !utf8.ValidString(key) || key[0] == '/' || strings.Contains(key, "\\") {
		return ErrInvalidKey
	}
	for _, r := range key {
		if r < 0x20 || r == 0x7f {
			return ErrInvalidKey
		}
	}
	for _, seg := range strings.Split(key, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return ErrInvalidKey
		}
	}
	return nil
}

// KeyFromURL resolves a stored location back to its object key. It accepts:
//
//   - bare keys ("businesses/1/x.png", optionally with a leading "/"),
//   - same-origin media URLs ("/media/<key>", "https://host/media/<key>"),
//   - CDN URLs ("<S3_PUBLIC_BASE_URL>/<key>", including a base path),
//   - legacy S3 URLs: virtual-hosted ("https://bucket.endpoint/<key>") and
//     path-style against the configured endpoint ("https://endpoint/bucket/<key>").
//
// Percent-escapes are decoded before validation, so "%2e%2e" cannot smuggle a
// traversal. The result passes validateLooseKey (legacy S3 keys may contain
// spaces); stores apply their own stricter checks. Otherwise ErrInvalidKey.
func KeyFromURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrInvalidKey
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", ErrInvalidKey
	}
	if u.Scheme != "" || u.Host != "" {
		if u.Scheme != "http" && u.Scheme != "https" {
			return "", ErrInvalidKey
		}
		return keyFromAbsoluteURL(u)
	}
	key := u.Path
	if strings.HasPrefix(key, MediaPathPrefix) {
		key = strings.TrimPrefix(key, MediaPathPrefix)
	} else {
		key = strings.TrimPrefix(key, "/")
	}
	if err := validateLooseKey(key); err != nil {
		return "", err
	}
	return key, nil
}

func keyFromAbsoluteURL(u *url.URL) (string, error) {
	s := snapshot()
	host := strings.ToLower(u.Host)
	p := u.Path
	var key string
	switch {
	case s.cdnBaseURL != "" && strings.HasPrefix(u.String(), s.cdnBaseURL+"/"):
		cdn, _ := url.Parse(s.cdnBaseURL)
		key = strings.TrimPrefix(strings.TrimPrefix(p, strings.TrimRight(cdn.Path, "/")), "/")
	case s.publicURLHost != "" && host == s.publicURLHost && strings.HasPrefix(p, s.publicURLPath+MediaPathPrefix):
		key = strings.TrimPrefix(p, s.publicURLPath+MediaPathPrefix)
	case strings.HasPrefix(p, MediaPathPrefix):
		key = strings.TrimPrefix(p, MediaPathPrefix)
	case s.s3EndpointHost != "" && host == s.s3EndpointHost && s.s3Bucket != "" &&
		strings.HasPrefix(p, "/"+s.s3Bucket+"/"):
		key = strings.TrimPrefix(p, "/"+s.s3Bucket+"/")
	default:
		key = strings.TrimPrefix(p, "/")
	}
	if err := validateLooseKey(key); err != nil {
		return "", err
	}
	return key, nil
}

// IsOwnMediaURL reports whether raw is a same-origin media URL this instance
// serves: "/media/<key>" or "${PUBLIC_URL}/media/<key>" (same host), with a
// valid key and no query, fragment or credentials. Used where a URL must be
// trusted as ours (AI waiter image sanitizer, server-side asset reads).
func IsOwnMediaURL(raw string) bool {
	_, ok := ownMediaKey(raw)
	return ok
}

func ownMediaKey(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", false
	}
	prefix := MediaPathPrefix
	if u.Scheme != "" || u.Host != "" {
		if u.Scheme != "http" && u.Scheme != "https" {
			return "", false
		}
		s := snapshot()
		if s.publicURLHost == "" || strings.ToLower(u.Host) != s.publicURLHost {
			return "", false
		}
		prefix = s.publicURLPath + MediaPathPrefix
	} else if strings.HasPrefix(raw, "//") {
		return "", false
	}
	if !strings.HasPrefix(u.Path, prefix) {
		return "", false
	}
	key := strings.TrimPrefix(u.Path, prefix)
	if ValidateKey(key) != nil {
		return "", false
	}
	return key, true
}
