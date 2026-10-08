package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRedirectURL_AllowsRelativePath(t *testing.T) {
	t.Setenv("ALLOWED_REDIRECT_DOMAINS", "payverge.io,www.payverge.io")

	got, err := ValidateRedirectURL("/business/register/success", false)

	require.NoError(t, err)
	assert.Equal(t, "/business/register/success", got)
}

func TestValidateRedirectURL_RejectsProtocolRelative(t *testing.T) {
	_, err := ValidateRedirectURL("//evil.example/path", false)

	assert.Error(t, err)
}

func TestValidateRedirectURL_RejectsRelativePathsWithBackslashes(t *testing.T) {
	for _, raw := range []string{`/\evil.example/path`, `/\/evil.example`, `/\\evil.example`} {
		t.Run(raw, func(t *testing.T) {
			_, err := ValidateRedirectURL(raw, false)
			assert.Error(t, err)
		})
	}
}

func TestValidateRedirectURL_AllowsPayvergeHTTPS(t *testing.T) {
	t.Setenv("ALLOWED_REDIRECT_DOMAINS", "payverge.io,www.payverge.io")

	got, err := ValidateRedirectURL("https://payverge.io/payment/success", true)

	require.NoError(t, err)
	assert.Equal(t, "https://payverge.io/payment/success", got)
}

func TestValidateRedirectURL_RejectsExternalHTTPS(t *testing.T) {
	t.Setenv("ALLOWED_REDIRECT_DOMAINS", "payverge.io")

	_, err := ValidateRedirectURL("https://evil.example/payment/success", true)

	assert.Error(t, err)
}

func TestValidateRedirectURL_AllowsLocalhostOnlyOutsideProduction(t *testing.T) {
	got, err := ValidateRedirectURL("http://localhost:3000/payment/success", false)

	require.NoError(t, err)
	assert.Equal(t, "http://localhost:3000/payment/success", got)

	_, err = ValidateRedirectURL("http://localhost:3000/payment/success", true)
	assert.Error(t, err)
}

func TestValidateRedirectURL_RejectsProductionLoopbackEvenWhenAllowlisted(t *testing.T) {
	t.Setenv("ALLOWED_REDIRECT_DOMAINS", "localhost,localhost.,127.0.0.1,127.42.1.9,::1")

	for _, raw := range []string{
		"https://localhost/payment/success",
		"https://localhost./payment/success",
		"https://127.0.0.1/payment/success",
		"https://127.42.1.9/payment/success",
		"https://[::1]/payment/success",
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := ValidateRedirectURL(raw, true)
			assert.Error(t, err)
		})
	}
}

func TestValidateRedirectURL_RejectsProductionLoopbackFromAppDomain(t *testing.T) {
	t.Setenv("ALLOWED_REDIRECT_DOMAINS", "")
	t.Setenv("APP_DOMAIN", "localhost")

	_, err := ValidateRedirectURL("https://localhost/payment/success", true)

	assert.Error(t, err)
}

func TestValidateRedirectURL_DevelopmentHTTPSLocalhostRequiresAllowlist(t *testing.T) {
	t.Setenv("ALLOWED_REDIRECT_DOMAINS", "")
	t.Setenv("APP_DOMAIN", "")
	_, err := ValidateRedirectURL("https://localhost/payment/success", false)
	assert.Error(t, err)

	t.Setenv("ALLOWED_REDIRECT_DOMAINS", "localhost")
	got, err := ValidateRedirectURL("https://localhost/payment/success", false)
	require.NoError(t, err)
	assert.Equal(t, "https://localhost/payment/success", got)
}

func TestValidateRedirectURL_RejectsLocalhostWhenProductionOverrideSet(t *testing.T) {
	t.Setenv("ENV", "")
	t.Setenv("APP_ENV", "")
	SetProductionModeOverride(true)
	t.Cleanup(func() {
		SetProductionModeOverride(false)
	})

	_, err := ValidateRedirectURL("http://localhost:3000/payment/success", IsProductionMode(false))

	assert.Error(t, err)
}

func TestValidateRedirectURL_AppendsAppDomainDefault(t *testing.T) {
	t.Setenv("ALLOWED_REDIRECT_DOMAINS", "")
	t.Setenv("APP_DOMAIN", "app.payverge.io")

	got, err := ValidateRedirectURL("https://app.payverge.io/payment/success", true)

	require.NoError(t, err)
	assert.Equal(t, "https://app.payverge.io/payment/success", got)
}

func TestValidateRedirectURL_NormalizesAllowedRedirectDomains(t *testing.T) {
	for _, allowed := range []string{"https://payverge.io", "payverge.io:443"} {
		t.Run(allowed, func(t *testing.T) {
			t.Setenv("ALLOWED_REDIRECT_DOMAINS", allowed)

			got, err := ValidateRedirectURL("https://payverge.io/payment/success", true)

			require.NoError(t, err)
			assert.Equal(t, "https://payverge.io/payment/success", got)
		})
	}
}

func TestValidateRedirectURL_RejectsMalformedAndMissingHost(t *testing.T) {
	for _, raw := range []string{"", "   ", "https://", "payment/success", "mailto:ops@payverge.io"} {
		t.Run(raw, func(t *testing.T) {
			_, err := ValidateRedirectURL(raw, false)
			assert.Error(t, err)
		})
	}
}

func TestValidateRedirectURL_AllowsRelativePathWithQuery(t *testing.T) {
	got, err := ValidateRedirectURL("/payment/success?bill=B1", true)

	require.NoError(t, err)
	assert.Equal(t, "/payment/success?bill=B1", got)
}

func TestFrontendURLBuildsAbsoluteURL(t *testing.T) {
	t.Setenv("PUBLIC_URL", "https://payverge.io/")

	got := FrontendURL("/payment/success?bill=B1")

	assert.Equal(t, "https://payverge.io/payment/success?bill=B1", got)
}

func TestAbsoluteRedirectURL_ConvertsRelativePathToFrontendURL(t *testing.T) {
	t.Setenv("PUBLIC_URL", "https://payverge.io/")
	t.Setenv("ALLOWED_REDIRECT_DOMAINS", "payverge.io")

	got, err := AbsoluteRedirectURL("/payment/success?bill=B1", true)

	require.NoError(t, err)
	assert.Equal(t, "https://payverge.io/payment/success?bill=B1", got)
}

func TestAbsoluteRedirectURL_RejectsLocalhostFrontendURLInProduction(t *testing.T) {
	t.Setenv("PUBLIC_URL", "http://localhost:3000")

	_, err := AbsoluteRedirectURL("/payment/success?bill=B1", true)

	assert.Error(t, err)
}
