package fiscal

import "testing"

func TestSplitInclusiveVAT_21(t *testing.T) {
	net, vat := SplitInclusiveVAT(12100, 21.0)
	if net != 10000 || vat != 2100 {
		t.Fatalf("net=%d vat=%d", net, vat)
	}
}

func TestSplitInclusiveVAT_RoundingSumsToTotal(t *testing.T) {
	for _, total := range []int64{999, 12345, 7, 1} {
		net, vat := SplitInclusiveVAT(total, 21.0)
		if net+vat != total {
			t.Errorf("total=%d net+vat=%d", total, net+vat)
		}
	}
}

func TestSplitInclusiveVAT_ZeroRate(t *testing.T) {
	net, vat := SplitInclusiveVAT(12100, 0)
	if net != 12100 || vat != 0 {
		t.Fatalf("zero-rate: net=%d vat=%d", net, vat)
	}
}

func TestSplitInclusiveVAT_UsesDefaultConstant(t *testing.T) {
	net, vat := SplitInclusiveVAT(12100, DefaultArgentinaVATRate)
	if net != 10000 || vat != 2100 {
		t.Fatalf("net=%d vat=%d", net, vat)
	}
}
