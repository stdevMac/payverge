package config

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"sync"
	"testing"
)

// TestConfig holds configuration for test environments
type TestConfig struct {
	DatabaseURL   string
	JWTSecret     string
	S3Bucket      string
	S3Region      string
	EmailProvider string
	TelegramToken string
}

var (
	testJWTSecretOnce sync.Once
	testJWTSecret     string
)

// ephemeralTestJWTSecret returns a random per-process secret for test runs when
// TEST_JWT_SECRET is not set. The secret is never a hardcoded constant, so it
// cannot bleed into production if accidentally loaded via test_config.
func ephemeralTestJWTSecret() string {
	testJWTSecretOnce.Do(func() {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			// Fall back to a process-unique marker; test setup will surface this via JWT init.
			testJWTSecret = "test-jwt-secret-unavailable"
			return
		}
		testJWTSecret = "test-" + hex.EncodeToString(buf)
	})
	return testJWTSecret
}

// NewTestConfig creates a new test configuration
func NewTestConfig() *TestConfig {
	return &TestConfig{
		DatabaseURL:   getEnvOrDefault("TEST_DATABASE_URL", "mongodb://localhost:27017/web3_boilerplate_test"),
		JWTSecret:     getEnvOrDefault("TEST_JWT_SECRET", ephemeralTestJWTSecret()),
		S3Bucket:      getEnvOrDefault("TEST_S3_BUCKET", "test-bucket"),
		S3Region:      getEnvOrDefault("TEST_S3_REGION", "us-east-1"),
		EmailProvider: getEnvOrDefault("TEST_EMAIL_PROVIDER", "mock"),
		TelegramToken: getEnvOrDefault("TEST_TELEGRAM_TOKEN", "mock-telegram-token"),
	}
}

// SetupTestEnvironment configures environment variables for testing
func SetupTestEnvironment(t *testing.T) {
	config := NewTestConfig()
	ClearTestURLConfig(t)

	// Set test environment variables
	t.Setenv("JWT_SECRET_KEY", config.JWTSecret)
	t.Setenv("MONGODB_URI", config.DatabaseURL)
	t.Setenv("S3_BUCKET", config.S3Bucket)
	t.Setenv("AWS_REGION", config.S3Region)
	t.Setenv("TELEGRAM_BOT_TOKEN", config.TelegramToken)

	// Disable external services in tests
	t.Setenv("DISABLE_POSTHOG", "true")
	t.Setenv("DISABLE_EMAIL", "true")
	t.Setenv("DISABLE_TELEGRAM", "true")
}

// ClearTestURLConfig prevents developer, CI runner, or shell production config
// from changing tests or causing an accidental provider call. t.Setenv restores
// every inherited value when the test scope ends.
func ClearTestURLConfig(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"FRONTEND_URL",
		"BASE_URL",
		"NEXT_PUBLIC_BASE_URL",
		"APP_BASE_URL",
		"PUBLIC_URL",
		"ALLOWED_REDIRECT_DOMAINS",
		"APP_DOMAIN",
		"OPENROUTER_API_KEY",
		"OPENROUTER_ZDR_MODE",
		"LLM_BASE_URL",
		"LLM_API_KEY",
		"EMAIL_API_KEY",
		"AWS_ACCESS_KEY",
		"AWS_SECRET_KEY",
		"AWS_PROTECTED_ACCESS_KEY",
		"AWS_PROTECTED_SECRET_KEY",
		"PLUGIN_SECRET_KEY",
		"STRIPE_WEBHOOK_SECRET",
		"PAYPAL_WEBHOOK_SECRET",
		"MERCADOPAGO_WEBHOOK_SECRET",
		"FISCAL_WSAA_URL",
		"FISCAL_WSFE_URL",
	} {
		t.Setenv(key, "")
	}
}

// getEnvOrDefault returns environment variable value or default if not set
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
