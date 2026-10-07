package database

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupGuestBillAccessDB seeds one active business and one bill on it (plus
// several payments and a bill.Items JSON snapshot so the over-fetching legacy
// path has extra rows / payload to hydrate), then wires the package-level db at
// it. It returns the active business id, the operator bill number, and the
// guest public_token capability.
func setupGuestBillAccessDB(t testing.TB) (activeBusinessID uint, billNumber string, publicToken string) {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err, "open in-memory database")

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	SetTestDB(gormDB)
	// BillItem is intentionally NOT auto-migrated: its gen_random_uuid() default
	// is unsupported under SQLite, and GetBillByID falls back to the bill.Items
	// JSON snapshot when the table is absent — which is exactly the over-fetch
	// shape the _Before benchmark must exercise.
	require.NoError(t, db.AutoMigrate(
		&Business{}, &Table{}, &Bill{}, &Payment{}, &AlternativePayment{},
	))

	business := &Business{
		BusinessId:      fmt.Sprintf("guest-bill-access-%d", time.Now().UnixNano()),
		Name:            "Guest Bill Access",
		OwnerAddress:    "0xGuestBillAccessOwner",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "AED",
		IsActive:        true,
	}
	require.NoError(t, db.Create(business).Error)

	billItems := "[" + strings.TrimSuffix(strings.Repeat(`{"id":"item","name":"Bench","quantity":1},`, 64), ",") + "]"
	bill := &Bill{
		BusinessID:     business.ID,
		BillNumber:     "B-1001",
		Items:          billItems,
		Subtotal:       2500,
		TotalAmount:    2500,
		Status:         BillStatusOpen,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, db.Create(bill).Error)

	for i := 0; i < 5; i++ {
		require.NoError(t, db.Create(&Payment{
			BillID:    bill.ID,
			PayerAddr: fmt.Sprintf("0xpayer-%d", i),
			Amount:    100,
			TxHash:    fmt.Sprintf("0xpayment-%d", i),
		}).Error)
	}

	var saved Bill
	require.NoError(t, db.Select("id", "bill_number", "public_token").First(&saved, bill.ID).Error)
	require.NotEmpty(t, saved.PublicToken)

	return business.ID, saved.BillNumber, saved.PublicToken
}

// newQueryCounter registers a post-query callback that increments a counter on
// every SELECT issued against the package-level db, returning the live counter
// and a cleanup func. This is the access-shape probe: the lean resolver must
// emit exactly ONE query (no preloads, no items snapshot, no alt-payment load).
func newQueryCounter(t testing.TB) (count *int, cleanup func()) {
	t.Helper()
	n := 0
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("guest_bill_access_count", func(tx *gorm.DB) {
		n++
	}))
	return &n, func() {
		_ = db.Callback().Query().Remove("guest_bill_access_count")
	}
}

func TestGetGuestBillIDByToken(t *testing.T) {
	activeBusinessID, _, publicToken := setupGuestBillAccessDB(t)
	_ = activeBusinessID

	count, cleanup := newQueryCounter(t)
	defer cleanup()

	id, err := GetGuestBillIDByToken(publicToken)
	require.NoError(t, err)
	require.NotZero(t, id, "resolver must return the bill id")
	assert.Equal(t, 1, *count, "lean resolver must issue exactly ONE query (no preloads, no items snapshot, no alt-payments load)")
}

func TestGetGuestBillIDByTokenNotFound(t *testing.T) {
	setupGuestBillAccessDB(t)

	id, err := GetGuestBillIDByToken("does-not-exist")
	require.Error(t, err)
	assert.Zero(t, id)
}

func TestGetGuestBillIDByTokenRejectsInactiveBusiness(t *testing.T) {
	activeBusinessID, _, publicToken := setupGuestBillAccessDB(t)
	require.NoError(t, db.Model(&Business{}).Where("id = ?", activeBusinessID).
		Update("is_active", false).Error)

	id, err := GetGuestBillIDByToken(publicToken)
	require.Error(t, err, "a bill belonging to an inactive business must not be resolvable")
	assert.Zero(t, id)
}

func TestGetGuestBillIDByTokenRejectsBillNumber(t *testing.T) {
	_, billNumber, _ := setupGuestBillAccessDB(t)
	id, err := GetGuestBillIDByToken(billNumber)
	require.Error(t, err, "guessable bill_number must not act as guest capability")
	assert.Zero(t, id)
}

