package config

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// minimalSelfHostInputs is the smallest production configuration the OSS
// one-click deploy promises: PUBLIC_URL, DB_PASSWORD, JWT_SECRET_KEY,
// PLUGIN_SECRET_KEY, ADMIN_EMAIL, ADMIN_PASSWORD (plus DB host/user/name,
// which ValidateConfig owns). No email, storage, RPC, CDN, or AI accounts.
func minimalSelfHostInputs() ProductionInputs {
	return ProductionInputs{
		Production:      true,
		PublicURL:       "https://restaurant.example.com",
		DBPassword:      "matrix-db-password-3f9a1c7e5b",
		JWTSecretKey:    "matrix-jwt-secret-0123456789abcdefghijklmnopqrstuvwxyz",
		PluginSecretKey: base64.StdEncoding.EncodeToString([]byte("matrix-plugin-key-32-bytes-long!")),
		AdminPassword:   "matrix-admin-password-71d0",
	}
}

func TestOSSPreflightMatrix(t *testing.T) {
	tests := []struct {
		name         string
		mutate       func(*ProductionInputs)
		wantCodes    []string
		wantWarnings []string
	}{
		{
			name:   "minimal six-variable production deploy passes",
			mutate: func(*ProductionInputs) {},
			// Optional integrations fall back to safe defaults and only warn.
			wantWarnings: []string{"rpc_url.default", "email.provider.log"},
		},
		{
			name: "explicit defaults pass",
			mutate: func(in *ProductionInputs) {
				in.EmailProvider = "log"
				in.StorageDriver = "local"
				in.Edge = "none"
				in.RPCURL = DefaultRPCURL
				in.TrustedProxies = DefaultTrustedProxies
			},
			wantWarnings: []string{"email.provider.log"},
		},
		{
			name: "EMAIL_PROVIDER=resend without keys fails clearly",
			mutate: func(in *ProductionInputs) {
				in.EmailProvider = "resend"
			},
			// Resend additionally owns the sender-domain, webhook, and
			// SPF/DKIM/DMARC alignment block.
			wantCodes: []string{
				"email.api_key.missing",
				"email.from.missing",
				"email.sender_domains.missing",
				"email.webhook_secret.missing",
				"email.dns_alignment.missing",
				"email.dmarc_policy.invalid",
			},
		},
		{
			name: "EMAIL_PROVIDER=postmark without a token fails",
			mutate: func(in *ProductionInputs) {
				in.EmailProvider = "postmark"
			},
			wantCodes: []string{"email.api_key.missing", "email.from.missing"},
		},
		{
			name: "EMAIL_PROVIDER=smtp without a host fails",
			mutate: func(in *ProductionInputs) {
				in.EmailProvider = "smtp"
			},
			wantCodes: []string{"email.smtp.host.missing", "email.from.missing"},
		},
		{
			name: "STORAGE_DRIVER=s3 without a bucket fails",
			mutate: func(in *ProductionInputs) {
				in.StorageDriver = "s3"
			},
			wantCodes:    []string{"s3.public.missing"},
			wantWarnings: []string{"s3.protected.shared"},
		},
		{
			name: "STORAGE_DRIVER=s3 with half a protected credential pair fails",
			mutate: func(in *ProductionInputs) {
				in.StorageDriver = "s3"
				in.S3Bucket = "media"
				in.AWSAccessKey = "public-access-key-id"
				in.AWSSecretKey = "public-secret-value"
				in.AWSProtectedAccessKey = "protected-access-key-id"
			},
			wantCodes:    []string{"s3.protected.missing"},
			wantWarnings: []string{"s3.protected.shared"},
		},
		{
			name: "EDGE=cloudflare with the default proxy ranges passes",
			mutate: func(in *ProductionInputs) {
				in.Edge = "cloudflare"
			},
		},
		{
			name: "compose default JWT secret fails",
			mutate: func(in *ProductionInputs) {
				in.JWTSecretKey = "payverge_local_dev_jwt_secret_2026_min32"
			},
			wantCodes: []string{"jwt.secret.unsafe"},
		},
		{
			name: "retired plugin fallback key fails",
			mutate: func(in *ProductionInputs) {
				in.PluginSecretKey = "payverge-local-plugin-secret-key"
			},
			wantCodes: []string{"plugin_secret.unsafe"},
		},
		{
			name: "compose default database password fails",
			mutate: func(in *ProductionInputs) {
				in.DBPassword = "payverge_password"
			},
			wantCodes: []string{"database.password.unsafe"},
		},
		{
			name: "trivial admin password fails",
			mutate: func(in *ProductionInputs) {
				in.AdminPassword = "admin"
			},
			wantCodes: []string{"admin.password.unsafe"},
		},
		{
			name: "misspelled REGISTRATION_MODE fails",
			mutate: func(in *ProductionInputs) {
				in.RegistrationMode = "invte"
			},
			wantCodes: []string{"registration_mode.invalid"},
		},
		{
			name: "explicit REGISTRATION_MODE values pass",
			mutate: func(in *ProductionInputs) {
				in.RegistrationMode = " Closed "
			},
			wantWarnings: []string{"rpc_url.default", "email.provider.log"},
		},
		{
			name: "missing PUBLIC_URL fails",
			mutate: func(in *ProductionInputs) {
				in.PublicURL = ""
			},
			wantCodes: []string{"public_url.missing", "origins.missing"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := minimalSelfHostInputs()
			tt.mutate(&in)
			report := ValidateProduction(in)

			if len(tt.wantCodes) == 0 {
				assert.True(t, report.OK(), "codes=%v", report.Codes())
			} else {
				require.False(t, report.OK())
				assert.ElementsMatch(t, tt.wantCodes, report.Codes())
				for _, issue := range report.Issues() {
					assert.NotContains(t, issue.Message, in.JWTSecretKey)
					assert.NotContains(t, issue.Message, in.PluginSecretKey)
					assert.NotContains(t, issue.Message, in.DBPassword)
					assert.NotContains(t, issue.Message, in.AdminPassword)
				}
			}
			assert.Subset(t, warningCodes(report), tt.wantWarnings)
		})
	}
}

