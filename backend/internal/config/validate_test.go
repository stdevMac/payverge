package config

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
)

func hasValidationError(errors []ValidationError, field string) bool {
	for _, err := range errors {
		if err.Field == field {
			return true
		}
	}
	return false
}

func TestValidateConfig_RequiresJWTSecretKey(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "")

	errors := ValidateConfig("localhost", "user", "password", "payverge", false)

	assert.True(t, hasValidationError(errors, "JWT_SECRET_KEY"))
}

func TestValidateConfig_RejectsShortJWTSecretKey(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "short-secret")

	errors := ValidateConfig("localhost", "user", "password", "payverge", false)

	assert.True(t, hasValidationError(errors, "JWT_SECRET_KEY"))
}

func TestValidateConfig_AcceptsStrongJWTSecretKey(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "0123456789abcdefghijklmnopqrstuv")

	errors := ValidateConfig("localhost", "user", "password", "payverge", false)

	assert.False(t, hasValidationError(errors, "JWT_SECRET_KEY"))
}

func TestValidateConfig_RejectsKnownDatabasePasswordDefaultInProduction(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "an-acceptably-long-production-jwt-secret-key-value")
	t.Setenv("PLUGIN_SECRET_KEY", "0123456789abcdef0123456789abcdef")

	errors := ValidateConfig("postgres", "payverge", "payverge_password", "payverge", true)

	assert.True(t, hasValidationError(errors, "db-password"))
}

func TestValidateConfig_AllowsDatabasePasswordDefaultOutsideProduction(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "an-acceptably-long-production-jwt-secret-key-value")
	t.Setenv("PLUGIN_SECRET_KEY", "")
	t.Setenv("ENV", "development")
	t.Setenv("APP_ENV", "")

	errors := ValidateConfig("postgres", "payverge", "payverge_password", "payverge", false)

	assert.False(t, hasValidationError(errors, "db-password"))
}

func TestValidateConfig_ProductionRequiresPluginSecretKeyUnconditionally(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "an-acceptably-long-production-jwt-secret-key-value")
	t.Setenv("PLUGIN_SECRET_KEY", "")

	errs := ValidateConfig("db-host", "db-user", "db-pass", "db-name", true)

	found := false
	for _, e := range errs {
		if e.Field == "PLUGIN_SECRET_KEY" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected PLUGIN_SECRET_KEY validation error in production mode, got: %+v", errs)
	}
}

func TestValidateConfig_DevDoesNotRequirePluginSecretKey(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "an-acceptably-long-production-jwt-secret-key-value")
	t.Setenv("PLUGIN_SECRET_KEY", "")
	t.Setenv("ENV", "")
	t.Setenv("APP_ENV", "")

	errs := ValidateConfig("db-host", "db-user", "db-pass", "db-name", false)
	for _, e := range errs {
		if e.Field == "PLUGIN_SECRET_KEY" {
			t.Fatalf("dev mode must not require PLUGIN_SECRET_KEY, got: %+v", errs)
		}
	}
}

func TestValidateConfig_AcceptsBase64PluginSecretKeyInProduction(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "0123456789abcdefghijklmnopqrstuv")
	t.Setenv("PLUGIN_SECRET_KEY", base64.StdEncoding.EncodeToString([]byte(unitPluginKeyRaw)))

	errors := ValidateConfig("localhost", "user", "password", "payverge", true)

	assert.False(t, hasValidationError(errors, "PLUGIN_SECRET_KEY"))
}

// A key that IS set but malformed silently breaks credential encrypt/decrypt
// for fiscal and every plugin secret — reject it at boot.
func TestValidateConfig_RejectsMalformedPluginSecretKeyInProduction(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "0123456789abcdefghijklmnopqrstuv")
	t.Setenv("PLUGIN_SECRET_KEY", "too-short")

	errors := ValidateConfig("localhost", "user", "password", "payverge", true)

	assert.True(t, hasValidationError(errors, "PLUGIN_SECRET_KEY"))
}

func TestValidateConfig_RejectsKnownDevJWTSecretInProduction(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "payverge_local_dev_jwt_secret_2026")
	t.Setenv("ENV", "production")

	errors := ValidateConfig("localhost", "user", "password", "payverge", false)

	assert.True(t, hasValidationError(errors, "JWT_SECRET_KEY"))
}

func TestValidateConfig_AllowsKnownDevJWTSecretOutsideProduction(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "payverge_local_dev_jwt_secret_2026")
	t.Setenv("ENV", "development")
	t.Setenv("APP_ENV", "")

	errors := ValidateConfig("localhost", "user", "password", "payverge", false)

	assert.False(t, hasValidationError(errors, "JWT_SECRET_KEY"))
}

