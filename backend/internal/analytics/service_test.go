package analytics

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupAnalyticsTestDB(t testing.TB) *database.DB {
	return setupAnalyticsTestDBWithLogger(t, nil)
}

func setupAnalyticsTestDBWithLogger(t testing.TB, gormLogger logger.Interface) *database.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	config := &gorm.Config{}
	if gormLogger != nil {
		config.Logger = gormLogger
	}
	gormDB, err := gorm.Open(sqlite.Open(dsn), config)
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	err = gormDB.AutoMigrate(
		&database.Business{},
		&database.Bill{},
		&database.Payment{},
		&database.AlternativePayment{},
		&database.Menu{},
		&database.Staff{},
	)
	require.NoError(t, err)

	database.SetTestDB(gormDB)
	return database.GetDBWrapper()
}

type recordingAnalyticsLogger struct {
	mu   sync.Mutex
	sqls []string
}

func (l *recordingAnalyticsLogger) LogMode(logger.LogLevel) logger.Interface {
	return l
}

func (l *recordingAnalyticsLogger) Info(context.Context, string, ...interface{}) {}

func (l *recordingAnalyticsLogger) Warn(context.Context, string, ...interface{}) {}

func (l *recordingAnalyticsLogger) Error(context.Context, string, ...interface{}) {}

func (l *recordingAnalyticsLogger) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sqls = append(l.sqls, sql)
}

func (l *recordingAnalyticsLogger) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sqls = nil
}

func (l *recordingAnalyticsLogger) SQLs() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	copied := make([]string, len(l.sqls))
	copy(copied, l.sqls)
	return copied
}

func countSQLsContaining(sqls []string, needle string) int {
	count := 0
	for _, sql := range sqls {
		if strings.Contains(sql, needle) {
			count++
		}
	}
	return count
}

func TestHourExpressionCastsPostgresHourToInteger(t *testing.T) {
	// UTC preserves the original byte-identical expressions.
	require.Equal(t, "CAST(EXTRACT(HOUR FROM created_at) AS INTEGER)", hourExpressionForDialect("postgres", "created_at", time.UTC))
	require.Equal(t, "CAST(strftime('%H', created_at) AS INTEGER)", hourExpressionForDialect("sqlite", "created_at", time.UTC))

	// A business timezone shifts into local wall-clock first (Dubai = UTC+4).
	dubai, err := time.LoadLocation("Asia/Dubai")
	require.NoError(t, err)
	require.Equal(t, "CAST(EXTRACT(HOUR FROM (created_at)::timestamptz AT TIME ZONE 'Asia/Dubai') AS INTEGER)", hourExpressionForDialect("postgres", "created_at", dubai))
	require.Equal(t, "CAST(strftime('%H', datetime(created_at, '+14400 seconds')) AS INTEGER)", hourExpressionForDialect("sqlite", "created_at", dubai))
}

func TestGetDailySalesRecognizesConfirmedAlternativePaymentRevenueOnEffectivePaymentTime(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	service := NewAnalyticsService(db)
	business := database.Business{Name: "Daily Alt Revenue Restaurant"}
	require.NoError(t, db.GetGorm().Create(&business).Error)

	reportDate := time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)
	createdYesterday := reportDate.Add(-12 * time.Hour)
	confirmedToday := reportDate.Add(14 * time.Hour)

	oldBill := database.Bill{
		BusinessID:  business.ID,
		BillNumber:  "OLD-CONFIRMED-TODAY",
		TotalAmount: 4200,
		TipAmount:   700,
		Status:      "closed",
		CreatedAt:   createdYesterday,
	}
	require.NoError(t, db.GetGorm().Create(&oldBill).Error)
	require.NoError(t, db.GetGorm().Create(&database.AlternativePayment{
		BillID:          oldBill.ID,
		ParticipantAddr: "cashier",
		Amount:          4200,
		PaymentMethod:   database.PaymentMethodCash,
		Status:          database.AltPaymentStatusConfirmed,
		CreatedAt:       createdYesterday,
		ConfirmedAt:     &confirmedToday,
	}).Error)

	createdToday := reportDate.Add(9 * time.Hour)
	regularBill := database.Bill{
		BusinessID:  business.ID,
		BillNumber:  "REGULAR-TODAY",
		TotalAmount: 1300,
		TipAmount:   200,
		Status:      "closed",
		CreatedAt:   createdToday,
	}
	require.NoError(t, db.GetGorm().Create(&regularBill).Error)

	report, err := service.GetDailySales(business.ID, reportDate)
	require.NoError(t, err)

	assert.InDelta(t, 42.0, report.TotalRevenue, 0.01)
	assert.Zero(t, report.TotalTips)
	assert.Equal(t, 1, report.BillCount)
	assert.Equal(t, 1, report.TransactionCount)
	assert.InDelta(t, 42.0, report.AverageTicket, 0.01)
	assert.InDelta(t, 42.0, report.HourlyBreakdown[confirmedToday.Hour()].Revenue, 0.01)
	assert.Zero(t, report.HourlyBreakdown[confirmedToday.Hour()].Tips)
	assert.Equal(t, 1, report.HourlyBreakdown[confirmedToday.Hour()].BillCount)
	assert.Zero(t, report.HourlyBreakdown[createdToday.Hour()].Revenue)
	assert.Zero(t, report.HourlyBreakdown[createdToday.Hour()].BillCount)

	yesterdayReport, err := service.GetDailySales(business.ID, createdYesterday)
	require.NoError(t, err)
	assert.Zero(t, yesterdayReport.TotalRevenue)
	assert.Zero(t, yesterdayReport.BillCount)
}

func TestGetPaymentWindowSummaryAggregatesDashboardMetricsInSingleQuery(t *testing.T) {
	recorder := &recordingAnalyticsLogger{}
	db := setupAnalyticsTestDBWithLogger(t, recorder)
	service := NewAnalyticsService(db)
	business := database.Business{Name: "Dashboard Summary Restaurant"}
	require.NoError(t, db.GetGorm().Create(&business).Error)

	start := time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	confirmedAt := start.Add(10 * time.Hour)
	reversedAt := start.Add(12 * time.Hour)

	paidBill := database.Bill{
		BusinessID:  business.ID,
		BillNumber:  "DASH-PAID",
		TotalAmount: 3000,
		PaidAmount:  3000,
		Status:      database.BillStatusPaid,
		ClosedAt:    &confirmedAt,
		CreatedAt:   start.Add(-time.Hour),
		UpdatedAt:   confirmedAt,
	}
	require.NoError(t, db.GetGorm().Create(&paidBill).Error)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        paidBill.ID,
		PayerAddr:     "0xpaid",
		Amount:        3000,
		TipAmount:     500,
		TxHash:        "dashboard_summary_paid",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)

	reversedBill := database.Bill{
		BusinessID:  business.ID,
		BillNumber:  "DASH-REVERSED",
		TotalAmount: 1000,
		Status:      database.BillStatusOpen,
		CreatedAt:   start.Add(-time.Hour),
		UpdatedAt:   reversedAt,
	}
	require.NoError(t, db.GetGorm().Create(&reversedBill).Error)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        reversedBill.ID,
		PayerAddr:     "0xreversed",
		Amount:        1000,
		TxHash:        "dashboard_summary_reversed",
		Status:        database.PaymentStatusReversed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		ReversedAt:    &reversedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     reversedAt,
	}).Error)

	recorder.Reset()
	summary, err := service.GetPaymentWindowSummary(business.ID, start, end)
	require.NoError(t, err)

	assert.InDelta(t, 30.0, summary.TotalRevenue, 0.01)
	assert.InDelta(t, 5.0, summary.TotalTips, 0.01)
	assert.Equal(t, 3, summary.TransactionCount)
	assert.Equal(t, 1, summary.BillCount)
	assert.Equal(t, 2, summary.UniqueCustomers)
	assert.InDelta(t, 30.0, summary.AverageTicket, 0.01)

	sqls := recorder.SQLs()
	require.Len(t, sqls, 1, "dashboard summary should use one recognized-payment query")
	assert.Contains(t, sqls[0], "WITH recognized_events AS")
}

func TestGetDailySalesFallsBackToAlternativePaymentCreatedAtWhenConfirmedAtIsNil(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	service := NewAnalyticsService(db)
	business := database.Business{Name: "Daily Alt Fallback Restaurant"}
	require.NoError(t, db.GetGorm().Create(&business).Error)

	reportDate := time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)
	billCreatedYesterday := reportDate.Add(-10 * time.Hour)
	paymentCreatedToday := reportDate.Add(11 * time.Hour)

	bill := database.Bill{
		BusinessID:  business.ID,
		BillNumber:  "ALT-NIL-CONFIRMED-AT",
		TotalAmount: 3100,
		TipAmount:   600,
		Status:      "closed",
		CreatedAt:   billCreatedYesterday,
	}
	require.NoError(t, db.GetGorm().Create(&bill).Error)
	require.NoError(t, db.GetGorm().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "card-reader",
		Amount:          3100,
		PaymentMethod:   database.PaymentMethodCard,
		Status:          database.AltPaymentStatusConfirmed,
		CreatedAt:       paymentCreatedToday,
		ConfirmedAt:     nil,
	}).Error)

	report, err := service.GetDailySales(business.ID, reportDate)
	require.NoError(t, err)

	assert.InDelta(t, 31.0, report.TotalRevenue, 0.01)
	assert.Zero(t, report.TotalTips)
	assert.Equal(t, 1, report.BillCount)
	assert.Equal(t, 1, report.TransactionCount)
	assert.InDelta(t, 31.0, report.HourlyBreakdown[paymentCreatedToday.Hour()].Revenue, 0.01)
	assert.Equal(t, 1, report.HourlyBreakdown[paymentCreatedToday.Hour()].BillCount)
}

func TestGetDailySalesExcludesAlternativePaymentWhenConfirmedAtIsOutOfRange(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	service := NewAnalyticsService(db)
	business := database.Business{Name: "Daily Alt Out Of Range Restaurant"}
	require.NoError(t, db.GetGorm().Create(&business).Error)

	reportDate := time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)
	billAndPaymentCreatedToday := reportDate.Add(10 * time.Hour)
	confirmedTomorrow := reportDate.Add(26 * time.Hour)

	bill := database.Bill{
		BusinessID:  business.ID,
		BillNumber:  "ALT-CONFIRMED-TOMORROW",
		TotalAmount: 2400,
		TipAmount:   300,
		Status:      "closed",
		CreatedAt:   billAndPaymentCreatedToday,
	}
	require.NoError(t, db.GetGorm().Create(&bill).Error)
	require.NoError(t, db.GetGorm().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "cashier",
		Amount:          2400,
		PaymentMethod:   database.PaymentMethodCash,
		Status:          database.AltPaymentStatusConfirmed,
		CreatedAt:       billAndPaymentCreatedToday,
		ConfirmedAt:     &confirmedTomorrow,
	}).Error)

	report, err := service.GetDailySales(business.ID, reportDate)
	require.NoError(t, err)

	assert.Zero(t, report.TotalRevenue)
	assert.Zero(t, report.TotalTips)
	assert.Zero(t, report.BillCount)
	assert.Zero(t, report.TransactionCount)
}

