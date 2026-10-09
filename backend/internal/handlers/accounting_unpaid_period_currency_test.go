package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// unpaidEnvelope is the full #900 wire shape: the rows plus the envelope
// aggregate, both of which are money and therefore both of which need a
// currency to be readable on a non-USD venue.
type unpaidEnvelope struct {
	Data struct {
		Bills             []UnpaidBillRow `json:"bills"`
		Currency          string          `json:"currency"`
		Total             int64           `json:"total"`
		CarriedOverAmount float64         `json:"carried_over_amount"`
	} `json:"data"`
}

func decodeUnpaidEnvelope(t *testing.T, body []byte) unpaidEnvelope {
	t.Helper()
	var resp unpaidEnvelope
	require.NoError(t, json.Unmarshal(body, &resp))
	return resp
}

// TestGetUnpaidBillsAcceptsPeriodPreset is the first half of #900: QA asked
// Outstanding for `period=week` — the preset every other money surface on the
// dashboard takes (summary sparklines, food cost, labor, analytics) — and got
// 400 "start and end query parameters are required". A period preset is not a
// missing range; the canonical resolver already knows how to turn one into a
// window.
func TestGetUnpaidBillsAcceptsPeriodPreset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xPeriodPreset")

	// Monday-anchored ISO week-to-date: something from earlier today is in it,
	// something from three weeks ago is not.
	now := time.Now().UTC()
	inWeek := now.Add(-2 * time.Minute)
	longAgo := now.AddDate(0, 0, -21)
	recent := createUnpaidTestBill(t, business.ID, 1, 120_00, 20_00, database.BillStatusOpen, inWeek)
	createUnpaidTestBill(t, business.ID, 2, 90_00, 0, database.BillStatusClosed, longAgo)

	router := unpaidBillsRouter(t, "0xPeriodPreset")
	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/unpaid-bills?period=week&page=1&page_size=50", business.ID), nil)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	resp := decodeUnpaidEnvelope(t, w.Body.Bytes())
	require.Len(t, resp.Data.Bills, 1, "period=week must scope to the ISO week, not replay the book")
	assert.Equal(t, recent.ID, resp.Data.Bills[0].ID)
	assert.InDelta(t, 90.0, resp.Data.CarriedOverAmount, 0.001,
		"the three-week-old leftover stays visible as carried_over")
}

// TestGetUnpaidBillsExplicitRangeStillWinsOverPeriod pins the precedence the
// canonical resolver documents: an explicit start+end beats a preset even when
// both are supplied, so no existing caller changes behavior.
func TestGetUnpaidBillsExplicitRangeStillWinsOverPeriod(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xRangeWins")

	inMay := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	may := createUnpaidTestBill(t, business.ID, 1, 200_00, 0, database.BillStatusClosed, inMay)

	router := unpaidBillsRouter(t, "0xRangeWins")
	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/unpaid-bills?period=today&start=2026-05-01&end=2026-05-31", business.ID), nil)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	resp := decodeUnpaidEnvelope(t, w.Body.Bytes())
	require.Len(t, resp.Data.Bills, 1)
	assert.Equal(t, may.ID, resp.Data.Bills[0].ID)
}

// TestGetUnpaidBillsRejectsUnknownPeriodAndBareRequest keeps the 400 where it
// belongs: no window at all, a half-typed range, and a period nobody supports
// must still be errors rather than a silent default window.
func TestGetUnpaidBillsRejectsUnknownPeriodAndBareRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xBadWindow")

	router := unpaidBillsRouter(t, "0xBadWindow")
	for _, query := range []string{"", "period=fortnight", "start=2026-05-01", "end=2026-05-31", "start=2026-05-31&end=2026-05-01"} {
		w := performAccountingRequest(t, router, http.MethodGet, fmt.Sprintf(
			"/inside/businesses/%d/accounting/unpaid-bills?%s", business.ID, query), nil)
		require.Equalf(t, http.StatusBadRequest, w.Code, "query %q: %s", query, w.Body.String())
	}
}

