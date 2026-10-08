package database

// Tests for P6.4 — assert that GetBillByID projects the Business preload to
// specific columns instead of issuing SELECT *.
//
// Consumer-field audit (every bill.Business.<Field> read in non-test code):
//
//   Field             Caller(s)
//   ──────────────    ──────────────────────────────────────────────────────
//   ID (PK)           GORM FK association
//   business_id       guest_feedback_scheduler.go (bill.Business.BusinessId)
//   user_id           RBAC checks (GetBillBusinessAccessByBillID pattern)
//   owner_address     RBAC checks
//   name              guest_feedback_scheduler.go, print/build_inputs.go
//   default_currency  public_guest_response_helpers.go, business_handlers.go,
//                     print/build_inputs.go
//   display_currency  same as default_currency
//   default_language  guest_feedback_scheduler.go
//   crm_enabled       crm/settlement.go
//   address.*         print/build_inputs.go (embedded: street/city/state/
//                       postal_code/country)
//   is_active         admin lifecycle lock (suspend)
//   closed_at         admin lifecycle lock (close)

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// billProjectionSQLRecorder is a minimal GORM logger that captures SQL
// statements so tests can assert the access shape without running Postgres.
type billProjectionSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *billProjectionSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func (r *billProjectionSQLRecorder) statementSelectsFrom(table, statement string) bool {
	n := strings.ToLower(strings.TrimSpace(statement))
	return strings.Contains(n, "from `"+table+"`") ||
		strings.Contains(n, `from "`+table+`"`) ||
		strings.Contains(n, "from "+table)
}

// isSelectStar returns true when the statement is a bare SELECT * over the
// given table. GORM emits "SELECT `businesses`.*" (table-qualified star) for
// unprojected Preloads; we check for both forms.
func (r *billProjectionSQLRecorder) isSelectStar(table, statement string) bool {
	n := strings.ToLower(strings.TrimSpace(statement))
	if !r.statementSelectsFrom(table, statement) {
		return false
	}
	return strings.HasPrefix(n, "select *") ||
		strings.HasPrefix(n, "select `"+table+"`.*") ||
		strings.HasPrefix(n, `select "`+table+`".*`)
}

func (r *billProjectionSQLRecorder) selectStarCountForTable(table string) int {
	count := 0
	for _, s := range r.statements {
		if r.isSelectStar(table, s) {
			count++
		}
	}
	return count
}

func (r *billProjectionSQLRecorder) selectMentionsColumn(table, column string) bool {
	needleBacktick := "`" + strings.ToLower(column) + "`"
	needleDouble := `"` + strings.ToLower(column) + `"`
	for _, s := range r.statements {
		if !r.statementSelectsFrom(table, s) {
			continue
		}
		n := strings.ToLower(s)
		if strings.Contains(n, needleBacktick) ||
			strings.Contains(n, needleDouble) ||
			strings.Contains(n, "."+strings.ToLower(column)) {
			return true
		}
	}
	return false
}

