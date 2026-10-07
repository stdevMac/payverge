package services

import (
	"strings"
	"testing"
)

// Guests pay their own Base gas (plain ERC-20 transfer from the guest
// wallet). No seed copy may claim gasless payments.
func TestPluginSeedCopyDoesNotClaimZeroGas(t *testing.T) {
	forbidden := []string{"Zero Gas", "zero gas", "gasless", "comisiones de gas", "sin gas"}

	for _, p := range defaultSeedPlugins {
		blob := p.Description + " " + p.Features + " " + p.Message
		for _, f := range forbidden {
			if strings.Contains(blob, f) {
				t.Errorf("seed plugin %q contains forbidden claim %q", p.Name, f)
			}
		}
	}
	for lang, plugins := range pluginSeedTranslationsByLanguage {
		for name, fields := range plugins {
			for field, content := range fields {
				for _, f := range forbidden {
					if strings.Contains(content, f) {
						t.Errorf("%s translation of %s.%s contains forbidden claim %q", lang, name, field, f)
					}
				}
			}
		}
	}
}

func TestCryptoPluginSeedCopyQualifiesSettlementAndFees(t *testing.T) {
	forbidden := []string{
		"instant settlement",
		"Instant Settlement",
		"zero-fee",
		"zero fee",
		"fee-free",
	}

	for _, p := range defaultSeedPlugins {
		if p.Name != PluginNameUSDCPayment && p.Name != PluginNameCrossChainPayment {
			continue
		}
		blob := p.Description + " " + p.Features + " " + p.Message
		for _, claim := range forbidden {
			if strings.Contains(blob, claim) {
				t.Errorf("seed plugin %q contains unqualified claim %q", p.Name, claim)
			}
		}
	}

	for _, p := range defaultSeedPlugins {
		if p.Name != PluginNameUSDCPayment {
			continue
		}
		blob := strings.ToLower(p.Description + " " + p.Features)
		if !strings.Contains(blob, "network confirmation") {
			t.Errorf("USDC seed copy must explain that settlement follows network confirmation")
		}
		if !strings.Contains(blob, "network fee") {
			t.Errorf("USDC seed copy must disclose network fees")
		}
	}
}

// The seed never describes the cross-chain rail as live while guest
// settlement through it is switched off (review M1, oss/c1).
func TestCrossChainSeedCopyMatchesGuestSettlementAvailability(t *testing.T) {
	for _, p := range defaultSeedPlugins {
		if p.Name != PluginNameCrossChainPayment {
			continue
		}
		if p.ComingSoon == CrossChainGuestSettlementAvailable {
			t.Fatalf("cross_chain_payment ComingSoon=%v but CrossChainGuestSettlementAvailable=%v", p.ComingSoon, CrossChainGuestSettlementAvailable)
		}
		return
	}
	t.Fatal("cross_chain_payment missing from the seed catalog")
}