// TestGetUnpaidBillsRowsCarryCurrencyAndTableID is the second half of #900.
// Every number in this payload is money, and nothing in it says in which
// currency — so an ARS venue's outstanding book reads as dollars. The Outstanding
// tab even formats these rows with a currency borrowed from a *different*
// endpoint's response (summary.currency, defaulting to USD), so a row that
// arrives before or without a summary is printed in the wrong unit. table_id is
// the other omission: the join for the label is already there, the id is not
// projected, so a row cannot be linked back to its table.
func TestGetUnpaidBillsRowsCarryCurrencyAndTableID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xArsVenue")
	require.NoError(t, database.GetDB().Model(business).
		Updates(map[string]any{"display_currency": "ARS", "default_currency": "ARS"}).Error)

	table := &database.Table{BusinessID: business.ID, Name: "Mesa 4", TableCode: "mesa-4"}
	require.NoError(t, database.GetDB().Create(table).Error)

	at := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	bill := createUnpaidTestBill(t, business.ID, 1, 1702_00, 0, database.BillStatusOpen, at)
	require.NoError(t, database.GetDB().Model(bill).UpdateColumn("table_id", table.ID).Error)
	// Pre-range debt so the envelope aggregate is non-zero and needs a unit too.
	createUnpaidTestBill(t, business.ID, 2, 500_00, 0, database.BillStatusClosed, at.AddDate(0, 0, -40))

	router := unpaidBillsRouter(t, "0xArsVenue")
	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/unpaid-bills?start=2026-05-01&end=2026-05-31", business.ID), nil)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	resp := decodeUnpaidEnvelope(t, w.Body.Bytes())
	require.Len(t, resp.Data.Bills, 1)

	assert.Equal(t, "ARS", resp.Data.Currency,
		"carried_over_amount is money on the envelope and needs a unit")
	assert.Equal(t, "ARS", resp.Data.Bills[0].Currency,
		"a row must name its own currency instead of borrowing summary.currency")
	require.NotNil(t, resp.Data.Bills[0].TableID)
	assert.Equal(t, table.ID, *resp.Data.Bills[0].TableID)
	assert.Equal(t, "Mesa 4", resp.Data.Bills[0].TableLabel)
}

// TestGetUnpaidBillsCurrencyFallsBackToUSD keeps the resolution order identical
// to every other money surface: display, then default, then USD.
func TestGetUnpaidBillsCurrencyFallsBackToUSD(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xNoCurrency")
	require.NoError(t, database.GetDB().Model(business).
		Updates(map[string]any{"display_currency": "", "default_currency": ""}).Error)

	at := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	createUnpaidTestBill(t, business.ID, 1, 100_00, 0, database.BillStatusOpen, at)

	router := unpaidBillsRouter(t, "0xNoCurrency")
	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/unpaid-bills?start=2026-05-01&end=2026-05-31", business.ID), nil)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	resp := decodeUnpaidEnvelope(t, w.Body.Bytes())
	assert.Equal(t, "USD", resp.Data.Currency)
	require.Len(t, resp.Data.Bills, 1)
	assert.Equal(t, "USD", resp.Data.Bills[0].Currency)
}

// TestGetUnpaidBillsCurrencyCostsNoExtraQuery is the access-shape gate for the
// new column: the handler already holds the business row, so naming the
// currency must not add a per-request lookup — and table_id must ride the join
// that is already there rather than a second one.
func TestGetUnpaidBillsCurrencyCostsNoExtraQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cap := &unpaidSQLCapture{Interface: logger.Default.LogMode(logger.Silent)}
	dsn := fmt.Sprintf("file:unpaid_currency_shape_%d?mode=memory&cache=shared", time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: cap})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	prev := database.GetDB()
	database.SetTestDB(gormDB)
	t.Cleanup(func() { database.SetTestDB(prev) })
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{}, &database.User{}, &database.Staff{},
		&database.StaffPermissionDeny{}, &database.Table{}, &database.Bill{},
	))
	server.InitializeRBAC(database.GetDBWrapper())

	business := createAccountingHandlerBusiness(t, "0xCurrencyShape")
	inWindow := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	for i := 1; i <= 5; i++ {
		createUnpaidTestBill(t, business.ID, i, int64(i)*100_00, 0, database.BillStatusClosed, inWindow)
	}

	router := unpaidBillsRouter(t, "0xCurrencyShape")
	cap.sqls = nil
	w := performAccountingRequest(t, router, http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/accounting/unpaid-bills?start=2026-05-01&end=2026-05-31&page=1&page_size=20", business.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	businessSelects, tableJoins := 0, 0
	for _, raw := range cap.sqls {
		low := strings.ToLower(raw)
		if strings.Contains(low, "from `businesses`") || strings.Contains(low, "from \"businesses\"") {
			businessSelects++
		}
		if strings.Contains(low, "join") && strings.Contains(low, "tables") {
			tableJoins++
		}
	}
	assert.LessOrEqualf(t, businessSelects, 1, "currency must reuse the loaded business row, got %d business selects: %v", businessSelects, cap.sqls)
	assert.LessOrEqualf(t, tableJoins, 1, "table_id must ride the existing label join, got %d joins: %v", tableJoins, cap.sqls)
}