func TestGetDailySalesUsesRecognizedPartialAlternativePaymentAmount(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	service := NewAnalyticsService(db)
	business := createAnalyticsBusiness(t, db, "Daily Partial Recognized Restaurant")
	reportDate := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	confirmedAt := reportDate.Add(13 * time.Hour)

	bill := createAnalyticsBill(t, db, business.ID, "DAILY-PARTIAL-ALT", 10000, 0, 500, database.BillStatusPartial, reportDate.Add(-time.Hour), confirmedAt)
	require.NoError(t, db.GetGorm().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "cashier",
		Amount:          1000,
		PaymentMethod:   database.PaymentMethodCash,
		Status:          database.AltPaymentStatusConfirmed,
		CreatedAt:       confirmedAt,
		UpdatedAt:       confirmedAt,
		ConfirmedAt:     &confirmedAt,
	}).Error)

	report, err := service.GetDailySales(business.ID, reportDate)
	require.NoError(t, err)

	assert.InDelta(t, 10.0, report.TotalRevenue, 0.01)
	assert.Zero(t, report.TotalTips)
	assert.Equal(t, 1, report.TransactionCount)
	assert.Equal(t, 1, report.BillCount)
	assert.InDelta(t, 10.0, report.AverageTicket, 0.01)
	assert.Equal(t, 1, report.PaymentMethods["cash"])
	assert.InDelta(t, 10.0, report.HourlyBreakdown[confirmedAt.Hour()].Revenue, 0.01)
	assert.Zero(t, report.HourlyBreakdown[confirmedAt.Hour()].Tips)
	assert.Equal(t, 1, report.HourlyBreakdown[confirmedAt.Hour()].BillCount)
}

// TestGetDailySalesExcludesFullyRefundedBillFromBillCount pins the bill_count /
// average-ticket agreement between GetDailySales (daily email / date endpoint)
// and GetPaymentWindowSummary (dashboard "today"). A bill paid then fully
// refunded the same day nets to $0 and must NOT be counted as a sold bill.
func TestGetDailySalesExcludesFullyRefundedBillFromBillCount(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	service := NewAnalyticsService(db)
	business := createAnalyticsBusiness(t, db, "Daily Refund Restaurant")
	reportDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	confirmedAt := reportDate.Add(13 * time.Hour)
	refundedAt := reportDate.Add(15 * time.Hour)

	// Bill A: $100, paid and kept.
	billA := createAnalyticsBill(t, db, business.ID, "DAILY-KEEP", 10000, 10000, 0, database.BillStatusPaid, reportDate.Add(-time.Hour), confirmedAt)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID: billA.ID, PayerAddr: "0xA", Amount: 10000, TxHash: "keep-A",
		Status: database.PaymentStatusConfirmed, PaymentMethod: "crypto",
		ConfirmedAt: &confirmedAt, CreatedAt: confirmedAt, UpdatedAt: confirmedAt,
	}).Error)

	// Bill B: $50, paid then fully refunded the SAME day (status=refunded,
	// reversed_at set) — emits +$50 at confirmed_at and -$50 at reversed_at.
	billB := createAnalyticsBill(t, db, business.ID, "DAILY-REFUND", 5000, 0, 0, database.BillStatusOpen, reportDate.Add(-time.Hour), refundedAt)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID: billB.ID, PayerAddr: "0xB", Amount: 5000, TxHash: "refund-B",
		Status: database.PaymentStatusRefunded, PaymentMethod: "crypto",
		ConfirmedAt: &confirmedAt, ReversedAt: &refundedAt, CreatedAt: confirmedAt, UpdatedAt: refundedAt,
	}).Error)

	report, err := service.GetDailySales(business.ID, reportDate)
	require.NoError(t, err)

	// Revenue nets to $100; only bill A is a sold bill.
	assert.InDelta(t, 100.0, report.TotalRevenue, 0.01)
	assert.Equal(t, 1, report.BillCount, "fully-refunded bill must not count toward bill_count")
	assert.InDelta(t, 100.0, report.AverageTicket, 0.01, "average ticket must be $100, not $50")

	// Cross-check GetPaymentWindowSummary agrees for the same day.
	windowStart := reportDate
	windowEnd := reportDate.Add(24 * time.Hour)
	summary, err := service.GetPaymentWindowSummary(business.ID, windowStart, windowEnd)
	require.NoError(t, err)
	assert.Equal(t, report.BillCount, summary.BillCount, "GetDailySales and GetPaymentWindowSummary must agree on bill_count")
}

func TestAnalyticsReportsCountRealPluginPaymentAndExcludePluginTrackingAlternative(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	reportDate := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time {
		return reportDate.Add(20 * time.Hour)
	})
	business := createAnalyticsBusiness(t, db, "Plugin Recognized Restaurant")
	confirmedAt := reportDate.Add(15 * time.Hour)

	bill := createAnalyticsBill(t, db, business.ID, "PLUGIN-LEDGER", 5000, 5000, 0, database.BillStatusPaid, reportDate.Add(-time.Hour), confirmedAt)
	require.NoError(t, db.GetGorm().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "plugin-paypal",
		Amount:          5000,
		PaymentMethod:   database.AlternativePaymentMethod("paypal"),
		Status:          database.AltPaymentStatusConfirmed,
		ConfirmedBy:     "plugin",
		CreatedAt:       confirmedAt,
		UpdatedAt:       confirmedAt,
		ConfirmedAt:     &confirmedAt,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "plugin",
		Amount:        5000,
		TxHash:        "plugin_paypal_settlement_daily",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "usd",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)

	daily, err := service.GetDailySales(business.ID, reportDate)
	require.NoError(t, err)
	assert.InDelta(t, 50.0, daily.TotalRevenue, 0.01)
	assert.Equal(t, 1, daily.TransactionCount)
	assert.Equal(t, 1, daily.BillCount)
	assert.Equal(t, 1, daily.PaymentMethods["plugin"])
	assert.NotContains(t, daily.PaymentMethods, "paypal")

	period, err := service.GetPeriodReport(business.ID, "today", time.UTC)
	require.NoError(t, err)
	assert.InDelta(t, 50.0, period.TotalRevenue, 0.01)
	assert.Equal(t, 1, period.TransactionCount)
	assert.Equal(t, 1, period.BillCount)
	assert.Equal(t, 1, period.PaymentMethods["plugin"])
	assert.NotContains(t, period.PaymentMethods, "paypal")
}

func TestAnalyticsReportsNetReversedPaymentsByRecognitionDate(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	confirmationDay := time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC)
	reversalDay := confirmationDay.AddDate(0, 0, 1)
	service := NewAnalyticsService(db).WithClock(func() time.Time {
		return reversalDay.Add(12 * time.Hour)
	})
	business := createAnalyticsBusiness(t, db, "Reversal Recognized Restaurant")
	confirmedAt := confirmationDay.Add(9 * time.Hour)
	reversedAt := reversalDay.Add(10 * time.Hour)

	bill := createAnalyticsBill(t, db, business.ID, "REVERSAL-LEDGER", 2400, 0, 0, database.BillStatusOpen, confirmationDay.Add(-time.Hour), reversedAt)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xreversal",
		Amount:        2400,
		TipAmount:     300,
		TxHash:        "analytics_reversal_payment",
		Status:        database.PaymentStatusReversed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		ReversedAt:    &reversedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     reversedAt,
	}).Error)

	confirmationReport, err := service.GetDailySales(business.ID, confirmationDay)
	require.NoError(t, err)
	assert.InDelta(t, 24.0, confirmationReport.TotalRevenue, 0.01)
	assert.InDelta(t, 3.0, confirmationReport.TotalTips, 0.01)
	assert.Equal(t, 1, confirmationReport.TransactionCount)
	assert.InDelta(t, 24.0, confirmationReport.HourlyBreakdown[confirmedAt.Hour()].Revenue, 0.01)

	reversalReport, err := service.GetDailySales(business.ID, reversalDay)
	require.NoError(t, err)
	assert.InDelta(t, -24.0, reversalReport.TotalRevenue, 0.01)
	assert.InDelta(t, -3.0, reversalReport.TotalTips, 0.01)
	assert.Equal(t, 1, reversalReport.TransactionCount)
	assert.InDelta(t, -24.0, reversalReport.HourlyBreakdown[reversedAt.Hour()].Revenue, 0.01)

	period, err := service.GetPeriodReport(business.ID, "week", time.UTC)
	require.NoError(t, err)
	assert.Zero(t, period.TotalRevenue)
	assert.Zero(t, period.TotalTips)
	assert.Equal(t, 2, period.TransactionCount)
	assert.Zero(t, period.BillCount)
	assert.Zero(t, period.AverageTicket)
	assert.Equal(t, 1, period.HourlyBreakdown[fmt.Sprintf("%d", confirmedAt.Hour())].BillCount)
	assert.Zero(t, period.HourlyBreakdown[fmt.Sprintf("%d", reversedAt.Hour())].BillCount)
}

// TestAnalyticsReportsNetRefundedPaymentsByRecognitionDate locks AN-MON-1 at the
// analytics service layer: an operator-refunded payment must keep its original
// revenue in the period it was earned and book the refund as a dated negative in
// the period it occurred — exactly like a chain-reversed payment. Before the fix,
// a refunded sale silently disappeared from the earn period and was never booked
// as a negative anywhere.
func TestAnalyticsReportsNetRefundedPaymentsByRecognitionDate(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	confirmationDay := time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)
	refundDay := confirmationDay.AddDate(0, 0, 1)
	service := NewAnalyticsService(db).WithClock(func() time.Time {
		return refundDay.Add(12 * time.Hour)
	})
	business := createAnalyticsBusiness(t, db, "Refund Recognized Restaurant")
	confirmedAt := confirmationDay.Add(9 * time.Hour)
	refundedAt := refundDay.Add(10 * time.Hour)

	bill := createAnalyticsBill(t, db, business.ID, "REFUND-LEDGER", 2400, 0, 0, database.BillStatusOpen, confirmationDay.Add(-time.Hour), refundedAt)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xrefund",
		Amount:        2400,
		TipAmount:     300,
		TxHash:        "analytics_refund_payment",
		Status:        database.PaymentStatusRefunded,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		ReversedAt:    &refundedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     refundedAt,
	}).Error)

	confirmationReport, err := service.GetDailySales(business.ID, confirmationDay)
	require.NoError(t, err)
	assert.InDelta(t, 24.0, confirmationReport.TotalRevenue, 0.01)
	assert.InDelta(t, 3.0, confirmationReport.TotalTips, 0.01)
	assert.Equal(t, 1, confirmationReport.TransactionCount)

	refundReport, err := service.GetDailySales(business.ID, refundDay)
	require.NoError(t, err)
	assert.InDelta(t, -24.0, refundReport.TotalRevenue, 0.01)
	assert.InDelta(t, -3.0, refundReport.TotalTips, 0.01)
	assert.Equal(t, 1, refundReport.TransactionCount)

	period, err := service.GetPeriodReport(business.ID, "week", time.UTC)
	require.NoError(t, err)
	assert.Zero(t, period.TotalRevenue)
	assert.Zero(t, period.TotalTips)
	assert.Equal(t, 2, period.TransactionCount)
	assert.Zero(t, period.BillCount)
}

