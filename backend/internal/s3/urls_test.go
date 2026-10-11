package s3

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublicURLContract(t *testing.T) {
	restore := SetPublicURL("")
	defer restore()
	require.Equal(t, "/media/businesses/1/logo.png", PublicURL("businesses/1/logo.png"))
	require.Equal(t, "/media/businesses/1/logo.png", PublicURL("/businesses/1/logo.png"))

	SetPublicURL("https://pos.example.test/")
	require.Equal(t, "https://pos.example.test/media/a/b.png", PublicURL("a/b.png"))

	// A sub-path deployment keeps its prefix.
	SetPublicURL("https://example.test/payverge")
	require.Equal(t, "https://example.test/payverge/media/a/b.png", PublicURL("a/b.png"))

	// Garbage PUBLIC_URL degrades to relative URLs instead of emitting junk.
	SetPublicURL("javascript:alert(1)")
	require.Equal(t, "/media/a/b.png", PublicURL("a/b.png"))
	SetPublicURL("https://user:pass@example.test")
	require.Equal(t, "/media/a/b.png", PublicURL("a/b.png"))

	// The s3 driver with a CDN base keeps the CDN contract (production).
	SetPublicURL("https://pos.example.test")
	updateState(func(s *storageState) { s.cdnBaseURL = "https://images.example.test" })
	defer updateState(func(s *storageState) { s.cdnBaseURL = "" })
	require.Equal(t, "https://images.example.test/a/b.png", PublicURL("a/b.png"))
}

func TestKeyFromURLAcceptsEveryStoredShape(t *testing.T) {
	restoreURL := SetPublicURL("https://pos.example.test")
	defer restoreURL()
	updateState(func(s *storageState) {
		s.cdnBaseURL = "https://images.example.test/base"
		s.s3EndpointHost = "minio.example.test:9000"
		s.s3Bucket = "payverge"
	})
	defer updateState(func(s *storageState) { s.cdnBaseURL, s.s3EndpointHost, s.s3Bucket = "", "", "" })

	cases := map[string]string{
		"businesses/1/menu_items/a.png":                                     "businesses/1/menu_items/a.png",
		"/businesses/1/menu_items/a.png":                                    "businesses/1/menu_items/a.png",
		"/media/businesses/1/menu_items/a.png":                              "businesses/1/menu_items/a.png",
		"https://pos.example.test/media/businesses/1/a.png":                 "businesses/1/a.png",
		"https://other-origin.test/media/businesses/1/a.png":                "businesses/1/a.png",
		"https://images.example.test/base/businesses/1/a.png":               "businesses/1/a.png",
		"https://payverge.s3.us-east-1.amazonaws.com/businesses/1/a.png":    "businesses/1/a.png",
		"https://payverge.j5f7.c18.e2-3.dev/businesses/1/a.png":             "businesses/1/a.png",
		"http://minio.example.test:9000/payverge/businesses/1/a.png":        "businesses/1/a.png",
		"https://payverge.s3.amazonaws.com/businesses/1/Menu%20Photo.png":   "businesses/1/Menu Photo.png",
		"https://pos.example.test/media/businesses/1/a.png?v=2#frag":        "businesses/1/a.png",
		"  https://pos.example.test/media/businesses/1/a.png  ":             "businesses/1/a.png",
		"https://images.example.test/base/demo-arg/assets/carta/flan.jpg":   "demo-arg/assets/carta/flan.jpg",
		"https://pos.example.test/media/demo-arg/assets/venues/hero-01.jpg": "demo-arg/assets/venues/hero-01.jpg",
	}
	for raw, want := range cases {
		got, err := KeyFromURL(raw)
		require.NoError(t, err, raw)
		require.Equal(t, want, got, raw)
	}

	for _, raw := range []string{
		"",
		"   ",
		"/media/",
		"/media/../etc/passwd",
		"/media/a/../../etc/passwd",
		"https://pos.example.test/media/%2e%2e/%2e%2e/etc/passwd",
		"https://pos.example.test/media/a/%2e%2e/b",
		"../etc/passwd",
		"a/../../b",
		"file:///etc/passwd",
		"javascript:alert(1)",
		"https://host.test/",
		"https://host.test/a//b",
		`a\..\b`,
		"a/\x00/b",
	} {
		_, err := KeyFromURL(raw)
		require.ErrorIs(t, err, ErrInvalidKey, "%q", raw)
	}
}

func TestIsOwnMediaURL(t *testing.T) {
	restore := SetPublicURL("https://pos.example.test")
	defer restore()
	for _, ok := range []string{
		"/media/businesses/1/a.png",
		"https://pos.example.test/media/businesses/1/a.png",
		"HTTPS://POS.EXAMPLE.TEST/media/businesses/1/a.png",
		"http://pos.example.test/media/a.png",
	} {
		require.True(t, IsOwnMediaURL(ok), ok)
	}
	for _, bad := range []string{
		"",
		"//evil.test/media/a.png",
		"https://evil.test/media/a.png",
		"https://pos.example.test.evil.test/media/a.png",
		"https://user@pos.example.test/media/a.png",
		"https://pos.example.test/media/a.png?x=1",
		"https://pos.example.test/media/a.png#x",
		"https://pos.example.test/api/v1/a.png",
		"/media/../etc/passwd",
		"/media/a%20b.png",
		"javascript:/media/a.png",
		"data:image/png;base64,AAAA",
		"/mediaX/a.png",
	} {
		require.False(t, IsOwnMediaURL(bad), bad)
	}

	// Without PUBLIC_URL only relative URLs are ours.
	SetPublicURL("")
	require.True(t, IsOwnMediaURL("/media/a.png"))
	require.False(t, IsOwnMediaURL("https://pos.example.test/media/a.png"))

	// Sub-path deployments.
	SetPublicURL("https://example.test/pv")
	require.True(t, IsOwnMediaURL("https://example.test/pv/media/a.png"))
	require.False(t, IsOwnMediaURL("https://example.test/media/a.png"))
}

func TestSafeKeySegment(t *testing.T) {
	cases := map[string]string{
		"Milanesa Napolitana":      "milanesa_napolitana",
		"Ñoquis de papá":           "noquis_de_papa",
		"  ../../etc/passwd  ":     "etc_passwd",
		"Bife (400 g) — con hueso": "bife_400_g_con_hueso",
		"":                         "item",
		"!!!":                      "item",
		"café-crème":               "cafe-creme",
		"漢字":                       "item",
	}
	for in, want := range cases {
		got := SafeKeySegment(in)
		require.Equal(t, want, got, in)
		require.NoError(t, ValidateKey(got), in)
	}
	long := SafeKeySegment(string(make([]byte, 0)) + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	require.LessOrEqual(t, len(long), 64)
}

func TestValidateKeyAcceptsGeneratedShapes(t *testing.T) {
	for _, key := range []string{
		"businesses/12/menu_items/0123456789abcdef_milanesa.png",
		"menu_items/ai_generated/12/20260101T000000_0123456789abcdef.webp",
		"fiscal-receipts/1/2/550e8400-e29b-41d4-a716-446655440000.pdf",
		"demo-arg/assets/carta/bife-de-chorizo.jpg",
		"a",
	} {
		require.NoError(t, ValidateKey(key), key)
	}
	require.Equal(t, "folder/name.png", JoinKey("/folder/", "name.png"))
	require.Equal(t, "name.png", JoinKey("", "name.png"))
}
