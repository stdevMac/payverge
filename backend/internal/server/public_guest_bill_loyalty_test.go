package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildPublicGuestBillResponse_IncludesLoyaltyDiscountDollars(t *testing.T) {
	t.Parallel()

	t.Run("zero when none", func(t *testing.T) {
		t.Parallel()
		got := buildPublicGuestBillResponse(&database.Bill{
			ID:                   7,
			BillNumber:           "B-NONE",
			PublicToken:          "tok-none",
			Subtotal:             3100,
			TaxAmount:            275,
			ServiceFeeAmount:     124,
			TotalAmount:          3499,
			PaidAmount:           1749,
			TipAmount:            0,
			LoyaltyDiscountCents: 0,
			Status:               database.BillStatusPartial,
		})

		require.Contains(t, got, "loyalty_discount",
			"guest helper must emit loyalty_discount even when no redemption is applied")
		assert.Equal(t, float64(0), got["loyalty_discount"])
		assert.Equal(t, 31.00, got["subtotal"])
		assert.Equal(t, 2.75, got["tax_amount"])
		assert.Equal(t, 1.24, got["service_fee_amount"])
		assert.Equal(t, 34.99, got["total_amount"])
		assertNoGuestLoyaltyPII(t, got)
	})

	t.Run("cents to dollars without re-dividing", func(t *testing.T) {
		t.Parallel()
		got := buildPublicGuestBillResponse(&database.Bill{
			ID:                   8,
			BillNumber:           "B-REDEEMED",
			PublicToken:          "tok-redeemed",
			Subtotal:             3100,
			TaxAmount:            275,
			ServiceFeeAmount:     124,
			TotalAmount:          3224,
			PaidAmount:           1749,
			TipAmount:            0,
			LoyaltyDiscountCents: 275,
			Status:               database.BillStatusPartial,
		})

		require.Contains(t, got, "loyalty_discount")
		assert.Equal(t, 2.75, got["loyalty_discount"],
			"loyalty_discount must use the same cents→dollars contract as other bill money fields")
		assert.NotEqual(t, float64(275), got["loyalty_discount"], "must not emit raw cents")
		assert.NotEqual(t, 0.0275, got["loyalty_discount"], "must not divide cents that are already dollars")
		assert.InDelta(t,
			got["subtotal"].(float64)+got["tax_amount"].(float64)+got["service_fee_amount"].(float64)-got["loyalty_discount"].(float64),
			got["total_amount"].(float64),
			0.001,
			"guest breakdown must reconcile: subtotal + tax + service fee − loyalty_discount == total",
		)
		assertNoGuestLoyaltyPII(t, got)
	})
}

func TestBuildPublicGuestBillResponse_IncludesRemaining(t *testing.T) {
	t.Parallel()
	got := buildPublicGuestBillResponse(&database.Bill{
		ID:          86,
		BillNumber:  "B-REMAIN",
		PublicToken: "tok-remain",
		Subtotal:    5000,
		TotalAmount: 5400,
		PaidAmount:  1800,
		Status:      database.BillStatusPartial,
	})
	require.Contains(t, got, "remaining", "guest bill payload must emit remaining like live-bills")
	assert.Equal(t, 36.00, got["remaining"], "remaining is dollars, not cents and not re-divided")
	assert.Equal(t, 54.00, got["total_amount"])
	assert.Equal(t, 18.00, got["paid_amount"])
}

func TestGuestBillSurfacesEmitLoyaltyDiscount(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name          string
		discountCents int64
		wantDollars   float64
	}{
		{name: "present and zero when none", discountCents: 0, wantDollars: 0},
		{name: "applied redemption in dollars", discountCents: 275, wantDollars: 2.75},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupPublicGuestTableHandlerTestDB(t)
			services.ResetPricingCache()

			tableCode := fmt.Sprintf("L%d", time.Now().UnixNano()%1_000_000_000)
			business := createSensitiveGuestTableBusiness(t, tableCode)
			table := createGuestPublicTable(t, business.ID, tableCode)

			subtotal, tax, fee := int64(3100), int64(275), int64(124)
			total := subtotal + tax + fee - tc.discountCents
			bill := &database.Bill{
				BusinessID:            business.ID,
				TableID:               table.ID,
				BillNumber:            fmt.Sprintf("B-loy-%d", time.Now().UnixNano()),
				Subtotal:              subtotal,
				TaxAmount:             tax,
				ServiceFeeAmount:      fee,
				TotalAmount:           total,
				PaidAmount:            1749,
				TipAmount:             0,
				LoyaltyDiscountCents:  tc.discountCents,
				LoyaltyPointsRedeemed: int(tc.discountCents),
				Status:                database.BillStatusPartial,
				SettlementAddr:        business.SettlementAddr,
				TippingAddr:           business.TippingAddr,
				CreatedAt:             time.Now(),
				UpdatedAt:             time.Now(),
			}
			require.NoError(t, database.GetDB().Create(bill).Error)
			require.NotEmpty(t, bill.PublicToken)

			surfaces := []struct {
				name    string
				handler gin.HandlerFunc
				params  gin.Params
				path    string
			}{
				{
					name:    "GET /guest/bill/:bill_token",
					handler: GetBillByNumberPublic,
					params:  gin.Params{{Key: "bill_token", Value: bill.PublicToken}},
					path:    fmt.Sprintf("/guest/bill/%s", bill.PublicToken),
				},
				{
					name:    "GET /guest/table/:code/bill",
					handler: GetOpenBillByTableCode,
					params:  gin.Params{{Key: "code", Value: table.TableCode}},
					path:    fmt.Sprintf("/guest/table/%s/bill", table.TableCode),
				},
			}

			for _, surface := range surfaces {
				t.Run(surface.name, func(t *testing.T) {
					w := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(w)
					c.Params = surface.params
					c.Request = httptest.NewRequest(http.MethodGet, surface.path, nil)

					surface.handler(c)

					require.Equal(t, http.StatusOK, w.Code, w.Body.String())

					var resp map[string]any
					require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
					billResp := requireMapField(t, resp, "bill")

					require.Contains(t, billResp, "loyalty_discount",
						"%s must include loyalty_discount (present and 0 when none)", surface.name)
					assert.Equal(t, tc.wantDollars, billResp["loyalty_discount"],
						"%s must emit loyalty_discount as dollars", surface.name)
					assert.Equal(t, 31.00, billResp["subtotal"])
					assert.Equal(t, 2.75, billResp["tax_amount"])
					assert.Equal(t, 1.24, billResp["service_fee_amount"])
					assert.InDelta(t,
						asJSONFloat(t, billResp["subtotal"])+asJSONFloat(t, billResp["tax_amount"])+asJSONFloat(t, billResp["service_fee_amount"])-asJSONFloat(t, billResp["loyalty_discount"]),
						asJSONFloat(t, billResp["total_amount"]),
						0.001,
					)
					assertNoGuestLoyaltyPII(t, billResp)
					assertSensitiveBillFieldsHidden(t, billResp)
				})
			}
		})
	}
}

func assertNoGuestLoyaltyPII(t *testing.T, payload map[string]any) {
	t.Helper()
	assert.NotContains(t, payload, "loyalty_redeemed_by_customer_id")
	assert.NotContains(t, payload, "loyalty_discount_cents")
	assert.NotContains(t, payload, "crm_customer_id")
}

func asJSONFloat(t *testing.T, v any) float64 {
	t.Helper()
	n, ok := v.(float64)
	require.True(t, ok, "expected JSON number, got %T (%v)", v, v)
	return n
}
