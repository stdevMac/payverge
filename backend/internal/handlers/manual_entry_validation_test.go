package handlers

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestCreateManualEntryRequest_RejectsNonPositiveAmount guards the binding on
// CreateManualEntryRequest.Amount. Direction comes from EntryType (income vs
// expense) and Amount is a positive magnitude summed per-type, so a negative
// amount would reverse the direction (negative "income" reduces revenue) and
// corrupt the books — it must be rejected at bind time.
func TestCreateManualEntryRequest_RejectsNonPositiveAmount(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name      string
		body      string
		wantBound bool
	}{
		{"valid income", `{"entry_type":"income","category":"other","amount":1500,"occurred_at":"2026-05-31T00:00:00Z","description":"catering"}`, true},
		{"zero rejected", `{"entry_type":"income","category":"other","amount":0,"occurred_at":"2026-05-31T00:00:00Z","description":"x"}`, false},
		{"negative rejected (would reverse direction)", `{"entry_type":"income","category":"other","amount":-1500,"occurred_at":"2026-05-31T00:00:00Z","description":"x"}`, false},
		{"absurd over-cap rejected", `{"entry_type":"expense","category":"other","amount":200000000,"occurred_at":"2026-05-31T00:00:00Z","description":"x"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/manual-entry", bytes.NewBufferString(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")

			var req CreateManualEntryRequest
			err := c.ShouldBindJSON(&req)
			if tc.wantBound && err != nil {
				t.Fatalf("expected %s to bind, got error: %v", tc.body, err)
			}
			if !tc.wantBound && err == nil {
				t.Fatalf("expected %s to be REJECTED, but it bound", tc.body)
			}
		})
	}
}