func TestValidateConfig_RejectsExamplePlaceholderJWTSecretInProduction(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "replace_with_at_least_32_random_characters_1234567890")
	t.Setenv("ENV", "production")
	t.Setenv("APP_ENV", "")

	errors := ValidateConfig("localhost", "user", "password", "payverge", false)

	assert.True(t, hasValidationError(errors, "JWT_SECRET_KEY"))
}

func TestValidateConfig_RejectsExamplePlaceholderJWTSecretWithFlagInProduction(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "replace_with_at_least_32_random_characters_1234567890")
	t.Setenv("ENV", "development")

	errors := ValidateConfig("localhost", "user", "password", "payverge", true)

	assert.True(t, hasValidationError(errors, "JWT_SECRET_KEY"))
}

func TestValidateConfig_AcceptsRandom53CharSecretInProduction(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4y5z6A7B8C9")
	t.Setenv("ENV", "production")

	errors := ValidateConfig("localhost", "user", "password", "payverge", false)

	assert.False(t, hasValidationError(errors, "JWT_SECRET_KEY"))
}

func TestIsKnownUnsafeJWTSecret_RejectsReplaceWithPlaceholders(t *testing.T) {
	assert.True(t, isKnownUnsafeJWTSecret("replace_with_at_least_32_random_characters_1234567890"))
	assert.True(t, isKnownUnsafeJWTSecret("replace_with_anything"))
	assert.False(t, isKnownUnsafeJWTSecret("a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6q7r8s9t0u1v2w3x4y5z6A7B8C9"))
}

func TestIsProductionMode_UsesFlagOrEnv(t *testing.T) {
	t.Setenv("ENV", "")
	t.Setenv("APP_ENV", "")
	assert.True(t, IsProductionMode(true))

	t.Setenv("ENV", "production")
	assert.True(t, IsProductionMode(false))

	t.Setenv("ENV", "")
	t.Setenv("APP_ENV", "production")
	assert.True(t, IsProductionMode(false))
}

func TestIsProductionMode_UsesOverride(t *testing.T) {
	t.Setenv("ENV", "")
	t.Setenv("APP_ENV", "")
	SetProductionModeOverride(true)
	t.Cleanup(func() {
		SetProductionModeOverride(false)
	})

	assert.True(t, IsProductionMode(false))
}

func TestIsProductionMode_NormalizesEnvAliases(t *testing.T) {
	tests := []struct {
		name   string
		env    string
		appEnv string
	}{
		{name: "trims ENV", env: " production "},
		{name: "uppercases ENV", env: "PRODUCTION"},
		{name: "mixed case ENV", env: "ProDucTion"},
		{name: "prod ENV alias", env: "prod"},
		{name: "trims APP_ENV", appEnv: " prod "},
		{name: "uppercases APP_ENV", appEnv: "PROD"},
		{name: "mixed case APP_ENV", appEnv: "PrOd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ENV", tt.env)
			t.Setenv("APP_ENV", tt.appEnv)

			assert.True(t, IsProductionMode(false))
		})
	}
}

func hasWarning(warnings []ValidationError, field string) bool {
	for _, w := range warnings {
		if w.Field == field {
			return true
		}
	}
	return false
}

func TestProductionWarnings_EmptyOutsideProduction(t *testing.T) {
	t.Setenv("ENV", "development")
	t.Setenv("APP_ENV", "")
	t.Setenv("PLUGIN_SECRET_KEY", "")
	t.Setenv("EMAIL_PROVIDER", "")
	t.Setenv("EMAIL_API_KEY", "")

	warnings := ProductionWarnings(false, "", "")

	assert.Empty(t, warnings)
}

func TestProductionWarnings_NeverWarnsPluginSecretKey(t *testing.T) {
	// PLUGIN_SECRET_KEY is a hard ValidateConfig requirement in production
	// (see TestValidateConfig_* above) — ProductionWarnings must not
	// duplicate it as a warning.
	t.Setenv("PLUGIN_SECRET_KEY", "")
	t.Setenv("EMAIL_PROVIDER", "")
	t.Setenv("EMAIL_API_KEY", "")

	warnings := ProductionWarnings(true, "", "re_live_key_123")

	assert.False(t, hasWarning(warnings, "PLUGIN_SECRET_KEY"))
}

