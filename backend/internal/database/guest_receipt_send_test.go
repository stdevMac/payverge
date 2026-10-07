package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestClaimGuestReceiptSendSlotIgnoresSendsOutsideWindow(t *testing.T) {
	dsn := "file:guest_receipt_send_window_" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&Business{}, &Bill{}, &GuestReceiptSend{}))
	require.NoError(t, db.Create(&Business{
		ID: 1, BusinessId: "biz", Name: "B", OwnerAddress: "0x",
		SettlementAddr: "0x1", TippingAddr: "0x2",
	}).Error)
	require.NoError(t, db.Create(&Bill{
		ID: 1, BusinessID: 1, BillNumber: "B-1", PublicToken: "tok",
		Status: BillStatusPaid, SettlementAddr: "0x1", TippingAddr: "0x2",
	}).Error)
	old := time.Now().UTC().Add(-25 * time.Hour)
	for i := 0; i < GuestReceiptSendLimit; i++ {
		require.NoError(t, db.Create(&GuestReceiptSend{
			BillID: 1, SentAt: old, RecipientRedacted: "g***@example.com",
		}).Error)
	}

	id, err := ClaimGuestReceiptSendSlot(db, 1, "g***@example.com")
	require.NoError(t, err)
	require.NotZero(t, id)

	for i := 0; i < GuestReceiptSendLimit-1; i++ {
		_, err = ClaimGuestReceiptSendSlot(db, 1, "g***@example.com")
		require.NoError(t, err)
	}
	_, err = ClaimGuestReceiptSendSlot(db, 1, "g***@example.com")
	require.ErrorIs(t, err, ErrGuestReceiptSendLimit)
}
