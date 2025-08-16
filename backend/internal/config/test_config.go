package config

import (
	"os"
	"testing"
)

// TestConfig holds configuration for test environments
type TestConfig struct {
	DatabaseURL    string
	JWTSecret      string
	S3Bucket       string
	S3Region       string
	EmailProvider  string
	TelegramToken  string
}

// NewTestConfig creates a new test configuration
func NewTestConfig() *TestConfig {
	return &TestConfig{
		DatabaseURL:   getEnvOrDefault("TEST_DATABASE_URL", "mongodb://localhost:27017/web3_boilerplate_test"),
		JWTSecret:     getEnvOrDefault("TEST_JWT_SECRET", "test-secret-key-for-testing-only"),
		S3Bucket:      getEnvOrDefault("TEST_S3_BUCKET", "test-bucket"),
		S3Region:      getEnvOrDefault("TEST_S3_REGION", "us-east-1"),
		EmailProvider: getEnvOrDefault("TEST_EMAIL_PROVIDER", "mock"),
		TelegramToken: getEnvOrDefault("TEST_TELEGRAM_TOKEN", "mock-telegram-token"),
	}
}

// SetupTestEnvironment configures environment variables for testing
func SetupTestEnvironment(t *testing.T) {
	config := NewTestConfig()
	
	// Set test environment variables
	os.Setenv("JWT_SECRET_KEY", config.JWTSecret)
	os.Setenv("MONGODB_URI", config.DatabaseURL)
	os.Setenv("S3_BUCKET", config.S3Bucket)
	os.Setenv("AWS_REGION", config.S3Region)
	os.Setenv("TELEGRAM_BOT_TOKEN", config.TelegramToken)
	
	// Disable external services in tests
	os.Setenv("DISABLE_POSTHOG", "true")
	os.Setenv("DISABLE_EMAIL", "true")
	os.Setenv("DISABLE_TELEGRAM", "true")
	
	// Cleanup function
	t.Cleanup(func() {
		os.Unsetenv("JWT_SECRET_KEY")
		os.Unsetenv("MONGODB_URI")
		os.Unsetenv("S3_BUCKET")
		os.Unsetenv("AWS_REGION")
		os.Unsetenv("TELEGRAM_BOT_TOKEN")
		os.Unsetenv("DISABLE_POSTHOG")
		os.Unsetenv("DISABLE_EMAIL")
		os.Unsetenv("DISABLE_TELEGRAM")
	})
}

// getEnvOrDefault returns environment variable value or default if not set
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// IsTestEnvironment checks if we're running in test mode
func IsTestEnvironment() bool {
	return os.Getenv("GO_ENV") == "test" || 
		   os.Getenv("GIN_MODE") == "test" ||
		   testing.Testing()
}

// MockExternalServices disables external service calls for testing
func MockExternalServices() {
	os.Setenv("MOCK_EXTERNAL_SERVICES", "true")
	os.Setenv("DISABLE_POSTHOG", "true")
	os.Setenv("DISABLE_EMAIL", "true")
	os.Setenv("DISABLE_TELEGRAM", "true")
	os.Setenv("DISABLE_S3", "true")
	os.Setenv("DISABLE_DATABASE", "true")
}
