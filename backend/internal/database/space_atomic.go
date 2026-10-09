package database

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"gorm.io/gorm"
)

// MaterializeLayoutCandidate describes one unlinked table placement to create or reuse.
type MaterializeLayoutCandidate struct {
	// Index in the caller's layout tables slice (for rewriting).
	Index            int
	ClientKey        string
	Name             string
	Shape            string
	Capacity         int
	MinCapacity      *int
	MaxCapacity      *int
	VisibleSeatCount *int
	IsReservable     *bool
	IsCombinable     *bool
	IsAccessible     *bool
	PosXMm           *int
	PosYMm           *int
	WidthMm          *int
	HeightMm         *int
	RotationDeg      float64
	RegionID         *uint
}

// MaterializeResult maps layout table index → real table ID.
type MaterializeResult map[int]uint

// PublishLayoutAtomicInput is the all-or-nothing publish payload.
type PublishLayoutAtomicInput struct {
	BusinessID       uint
	SpaceID          uint
	ExpectedRevision int64
	// DraftLayoutJSON is the layout AFTER candidate table_ids have been rewritten
	// (caller may pass pre-materialized JSON; MaterializeCandidates may still run
	// if CandidateTables is non-empty).
	DraftLayoutJSON JSONRawMessage
	// Boundary/width/height optional updates alongside draft.
	Boundary JSONRawMessage
	WidthMm  *int
	HeightMm *int
	// Candidates to create/reuse inside the same transaction (before draft write).
	Candidates []MaterializeLayoutCandidate
	// RewriteTableIDs is filled by materialize; caller should rewrite layout before
	// setting DraftLayoutJSON. Prefer using PublishSpaceLayoutAtomic which does rewrite
	// when RewriteHook is set.
	Placements []TableLayoutPlacement
}

// PublishLayoutAtomicResult is returned on successful atomic publish.
type PublishLayoutAtomicResult struct {
	Space             *RestaurantSpace
	MaterializedIDs   MaterializeResult
	PublishedRevision int64
	TableCountSynced  int
}

// MaterializeCandidatesTx creates new tables for candidates inside tx.
// It never reuses an existing table by display name (that rebinds QR identity).
// Idempotency comes from CAS: if draft already has real IDs there are no
// candidates; if the transaction rolls back, creates are undone.
// linkedIDs should be seeded with table_ids already present in the layout.
func MaterializeCandidatesTx(
	tx *gorm.DB,
	businessID, spaceID uint,
	candidates []MaterializeLayoutCandidate,
	linkedIDs map[uint]struct{},
) (MaterializeResult, error) {
	out := MaterializeResult{}
	if len(candidates) == 0 {
		return out, nil
	}
	if linkedIDs == nil {
		linkedIDs = map[uint]struct{}{}
	}

	now := time.Now().UTC()
	for _, c := range candidates {
		name := strings.TrimSpace(c.Name)
		if name == "" {
			name = fmt.Sprintf("T%02d", c.Index+1)
		}
		// Optional: if client_key is non-empty and matches a reserved mapping
		// we only ever create new rows — client_key is for layout-side identity
		// across retries after a successful draft write (no candidates remain).

		code, err := generateUniqueTableCodeTx(tx, 10)
		if err != nil {
			return nil, err
		}
		cap := c.Capacity
		if cap <= 0 {
			cap = 4
		}
		row := &Table{
			BusinessID:    businessID,
			Name:          name,
			TableCode:     code,
			Capacity:      cap,
			IsActive:      true,
			CreatedAt:     now,
			UpdatedAt:     now,
			SpaceID:       &spaceID,
			RegionID:      c.RegionID,
			PosXMm:        c.PosXMm,
			PosYMm:        c.PosYMm,
			WidthMm:       c.WidthMm,
			HeightMm:      c.HeightMm,
			RotationDeg:   c.RotationDeg,
			Shape:         c.Shape,
			LayoutInDraft: true,
		}
		if row.Shape == "" {
			row.Shape = TableShapeRectangle
		}
		if c.MinCapacity != nil {
			row.MinCapacity = c.MinCapacity
		}
		if c.MaxCapacity != nil {
			row.MaxCapacity = c.MaxCapacity
		} else {
			mc := cap
			row.MaxCapacity = &mc
		}
		if c.VisibleSeatCount != nil {
			row.VisibleSeatCount = c.VisibleSeatCount
		}
		if c.IsReservable != nil {
			row.IsReservable = *c.IsReservable
		} else {
			row.IsReservable = true
		}
		if c.IsCombinable != nil {
			row.IsCombinable = *c.IsCombinable
		}
		if c.IsAccessible != nil {
			row.IsAccessible = *c.IsAccessible
		}
		if err := tx.Create(row).Error; err != nil {
			return nil, fmt.Errorf("create candidate table %q: %w", name, err)
		}
		out[c.Index] = row.ID
		linkedIDs[row.ID] = struct{}{}
	}
	return out, nil
}

