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

type guestCheckoutSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *guestCheckoutSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func (r *guestCheckoutSQLRecorder) selectCount(table string) int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		if !strings.HasPrefix(normalized, "select ") {
			continue
		}
		if strings.Contains(normalized, "from `"+table+"`") ||
			strings.Contains(normalized, "from \""+table+"\"") ||
			strings.Contains(normalized, "from "+table) {
			count++
		}
	}
	return count
}

func (r *guestCheckoutSQLRecorder) selectStarCount(table string) int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		if !strings.HasPrefix(normalized, "select *") {
			continue
		}
		if strings.Contains(normalized, "from `"+table+"`") ||
			strings.Contains(normalized, "from \""+table+"\"") ||
			strings.Contains(normalized, "from "+table) {
			count++
		}
	}
	return count
}

func (r *guestCheckoutSQLRecorder) selectMentionsColumn(table, column string) bool {
	needleBacktick := "`" + strings.ToLower(column) + "`"
	needleDouble := `"` + strings.ToLower(column) + `"`
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		if !strings.Contains(normalized, "from `"+table+"`") &&
			!strings.Contains(normalized, "from \""+table+"\"") &&
			!strings.Contains(normalized, "from "+table) {
			continue
		}
		if strings.Contains(normalized, needleBacktick) ||
			strings.Contains(normalized, needleDouble) ||
			strings.Contains(normalized, "."+strings.ToLower(column)) {
			return true
		}
	}
	return false
}

func setupGuestCheckoutAccessDB(t testing.TB, rec logger.Interface) (*Business, *Table, *Bill) {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	cfg := &gorm.Config{}
	if rec != nil {
		cfg.Logger = rec
	}
	gormDB, err := gorm.Open(sqlite.Open(dsn), cfg)
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	SetTestDB(gormDB)
	require.NoError(t, db.AutoMigrate(&Business{}, &Table{}, &Bill{}, &Payment{}, &AlternativePayment{}))

	business := &Business{
		BusinessId:      fmt.Sprintf("guest-checkout-%d", time.Now().UnixNano()),
		Name:            "Guest Checkout Restaurant",
		OwnerAddress:    "0xGuestCheckoutOwner",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
		IsActive:        true,
	}
	require.NoError(t, db.Create(business).Error)

	table := &Table{
		BusinessID: business.ID,
		TableCode:  fmt.Sprintf("CHK-%d", time.Now().UnixNano()),
		Name:       "Checkout Table",
		Capacity:   4,
		QRCode:     strings.Repeat("qr-payload", 64),
		IsActive:   true,
	}
	require.NoError(t, db.Create(table).Error)

	bill := &Bill{
		BusinessID:  business.ID,
		TableID:     table.ID,
		BillNumber:  fmt.Sprintf("CHK-BILL-%d", time.Now().UnixNano()),
		Status:      BillStatusOpen,
		Items:       "[]",
		TotalAmount: 4200,
		PaidAmount:  0,
	}
	require.NoError(t, db.Create(bill).Error)

	require.NoError(t, db.Create(&Payment{
		BillID:    bill.ID,
		PayerAddr: "0xpayer",
		Amount:    100,
		TxHash:    "0xexisting-payment",
	}).Error)

	return business, table, bill
}

func TestGetBillBusinessIDByBillIDResolvesScalarOnly(t *testing.T) {
	recorder := &guestCheckoutSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	business, _, bill := setupGuestCheckoutAccessDB(t, recorder)

	recorder.statements = nil
	gotBusinessID, err := GetBillBusinessIDByBillID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, business.ID, gotBusinessID)
	assert.Equal(t, 1, recorder.selectCount("bills"))
	assert.Zero(t, recorder.selectCount("payments"))
	assert.Zero(t, recorder.selectCount("businesses"))
	assert.Zero(t, recorder.selectCount("alternative_payments"))
}

func TestGetGuestPluginCheckoutBillByTokenQueryShape(t *testing.T) {
	recorder := &guestCheckoutSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	_, table, bill := setupGuestCheckoutAccessDB(t, recorder)

	var saved Bill
	require.NoError(t, db.Select("id", "public_token", "bill_number").First(&saved, bill.ID).Error)
	require.NotEmpty(t, saved.PublicToken)

	recorder.statements = nil
	gotBill, gotTable, err := GetGuestPluginCheckoutBillByToken(saved.PublicToken)
	require.NoError(t, err)
	require.NotNil(t, gotBill)
	require.NotNil(t, gotTable)

	assert.Equal(t, bill.ID, gotBill.ID)
	assert.Equal(t, bill.BusinessID, gotBill.BusinessID)
	assert.Equal(t, bill.BillNumber, gotBill.BillNumber)
	assert.Equal(t, bill.TotalAmount, gotBill.TotalAmount)
	assert.Equal(t, bill.PaidAmount, gotBill.PaidAmount)
	assert.Equal(t, table.TableCode, gotTable.TableCode)

	assert.Equal(t, 1, recorder.selectCount("bills"), "bill lookup should be one lean join")
	assert.Equal(t, 1, recorder.selectCount("tables"), "table lookup should project only table_code")
	assert.Zero(t, recorder.selectCount("payments"), "guest checkout must not preload payments")
	assert.Zero(t, recorder.selectCount("alternative_payments"))
	assert.Zero(t, recorder.selectStarCount("bills"))
	assert.Zero(t, recorder.selectStarCount("businesses"))
	assert.False(t, recorder.selectMentionsColumn("bills", "items"))
	// Capability probe is public_token only — never bill_number.
	for _, stmt := range recorder.statements {
		lower := strings.ToLower(stmt)
		if strings.Contains(lower, "from") && strings.Contains(lower, "bills") && strings.Contains(lower, "where") {
			assert.NotContains(t, lower, "bill_number =", "plugin checkout WHERE must not use bill_number")
			assert.Contains(t, lower, "public_token")
		}
	}
}

func TestGetGuestPluginCheckoutBusinessQueryShape(t *testing.T) {
	recorder := &guestCheckoutSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	business, _, _ := setupGuestCheckoutAccessDB(t, recorder)

	recorder.statements = nil
	gotBusiness, err := GetGuestPluginCheckoutBusiness(business.ID)
	require.NoError(t, err)
	require.NotNil(t, gotBusiness)

	assert.Equal(t, business.ID, gotBusiness.ID)
	assert.Equal(t, business.Name, gotBusiness.Name)
	assert.Equal(t, business.DefaultCurrency, gotBusiness.DefaultCurrency)

	assert.Equal(t, 1, recorder.selectCount("businesses"))
	assert.Zero(t, recorder.selectStarCount("businesses"))
	assert.True(t, recorder.selectMentionsColumn("businesses", "default_currency"))
	assert.False(t, recorder.selectMentionsColumn("businesses", "stripe_customer_id"))
}