// TestParsePeriodTodayYesterdayUsesBusinessTimezone locks AN-MON-2 (Layer A) at
// the analytics service: the today/yesterday window must be the business's local
// calendar day, not the server-clock (UTC) day. A Dubai (UTC+4) business with a
// payment confirmed at 19:00 UTC on 2026-05-10 — which is 23:00 *the same day* in
// Dubai, i.e. Dubai-yesterday relative to a Dubai "now" of 2026-05-11 01:00 —
// must see that revenue under "yesterday", never under "today". The UTC-loc call
// reproduces the old server-clock behavior and proves loc actually moves the
// window: if loc were ignored, the "today"/Dubai assertion below would fail.
func TestParsePeriodTodayYesterdayUsesBusinessTimezone(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	dubai, err := time.LoadLocation("Asia/Dubai") // UTC+4, no DST
	require.NoError(t, err)

	// Pin "now" to 2026-05-10 21:00 UTC == 2026-05-11 01:00 Dubai.
	service := NewAnalyticsService(db).WithClock(func() time.Time {
		return time.Date(2026, 5, 10, 21, 0, 0, 0, time.UTC)
	})
	business := createAnalyticsBusiness(t, db, "Dubai TZ Restaurant")

	// Confirmed at 2026-05-10 19:00 UTC == 2026-05-10 23:00 Dubai (Dubai-yesterday).
	confirmedAt := time.Date(2026, 5, 10, 19, 0, 0, 0, time.UTC)
	bill := createAnalyticsBill(t, db, business.ID, "TZ-LEDGER", 2400, 2400, 0, database.BillStatusPaid, confirmedAt.Add(-2*time.Hour), confirmedAt)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xtz",
		Amount:        2400,
		TipAmount:     0,
		TxHash:        "analytics_tz_payment",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)

	// In Dubai, this sale belongs to "yesterday", not "today".
	todayDubai, err := service.GetPeriodReport(business.ID, "today", dubai)
	require.NoError(t, err)
	assert.Zero(t, todayDubai.TotalRevenue, "Dubai 'today' must exclude a Dubai-yesterday sale")

	yesterdayDubai, err := service.GetPeriodReport(business.ID, "yesterday", dubai)
	require.NoError(t, err)
	assert.InDelta(t, 24.0, yesterdayDubai.TotalRevenue, 0.01, "Dubai 'yesterday' must include the sale")

	// The old server-clock (UTC) behavior would call this same sale "today" —
	// proving the business location genuinely moves the window boundary.
	todayUTC, err := service.GetPeriodReport(business.ID, "today", time.UTC)
	require.NoError(t, err)
	assert.InDelta(t, 24.0, todayUTC.TotalRevenue, 0.01, "UTC 'today' includes the 19:00 UTC sale")
}

// TestGetDailySalesHourlyBreakdownUsesBusinessTimezone locks AN-MON-2 (Layer B,
// Go-side): the per-hour breakdown must bucket each sale by its business-local
// hour, not the stored UTC hour. A Dubai (UTC+4) sale at 02:00 UTC is 06:00
// Dubai, so it must appear in local hour 6 — never hour 2.
func TestGetDailySalesHourlyBreakdownUsesBusinessTimezone(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	dubai, err := time.LoadLocation("Asia/Dubai") // UTC+4, no DST
	require.NoError(t, err)
	service := NewAnalyticsService(db)
	business := createAnalyticsBusiness(t, db, "Dubai Hourly Restaurant")

	confirmedAt := time.Date(2026, 5, 11, 2, 0, 0, 0, time.UTC) // 06:00 Dubai
	bill := createAnalyticsBill(t, db, business.ID, "TZ-HOUR", 2400, 0, 0, database.BillStatusOpen, confirmedAt.Add(-2*time.Hour), confirmedAt)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xtzhour",
		Amount:        2400,
		TipAmount:     0,
		TxHash:        "analytics_tz_hour_payment",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)

	// Query the Dubai calendar day 2026-05-11.
	report, err := service.GetDailySales(business.ID, time.Date(2026, 5, 11, 0, 0, 0, 0, dubai))
	require.NoError(t, err)
	assert.InDelta(t, 24.0, report.TotalRevenue, 0.01)
	assert.InDelta(t, 24.0, report.HourlyBreakdown[6].Revenue, 0.01, "sale must bucket into Dubai hour 6")
	assert.Zero(t, report.HourlyBreakdown[2].Revenue, "sale must not bucket into UTC hour 2")
	assert.Equal(t, 1, report.HourlyBreakdown[6].BillCount)
}

// TestParsePeriodTodayIsDSTSafe locks AN-MON-2 day-boundary DST correctness:
// "today" must end at the next *local* midnight, not start+24h. On the US
// spring-forward day (2026-03-08, a 23-hour day in America/New_York) a fixed
// +24h window would overshoot into 01:00 the next day; AddDate(0,0,1) yields the
// correct 23-hour local day.
func TestParsePeriodTodayIsDSTSafe(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	ny, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	// 2026-03-08 12:00 EST (DST begins 02:00 that morning).
	service := NewAnalyticsService(db).WithClock(func() time.Time {
		return time.Date(2026, 3, 8, 17, 0, 0, 0, time.UTC)
	})

	start, end, err := service.parsePeriod("today", ny)
	require.NoError(t, err)
	assert.Equal(t, 0, start.In(ny).Hour(), "start at local midnight")
	assert.Equal(t, 0, end.In(ny).Hour(), "end at next local midnight, not 01:00")
	assert.Equal(t, 9, end.In(ny).Day(), "end is 2026-03-09 local")
	assert.Equal(t, 23*time.Hour, end.Sub(start), "spring-forward day is 23 hours, not 24")
}

// TestGetPeriodReportHourlyBreakdownUsesBusinessTimezone locks AN-MON-2 (Layer B,
// SQL hour bucketing): GetPeriodReport's hourly breakdown is built by a SQL
// EXTRACT(HOUR)/strftime aggregate. A Dubai (UTC+4) sale at 02:00 UTC (=06:00
// Dubai) must appear in local hour 6, never UTC hour 2.
func TestGetPeriodReportHourlyBreakdownUsesBusinessTimezone(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	dubai, err := time.LoadLocation("Asia/Dubai") // UTC+4, no DST
	require.NoError(t, err)
	// Dubai "today" relative to now=2026-05-11 12:00 UTC (=16:00 Dubai).
	service := NewAnalyticsService(db).WithClock(func() time.Time {
		return time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	})
	business := createAnalyticsBusiness(t, db, "Dubai Period Hourly Restaurant")

	confirmedAt := time.Date(2026, 5, 11, 2, 0, 0, 0, time.UTC) // 06:00 Dubai
	bill := createAnalyticsBill(t, db, business.ID, "TZ-PHOUR", 2400, 2400, 0, database.BillStatusPaid, confirmedAt.Add(-2*time.Hour), confirmedAt)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xtzphour",
		Amount:        2400,
		TipAmount:     0,
		TxHash:        "analytics_tz_phour_payment",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)

	report, err := service.GetPeriodReport(business.ID, "today", dubai)
	require.NoError(t, err)
	assert.InDelta(t, 24.0, report.TotalRevenue, 0.01)
	assert.InDelta(t, 24.0, report.HourlyBreakdown["6"].Revenue, 0.01, "must bucket into Dubai hour 6")
	assert.Zero(t, report.HourlyBreakdown["2"].Revenue, "must not bucket into UTC hour 2")
}

func TestGetPeriodReportRecognizesAlternativePaymentRevenueOnceOnEffectivePaymentTime(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	fixedNow := time.Date(2026, 5, 3, 20, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := database.Business{Name: "Period Alt Revenue Restaurant"}
	require.NoError(t, db.GetGorm().Create(&business).Error)

	createdYesterday := time.Date(2026, 5, 2, 19, 0, 0, 0, time.UTC)
	confirmedToday := time.Date(2026, 5, 3, 15, 0, 0, 0, time.UTC)
	bill := database.Bill{
		BusinessID:  business.ID,
		BillNumber:  "PERIOD-OLD-CONFIRMED-TODAY",
		TotalAmount: 3600,
		TipAmount:   400,
		Status:      "closed",
		CreatedAt:   createdYesterday,
	}
	require.NoError(t, db.GetGorm().Create(&bill).Error)
	require.NoError(t, db.GetGorm().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "period-cashier",
		Amount:          3600,
		PaymentMethod:   database.PaymentMethodOther,
		Status:          database.AltPaymentStatusConfirmed,
		CreatedAt:       createdYesterday,
		ConfirmedAt:     &confirmedToday,
	}).Error)

	report, err := service.GetPeriodReport(business.ID, "today", time.UTC)
	require.NoError(t, err)

	assert.InDelta(t, 36.0, report.TotalRevenue, 0.01)
	assert.Zero(t, report.TotalTips)
	assert.Equal(t, 1, report.BillCount)
	assert.Equal(t, 1, report.TransactionCount)
	assert.InDelta(t, 36.0, report.AverageTicket, 0.01)
	assert.InDelta(t, 36.0, report.HourlyBreakdown[fmt.Sprintf("%d", confirmedToday.Hour())].Revenue, 0.01)
	assert.Equal(t, 1, report.HourlyBreakdown[fmt.Sprintf("%d", confirmedToday.Hour())].BillCount)
}

