// Package reporting — payment-method vocabulary (Task 7 / finding 62).
//
// One canonical Method set for both payment ledgers. payments.payment_method
// historically held crypto / cross-chain / plugin (plus plugin names and
// legacy currency codes); alternative_payments.payment_method holds cash /
// card / venmo / other (plus plugin names). The operator filter used a third
// vocabulary (crypto / manual / stripe→"Online") that matched zero rows.
// Canonicalize is the single mapping; filter options are generated from
// DistinctMethods over the business+window, never hand-listed.
package reporting

import (
	"strings"
)

// Method is the ONE recognized payment-method enum for every money surface.
type Method string

const (
	MethodCrypto     Method = "crypto"
	MethodCrossChain Method = "cross_chain"
	MethodCard       Method = "card"
	MethodCash       Method = "cash"
	MethodWallet     Method = "wallet" // venmo, paypal, and similar wallets
	MethodOther      Method = "other"
)

// allMethods is the stable ordered set. Prefer AllMethods().
var allMethods = []Method{
	MethodCrypto,
	MethodCrossChain,
	MethodCard,
	MethodCash,
	MethodWallet,
	MethodOther,
}

// AllMethods returns the six canonical Methods in UI order.
func AllMethods() []Method {
	out := make([]Method, len(allMethods))
	copy(out, allMethods)
	return out
}

// IsKnown reports whether m is a member of the canonical set.
func IsKnown(m Method) bool {
	switch m {
	case MethodCrypto, MethodCrossChain, MethodCard, MethodCash, MethodWallet, MethodOther:
		return true
	default:
		return false
	}
}

// ParseMethod accepts a filter key from the client. Only canonical Method
// strings succeed — "stripe", "manual", "plugin", "Online" are rejected so a
// dead filter key cannot be offered or applied.
func ParseMethod(raw string) (Method, bool) {
	m := Method(strings.ToLower(strings.TrimSpace(raw)))
	if !IsKnown(m) {
		return "", false
	}
	return m, true
}

// normalizeRaw lowercases, trims, and collapses hyphens to underscores so
// "cross-chain" and "cross_chain" share one switch arm.
func normalizeRaw(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, "-", "_")
	return s
}

// Canonicalize maps a raw payment_method column value from either source
// table onto exactly one Method.
//
// sourceTable is "payments" or "alternative_payments" (or empty). It only
// affects the empty-string default: payments historically defaulted to
// crypto; alternative_payments treat empty as other.
//
// Plugin names resolve by settlement type (stripe/mercadopago → card,
// paypal/venmo → wallet). The bare token "plugin" has no settlement type
// and becomes other — never a UI word like "Online".
func Canonicalize(sourceTable, raw string) Method {
	s := normalizeRaw(raw)
	switch s {
	case "crypto", "usdc", "usdc_payment":
		return MethodCrypto
	case "cross_chain", "cross_chain_payment", "crosschain":
		return MethodCrossChain
	case "card", "stripe", "mercadopago":
		return MethodCard
	case "cash":
		return MethodCash
	case "wallet", "venmo", "paypal":
		return MethodWallet
	case "other", "plugin", "manual":
		return MethodOther
	case "":
		if strings.Contains(strings.ToLower(sourceTable), "alternative") {
			return MethodOther
		}
		return MethodCrypto
	default:
		// Legacy currency codes stored as payment_method (usd, eur, …) and
		// any unknown plugin / free-text tender collapse to other.
		return MethodOther
	}
}

// rawAliases lists every known raw spelling that Canonicalize maps to m.
// Used to build SQL IN filters so method=card matches stripe + card +
// mercadopago rows, etc. Always includes the canonical string itself.
func rawAliases(m Method) []string {
	switch m {
	case MethodCrypto:
		return []string{"crypto", "usdc", "usdc_payment", ""}
	case MethodCrossChain:
		return []string{"cross_chain", "cross-chain", "cross_chain_payment", "crosschain"}
	case MethodCard:
		return []string{"card", "stripe", "mercadopago"}
	case MethodCash:
		return []string{"cash"}
	case MethodWallet:
		return []string{"wallet", "venmo", "paypal"}
	case MethodOther:
		// plugin/manual + common legacy currency codes. Unknown free-text is
		// NOT listed — filter-by-other uses Canonicalize-side matching for
		// known aliases only; free-text other still surfaces in available
		// methods via DistinctMethods.
		return []string{"other", "plugin", "manual", "usd", "eur", "ars", "brl", "mxn", "gbp"}
	default:
		return nil
	}
}

// RawAliases is the exported form of rawAliases.
func RawAliases(m Method) []string {
	return rawAliases(m)
}

// DistinctMethods canonicalizes a bag of raw method strings and returns the
// unique Methods that appeared, ordered by AllMethods. Methods with zero
// backing raws are omitted — so a filter built from the result cannot
// produce "0 of 0".
func DistinctMethods(sourceTable string, raws []string) []Method {
	present := make(map[Method]struct{}, len(allMethods))
	for _, r := range raws {
		present[Canonicalize(sourceTable, r)] = struct{}{}
	}
	out := make([]Method, 0, len(present))
	for _, m := range allMethods {
		if _, ok := present[m]; ok {
			out = append(out, m)
		}
	}
	return out
}

// FilterMatchSQL returns a SQL fragment and args that match payment_events.method
// (or any method column) for the given canonical Method. The column expression
// is supplied by the caller (e.g. "payment_events.method") so this stays
// driver-agnostic. Empty string aliases are matched via TRIM equality.
//
// For MethodOther, also matches any method that is NOT a known alias of the
// other five Methods — so free-text "foo" rows classified as other still
// filter correctly without enumerating every free-text value.
func FilterMatchArgs(m Method) (include []string, excludeOther bool) {
	if m == MethodOther {
		// Match known other aliases PLUS anything that is not a known non-other alias.
		return rawAliases(MethodOther), true
	}
	return rawAliases(m), false
}

// NonOtherAliases returns every raw spelling that belongs to a Method other
// than MethodOther. Used with FilterMatchArgs when excludeOther is true.
func NonOtherAliases() []string {
	var out []string
	for _, m := range allMethods {
		if m == MethodOther {
			continue
		}
		out = append(out, rawAliases(m)...)
	}
	return out
}
