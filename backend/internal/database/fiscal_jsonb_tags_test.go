package database

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestFiscalReceiptRawColumnsNotSerialized asserts that the raw AFIP SOAP
// request/response columns are never serialized to a client (json:"-"). They can
// carry sensitive provider payloads (and the columns are persisted), so they must
// not leak through ListReceipts to any fiscal:read holder (F-RAWCOLS).
func TestFiscalReceiptRawColumnsNotSerialized(t *testing.T) {
	r := FiscalReceipt{
		ID:          1,
		RawRequest:  map[string]interface{}{"token": "secret-soap-token"},
		RawResponse: map[string]interface{}{"cae": "12345"},
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal FiscalReceipt: %v", err)
	}
	out := string(b)
	for _, key := range []string{"raw_request", "raw_response", "secret-soap-token"} {
		if strings.Contains(out, key) {
			t.Errorf("FiscalReceipt JSON must not contain %q (raw SOAP columns must be json:\"-\"): %s", key, out)
		}
	}
}

// TestFiscalJsonbStructTags asserts that the four fiscal JSONB columns carry
// a "type:jsonb" directive in their gorm struct tags.  Without this directive,
// GORM AutoMigrate sees a serializer:json field with no explicit column type
// and attempts to ALTER the column to "text", which would fail (or silently
// truncate) on a production Postgres column that is already "jsonb".
//
// The genesis schema creates the columns as jsonb; the struct tags must
// declare that fact so AutoMigrate leaves them alone.
func TestFiscalJsonbStructTags(t *testing.T) {
	cases := []struct {
		structName string
		fieldName  string
		typ        interface{}
	}{
		{"BusinessFiscalSettings", "ProviderConfig", BusinessFiscalSettings{}},
		{"FiscalReceipt", "RawRequest", FiscalReceipt{}},
		{"FiscalReceipt", "RawResponse", FiscalReceipt{}},
		{"FiscalAuditEvent", "Metadata", FiscalAuditEvent{}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.structName+"."+tc.fieldName, func(t *testing.T) {
			rt := reflect.TypeOf(tc.typ)
			field, ok := rt.FieldByName(tc.fieldName)
			if !ok {
				t.Fatalf("field %s.%s not found", tc.structName, tc.fieldName)
			}
			tag := field.Tag.Get("gorm")
			if !strings.Contains(tag, "type:jsonb") {
				t.Errorf(
					"%s.%s gorm tag %q is missing \"type:jsonb\" — "+
						"AutoMigrate will attempt to ALTER the column to text",
					tc.structName, tc.fieldName, tag,
				)
			}
		})
	}
}
