package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"
)

func TestGuestOrderRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	max := strings.Repeat("a", services.MaxGuestCheckoutRequestIDLen)

	cases := []struct {
		name      string
		header    *string
		wantID    string
		wantOK    bool
		wantError string
		wantCode  string
	}{
		{name: "absent", header: nil, wantError: "missing_request_id", wantCode: "request_id_required"},
		{name: "empty", header: reqIDPtr(""), wantError: "missing_request_id", wantCode: "request_id_required"},
		{name: "whitespace", header: reqIDPtr("   "), wantError: "missing_request_id", wantCode: "request_id_required"},
		{name: "uuid", header: reqIDPtr("3f1c9a4e-7b2d-4c8e-9f10-2a6b5d4e3c21"), wantID: "3f1c9a4e-7b2d-4c8e-9f10-2a6b5d4e3c21", wantOK: true},
		{name: "trimmed", header: reqIDPtr("  order-1  "), wantID: "order-1", wantOK: true},
		{name: "exactly max", header: reqIDPtr(max), wantID: max, wantOK: true},
		{name: "max after trim", header: reqIDPtr(" " + max + " "), wantID: max, wantOK: true},
		{name: "one over max", header: reqIDPtr(max + "a"), wantError: "invalid_request_id", wantCode: "request_id_invalid"},
		// Length is bytes, matching the service-side dedupe contract.
		{name: "multibyte over max bytes", header: reqIDPtr(strings.Repeat("é", 33)), wantError: "invalid_request_id", wantCode: "request_id_invalid"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/guest/table/T1/order", nil)
			if tc.header != nil {
				c.Request.Header.Set("X-Request-Id", *tc.header)
			}

			id, ok := guestOrderRequestID(c)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantID, id)
			if tc.wantOK {
				assert.False(t, c.Writer.Written(), "valid id must not write a response")
				return
			}
			require.Equal(t, http.StatusBadRequest, w.Code)
			var body map[string]string
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, tc.wantError, body["error"])
			assert.Equal(t, tc.wantCode, body["code"])
			assert.Contains(t, body["message"], "X-Request-Id")
			assert.Len(t, body, 3, "error body is exactly error, code, message")
		})
	}
}

func TestGuestOrderRequestIDCodes(t *testing.T) {
	// request_id_required is localized by the guest frontend (apiErrors.json);
	// renaming it silently breaks the guest toast.
	assert.Equal(t, "request_id_required", ErrCodeRequestIDRequired)
	assert.Equal(t, "request_id_invalid", ErrCodeRequestIDInvalid)
	assert.Equal(t, 64, services.MaxGuestCheckoutRequestIDLen)
}

func TestCreateGuestOrder_RequestIDContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicGuestTableHandlerTestDBWithLogger(t, logger.Discard)
	services.ResetPricingCache()
	services.ResetTelegramNotificationEligibilityCache()

	business := createSensitiveGuestTableBusiness(t, "REQID001")
	table := createGuestPublicTable(t, business.ID, "REQID001")
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: business.ID, IsActive: true,
		Categories: `[{"id":"cat1","name":"Drinks","items":[{"id":"item1","name":"Coffee","price":3.5,"is_available":true}]}]`,
	}).Error)
	bill := database.Bill{
		BusinessID: business.ID, TableID: table.ID, Status: database.BillStatusOpen,
		BillNumber: "B-REQID-1", SettlementAddr: business.SettlementAddr, TippingAddr: business.TippingAddr,
	}
	require.NoError(t, database.GetDB().Create(&bill).Error)
	body := fmt.Sprintf(`{"bill_id":%d,"items":[{"menu_item_id":"item1","menu_item_name":"Coffee","quantity":1,"price":3.5}]}`, bill.ID)

	post := func(requestID *string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
		c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/guest/table/%s/order", table.TableCode), strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		if requestID != nil {
			c.Request.Header.Set("X-Request-Id", *requestID)
		}
		CreateGuestOrder(c)
		return w
	}
	orderCount := func() int64 {
		var n int64
		require.NoError(t, database.GetDB().Model(&database.Order{}).Where("business_id = ?", business.ID).Count(&n).Error)
		return n
	}

	w := post(nil)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"error":"missing_request_id"`)
	assert.Contains(t, w.Body.String(), `"code":"request_id_required"`)

	w = post(reqIDPtr(strings.Repeat("x", services.MaxGuestCheckoutRequestIDLen+1)))
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"error":"invalid_request_id"`)
	assert.Contains(t, w.Body.String(), `"code":"request_id_invalid"`)
	assert.Equal(t, int64(0), orderCount(), "rejected request ids must not create orders")

	// A 64-byte id is accepted, and a retry with it replays the same order.
	maxID := strings.Repeat("y", services.MaxGuestCheckoutRequestIDLen)
	w = post(&maxID)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	w = post(&maxID)
	require.Equal(t, http.StatusOK, w.Code, "same X-Request-Id must replay, not create: %s", w.Body.String())
	assert.Equal(t, int64(1), orderCount())
}

func reqIDPtr(s string) *string { return &s }
