package database

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// Guest fiscal identity payer binding (M-545).
//
// The public POST /guest/bill/:bill_token/fiscal-customer is reachable by
// anyone holding the table QR / bill token. The identity it writes becomes the
// factura receptor, so it is bound to the guest browser session that paid:
//
//   - bills.fiscal_customer_guest_session records which guest session set the
//     identity (NULL = operator-set or legacy, which guests cannot replace);
//   - payments.payer_guest_session and alternative_payments.payer_guest_session
//     record which guest session initiated each guest payment.
//
// All three hold guestsession.Fingerprint values, never raw session ids.

// GuestPaymentProof ranks the payment evidence one guest session holds on one
// bill. A later guest write may replace a guest-set identity only with strictly
// stronger proof than the session that set it.
type GuestPaymentProof int

const (
	// GuestPaymentProofNone: the session has not started paying this bill.
	GuestPaymentProofNone GuestPaymentProof = 0
	// GuestPaymentProofPending: the session has an open (pending, unexpired)
	// tender request, provider checkout or payment on this bill.
	GuestPaymentProofPending GuestPaymentProof = 1
	// GuestPaymentProofConfirmed: money from the session has settled.
	GuestPaymentProofConfirmed GuestPaymentProof = 2
)

// guestConfirmedPaymentExistsSQL is true when a confirmed payment on the bill
// was initiated by a guest session matching %s. A plugin checkout tracker
// (alternative_payments, participant_addr = provider payment id) counts as
// confirmed as soon as its settled payments row ("plugin_" || id) is
// confirmed: the tracker row itself is flipped only after the settlement
// transaction commits, after the fiscal job is already enqueued. That branch
// is a join (not a nested EXISTS under OR) so Postgres probes the unique
// idx_payments_tx_hash per tracker instead of hashing every confirmed payment.
const guestConfirmedPaymentExistsSQL = `(EXISTS (SELECT 1 FROM payments gp
	WHERE gp.bill_id = %[1]s AND gp.status = 'confirmed' AND gp.payer_guest_session %[2]s)
OR EXISTS (SELECT 1 FROM alternative_payments gap
	WHERE gap.bill_id = %[1]s AND gap.payer_guest_session %[2]s AND gap.status = 'confirmed')
OR EXISTS (SELECT 1 FROM alternative_payments gat
	JOIN payments gpp ON gpp.tx_hash = 'plugin_' || gat.participant_addr
	WHERE gat.bill_id = %[1]s AND gat.payer_guest_session %[2]s
	AND gpp.bill_id = gat.bill_id AND gpp.status = 'confirmed'))`

func guestConfirmedPaymentExists(billRef, sessionPredicate string) string {
	return strings.NewReplacer("%[1]s", billRef, "%[2]s", sessionPredicate).Replace(guestConfirmedPaymentExistsSQL)
}

var (
	guestProofConfirmedSQL = guestConfirmedPaymentExists("@bill", "= @fp")
	guestProofSQL          = `SELECT CASE
WHEN ` + guestProofConfirmedSQL + ` THEN 2
WHEN EXISTS (SELECT 1 FROM alternative_payments pap
	WHERE pap.bill_id = @bill AND pap.payer_guest_session = @fp AND pap.status = 'pending'
	AND (pap.expires_at IS NULL OR pap.expires_at > @now))
OR EXISTS (SELECT 1 FROM payments pp2
	WHERE pp2.bill_id = @bill AND pp2.payer_guest_session = @fp AND pp2.status = 'pending')
THEN 1
ELSE 0 END`

	// clearUnpaidGuestIdentityWhere: the identity was set by a guest session
	// and that session has no confirmed payment on the bill. Who else paid
	// (another guest, staff at the counter, a webhook) does not matter: a
	// token holder who never paid must not become the factura receptor.
	clearUnpaidGuestIdentityWhere = `id = @bill AND fiscal_customer_guest_session IS NOT NULL
AND NOT ` + guestConfirmedPaymentExists("bills.id", "= bills.fiscal_customer_guest_session")
)

// GuestPaymentProofForSession returns the strongest payment evidence the guest
// session fingerprint holds on the bill, in one indexed query.
func GuestPaymentProofForSession(db *gorm.DB, billID uint, fingerprint string, now time.Time) (GuestPaymentProof, error) {
	fingerprint = strings.TrimSpace(fingerprint)
	if billID == 0 || fingerprint == "" {
		return GuestPaymentProofNone, nil
	}
	var rank int
	if err := db.Raw(guestProofSQL, map[string]any{
		"bill": billID,
		"fp":   fingerprint,
		"now":  now,
	}).Scan(&rank).Error; err != nil {
		return GuestPaymentProofNone, err
	}
	return GuestPaymentProof(rank), nil
}

// ClearUnpaidGuestFiscalIdentity drops a guest-set fiscal identity from the
// bill unless the session that set it has a confirmed payment on the bill. It
// is the issuance-time guard against a token holder squatting the factura
// receptor: one conditional UPDATE, a no-op (returns false) for operator-set
// identities and identities set by a paying guest. A bill settled only by
// staff-recorded or webhook payments issues without a guest-set identity; the
// operator can still set one through the operator route.
func ClearUnpaidGuestFiscalIdentity(db *gorm.DB, billID uint) (bool, error) {
	if billID == 0 {
		return false, nil
	}
	res := db.Model(&Bill{}).
		Where(clearUnpaidGuestIdentityWhere, map[string]any{"bill": billID}).
		Updates(map[string]any{
			"fiscal_customer_doc_type":      nil,
			"fiscal_customer_doc_number":    nil,
			"fiscal_customer_tax_condition": nil,
			"fiscal_customer_name":          nil,
			"fiscal_customer_email":         nil,
			"fiscal_customer_guest_session": nil,
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}
