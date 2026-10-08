package database

import (
	"bytes"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MarketingCrop captures the exact image framing used by a marketing creative.
type MarketingCrop struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Zoom float64 `json:"zoom"`
}

// MarketingCreativeSnapshot is the exact operator-approved creative persisted
// with an activity row. Validation belongs at the HTTP boundary; this database
// type is responsible only for lossless JSONB round trips. ImageSource records
// UI creative provenance (the source selected in the composer); it is not
// authoritative evidence that an image was AI-generated. AI-generation
// compliance provenance, when recorded, remains in ai_generated_images and S3
// object metadata. Those writes are best-effort, so this field and URL shape
// must not be used as substitutes for generation-provenance verification.
type MarketingCreativeSnapshot struct {
	Caption     string            `json:"caption"`
	ImageURL    string            `json:"image_url"`
	ImageSource string            `json:"image_source"`
	Template    string            `json:"template"`
	Aspect      string            `json:"aspect"`
	Slots       map[string]string `json:"slots"`
	Crop        MarketingCrop     `json:"crop"`
	FontFamily  string            `json:"font_family,omitempty"`
	Kit         string            `json:"kit,omitempty"`
	Composition string            `json:"composition,omitempty"`
	Treatment   string            `json:"treatment,omitempty"`
	// Wave 3 motion. Absent on every snapshot written before video export, which
	// is why both are optional: absent MediaKind means a still image, and the
	// row renders exactly as it always did. MediaKind is a closed backend
	// concept ("" | image | video) and is whitelisted; MotionPreset is a
	// frontend-owned id and is only length- and charset-bounded, matching how
	// Composition and Treatment are handled.
	MediaKind    string `json:"media_kind,omitempty"`
	MotionPreset string `json:"motion_preset,omitempty"`
	// Wave 4 campaign kit. `Aspect` remains the HERO format; this is the
	// coordinated set the operator exported alongside it.
	//
	// `omitempty` is load-bearing, not cosmetic: it keeps a legacy row's JSON
	// byte-identical to what it was before this field existed, and it is what
	// makes ABSENT (legacy, single format) distinguishable from PRESENT-BUT-EMPTY.
	// A nil slice therefore means "not a campaign", never "a campaign of nothing".
	//
	// This is a jsonb document field, so adding it needs no migration.
	KitFormats []string `json:"kit_formats,omitempty"`
	// Season 1 destination pack id (ig_feed, tiktok, …). Absent on every
	// snapshot written before destination packs. Frontend-owned; length- and
	// charset-bounded at the HTTP boundary like composition. omitempty keeps
	// legacy rows byte-identical when empty.
	DestinationID string `json:"destination_id,omitempty"`
	// S3-Loop: optional freeform channel the operator typed when confirming
	// mark_posted (e.g. "Instagram Stories", "WhatsApp group"). Not a scheduler
	// field and not a closed enum — Director surfaces it for recent-posts context.
	// Absent on every pre-S3-Loop snapshot; omitempty keeps legacy rows identical.
	PostedChannel string `json:"posted_channel,omitempty"`
	// S3-Reach: short promo / correlation code stamped on print & destination
	// pack QR payloads (e.g. PV-HH-A3F2). Not a coupon engine; not multi-touch
	// ads attribution. Frontend-owned; length- and charset-bounded at the HTTP
	// boundary. omitempty keeps legacy rows byte-identical when empty.
	PromoCode string `json:"promo_code,omitempty"`
	// S3-Reach: print-shop preset id (table_tent_5x7, window_clings_square, …).
	// Charset-bounded like destination_id — not a closed whitelist. Absent on
	// every pre-S3-Reach snapshot.
	PrintPreset string `json:"print_preset,omitempty"`
}

// Scan accepts PostgreSQL JSONB ([]byte), SQLite JSON (string), and SQL NULL.
func (s *MarketingCreativeSnapshot) Scan(value any) error {
	if s == nil {
		return fmt.Errorf("database.MarketingCreativeSnapshot: Scan on nil pointer")
	}
	if value == nil {
		*s = MarketingCreativeSnapshot{}
		return nil
	}

	var data []byte
	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("database.MarketingCreativeSnapshot: cannot scan %T", value)
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("database.MarketingCreativeSnapshot: JSON null is not a snapshot")
	}
	var decoded MarketingCreativeSnapshot
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("database.MarketingCreativeSnapshot: decode JSON: %w", err)
	}
	*s = decoded
	return nil
}

// Value emits JSON for JSONB/TEXT drivers. A nil pointer maps to SQL NULL so
// legacy activity rows remain distinguishable from captured creatives.
func (s *MarketingCreativeSnapshot) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	data, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("database.MarketingCreativeSnapshot: encode JSON: %w", err)
	}
	return data, nil
}

