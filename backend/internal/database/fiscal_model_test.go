package database

import "testing"

func TestFiscalTableNames(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"settings", (BusinessFiscalSettings{}).TableName(), "business_fiscal_settings"},
		{"receipt", (FiscalReceipt{}).TableName(), "fiscal_receipts"},
		{"job", (FiscalJob{}).TableName(), "fiscal_jobs"},
		{"audit", (FiscalAuditEvent{}).TableName(), "fiscal_audit_events"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("expected %s, got %s", tt.want, tt.got)
			}
		})
	}
}

func TestFiscalMode_IsValid(t *testing.T) {
	for _, mode := range []FiscalMode{
		FiscalModeOff,
		FiscalModeManual,
		FiscalModeAutomaticNonBlocking,
	} {
		if !mode.IsValid() {
			t.Fatalf("expected mode %q to be valid", mode)
		}
	}
	// automatic_blocking never blocked anything and is not a mode.
	if FiscalMode("automatic_blocking").IsValid() {
		t.Fatal("expected removed mode automatic_blocking to be invalid")
	}
	if FiscalMode("strict").IsValid() {
		t.Fatal("expected unknown mode to be invalid")
	}
}

func TestFiscalStatus_IsValid(t *testing.T) {
	for _, status := range []FiscalStatus{
		FiscalStatusPending,
		FiscalStatusAuthorized,
		FiscalStatusRejected,
		FiscalStatusFailedRetryable,
		FiscalStatusFailedPermanent,
		FiscalStatusCancelled,
		FiscalStatusCredited,
	} {
		if !status.IsValid() {
			t.Fatalf("expected %q to be valid", status)
		}
	}
	if FiscalStatus("lost").IsValid() {
		t.Fatal("unexpected valid fiscal status")
	}
}
