package database

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MissingTranslation is the storage row for the IMP-31 telemetry beacon. The
// frontend already dedups per (locale, key) per session; this table dedups
// across sessions so an under-translated screen produces one row per page.
type MissingTranslation struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	Locale          string     `gorm:"type:varchar(32);not null;uniqueIndex:idx_missing_translations_unique,priority:1;index:idx_missing_translations_locale_key,priority:1" json:"locale"`
	KeyPath         string     `gorm:"type:varchar(255);not null;uniqueIndex:idx_missing_translations_unique,priority:2;index:idx_missing_translations_locale_key,priority:2" json:"key_path"`
	Page            string     `gorm:"type:varchar(500);not null;default:'';uniqueIndex:idx_missing_translations_unique,priority:3" json:"page"`
	FallbackUsed    string     `gorm:"type:varchar(16);not null;default:'leaf';uniqueIndex:idx_missing_translations_unique,priority:4" json:"fallback_used"`
	OccurrenceCount int64      `gorm:"not null;default:1" json:"occurrence_count"`
	FirstSeenAt     time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"first_seen_at"`
	LastSeenAt      time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP;index:idx_missing_translations_last_seen,sort:desc;index:idx_missing_translations_status_last_seen,priority:2,sort:desc" json:"last_seen_at"`
	Status          string     `gorm:"type:varchar(16);not null;default:'open';index:idx_missing_translations_status_last_seen,priority:1" json:"status"`
	StatusUpdatedAt *time.Time `json:"status_updated_at"`
}

const (
	MissingTranslationStatusOpen     = "open"
	MissingTranslationStatusResolved = "resolved"
	MissingTranslationStatusIgnored  = "ignored"
)

func IsValidMissingTranslationStatus(status string) bool {
	switch status {
	case MissingTranslationStatusOpen, MissingTranslationStatusResolved, MissingTranslationStatusIgnored:
		return true
	default:
		return false
	}
}

// TableName pins the table name so GORM's pluraliser doesn't drift across
// installs.
func (MissingTranslation) TableName() string { return "missing_translations" }

// RecordMissingTranslation upserts a single (locale, key_path, page,
// fallback_used) tuple. The "first row wins, subsequent rows increment"
// shape lets us aggregate without the caller having to pre-check existence.
//
// We do an UPDATE first, then INSERT-with-OnConflict-DoNothing as a race
// guard. This pattern is dialect-portable (works on Postgres and the
// SQLite in-memory test DB) and never produces duplicate rows because the
// uniqueIndex constraint protects against the narrow race window.
func RecordMissingTranslation(ctx context.Context, entry *MissingTranslation) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	if entry == nil {
		return nil
	}

	now := time.Now()

	res := db.WithContext(ctx).
		Model(&MissingTranslation{}).
		Where(
			"locale = ? AND key_path = ? AND page = ? AND fallback_used = ?",
			entry.Locale, entry.KeyPath, entry.Page, entry.FallbackUsed,
		).
		Updates(map[string]interface{}{
			"occurrence_count": gorm.Expr("occurrence_count + 1"),
			"last_seen_at":     now,
			"status": gorm.Expr(
				"CASE WHEN status = ? THEN ? ELSE status END",
				MissingTranslationStatusResolved,
				MissingTranslationStatusOpen,
			),
			"status_updated_at": gorm.Expr(
				"CASE WHEN status = ? THEN NULL ELSE status_updated_at END",
				MissingTranslationStatusResolved,
			),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		return nil
	}

	entry.OccurrenceCount = 1
	entry.FirstSeenAt = now
	entry.LastSeenAt = now
	entry.Status = MissingTranslationStatusOpen
	entry.StatusUpdatedAt = nil

	return db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "locale"},
				{Name: "key_path"},
				{Name: "page"},
				{Name: "fallback_used"},
			},
			DoNothing: true,
		}).
		Create(entry).Error
}

// ListMissingTranslations returns the top-N rows for the admin view, ordered
// by recency. Optional locale and lifecycle-status filters narrow the result.
func ListMissingTranslations(ctx context.Context, limit, offset int, locale, status string) ([]*MissingTranslation, int64, error) {
	if db == nil {
		return nil, 0, gorm.ErrInvalidDB
	}

	query := db.WithContext(ctx).Model(&MissingTranslation{}).Order("last_seen_at DESC")
	if locale != "" {
		query = query.Where("locale = ?", locale)
	}
	if IsValidMissingTranslationStatus(status) {
		query = query.Where("status = ?", status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []*MissingTranslation
	if err := query.Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}

	return rows, total, nil
}

// UpdateMissingTranslationStatus applies an admin lifecycle decision and
// returns the updated row. Every explicit decision records when it happened.
func UpdateMissingTranslationStatus(ctx context.Context, id uint, status string) (*MissingTranslation, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}

	now := time.Now()
	result := db.WithContext(ctx).
		Model(&MissingTranslation{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":            status,
			"status_updated_at": now,
		})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	var row MissingTranslation
	if err := db.WithContext(ctx).First(&row, id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}