var (
	_ sql.Scanner   = (*MarketingCreativeSnapshot)(nil)
	_ driver.Valuer = (*MarketingCreativeSnapshot)(nil)
)

// MarketingActivity is one durable row per (business, suggestion). A row
// transitions status (posted <-> dismissed) rather than duplicating. It is
// SQL-only (defined in the genesis schema)
// and is intentionally NOT in the AutoMigrate list.
type MarketingActivity struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// The (business_id, suggestion_id) unique index is the upsert arbiter for
	// RecordMarketingActivity's ON CONFLICT. Its name is pinned to migration
	// 000144's idx_marketing_activities_biz_suggestion so the AutoMigrate safety
	// net recognizes the existing SQL index instead of creating a duplicate.
	BusinessID   uint   `gorm:"column:business_id;uniqueIndex:idx_marketing_activities_biz_suggestion,priority:1" json:"business_id"`
	SuggestionID string `gorm:"column:suggestion_id;uniqueIndex:idx_marketing_activities_biz_suggestion,priority:2" json:"suggestion_id"`
	Play         string `gorm:"column:play" json:"play"`
	Title        string `gorm:"column:title" json:"title"`
	TargetName   string `gorm:"column:target_name" json:"target_name"`
	// Status: posted | dismissed | ready | approved (S2-E handoff; no schedule).
	Status           string                     `gorm:"column:status" json:"status"`
	ImageURL         string                     `gorm:"column:image_url" json:"image_url"`
	Caption          string                     `gorm:"column:caption" json:"caption"`
	CreativeSnapshot *MarketingCreativeSnapshot `gorm:"column:creative_snapshot;type:jsonb" json:"creative_snapshot"`
	PostedAt         *time.Time                 `gorm:"column:posted_at" json:"posted_at,omitempty"`
	DismissedAt      *time.Time                 `gorm:"column:dismissed_at" json:"dismissed_at,omitempty"`
	CreatedBy        string                     `gorm:"column:created_by" json:"created_by"`
	CreatedAt        time.Time                  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt        time.Time                  `gorm:"column:updated_at" json:"updated_at"`
}

// TableName pins the table so GORM does not pluralize to marketing_activities
// differently. (Default pluralization already yields this, but pin for safety.)
func (MarketingActivity) TableName() string { return "marketing_activities" }

// marketingActivityMaxPerPage bounds the Library list result set.
const marketingActivityMaxPerPage = 50

// MarketingActivityQuery is the filter/pagination input for the Library list.
type MarketingActivityQuery struct {
	BusinessID  uint
	Status      string // "", "posted", "dismissed"
	Play        string // "", or a play key
	From        *time.Time
	To          *time.Time
	HandledFrom *time.Time
	Page        int
	PerPage     int
}

// Normalize clamps page/per_page to valid, bounded ranges.
func (q MarketingActivityQuery) Normalize() MarketingActivityQuery {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PerPage < 1 {
		q.PerPage = 20
	}
	if q.PerPage > marketingActivityMaxPerPage {
		q.PerPage = marketingActivityMaxPerPage
	}
	return q
}

func (q MarketingActivityQuery) offset() int { return (q.Page - 1) * q.PerPage }

// RecordMarketingActivity upserts one activity row keyed by
// (business_id, suggestion_id). It stamps posted_at / dismissed_at from status
// and returns the persisted row (with its stable id).
//
// Handoff statuses ready / approved (S2-E) do not touch posted_at or
// dismissed_at — they are review states with no clock.
func (d *DB) RecordMarketingActivity(in MarketingActivity) (MarketingActivity, error) {
	now := time.Now().UTC()
	if in.CreatedAt.IsZero() {
		in.CreatedAt = now
	}
	in.UpdatedAt = now
	switch in.Status {
	case "posted":
		in.PostedAt = &now
	case "dismissed":
		in.DismissedAt = &now
	case "ready", "approved":
		// Review handoff only — no lifecycle timestamps.
	}
	// Upsert on the (business_id, suggestion_id) unique index. Snapshot and
	// posted_at use atomic SQL COALESCE assignments so a later dismissal, whose
	// excluded values are NULL, cannot erase the exact posted creative or its
	// historical posting time. A never-posted dismissal still inserts NULLs.
	updates := clause.AssignmentColumns([]string{
		"play", "title", "target_name", "status", "image_url", "caption",
		"dismissed_at", "created_by", "updated_at",
	})
	updates = append(updates,
		clause.Assignment{
			Column: clause.Column{Name: "creative_snapshot"},
			Value:  clause.Expr{SQL: "COALESCE(excluded.creative_snapshot, marketing_activities.creative_snapshot)"},
		},
		clause.Assignment{
			Column: clause.Column{Name: "posted_at"},
			Value:  clause.Expr{SQL: "COALESCE(excluded.posted_at, marketing_activities.posted_at)"},
		},
	)
	err := d.GetGorm().Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "business_id"}, {Name: "suggestion_id"}},
		DoUpdates: updates,
	}).Create(&in).Error
	if err != nil {
		return MarketingActivity{}, err
	}
	// Re-read to return the canonical row (id + any conflict-updated fields).
	var out MarketingActivity
	if err := d.GetGorm().
		Where("business_id = ? AND suggestion_id = ?", in.BusinessID, in.SuggestionID).
		First(&out).Error; err != nil {
		return MarketingActivity{}, err
	}
	return out, nil
}

