package fiscal

import "testing"

// RG 5700/2025 set the Factura B unidentified-consumidor-final identification
// threshold to a flat ARS 10,000,000. The shipped default must match the law,
// not require every operator to discover the env override.
func TestDefaultCFIDThresholdMatchesRG5700(t *testing.T) {
	t.Setenv("FISCAL_AR_CF_ID_THRESHOLD_CENTS", "")
	const rg5700Cents = int64(1_000_000_000) // ARS 10,000,000.00
	if got := CFIDThresholdCents(); got != rg5700Cents {
		t.Fatalf("CFIDThresholdCents() = %d, want %d (RG 5700/2025: ARS 10,000,000)", got, rg5700Cents)
	}
	if DefaultCFIDThresholdCents != rg5700Cents {
		t.Fatalf("DefaultCFIDThresholdCents = %d, want %d", DefaultCFIDThresholdCents, rg5700Cents)
	}
}
