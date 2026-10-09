package database

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database/dbtest"
	"github.com/stdevmac/payverge/backend/schema/genesis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupPaymentEventsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:payment-events-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	require.NoError(t, db.AutoMigrate(&Business{}, &Table{}, &Bill{}, &Payment{}, &AlternativePayment{}))
	require.NoError(t, dbtest.EnsurePaymentEventsView(db))
	SetTestDB(db)
	return db
}

func TestEnsurePaymentEventsViewUnionsBothTables(t *testing.T) {
	db := setupPaymentEventsTestDB(t)

	business := &Business{
		BusinessId:     "pe-view-biz",
		Name:           "PE View",
		OwnerAddress:   "0xowner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, db.Create(business).Error)

	table := &Table{BusinessID: business.ID, TableCode: "T1", Name: "One", IsActive: true}
	require.NoError(t, db.Create(table).Error)

	now := time.Now().UTC().Truncate(time.Second)
	billA := &Bill{
		BusinessID: business.ID, TableID: table.ID, BillNumber: "A-1",
		TotalAmount: 1000, Status: BillStatusPaid,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		CreatedAt:      now, UpdatedAt: now,
	}
	billB := &Bill{
		BusinessID: business.ID, TableID: table.ID, BillNumber: "B-1",
		TotalAmount: 2000, Status: BillStatusPaid,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		CreatedAt:      now, UpdatedAt: now,
	}
	require.NoError(t, db.Create(billA).Error)
	require.NoError(t, db.Create(billB).Error)

	require.NoError(t, db.Create(&Payment{
		BillID: billA.ID, PayerAddr: "0xp", Amount: 1000, TipAmount: 100,
		TxHash: "0xtx", Status: PaymentStatusConfirmed, PaymentMethod: "crypto",
		CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&AlternativePayment{
		BillID: billB.ID, ParticipantAddr: "guest", Amount: 2000,
		BillAmountCents: 2000, TipAmountCents: 200,
		PaymentMethod: PaymentMethodCard, Status: AltPaymentStatusConfirmed,
		IdempotencyKey: "auth_XYZ", CreatedAt: now, UpdatedAt: now,
	}).Error)

	var events []PaymentEvent
	require.NoError(t, PaymentEventsQuery(db).
		Where("business_id = ?", business.ID).
		Order("source_table ASC").
		Find(&events).Error)
	require.Len(t, events, 2)

	bySource := map[string]PaymentEvent{}
	for _, e := range events {
		bySource[e.SourceTable] = e
	}
	require.Contains(t, bySource, "payments")
	require.Contains(t, bySource, "alternative_payments")
	assert.Equal(t, int64(1000), bySource["payments"].AmountCents)
	assert.Equal(t, "0xtx", bySource["payments"].ExternalRef)
	assert.Equal(t, int64(2000), bySource["alternative_payments"].AmountCents)
	assert.Equal(t, "auth_XYZ", bySource["alternative_payments"].ExternalRef)
	assert.Equal(t, "card", bySource["alternative_payments"].Method)
}

func TestEnsurePaymentEventsViewIsIdempotent(t *testing.T) {
	db := setupPaymentEventsTestDB(t)
	require.NoError(t, dbtest.EnsurePaymentEventsView(db))
	require.NoError(t, dbtest.EnsurePaymentEventsView(db))
}

// The SQLite test view must expose the same columns, in the same order, as the
// genesis view production reads.
func TestPaymentEventsTestViewMatchesGenesisColumns(t *testing.T) {
	columns := func(body string) []string {
		var out []string
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), ","))
			if strings.HasPrefix(line, "FROM") || strings.HasPrefix(line, "UNION") {
				break
			}
			line = strings.TrimPrefix(line, "SELECT")
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "CREATE") {
				continue
			}
			if i := strings.LastIndex(line, " AS "); i >= 0 {
				out = append(out, line[i+4:])
				continue
			}
			out = append(out, line[strings.LastIndex(line, ".")+1:])
		}
		return out
	}
	start := strings.Index(genesis.SchemaSQL, "CREATE VIEW public.payment_events AS")
	require.GreaterOrEqual(t, start, 0)
	want := columns(genesis.SchemaSQL[start:])
	require.Len(t, want, 13)
	require.Equal(t, want, columns(dbtest.PaymentEventsViewSQL))
}
