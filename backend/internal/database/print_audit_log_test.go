package database

import "testing"

func TestPrintAuditLog_TableName(t *testing.T) {
	if (PrintAuditLog{}).TableName() != "print_audit_log" {
		t.Fatalf("expected print_audit_log, got %q", (PrintAuditLog{}).TableName())
	}
}
