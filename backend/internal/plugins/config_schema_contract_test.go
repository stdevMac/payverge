package plugins_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/plugins/mercadopago"
	"github.com/stdevmac/payverge/backend/internal/plugins/paypal"
	"github.com/stdevmac/payverge/backend/internal/plugins/stripe"
)

// configSchemaProvider is the minimal surface each payment plugin exposes for
// its dashboard configuration schema.
type configSchemaProvider interface {
	GetConfigSchema() string
}

// fixtureDir points at the frontend-owned fixtures that the operator config
// editors validate themselves against. Keeping a single copy that both the Go
// backend and the Jest frontend read means a schema change on either side that
// isn't mirrored fails CI on the other side.
//
// The path is resolved relative to this test's package directory
// (backend/internal/plugins) up to the repo root, then into frontend.
const fixtureRelDir = "../../../frontend/src/components/business/plugins/__fixtures__/plugin-config-schemas"

// TestPaymentPluginConfigSchemaContract asserts each payment plugin's live
// GetConfigSchema() exactly matches (semantically, ignoring formatting) its
// checked-in fixture. If a backend schema drifts, this fails and tells you to
// regenerate the fixture so the frontend form-contract test can see the change.
func TestPaymentPluginConfigSchemaContract(t *testing.T) {
	cases := []struct {
		name    string
		fixture string
		plugin  configSchemaProvider
	}{
		{"stripe", "stripe.json", stripe.NewStripePlugin(nil)},
		{"mercadopago", "mercadopago.json", mercadopago.NewMercadoPagoPlugin(nil)},
		{"paypal", "paypal.json", paypal.NewPayPalPlugin(nil)},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			live := normalizeJSON(t, []byte(tc.plugin.GetConfigSchema()))

			fixturePath := filepath.Join(fixtureRelDir, tc.fixture)
			raw, err := os.ReadFile(fixturePath)
			if err != nil {
				t.Fatalf("read fixture %s: %v", fixturePath, err)
			}
			fixture := normalizeJSON(t, raw)

			if live != fixture {
				t.Fatalf(
					"%s GetConfigSchema() drifted from fixture %s.\n"+
						"Regenerate the fixture from the backend schema so the frontend "+
						"form-contract test picks up the change.\n\nLIVE:\n%s\n\nFIXTURE:\n%s",
					tc.name, tc.fixture, live, fixture,
				)
			}
		})
	}
}

// normalizeJSON round-trips through a generic map so key order and whitespace
// don't cause spurious mismatches — only structural/value differences fail.
func normalizeJSON(t *testing.T, raw []byte) string {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, string(raw))
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("re-marshal JSON: %v", err)
	}
	return string(out)
}
