package ar

// NewProviderFromSettings builds a *Provider from already-parsed credential
// material. It resolves the WSAA and WSFE endpoint URLs from the environment
// string ("production" → live AFIP hosts; anything else → homologation hosts).
// Non-production environments can override either endpoint at runtime via
// FISCAL_WSAA_URL / FISCAL_WSFE_URL (tests and homologación). Production always
// uses the official AFIP hosts and ignores those overrides.
//
// The caller is responsible for obtaining the CertificateBundle (i.e.
// decrypting and parsing the stored PEM bytes before calling this function) and
// the issuer CUIT (already parsed to int64). Key material inside bundle must
// never be logged. The CUIT is required by GetStatus to build the WSFE <Auth>
// header for FECompConsultar.
func NewProviderFromSettings(env string, cuit int64, bundle *CertificateBundle) *Provider {
	wsaa := NewWSAAClient(wsaaEndpoint(env), bundle.Cert, bundle.Key)
	wsfe := NewWSFEClient(wsfeEndpoint(env), wsaa)
	return NewProvider(wsfe, cuit)
}
