package director_tools

import (
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newTestDB opens a fresh in-memory SQLite DB, registers it as the
// package-level database connection (so package-level helpers like
// database.GetBusinessByID work), AutoMigrates the models the
// director_tools tests touch, and returns a *database.DB wrapper.
//
// Each test gets a unique DSN ("file:<test-name>?mode=memory&cache=shared")
// so parallel suites don't trample each other.
func newTestDB(t *testing.T) *database.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Bill{},
		&database.Payment{},
		&database.AlternativePayment{},
		&database.Order{},
		&database.DirectorConsoleThread{},
		&database.DirectorConsoleMessage{},
		&database.DirectorToolCall{},
		&database.CustomerBusiness{},
		&database.TableReservation{},
		&database.AiWaiterConversation{},
		&database.AiWaiterMessage{},
		&database.Plugin{},
		&database.BusinessPlugin{},
		&database.Menu{},
		&database.DirectorProposedAction{},
		&database.InventoryItem{},
		&database.InventoryRecipe{},
	))

	// bill_items uses a PostgreSQL UUID primary key that GORM AutoMigrate cannot
	// apply to SQLite, so create it via raw SQL (matching analytics/service_test.go's
	// createAnalyticsBillItemsTable pattern). Without this table, AnalyticsService
	// queries fail even when the table is empty and no data is seeded.
	require.NoError(t, gormDB.Exec(`
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
	require.NoError(t, gormDB.Exec("CREATE INDEX IF NOT EXISTS idx_bill_items_bill_id ON bill_items(bill_id)").Error)

	// Register as the package-level db so database.GetBusinessByID and
	// friends (which key off the global) work in tests.
	database.SetTestDB(gormDB)
	return database.GetDBWrapper()
}

// createTestBusiness inserts a single Business row with sensible defaults
// (timezone "America/New_York", AiName "Sage")
// and returns its primary key.
func createTestBusiness(t *testing.T, db *database.DB, name string) uint {
	t.Helper()

	business := database.Business{
		Name:     name,
		Timezone: "America/New_York",
		AiSettings: database.BusinessAiSettings{
			AiName: "Sage",
		},
	}
	require.NoError(t, db.GetGorm().Create(&business).Error)
	return business.ID
}

// newTestAnalytics constructs a real *analytics.AnalyticsService bound to
// the in-memory test DB. The revenue tool keeps a RevenueProvider interface
// seam so tests can swap in a stub, but suites that exercise the full
// integration path use this helper to construct the live service.
func newTestAnalytics(_ *testing.T, db *database.DB) *analytics.AnalyticsService {
	return analytics.NewAnalyticsService(db)
}

// stubRevenueProvider returns canned PaymentWindowSummary values keyed by
// period — handy for testing the revenue tool's formatting/validation
// logic without seeding the (substantial) ledger fixtures the real
// AnalyticsService SQL pipeline requires.
//
// window/windowCalls support the compare_to_prior seam: set window to a
// non-nil summary to simulate a successful prior-window fetch; windowCalls
// counts how many times GetPaymentWindowSummary was invoked.
type stubRevenueProvider struct {
	byPeriod      map[string]*analytics.PaymentWindowSummary
	err           error
	window        *analytics.PaymentWindowSummary
	windowCalls   int
	openRemaining float64
}

func (s *stubRevenueProvider) GetPaymentPeriodSummary(_ uint, period string, _ *time.Location) (*analytics.PaymentWindowSummary, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.byPeriod == nil {
		return nil, fmt.Errorf("stub: no fixture for period %q", period)
	}
	v, ok := s.byPeriod[period]
	if !ok {
		return nil, fmt.Errorf("stub: no fixture for period %q", period)
	}
	return v, nil
}

func (s *stubRevenueProvider) GetPaymentWindowSummary(_ uint, _, _ time.Time) (*analytics.PaymentWindowSummary, error) {
	s.windowCalls++
	if s.window == nil {
		return nil, fmt.Errorf("stub: no window summary")
	}
	return s.window, nil
}

func (s *stubRevenueProvider) LiveOpenCheckRemaining(_ uint) (float64, error) {
	return s.openRemaining, nil
}

// stubPopularItemsProvider returns canned ItemStats slices keyed by period —
// the analytics service's GetPopularItems path executes a chunky recognized-
// events CTE that is impractical to seed in unit tests. The seam lets us
// exercise the menu top/underperformer tools' shaping logic without that.
type stubPopularItemsProvider struct {
	byPeriod map[string][]analytics.ItemStats
	err      error
}

func (s *stubPopularItemsProvider) GetPopularItems(_ uint, limit int, period string, _ *time.Location) ([]analytics.ItemStats, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.byPeriod == nil {
		return nil, fmt.Errorf("stub: no fixture for period %q", period)
	}
	v, ok := s.byPeriod[period]
	if !ok {
		return nil, fmt.Errorf("stub: no fixture for period %q", period)
	}
	// Honour limit so the stub behaves consistently with the real service.
	if limit > 0 && len(v) > limit {
		return v[:limit], nil
	}
	return v, nil
}

// seedOrdersForFunnel inserts a Business plus the requested count of Order
// rows per status, all dated within the past 24 hours (so they fall inside
// any of the day/week/month windows under test). Returns the business ID.
//
// counts is keyed by OrderStatus string ("pending", "approved", etc.).
func seedOrdersForFunnel(t *testing.T, db *database.DB, counts map[string]int) uint {
	t.Helper()

	bizID := createTestBusiness(t, db, "Funnel Test Bistro")

	// One placeholder bill is enough — orders share BillID, and SQLite
	// treats nil ClientRequestID rows as distinct under the unique index.
	bill := database.Bill{
		BusinessID: bizID,
		BillNumber: "FUNNEL-BILL",
		Status:     database.BillStatusOpen,
		Items:      "[]",
	}
	require.NoError(t, db.GetGorm().Create(&bill).Error)

	i := 0
	for status, count := range counts {
		for j := 0; j < count; j++ {
			order := database.Order{
				BillID:      bill.ID,
				BusinessID:  bizID,
				OrderNumber: fmt.Sprintf("O-%s-%d", status, j),
				Status:      database.OrderStatus(status),
				CreatedBy:   "guest",
				Items:       "[]",
			}
			require.NoError(t, db.GetGorm().Create(&order).Error)
			i++
		}
	}
	return bizID
}
