package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/blockchain"
	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/security"
)

// OSS self-host preflight wiring. main.go builds config.ProductionInputs from
// its flags; these helpers add the inputs and defaults the one-click deploy
// contract introduces (PUBLIC_URL, STORAGE_DRIVER, EDGE, SMTP_*, derived
// ALLOWED_ORIGINS / TRUSTED_PROXIES, default RPC, per-instance plugin key) so
// a production instance validates only what is actually configured.

// preflightPublicURL returns the raw PUBLIC_URL preflight validates, read the
// same way as config.PublicURL so preflight and the runtime never disagree.
func preflightPublicURL() string {
	raw, _, _ := config.PublicURLSetting()
	return raw
}

// preflightAllowedOrigin is the ALLOWED_ORIGINS default: the browser-facing
// origin config.PublicOrigins trusts (PUBLIC_URL; never APP_BASE_URL, the API
// origin), and only when the raw value also passes preflight's
// PUBLIC_URL rules, so an insecure or malformed URL derives nothing.
func preflightAllowedOrigin(publicURL string) (string, bool) {
	trusted := config.PublicOrigins()
	if len(trusted) == 0 {
		return "", false
	}
	if _, err := config.PublicOrigin(publicURL); err != nil {
		return "", false
	}
	return trusted[0], true
}

// applyOSSPreflightInputs completes the preflight inputs main.go resolved and
// applies the zero-config defaults before ValidateProduction runs:
//
//   - the email provider follows the shared config.EmailProvider() rule when
//     no flag picks one, and only resend/postmark resolve an API key; see
//     alignRuntimeEmailEnv for how the runtime is kept on the same values;
//   - ALLOWED_ORIGINS defaults to the trusted PUBLIC_URL origin
//     (preflightAllowedOrigin);
//   - TRUSTED_PROXIES defaults to loopback only (config.DefaultTrustedProxies,
//     decision D-1); a proxy on a container network or another host must be
//     listed explicitly, as the bundled compose files do.
//
// Defaults are written back to the process environment because the router
// reads ALLOWED_ORIGINS / TRUSTED_PROXIES from it later in main.
func applyOSSPreflightInputs(in *config.ProductionInputs, emailProviderFlag, emailAPIKeyFlag string) {
	in.PublicURL = preflightPublicURL()
	in.AdminPassword = strings.TrimSpace(os.Getenv(config.AdminPasswordEnv))
	in.RegistrationMode = os.Getenv(config.RegistrationModeEnv)
	in.StorageDriver = strings.TrimSpace(os.Getenv("STORAGE_DRIVER"))
	in.Edge = strings.TrimSpace(os.Getenv("EDGE"))
	in.SMTPHost = strings.TrimSpace(os.Getenv("SMTP_HOST"))
	in.SMTPPort = strings.TrimSpace(os.Getenv("SMTP_PORT"))
	in.SMTPUsername = strings.TrimSpace(os.Getenv("SMTP_USERNAME"))
	in.SMTPPassword = strings.TrimSpace(os.Getenv("SMTP_PASSWORD"))
	in.SMTPFrom = strings.TrimSpace(os.Getenv("SMTP_FROM"))

	provider := strings.ToLower(strings.TrimSpace(emailProviderFlag))
	if provider == "" {
		provider = config.EmailProvider()
	}
	in.EmailProvider = provider
	in.EmailAPIKey = config.PreflightEmailAPIKey(provider, emailAPIKeyFlag)
	alignRuntimeEmailEnv(provider, in.EmailAPIKey, emailProviderFlag, emailAPIKeyFlag)
	if in.Production {
		in.EmailTransportError = emailTransportError(provider, in.EmailAPIKey)
	}

	if strings.TrimSpace(os.Getenv("ALLOWED_ORIGINS")) == "" {
		if origin, ok := preflightAllowedOrigin(in.PublicURL); ok {
			_ = os.Setenv("ALLOWED_ORIGINS", origin)
			in.AllowedOrigins = origin
		}
	}
	if strings.TrimSpace(os.Getenv("TRUSTED_PROXIES")) == "" {
		_ = os.Setenv("TRUSTED_PROXIES", config.DefaultTrustedProxies)
		in.TrustedProxies = config.DefaultTrustedProxies
	}
}

// alignRuntimeEmailEnv makes the email transport main.go builds after
// preflight use exactly the provider and key preflight validated. The runtime
// reads --email-provider, then EMAIL_PROVIDER; and --email-api-key, then
// EMAIL_API_KEY. A provider selected only by RESEND_API_KEY, or a key that
// only lives there, is therefore
// mirrored into EMAIL_PROVIDER / EMAIL_API_KEY when those are empty. Explicit
// flags and env values are never overwritten, and providers without an API
// key (log, smtp) mirror nothing.
func alignRuntimeEmailEnv(provider, apiKey, emailProviderFlag, emailAPIKeyFlag string) {
	if provider != "resend" && provider != "postmark" {
		return
	}
	if strings.TrimSpace(emailProviderFlag) == "" && strings.TrimSpace(os.Getenv("EMAIL_PROVIDER")) == "" {
		_ = os.Setenv("EMAIL_PROVIDER", provider)
	}
	if apiKey != "" && strings.TrimSpace(emailAPIKeyFlag) == "" && strings.TrimSpace(os.Getenv("EMAIL_API_KEY")) == "" {
		_ = os.Setenv("EMAIL_API_KEY", apiKey)
	}
}