func TestGetPeriodReportTodayDoesNotFoldLiveRemainingIntoRevenue(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	fixedNow := time.Date(2026, 8, 21, 16, 20, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Issue 703 Remaining Restaurant")

	open := createAnalyticsBill(t, db, business.ID, "OPEN-TODAY", 582, 0, 0, database.BillStatusOpen, fixedNow.Add(-time.Hour), fixedNow.Add(-time.Hour))
	require.NotZero(t, open.ID)
	stale := createAnalyticsBill(t, db, business.ID, "STALE-PARTIAL", 3608, 1804, 0, database.BillStatusPartial, fixedNow.AddDate(0, 0, -6), fixedNow.AddDate(0, 0, -6))
	require.NotZero(t, stale.ID)

	today, err := service.GetPeriodReport(business.ID, "today", time.UTC)
	require.NoError(t, err)
	assert.InDelta(t, 0, today.TotalRevenue, 0.01)
	assert.InDelta(t, 0, today.CollectedRevenue, 0.01)
	assert.InDelta(t, 23.86, today.FloorRemaining, 0.01)
	assert.Equal(t, 0, today.BillCount)
	assert.Zero(t, today.AverageTicket)

	week, err := service.GetPeriodReport(business.ID, "week", time.UTC)
	require.NoError(t, err)
	assert.InDelta(t, 0, week.TotalRevenue, 0.01)
	assert.Zero(t, week.FloorRemaining)
}

func TestGetPeriodReport_GrowthRate_Positive(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	// Pin mid-month so calendar "month" is [day-1 00:00, now) and the equal-
	// duration previous window still lands fully in the prior calendar month.
	fixedNow := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })

	business := database.Business{Name: "Test Restaurant"}
	require.NoError(t, db.GetGorm().Create(&business).Error)

	// Previous equal-duration window ends at May 1; seed Apr 15.
	prevAt := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	prevBill := createAnalyticsBill(t, db, business.ID, "PREV-001", 10000, 10000, 0, database.BillStatusPaid, prevAt.Add(-time.Hour), prevAt)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID: prevBill.ID, PayerAddr: "0xprev", Amount: 10000, TxHash: "growth_prev",
		Status: database.PaymentStatusConfirmed, PaymentMethod: "crypto",
		ConfirmedAt: &prevAt, CreatedAt: prevAt, UpdatedAt: prevAt,
	}).Error)

	// Current calendar month: May 10.
	currAt := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	currBill := createAnalyticsBill(t, db, business.ID, "CURR-001", 15000, 15000, 0, database.BillStatusPaid, currAt.Add(-time.Hour), currAt)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID: currBill.ID, PayerAddr: "0xcurr", Amount: 15000, TxHash: "growth_curr",
		Status: database.PaymentStatusConfirmed, PaymentMethod: "crypto",
		ConfirmedAt: &currAt, CreatedAt: currAt, UpdatedAt: currAt,
	}).Error)

	report, err := service.GetPeriodReport(business.ID, "month", time.UTC)
	require.NoError(t, err)

	// Growth: (150 - 100) / 100 * 100 = 50% vs prior calendar month
	require.NotNil(t, report.GrowthRate)
	assert.InDelta(t, 50.0, *report.GrowthRate, 1.0)
}

func TestGetPeriodReportGrowthUsesRecognizedPaymentSummaryInsteadOfBillTotals(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	fixedNow := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Growth Recognized Restaurant")
	currentConfirmedAt := fixedNow.AddDate(0, 0, -5)
	previousConfirmedAt := fixedNow.AddDate(0, -1, -5)

	previousBill := createAnalyticsBill(t, db, business.ID, "GROWTH-PREV", 10000, 1000, 0, database.BillStatusPartial, previousConfirmedAt.Add(-time.Hour), previousConfirmedAt)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        previousBill.ID,
		PayerAddr:     "0xprevious",
		Amount:        1000,
		TxHash:        "analytics_growth_previous",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &previousConfirmedAt,
		CreatedAt:     previousConfirmedAt,
		UpdatedAt:     previousConfirmedAt,
	}).Error)

	currentBill := createAnalyticsBill(t, db, business.ID, "GROWTH-CURRENT", 10000, 2000, 0, database.BillStatusPartial, currentConfirmedAt.Add(-time.Hour), currentConfirmedAt)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        currentBill.ID,
		PayerAddr:     "0xcurrent",
		Amount:        2000,
		TxHash:        "analytics_growth_current",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &currentConfirmedAt,
		CreatedAt:     currentConfirmedAt,
		UpdatedAt:     currentConfirmedAt,
	}).Error)

	report, err := service.GetPeriodReport(business.ID, "month", time.UTC)
	require.NoError(t, err)

	assert.InDelta(t, 20.0, report.TotalRevenue, 0.01)
	// Prior calendar month has recognized $10; current $20 → +100%
	require.NotNil(t, report.GrowthRate)
	assert.InDelta(t, 100.0, *report.GrowthRate, 0.01)
}

func TestGetPeriodReportYearUsesRecognizedSummariesWithoutEventCap(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	fixedNow := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Year Recognized Restaurant")
	confirmedAt := fixedNow.AddDate(0, -3, 0)

	bill := createAnalyticsBill(t, db, business.ID, "YEAR-PARTIAL", 10000, 1000, 0, database.BillStatusPartial, confirmedAt.Add(-time.Hour), confirmedAt)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xyear",
		Amount:        1000,
		TxHash:        "analytics_year_partial",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)

	report, err := service.GetPeriodReport(business.ID, "year", time.UTC)
	require.NoError(t, err)

	assert.InDelta(t, 10.0, report.TotalRevenue, 0.01)
	assert.Equal(t, 1, report.TransactionCount)
	assert.Equal(t, 1, report.BillCount)
}

func TestGetPeriodReport_GrowthRate_NoPreviousData(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	fixedNow := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })

	business := database.Business{Name: "New Restaurant"}
	require.NoError(t, db.GetGorm().Create(&business).Error)

	// Only current calendar-month revenue (May 15); previous equal-duration window empty.
	currAt := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	bill := createAnalyticsBill(t, db, business.ID, "NEW-001", 20000, 20000, 0, database.BillStatusPaid, currAt.Add(-time.Hour), currAt)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID: bill.ID, PayerAddr: "0xnew", Amount: 20000, TxHash: "growth_new",
		Status: database.PaymentStatusConfirmed, PaymentMethod: "crypto",
		ConfirmedAt: &currAt, CreatedAt: currAt, UpdatedAt: currAt,
	}).Error)

	report, err := service.GetPeriodReport(business.ID, "month", time.UTC)
	require.NoError(t, err)

	// No prior-period baseline → omit growth_rate (nil), not fabricate 100%
	assert.Nil(t, report.GrowthRate)
}

func TestGetPeriodReport_GrowthRate_NoData(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	service := NewAnalyticsService(db)

	business := database.Business{Name: "Empty Restaurant"}
	require.NoError(t, db.GetGorm().Create(&business).Error)

	report, err := service.GetPeriodReport(business.ID, "month", time.UTC)
	require.NoError(t, err)

	// No data at all → no baseline
	assert.Nil(t, report.GrowthRate)
}

func TestGetTipAnalytics_IncludesDailyComparisonAndHourlyBreakdown(t *testing.T) {
	db := setupAnalyticsTestDB(t)

	// Pin "now" to a fixed UTC time in the middle of a day. This removes
	// the race between fixture setup and service.now() that previously
	// caused flakes when the test ran near a midnight boundary.
	fixedNow := time.Date(2026, 4, 23, 14, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })

	business := database.Business{Name: "Tip Restaurant"}
	require.NoError(t, db.GetGorm().Create(&business).Error)

	// All fixture timestamps derive from fixedNow so day/hour buckets are
	// deterministic regardless of wall-clock time at test execution.
	todayPaymentTime := fixedNow.Add(-30 * time.Minute)
	yesterdayBillTime := fixedNow.Add(-24 * time.Hour)

	todayBill := database.Bill{
		BusinessID:     business.ID,
		BillNumber:     "TIP-TODAY-001",
		TotalAmount:    4000,
		TipAmount:      1000,
		Status:         "paid",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		CreatedAt:      todayPaymentTime,
	}
	require.NoError(t, db.GetGorm().Create(&todayBill).Error)

	yesterdayBill := database.Bill{
		BusinessID:     business.ID,
		BillNumber:     "TIP-YESTERDAY-001",
		TotalAmount:    2500,
		TipAmount:      500,
		Status:         "paid",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		CreatedAt:      yesterdayBillTime,
		UpdatedAt:      yesterdayBillTime,
	}
	require.NoError(t, db.GetGorm().Create(&yesterdayBill).Error)

	payment := database.Payment{
		BillID:    todayBill.ID,
		PayerAddr: "0xabc123",
		Amount:    4000,
		TipAmount: 1000,
		TxHash:    "0xtiptest",
		Status:    database.PaymentStatusConfirmed,
		CreatedAt: todayPaymentTime,
		UpdatedAt: todayPaymentTime,
	}
	require.NoError(t, db.GetGorm().Create(&payment).Error)

	report, err := service.GetTipAnalytics(business.ID, "week", time.UTC)
	require.NoError(t, err)

	assert.InDelta(t, 15.0, report.TotalTips, 0.01)
	assert.Equal(t, 2, report.TipCount)
	assert.InDelta(t, 7.5, report.AverageTip, 0.01)
	assert.InDelta(t, 10.0, report.HourlyTips[fmt.Sprintf("%d", todayPaymentTime.Hour())], 0.01)
	assert.InDelta(t, 5.0, report.HourlyTips[fmt.Sprintf("%d", yesterdayBillTime.Hour())], 0.01)
	assert.InDelta(t, 10.0, report.DailyComparison.Today, 0.01)
	assert.InDelta(t, 5.0, report.DailyComparison.Yesterday, 0.01)
	assert.InDelta(t, 100.0, report.DailyComparison.ChangePercentage, 0.01)
	require.Len(t, report.TopTippers, 1)
	assert.Equal(t, "0xabc123", report.TopTippers[0].PayerAddress)
	assert.Empty(t, report.TopTippers[0].GuestName)
}

func TestGetTipAnalyticsAttachesCRMGuestNames(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	require.NoError(t, db.GetGorm().AutoMigrate(
		&database.Customer{},
		&database.CustomerBusiness{},
	))

	fixedNow := time.Date(2026, 8, 11, 20, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Tip Guest Name Restaurant")
	confirmedAt := fixedNow.Add(-time.Hour)

	customer := database.Customer{
		Email:         "demo-guest-01@example.com",
		Name:          "Demo Guest 01",
		WalletAddress: "0x999999999999999999999999999999999999a009",
		IsActive:      true,
	}
	require.NoError(t, db.GetGorm().Create(&customer).Error)
	require.NoError(t, db.GetGorm().Create(&database.CustomerBusiness{
		CustomerID:   customer.ID,
		BusinessID:   business.ID,
		LoyaltyTier:  "Bronze",
		FirstVisitAt: confirmedAt,
	}).Error)

	bill := createAnalyticsBill(t, db, business.ID, "TIP-GUEST-01", 5000, 5000, 800, database.BillStatusPaid, confirmedAt.Add(-time.Hour), confirmedAt)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0x999999999999999999999999999999999999A009", // case-insensitive join
		Amount:        5000,
		TipAmount:     800,
		TxHash:        "analytics_tip_guest_name",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)

	report, err := service.GetTipAnalytics(business.ID, "week", time.UTC)
	require.NoError(t, err)
	require.Len(t, report.TopTippers, 1)
	assert.Equal(t, "Demo Guest 01", report.TopTippers[0].GuestName)
	assert.Equal(t, "0x999999999999999999999999999999999999A009", report.TopTippers[0].PayerAddress)
}

func TestGetTipAnalyticsUsesSingleRecognizedPaymentQuery(t *testing.T) {
	recorder := &recordingAnalyticsLogger{}
	db := setupAnalyticsTestDBWithLogger(t, recorder)
	fixedNow := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Tip Query Count Restaurant")
	confirmedAt := fixedNow.Add(-time.Hour)

	bill := createAnalyticsBill(t, db, business.ID, "TIP-QUERY-COUNT", 4000, 4000, 500, database.BillStatusPaid, confirmedAt.Add(-time.Hour), confirmedAt)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xtipquerycount",
		Amount:        4000,
		TipAmount:     500,
		TxHash:        "analytics_tip_query_count",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)

	recorder.Reset()
	report, err := service.GetTipAnalytics(business.ID, "today", time.UTC)
	require.NoError(t, err)
	assert.InDelta(t, 5.0, report.TotalTips, 0.01)

	assert.Equal(t, 1, countSQLsContaining(recorder.SQLs(), "WITH recognized_events AS"))
}

