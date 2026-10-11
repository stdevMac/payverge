package txhash

import (
	"strings"
	"testing"
)

const body = "5c504ed432cb51138bcf09aa5e8a410dd4a1e204ef84bfed1be16dfba1b22060"

func TestCanonical(t *testing.T) {
	want := "0x" + body
	for _, ok := range []string{"0x" + body, "0X" + body, "0x" + strings.ToUpper(body), "  0x" + body + "\n"} {
		got, valid := Canonical(ok)
		if !valid || got != want {
			t.Errorf("Canonical(%q) = %q,%v; want %q,true", ok, got, valid, want)
		}
	}
	for _, bad := range []string{"", body, "0x00" + body, "0x" + body + "zz", "0x" + body[:63], "0x" + strings.Repeat("g", 64), "0x" + body[:32] + " " + body[33:], "tx-crypto-1"} {
		if got, valid := Canonical(bad); valid {
			t.Errorf("Canonical(%q) = %q,true; want rejection", bad, got)
		}
	}
}

func TestNormalizeReference(t *testing.T) {
	want := "0x" + body
	for _, in := range []string{body, "0x" + body, "0X" + strings.ToUpper(body), strings.ToUpper(body)} {
		if got := NormalizeReference(in); got != want {
			t.Errorf("NormalizeReference(%q) = %q; want %q", in, got, want)
		}
	}
	for _, untouched := range []string{"pi_3AbCdEfGh", "plugin_ABCdef", "manual_7_3F2504E0-4F89-11D3-9A0C-0305E82C3301", "split_" + body[:40], "tx-crypto-1"} {
		if got := NormalizeReference(untouched); got != untouched {
			t.Errorf("NormalizeReference(%q) = %q; non-EVM references must not be folded", untouched, got)
		}
	}
}
