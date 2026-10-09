package database

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GetBusinessIDByPluginPaymentTracker resolves bills.business_id for the newest
// alternative_payments row matching participant_addr + payment_method.
// Used by MercadoPago orders-topic webhooks when business_id is absent from the
// notification URL (application-level webhooks only carry data.id = order id).
func GetBusinessIDByPluginPaymentTracker(participantAddr, paymentMethod string) (uint, error) {
	participantAddr = strings.TrimSpace(participantAddr)
	paymentMethod = strings.TrimSpace(paymentMethod)
	if participantAddr == "" || paymentMethod == "" {
		return 0, gorm.ErrRecordNotFound
	}
	if db == nil {
		return 0, errors.New("database not initialized")
	}

	// P4: expired pending trackers must not resolve business (no settle path).
	// Confirmed trackers still resolve (refunds/lifecycle). Pending only when
	// expires_at is null (legacy) or still in the future.
	now := time.Now().UTC()
	var businessID uint
	err := db.Raw(`
		SELECT b.business_id
		FROM alternative_payments ap
		INNER JOIN bills b ON b.id = ap.bill_id
		WHERE ap.participant_addr = ? AND ap.payment_method = ?
		  AND (
		    ap.status = ?
		    OR (ap.status = ? AND (ap.expires_at IS NULL OR ap.expires_at > ?))
		  )
		ORDER BY ap.id DESC
		LIMIT 1
	`, participantAddr, paymentMethod, AltPaymentStatusConfirmed, AltPaymentStatusPending, now).Scan(&businessID).Error
	if err != nil {
		return 0, err
	}
	if businessID == 0 {
		return 0, gorm.ErrRecordNotFound
	}
	return businessID, nil
}

// ResolvePendingPluginPayment moves one provider-backed tracker out of pending
// and releases any split-share hold linked to it. Confirmed and other terminal
// rows are left untouched so a late cancellation cannot regress captured money.
func ResolvePendingPluginPayment(billID uint, provider, providerPaymentID string, target AlternativePaymentStatus, now time.Time) (bool, error) {
	if billID == 0 || strings.TrimSpace(provider) == "" || strings.TrimSpace(providerPaymentID) == "" {
		return false, nil
	}
	if target != AltPaymentStatusCancelled && target != AltPaymentStatusExpired && target != AltPaymentStatusFailed {
		return false, ErrAlternativePaymentRequestNotPending
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	resolved := false
	err := db.Transaction(func(tx *gorm.DB) error {
		var payment AlternativePayment
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("bill_id = ? AND payment_method = ? AND participant_addr = ?", billID, AlternativePaymentMethod(strings.TrimSpace(provider)), strings.TrimSpace(providerPaymentID)).
			First(&payment).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if payment.Status != AltPaymentStatusPending {
			return nil
		}
		if err := releaseBillSplitShareForAlternativePaymentTx(tx, payment, now); err != nil {
			return err
		}
		result := tx.Model(&AlternativePayment{}).
			Where("id = ? AND status = ?", payment.ID, AltPaymentStatusPending).
			Updates(map[string]any{
				"status":            target,
				"resolved_by":       "plugin_webhook",
				"resolution_reason": "provider reported terminal status",
				"resolved_at":       &now,
				"updated_at":        now,
			})
		if result.Error != nil {
			return result.Error
		}
		resolved = result.RowsAffected == 1
		return nil
	})
	return resolved, err
}
