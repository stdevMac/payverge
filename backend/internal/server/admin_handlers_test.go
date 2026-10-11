package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAdminStatsLedgerTestDB(t testing.TB) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.User{},
		&database.Bill{},
		&database.Payment{},
		&database.AlternativePayment{},
		&database.BusinessRevenueAggregate{},
	))
	return gormDB
}

func TestGetBusinessListEmitsLastActiveWithoutBillingDates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAdminStatsLedgerTestDB(t)

	lastActive := time.Date(2026, time.May, 20, 9, 30, 0, 0, time.UTC)
	require.NoError(t, db.Create(&database.Business{
		BusinessId: "admin-list-dates",
		Name:       "Admin List Dates",
		OwnerName:  "Owner",
		CreatedAt:  time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:  lastActive,
	}).Error)

	router := gin.New()
	router.GET("/admin/businesses", GetBusinessList)

	req := httptest.NewRequest(http.MethodGet, "/admin/businesses", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Businesses []map[string]interface{} `json:"businesses"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Businesses, 1)
	_, hasLastPayment := response.Businesses[0]["last_payment_date"]
	assert.False(t, hasLastPayment, "billing dates are gone from the admin business list")
	assert.Equal(t, lastActive.Format(time.RFC3339Nano), response.Businesses[0]["last_active_at"])
}

func TestAdminPaymentMetricsUseRecognizedLedgerForAllBusinesses(t *testing.T) {
	db := setupAdminStatsLedgerTestDB(t)
	now := time.Date(2026, time.May, 20, 12, 0, 0, 0, time.UTC)
	previousAdminStatsNow := adminStatsNow
	adminStatsNow = func() time.Time { return now }
	t.Cleanup(func() {
		adminStatsNow = previousAdminStatsNow
	})

	currentPaymentAt := time.Date(2026, time.May, 10, 12, 0, 0, 0, time.UTC)
	previousMonth := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	reversedAt := time.Date(2026, time.May, 12, 12, 0, 0, 0, time.UTC)

	firstBusiness := createAdminStatsLedgerBusiness(t, db, "Admin Stats One")
	secondBusiness := createAdminStatsLedgerBusiness(t, db, "Admin Stats Two")

	confirmedBill := createAdminStatsLedgerBill(t, db, firstBusiness.ID, "ADMIN-STATS-CONFIRMED", 1000, 1000, database.BillStatusPaid, currentPaymentAt)
	require.NoError(t, db.Create(&database.Payment{
		BillID:        confirmedBill.ID,
		PayerAddr:     "0xadminstatsconfirmed",
		Amount:        1000,
		TipAmount:     100,
		TxHash:        "admin_stats_confirmed",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &currentPaymentAt,
		CreatedAt:     currentPaymentAt,
		UpdatedAt:     currentPaymentAt,
	}).Error)

	alternativeBill := createAdminStatsLedgerBill(t, db, secondBusiness.ID, "ADMIN-STATS-ALT", 500, 500, database.BillStatusPaid, currentPaymentAt)
	require.NoError(t, db.Create(&database.AlternativePayment{
		BillID:          alternativeBill.ID,
		ParticipantAddr: "cashier",
		Amount:          500,
		PaymentMethod:   database.PaymentMethodCash,
		Status:          database.AltPaymentStatusConfirmed,
		CreatedAt:       currentPaymentAt,
		UpdatedAt:       currentPaymentAt,
		ConfirmedAt:     &currentPaymentAt,
	}).Error)
	require.NoError(t, db.Create(&database.AlternativePayment{
		BillID:          alternativeBill.ID,
		ParticipantAddr: "plugin-paypal",
		Amount:          9999,
		PaymentMethod:   database.AlternativePaymentMethod("paypal"),
		Status:          database.AltPaymentStatusConfirmed,
		CreatedAt:       currentPaymentAt,
		UpdatedAt:       currentPaymentAt,
		ConfirmedAt:     &currentPaymentAt,
	}).Error)

	reversedBill := createAdminStatsLedgerBill(t, db, firstBusiness.ID, "ADMIN-STATS-REVERSED", 700, 0, database.BillStatusOpen, previousMonth)
	require.NoError(t, db.Create(&database.Payment{
		BillID:        reversedBill.ID,
		PayerAddr:     "0xadminstatsreversed",
		Amount:        700,
		TipAmount:     70,
		TxHash:        "admin_stats_reversed",
		Status:        database.PaymentStatusReversed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &previousMonth,
		ReversedAt:    &reversedAt,
		CreatedAt:     previousMonth,
		UpdatedAt:     reversedAt,
	}).Error)
	require.NoError(t, db.Create(&database.BusinessRevenueAggregate{
		BusinessID:          firstBusiness.ID,
		NetRevenueCents:     1000,
		NetTipCents:         100,
		GrossRevenueCents:   1700,
		GrossTipCents:       170,
		PositiveEventCount:  2,
		RecognizedBillCount: 1,
	}).Error)
	require.NoError(t, db.Create(&database.BusinessRevenueAggregate{
		BusinessID:          secondBusiness.ID,
		NetRevenueCents:     500,
		NetTipCents:         0,
		GrossRevenueCents:   500,
		GrossTipCents:       0,
		PositiveEventCount:  1,
		RecognizedBillCount: 1,
	}).Error)

	stats := AdminStats{}
	require.NoError(t, getPaymentMetrics(&stats))

	assert.InDelta(t, 15.0, stats.GrossMerchandiseVolume, 0.01)
	assert.InDelta(t, 16.0, stats.TotalPaymentVolume, 0.01)
	assert.InDelta(t, 7.90, stats.AverageTransactionSize, 0.01)

	currentMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).Format("Jan 2006")
	currentGrowth := findMonthlyGrowth(t, stats.PaymentVolumeGrowth, currentMonth)
	assert.InDelta(t, 8.30, currentGrowth.Value, 0.01)
	assert.Equal(t, int64(2), currentGrowth.Count)

	currentRevenueGrowth := findMonthlyGrowth(t, stats.RevenueGrowth, currentMonth)
	assert.InDelta(t, 8.0, currentRevenueGrowth.Value, 0.01)
	assert.Equal(t, int64(2), currentRevenueGrowth.Count)

	previousRevenueGrowth := findMonthlyGrowth(t, stats.RevenueGrowth, previousMonth.Format("Jan 2006"))
	assert.InDelta(t, 7.0, previousRevenueGrowth.Value, 0.01)
	assert.Equal(t, int64(1), previousRevenueGrowth.Count)
}

func TestAdminMetricsFallBackToLedgerWhenRevenueAggregatesArePartiallySeeded(t *testing.T) {
	db := setupAdminStatsLedgerTestDB(t)
	now := time.Date(2026, time.May, 20, 12, 0, 0, 0, time.UTC)
	previousAdminStatsNow := adminStatsNow
	adminStatsNow = func() time.Time { return now }
	t.Cleanup(func() {
		adminStatsNow = previousAdminStatsNow
	})

	firstBusiness := createAdminStatsLedgerBusiness(t, db, "Seeded Business")
	secondBusiness := createAdminStatsLedgerBusiness(t, db, "Unseeded Business")

	firstPaidAt := time.Date(2026, time.May, 8, 12, 0, 0, 0, time.UTC)
	firstBill := createAdminStatsLedgerBill(t, db, firstBusiness.ID, "ADMIN-PARTIAL-AGGREGATE-SEEDED", 1000, 1000, database.BillStatusPaid, firstPaidAt)
	require.NoError(t, db.Create(&database.Payment{
		BillID:        firstBill.ID,
		PayerAddr:     "0xseeded",
		Amount:        1000,
		TipAmount:     100,
		TxHash:        "admin_partial_aggregate_seeded",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &firstPaidAt,
		CreatedAt:     firstPaidAt,
		UpdatedAt:     firstPaidAt,
	}).Error)
	require.NoError(t, db.Create(&database.BusinessRevenueAggregate{
		BusinessID:          firstBusiness.ID,
		NetRevenueCents:     1000,
		NetTipCents:         100,
		GrossRevenueCents:   1000,
		GrossTipCents:       100,
		PositiveEventCount:  1,
		RecognizedBillCount: 1,
	}).Error)

	secondPaidAt := time.Date(2026, time.May, 9, 12, 0, 0, 0, time.UTC)
	secondBill := createAdminStatsLedgerBill(t, db, secondBusiness.ID, "ADMIN-PARTIAL-AGGREGATE-UNSEEDED", 500, 500, database.BillStatusPaid, secondPaidAt)
	require.NoError(t, db.Create(&database.Payment{
		BillID:        secondBill.ID,
		PayerAddr:     "0xunseeded",
		Amount:        500,
		TxHash:        "admin_partial_aggregate_unseeded",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &secondPaidAt,
		CreatedAt:     secondPaidAt,
		UpdatedAt:     secondPaidAt,
	}).Error)

	stats := AdminStats{}
	require.NoError(t, getPaymentMetrics(&stats))
	require.NoError(t, getBillMetrics(&stats))

	assert.InDelta(t, 15.0, stats.GrossMerchandiseVolume, 0.01)
	assert.InDelta(t, 16.0, stats.TotalPaymentVolume, 0.01)
	assert.InDelta(t, 8.0, stats.AverageTransactionSize, 0.01)
	assert.Equal(t, int64(2), stats.RecognizedBills)

	currentGrowth := findMonthlyGrowth(t, stats.BillGrowth, now.Format("Jan 2006"))
	assert.Equal(t, int64(2), currentGrowth.Count)
}

func TestAdminBillMetricsExposeRecognizedBillCount(t *testing.T) {
	db := setupAdminStatsLedgerTestDB(t)
	now := time.Date(2026, time.May, 20, 12, 0, 0, 0, time.UTC)
	previousAdminStatsNow := adminStatsNow
	adminStatsNow = func() time.Time { return now }
	t.Cleanup(func() {
		adminStatsNow = previousAdminStatsNow
	})

	business := createAdminStatsLedgerBusiness(t, db, "Admin Bill Metrics")
	confirmedAt := time.Date(2026, time.May, 10, 12, 0, 0, 0, time.UTC)
	reversedAt := confirmedAt.Add(2 * time.Hour)

	recognizedBill := createAdminStatsLedgerBill(t, db, business.ID, "ADMIN-BILLS-RECOGNIZED", 1000, 1000, database.BillStatusPartial, confirmedAt)
	require.NoError(t, db.Create(&database.Payment{
		BillID:        recognizedBill.ID,
		PayerAddr:     "0xadminbillsrecognized",
		Amount:        1000,
		TxHash:        "admin_bills_recognized",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)

	reversedBill := createAdminStatsLedgerBill(t, db, business.ID, "ADMIN-BILLS-REVERSED", 1000, 1000, database.BillStatusPartial, confirmedAt)
	require.NoError(t, db.Create(&database.Payment{
		BillID:        reversedBill.ID,
		PayerAddr:     "0xadminbillsreversed",
		Amount:        1000,
		TxHash:        "admin_bills_reversed",
		Status:        database.PaymentStatusReversed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		ReversedAt:    &reversedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     reversedAt,
	}).Error)

	createAdminStatsLedgerBill(t, db, business.ID, "ADMIN-BILLS-UNPAID", 1000, 0, database.BillStatusOpen, confirmedAt)

	stats := AdminStats{}
	require.NoError(t, getBillMetrics(&stats))

	assert.Equal(t, int64(3), stats.TotalBills)
	assert.Equal(t, int64(1), stats.RecognizedBills)
	assert.Equal(t, int64(2), stats.BillsByStatus[string(database.BillStatusPartial)])
	assert.Equal(t, int64(1), stats.BillsByStatus[string(database.BillStatusOpen)])
	currentGrowth := findMonthlyGrowth(t, stats.BillGrowth, now.Format("Jan 2006"))
	assert.Equal(t, int64(3), currentGrowth.Count)
}

func createAdminStatsLedgerBusiness(t *testing.T, db *gorm.DB, name string) database.Business {
	t.Helper()

	business := database.Business{
		BusinessId: fmt.Sprintf("admin-stats-%d", time.Now().UnixNano()),
		Name:       name,
	}
	require.NoError(t, db.Create(&business).Error)
	return business
}

func createAdminStatsLedgerBill(t *testing.T, db *gorm.DB, businessID uint, number string, total, paid int64, status database.BillStatus, at time.Time) database.Bill {
	t.Helper()

	bill := database.Bill{
		BusinessID:     businessID,
		BillNumber:     fmt.Sprintf("%s-%d", number, time.Now().UnixNano()),
		Subtotal:       total,
		TotalAmount:    total,
		PaidAmount:     paid,
		Status:         status,
		SettlementAddr: "settlement-" + number,
		TippingAddr:    "tipping-" + number,
		CreatedAt:      at,
		UpdatedAt:      at,
	}
	require.NoError(t, db.Create(&bill).Error)
	return bill
}

func findMonthlyGrowth(t *testing.T, rows []MonthlyGrowth, month string) MonthlyGrowth {
	t.Helper()

	for _, row := range rows {
		if row.Month == month {
			return row
		}
	}

	require.Failf(t, "missing monthly growth", "month %q was not found", month)
	return MonthlyGrowth{}
}

// TestGetOperationalMetrics_FailedWebhooksExcludesProcessing guards audit F2:
// the admin "Failed Webhooks" count is the dead-letter queue (terminal failures)
// only — it must not include in-flight 'processing' retries or 'processed' events.
func TestGetOperationalMetrics_FailedWebhooksExcludesProcessing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAdminStatsLedgerTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.WebhookEvent{}))

	now := time.Now()
	mk := func(id, status string) {
		require.NoError(t, db.Create(&database.WebhookEvent{
			Provider:   "stripe",
			WebhookID:  id,
			EventType:  "invoice.paid",
			Status:     status,
			ReceivedAt: now,
		}).Error)
	}
	mk("evt_failed_1", "failed")
	mk("evt_processing_1", "processing")
	mk("evt_processed_1", "processed")

	var stats AdminStats
	require.NoError(t, getOperationalMetrics(&stats))
	assert.Equal(t, 1, stats.FailedWebhooksCount,
		"DLQ count must be terminal failures only — not in-flight 'processing' or completed 'processed'")
}

// TestGetBusinessListExcludesDemoByDefault guards audit F1 / Task 12: the admin
// business list defaults to kind=real (demo/test excluded) and returns non-real
// rows only when kind=all is passed. The removed include_demo alias is ignored.
func TestGetBusinessListExcludesDemoByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAdminStatsLedgerTestDB(t)

	now := time.Now()
	require.NoError(t, db.Create(&database.Business{
		BusinessId: "f1-prod", Name: "Prod Biz", OwnerName: "Owner",
		Kind: database.BusinessKindReal, IsDemo: false, CreatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&database.Business{
		BusinessId: "f1-demo", Name: "Demo Biz", OwnerName: "Tester",
		Kind: database.BusinessKindDemo, IsDemo: true, CreatedAt: now,
	}).Error)

	router := gin.New()
	router.GET("/admin/businesses", GetBusinessList)

	var resp struct {
		Businesses []map[string]interface{} `json:"businesses"`
		Total      int64                    `json:"total"`
	}

	// Default: demo excluded (kind=real).
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/businesses", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.EqualValues(t, 1, resp.Total, "demo businesses must be excluded by default")
	require.Len(t, resp.Businesses, 1)
	assert.Equal(t, "Prod Biz", resp.Businesses[0]["name"])

	// kind=all: both returned.
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/admin/businesses?kind=all", nil))
	require.Equal(t, http.StatusOK, w2.Code)
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp))
	require.EqualValues(t, 2, resp.Total, "kind=all must return demo + production")

	// The old include_demo alias is gone: it no longer widens the list.
	w2b := httptest.NewRecorder()
	router.ServeHTTP(w2b, httptest.NewRequest(http.MethodGet, "/admin/businesses?include_demo=true", nil))
	require.Equal(t, http.StatusOK, w2b.Code)
	require.NoError(t, json.Unmarshal(w2b.Body.Bytes(), &resp))
	require.EqualValues(t, 1, resp.Total, "include_demo is not a supported query parameter")

	// kind=demo returns only the seeder row.
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, httptest.NewRequest(http.MethodGet, "/admin/businesses?kind=demo", nil))
	require.Equal(t, http.StatusOK, w3.Code)
	require.NoError(t, json.Unmarshal(w3.Body.Bytes(), &resp))
	require.EqualValues(t, 1, resp.Total)
	assert.Equal(t, "Demo Biz", resp.Businesses[0]["name"])
}

// BenchmarkGetBusinessList baselines the admin list with the is_demo exclusion
// filter over 500 production + 50 demo businesses (audit F1 performance gate).
func BenchmarkGetBusinessList(b *testing.B) {
	gin.SetMode(gin.TestMode)
	db := setupAdminStatsLedgerTestDB(b)

	// Go re-invokes the benchmark function during calibration; the shared-cache
	// in-memory DB (keyed by b.Name()) persists rows across invocations, so seed
	// only once to avoid duplicate-business_id inserts.
	var seeded int64
	db.Model(&database.Business{}).Count(&seeded)
	if seeded == 0 {
		now := time.Now()
		for i := 0; i < 500; i++ {
			require.NoError(b, db.Create(&database.Business{
				BusinessId: fmt.Sprintf("bench-prod-%d", i), Name: fmt.Sprintf("Bench %d", i),
				OwnerName: "Benchmarker",
				Kind:      database.BusinessKindReal, IsDemo: false,
				CreatedAt: now.Add(-time.Duration(i) * time.Hour),
			}).Error)
		}
		for i := 0; i < 50; i++ {
			require.NoError(b, db.Create(&database.Business{
				BusinessId: fmt.Sprintf("bench-demo-%d", i), Name: fmt.Sprintf("Demo %d", i),
				OwnerName: "Tester",
				Kind:      database.BusinessKindDemo, IsDemo: true,
				CreatedAt: now.Add(-time.Duration(i) * time.Hour),
			}).Error)
		}
	}

	router := gin.New()
	router.GET("/admin/businesses", GetBusinessList)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/businesses?page=1&limit=20", nil))
		if w.Code != http.StatusOK {
			b.Fatalf("unexpected status %d", w.Code)
		}
	}
}

// TestAdminStatsPayloadReportsGMVWithoutPlatformBilling guards finding #59:
// TotalRevenue was restaurant GMV mislabeled as platform revenue. The admin
// stats payload exposes gross_merchandise_volume (guest takings), never emits
// total_revenue, and — with SaaS billing removed — never emits platform_mrr.
func TestAdminStatsPayloadReportsGMVWithoutPlatformBilling(t *testing.T) {
	db := setupAdminStatsLedgerTestDB(t)
	now := time.Date(2026, time.May, 20, 12, 0, 0, 0, time.UTC)
	previousAdminStatsNow := adminStatsNow
	adminStatsNow = func() time.Time { return now }
	t.Cleanup(func() { adminStatsNow = previousAdminStatsNow })

	// Restaurant GMV: $15 net recognized across two businesses.
	paidAt := time.Date(2026, time.May, 10, 12, 0, 0, 0, time.UTC)
	biz := createAdminStatsLedgerBusiness(t, db, "GMV Biz")
	bill := createAdminStatsLedgerBill(t, db, biz.ID, "GMV-BILL", 1500, 1500, database.BillStatusPaid, paidAt)
	require.NoError(t, db.Create(&database.Payment{
		BillID: bill.ID, PayerAddr: "0xgmv", Amount: 1500, TipAmount: 0,
		TxHash: "gmv_tx", Status: database.PaymentStatusConfirmed, PaymentMethod: "crypto",
		ConfirmedAt: &paidAt, CreatedAt: paidAt, UpdatedAt: paidAt,
	}).Error)
	require.NoError(t, db.Create(&database.BusinessRevenueAggregate{
		BusinessID: biz.ID, NetRevenueCents: 1500, NetTipCents: 0,
		GrossRevenueCents: 1500, GrossTipCents: 0, PositiveEventCount: 1, RecognizedBillCount: 1,
	}).Error)

	stats := AdminStats{}
	require.NoError(t, getPaymentMetrics(&stats))

	// Struct-level: separate fields, correct values.
	assert.InDelta(t, 15.0, stats.GrossMerchandiseVolume, 0.01,
		"gross_merchandise_volume is restaurant net recognized takings")

	// Wire-level: JSON keys must match the public contract; total_revenue gone.
	raw, err := json.Marshal(stats)
	require.NoError(t, err)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &payload))

	_, hasTotalRevenue := payload["total_revenue"]
	assert.False(t, hasTotalRevenue, "total_revenue must not appear in admin stats payload")

	gmv, hasGMV := payload["gross_merchandise_volume"]
	require.True(t, hasGMV, "gross_merchandise_volume must be present")
	assert.InDelta(t, 15.0, gmv.(float64), 0.01)

	_, hasMRR := payload["platform_mrr"]
	assert.False(t, hasMRR, "platform_mrr must not appear: the platform bills nobody")
}

// TestAdminSurfacesShareBusinessCount guards SEAM 4 Task 12: dashboard stats,
// business registry list, email recipient list, and fiscal summary all resolve
// business scope through database.CountBusinessesForAdmin / ListBusinessesForAdmin
// so they report the same kind=real population.
func TestAdminSurfacesShareBusinessCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAdminStatsLedgerTestDB(t)

	now := time.Now()
	realOwner := database.User{Email: "real-owner@cafe.com", Name: "Real", Role: "user"}
	testOwner := database.User{Email: "fixture@payverge.test", Name: "Fixture", Role: "user"}
	localOwner := database.User{Email: "local@payverge.local", Name: "Local", Role: "user"}
	require.NoError(t, db.Create(&realOwner).Error)
	require.NoError(t, db.Create(&testOwner).Error)
	require.NoError(t, db.Create(&localOwner).Error)

	seed := []struct {
		id, name, email string
		kind            database.BusinessKind
		userID          *uint
		isDemo          bool
	}{
		{"surf-real-1", "Real One", "real1@cafe.com", database.BusinessKindReal, &realOwner.ID, false},
		{"surf-real-2", "Real Two", "real2@cafe.com", database.BusinessKindReal, &realOwner.ID, false},
		{"surf-demo", "Demo Seed", "demo@payverge.example", database.BusinessKindDemo, nil, true},
		{"surf-test", "CI Fixture", "ci@payverge.test", database.BusinessKindTest, &testOwner.ID, false},
		{"surf-local", "Local Fixture", "qa@payverge.local", database.BusinessKindTest, &localOwner.ID, false},
	}
	for _, s := range seed {
		require.NoError(t, db.Create(&database.Business{
			BusinessId: s.id, Name: s.name, Email: s.email, OwnerName: "Owner",
			UserID: s.userID, Kind: s.kind, IsDemo: s.isDemo,
			CreatedAt: now, UpdatedAt: now,
		}).Error)
	}

	// Canonical count via the single entry point.
	want, err := database.CountBusinessesForAdmin(database.AdminBusinessFilter{Kind: string(database.BusinessKindReal)})
	require.NoError(t, err)
	require.EqualValues(t, 2, want)

	// Surface 1: dashboard business metrics.
	var stats AdminStats
	require.NoError(t, getBusinessMetrics(&stats))
	assert.Equal(t, want, stats.TotalBusinesses, "dashboard TotalBusinesses must use CountBusinessesForAdmin(kind=real)")

	// Surface 2: registry list total (default excludes non-real).
	router := gin.New()
	router.GET("/admin/businesses", GetBusinessList)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/businesses", nil))
	require.Equal(t, http.StatusOK, w.Code)
	var listResp struct {
		Total int64 `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	assert.Equal(t, want, listResp.Total, "registry total must match CountBusinessesForAdmin(kind=real)")

	// Surface 3: email business list (only real, with emails).
	router.GET("/admin/emails/businesses", GetBusinessEmails)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/admin/emails/businesses", nil))
	require.Equal(t, http.StatusOK, w2.Code)
	var emailResp struct {
		Businesses []struct {
			Email string `json:"email"`
		} `json:"businesses"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &emailResp))
	assert.EqualValues(t, want, int64(len(emailResp.Businesses)),
		"email business list must only include kind=real businesses")
	for _, b := range emailResp.Businesses {
		assert.NotContains(t, b.Email, "@payverge.test")
		assert.NotContains(t, b.Email, "@payverge.local")
	}

	// Surface 4: fiscal summary scopes businesses_with_fiscal to kind=real.
	// Seed fiscal settings on one real + one demo; only real should count.
	// AutoMigrate fiscal tables so the summary path can group by status.
	require.NoError(t, db.AutoMigrate(
		&database.BusinessFiscalSettings{},
		&database.FiscalJob{},
		&database.FiscalReceipt{},
	))
	var realBiz, demoBiz database.Business
	require.NoError(t, db.Where("business_id = ?", "surf-real-1").First(&realBiz).Error)
	require.NoError(t, db.Where("business_id = ?", "surf-demo").First(&demoBiz).Error)
	require.NoError(t, db.Create(&database.BusinessFiscalSettings{
		BusinessID: realBiz.ID, Mode: database.FiscalModeManual, Country: "AR", Provider: "arca",
	}).Error)
	require.NoError(t, db.Create(&database.BusinessFiscalSettings{
		BusinessID: demoBiz.ID, Mode: database.FiscalModeManual, Country: "AR", Provider: "arca",
	}).Error)

	summary, err := database.GetAdminFiscalSummary()
	require.NoError(t, err)
	assert.EqualValues(t, 1, summary.BusinessesWithFiscal,
		"fiscal businesses_with_fiscal must exclude kind!=real")
}

// The ledger sums cents across venues without converting, so the stats must
// name the currency: one code when every venue with bills shares it, and an
// empty code (plus the list) when they differ.
func TestAdminPaymentMetricsNameTheVolumeCurrency(t *testing.T) {
	db := setupAdminStatsLedgerTestDB(t)
	at := time.Date(2026, time.May, 10, 12, 0, 0, 0, time.UTC)

	ars := createAdminStatsLedgerBusiness(t, db, "Peso venue")
	require.NoError(t, db.Model(&ars).Updates(map[string]any{"display_currency": "ARS", "default_currency": "ARS"}).Error)
	createAdminStatsLedgerBill(t, db, ars.ID, "ADMIN-CUR-ARS", 1000, 1000, database.BillStatusPaid, at)
	// A venue with no bills must not count.
	unused := createAdminStatsLedgerBusiness(t, db, "Euro venue without bills")
	require.NoError(t, db.Model(&unused).Update("display_currency", "EUR").Error)

	var stats AdminStats
	require.NoError(t, getPaymentMetrics(&stats))
	require.Equal(t, "ARS", stats.PaymentVolumeCurrency)
	require.Equal(t, []string{"ARS"}, stats.PaymentVolumeCurrencies)

	usd := createAdminStatsLedgerBusiness(t, db, "Dollar venue")
	createAdminStatsLedgerBill(t, db, usd.ID, "ADMIN-CUR-USD", 500, 500, database.BillStatusPaid, at)

	stats = AdminStats{}
	require.NoError(t, getPaymentMetrics(&stats))
	require.Equal(t, "", stats.PaymentVolumeCurrency)
	require.Equal(t, []string{"ARS", "USD"}, stats.PaymentVolumeCurrencies)
}
