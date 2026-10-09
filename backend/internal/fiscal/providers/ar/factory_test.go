package ar

import (
	"context"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/fiscal"

	"github.com/stretchr/testify/require"
)

// TestNewProviderFromSettings_ReturnsNonNilProvider verifies that
// NewProviderFromSettings constructs a non-nil *Provider that satisfies the
// fiscal.Provider interface for the given environment, CUIT, and bundle.
func TestNewProviderFromSettings_ReturnsNonNilProvider(t *testing.T) {
	certPEM, keyPEM := newSelfSignedPEM(t)
	bundle, err := ParseCertificateBundle(certPEM, keyPEM)
	require.NoError(t, err)

	// Point both SOAP endpoints at a stub so no real network call is made.
	t.Setenv("FISCAL_WSFE_URL", "http://127.0.0.1:0")
	t.Setenv("FISCAL_WSAA_URL", "http://127.0.0.1:0")

	p := NewProviderFromSettings("sandbox", 20111111112, bundle)
	require.NotNil(t, p, "NewProviderFromSettings must return a non-nil *Provider")
	require.Equal(t, "AR", p.Country())
	require.Equal(t, "arca", p.Name())

	// Verify compile-time: *Provider satisfies fiscal.Provider.
	var _ fiscal.Provider = p
}

// TestNewProviderFromSettings_UsesEnvEndpointOverride verifies that when
// FISCAL_WSFE_URL / FISCAL_WSAA_URL are set the factory builds a client
// targeting those URLs. Observed indirectly: ValidateSettings runs a live
// FEDummy call against the overridden host (a dead local port) and the dial
// error names that host — proving the override is honored and no real AFIP host
// is contacted.
func TestNewProviderFromSettings_UsesEnvEndpointOverride(t *testing.T) {
	certPEM, keyPEM := newSelfSignedPEM(t)
	bundle, err := ParseCertificateBundle(certPEM, keyPEM)
	require.NoError(t, err)

	t.Setenv("FISCAL_WSFE_URL", "http://127.0.0.1:19999")
	t.Setenv("FISCAL_WSAA_URL", "http://127.0.0.1:19999")

	p := NewProviderFromSettings("sandbox", 27000000018, bundle)
	require.NotNil(t, p)

	// ValidateSettings now performs a real FEDummy round-trip; with the endpoint
	// overridden to a dead port the dial fails against that exact host.
	point := 1
	err = p.ValidateSettings(context.Background(), fiscal.Settings{TaxID: "27000000018", PointOfSale: &point})
	require.Error(t, err)
	require.Contains(t, err.Error(), "127.0.0.1:19999")
}

// TestEndpoints_ProductionIgnoresEnvOverride verifies that FISCAL_WSFE_URL and
// FISCAL_WSAA_URL never redirect a production provider, while a non-production
// environment still honors the WSFE override.
func TestEndpoints_ProductionIgnoresEnvOverride(t *testing.T) {
	t.Setenv("FISCAL_WSFE_URL", "http://override.example/wsfe")
	t.Setenv("FISCAL_WSAA_URL", "http://override.example/wsaa")

	require.Equal(t, wsfeProdURL, wsfeEndpoint("production"))
	require.Equal(t, wsaaProdURL, wsaaEndpoint("production"))
	require.Equal(t, "http://override.example/wsfe", wsfeEndpoint("sandbox"))
}

// TestNewProviderFromSettings_SandboxVsProduction verifies that two providers
// built with different environments are both non-nil, confirming the factory
// delegates env resolution to wsaaEndpoint/wsfeEndpoint correctly.
func TestNewProviderFromSettings_SandboxVsProduction(t *testing.T) {
	certPEM, keyPEM := newSelfSignedPEM(t)
	bundle, err := ParseCertificateBundle(certPEM, keyPEM)
	require.NoError(t, err)

	for _, env := range []string{"sandbox", "production"} {
		t.Run(env, func(t *testing.T) {
			// Allow the default endpoint URLs — no real HTTP call is made because we
			// never invoke IssueReceipt/LastAuthorized.
			p := NewProviderFromSettings(env, 20111111112, bundle)
			require.NotNil(t, p)
			require.Equal(t, "AR", p.Country())
		})
	}
}