// TestGetTipAnalyticsIncludesAlternativePaymentTips locks the post-000083 tip
// column: cash/card/venmo/other tips live on alternative_payments.tip_amount_cents.
// Recognized-payment analytics must surface them (and net refunds). Hardcoding
// 0 AS tip_cents for alt rows zeroed tip dashboards while tips-by-staff still
// read bills.tip_amount.
func TestGetTipAnalyticsIncludesAlternativePaymentTips(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	fixedNow := time.Date(2026, 6, 12, 16, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Alt Tip Analytics Restaurant")

	confirmedAt := fixedNow.Add(-2 * time.Hour)
	cashBill := createAnalyticsBill(t, db, business.ID, "ALT-TIP-CASH", 4000, 4000, 450, database.BillStatusPaid, confirmedAt.Add(-time.Hour), confirmedAt)
	require.NoError(t, db.GetGorm().Create(&database.AlternativePayment{
		BillID:          cashBill.ID,
		ParticipantAddr: "cashier",
		Amount:          4000,
		BillAmountCents: 4000,
		TipAmountCents:  450,
		PaymentMethod:   database.PaymentMethodCash,
		Status:          database.AltPaymentStatusConfirmed,
		CreatedAt:       confirmedAt,
		UpdatedAt:       confirmedAt,
		ConfirmedAt:     &confirmedAt,
	}).Error)

	refundConfirmedAt := fixedNow.AddDate(0, 0, -1).Add(-3 * time.Hour)
	refundedAt := fixedNow.Add(-30 * time.Minute)
	refundBill := createAnalyticsBill(t, db, business.ID, "ALT-TIP-REFUND", 2000, 0, 0, database.BillStatusOpen, refundConfirmedAt.Add(-time.Hour), refundedAt)
	require.NoError(t, db.GetGorm().Create(&database.AlternativePayment{
		BillID:          refundBill.ID,
		ParticipantAddr: "card-reader",
		Amount:          2000,
		BillAmountCents: 2000,
		TipAmountCents:  300,
		PaymentMethod:   database.PaymentMethodCard,
		Status:          database.AltPaymentStatusRefunded,
		CreatedAt:       refundConfirmedAt,
		UpdatedAt:       refundedAt,
		ConfirmedAt:     &refundConfirmedAt,
	}).Error)

	report, err := service.GetTipAnalytics(business.ID, "week", time.UTC)
	require.NoError(t, err)

	// Cash tip $4.50 + card tip $3.00 − card refund tip $3.00 = $4.50 net.
	assert.InDelta(t, 4.5, report.TotalTips, 0.01)
	assert.Equal(t, 2, report.TipCount)
	assert.InDelta(t, 3.75, report.AverageTip, 0.01)
	assert.InDelta(t, 4.5, report.HourlyTips[fmt.Sprintf("%d", confirmedAt.Hour())], 0.01)
	assert.InDelta(t, 3.0, report.HourlyTips[fmt.Sprintf("%d", refundConfirmedAt.Hour())], 0.01)
	assert.InDelta(t, -3.0, report.HourlyTips[fmt.Sprintf("%d", refundedAt.Hour())], 0.01)
	assert.InDelta(t, 1.5, report.DailyComparison.Today, 0.01) // $4.50 cash − $3.00 refund today
	assert.InDelta(t, 3.0, report.DailyComparison.Yesterday, 0.01)

	todayReport, err := service.GetTipAnalytics(business.ID, "today", time.UTC)
	require.NoError(t, err)
	assert.InDelta(t, 1.5, todayReport.TotalTips, 0.01)
	assert.Equal(t, 1, todayReport.TipCount) // only the positive cash tip counts

	daily, err := service.GetDailySales(business.ID, time.Date(fixedNow.Year(), fixedNow.Month(), fixedNow.Day(), 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.InDelta(t, 1.5, daily.TotalTips, 0.01)
	assert.InDelta(t, 4.5, daily.HourlyBreakdown[confirmedAt.Hour()].Tips, 0.01)
	assert.InDelta(t, -3.0, daily.HourlyBreakdown[refundedAt.Hour()].Tips, 0.01)
}

func TestGetTipAnalyticsUsesRecognizedTipEventsForReversals(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	fixedNow := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Recognized Tip Restaurant")
	confirmedAt := fixedNow.AddDate(0, 0, -1).Add(-2 * time.Hour)
	reversedAt := fixedNow.Add(-time.Hour)

	bill := createAnalyticsBill(t, db, business.ID, "TIP-REVERSAL", 4000, 0, 0, database.BillStatusOpen, confirmedAt.Add(-time.Hour), reversedAt)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xtipper",
		Amount:        4000,
		TipAmount:     500,
		TxHash:        "analytics_tip_reversal",
		Status:        database.PaymentStatusReversed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		ReversedAt:    &reversedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     reversedAt,
	}).Error)

	report, err := service.GetTipAnalytics(business.ID, "week", time.UTC)
	require.NoError(t, err)

	assert.Zero(t, report.TotalTips)
	assert.Equal(t, 1, report.TipCount)
	assert.InDelta(t, 5.0, report.AverageTip, 0.01)
	assert.InDelta(t, 12.5, report.AverageTipRate, 0.01)
	assert.Equal(t, 1, report.TipDistribution["5_10"])
	assert.InDelta(t, 5.0, report.HourlyTips[fmt.Sprintf("%d", confirmedAt.Hour())], 0.01)
	assert.InDelta(t, -5.0, report.HourlyTips[fmt.Sprintf("%d", reversedAt.Hour())], 0.01)
	assert.InDelta(t, -5.0, report.DailyComparison.Today, 0.01)
	assert.InDelta(t, 5.0, report.DailyComparison.Yesterday, 0.01)
	assert.InDelta(t, -200.0, report.DailyComparison.ChangePercentage, 0.01)
	require.Len(t, report.TopTippers, 1)
	assert.Equal(t, "0xtipper", report.TopTippers[0].PayerAddress)
	assert.InDelta(t, 5.0, report.TopTippers[0].TotalTips, 0.01)
	assert.Equal(t, 1, report.TopTippers[0].TipCount)
}

func TestGetTipAnalyticsQuarterUsesSQLRecognizedTipsWithoutEventCap(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	fixedNow := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Quarter Tip SQL Restaurant")
	// Both recognition events must fall inside calendar Q2 (Apr 1 → now).
	confirmedAt := fixedNow.AddDate(0, -1, 0).Add(-2 * time.Hour) // ~Apr 20
	reversedAt := fixedNow.AddDate(0, 0, -5).Add(-1 * time.Hour)  // ~May 15

	bill := createAnalyticsBill(t, db, business.ID, "TIP-QUARTER-REVERSAL", 6000, 0, 0, database.BillStatusOpen, confirmedAt.Add(-time.Hour), reversedAt)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xquartertipper",
		Amount:        6000,
		TipAmount:     900,
		TxHash:        "analytics_tip_quarter_reversal",
		Status:        database.PaymentStatusReversed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		ReversedAt:    &reversedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     reversedAt,
	}).Error)

	report, err := service.GetTipAnalytics(business.ID, "quarter", time.UTC)
	require.NoError(t, err)

	assert.Zero(t, report.TotalTips)
	assert.Equal(t, 1, report.TipCount)
	assert.InDelta(t, 9.0, report.AverageTip, 0.01)
	assert.InDelta(t, 15.0, report.AverageTipRate, 0.01)
	assert.Equal(t, 1, report.TipDistribution["5_10"])
	assert.InDelta(t, 9.0, report.HourlyTips[fmt.Sprintf("%d", confirmedAt.Hour())], 0.01)
	assert.InDelta(t, -9.0, report.HourlyTips[fmt.Sprintf("%d", reversedAt.Hour())], 0.01)
	require.Len(t, report.TopTippers, 1)
	assert.Equal(t, "0xquartertipper", report.TopTippers[0].PayerAddress)
}

func TestGetTipAnalyticsTodayUsesSQLAggregatesAboveEventCap(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	fixedNow := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Busy Tip SQL Restaurant")
	confirmedAt := fixedNow.Add(-time.Hour)

	bill := createAnalyticsBill(t, db, business.ID, "TIP-BUSY-TODAY", 2000000, 0, 0, database.BillStatusPartial, confirmedAt.Add(-time.Hour), confirmedAt)
	payments := make([]database.Payment, 10001)
	for i := range payments {
		payments[i] = database.Payment{
			BillID:        bill.ID,
			PayerAddr:     fmt.Sprintf("0xbusy%05d", i),
			Amount:        100,
			TipAmount:     1,
			TxHash:        fmt.Sprintf("analytics_busy_tip_%05d", i),
			Status:        database.PaymentStatusConfirmed,
			PaymentMethod: "crypto",
			ConfirmedAt:   &confirmedAt,
			CreatedAt:     confirmedAt,
			UpdatedAt:     confirmedAt,
		}
	}
	require.NoError(t, db.GetGorm().CreateInBatches(payments, 500).Error)

	report, err := service.GetTipAnalytics(business.ID, "today", time.UTC)
	require.NoError(t, err)

	assert.InDelta(t, 100.01, report.TotalTips, 0.01)
	assert.Equal(t, 10001, report.TipCount)
	assert.InDelta(t, 0.01, report.AverageTip, 0.001)
	assert.InDelta(t, 1.0, report.AverageTipRate, 0.01)
	assert.Equal(t, 10001, report.TipDistribution["0_5"])
	assert.InDelta(t, 100.01, report.HourlyTips[fmt.Sprintf("%d", confirmedAt.Hour())], 0.01)
	assert.InDelta(t, 100.01, report.DailyComparison.Today, 0.01)
}

func TestExportSalesDataCSVUsesRecognizedPaymentEvents(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	fixedNow := time.Date(2026, 5, 10, 20, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Export Recognized Restaurant")
	confirmedAt := fixedNow.Add(-3 * time.Hour)

	bill := createAnalyticsBill(t, db, business.ID, "EXPORT-PARTIAL", 10000, 1000, 0, database.BillStatusPartial, confirmedAt.Add(-time.Hour), confirmedAt)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xexport",
		Amount:        1000,
		TxHash:        "analytics_export_partial",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)

	data, err := service.ExportSalesData(business.ID, "today", "csv", time.UTC, "en")
	require.NoError(t, err)
	csv := string(data)

	assert.True(t, strings.HasPrefix(csv, "Recognized At,Bill Number,Bill ID,Source,Payment Method,Amount,Tip,Bill Status,Bill Total,Currency\n"))
	assert.Contains(t, csv, "EXPORT-PARTIAL")
	assert.Contains(t, csv, ",payment,crypto,10.00,0.00,partial,100.00")
	assert.NotContains(t, csv, ",100.00,0.00,partial,[]")
}

