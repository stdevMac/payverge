package main

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestResolveAllowedOrigins_IncludesLocalhostOutsideProduction(t *testing.T) {
	// Default test environment has no ENV var set, so this exercises the
	// dev-defaults branch in resolveAllowedOrigins.
	origins := resolveAllowedOrigins("https://payverge.io,https://www.payverge.io")
	assert.Contains(t, origins, "http://localhost:3000")
	assert.Contains(t, origins, "http://localhost:3001")
	assert.Contains(t, origins, "https://payverge.io")
	assert.Contains(t, origins, "https://www.payverge.io")
}

func TestResolveAllowedOrigins_ExcludesLocalhostInProduction(t *testing.T) {
	// CORS allows credentials, so an attacker-controlled localhost page must
	// not be a trusted origin in prod. See the security review round-3 finding.
	t.Setenv("ENV", "production")
	origins := resolveAllowedOrigins("https://payverge.io")
	assert.NotContains(t, origins, "http://localhost:3000")
	assert.NotContains(t, origins, "http://localhost:3001")
	assert.Contains(t, origins, "https://payverge.io")
}

func TestResolveAllowedOrigins_DedupesDefaults(t *testing.T) {
	origins := resolveAllowedOrigins("http://localhost:3000,https://payverge.io")
	count := 0
	for _, o := range origins {
		if o == "http://localhost:3000" {
			count++
		}
	}
	assert.Equal(t, 1, count, "localhost:3000 should appear exactly once")
}

func clearCORSPublicURLEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"PUBLIC_URL", "FRONTEND_URL", "BASE_URL", "NEXT_PUBLIC_BASE_URL", "APP_BASE_URL"} {
		t.Setenv(k, "")
	}
}

func TestResolveAllowedOrigins_EmptyEnvYieldsOnlyDefaults(t *testing.T) {
	clearCORSPublicURLEnv(t)
	t.Setenv("PUBLIC_URL", "https://payverge.io")
	origins := resolveAllowedOrigins("")
	assert.Contains(t, origins, "http://localhost:3000")
	assert.Contains(t, origins, "https://payverge.io")
	assert.NotContains(t, origins, "https://www.payverge.io", "no implicit www twin")
	// Outside production: exactly the PUBLIC_URL origin + 2 localhost defaults.
	assert.Len(t, origins, 3, "empty env outside production should yield 3 defaults")
}

// Self-host: the instance trusts its own PUBLIC_URL origin, never the
// upstream domain.
func TestResolveAllowedOrigins_DefaultsFollowPublicURL(t *testing.T) {
	clearCORSPublicURLEnv(t)
	t.Setenv("ENV", "production")
	t.Setenv("PUBLIC_URL", "https://eat.example.com")
	origins := resolveAllowedOrigins("")
	// Exactly the declared origin: an undeclared www twin (dangling or taken
	// over DNS) must never become a trusted credentialed origin.
	assert.Equal(t, []string{"https://eat.example.com"}, origins)
	for _, o := range origins {
		assert.NotContains(t, o, "payverge.io")
	}
}

// An operator who serves both apex and www opts in through ALLOWED_ORIGINS.
func TestResolveAllowedOrigins_TwinIsExplicitOptIn(t *testing.T) {
	clearCORSPublicURLEnv(t)
	t.Setenv("ENV", "production")
	t.Setenv("PUBLIC_URL", "https://eat.example.com")
	origins := resolveAllowedOrigins("https://www.eat.example.com")
	assert.Equal(t, []string{"https://eat.example.com", "https://www.eat.example.com"}, origins)
}

// APP_BASE_URL carries the API origin; with PUBLIC_URL unset neither it nor a
// retired alias may become a trusted browser origin.
func TestResolveAllowedOrigins_APIOriginKeysAreNotTrusted(t *testing.T) {
	clearCORSPublicURLEnv(t)
	t.Setenv("ENV", "production")
	t.Setenv("BASE_URL", "https://api.eat.example.com")
	t.Setenv("APP_BASE_URL", "https://api.eat.example.com")
	t.Setenv("NEXT_PUBLIC_BASE_URL", "https://api.eat.example.com")
	t.Setenv("FRONTEND_URL", "https://eat.example.com")
	assert.Empty(t, resolveAllowedOrigins(""))
}

func TestResolveAllowedOrigins_NoPublicURLHasNoUpstreamDefault(t *testing.T) {
	clearCORSPublicURLEnv(t)
	t.Setenv("ENV", "")
	origins := resolveAllowedOrigins("")
	assert.Equal(t, []string{"http://localhost:3000", "http://localhost:3001"}, origins)
}

