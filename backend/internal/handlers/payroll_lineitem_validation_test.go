package handlers

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestCreatePayrollLineItemRequest_RejectsNegativeAmounts guards the binding
// tags on payroll amounts. Net = Gross + Bonus - Deduction, so all three are
// positive magnitudes — a negative DeductionAmount would INFLATE net pay
// (subtracting a negative), so negatives must be rejected at bind time.
func TestCreatePayrollLineItemRequest_RejectsNegativeAmounts(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name      string
		body      string
		wantBound bool
	}{
		{"valid", `{"payee_type":"staff","gross_amount":2000,"bonus_amount":100,"deduction_amount":50}`, true},
		{"valid zero bonus/deduction", `{"payee_type":"staff","gross_amount":2000}`, true},
		{"zero gross rejected", `{"payee_type":"staff","gross_amount":0}`, false},
		{"negative gross rejected", `{"payee_type":"staff","gross_amount":-100}`, false},
		{"negative bonus rejected", `{"payee_type":"staff","gross_amount":2000,"bonus_amount":-100}`, false},
		{"negative deduction rejected (would inflate net)", `{"payee_type":"staff","gross_amount":2000,"deduction_amount":-500}`, false},
		{"over-cap gross rejected", `{"payee_type":"staff","gross_amount":2000000}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/payroll", bytes.NewBufferString(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")

			var req CreatePayrollLineItemRequest
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