// EnsureInventoryHiddenMarketingActivity records an inventory-blocked
// suggestion as dismissed so Historial Ocultos can recover it. Existing rows
// (posted / ready / already dismissed) are left untouched.
func (d *DB) EnsureInventoryHiddenMarketingActivity(in MarketingActivity) error {
	if d == nil || d.GetGorm() == nil {
		return fmt.Errorf("database is not initialized")
	}
	if strings.TrimSpace(in.SuggestionID) == "" || in.BusinessID == 0 {
		return nil
	}
	var existing MarketingActivity
	err := d.GetGorm().
		Select("id").
		Where("business_id = ? AND suggestion_id = ?", in.BusinessID, in.SuggestionID).
		First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	in.Status = "dismissed"
	in.CreatedBy = strings.TrimSpace(in.CreatedBy)
	if in.CreatedBy == "" {
		in.CreatedBy = "inventory"
	}
	_, err = d.RecordMarketingActivity(in)
	return err
}

// RestoreMarketingActivity restores a dismissed suggestion without erasing
// posted history. A dismissed row with posted_at is atomically transitioned
// back to posted and has only dismissed_at cleared; its original posted_at,
// creative snapshot, image, and caption remain unchanged, so the normal posted
// cooldown still applies. A never-posted dismissal (posted_at IS NULL) is
// deleted so it can return to the live feed. A currently posted row is a no-op.
// Conditional predicates make a concurrent or repeated second call harmless.
func (d *DB) RestoreMarketingActivity(businessID uint, suggestionID string) (bool, error) {
	now := time.Now().UTC()
	revived := d.GetGorm().Model(&MarketingActivity{}).
		Where("business_id = ? AND suggestion_id = ? AND status = ? AND posted_at IS NOT NULL", businessID, suggestionID, "dismissed").
		Updates(map[string]any{
			"status":       "posted",
			"dismissed_at": nil,
			"updated_at":   now,
		})
	if revived.Error != nil {
		return false, revived.Error
	}
	if revived.RowsAffected > 0 {
		return true, nil
	}

	deleted := d.GetGorm().
		Where("business_id = ? AND suggestion_id = ? AND status = ? AND posted_at IS NULL", businessID, suggestionID, "dismissed").
		Delete(&MarketingActivity{})
	if deleted.Error != nil {
		return false, deleted.Error
	}
	return deleted.RowsAffected > 0, nil
}