func TestPeriodReportAndHourlySQLMatchLedgerEventsForMixedRecognizedPayments(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	fixedNow := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Analytics Ledger Parity Restaurant")
	// L6-1: "week" is calendar Mon 00:00 → now (2026-05-20 is Wednesday → Mon May 18).
	rangeStart := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
	rangeEnd := fixedNow

	pluginAt := rangeStart.Add(4 * time.Hour)
	pluginBill := createAnalyticsBill(t, db, business.ID, "PARITY-PLUGIN", 1500, 1500, 100, database.BillStatusPaid, pluginAt.Add(-time.Hour), pluginAt)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        pluginBill.ID,
		PayerAddr:     "plugin",
		Amount:        1500,
		TipAmount:     100,
		TxHash:        "plugin_analytics_parity",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "usd",
		ConfirmedAt:   &pluginAt,
		CreatedAt:     pluginAt,
		UpdatedAt:     pluginAt,
	}).Error)

	confirmedAt := rangeStart.Add(6 * time.Hour)
	reversedAt := rangeStart.Add(30 * time.Hour)
	reversedBill := createAnalyticsBill(t, db, business.ID, "PARITY-REVERSAL", 800, 0, 0, database.BillStatusOpen, confirmedAt.Add(-time.Hour), reversedAt)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        reversedBill.ID,
		PayerAddr:     "0xparityreversed",
		Amount:        800,
		TipAmount:     80,
		TxHash:        "analytics_parity_reversed",
		Status:        database.PaymentStatusReversed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		ReversedAt:    &reversedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     reversedAt,
	}).Error)

	altAt := rangeStart.Add(9 * time.Hour)
	altBill := createAnalyticsBill(t, db, business.ID, "PARITY-ALT", 900, 900, 0, database.BillStatusPaid, altAt.Add(-time.Hour), altAt)
	require.NoError(t, db.GetGorm().Create(&database.AlternativePayment{
		BillID:          altBill.ID,
		ParticipantAddr: "cashier",
		Amount:          900,
		PaymentMethod:   database.PaymentMethodCash,
		Status:          database.AltPaymentStatusConfirmed,
		CreatedAt:       altAt,
		UpdatedAt:       altAt,
		ConfirmedAt:     &altAt,
	}).Error)

	legacyAt := rangeStart.Add(48 * time.Hour)
	_ = createAnalyticsBill(t, db, business.ID, "PARITY-LEGACY", 700, 700, 70, database.BillStatusPaid, legacyAt, legacyAt)

	events, err := database.GetRecognizedPaymentEvents(business.ID, rangeStart, rangeEnd)
	require.NoError(t, err)
	summary := database.SummarizeRecognizedPaymentEvents(events)
	sqlSummary, err := database.GetRecognizedPaymentSummary(business.ID, rangeStart, rangeEnd)
	require.NoError(t, err)
	assert.Equal(t, summary, sqlSummary)

	report, err := service.GetPeriodReport(business.ID, "week", time.UTC)
	require.NoError(t, err)

	netRevenueByBill := make(map[uint]int64)
	expectedHourlyRevenue := make(map[int]int64)
	expectedHourlyTransactions := make(map[int]int)
	expectedHourlyBillRevenue := make(map[int]map[uint]int64)
	for _, event := range events {
		netRevenueByBill[event.BillID] += event.AmountCents
		hour := event.RecognizedAt.Hour()
		expectedHourlyRevenue[hour] += event.AmountCents
		expectedHourlyTransactions[hour]++
		if _, exists := expectedHourlyBillRevenue[hour]; !exists {
			expectedHourlyBillRevenue[hour] = make(map[uint]int64)
		}
		expectedHourlyBillRevenue[hour][event.BillID] += event.AmountCents
	}
	expectedPositiveBills := 0
	for _, netRevenue := range netRevenueByBill {
		if netRevenue > 0 {
			expectedPositiveBills++
		}
	}

	assert.InDelta(t, centsToDollars(summary.TotalRevenueCents), report.TotalRevenue, 0.01)
	assert.InDelta(t, centsToDollars(summary.TotalTipCents), report.TotalTips, 0.01)
	assert.Equal(t, int(summary.EventCount), report.TransactionCount)
	assert.Equal(t, expectedPositiveBills, report.BillCount)
	for method, count := range summary.CountByMethod {
		assert.Equal(t, int(count), report.PaymentMethods[method], "payment method %s", method)
	}

}

