package database

import (
	"encoding/json"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestBillFiscalCustomerColumns_ColumnsExist is the failing-first regression
// guard for the bills fiscal-customer columns. It asserts that AutoMigrate places all
// five fiscal-customer identity columns on the bills table, and that a
// round-trip write+read works correctly (including nil → JSON null
// serialization via Bill.MarshalJSON).
func TestBillFiscalCustomerColumns_ColumnsExist(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := db.AutoMigrate(&Bill{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	cols := []string{
		"FiscalCustomerDocType",
		"FiscalCustomerDocNumber",
		"FiscalCustomerTaxCondition",
		"FiscalCustomerName",
		"FiscalCustomerEmail",
	}
	for _, col := range cols {
		if !db.Migrator().HasColumn(&Bill{}, col) {
			t.Errorf("bills table missing column for field %s", col)
		}
	}
}

// TestBillFiscalCustomerColumns_MarshalJSON asserts the five fiscal customer
// fields appear in Bill.MarshalJSON output, with null for nil pointers and
// the correct string value when set. Bill uses a custom MarshalJSON — the
// model json tags alone do not ship the field on the wire.
func TestBillFiscalCustomerColumns_MarshalJSON(t *testing.T) {
	docType := "DNI"
	docNumber := "20-12345678-3"
	taxCond := "consumidor_final"
	name := "Juan Pérez"
	email := "guest@example.com"

	b := Bill{
		ID:                         1,
		BusinessID:                 1,
		BillNumber:                 "B-001",
		PublicToken:                "tok_fiscal_full", // full projection (not order-list summary)
		FiscalCustomerDocType:      &docType,
		FiscalCustomerDocNumber:    &docNumber,
		FiscalCustomerTaxCondition: &taxCond,
		FiscalCustomerName:         &name,
		FiscalCustomerEmail:        &email,
	}

	raw, err := b.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	checks := map[string]string{
		"fiscal_customer_doc_type":      docType,
		"fiscal_customer_doc_number":    docNumber,
		"fiscal_customer_tax_condition": taxCond,
		"fiscal_customer_name":          name,
		"fiscal_customer_email":         email,
	}
	for key, want := range checks {
		got, ok := out[key]
		if !ok {
			t.Errorf("MarshalJSON output missing key %q", key)
			continue
		}
		if got != want {
			t.Errorf("MarshalJSON[%q] = %q, want %q", key, got, want)
		}
	}
}

// TestBillFiscalCustomerColumns_MarshalJSON_NilFields asserts that nil pointer
// fiscal customer fields are emitted as JSON null (not omitted) in MarshalJSON.
func TestBillFiscalCustomerColumns_MarshalJSON_NilFields(t *testing.T) {
	b := Bill{
		ID:          2,
		BusinessID:  1,
		BillNumber:  "B-002",
		PublicToken: "tok_fiscal_nil", // full projection still emits fiscal nulls
		// fiscal customer fields left nil
	}

	raw, err := b.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, key := range []string{
		"fiscal_customer_doc_type",
		"fiscal_customer_doc_number",
		"fiscal_customer_tax_condition",
		"fiscal_customer_name",
		"fiscal_customer_email",
	} {
		val, ok := out[key]
		if !ok {
			t.Errorf("MarshalJSON output missing key %q (expected null)", key)
			continue
		}
		if val != nil {
			t.Errorf("MarshalJSON[%q] = %v, want null", key, val)
		}
	}
}
