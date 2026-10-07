package database

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// GuestReceiptSendLimit is the number of successful guest receipt emails
	// allowed per bill inside GuestReceiptSendWindow.
	GuestReceiptSendLimit  = 3
	GuestReceiptSendWindow = 24 * time.Hour
)

// ErrGuestReceiptSendLimit is returned when a bill is already at the 24h send cap.
var ErrGuestReceiptSendLimit = errors.New("guest receipt send limit")

// GuestReceiptSend is one successful (or reserved) guest-initiated receipt email.
type GuestReceiptSend struct {
	ID                uint      `gorm:"primaryKey"`
	BillID            uint      `gorm:"column:bill_id;index:idx_guest_receipt_sends_bill_sent,priority:1;not null"`
	SentAt            time.Time `gorm:"column:sent_at;index:idx_guest_receipt_sends_bill_sent,priority:2;not null"`
	RecipientRedacted string    `gorm:"column:recipient_redacted;not null"`
}

// TableName pins the genesis table name.
func (GuestReceiptSend) TableName() string { return "guest_receipt_sends" }

// ClaimGuestReceiptSendSlot reserves one send against the per-bill 24h cap.
// On ErrGuestReceiptSendLimit the mailer must not be called. Release the
// returned id if the subsequent send fails so failed attempts do not count.
func ClaimGuestReceiptSendSlot(db *gorm.DB, billID uint, recipientRedacted string) (uint, error) {
	if db == nil {
		return 0, gorm.ErrInvalidDB
	}
	var id uint
	err := db.Transaction(func(tx *gorm.DB) error {
		var bill Bill
		if err := tx.Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).First(&bill, billID).Error; err != nil {
			return err
		}
		since := time.Now().Add(-GuestReceiptSendWindow)
		n, err := countRecentGuestReceiptSends(tx, billID, since)
		if err != nil {
			return err
		}
		if n >= GuestReceiptSendLimit {
			return ErrGuestReceiptSendLimit
		}
		row := GuestReceiptSend{
			BillID:            billID,
			SentAt:            time.Now().UTC(),
			RecipientRedacted: recipientRedacted,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		id = row.ID
		return nil
	})
	return id, err
}

func countRecentGuestReceiptSends(tx *gorm.DB, billID uint, since time.Time) (int64, error) {
	var rows []GuestReceiptSend
	if err := tx.Select("id", "sent_at").Where("bill_id = ?", billID).Find(&rows).Error; err != nil {
		return 0, err
	}
	var n int64
	for _, row := range rows {
		if row.SentAt.After(since) {
			n++
		}
	}
	return n, nil
}

// ReleaseGuestReceiptSendSlot drops a reserved send so a failed mailer call
// does not consume the 24h quota.
func ReleaseGuestReceiptSendSlot(db *gorm.DB, sendID uint) error {
	if db == nil || sendID == 0 {
		return nil
	}
	return db.Delete(&GuestReceiptSend{}, sendID).Error
}