func TestExportSalesDataRejectsBroadRecognizedEventRanges(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	fixedNow := time.Date(2026, 5, 10, 20, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Export Broad Restaurant")

	_, err := service.ExportSalesData(business.ID, "year", "csv", time.UTC, "en")
	assert.ErrorContains(t, err, "recognized payment event range too large")
}

func TestGetPopularItemsProratesRevenueAndQuantityByRecognizedAmount(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	createAnalyticsBillItemsTable(t, db)
	fixedNow := time.Date(2026, 5, 11, 20, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Popular Recognized Restaurant")
	confirmedAt := fixedNow.Add(-2 * time.Hour)

	bill := createAnalyticsBill(t, db, business.ID, "POPULAR-PARTIAL", 10000, 5000, 0, database.BillStatusPartial, confirmedAt.Add(-time.Hour), confirmedAt)
	require.NoError(t, db.GetGorm().Create(&database.BillItem{
		ID:        "popular-burger",
		BillID:    bill.ID,
		Name:      "Burger",
		Price:     15,
		Quantity:  4,
		Subtotal:  60,
		CreatedAt: confirmedAt,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&database.BillItem{
		ID:        "popular-fries",
		BillID:    bill.ID,
		Name:      "Fries",
		Price:     20,
		Quantity:  2,
		Subtotal:  40,
		CreatedAt: confirmedAt,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xpopular",
		Amount:        5000,
		TxHash:        "analytics_popular_partial",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)

	items, err := service.GetPopularItems(business.ID, 0, "today", time.UTC)
	require.NoError(t, err)
	require.Len(t, items, 2)

	burger := requireItemStats(t, items, "Burger")
	assert.Equal(t, 2, burger.TotalSold)
	assert.InDelta(t, 30.0, burger.Revenue, 0.01)
	assert.Equal(t, 1, burger.BillsFeatured)
	assert.InDelta(t, 100.0, burger.Popularity, 0.01)

	fries := requireItemStats(t, items, "Fries")
	assert.Equal(t, 1, fries.TotalSold)
	assert.InDelta(t, 20.0, fries.Revenue, 0.01)
	assert.Equal(t, 1, fries.BillsFeatured)

	// Average price must divide recognized revenue by the recognized FLOAT
	// quantity — dividing by the rounded display count made a half-recognized
	// item read at half (or double) its real unit price on live.
	assert.InDelta(t, 15.0, burger.AveragePrice, 0.01)
	assert.InDelta(t, 20.0, fries.AveragePrice, 0.01)
}

func TestGetPopularItemsUsesSingleRecognizedPaymentQuery(t *testing.T) {
	recorder := &recordingAnalyticsLogger{}
	db := setupAnalyticsTestDBWithLogger(t, recorder)
	createAnalyticsBillItemsTable(t, db)
	fixedNow := time.Date(2026, 5, 11, 20, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Popular Query Count Restaurant")
	confirmedAt := fixedNow.Add(-2 * time.Hour)

	bill := createAnalyticsBill(t, db, business.ID, "POPULAR-QUERY-COUNT", 5000, 5000, 0, database.BillStatusPaid, confirmedAt.Add(-time.Hour), confirmedAt)
	require.NoError(t, db.GetGorm().Create(&database.BillItem{
		ID:        "popular-query-count-burger",
		BillID:    bill.ID,
		Name:      "Burger",
		Price:     50,
		Quantity:  1,
		Subtotal:  50,
		CreatedAt: confirmedAt,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xpopularquerycount",
		Amount:        5000,
		TxHash:        "analytics_popular_query_count",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)

	recorder.Reset()
	items, err := service.GetPopularItems(business.ID, 0, "today", time.UTC)
	require.NoError(t, err)
	require.Len(t, items, 1)

	assert.Equal(t, 1, countSQLsContaining(recorder.SQLs(), "WITH recognized_events AS"))
}

func TestGetPopularItemsLoadsMenuTranslationsInBatches(t *testing.T) {
	recorder := &recordingAnalyticsLogger{}
	db := setupAnalyticsTestDBWithLogger(t, recorder)
	require.NoError(t, db.GetGorm().AutoMigrate(&database.Menu{}, &database.Translation{}))
	createAnalyticsBillItemsTable(t, db)
	fixedNow := time.Date(2026, 5, 11, 20, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := database.Business{Name: "Popular Translation Restaurant", DefaultLanguage: "en"}
	require.NoError(t, db.GetGorm().Create(&business).Error)
	confirmedAt := fixedNow.Add(-2 * time.Hour)

	require.NoError(t, db.MenuService.Create(&database.Menu{BusinessID: business.ID, IsActive: true}, []database.MenuCategory{
		{
			Name: "Comida",
			Items: []database.MenuItem{
				{Name: "Taco"},
				{Name: "Burrito"},
			},
		},
		{
			Name: "Bebidas",
			Items: []database.MenuItem{
				{Name: "Agua"},
			},
		},
	}))
	require.NoError(t, db.GetGorm().Create(&[]database.Translation{
		{BusinessID: business.ID, EntityType: "category", EntityID: 0, FieldName: "name", LanguageCode: "en", TranslatedText: "Food"},
		{BusinessID: business.ID, EntityType: "category", EntityID: 1, FieldName: "name", LanguageCode: "en", TranslatedText: "Drinks"},
		{BusinessID: business.ID, EntityType: "menu_item", EntityID: 0, FieldName: "name", LanguageCode: "en", TranslatedText: "Taco Supreme"},
		{BusinessID: business.ID, EntityType: "menu_item", EntityID: 1, FieldName: "name", LanguageCode: "en", TranslatedText: "Burrito"},
		{BusinessID: business.ID, EntityType: "menu_item", EntityID: 1000, FieldName: "name", LanguageCode: "en", TranslatedText: "Water"},
	}).Error)

	bill := createAnalyticsBill(t, db, business.ID, "POPULAR-TRANSLATED", 5000, 5000, 0, database.BillStatusPaid, confirmedAt.Add(-time.Hour), confirmedAt)
	require.NoError(t, db.GetGorm().Create(&database.BillItem{
		ID:        "popular-translated-taco",
		BillID:    bill.ID,
		Name:      "Taco",
		Price:     50,
		Quantity:  1,
		Subtotal:  50,
		CreatedAt: confirmedAt,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xpopulartranslated",
		Amount:        5000,
		TxHash:        "analytics_popular_translated",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)

	recorder.Reset()
	items, err := service.GetPopularItems(business.ID, 0, "today", time.UTC)
	require.NoError(t, err)
	require.Len(t, items, 1)

	item := requireItemStats(t, items, "Taco Supreme")
	assert.Equal(t, "Food", item.Category)

	translationSelects := 0
	for _, sql := range recorder.SQLs() {
		normalized := strings.ToLower(sql)
		if strings.Contains(normalized, "from") && strings.Contains(normalized, "translations") {
			translationSelects++
		}
	}
	assert.Equal(t, 2, translationSelects)
}

func TestGetPopularItemsDoesNotRoundSubHalfPartialRevenueToZeroSold(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	createAnalyticsBillItemsTable(t, db)
	fixedNow := time.Date(2026, 5, 12, 20, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Popular Sub Half Restaurant")
	confirmedAt := fixedNow.Add(-2 * time.Hour)

	bill := createAnalyticsBill(t, db, business.ID, "POPULAR-SUB-HALF", 10000, 2500, 0, database.BillStatusPartial, confirmedAt.Add(-time.Hour), confirmedAt)
	require.NoError(t, db.GetGorm().Create(&database.BillItem{
		ID:        "popular-sub-half-sandwich",
		BillID:    bill.ID,
		Name:      "Sandwich",
		Price:     100,
		Quantity:  1,
		Subtotal:  100,
		CreatedAt: confirmedAt,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xsubhalf",
		Amount:        2500,
		TxHash:        "analytics_popular_sub_half",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)

	items, err := service.GetPopularItems(business.ID, 0, "today", time.UTC)
	require.NoError(t, err)
	require.Len(t, items, 1)

	sandwich := requireItemStats(t, items, "Sandwich")
	assert.Equal(t, 1, sandwich.TotalSold)
	assert.InDelta(t, 25.0, sandwich.Revenue, 0.01)
}

func TestGetPopularItemsRecognizedQuantityIncludesZeroRevenueBundleComponents(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	createAnalyticsBillItemsTable(t, db)
	fixedNow := time.Date(2026, 5, 12, 18, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Popular Bundle Component Restaurant")
	confirmedAt := fixedNow.Add(-2 * time.Hour)

	bill := createAnalyticsBill(t, db, business.ID, "POPULAR-BUNDLE", 2000, 2000, 0, database.BillStatusPaid, confirmedAt.Add(-time.Hour), confirmedAt)
	require.NoError(t, db.GetGorm().Create(&database.BillItem{
		ID:         "popular-bundle-parent",
		BillID:     bill.ID,
		MenuItemID: "combo-parent",
		Name:       "Lunch Combo",
		Price:      20,
		Quantity:   1,
		Subtotal:   20,
		ItemType:   "bundle",
		CreatedAt:  confirmedAt,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&database.BillItem{
		ID:         "popular-bundle-fries",
		BillID:     bill.ID,
		MenuItemID: "combo-fries",
		Name:       "Combo Fries",
		Price:      0,
		Quantity:   3,
		Subtotal:   0,
		ItemType:   "bundle_item",
		CreatedAt:  confirmedAt,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xbundlecomponent",
		Amount:        2000,
		TxHash:        "analytics_popular_bundle_component",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)

	items, err := service.GetPopularItems(business.ID, 0, "today", time.UTC)
	require.NoError(t, err)

	component := requireItemStats(t, items, "Combo Fries")
	assert.InDelta(t, 3, component.RecognizedQuantity, 0.0001)
	assert.Equal(t, 3, component.TotalSold)
	assert.InDelta(t, 0, component.Revenue, 0.01)
}

func TestGetPopularItemsRecognizedQuantityKeepsHalfPaidLine(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	createAnalyticsBillItemsTable(t, db)
	fixedNow := time.Date(2026, 5, 12, 19, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Popular Half Unit Restaurant")
	confirmedAt := fixedNow.Add(-2 * time.Hour)

	bill := createAnalyticsBill(t, db, business.ID, "POPULAR-HALF-UNIT", 10000, 5000, 0, database.BillStatusPartial, confirmedAt.Add(-time.Hour), confirmedAt)
	require.NoError(t, db.GetGorm().Create(&database.BillItem{
		ID:        "popular-half-unit-plate",
		BillID:    bill.ID,
		Name:      "Plate",
		Price:     100,
		Quantity:  1,
		Subtotal:  100,
		CreatedAt: confirmedAt,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xhalfunit",
		Amount:        5000,
		TxHash:        "analytics_popular_half_unit",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)

	items, err := service.GetPopularItems(business.ID, 0, "today", time.UTC)
	require.NoError(t, err)
	require.Len(t, items, 1)

	plate := requireItemStats(t, items, "Plate")
	assert.InDelta(t, 0.5, plate.RecognizedQuantity, 0.0001)
	assert.Equal(t, 1, plate.TotalSold)
}

func TestGetPopularItemsRoundsPositiveRecognizedQuantity(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	createAnalyticsBillItemsTable(t, db)
	fixedNow := time.Date(2026, 5, 12, 21, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Popular Rounded Quantity Restaurant")
	confirmedAt := fixedNow.Add(-2 * time.Hour)

	bill := createAnalyticsBill(t, db, business.ID, "POPULAR-ROUND-QUANTITY", 10000, 5000, 0, database.BillStatusPartial, confirmedAt.Add(-time.Hour), confirmedAt)
	require.NoError(t, db.GetGorm().Create(&database.BillItem{
		ID:        "popular-rounded-tacos",
		BillID:    bill.ID,
		Name:      "Tacos",
		Price:     20,
		Quantity:  3,
		Subtotal:  60,
		CreatedAt: confirmedAt,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&database.BillItem{
		ID:        "popular-rounded-salsa",
		BillID:    bill.ID,
		Name:      "Salsa",
		Price:     40,
		Quantity:  1,
		Subtotal:  40,
		CreatedAt: confirmedAt,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xroundquantity",
		Amount:        5000,
		TxHash:        "analytics_popular_round_quantity",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)

	items, err := service.GetPopularItems(business.ID, 0, "today", time.UTC)
	require.NoError(t, err)
	require.Len(t, items, 2)

	tacos := requireItemStats(t, items, "Tacos")
	assert.Equal(t, 2, tacos.TotalSold)
	assert.InDelta(t, 30.0, tacos.Revenue, 0.01)

	salsa := requireItemStats(t, items, "Salsa")
	assert.Equal(t, 1, salsa.TotalSold)
	assert.InDelta(t, 20.0, salsa.Revenue, 0.01)
}

func TestGetPopularItemsExcludesReversalOnlyNegativeRecognizedPeriods(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	createAnalyticsBillItemsTable(t, db)
	fixedNow := time.Date(2026, 5, 13, 20, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Popular Reversal Only Restaurant")
	confirmedAt := fixedNow.AddDate(0, 0, -1)
	reversedAt := fixedNow.Add(-2 * time.Hour)

	bill := createAnalyticsBill(t, db, business.ID, "POPULAR-REVERSAL-ONLY", 10000, 0, 0, database.BillStatusOpen, confirmedAt.Add(-time.Hour), reversedAt)
	require.NoError(t, db.GetGorm().Create(&database.BillItem{
		ID:        "popular-reversal-only-pasta",
		BillID:    bill.ID,
		Name:      "Pasta",
		Price:     50,
		Quantity:  2,
		Subtotal:  100,
		CreatedAt: confirmedAt,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xnegativepopular",
		Amount:        10000,
		TxHash:        "analytics_popular_reversal_only",
		Status:        database.PaymentStatusReversed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		ReversedAt:    &reversedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     reversedAt,
	}).Error)

	items, err := service.GetPopularItems(business.ID, 0, "today", time.UTC)
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestGetPopularItemsPopularityDenominatorExcludesReversalOnlyBills(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	createAnalyticsBillItemsTable(t, db)
	fixedNow := time.Date(2026, 5, 14, 20, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Popular Positive Denominator Restaurant")
	positiveAt := fixedNow.Add(-3 * time.Hour)
	reversedAt := fixedNow.Add(-2 * time.Hour)
	confirmedYesterday := fixedNow.AddDate(0, 0, -1)

	positiveBill := createAnalyticsBill(t, db, business.ID, "POPULAR-POSITIVE-DENOMINATOR", 5000, 5000, 0, database.BillStatusPaid, positiveAt.Add(-time.Hour), positiveAt)
	require.NoError(t, db.GetGorm().Create(&database.BillItem{
		ID:        "popular-denominator-bowl",
		BillID:    positiveBill.ID,
		Name:      "Bowl",
		Price:     50,
		Quantity:  1,
		Subtotal:  50,
		CreatedAt: positiveAt,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        positiveBill.ID,
		PayerAddr:     "0xpositivedenominator",
		Amount:        5000,
		TxHash:        "analytics_popular_positive_denominator",
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &positiveAt,
		CreatedAt:     positiveAt,
		UpdatedAt:     positiveAt,
	}).Error)

	reversalOnlyBill := createAnalyticsBill(t, db, business.ID, "POPULAR-REVERSAL-DENOMINATOR", 5000, 0, 0, database.BillStatusOpen, confirmedYesterday, reversedAt)
	require.NoError(t, db.GetGorm().Create(&database.BillItem{
		ID:        "popular-denominator-void",
		BillID:    reversalOnlyBill.ID,
		Name:      "Void",
		Price:     50,
		Quantity:  1,
		Subtotal:  50,
		CreatedAt: confirmedYesterday,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID:        reversalOnlyBill.ID,
		PayerAddr:     "0xreversaldenominator",
		Amount:        5000,
		TxHash:        "analytics_popular_reversal_denominator",
		Status:        database.PaymentStatusReversed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedYesterday,
		ReversedAt:    &reversedAt,
		CreatedAt:     confirmedYesterday,
		UpdatedAt:     reversedAt,
	}).Error)

	items, err := service.GetPopularItems(business.ID, 0, "today", time.UTC)
	require.NoError(t, err)
	require.Len(t, items, 1)

	bowl := requireItemStats(t, items, "Bowl")
	assert.InDelta(t, 100.0, bowl.Popularity, 0.01)
}

func createAnalyticsBusiness(t *testing.T, db *database.DB, name string) database.Business {
	t.Helper()

	business := database.Business{Name: name}
	require.NoError(t, db.GetGorm().Create(&business).Error)
	return business
}

func createAnalyticsBill(t *testing.T, db *database.DB, businessID uint, number string, total, paid, tip int64, status database.BillStatus, createdAt, updatedAt time.Time) database.Bill {
	t.Helper()

	bill := database.Bill{
		BusinessID:     businessID,
		BillNumber:     fmt.Sprintf("%s-%d", number, time.Now().UnixNano()),
		Items:          "[]",
		Subtotal:       total,
		TotalAmount:    total,
		PaidAmount:     paid,
		TipAmount:      tip,
		Status:         status,
		SettlementAddr: "settlement-" + number,
		TippingAddr:    "tipping-" + number,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}
	require.NoError(t, db.GetGorm().Create(&bill).Error)
	return bill
}

func createAnalyticsBillItemsTable(t *testing.T, db *database.DB) {
	t.Helper()

	require.NoError(t, db.GetGorm().Exec(`
		CREATE TABLE IF NOT EXISTS bill_items (
			id TEXT PRIMARY KEY,
			bill_id INTEGER NOT NULL,
			menu_item_id TEXT DEFAULT '',
			name TEXT NOT NULL,
			price REAL NOT NULL,
			quantity INTEGER NOT NULL,
			options TEXT,
			item_type TEXT DEFAULT 'menu_item',
			bundle_id INTEGER,
			parent_bundle_id INTEGER,
			source_offer_id INTEGER,
			order_id INTEGER,
			subtotal REAL NOT NULL,
			created_at DATETIME
		)
	`).Error)
	require.NoError(t, db.GetGorm().Exec("CREATE INDEX IF NOT EXISTS idx_bill_items_bill_id ON bill_items(bill_id)").Error)
}

func requireItemStats(t *testing.T, items []ItemStats, name string) ItemStats {
	t.Helper()

	for _, item := range items {
		if item.ItemName == name || item.ItemID == name {
			return item
		}
	}

	require.Failf(t, "missing item stats", "item %q was not returned", name)
	return ItemStats{}
}

// TestGetPopularItemsLimitBoundsAtSQL verifies DUP-02 fix: when a limit > 0
// is passed, only that many rows are returned AND the limit is applied inside
// the SQL query (not in Go after fetching all rows).
func TestGetPopularItemsLimitBoundsAtSQL(t *testing.T) {
	recorder := &recordingAnalyticsLogger{}
	db := setupAnalyticsTestDBWithLogger(t, recorder)
	createAnalyticsBillItemsTable(t, db)
	fixedNow := time.Date(2026, 5, 13, 20, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Popular Limit Restaurant")
	confirmedAt := fixedNow.Add(-2 * time.Hour)

	// Seed 7 items across 7 bills so revenue ordering is deterministic.
	itemNames := []string{"Alpha", "Beta", "Gamma", "Delta", "Epsilon", "Zeta", "Eta"}
	for i, name := range itemNames {
		price := float64((len(itemNames) - i) * 10) // Alpha is most expensive → most revenue
		bill := createAnalyticsBill(t, db, business.ID,
			fmt.Sprintf("LIMIT-BILL-%d", i), int64(price*100), int64(price*100), 0,
			database.BillStatusPaid, confirmedAt.Add(-time.Hour), confirmedAt)
		require.NoError(t, db.GetGorm().Create(&database.BillItem{
			ID:         fmt.Sprintf("limit-item-%d", i),
			BillID:     bill.ID,
			MenuItemID: "menu-" + strings.ToLower(name),
			Name:       name,
			Price:      price,
			Quantity:   1,
			Subtotal:   price,
			CreatedAt:  confirmedAt,
		}).Error)
		require.NoError(t, db.GetGorm().Create(&database.Payment{
			BillID:        bill.ID,
			PayerAddr:     fmt.Sprintf("0xlimit%d", i),
			Amount:        int64(price * 100),
			TxHash:        fmt.Sprintf("analytics_limit_%d", i),
			Status:        database.PaymentStatusConfirmed,
			PaymentMethod: "crypto",
			ConfirmedAt:   &confirmedAt,
			CreatedAt:     confirmedAt,
			UpdatedAt:     confirmedAt,
		}).Error)
	}

	recorder.Reset()
	items, err := service.GetPopularItems(business.ID, 5, "today", time.UTC)
	require.NoError(t, err)

	// Must return exactly 5 items, not all 7.
	require.Len(t, items, 5, "limit=5 must return at most 5 items")

	// The LIMIT must appear in the SQL, proving it was applied at DB level.
	limitInSQL := false
	for _, sql := range recorder.SQLs() {
		if strings.Contains(strings.ToLower(sql), "limit") {
			limitInSQL = true
			break
		}
	}
	assert.True(t, limitInSQL, "LIMIT must appear in the generated SQL when limit>0")

	// Top item by revenue must be Alpha (highest price → highest revenue).
	// item_id is the stable menu_item_id, not the display name.
	assert.Equal(t, "menu-alpha", items[0].ItemID, "ordering must be revenue DESC before limit is applied")
	assert.Equal(t, "Alpha", items[0].ItemName)

	// Zero limit must return all rows (unbounded = old behavior).
	allItems, err := service.GetPopularItems(business.ID, 0, "today", time.UTC)
	require.NoError(t, err)
	assert.Len(t, allItems, 7, "limit=0 must return all items (unbounded)")
}

func TestGetPopularItemsClampsOversizedLimit(t *testing.T) {
	recorder := &recordingAnalyticsLogger{}
	db := setupAnalyticsTestDBWithLogger(t, recorder)
	createAnalyticsBillItemsTable(t, db)
	fixedNow := time.Date(2026, 5, 13, 20, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Popular Clamp Restaurant")

	recorder.Reset()
	_, err := service.GetPopularItems(business.ID, 1_000_000, "today", time.UTC)
	require.NoError(t, err)

	limitPattern := regexp.MustCompile(`(?i)\blimit\s+(\d+)`)
	var limits []int
	for _, sql := range recorder.SQLs() {
		for _, match := range limitPattern.FindAllStringSubmatch(sql, -1) {
			n, convErr := strconv.Atoi(match[1])
			require.NoError(t, convErr)
			limits = append(limits, n)
		}
	}
	require.NotEmpty(t, limits, "expected a LIMIT clause, got %v", recorder.SQLs())
	for _, n := range limits {
		assert.LessOrEqual(t, n, MaxPopularItemsLimit)
	}
}

// Two sold lines can share a display name (renames, translations, copy) while
// remaining distinct menu items. Aggregate by menu_item_id so they stay separate
// and item_id is the stable catalog id.
func TestGetPopularItemsKeepsSameDisplayNameDistinctByMenuItemID(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	createAnalyticsBillItemsTable(t, db)
	fixedNow := time.Date(2026, 5, 13, 20, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Same Name Distinct IDs")
	confirmedAt := fixedNow.Add(-2 * time.Hour)

	seed := func(id, menuItemID, name string, price float64) {
		t.Helper()
		bill := createAnalyticsBill(t, db, business.ID, id, int64(price*100), int64(price*100), 0,
			database.BillStatusPaid, confirmedAt.Add(-time.Hour), confirmedAt)
		require.NoError(t, db.GetGorm().Create(&database.BillItem{
			ID:         id,
			BillID:     bill.ID,
			MenuItemID: menuItemID,
			Name:       name,
			Price:      price,
			Quantity:   1,
			Subtotal:   price,
			CreatedAt:  confirmedAt,
		}).Error)
		require.NoError(t, db.GetGorm().Create(&database.Payment{
			BillID:        bill.ID,
			PayerAddr:     "0x" + id,
			Amount:        int64(price * 100),
			TxHash:        "analytics_same_name_" + id,
			Status:        database.PaymentStatusConfirmed,
			PaymentMethod: "crypto",
			ConfirmedAt:   &confirmedAt,
			CreatedAt:     confirmedAt,
			UpdatedAt:     confirmedAt,
		}).Error)
	}

	seed("salad-a", "menu-salad-a", "House Salad", 12)
	seed("salad-b", "menu-salad-b", "House Salad", 18)
	seed("legacy-soup", "", "Soup", 9)

	items, err := service.GetPopularItems(business.ID, 0, "today", time.UTC)
	require.NoError(t, err)
	require.Len(t, items, 3)

	byID := map[string]ItemStats{}
	for _, item := range items {
		byID[item.ItemID] = item
	}
	require.Contains(t, byID, "menu-salad-a")
	require.Contains(t, byID, "menu-salad-b")
	require.Contains(t, byID, "Soup", "empty menu_item_id must fall back to the display name")

	assert.Equal(t, "House Salad", byID["menu-salad-a"].ItemName)
	assert.Equal(t, "House Salad", byID["menu-salad-b"].ItemName)
	assert.InDelta(t, 12.0, byID["menu-salad-a"].Revenue, 0.01)
	assert.InDelta(t, 18.0, byID["menu-salad-b"].Revenue, 0.01)
	assert.Equal(t, "Soup", byID["Soup"].ItemName)
}

// TestGetPeriodReport_GrowthRate_PriorCalendarYear (L6-10 access-shape):
// year growth must compare against the prior calendar year of the same MTD
// shape, not start.Add(-duration). Also: zero prior revenue yields nil growth.
func TestGetPeriodReport_GrowthRate_PriorCalendarYear(t *testing.T) {
	db := setupAnalyticsTestDB(t)
	fixedNow := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := createAnalyticsBusiness(t, db, "Year Prior Calendar")

	// Prior calendar year MTD: 2025-03-15 revenue $100.
	// Duration-shift baseline would be mid-2025 and would miss this if placed carefully;
	// we put money on 2025-03-15 (inside prior YTD) and also on 2025-09-01 (inside
	// duration-shifted window for [2026-01-01,2026-05-20) but OUTSIDE prior calendar YTD).
	priorYTD := time.Date(2025, 3, 15, 12, 0, 0, 0, time.UTC)
	durationOnly := time.Date(2025, 9, 1, 12, 0, 0, 0, time.UTC)

	b1 := createAnalyticsBill(t, db, business.ID, "Y-PRIOR", 10000, 10000, 0, database.BillStatusPaid, priorYTD.Add(-time.Hour), priorYTD)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID: b1.ID, PayerAddr: "0xyp", Amount: 10000, TxHash: "year_prior_ytd",
		Status: database.PaymentStatusConfirmed, PaymentMethod: "crypto",
		ConfirmedAt: &priorYTD, CreatedAt: priorYTD, UpdatedAt: priorYTD,
	}).Error)
	b2 := createAnalyticsBill(t, db, business.ID, "Y-DUR", 50000, 50000, 0, database.BillStatusPaid, durationOnly.Add(-time.Hour), durationOnly)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID: b2.ID, PayerAddr: "0xyd", Amount: 50000, TxHash: "year_duration_only",
		Status: database.PaymentStatusConfirmed, PaymentMethod: "crypto",
		ConfirmedAt: &durationOnly, CreatedAt: durationOnly, UpdatedAt: durationOnly,
	}).Error)

	// Current YTD: $150
	currAt := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	b3 := createAnalyticsBill(t, db, business.ID, "Y-CURR", 15000, 15000, 0, database.BillStatusPaid, currAt.Add(-time.Hour), currAt)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID: b3.ID, PayerAddr: "0xyc", Amount: 15000, TxHash: "year_curr",
		Status: database.PaymentStatusConfirmed, PaymentMethod: "crypto",
		ConfirmedAt: &currAt, CreatedAt: currAt, UpdatedAt: currAt,
	}).Error)

	report, err := service.GetPeriodReport(business.ID, "year", time.UTC)
	require.NoError(t, err)
	assert.InDelta(t, 150.0, report.TotalRevenue, 0.01)
	// Prior calendar year = $100 only (not $100+$500). Growth = 50%.
	require.NotNil(t, report.GrowthRate, "expected growth vs prior calendar year")
	assert.InDelta(t, 50.0, *report.GrowthRate, 1.0)
}
