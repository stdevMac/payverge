package database

import "testing"

func TestPrinter_TableName(t *testing.T) {
	if (Printer{}).TableName() != "printers" {
		t.Fatalf("expected printers, got %q", (Printer{}).TableName())
	}
}

func TestPrinter_DefaultsApplied(t *testing.T) {
	p := Printer{}
	if p.PaperWidthMM != 0 {
		t.Fatalf("zero-value should be 0; GORM applies default at insert time")
	}
	if p.Enabled {
		t.Fatalf("zero-value Enabled must be false; GORM applies default at insert")
	}
}
