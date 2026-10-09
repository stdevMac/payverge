package structs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetSecretKey_UsesPresetSecretKey(t *testing.T) {
	original := SecretKey
	SecretKey = []byte("preset-secret")
	t.Cleanup(func() {
		SecretKey = original
	})
	t.Setenv("JWT_SECRET_KEY", "")

	assert.Equal(t, []byte("preset-secret"), GetSecretKey())
}

func TestGetSecretKey_LoadsFromEnvironmentWhenUnset(t *testing.T) {
	original := SecretKey
	SecretKey = nil
	t.Cleanup(func() {
		SecretKey = original
	})
	t.Setenv("JWT_SECRET_KEY", "env-secret")

	assert.Equal(t, []byte("env-secret"), GetSecretKey())
	assert.Equal(t, []byte("env-secret"), SecretKey)
}

func TestGetSecretKey_PanicsWhenRequestedWithoutConfiguration(t *testing.T) {
	original := SecretKey
	SecretKey = nil
	t.Cleanup(func() {
		SecretKey = original
	})
	t.Setenv("JWT_SECRET_KEY", "")

	assert.PanicsWithValue(t, "JWT_SECRET_KEY environment variable is required", func() {
		GetSecretKey()
	})
}
