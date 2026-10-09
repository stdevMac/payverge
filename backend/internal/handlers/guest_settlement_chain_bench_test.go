package handlers

import "testing"

// BenchmarkGuestSettlementChainSupported covers the chain gate that runs on
// every guest crypto quote and every crypto payment verification
// (currentGuestSettlementContract). The Sepolia cases exercise the
// production-mode lookup; the mainnet case must stay a constant compare.
func BenchmarkGuestSettlementChainSupported(b *testing.B) {
	cases := []struct {
		name    string
		chainID int64
		method  string
	}{
		{"mainnet_usdc", baseMainnetChainID, guestPaymentMethodUSDC},
		{"mainnet_cross_chain", baseMainnetChainID, guestPaymentMethodCrossChain},
		{"sepolia_usdc", baseSepoliaChainID, guestPaymentMethodUSDC},
		{"unsupported_chain", 1, guestPaymentMethodUSDC},
	}
	for _, bc := range cases {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			var ok bool
			for i := 0; i < b.N; i++ {
				ok = guestSettlementChainSupported(bc.chainID, bc.method)
			}
			benchSettlementChainSink = ok
		})
	}
}

var benchSettlementChainSink bool