// ListMarketingActivities returns a bounded, newest-first, filterable page plus
// the total count. Explicit projection (no SELECT *); single count + single list
// query.
func (d *DB) ListMarketingActivities(q MarketingActivityQuery) ([]MarketingActivity, int64, error) {
	q = q.Normalize()
	base := d.GetGorm().Model(&MarketingActivity{}).Where("business_id = ?", q.BusinessID)
	if q.Status != "" {
		base = base.Where("status = ?", q.Status)
	}
	if q.Play != "" {
		base = base.Where("play = ?", q.Play)
	}
	if q.From != nil {
		base = base.Where("created_at >= ?", *q.From)
	}
	if q.To != nil {
		base = base.Where("created_at <= ?", *q.To)
	}
	if q.HandledFrom != nil {
		base = base.Where(`(
			(status = ? AND (posted_at >= ? OR updated_at >= ?))
			OR (status = ? AND (dismissed_at >= ? OR updated_at >= ?))
		)`,
			"posted", *q.HandledFrom, *q.HandledFrom,
			"dismissed", *q.HandledFrom, *q.HandledFrom,
		)
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []MarketingActivity
	if err := base.
		Select("id", "business_id", "suggestion_id", "play", "title", "target_name",
			"status", "image_url", "caption", "creative_snapshot", "posted_at", "dismissed_at", "created_by",
			"created_at", "updated_at").
		Order("created_at DESC").Order("id DESC").
		Limit(q.PerPage).Offset(q.offset()).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// HandledMarketingActivity is the narrow lifecycle projection consumed by the
// suggestion engine. PostedAt is required to apply the posted cooldown without
// treating a post as a durable dismissal.
type HandledMarketingActivity struct {
	SuggestionID string     `gorm:"column:suggestion_id"`
	Status       string     `gorm:"column:status"`
	PostedAt     *time.Time `gorm:"column:posted_at"`
}

// HandledMarketingActivities returns durable dismissals, in-flight handoff
// rows (ready / approved), plus posts newer than postedAfter for the engine's
// bounded candidate ID set. The strict posted_at comparison makes a post
// eligible again at the exact cooldown boundary. Ready/approved hide the
// suggestion from the live feed without starting the 30-day cooldown.
func (d *DB) HandledMarketingActivities(businessID uint, suggestionIDs []string, postedAfter time.Time) ([]HandledMarketingActivity, error) {
	rows := make([]HandledMarketingActivity, 0)
	if len(suggestionIDs) == 0 {
		return rows, nil
	}
	if err := d.GetGorm().Model(&MarketingActivity{}).
		Select("suggestion_id", "status", "posted_at").
		Where("business_id = ? AND suggestion_id IN ?", businessID, suggestionIDs).
		Where(
			"status IN ? OR (status = ? AND posted_at > ?)",
			[]string{"dismissed", "ready", "approved"},
			"posted",
			postedAfter,
		).
		Order("updated_at DESC").Order("id DESC").
		Limit(len(suggestionIDs)).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// MarketingPostSummary is one recently posted marketing creative for Director
// closed-loop surfaces (count + titles + optional freeform channel tags).
// No schedule fields — only what the operator recorded via mark_posted.
type MarketingPostSummary struct {
	Title         string     `json:"title"`
	Play          string     `json:"play"`
	PostedChannel string     `json:"posted_channel,omitempty"`
	DestinationID string     `json:"destination_id,omitempty"`
	PostedAt      *time.Time `json:"posted_at,omitempty"`
}

// MarketingPostsPeriod is the Director-facing aggregate for posts in a window.
type MarketingPostsPeriod struct {
	Count        int64                  `json:"count"`
	PeriodDays   int                    `json:"period_days"`
	Recent       []MarketingPostSummary `json:"recent"`
	Channels     []string               `json:"channels"`
	RecentTitles []string               `json:"recent_titles"`
}

// RecentMarketingPosts returns posted activities with posted_at >= since,
// newest first, capped at limit (default 5, max 10). Channels prefer the
// freeform posted_channel, falling back to destination_id when empty.
// Failures return a zero summary rather than erroring the Director front door.
func (d *DB) RecentMarketingPosts(businessID uint, since time.Time, limit int) (MarketingPostsPeriod, error) {
	if limit <= 0 {
		limit = 5
	}
	if limit > 10 {
		limit = 10
	}
	out := MarketingPostsPeriod{
		PeriodDays:   int(time.Since(since).Hours()/24 + 0.5),
		Recent:       []MarketingPostSummary{},
		Channels:     []string{},
		RecentTitles: []string{},
	}
	if out.PeriodDays < 1 {
		out.PeriodDays = 1
	}

	var total int64
	if err := d.GetGorm().Model(&MarketingActivity{}).
		Where("business_id = ? AND status = ? AND posted_at IS NOT NULL AND posted_at >= ?",
			businessID, "posted", since).
		Count(&total).Error; err != nil {
		return out, err
	}
	out.Count = total
	if total == 0 {
		return out, nil
	}

	var rows []MarketingActivity
	if err := d.GetGorm().Model(&MarketingActivity{}).
		Select("id", "title", "play", "creative_snapshot", "posted_at").
		Where("business_id = ? AND status = ? AND posted_at IS NOT NULL AND posted_at >= ?",
			businessID, "posted", since).
		Order("posted_at DESC").Order("id DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return out, err
	}

	channelSeen := make(map[string]struct{})
	for _, row := range rows {
		summary := MarketingPostSummary{
			Title:    row.Title,
			Play:     row.Play,
			PostedAt: row.PostedAt,
		}
		if row.CreativeSnapshot != nil {
			summary.PostedChannel = strings.TrimSpace(row.CreativeSnapshot.PostedChannel)
			summary.DestinationID = strings.TrimSpace(row.CreativeSnapshot.DestinationID)
		}
		out.Recent = append(out.Recent, summary)
		if row.Title != "" && len(out.RecentTitles) < 3 {
			out.RecentTitles = append(out.RecentTitles, row.Title)
		}
		channel := summary.PostedChannel
		if channel == "" {
			channel = summary.DestinationID
		}
		if channel == "" {
			continue
		}
		if _, ok := channelSeen[channel]; ok {
			continue
		}
		channelSeen[channel] = struct{}{}
		if len(out.Channels) < 5 {
			out.Channels = append(out.Channels, channel)
		}
	}
	return out, nil
}
