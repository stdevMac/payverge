package handlers

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPaidBillReceiptContract_AllSettlementPathsUseDomainOperation(t *testing.T) {
	t.Parallel()
	cases := map[string]map[string]int{
		"payments.go": {
			"MarkAlternativePayment":   2,
			"ProcessCryptoPayment":     1,
			"ProcessCrossChainPayment": 1,
		},
		"plugin_handlers.go": {
			"updateBillPaymentStatus": 1,
		},
	}

	for filename, expected := range cases {
		filename, expected := filename, expected
		t.Run(filename, func(t *testing.T) {
			parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(".", filename), nil, 0)
			require.NoError(t, err)
			seen := make(map[string]int)
			for _, decl := range parsed.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				ast.Inspect(fn.Body, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok {
						return true
					}
					ident, ok := call.Fun.(*ast.Ident)
					if ok && ident.Name == "enqueueReceiptForPaidBill" {
						seen[fn.Name.Name]++
					}
					return true
				})
			}
			require.Equal(t, expected, seen)
		})
	}
}
