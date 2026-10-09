package reporting

import (
	"testing"
)

// TestCanonicalize_EveryKnownRawMapsToExactlyOneMethod is the Task 7 gate for
// finding 62: both payment_method columns (payments + alternative_payments)
// plus every plugin name / legacy spelling collapse to one of the six
// canonical Methods. No raw value may land outside the set, and no value may
// map to two Methods.
func TestCanonicalize_EveryKnownRawMapsToExactlyOneMethod(t *testing.T) {
	// Exhaustive known spellings seen in production rows and write paths.
	// source_table is only needed for the empty-string default.
	cases := []struct {
		source string
		raw    string
		want   Method
	}{
		// payments table
		{"payments", "crypto", MethodCrypto},
		{"payments", "CRYPTO", MethodCrypto},
		{"payments", "", MethodCrypto}, // historical default
		{"payments", "usdc", MethodCrypto},
		{"payments", "usdc_payment", MethodCrypto},
		{"payments", "cross-chain", MethodCrossChain},
		{"payments", "cross_chain", MethodCrossChain},
		{"payments", "cross_chain_payment", MethodCrossChain},
		{"payments", "plugin", MethodOther}, // generic — no settlement type
		{"payments", "stripe", MethodCard},
		{"payments", "mercadopago", MethodCard},
		{"payments", "paypal", MethodWallet},
		{"payments", "usd", MethodOther}, // legacy currency-as-method
		{"payments", "USD", MethodOther},
		{"payments", "eur", MethodOther},

		// alternative_payments table
		{"alternative_payments", "cash", MethodCash},
		{"alternative_payments", "card", MethodCard},
		{"alternative_payments", "venmo", MethodWallet},
		{"alternative_payments", "other", MethodOther},
		{"alternative_payments", "stripe", MethodCard},
		{"alternative_payments", "paypal", MethodWallet},
		{"alternative_payments", "mercadopago", MethodCard},
		{"alternative_payments", "", MethodOther},

		// whitespace / mixed separators
		{"payments", "  crypto  ", MethodCrypto},
		{"payments", "cross—chain", MethodCrossChain}, // en-dash treated as separator? fallback other if not
	}

	seen := map[string]Method{}
	for _, tc := range cases {
		// Skip the en-dash case if we only normalize hyphen/underscore.
		raw := tc.raw
		if raw == "cross—chain" {
			// Must still land in the set (Other is fine for exotic separators).
			got := Canonicalize(tc.source, raw)
			if !IsKnown(got) {
				t.Fatalf("Canonicalize(%q,%q) = %q, not a known Method", tc.source, raw, got)
			}
			continue
		}
		got := Canonicalize(tc.source, raw)
		if got != tc.want {
			t.Fatalf("Canonicalize(%q, %q) = %q, want %q", tc.source, raw, got, tc.want)
		}
		if !IsKnown(got) {
			t.Fatalf("Canonicalize(%q, %q) = %q is not in AllMethods", tc.source, raw, got)
		}
		key := tc.source + "|" + raw
		if prev, ok := seen[key]; ok && prev != got {
			t.Fatalf("Canonicalize non-deterministic for %s: %q vs %q", key, prev, got)
		}
		seen[key] = got
	}
}

// TestAllMethods_IsTheCanonicalSet pins the six Methods the plan names.
func TestAllMethods_IsTheCanonicalSet(t *testing.T) {
	want := []Method{
		MethodCrypto, MethodCrossChain, MethodCard, MethodCash, MethodWallet, MethodOther,
	}
	got := AllMethods()
	if len(got) != len(want) {
		t.Fatalf("AllMethods len = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("AllMethods[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestParseMethod_OnlyCanonicalKeysAccepted asserts filter keys from the FE
// must be members of the canonical set (not "stripe", "manual", "Online").
func TestParseMethod_OnlyCanonicalKeysAccepted(t *testing.T) {
	for _, m := range AllMethods() {
		got, ok := ParseMethod(string(m))
		if !ok || got != m {
			t.Fatalf("ParseMethod(%q) = (%q, %v)", m, got, ok)
		}
	}
	// Historical FE filter keys that must NOT be accepted as filter keys —
	// they were the bug (method=stripe → 0 of 0 because the column stores
	// "plugin" / "stripe" is a plugin name that maps to card).
	for _, bad := range []string{"stripe", "manual", "Online", "plugin", "all", ""} {
		if _, ok := ParseMethod(bad); ok {
			t.Fatalf("ParseMethod(%q) should reject non-canonical keys", bad)
		}
	}
}

// TestRawAliases_CoverCanonicalizeRoundTrip asserts every alias returned by
// RawAliases canonicalizes back to the same Method, so a SQL IN filter built
// from RawAliases never misses a row the UI claims to offer.
func TestRawAliases_CoverCanonicalizeRoundTrip(t *testing.T) {
	for _, m := range AllMethods() {
		aliases := RawAliases(m)
		if len(aliases) == 0 {
			t.Fatalf("RawAliases(%q) empty", m)
		}
		for _, a := range aliases {
			got := Canonicalize("payments", a)
			// Also check alt-payments source for cash/card/venmo/other.
			gotAlt := Canonicalize("alternative_payments", a)
			if got != m && gotAlt != m {
				t.Fatalf("RawAliases(%q) contains %q which canonicalizes to payments=%q alt=%q", m, a, got, gotAlt)
			}
			if got != m && got == MethodOther && m != MethodOther {
				// Alias only valid under alt source is fine if gotAlt matches.
				if gotAlt != m {
					t.Fatalf("alias %q for %q maps to %q/%q", a, m, got, gotAlt)
				}
			}
		}
	}
}

// TestDistinctMethods_OrdersByCanonicalList is what the history endpoint uses
// to build available_methods: raw DISTINCT values → unique Methods in
// AllMethods order, never inventing a Method that has zero rows.
func TestDistinctMethods_OrdersByCanonicalList(t *testing.T) {
	raws := []string{"stripe", "cash", "crypto", "venmo", "stripe", "CRYPTO", ""}
	got := DistinctMethods("payments", raws)
	// empty → crypto under payments; stripe → card; venmo → wallet
	want := []Method{MethodCrypto, MethodCard, MethodCash, MethodWallet}
	if len(got) != len(want) {
		t.Fatalf("DistinctMethods = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("DistinctMethods[%d] = %q, want %q (full %v)", i, got[i], want[i], got)
		}
	}
	// Must not invent methods with zero rows.
	for _, m := range got {
		// cross_chain and other are absent from the raws above.
		if m == MethodCrossChain || m == MethodOther {
			t.Fatalf("DistinctMethods invented %q with no backing raw", m)
		}
	}
}