func TestOSSPreflightEmailProviderSelection(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		expected string
	}{
		{name: "nothing configured logs", expected: "log"},
		{name: "explicit provider wins", env: map[string]string{"EMAIL_PROVIDER": " SMTP ", "RESEND_API_KEY": "re_x"}, expected: "smtp"},
		{name: "resend key selects resend", env: map[string]string{"RESEND_API_KEY": "re_x"}, expected: "resend"},
		{name: "legacy EMAIL_API_KEY alone does not pick a provider", env: map[string]string{"EMAIL_API_KEY": "re_x"}, expected: "log"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{"EMAIL_PROVIDER", "RESEND_API_KEY", "EMAIL_API_KEY"} {
				t.Setenv(key, tt.env[key])
			}
			assert.Equal(t, tt.expected, EmailProvider())
		})
	}
}

func TestPreflightEmailAPIKey(t *testing.T) {
	t.Setenv("EMAIL_API_KEY", "")
	t.Setenv("RESEND_API_KEY", "re_from_env")

	assert.Equal(t, "re_from_env", PreflightEmailAPIKey("resend", ""))
	assert.Equal(t, "re_flag", PreflightEmailAPIKey("resend", "re_flag"))
	assert.Equal(t, "pm-flag", PreflightEmailAPIKey("postmark", "pm-flag"))
	assert.Empty(t, PreflightEmailAPIKey("POSTMARK", ""), "postmark without a flag or EMAIL_API_KEY has no key")
	assert.Empty(t, PreflightEmailAPIKey("log", "re_flag"))
	assert.Empty(t, PreflightEmailAPIKey("smtp", ""))

	t.Setenv("EMAIL_API_KEY", "shared-key")
	assert.Equal(t, "shared-key", PreflightEmailAPIKey("resend", ""))
	assert.Equal(t, "shared-key", PreflightEmailAPIKey("postmark", ""))
}