func TestGetGuestBillScopeByToken(t *testing.T) {
	activeBusinessID, _, publicToken := setupGuestBillAccessDB(t)

	count, cleanup := newQueryCounter(t)
	defer cleanup()

	id, businessID, err := GetGuestBillScopeByToken(publicToken)
	require.NoError(t, err)
	require.NotZero(t, id, "resolver must return the bill id")
	assert.Equal(t, activeBusinessID, businessID, "resolver must return the owning business id")
	assert.Equal(t, 1, *count, "scope resolver must issue exactly ONE query (no preloads, no snapshots)")
}

func TestGetGuestBillScopeByTokenNotFound(t *testing.T) {
	setupGuestBillAccessDB(t)

	id, businessID, err := GetGuestBillScopeByToken("does-not-exist")
	require.Error(t, err)
	assert.Zero(t, id)
	assert.Zero(t, businessID)
}

func TestGetGuestBillScopeByTokenRejectsInactiveBusiness(t *testing.T) {
	activeBusinessID, _, publicToken := setupGuestBillAccessDB(t)
	require.NoError(t, db.Model(&Business{}).Where("id = ?", activeBusinessID).
		Update("is_active", false).Error)

	id, businessID, err := GetGuestBillScopeByToken(publicToken)
	require.Error(t, err)
	assert.Zero(t, id)
	assert.Zero(t, businessID)
}

// TestGuestResolvers_AccessShapeNoBillNumberInWhere locks the security
// regression: guest resolvers must probe public_token only (unique index),
// never bill_number.
func TestGuestResolvers_AccessShapeNoBillNumberInWhere(t *testing.T) {
	_, billNumber, publicToken := setupGuestBillAccessDB(t)
	_ = billNumber

	var stmts []string
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("guest_token_where_shape", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.SQL.String() != "" {
			stmts = append(stmts, strings.ToLower(tx.Statement.SQL.String()))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove("guest_token_where_shape") })

	_, err := GetGuestBillIDByToken(publicToken)
	require.NoError(t, err)
	_, _, err = GetGuestBillScopeByToken(publicToken)
	require.NoError(t, err)
	_, _, err = GetPublicBillByToken(publicToken)
	require.NoError(t, err)

	require.NotEmpty(t, stmts)
	sawTokenWhere := false
	for _, sql := range stmts {
		if !strings.Contains(sql, "bills") {
			continue
		}
		// bill_number may appear in SELECT projections; forbid it only in WHERE.
		whereIdx := strings.Index(sql, "where")
		if whereIdx < 0 {
			continue
		}
		whereClause := sql[whereIdx:]
		assert.NotContains(t, whereClause, "bill_number", "guest resolver WHERE must not reference bill_number: %s", sql)
		if strings.Contains(whereClause, "public_token") {
			sawTokenWhere = true
		}
	}
	assert.True(t, sawTokenWhere, "at least one guest resolver query must WHERE on public_token")
}

// BenchmarkGuestOrdersBillResolve_Before captures the operator aggregate
// (relation preloads + items snapshot + alt-payment load) as the fat baseline
// next to the lean guest resolver.
func BenchmarkGuestOrdersBillResolve_Before(b *testing.B) {
	_, billNumber, _ := setupGuestBillAccessDB(b)
	var seed Bill
	if err := db.Select("id").Where("bill_number = ?", billNumber).First(&seed).Error; err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bill, _, err := GetBillByID(seed.ID)
		if err != nil {
			b.Fatal(err)
		}
		if bill.ID == 0 {
			b.Fatal("expected bill id")
		}
	}
}

// BenchmarkGuestOrdersBillResolve_After captures the lean single-query path.
func BenchmarkGuestOrdersBillResolve_After(b *testing.B) {
	_, _, publicToken := setupGuestBillAccessDB(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id, err := GetGuestBillIDByToken(publicToken)
		if err != nil {
			b.Fatal(err)
		}
		if id == 0 {
			b.Fatal("expected bill id")
		}
	}
}

// BenchmarkPublicBillByToken is the guest bill detail path (token + active join).
func BenchmarkPublicBillByToken(b *testing.B) {
	_, _, publicToken := setupGuestBillAccessDB(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bill, _, err := GetPublicBillByToken(publicToken)
		if err != nil {
			b.Fatal(err)
		}
		if bill.ID == 0 {
			b.Fatal("expected bill id")
		}
	}
}
