package database

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type aiAttributionSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *aiAttributionSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func setupAiAttributionDB(t testing.TB, gormLogger logger.Interface) *Business {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	cfg := &gorm.Config{}
	if gormLogger != nil {
		cfg.Logger = gormLogger
	}
	gormDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())), cfg)
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&Business{}, &Table{}, &Bill{}, &Order{}, &AiWaiterConversation{},
	))
	// bill_items: uuid default is Postgres-only; create with explicit ids.
	require.NoError(t, gormDB.Exec(`CREATE TABLE bill_items (
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
	)`).Error)

	business := &Business{
		BusinessId:     fmt.Sprintf("ai-attr-%d", time.Now().UnixNano()),
		Name:           "AI Attribution Biz",
		OwnerAddress:   fmt.Sprintf("0xaiattr%x", time.Now().UnixNano()),
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, gormDB.Create(business).Error)
	return business
}

func seedAiAttributionScenario(t testing.TB, business *Business, now time.Time) {
	t.Helper()
	gormDB := GetDB()

	table := &Table{BusinessID: business.ID, TableCode: fmt.Sprintf("ATTR-%d", now.UnixNano()), Name: "T1", Capacity: 4, IsActive: true}
	require.NoError(t, gormDB.Create(table).Error)

	bill := &Bill{
		BusinessID: business.ID, TableID: table.ID,
		BillNumber:     fmt.Sprintf("ATTR-BILL-%d", now.UnixNano()),
		Status:         BillStatusOpen,
		SettlementAddr: "settle", TippingAddr: "tip",
	}
	require.NoError(t, gormDB.Create(bill).Error)

	// AI conversation with cart adds: active 2h ago for 20 minutes.
	convStart := now.Add(-2 * time.Hour)
	convEnd := convStart.Add(20 * time.Minute)
	require.NoError(t, gormDB.Create(&AiWaiterConversation{
		SessionID: fmt.Sprintf("sess-%d", now.UnixNano()), BusinessID: business.ID,
		TableCode: table.TableCode, Mode: "ordering", Status: "closed",
		CartItemsAdded: 2, CreatedAt: convStart, UpdatedAt: convEnd,
	}).Error)

	suffix := fmt.Sprintf("%d", now.UnixNano())

	// Guest order placed 10 minutes after the last AI message → attributed.
	inWindow := &Order{
		BillID: bill.ID, BusinessID: business.ID, OrderNumber: "ORD-IN-" + suffix,
		Status: OrderStatusApproved, CreatedBy: "guest", CreatedAt: convEnd.Add(10 * time.Minute),
	}
	require.NoError(t, gormDB.Create(inWindow).Error)
	// $18.50 + $6.00 of items on the attributed order.
	require.NoError(t, gormDB.Exec(
		"INSERT INTO bill_items (id, bill_id, name, price, quantity, subtotal, order_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"attr-item-1-"+suffix, bill.ID, "Burger", 18.50, 1, 18.50, inWindow.ID, inWindow.CreatedAt).Error)
	require.NoError(t, gormDB.Exec(
		"INSERT INTO bill_items (id, bill_id, name, price, quantity, subtotal, order_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"attr-item-2-"+suffix, bill.ID, "Fries", 6.00, 1, 6.00, inWindow.ID, inWindow.CreatedAt).Error)

	// Guest order 3 hours after the conversation → NOT attributed.
	late := &Order{
		BillID: bill.ID, BusinessID: business.ID, OrderNumber: "ORD-LATE-" + suffix,
		Status: OrderStatusApproved, CreatedBy: "guest", CreatedAt: convEnd.Add(3 * time.Hour),
	}
	require.NoError(t, gormDB.Create(late).Error)
	require.NoError(t, gormDB.Exec(
		"INSERT INTO bill_items (id, bill_id, name, price, quantity, subtotal, order_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"late-item-1-"+suffix, bill.ID, "Steak", 42.00, 1, 42.00, late.ID, late.CreatedAt).Error)

	// Staff-created order during the window → NOT attributed (created_by != guest).
	staffOrder := &Order{
		BillID: bill.ID, BusinessID: business.ID, OrderNumber: "ORD-STAFF-" + suffix,
		Status: OrderStatusApproved, CreatedBy: "0xstaff", CreatedAt: convEnd.Add(5 * time.Minute),
	}
	require.NoError(t, gormDB.Create(staffOrder).Error)
	require.NoError(t, gormDB.Exec(
		"INSERT INTO bill_items (id, bill_id, name, price, quantity, subtotal, order_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"staff-item-1-"+suffix, bill.ID, "Wine", 30.00, 1, 30.00, staffOrder.ID, staffOrder.CreatedAt).Error)
}

func TestGetAiAttributedOrderValue(t *testing.T) {
	business := setupAiAttributionDB(t, logger.Default.LogMode(logger.Silent))
	now := time.Now().UTC()
	seedAiAttributionScenario(t, business, now)

	got, err := GetAiAttributedOrderValue(business.ID, now.AddDate(0, 0, -30), now)
	require.NoError(t, err)
	assert.InDelta(t, 24.50, got.RevenueDollars, 0.001,
		"only guest-order items inside the conversation window count")
	assert.EqualValues(t, 1, got.OrderCount)

	// Scoping: another business in the same window sees zero.
	other := &Business{
		BusinessId: fmt.Sprintf("ai-attr-other-%d", now.UnixNano()), Name: "Other",
		OwnerAddress:   fmt.Sprintf("0xother%x", now.UnixNano()),
		SettlementAddr: "s", TippingAddr: "t",
	}
	require.NoError(t, GetDB().Create(other).Error)
	empty, err := GetAiAttributedOrderValue(other.ID, now.AddDate(0, 0, -30), now)
	require.NoError(t, err)
	assert.Zero(t, empty.OrderCount)
	assert.Zero(t, empty.RevenueDollars)
}

// Access-shape gate: attribution is ONE aggregate SQL statement — no per-
// conversation loops, no SELECT *, no hydration of conversations/bills.
func TestGetAiAttributedOrderValueAccessShape(t *testing.T) {
	recorder := &aiAttributionSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	business := setupAiAttributionDB(t, recorder)
	now := time.Now().UTC()
	seedAiAttributionScenario(t, business, now)

	recorder.statements = nil
	_, err := GetAiAttributedOrderValue(business.ID, now.AddDate(0, 0, -30), now)
	require.NoError(t, err)

	selects := 0
	for _, s := range recorder.statements {
		normalized := strings.ToLower(strings.TrimSpace(s))
		if strings.HasPrefix(normalized, "select") {
			selects++
			assert.False(t, strings.HasPrefix(normalized, "select *"),
				"attribution must project aggregates, not full rows: %s", s)
		}
	}
	assert.Equal(t, 1, selects, "attribution must be a single aggregate query")
}

func BenchmarkGetAiAttributedOrderValueSQLite(b *testing.B) {
	business := setupAiAttributionDB(b, logger.Default.LogMode(logger.Silent))
	now := time.Now().UTC()
	for i := 0; i < 25; i++ {
		seedAiAttributionScenario(b, business, now.Add(-time.Duration(i)*time.Hour))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := GetAiAttributedOrderValue(business.ID, now.AddDate(0, 0, -30), now); err != nil {
			b.Fatal(err)
		}
	}
}
