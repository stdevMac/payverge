package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
)

// TestGetTimeseriesReturnsZeroFilledBuckets verifies that a valid [from,to] range
// returns the correct number of zero-filled day buckets and the correct range labels.
func TestGetTimeseriesReturnsZeroFilledBuckets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xTimeseriesOwner")

	// Seed one paid bill so the DB schema is exercised (revenue stays $0 for
	// this fixture because we don't create payments — zero-fill is sufficient).
	_ = seedTimeseriesBill(t, business.ID)

	router := newOwnerAnalyticsTestRouter("0xTimeseriesOwner")
	handler := NewAnalyticsHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/analytics/timeseries",
		server.RoleBasedAccessMiddleware("analytics:sales"),
		handler.GetTimeseries,
	)

	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/analytics/timeseries?from=2026-05-01&to=2026-05-03", business.ID),
		nil,
	)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Buckets []map[string]any `json:"buckets"`
			Range   struct {
				From string `json:"from"`
				To   string `json:"to"`
			} `json:"range"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.Len(t, body.Data.Buckets, 3)
	assert.Equal(t, "2026-05-01", body.Data.Range.From)
	assert.Equal(t, "2026-05-03", body.Data.Range.To)
}

// TestGetTimeseriesRejectsBadDate verifies that a non-YYYY-MM-DD `from` value
// returns HTTP 400.
func TestGetTimeseriesRejectsBadDate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xTimeseriesBadDate")

	router := newOwnerAnalyticsTestRouter("0xTimeseriesBadDate")
	handler := NewAnalyticsHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/analytics/timeseries",
		server.RoleBasedAccessMiddleware("analytics:sales"),
		handler.GetTimeseries,
	)

	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/analytics/timeseries?from=not-a-date", business.ID),
		nil,
	)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestGetTimeseriesRejectsInvertedRange verifies that to < from returns HTTP 400.
func TestGetTimeseriesRejectsInvertedRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xTimeseriesInverted")

	router := newOwnerAnalyticsTestRouter("0xTimeseriesInverted")
	handler := NewAnalyticsHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/analytics/timeseries",
		server.RoleBasedAccessMiddleware("analytics:sales"),
		handler.GetTimeseries,
	)

	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/analytics/timeseries?from=2026-05-10&to=2026-05-01", business.ID),
		nil,
	)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// seedTimeseriesBill inserts a minimal open bill so the Bill table row-count
// exercises the DB schema without affecting revenue totals.
func seedTimeseriesBill(t *testing.T, businessID uint) *database.Bill {
	t.Helper()
	now := time.Now().UTC()
	bill := &database.Bill{
		BusinessID:     businessID,
		BillNumber:     fmt.Sprintf("TSERIES-%d", now.UnixNano()),
		TotalAmount:    1000,
		Status:         database.BillStatusOpen,
		SettlementAddr: "0xsettlement",
		TippingAddr:    "0xtipping",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	return bill
}

func TestGetTimeseriesHonorsPeriodQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xTimeseriesPeriod")

	router := newOwnerAnalyticsTestRouter("0xTimeseriesPeriod")
	handler := NewAnalyticsHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/analytics/timeseries",
		server.RoleBasedAccessMiddleware("analytics:sales"),
		handler.GetTimeseries,
	)

	today := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/analytics/timeseries?period=today", business.ID),
		nil,
	)
	require.Equal(t, http.StatusOK, today.Code)
	var todayBody struct {
		Data struct {
			Buckets []map[string]any `json:"buckets"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(today.Body.Bytes(), &todayBody))
	assert.Len(t, todayBody.Data.Buckets, 1, "period=today must not return the default 30-day series")

	week := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/analytics/timeseries?period=7d", business.ID),
		nil,
	)
	require.Equal(t, http.StatusOK, week.Code)
	var weekBody struct {
		Data struct {
			Buckets []map[string]any `json:"buckets"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(week.Body.Bytes(), &weekBody))
	assert.Len(t, weekBody.Data.Buckets, 7, "period=7d must cover 7 local days, not 30")
	assert.NotEqual(t, len(todayBody.Data.Buckets), len(weekBody.Data.Buckets))
}

func TestGetTimeseriesHonorsMonthVsTodayPeriod(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xTimeseriesMonth")

	router := newOwnerAnalyticsTestRouter("0xTimeseriesMonth")
	handler := NewAnalyticsHandler(database.GetDBWrapper())
	router.GET("/inside/businesses/:id/analytics/timeseries",
		server.RoleBasedAccessMiddleware("analytics:sales"),
		handler.GetTimeseries,
	)

	today := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/analytics/timeseries?period=today", business.ID),
		nil,
	)
	require.Equal(t, http.StatusOK, today.Code)
	month := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/analytics/timeseries?period=month", business.ID),
		nil,
	)
	require.Equal(t, http.StatusOK, month.Code)

	var todayBody, monthBody struct {
		Data struct {
			Buckets []map[string]any `json:"buckets"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(today.Body.Bytes(), &todayBody))
	require.NoError(t, json.Unmarshal(month.Body.Bytes(), &monthBody))
	assert.Len(t, todayBody.Data.Buckets, 1)
	assert.Greater(t, len(monthBody.Data.Buckets), 1, "period=month must not collapse to today")
	assert.LessOrEqual(t, len(monthBody.Data.Buckets), 31)
	assert.NotEqual(t, len(todayBody.Data.Buckets), len(monthBody.Data.Buckets))
}
