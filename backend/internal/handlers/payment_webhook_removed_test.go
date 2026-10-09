package handlers

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The legacy POST /payments/webhook handler credited a bill as paid in crypto
// on an HMAC signature alone, with no on-chain verification of the transfer.
// It was unrouted, but a fork re-wiring it would open a "mark any bill paid"
// endpoint to anyone holding PAYMENT_WEBHOOK_SECRET. Crypto settlement must go
// through ProcessCryptoPayment / ProcessCrossChainPayment, which verify the
// transfer on chain. This guard keeps the handler and its helpers deleted.
func TestLegacyHMACOnlyPaymentWebhookStaysDeleted(t *testing.T) {
	t.Parallel()
	banned := map[string]bool{
		"WebhookPaymentConfirmation":    true,
		"verifyPaymentWebhookSignature": true,
		"parsePaymentWebhookBillID":     true,
	}

	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		require.NoError(t, err, name)
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			require.False(t, banned[fn.Name.Name],
				"%s: %s must stay deleted (HMAC-only crypto crediting, no on-chain check)", name, fn.Name.Name)
		}
		src, err := os.ReadFile(name)
		require.NoError(t, err)
		require.NotContains(t, string(src), `"/payments/webhook"`, "%s must not register the legacy payment webhook route", name)
	}
}