// emailTransportError asks the email provider factory whether this build can
// construct the resolved provider. Construction only builds an HTTP client
// (or reads SMTP_* settings); nothing is sent.
func emailTransportError(provider, apiKey string) string {
	if _, err := emails.NewProvider(provider, apiKey); err != nil {
		return err.Error()
	}
	return ""
}

// finishOSSPreflight runs after the fatal preflight gate passed: it surfaces
// the non-fatal warnings (optional integrations that fell back to defaults)
// and resolves the plugin encryption key for this instance.
func finishOSSPreflight(productionMode bool, report config.Report) {
	for _, warning := range report.Warnings() {
		logger.Logger.Warnf("PRODUCTION PREFLIGHT WARNING — %s [%s]: %s", warning.Code, warning.Component, warning.Message)
	}
	if err := ensureInstancePluginKey(productionMode, config.DataDir()); err != nil {
		logger.Logger.Fatalf("Plugin secret key: %v", err)
	}
}

// ensureInstancePluginKey resolves PLUGIN_SECRET_KEY. Production requires it;
// development generates a per-instance key on first boot and persists it
// (0600) under the data directory. The key value is never logged.
func ensureInstancePluginKey(productionMode bool, dataDir string) error {
	if !productionMode && strings.TrimSpace(os.Getenv("PLUGIN_SECRET_KEY")) == "" {
		storageDir := mediaStorageDir(productionMode)
		if config.PathWithin(filepath.Join(dataDir, "secrets"), storageDir) {
			return fmt.Errorf("DATA_DIR (%s) puts the generated plugin key inside STORAGE_DIR (%s), which is served over HTTP; "+
				"point DATA_DIR outside STORAGE_DIR or set PLUGIN_SECRET_KEY", dataDir, storageDir)
		}
	}
	status, err := security.EnsureDevPluginSecretKey(productionMode, dataDir)
	if err != nil {
		return err
	}
	switch status.Source {
	case security.PluginKeyGenerated:
		logger.Logger.Warnf("PLUGIN_SECRET_KEY is not set: generated a new per-instance plugin encryption key at %s (mode 0600). "+
			"Back this file up — losing it makes every stored payment-plugin credential unreadable. "+
			"Before going to production, set PLUGIN_SECRET_KEY explicitly (openssl rand -base64 32).", status.Path)
	case security.PluginKeyLoaded:
		logger.Logger.Infof("PLUGIN_SECRET_KEY is not set: using the per-instance development key at %s", status.Path)
	case security.PluginKeyEphemeral:
		logger.Logger.Errorf("PLUGIN_SECRET_KEY is not set and %s could not be written (%v): using an ephemeral in-memory key; "+
			"payment-plugin credentials saved by this process will not decrypt after a restart. Set PLUGIN_SECRET_KEY or DATA_DIR.",
			status.Path, status.PersistErr)
	}
	return nil
}

// mediaStorageDir is the local media directory per the env contract:
// STORAGE_DIR, else /data/storage in production and ./data/storage in
// development. Everything under it can be served by the media handler.
func mediaStorageDir(productionMode bool) string {
	if dir := strings.TrimSpace(os.Getenv("STORAGE_DIR")); dir != "" {
		return dir
	}
	if productionMode {
		return "/data/storage"
	}
	return filepath.Join("data", "storage")
}

// resolveSettlementRPCURL applies the Base mainnet default when RPC_URL is
// unset. A self-hosted instance needs no RPC account; an unreachable RPC only
// leaves crypto settlement unavailable.
func resolveSettlementRPCURL(raw string) string {
	if strings.TrimSpace(raw) == "" {
		logger.Logger.Infof("RPC_URL not set; using the public Base mainnet RPC %s for USDC verification", config.DefaultRPCURL)
	}
	return config.RPCURLOrDefault(raw)
}

// productionSettlementChainError reports a production instance whose RPC
// serves a chain other than Base mainnet. Testnet USDC is free to mint, so a
// production instance verifying on Base Sepolia (84532) would settle real
// bills for nothing. chainID 0 means the RPC was unreachable: crypto is
// already unavailable and startup only warned.
func productionSettlementChainError(productionMode bool, chainID int64) error {
	if !productionMode || chainID == 0 || chainID == config.ProductionSettlementChainID {
		return nil
	}
	return fmt.Errorf("RPC_URL serves chain id %d, but production settles guest USDC on Base mainnet (chain id %d) only; "+
		"point RPC_URL at a Base mainnet endpoint or unset it to use %s",
		chainID, config.ProductionSettlementChainID, config.DefaultRPCURL)
}

// enforceProductionSettlementChain is the startup fatal for
// productionSettlementChainError. Nil-safe: a nil service reports chain 0.
func enforceProductionSettlementChain(productionMode bool, svc *blockchain.BlockchainService) {
	if err := productionSettlementChainError(productionMode, svc.ChainID()); err != nil {
		logger.Logger.Fatalf("Production preflight failed: %v", err)
	}
}
