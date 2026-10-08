//go:build afip_homo

// Package ar — AFIP homologación (WSHOMO) end-to-end test.
//
// The hermetic httptest WSAA/WSFE fixture tests in this package (the normal
// suite) are the gate that runs on every change. This afip_homo test is an
// optional, manual end-to-end check against AFIP homologación: it runs the full
// flow with a real test CUIT certificate and key and asserts a real CAE comes
// back. Operators may run it, with real test credentials, before pointing a
// business at AFIP production. It is build-tagged (`afip_homo`) so it never
// runs in the normal suite or CI — it needs live AFIP connectivity.
//
// Run:
//
//	FISCAL_HOMO_CUIT=20123456789 \
//	FISCAL_HOMO_POS=1 \
//	FISCAL_HOMO_CERT_PATH=/abs/path/test.crt \
//	FISCAL_HOMO_KEY_PATH=/abs/path/test.key \
//	go test -tags afip_homo -run TestAFIPHomologacionE2E -v ./internal/fiscal/providers/ar/
//
// Endpoints default to WSHOMO (environment "sandbox"); override with
// FISCAL_WSAA_URL / FISCAL_WSFE_URL if AFIP rotates the homologation hosts.
//
// Do not point a business's fiscal environment at AFIP production until this
// check is green for a real test CUIT. See docs/fiscal/argentina-afip.md.
package ar

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/fiscal"
)

func TestAFIPHomologacionE2E(t *testing.T) {
	cuitStr := os.Getenv("FISCAL_HOMO_CUIT")
	posStr := os.Getenv("FISCAL_HOMO_POS")
	certPath := os.Getenv("FISCAL_HOMO_CERT_PATH")
	keyPath := os.Getenv("FISCAL_HOMO_KEY_PATH")
	if cuitStr == "" || posStr == "" || certPath == "" || keyPath == "" {
		t.Skip("AFIP homologación e2e requires FISCAL_HOMO_CUIT, FISCAL_HOMO_POS, FISCAL_HOMO_CERT_PATH, FISCAL_HOMO_KEY_PATH")
	}

	cuit, err := strconv.ParseInt(cuitStr, 10, 64)
	if err != nil {
		t.Fatalf("FISCAL_HOMO_CUIT must be numeric: %v", err)
	}
	pos, err := strconv.Atoi(posStr)
	if err != nil || pos <= 0 {
		t.Fatalf("FISCAL_HOMO_POS must be a positive integer: %v", err)
	}

	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("read cert: %v", err)
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read key: %v", err)
	}

	bundle, err := ParseCertificateBundle(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("parse certificate bundle: %v", err)
	}

	// "sandbox" resolves to the WSHOMO hosts (overridable via FISCAL_WSAA_URL /
	// FISCAL_WSFE_URL). Never point this at production.
	provider := NewProviderFromSettings("sandbox", cuit, bundle)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	settings := fiscal.Settings{
		Country:      "AR",
		Provider:     "arca",
		Environment:  "sandbox",
		TaxID:        cuitStr,
		TaxCondition: "monotributo",
		PointOfSale:  &pos,
	}

	// Step 1: FEDummy health check + WSAA login round-trip (proves the cert/key are
	// accepted and the service is up). This is exactly what the operator "Validate"
	// button runs.
	if err := provider.ValidateSettings(ctx, settings); err != nil {
		t.Fatalf("ValidateSettings against WSHOMO failed (cert not authorized for wsfe? PoS not registered? clock skew?): %v", err)
	}
	t.Log("WSHOMO ValidateSettings OK (FEDummy + WSAA login)")

	// Step 2: request a real CAE for a factura C (monotributo) — FECompUltimoAutorizado
	// then FECAESolicitar. A factura C is IVA-exento so the amount is the full total.
	now := time.Now()
	input := fiscal.IssueInput{
		Settings:         settings,
		Bill:             database.Bill{TotalAmount: 12100, PaidAmount: 12100, Status: database.BillStatusPaid},
		ReceiptType:      "factura_c",
		TotalAmountCents: 12100,
		Currency:         "PES",
		IssuedAt:         now,
		IdempotencyKey:   "homo-e2e-" + strconv.FormatInt(now.Unix(), 10),
	}

	result, err := provider.IssueReceipt(ctx, input)
	if err != nil {
		t.Fatalf("IssueReceipt against WSHOMO failed: %v", err)
	}
	if result == nil {
		t.Fatal("IssueReceipt returned nil result")
	}
	if result.Status != fiscal.StatusAuthorized {
		t.Fatalf("expected authorized, got status=%q errors=%v", result.Status, result.ProviderErrors)
	}
	if result.AuthCode == "" {
		t.Fatal("authorized receipt has no CAE")
	}
	if result.ReceiptNumber == "" {
		t.Fatal("authorized receipt has no receipt number")
	}
	if result.QRPayload == "" {
		t.Fatal("authorized receipt has no AFIP QR payload")
	}
	t.Logf("WSHOMO CAE OK: CAE=%s number=%s type=%s expires=%v",
		result.AuthCode, result.ReceiptNumber, result.ReceiptType, result.AuthExpiresAt)
}