func generateUniqueTableCodeTx(tx *gorm.DB, length int) (string, error) {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	for attempts := 0; attempts < 16; attempts++ {
		buf := make([]byte, length)
		max := big.NewInt(int64(len(charset)))
		for i := range buf {
			n, err := rand.Int(rand.Reader, max)
			if err != nil {
				return "", err
			}
			buf[i] = charset[n.Int64()]
		}
		code := string(buf)
		var existing Table
		err := tx.Where("table_code = ?", code).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return code, nil
		}
		if err != nil {
			return "", err
		}
	}
	// Fallback unique enough for tests.
	raw := make([]byte, 8)
	_, _ = rand.Read(raw)
	return strings.ToUpper(hex.EncodeToString(raw))[:10], nil
}

// assertPlacementsInSpaceTx rejects placements that reference tables owned by
// a different space (cross-space reassignment via layout JSON).
func assertPlacementsInSpaceTx(tx *gorm.DB, businessID, spaceID uint, placements []TableLayoutPlacement) error {
	for _, p := range placements {
		if p.TableID == 0 {
			continue
		}
		var t Table
		if err := tx.Select("id", "business_id", "space_id").
			Where("id = ? AND business_id = ?", p.TableID, businessID).
			First(&t).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("table %d not found in business %d", p.TableID, businessID)
			}
			return err
		}
		if t.SpaceID != nil && *t.SpaceID != 0 && *t.SpaceID != spaceID {
			return fmt.Errorf("table %d belongs to space %d, not space %d", p.TableID, *t.SpaceID, spaceID)
		}
	}
	return nil
}

// SyncPublishedTablePositionsTx is the transactional form of SyncPublishedTablePositions.
func SyncPublishedTablePositionsTx(tx *gorm.DB, businessID, spaceID uint, placements []TableLayoutPlacement) error {
	now := time.Now().UTC()
	if err := tx.Model(&Table{}).
		Where("business_id = ? AND space_id = ?", businessID, spaceID).
		Updates(map[string]interface{}{
			"layout_published": false,
			"layout_in_draft":  false,
			"updated_at":       now,
		}).Error; err != nil {
		return err
	}
	for _, p := range placements {
		if p.TableID == 0 {
			continue
		}
		fields := map[string]interface{}{
			"space_id":           spaceID,
			"region_id":          p.RegionID,
			"pos_x_mm":           p.PosXMm,
			"pos_y_mm":           p.PosYMm,
			"width_mm":           p.WidthMm,
			"height_mm":          p.HeightMm,
			"rotation_deg":       p.RotationDeg,
			"shape":              p.Shape,
			"min_capacity":       p.MinCapacity,
			"max_capacity":       p.MaxCapacity,
			"visible_seat_count": p.VisibleSeatCount,
			"layout_published":   true,
			"layout_in_draft":    false,
			"updated_at":         now,
		}
		if p.Capacity != nil && *p.Capacity > 0 {
			fields["capacity"] = *p.Capacity
		} else if p.MaxCapacity != nil && *p.MaxCapacity > 0 {
			fields["capacity"] = *p.MaxCapacity
		} else if p.MinCapacity != nil && *p.MinCapacity > 0 {
			fields["capacity"] = *p.MinCapacity
		}
		if p.IsReservable != nil {
			fields["is_reservable"] = *p.IsReservable
		}
		if p.IsCombinable != nil {
			fields["is_combinable"] = *p.IsCombinable
		}
		if p.IsAccessible != nil {
			fields["is_accessible"] = *p.IsAccessible
		}
		res := tx.Model(&Table{}).
			Where("id = ? AND business_id = ?", p.TableID, businessID).
			Updates(fields)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("table %d not found in business %d", p.TableID, businessID)
		}
	}
	return nil
}

// PromoteDraftElementsToPublishedTx is the transactional promote helper.
func PromoteDraftElementsToPublishedTx(tx *gorm.DB, businessID, spaceID uint) error {
	if err := tx.Where("business_id = ? AND space_id = ? AND is_draft = ?", businessID, spaceID, false).
		Delete(&SpaceLayoutElement{}).Error; err != nil {
		return err
	}
	now := time.Now().UTC()
	return tx.Model(&SpaceLayoutElement{}).
		Where("business_id = ? AND space_id = ? AND is_draft = ?", businessID, spaceID, true).
		Updates(map[string]interface{}{
			"is_draft":   false,
			"updated_at": now,
		}).Error
}

