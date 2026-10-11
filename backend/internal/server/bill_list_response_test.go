package server

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// The bill list projection joins tables.name as table_name; the response
// shape must carry it through, or list UIs regress to the raw-ID fallback
// ("Table 237" instead of "Table 9").
func TestBillListResponseCarriesTableName(t *testing.T) {
	counterID := uint(7)
	rows := []database.BillListRow{
		{ID: 1, TableID: 237, TableName: "Table 9", BillNumber: "DEMO-1"},
		{ID: 2, TableID: 0, CounterID: &counterID, BillNumber: "DEMO-2"},
	}

	payload, err := json.Marshal(shapeBillListResponse(rows, "USD"))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(payload)

	if !strings.Contains(body, `"table_name":"Table 9"`) {
		t.Fatalf("expected table_name to survive response shaping, got %s", body)
	}
	if !strings.Contains(body, `"counter_id":7`) {
		t.Fatalf("expected counter_id to survive response shaping, got %s", body)
	}
}

func TestBillListResponseOmitsSettlementWallets(t *testing.T) {
	// FIND-045: list poll must not fan settlement/tipping wallets to every row.
	rows := []database.BillListRow{
		{
			ID: 9, BillNumber: "B-W", Status: database.BillStatusOpen,
			// Even if a row accidentally carried wallet columns, the response
			// shape must not serialize them.
		},
	}
	payload, err := json.Marshal(shapeBillListResponse(rows, "USD"))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(payload)
	for _, banned := range []string{"settlement_address", "tipping_address", "0x"} {
		if strings.Contains(body, banned) {
			t.Fatalf("bill list response must not include %q, got %s", banned, body)
		}
	}
	if !strings.Contains(body, `"bill_number":"B-W"`) {
		t.Fatalf("expected bill_number to survive, got %s", body)
	}
}

// FIND-057: empty legacy items snapshots must not pollute the list wire when
// item_count is the list UI source of truth (G-03). Non-empty snapshots stay
// for pre-bill_items fallback.
func TestBillListResponseOmitsEmptyItemsSnapshot(t *testing.T) {
	rows := []database.BillListRow{
		{ID: 1, BillNumber: "B-empty", Items: "[]", ItemCount: 3},
		{ID: 2, BillNumber: "B-blank", Items: "", ItemCount: 0},
		{ID: 3, BillNumber: "B-legacy", Items: `[{"name":"Old","quantity":1}]`, ItemCount: 0},
	}
	payload, err := json.Marshal(shapeBillListResponse(rows, "USD"))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out []map[string]any
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(out))
	}
	if _, ok := out[0]["items"]; ok {
		t.Fatalf("empty [] snapshot must be omitted, got %v", out[0]["items"])
	}
	if _, ok := out[1]["items"]; ok {
		t.Fatalf("blank snapshot must be omitted, got %v", out[1]["items"])
	}
	got, ok := out[2]["items"].([]any)
	if !ok || len(got) != 1 {
		t.Fatalf("legacy non-empty snapshot must survive as an array, got %v (%T)", out[2]["items"], out[2]["items"])
	}
	line, _ := got[0].(map[string]any)
	if line["name"] != "Old" {
		t.Fatalf("legacy snapshot line must keep name, got %v", got[0])
	}
	if out[0]["item_count"] != float64(3) {
		t.Fatalf("item_count must survive for modern bills, got %v", out[0]["item_count"])
	}
}

func TestBillListResponseIncludesRemaining(t *testing.T) {
	rows := []database.BillListRow{
		{ID: 1, BillNumber: "B-due", TotalAmount: 1800, PaidAmount: 0, Status: database.BillStatusOpen},
		{ID: 2, BillNumber: "B-partial", TotalAmount: 5000, PaidAmount: 2000, Status: database.BillStatusPartial},
	}
	payload, err := json.Marshal(shapeBillListResponse(rows, "USD"))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out []map[string]any
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out[0]["remaining"] != 18.0 {
		t.Fatalf("open remaining want 18, got %v", out[0]["remaining"])
	}
	if out[1]["remaining"] != 30.0 {
		t.Fatalf("partial remaining want 30, got %v", out[1]["remaining"])
	}
}