func TestPublicOrigin(t *testing.T) {
	ok := map[string]string{
		"https://Restaurant.Example.com":   "https://restaurant.example.com",
		"https://restaurant.example.com/":  "https://restaurant.example.com",
		" https://restaurant.example.com ": "https://restaurant.example.com",
		"https://example.com:8443":         "https://example.com:8443",
		"http://localhost:3000":            "http://localhost:3000",
		"http://127.0.0.1:8080":            "http://127.0.0.1:8080",
		"http://app.localhost":             "http://app.localhost",
	}
	for raw, want := range ok {
		got, err := PublicOrigin(raw)
		require.NoError(t, err, raw)
		assert.Equal(t, want, got, raw)
	}

	bad := map[string]string{
		"":                               "public_url.missing",
		"bistro.invalid":                 "public_url.invalid",
		"https://bistro.invalid/app":     "public_url.invalid",
		"https://bistro.invalid?x=1":     "public_url.invalid",
		"https://bistro.invalid#frag":    "public_url.invalid",
		"https://user:pw@bistro.invalid": "public_url.invalid",
		"ftp://bistro.invalid":           "public_url.invalid",
		"http://bistro.invalid":          "public_url.insecure",
		"http://10.0.0.5":                "public_url.insecure",
	}
	for raw, wantCode := range bad {
		_, err := PublicOrigin(raw)
		require.Error(t, err, raw)
		var pubErr *PublicURLError
		require.ErrorAs(t, err, &pubErr)
		assert.Equal(t, wantCode, pubErr.Code, raw)
		if raw != "" {
			assert.NotContains(t, pubErr.Message, raw)
		}
	}
}

func TestDeployDefaults(t *testing.T) {
	assert.Equal(t, DefaultRPCURL, RPCURLOrDefault(""))
	assert.Equal(t, DefaultRPCURL, RPCURLOrDefault("   "))
	assert.Equal(t, "https://rpc.example", RPCURLOrDefault(" https://rpc.example "))
	assert.Equal(t, int64(8453), ProductionSettlementChainID)
	assert.Empty(t, validatePrivateTrustedProxies(DefaultTrustedProxies), "default proxy trust must stay private")
	assert.Equal(t, "127.0.0.0/8,::1", DefaultTrustedProxies, "D-1: unset TRUSTED_PROXIES trusts loopback only, never LAN/container ranges")

	t.Setenv("DATA_DIR", "")
	t.Setenv("STORAGE_DIR", "")
	assert.Equal(t, "data", DataDir())
	// STORAGE_DIR never moves the data dir: its parent (/data in the image)
	// is root-owned and not a volume, and STORAGE_DIR itself is served.
	t.Setenv("STORAGE_DIR", "/data/storage")
	assert.Equal(t, "data", DataDir())
	t.Setenv("STORAGE_DIR", "uploads")
	assert.Equal(t, "data", DataDir())
	t.Setenv("DATA_DIR", "/var/lib/payverge/")
	assert.Equal(t, "/var/lib/payverge", DataDir())
}

func TestPathWithin(t *testing.T) {
	cases := []struct {
		path, dir string
		want      bool
	}{
		{"/data/storage", "/data/storage", true},
		{"/data/storage/secrets", "/data/storage", true},
		{"/data/storage/../secrets", "/data/storage", false},
		{"/data", "/data/storage", false},
		{"/data/storage-private", "/data/storage", false},
		{"data", "data/storage", false},
		{"data/storage/x", "data/storage", true},
		{"..data", ".", true},
		{"", "/data/storage", false},
		{"/data/storage", "", false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, PathWithin(tc.path, tc.dir), "PathWithin(%q, %q)", tc.path, tc.dir)
	}
}