// PublishSpaceLayoutAtomic materializes candidates, writes draft, publishes layout,
// syncs table positions/flags, and promotes elements in ONE transaction.
// On any failure the whole unit rolls back (no orphan tables, no half-published state).
func PublishSpaceLayoutAtomic(
	businessID, spaceID uint,
	expectedRevision int64,
	// layoutRewrite receives materialize map and must return final draft JSON with real IDs.
	layoutRewrite func(mat MaterializeResult) (draftJSON []byte, placements []TableLayoutPlacement, err error),
	candidates []MaterializeLayoutCandidate,
) (*RestaurantSpace, error) {
	var out *RestaurantSpace
	err := db.Transaction(func(tx *gorm.DB) error {
		var space RestaurantSpace
		// Single transaction + CAS on draft_revision provides concurrency control
		// (SQLite test DBs do not support reliable SELECT … FOR UPDATE).
		if err := tx.Where("id = ? AND business_id = ?", spaceID, businessID).
			First(&space).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSpaceNotFound
			}
			return err
		}
		if space.DraftRevision != expectedRevision {
			return ErrSpaceRevisionConflict
		}

		// linkedIDs seed is filled by the rewrite callback's document; here we
		// only materialize true candidates (table_id==0). Callers pre-checked
		// that non-zero IDs already belong to this space or are unassigned.
		linked := map[uint]struct{}{}
		mat, err := MaterializeCandidatesTx(tx, businessID, spaceID, candidates, linked)
		if err != nil {
			return err
		}

		draftBytes, placements, err := layoutRewrite(mat)
		if err != nil {
			return err
		}
		// Defense-in-depth: refuse to reassign tables already owned by another space.
		if err := assertPlacementsInSpaceTx(tx, businessID, spaceID, placements); err != nil {
			return err
		}

		now := time.Now().UTC()
		newRev := space.DraftRevision + 1
		// Persist materialized draft + publish in one update when possible.
		// Draft revision bumps; published mirrors the new draft.
		updates := map[string]interface{}{
			"draft_layout_json":       JSONRawMessage(draftBytes),
			"published_layout_json":   JSONRawMessage(draftBytes),
			"draft_revision":          newRev,
			"published_revision":      newRev,
			"has_unpublished_changes": false,
			"status":                  SpaceStatusPublished,
			"updated_at":              now,
		}
		res := tx.Model(&RestaurantSpace{}).
			Where("id = ? AND business_id = ? AND draft_revision = ?", spaceID, businessID, expectedRevision).
			Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrSpaceRevisionConflict
		}

		if err := SyncPublishedTablePositionsTx(tx, businessID, spaceID, placements); err != nil {
			return fmt.Errorf("sync published positions: %w", err)
		}
		if err := PromoteDraftElementsToPublishedTx(tx, businessID, spaceID); err != nil {
			return fmt.Errorf("promote elements: %w", err)
		}

		if err := tx.Where("id = ? AND business_id = ?", spaceID, businessID).First(&space).Error; err != nil {
			return err
		}
		out = &space
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CompleteSpaceScanSession marks a session completed and revokes its token.
func CompleteSpaceScanSession(businessID, sessionID uint) error {
	now := time.Now().UTC()
	// Scramble token_hash so the raw token no longer resolves.
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	deadHash := HashScanToken("completed:" + hex.EncodeToString(raw))
	res := db.Model(&SpaceScanSession{}).
		Where("id = ? AND business_id = ?", sessionID, businessID).
		Where("status NOT IN ?", []string{ScanStatusCancelled, ScanStatusExpired}).
		Updates(map[string]interface{}{
			"status":       ScanStatusCompleted,
			"token_hash":   deadHash,
			"completed_at": now,
			"updated_at":   now,
			"progress_pct": 100,
		})
	if res.Error != nil {
		return fmt.Errorf("complete space scan session: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrSpaceScanSessionNotFound
	}
	return nil
}

// MarkSpaceScanDraftApplyFailed records a non-fatal draft apply failure while
// keeping the session in review_ready so the operator can still fetch the result
// and apply manually.
func MarkSpaceScanDraftApplyFailed(businessID, sessionID uint, message string) error {
	if message == "" {
		message = "failed to apply scan layout to draft"
	}
	code := "draft_apply_failed"
	return db.Model(&SpaceScanSession{}).
		Where("id = ? AND business_id = ?", sessionID, businessID).
		Updates(map[string]interface{}{
			"error_code":       code,
			"error_message":    message,
			"progress_message": message,
			"updated_at":       time.Now().UTC(),
		}).Error
}

// ClearSpaceScanDraftApplyFailed clears sticky draft_apply_failed signals after a
// successful auto-apply so consumers no longer enter merge-only UI mode.
func ClearSpaceScanDraftApplyFailed(businessID, sessionID uint) error {
	return db.Model(&SpaceScanSession{}).
		Where("id = ? AND business_id = ?", sessionID, businessID).
		Updates(map[string]interface{}{
			"error_code":       nil,
			"error_message":    nil,
			"progress_message": "review ready",
			"updated_at":       time.Now().UTC(),
		}).Error
}
