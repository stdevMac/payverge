package events

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

const fullSplitPayload = `{"bill_number":"B-SLIM-1","status":"partial",` +
	`"total_amount":42.5,"total_cents":4250,"paid_amount":5.0,"paid_cents":500,` +
	`"held_amount":2.25,"held_cents":225,"available_amount":35.25,"available_cents":3525,` +
	`"updated_at":"2026-07-05T12:00:00Z","shares":[` +
	`{"id":"s1","display_name":"Alice","amount":5.0,"amount_cents":500,"tender":"cash","status":"settled"},` +
	`{"id":"s2","display_name":"Bob","amount_cents":225,"status":"held"},` +
	`{"id":"s3","display_name":"Cleo","amount_cents":0,"status":"pending"}]}`

// forbiddenSlimSubstrings must never appear in the operator-stream payload: the
// topic is gated at bills:read, but per-share amounts/tenders/payer names are
// financial:read signal.
var forbiddenSlimSubstrings = []string{
	"amount", "cents", "tender", "display_name", "available", "Alice", "Bob", "Cleo",
}

func TestSlimBillSplitUpdatedStripsFinancialDetail(t *testing.T) {
	slim := slimBillSplitUpdated(json.RawMessage(fullSplitPayload))
	s := string(slim)
	for _, forbidden := range forbiddenSlimSubstrings {
		if strings.Contains(s, forbidden) {
			t.Fatalf("slim operator payload leaks %q: %s", forbidden, s)
		}
	}

	var got map[string]any
	if err := json.Unmarshal(slim, &got); err != nil {
		t.Fatalf("slim payload is not valid JSON: %v", err)
	}
	if got["bill_number"] != "B-SLIM-1" {
		t.Fatalf("bill_number must survive slimming, got %#v", got["bill_number"])
	}
	if got["status"] != "partial" {
		t.Fatalf("status must survive slimming, got %#v", got["status"])
	}
	if got["share_count"].(float64) != 3 {
		t.Fatalf("share_count must be 3, got %#v", got["share_count"])
	}
	if got["settled_count"].(float64) != 1 {
		t.Fatalf("settled_count must be 1, got %#v", got["settled_count"])
	}
	if got["held_count"].(float64) != 1 {
		t.Fatalf("held_count must be 1, got %#v", got["held_count"])
	}
	if _, ok := got["shares"]; ok {
		t.Fatalf("per-share detail must be stripped, got %#v", got["shares"])
	}
}

func TestWriteBusinessSSESlimsBillSplitUpdated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	writeBusinessSSE(c, BusinessEvent{
		ID:         42,
		BusinessID: 9,
		Type:       "bill.split.updated",
		Data:       json.RawMessage(fullSplitPayload),
		Timestamp:  time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC),
	})

	body := rec.Body.String()
	for _, forbidden := range forbiddenSlimSubstrings {
		if strings.Contains(body, forbidden) {
			t.Fatalf("operator SSE frame leaks %q: %s", forbidden, body)
		}
	}
	if !strings.Contains(body, `"bill_number":"B-SLIM-1"`) {
		t.Fatalf("operator SSE frame must keep bill_number: %s", body)
	}
	if !strings.Contains(body, `"share_count":3`) {
		t.Fatalf("operator SSE frame must carry share_count: %s", body)
	}
}