// setupBillProjectionDB creates an in-memory SQLite database with one
// Business, Table, Bill, and Payment seeded, and wires the package-level db
// to it so GetBillByID can be exercised.
func setupBillProjectionDB(t testing.TB, rec logger.Interface) (business *Business, bill *Bill) {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())

	cfg := &gorm.Config{}
	if rec != nil {
		cfg.Logger = rec
	}

	gormDB, err := gorm.Open(sqlite.Open(dsn), cfg)
	if err != nil {
		t.Fatalf("open in-memory database: %v", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	SetTestDB(gormDB)
	if err := db.AutoMigrate(&Business{}, &Table{}, &Bill{}, &Payment{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	business = &Business{
		BusinessId:      fmt.Sprintf("billproj-%d", time.Now().UnixNano()),
		Name:            "Projection Test Biz",
		Logo:            "https://cdn.example.com/logo.png",
		Timezone:        "America/Argentina/Buenos_Aires",
		OwnerAddress:    "0xProjOwner",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
		DisplayCurrency: "USD",
		DefaultLanguage: "en",
		CRMEnabled:      true,
		IsActive:        true,
		// Heavy columns that should NOT be loaded — these will be populated in the
		// DB row but the projected query must omit them.
	}
	if err := db.Create(business).Error; err != nil {
		t.Fatalf("create business: %v", err)
	}

	table := &Table{
		BusinessID: business.ID,
		TableCode:  "PROJ-T1",
		Name:       "Proj Table",
		QRCode:     "qr",
		IsActive:   true,
	}
	if err := db.Create(table).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}

	bill = &Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     fmt.Sprintf("PROJ-BILL-%d", time.Now().UnixNano()),
		Items:          `[]`,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		Status:         BillStatusOpen,
	}
	if err := db.Create(bill).Error; err != nil {
		t.Fatalf("create bill: %v", err)
	}

	return business, bill
}

// TestGetBillByIDProjectsBusinessNoSelectStar asserts that GetBillByID issues
// a projected SELECT over businesses (no SELECT *) and that the projected row
// still carries the fields callers actually read.
func TestGetBillByIDProjectsBusinessNoSelectStar(t *testing.T) {
	rec := &billProjectionSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	_, seedBill := setupBillProjectionDB(t, rec)
	rec.statements = nil // only count queries from GetBillByID

	gotBill, _, err := GetBillByID(seedBill.ID)
	if err != nil {
		t.Fatalf("GetBillByID: %v", err)
	}

	// --- access-shape: no SELECT * over businesses ---
	if n := rec.selectStarCountForTable("businesses"); n > 0 {
		t.Fatalf("GetBillByID still issues %d SELECT * over businesses; statements:\n%v",
			n, rec.statements)
	}

	// --- key caller fields must be populated (projection not over-narrow) ---
	if gotBill.Business.DefaultCurrency == "" {
		t.Error("projected business preload: DefaultCurrency is empty")
	}
	if gotBill.Business.Name == "" {
		t.Error("projected business preload: Name is empty")
	}
	if gotBill.Business.ID == 0 {
		t.Error("projected business preload: ID is zero")
	}
	// Logo + Timezone are emitted by newBillPublicBusiness() in the bill JSON;
	// under-projecting them silently zeroes the bill payload fields.
	if gotBill.Business.Logo == "" {
		t.Error("projected business preload: Logo is empty (consumed by bill JSON serializer)")
	}
	if gotBill.Business.Timezone == "" {
		t.Error("projected business preload: Timezone is empty (consumed by bill JSON serializer)")
	}

	// --- selectMentionsColumn confirms the projected query names the column ---
	for _, col := range []string{
		"default_currency",
		"display_currency",
		"name",
		"logo",
		"timezone",
		"crm_enabled",
		"default_language",
	} {
		if !rec.selectMentionsColumn("businesses", col) {
			t.Errorf("projected business preload missing column %q in SELECT", col)
		}
	}

	// --- the heavy columns that should NOT be loaded ---
	for _, heavyCol := range []string{
		"stripe_customer_id",
		"stripe_subscription_id",
		"onboarding_state",
	} {
		if rec.selectMentionsColumn("businesses", heavyCol) {
			t.Errorf("projected business preload unexpectedly loads heavy column %q", heavyCol)
		}
	}
}

// BenchmarkGetBillByID measures bytes/op and allocs/op for the bill-detail
// path. Run before and after implementing the projection to capture the delta.
func BenchmarkGetBillByID(b *testing.B) {
	_, seedBill := setupBillProjectionDB(b, logger.Default.LogMode(logger.Silent))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bill, _, err := GetBillByID(seedBill.ID)
		if err != nil {
			b.Fatal(err)
		}
		if bill == nil {
			b.Fatal("nil bill")
		}
	}
}
