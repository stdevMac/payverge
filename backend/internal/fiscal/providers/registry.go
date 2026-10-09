// Package providers wires credential material from the database into concrete
// fiscal.Provider implementations. It is the only layer that knows about both
// the fiscal package (interfaces / credential helpers) and the country-specific
// sub-packages (ar, …).
//
// Import graph constraint: fiscal and fiscal/providers/ar must NOT import this
// package. Only callers higher in the stack (services, workers, handlers) do.
package providers

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/fiscal"
	"github.com/stdevmac/payverge/backend/internal/fiscal/providers/ar"
)

// compile-time assertion: *CredentialAwareFactory must satisfy fiscal.ProviderFactory.
var _ fiscal.ProviderFactory = (*CredentialAwareFactory)(nil)

// nonDigit strips all characters that are not ASCII digits.
var nonDigit = regexp.MustCompile(`[^0-9]`)

// CredentialAwareFactory implements fiscal.ProviderFactory. It decrypts the
// encrypted credentials stored on the BusinessFiscalSettings row, parses them
// into a country-specific form, and returns a ready-to-use fiscal.Provider.
//
// Key material is never logged or stored beyond the scope of Build.
type CredentialAwareFactory struct{}

// NewCredentialAwareFactory returns a CredentialAwareFactory ready to build
// providers for any supported country+provider combination.
func NewCredentialAwareFactory() *CredentialAwareFactory {
	return &CredentialAwareFactory{}
}

// Build constructs a fiscal.Provider from the business's persisted settings.
//
// It returns an error when:
//   - CredentialsEncrypted is nil or empty
//   - decryption or PEM parsing fails
//   - the country+provider combination is not supported
func (f *CredentialAwareFactory) Build(ctx context.Context, settings *database.BusinessFiscalSettings) (fiscal.Provider, error) {
	if len(settings.CredentialsEncrypted) == 0 {
		return nil, errors.New("fiscal: credentials are missing or empty")
	}

	bundle, err := fiscal.DecryptCredentialBundle(settings.CredentialsEncrypted)
	if err != nil {
		// Decrypt failure is deterministic: a corrupted or wrong-key ciphertext
		// cannot self-heal on retry. Wrap ErrPermanent so the worker stops
		// burning the retry budget and marks the job failed_permanent.
		return nil, fmt.Errorf("fiscal: decrypt credentials: %w: %w", err, fiscal.ErrPermanent)
	}
	if bundle.CertPEM == "" || bundle.KeyPEM == "" {
		// A successfully-decrypted but empty bundle is also a permanent config
		// error: re-running the job will yield the same empty bundle.
		return nil, fmt.Errorf("fiscal: decrypted bundle is missing cert or key PEM: %w", fiscal.ErrPermanent)
	}

	country := strings.ToUpper(strings.TrimSpace(settings.Country))
	provider := strings.ToLower(strings.TrimSpace(settings.Provider))

	switch {
	case country == "AR" && provider == "arca":
		return f.buildAR(settings.Environment, settings.TaxID, bundle)
	default:
		// Unknown country/provider combinations cannot succeed on retry — the
		// binary needs to be updated to support the combination. Permanent failure.
		return nil, fmt.Errorf("fiscal: unsupported country/provider combination: %s/%s: %w", country, provider, fiscal.ErrPermanent)
	}
}

// buildAR constructs the AR (ARCA/AFIP) provider from already-decrypted bundle material.
func (f *CredentialAwareFactory) buildAR(env, taxID string, bundle fiscal.CredentialBundle) (fiscal.Provider, error) {
	cuitStr := nonDigit.ReplaceAllString(taxID, "")
	if cuitStr == "" {
		return nil, fmt.Errorf("fiscal(AR): TaxID %q contains no digits", taxID)
	}
	cuit, err := strconv.ParseInt(cuitStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("fiscal(AR): parse CUIT from TaxID %q: %w", taxID, err)
	}

	certBundle, err := ar.ParseCertificateBundle([]byte(bundle.CertPEM), []byte(bundle.KeyPEM))
	if err != nil {
		return nil, fmt.Errorf("fiscal(AR): parse certificate bundle: %w", err)
	}

	return ar.NewProviderFromSettings(env, cuit, certBundle), nil
}
