package database

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrBusinessCreationIdempotencyConflict = errors.New("business creation idempotency payload conflict")

// BusinessCreationRequest is a durable, principal-scoped ledger for workspace
// creation. Only hashes are stored: raw idempotency keys never enter the DB.
// The ledger row and workspace commit in one transaction, closing the
// response-loss window where a retry could otherwise insert a second business.
type BusinessCreationRequest struct {
	ID                 uint      `gorm:"primaryKey"`
	OwnerScopeHash     string    `gorm:"size:64;not null;uniqueIndex:idx_business_creation_owner_key,priority:1"`
	IdempotencyKeyHash string    `gorm:"size:64;not null;uniqueIndex:idx_business_creation_owner_key,priority:2"`
	PayloadHash        string    `gorm:"size:64;not null"`
	BusinessID         *uint     `gorm:"index"`
	CreatedAt          time.Time `gorm:"not null"`
	UpdatedAt          time.Time `gorm:"not null"`
}

func (BusinessCreationRequest) TableName() string {
	return "business_creation_requests"
}

// CreateBusinessIdempotently either creates business and its request ledger
// atomically or returns the business committed by an exact replay. A key reused
// with a different canonical payload is rejected.
func CreateBusinessIdempotently(
	business *Business,
	ownerScopeHash string,
	idempotencyKeyHash string,
	payloadHash string,
) (*Business, bool, error) {
	if ownerScopeHash == "" || idempotencyKeyHash == "" || payloadHash == "" {
		return nil, false, errors.New("business creation idempotency hashes are required")
	}

	var resolved *Business
	var replay bool
	err := db.Transaction(func(tx *gorm.DB) error {
		claim := BusinessCreationRequest{
			OwnerScopeHash:     ownerScopeHash,
			IdempotencyKeyHash: idempotencyKeyHash,
			PayloadHash:        payloadHash,
		}
		insert := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "owner_scope_hash"},
				{Name: "idempotency_key_hash"},
			},
			DoNothing: true,
		}).Create(&claim)
		if insert.Error != nil {
			return fmt.Errorf("claim business creation request: %w", insert.Error)
		}

		if insert.RowsAffected == 0 {
			var existing BusinessCreationRequest
			if err := tx.Where("owner_scope_hash = ? AND idempotency_key_hash = ?",
				ownerScopeHash, idempotencyKeyHash).First(&existing).Error; err != nil {
				return fmt.Errorf("load business creation request: %w", err)
			}
			if existing.PayloadHash != payloadHash {
				return ErrBusinessCreationIdempotencyConflict
			}
			if existing.BusinessID == nil || *existing.BusinessID == 0 {
				return errors.New("business creation request committed without a workspace")
			}
			var original Business
			if err := tx.First(&original, *existing.BusinessID).Error; err != nil {
				return fmt.Errorf("load idempotent business creation result: %w", err)
			}
			resolved = &original
			replay = true
			return nil
		}

		applyCreateBusinessDefaults(business)
		if err := assignCreateCustomURLTx(tx, business); err != nil {
			return err
		}
		if err := tx.Create(business).Error; err != nil {
			return fmt.Errorf("failed to create business: %w", err)
		}
		if err := ensureBusinessRevenueAggregateRowTx(tx, business.ID); err != nil {
			return fmt.Errorf("failed to create business revenue aggregate: %w", err)
		}
		if err := tx.Model(&BusinessCreationRequest{}).
			Where("id = ?", claim.ID).
			Updates(map[string]interface{}{
				"business_id": business.ID,
				"updated_at":  time.Now(),
			}).Error; err != nil {
			return fmt.Errorf("complete business creation request: %w", err)
		}
		resolved = business
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return resolved, replay, nil
}
