package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/session"
)

// Account erasure janitor.
//
// DELETE /inside/account soft-deletes an operator account: it sets
// users.deleted_at and users.deletion_scheduled_at = now + 30d. When the grace
// window ends, this janitor anonymizes the account in place:
//
//   - users: email, wallet address, display name, username, picture,
//     the Google id and the free-text deletion
//     reason are cleared, email_verified is reset, and deleted_at stays set.
//   - user_auths: the login identifiers and secrets (provider_user_id,
//     wallet_address, password_hash, verification/reset tokens and expiries)
//     are cleared, so the account can no longer sign in by any provider.
//   - every operator session linked to the user id or the old wallet address
//     is revoked.
//
// The row ids are kept. Bills, payments, ledgers and businesses reference the
// user id, and owned businesses are deliberately not touched: their records
// are kept for accounting and tax retention.
//
// The pass is idempotent. It only selects due rows that still hold an
// identifier, so an account that has been erased is never picked up again.

const (
	defaultAccountErasureInterval  = time.Hour
	defaultAccountErasureBatchSize = 100
)

// AccountErasureResult reports one janitor pass.
type AccountErasureResult struct {
	UsersAnonymized int
}

// accountErasureCandidate is the narrow projection a pass reads.
type accountErasureCandidate struct {
	ID      uint
	Address *string
}

// RunAccountErasure anonymizes every account whose deletion_scheduled_at is at
// or before now. It drains in batches of batchSize. revoker can be nil, which
// skips session revocation (tests and tools without a session store).
func RunAccountErasure(ctx context.Context, db *gorm.DB, revoker *session.Store, now time.Time, batchSize int) (AccountErasureResult, error) {
	var result AccountErasureResult
	if db == nil {
		return result, fmt.Errorf("account erasure: nil database")
	}
	if batchSize <= 0 {
		batchSize = defaultAccountErasureBatchSize
	}
	for {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		var due []accountErasureCandidate
		if err := db.WithContext(ctx).Model(&database.User{}).
			Select("id", "address").
			Where("deletion_scheduled_at IS NOT NULL AND deletion_scheduled_at <= ?", now).
			Where(`(email IS NOT NULL OR COALESCE(address, '') <> '' OR COALESCE(name, '') <> ''
				OR COALESCE(username, '') <> '' OR COALESCE(picture, '') <> ''
				OR COALESCE(google_id, '') <> ''
				OR COALESCE(deletion_reason, '') <> '')`).
			Order("id ASC").
			Limit(batchSize).
			Find(&due).Error; err != nil {
			return result, fmt.Errorf("account erasure: select due accounts: %w", err)
		}
		if len(due) == 0 {
			return result, nil
		}
		for _, candidate := range due {
			if err := anonymizeAccount(ctx, db, revoker, candidate, now); err != nil {
				return result, err
			}
			result.UsersAnonymized++
		}
		if len(due) < batchSize {
			return result, nil
		}
	}
}

func anonymizeAccount(ctx context.Context, db *gorm.DB, revoker *session.Store, candidate accountErasureCandidate, now time.Time) error {
	// Revoke first. Revocation is idempotent, and the wallet address it needs
	// is cleared by the anonymize below; once that commits the row no longer
	// matches the due selection, so a revocation failure after it would never
	// be retried and wallet-linked sessions could outlive the erasure.
	if revoker != nil {
		var addresses []string
		if candidate.Address != nil && strings.TrimSpace(*candidate.Address) != "" {
			addresses = append(addresses, *candidate.Address)
		}
		if err := revoker.RevokeAllLinkedOperatorSessions(candidate.ID, addresses, session.RevocationReasonAccountDisabled); err != nil {
			return fmt.Errorf("account erasure: revoke sessions for user %d: %w", candidate.ID, err)
		}
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&database.User{}).Where("id = ?", candidate.ID).Updates(map[string]any{
			"email":           nil,
			"address":         nil,
			"name":            nil,
			"username":        nil,
			"picture":         nil,
			"google_id":       nil,
			"deletion_reason": "",
			"email_verified":  false,
			"deleted_at":      gorm.Expr("COALESCE(deleted_at, ?)", now),
			"updated_at":      now,
		}).Error; err != nil {
			return fmt.Errorf("anonymize user %d: %w", candidate.ID, err)
		}
		if err := tx.Table("user_auths").Where("user_id = ?", candidate.ID).Updates(map[string]any{
			"provider_user_id":    nil,
			"wallet_address":      nil,
			"password_hash":       nil,
			"verification_token":  nil,
			"verification_expiry": nil,
			"reset_token":         nil,
			"reset_expiry":        nil,
			"email_verified":      false,
			"updated_at":          now,
		}).Error; err != nil {
			return fmt.Errorf("anonymize user_auths for user %d: %w", candidate.ID, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("account erasure: %w", err)
	}
	return nil
}

// StartAccountErasureJanitor runs RunAccountErasure once at startup and then
// on every interval until ctx is cancelled. Each pass is wrapped in SafeTick,
// so a panic on one bad row is logged and the loop keeps running. The
// returned channel is closed when the loop has exited.
func StartAccountErasureJanitor(ctx context.Context, db *gorm.DB, revoker *session.Store, interval time.Duration) <-chan struct{} {
	if interval <= 0 {
		interval = defaultAccountErasureInterval
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		runAccountErasureTick(ctx, db, revoker)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runAccountErasureTick(ctx, db, revoker)
			}
		}
	}()
	return done
}

func runAccountErasureTick(ctx context.Context, db *gorm.DB, revoker *session.Store) {
	logger.SafeTick("account-erasure-janitor", func() {
		res, err := RunAccountErasure(ctx, db, revoker, time.Now().UTC(), defaultAccountErasureBatchSize)
		if err != nil {
			if ctx.Err() == nil {
				logger.Logger.Warnf("Account erasure janitor pass failed after %d accounts: %v", res.UsersAnonymized, err)
			}
			return
		}
		if res.UsersAnonymized > 0 {
			logger.Logger.Infof("Account erasure janitor anonymized %d accounts past their deletion date", res.UsersAnonymized)
		}
	})
}
