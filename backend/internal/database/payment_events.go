package database

import (
	"time"

	"gorm.io/gorm"
)

// PaymentEvent is one row of the payment_events read model — the union of
// `payments` (crypto/cross-chain/plugin) and `alternative_payments`
// (cash/card/venmo/other). Money fields are integer cents.
//
// The view comes from the genesis baseline
// (backend/schema/genesis/current_schema.sql). SQLite tests install an
// equivalent one with dbtest.EnsurePaymentEventsView.
type PaymentEvent struct {
	ID          uint      `gorm:"column:id" json:"id"`
	SourceTable string    `gorm:"column:source_table" json:"source_table"`
	BillID      uint      `gorm:"column:bill_id" json:"bill_id"`
	BusinessID  uint      `gorm:"column:business_id" json:"business_id"`
	Method      string    `gorm:"column:method" json:"method"`
	Status      string    `gorm:"column:status" json:"status"`
	AmountCents int64     `gorm:"column:amount_cents" json:"amount_cents"`
	TipCents    int64     `gorm:"column:tip_cents" json:"tip_cents"`
	Currency    string    `gorm:"column:currency" json:"currency"`
	PayerRef    string    `gorm:"column:payer_ref" json:"payer_ref"`
	ExternalRef string    `gorm:"column:external_ref" json:"external_ref"`
	CreatedAt   time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at" json:"updated_at"`
}

// TableName pins GORM to the payment_events view.
func (PaymentEvent) TableName() string { return "payment_events" }

// PaymentEventsQuery returns a GORM query scoped to the payment_events view.
// Callers project columns and join bills/tables as needed — never SELECT *.
func PaymentEventsQuery(db *gorm.DB) *gorm.DB {
	if db == nil {
		db = GetDB()
	}
	return db.Table("payment_events")
}
