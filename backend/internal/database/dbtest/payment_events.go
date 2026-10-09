// Package dbtest holds SQLite test scaffolding for database objects that the
// genesis baseline owns in Postgres. Only tests import it.
package dbtest

import "gorm.io/gorm"

// PaymentEventsViewSQL is a SQLite-portable copy of the genesis
// payment_events view (schema/genesis/current_schema.sql). The database
// package pins its output columns against the genesis definition.
const PaymentEventsViewSQL = `
CREATE VIEW payment_events AS
SELECT
    p.id AS id,
    'payments' AS source_table,
    p.bill_id AS bill_id,
    b.business_id AS business_id,
    COALESCE(p.payment_method, '') AS method,
    COALESCE(p.status, '') AS status,
    COALESCE(p.amount, 0) AS amount_cents,
    COALESCE(p.tip_amount, 0) AS tip_cents,
    '' AS currency,
    COALESCE(p.payer_addr, '') AS payer_ref,
    COALESCE(p.tx_hash, '') AS external_ref,
    p.created_at AS created_at,
    p.updated_at AS updated_at
FROM payments p
INNER JOIN bills b ON b.id = p.bill_id
UNION ALL
SELECT
    ap.id AS id,
    'alternative_payments' AS source_table,
    ap.bill_id AS bill_id,
    b.business_id AS business_id,
    COALESCE(ap.payment_method, '') AS method,
    COALESCE(ap.status, '') AS status,
    COALESCE(ap.amount, 0) AS amount_cents,
    COALESCE(ap.tip_amount_cents, 0) AS tip_cents,
    '' AS currency,
    COALESCE(NULLIF(ap.participant_name, ''), ap.participant_addr, '') AS payer_ref,
    COALESCE(ap.idempotency_key, '') AS external_ref,
    ap.created_at AS created_at,
    ap.updated_at AS updated_at
FROM alternative_payments ap
INNER JOIN bills b ON b.id = ap.bill_id
`

// EnsurePaymentEventsView installs the payment_events view on a test
// database. It drops any existing view first because SQLite has no
// CREATE OR REPLACE VIEW, and adds the covering index genesis declares.
func EnsurePaymentEventsView(db *gorm.DB) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	if err := db.Exec(`DROP VIEW IF EXISTS payment_events`).Error; err != nil {
		return err
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_alt_payments_bill_created_at ON alternative_payments (bill_id, created_at)`).Error; err != nil {
		return err
	}
	return db.Exec(PaymentEventsViewSQL).Error
}