func TestProductionWarnings_WarnsWhenNoEmailAPIKeyInProduction(t *testing.T) {
	t.Setenv("PLUGIN_SECRET_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("EMAIL_PROVIDER", "resend")
	t.Setenv("EMAIL_API_KEY", "")

	warnings := ProductionWarnings(true, "", "")

	assert.True(t, hasWarning(warnings, "EMAIL_API_KEY"))
}

func TestProductionWarnings_AcceptsEmailKeyFromFlag(t *testing.T) {
	t.Setenv("PLUGIN_SECRET_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("EMAIL_PROVIDER", "")
	t.Setenv("EMAIL_API_KEY", "")

	warnings := ProductionWarnings(true, "resend", "re_live_key_123")

	assert.False(t, hasWarning(warnings, "EMAIL_API_KEY"))
}

func TestProductionWarnings_AcceptsEmailKeyFromEnvFallback(t *testing.T) {
	t.Setenv("PLUGIN_SECRET_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("EMAIL_PROVIDER", "resend")
	t.Setenv("EMAIL_API_KEY", "re_live_key_123")

	warnings := ProductionWarnings(true, "", "")

	assert.False(t, hasWarning(warnings, "EMAIL_API_KEY"))
}

func TestProductionWarnings_EmailKeyFlagAloneSelectsNoProvider(t *testing.T) {
	clearEmailEnv(t)

	// --email-api-key alone is mirrored into EMAIL_API_KEY, which does not
	// pick a provider: the transport is log, which takes no key. The log sink
	// itself is reported by ValidateProduction (email.provider.log), not here.
	warnings := ProductionWarnings(true, "", "re_live_key_123")
	assert.Empty(t, warnings)

	t.Setenv("EMAIL_API_KEY", "re_live_key_123")
	assert.Empty(t, ProductionWarnings(true, "", ""))
}

func TestProductionWarnings_WarnsOnNonResendLookingKeyWhenProviderResend(t *testing.T) {
	t.Setenv("PLUGIN_SECRET_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("EMAIL_PROVIDER", "")
	t.Setenv("EMAIL_API_KEY", "")

	// A Postmark-shaped token while EMAIL_PROVIDER=resend is the exact
	// misconfiguration a provider rollback/rollforward would produce.
	warnings := ProductionWarnings(true, "resend", "01234567-89ab-cdef-0123-456789abcdef")

	assert.True(t, hasWarning(warnings, "EMAIL_API_KEY"))
}

func TestValidateConfig_RejectsPublishedSecretsInProduction(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("APP_ENV", "")

	t.Setenv("JWT_SECRET_KEY", "payverge_local_dev_jwt_secret_2026_min32")
	t.Setenv("PLUGIN_SECRET_KEY", "payverge-local-plugin-secret-key")
	errors := ValidateConfig("localhost", "user", "unit-db-password-8c4f1d907e2a", "payverge", false)
	assert.True(t, hasValidationError(errors, "JWT_SECRET_KEY"))
	assert.True(t, hasValidationError(errors, "PLUGIN_SECRET_KEY"))

	errors = ValidateConfig("localhost", "user", "payverge_password", "payverge", false)
	assert.True(t, hasValidationError(errors, "db-password"))
}

func TestValidateConfig_DevelopmentAllowsDevDefaults(t *testing.T) {
	t.Setenv("ENV", "development")
	t.Setenv("APP_ENV", "")
	t.Setenv("JWT_SECRET_KEY", "payverge_local_dev_jwt_secret_2026_min32")
	t.Setenv("PLUGIN_SECRET_KEY", "")

	errors := ValidateConfig("localhost", "user", "payverge_password", "payverge", false)

	assert.Empty(t, errors)
}

func TestProductionWarnings_FollowsSharedEmailProviderRule(t *testing.T) {
	for _, key := range []string{"EMAIL_PROVIDER", "EMAIL_API_KEY", "RESEND_API_KEY"} {
		t.Setenv(key, "")
	}

	// Nothing configured selects the log sink, which takes no API key.
	assert.Empty(t, ProductionWarnings(true, "", ""))
	assert.Empty(t, ProductionWarnings(true, "smtp", ""))

	// RESEND_API_KEY both selects resend and satisfies the key requirement.
	t.Setenv("RESEND_API_KEY", "re_live_key_123")
	assert.Empty(t, ProductionWarnings(true, "", ""))

	// An explicit provider without its key still warns.
	t.Setenv("RESEND_API_KEY", "")
	t.Setenv("EMAIL_PROVIDER", "postmark")
	assert.True(t, hasWarning(ProductionWarnings(true, "", ""), "EMAIL_API_KEY"))
}

func TestProductionWarnings_ResendAliasKeyCounts(t *testing.T) {
	clearEmailEnv(t)
	t.Setenv("RESEND_API_KEY", "re_live_key_123")

	warnings := ProductionWarnings(true, "", "")

	assert.Empty(t, warnings)
}

// Decision D-2: production validation accepts 64 hex characters.
func TestIsValidPluginSecretKey_AcceptsHex(t *testing.T) {
	hexKey := "00112233445566778899aabbccddeeff00112233445566778899AABBCCDDEEFF"
	assert.True(t, isValidPluginSecretKey(hexKey))
	assert.True(t, isValidPluginSecretKey(base64.StdEncoding.EncodeToString(make([]byte, 32))))
	assert.False(t, isValidPluginSecretKey(hexKey[:62]))
	assert.False(t, isValidPluginSecretKey(hexKey+"00"))
	assert.False(t, isValidPluginSecretKey("g"+hexKey[1:]))
}