func TestResolveAllowedOrigins_AddsCustomOrigin(t *testing.T) {
	origins := resolveAllowedOrigins("https://staging.payverge.io")
	assert.Contains(t, origins, "https://staging.payverge.io")
	assert.Contains(t, origins, "http://localhost:3000")
}

func TestResolveAllowedOrigins_TrimsWhitespace(t *testing.T) {
	origins := resolveAllowedOrigins("  https://staging.payverge.io  ,  https://preview.payverge.io  ")
	assert.Contains(t, origins, "https://staging.payverge.io")
	assert.Contains(t, origins, "https://preview.payverge.io")
}

func TestResolveAllowedOrigins_SkipsEmptyEntries(t *testing.T) {
	origins := resolveAllowedOrigins(",,https://staging.payverge.io,,")
	for _, o := range origins {
		assert.NotEmpty(t, o, "no entry should be empty")
	}
	assert.Contains(t, origins, "https://staging.payverge.io")
}

// Regression: a dev pointing APP_BASE_URL at a staging/preview HTTPS URL
// must not be misclassified as production. Previously, IsProduction inferred
// "prod" from any `https://` prefix on APP_BASE_URL, which silently dropped
// localhost from the CORS allowlist and broke local frontend dev (notably
// /api/v1/auth/session-info). Production is now explicit via ENV only.
func TestResolveAllowedOrigins_HttpsAppBaseUrlDoesNotDemoteToProd(t *testing.T) {
	t.Setenv("APP_BASE_URL", "https://api-staging.payverge.io")
	t.Setenv("ENV", "")
	origins := resolveAllowedOrigins("")
	assert.Contains(t, origins, "http://localhost:3000",
		"dev should retain localhost CORS even when APP_BASE_URL is https://")
	assert.Contains(t, origins, "http://localhost:3001")
}

// Env entries are validated: a bad ALLOWED_ORIGINS value is dropped with a
// warning instead of becoming a credentialed CORS origin.
func TestResolveAllowedOrigins_ValidatesEnvEntries(t *testing.T) {
	clearCORSPublicURLEnv(t)

	t.Run("http dropped in production", func(t *testing.T) {
		t.Setenv("ENV", "production")
		origins := resolveAllowedOrigins("http://x.example")
		assert.NotContains(t, origins, "http://x.example")
		assert.Empty(t, origins)
	})

	t.Run("http kept outside production", func(t *testing.T) {
		t.Setenv("ENV", "")
		origins := resolveAllowedOrigins("http://x.example")
		assert.Contains(t, origins, "http://x.example")
	})

	dropped := []string{
		"null",
		"NULL",
		"https://x.example/path",
		"https://x.example?q=1",
		"ftp://x.example",
		"https://user@x.example",
	}
	for _, envVal := range []string{"", "production"} {
		envVal := envVal
		t.Run("dropped when ENV="+envVal, func(t *testing.T) {
			t.Setenv("ENV", envVal)
			for _, bad := range dropped {
				origins := resolveAllowedOrigins(bad)
				assert.NotContains(t, origins, bad)
				for _, o := range origins {
					assert.NotContains(t, o, "x.example")
					assert.NotEqual(t, "null", o)
					assert.NotEqual(t, "NULL", o)
				}
			}
		})
	}

	t.Run("trailing slash normalized", func(t *testing.T) {
		t.Setenv("ENV", "production")
		assert.Equal(t, []string{"https://ok.example"}, resolveAllowedOrigins("https://ok.example/"))

		t.Setenv("ENV", "")
		origins := resolveAllowedOrigins("https://ok.example/")
		assert.Contains(t, origins, "https://ok.example")
		assert.NotContains(t, origins, "https://ok.example/")
	})
}

// Regression: docker-compose passes --production to the backend, which sets
// gin.SetMode(gin.ReleaseMode) for log-verbosity reasons. Previously,
// IsProduction also returned true whenever gin.Mode() == release, which
// silently dropped localhost from CORS for anyone running the local stack
// via docker-compose. Production is now explicit via ENV only.
func TestResolveAllowedOrigins_GinReleaseModeDoesNotDemoteToProd(t *testing.T) {
	originalMode := gin.Mode()
	gin.SetMode(gin.ReleaseMode)
	t.Cleanup(func() { gin.SetMode(originalMode) })
	t.Setenv("ENV", "")

	origins := resolveAllowedOrigins("")
	assert.Contains(t, origins, "http://localhost:3000",
		"dev should retain localhost CORS even when running in gin release mode")
	assert.Contains(t, origins, "http://localhost:3001")
}
