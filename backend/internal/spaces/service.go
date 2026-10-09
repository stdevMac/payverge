package spaces

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Service is the Spaces & Tables domain service (no HTTP wiring).
// All operations are business-scoped for tenant isolation.
type Service struct {
	// Clock is injectable for tests; defaults to time.Now.
	Clock func() time.Time
}

// NewService constructs a domain service.
func NewService() *Service {
	return &Service{Clock: time.Now}
}

func (s *Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
}

// ValidateLayout parses and validates a draft layout document for a space.
// businessID is required for tenant isolation of referenced tables.
// Uses draft rules: candidate table_id==0 allowed; overlap/bounds are warnings.
func (s *Service) ValidateLayout(businessID, spaceID uint, rawLayout []byte) (ValidationResult, error) {
	return s.validateLayoutMode(businessID, spaceID, rawLayout, ValidateDraft)
}

func (s *Service) validateLayoutMode(businessID, spaceID uint, rawLayout []byte, mode ValidationMode) (ValidationResult, error) {
	if businessID == 0 || spaceID == 0 {
		return ValidationResult{}, fmt.Errorf("%w: business_id and space_id required", ErrInvalidArgument)
	}
	if _, err := database.GetRestaurantSpaceByID(businessID, spaceID); err != nil {
		if errors.Is(err, database.ErrSpaceNotFound) {
			return ValidationResult{}, fmt.Errorf("%w: space %d", ErrNotFound, spaceID)
		}
		return ValidationResult{}, err
	}
	doc, err := ParseLayoutDocument(rawLayout)
	if err != nil {
		return ValidationResult{Valid: false, Issues: []ValidationIssue{{
			Code: "parse", Message: err.Error(),
		}}}, nil
	}
	res := ValidateLayoutDocumentMode(doc, mode)
	// Tenant isolation: every linked table_id must belong to this business.
	// Candidates (table_id==0) are skipped.
	if len(doc.Tables) > 0 {
		if err := assertTablesBelongToBusiness(businessID, doc.Tables); err != nil {
			res.add("tenant", "tables", err.Error())
		}
	}
	return res, nil
}

// UpdateDraft writes a new draft layout with optimistic concurrency.
// expectedRevision must match the current draft_revision or a ConflictError is returned.
// Candidate tables (table_id==0) are allowed; soft geometry warnings do not block.
func (s *Service) UpdateDraft(
	businessID, spaceID uint,
	expectedRevision int64,
	rawLayout []byte,
) (*database.RestaurantSpace, ValidationResult, error) {
	res, err := s.ValidateLayout(businessID, spaceID, rawLayout)
	if err != nil {
		return nil, res, err
	}
	if !res.Valid {
		return nil, res, &ValidationError{Result: res}
	}
	doc, err := ParseLayoutDocument(rawLayout)
	if err != nil {
		return nil, res, err
	}
	marshaled, err := MarshalLayoutDocument(doc)
	if err != nil {
		return nil, res, err
	}

	var boundary database.JSONRawMessage
	if doc.Boundary != nil {
		b, mErr := json.Marshal(doc.Boundary)
		if mErr != nil {
			return nil, res, mErr
		}
		boundary = database.JSONRawMessage(b)
	}
	var width, height *int
	if doc.WidthMm > 0 {
		w := doc.WidthMm
		width = &w
	}
	if doc.HeightMm > 0 {
		h := doc.HeightMm
		height = &h
	}

	space, err := database.UpdateRestaurantSpaceDraft(
		businessID, spaceID, expectedRevision,
		database.JSONRawMessage(marshaled),
		boundary, width, height, true,
	)
	if err != nil {
		if errors.Is(err, database.ErrSpaceRevisionConflict) {
			cur, _ := database.GetRestaurantSpaceByID(businessID, spaceID)
			actual := int64(0)
			if cur != nil {
				actual = cur.DraftRevision
			}
			return nil, res, &ConflictError{
				SpaceID:          spaceID,
				ExpectedRevision: expectedRevision,
				ActualRevision:   actual,
			}
		}
		if errors.Is(err, database.ErrSpaceNotFound) {
			return nil, res, fmt.Errorf("%w: space %d", ErrNotFound, spaceID)
		}
		return nil, res, err
	}
	_ = database.CreateSpaceLayoutAuditEvent(&database.SpaceLayoutAuditEvent{
		BusinessID: businessID,
		SpaceID:    &spaceID,
		Action:     "draft_updated",
		DetailJSON: database.JSONRawMessage(fmt.Sprintf(
			`{"expected_revision":%d,"new_revision":%d}`, expectedRevision, space.DraftRevision)),
	})
	return space, res, nil
}

// layoutCandidates builds MaterializeLayoutCandidate rows for table_id==0 placements.
func layoutCandidates(doc *LayoutDocument) []database.MaterializeLayoutCandidate {
	if doc == nil {
		return nil
	}
	var out []database.MaterializeLayoutCandidate
	for i, tbl := range doc.Tables {
		if !tbl.IsCandidate() {
			continue
		}
		cap := 4
		if tbl.MaxCapacity != nil && *tbl.MaxCapacity > 0 {
			cap = *tbl.MaxCapacity
		} else if tbl.VisibleSeatCount != nil && *tbl.VisibleSeatCount > 0 {
			cap = *tbl.VisibleSeatCount
		} else if tbl.MinCapacity != nil && *tbl.MinCapacity > 0 {
			cap = *tbl.MinCapacity
		}
		x, y, w, h := tbl.XMm, tbl.YMm, tbl.WidthMm, tbl.HeightMm
		out = append(out, database.MaterializeLayoutCandidate{
			Index:            i,
			ClientKey:        tbl.ClientKey,
			Name:             tbl.Name,
			Shape:            tbl.Shape,
			Capacity:         cap,
			MinCapacity:      tbl.MinCapacity,
			MaxCapacity:      tbl.MaxCapacity,
			VisibleSeatCount: tbl.VisibleSeatCount,
			IsReservable:     tbl.IsReservable,
			IsCombinable:     tbl.IsCombinable,
			IsAccessible:     tbl.IsAccessible,
			PosXMm:           &x,
			PosYMm:           &y,
			WidthMm:          &w,
			HeightMm:         &h,
			RotationDeg:      tbl.RotationDeg,
			RegionID:         tbl.RegionID,
		})
	}
	return out
}

// applyMaterializeMap rewrites candidate table_ids from MaterializeResult.
func applyMaterializeMap(doc *LayoutDocument, mat database.MaterializeResult) {
	if doc == nil || mat == nil {
		return
	}
	for i := range doc.Tables {
		if id, ok := mat[i]; ok && id > 0 {
			doc.Tables[i].TableID = id
		}
	}
}

// placementsFromDoc builds relational placements for sync (linked tables only).
func placementsFromDoc(doc *LayoutDocument) []database.TableLayoutPlacement {
	if doc == nil {
		return nil
	}
	placements := make([]database.TableLayoutPlacement, 0, len(doc.Tables))
	for _, tbl := range doc.Tables {
		if tbl.TableID == 0 {
			continue
		}
		x, y, w, h := tbl.XMm, tbl.YMm, tbl.WidthMm, tbl.HeightMm
		shape := tbl.Shape
		if shape == "" {
			shape = database.TableShapeRectangle
		}
		p := database.TableLayoutPlacement{
			TableID:          tbl.TableID,
			RegionID:         tbl.RegionID,
			PosXMm:           &x,
			PosYMm:           &y,
			WidthMm:          &w,
			HeightMm:         &h,
			RotationDeg:      tbl.RotationDeg,
			Shape:            shape,
			MinCapacity:      tbl.MinCapacity,
			MaxCapacity:      tbl.MaxCapacity,
			VisibleSeatCount: tbl.VisibleSeatCount,
			IsReservable:     tbl.IsReservable,
			IsCombinable:     tbl.IsCombinable,
			IsAccessible:     tbl.IsAccessible,
		}
		if tbl.MaxCapacity != nil && *tbl.MaxCapacity > 0 {
			cap := *tbl.MaxCapacity
			p.Capacity = &cap
		} else if tbl.VisibleSeatCount != nil && *tbl.VisibleSeatCount > 0 {
			cap := *tbl.VisibleSeatCount
			p.Capacity = &cap
		} else if tbl.MinCapacity != nil && *tbl.MinCapacity > 0 {
			cap := *tbl.MinCapacity
			p.Capacity = &cap
		}
		placements = append(placements, p)
	}
	return placements
}

// ApplyScanLayoutToDraft writes a scan-generated layout into the space draft
// when safe. Policy:
//   - empty draft → CAS write
//   - draft is scan-owned relative to this layout (subset of result) → CAS replace
//   - draft has linked tables / true operator content → ErrScanDraftOperatorContent
//     (worker should mark draft_apply_failed)
//   - draft has only unlinked candidates that do not match this result → skip
//     with ErrScanDraftSkipped (leave draft; do NOT mark draft_apply_failed)
//
// On CAS conflict it fails closed (no force-retry on latest).
func (s *Service) ApplyScanLayoutToDraft(businessID, spaceID uint, layoutJSON []byte) (*database.RestaurantSpace, ValidationResult, error) {
	space, err := database.GetRestaurantSpaceByID(businessID, spaceID)
	if err != nil {
		if errors.Is(err, database.ErrSpaceNotFound) {
			return nil, ValidationResult{}, fmt.Errorf("%w: space %d", ErrNotFound, spaceID)
		}
		return nil, ValidationResult{}, err
	}
	draftRaw := []byte(space.DraftLayoutJSON)
	if draftHasOperatorContent(space.DraftLayoutJSON) {
		// Safe to replace when draft is already this scan's content (auto-apply /
		// prior filter of the same result) — supports retry without false failure.
		if draftMayBeReplacedByScanReview(draftRaw, layoutJSON) {
			// fall through to UpdateDraft
		} else if draftHasOnlyUnlinkedCandidateTables(space.DraftLayoutJSON) {
			// Prior scan candidates (or editor candidates) that do not match this
			// result — leave draft; do not mark draft_apply_failed.
			return space, ValidationResult{Valid: true}, ErrScanDraftSkipped
		} else {
			// Linked tables, walls/regions/boundary, or other operator structure.
			return nil, ValidationResult{}, fmt.Errorf("%w: space %d", ErrScanDraftOperatorContent, spaceID)
		}
	}
	updated, res, uerr := s.UpdateDraft(businessID, spaceID, space.DraftRevision, layoutJSON)
	if uerr != nil {
		// Fail closed: never retry against a newer revision (would wipe concurrent edits).
		if errors.Is(uerr, ErrRevisionConflict) {
			cur, _ := database.GetRestaurantSpaceByID(businessID, spaceID)
			actual := int64(0)
			if cur != nil {
				actual = cur.DraftRevision
			}
			return nil, res, &ConflictError{
				SpaceID:          spaceID,
				ExpectedRevision: space.DraftRevision,
				ActualRevision:   actual,
			}
		}
		return nil, res, uerr
	}
	return updated, res, nil
}

// draftHasOperatorContent reports whether the draft already contains tables,
// regions, structural elements, boundary, or room dimensions that a scan
// auto-apply / full review-replace must not destroy.
func draftHasOperatorContent(raw database.JSONRawMessage) bool {
	if len(raw) == 0 || string(raw) == "{}" || string(raw) == "null" {
		return false
	}
	doc, err := ParseLayoutDocument(raw)
	if err != nil || doc == nil {
		return true // treat unparseable as content — safer than overwrite
	}
	if len(doc.Tables) > 0 || len(doc.Regions) > 0 || len(doc.Elements) > 0 {
		return true
	}
	// Non-default room dimensions count as intentional work (even without tables).
	if doc.WidthMm > 0 || doc.HeightMm > 0 {
		return true
	}
	if doc.Boundary != nil && len(doc.Boundary.PointsMm) >= 3 {
		return true
	}
	return false
}

// draftHasOnlyUnlinkedCandidateTables reports a draft that only has candidate
// placements (table_id==0) and no structural elements/regions/boundary.
// Used to skip auto-apply without treating prior scan candidates as operator
// failure, while still fail-closing on walls/linked tables.
func draftHasOnlyUnlinkedCandidateTables(raw database.JSONRawMessage) bool {
	doc, err := ParseLayoutDocument(raw)
	if err != nil || doc == nil {
		return false
	}
	if len(doc.Elements) > 0 || len(doc.Regions) > 0 {
		return false
	}
	if doc.Boundary != nil && len(doc.Boundary.PointsMm) >= 3 {
		return false
	}
	if len(doc.Tables) == 0 {
		// Dimensions-only or empty structural — not "candidate tables only".
		return false
	}
	for _, t := range doc.Tables {
		if t.TableID != 0 {
			return false
		}
	}
	return true
}

// IsScanDraftSkipped reports whether err is ErrScanDraftSkipped.
func IsScanDraftSkipped(err error) bool {
	return errors.Is(err, ErrScanDraftSkipped)
}

// draftMayBeReplacedByScanReview reports whether the current draft is safe to
// full-replace with a scan review apply. Empty drafts are always safe. Drafts
// that only contain scan-result content (subset of result, with at least one
// table/element/region) are safe — that is the auto-apply path. Dimension-only
// operator shells and drafts with operator-owned tables/walls fail closed.
func draftMayBeReplacedByScanReview(draftJSON, resultJSON []byte) bool {
	if !draftHasOperatorContent(database.JSONRawMessage(draftJSON)) {
		return true
	}
	doc, err := ParseLayoutDocument(draftJSON)
	if err != nil || doc == nil {
		return false
	}
	// Dimension/boundary-only drafts are operator work, not a prior scan apply.
	if len(doc.Tables) == 0 && len(doc.Elements) == 0 && len(doc.Regions) == 0 {
		return false
	}
	// Treat "draft is itself a valid review subset of the scan result" as
	// scan-owned (auto-applied or previously filtered). Operator additions
	// make ValidateReviewLayoutAgainstResult fail.
	return ValidateReviewLayoutAgainstResult(resultJSON, draftJSON) == nil
}

// ApplyReviewLayoutToDraft writes a review layout into the draft.
// expectedRevision must be > 0 and match current draft_revision (strict CAS).
// Callers that need empty-draft pin must pass the live revision explicitly.
func (s *Service) ApplyReviewLayoutToDraft(businessID, spaceID uint, expectedRevision int64, layoutJSON []byte) (*database.RestaurantSpace, ValidationResult, error) {
	if expectedRevision <= 0 {
		return nil, ValidationResult{}, fmt.Errorf("%w: expected_revision is required", ErrInvalidArgument)
	}
	space, err := database.GetRestaurantSpaceByID(businessID, spaceID)
	if err != nil {
		if errors.Is(err, database.ErrSpaceNotFound) {
			return nil, ValidationResult{}, fmt.Errorf("%w: space %d", ErrNotFound, spaceID)
		}
		return nil, ValidationResult{}, err
	}
	if space.DraftRevision != expectedRevision {
		return nil, ValidationResult{}, &ConflictError{
			SpaceID:          spaceID,
			ExpectedRevision: expectedRevision,
			ActualRevision:   space.DraftRevision,
		}
	}
	return s.UpdateDraft(businessID, spaceID, expectedRevision, layoutJSON)
}

// ApplyReviewLayoutFromScanResult validates that submitted is a safe subset of
// the scan result, refuses to clobber operator-owned drafts, then CAS-applies.
// Rejects foreign table_ids and structural rewrites that expand beyond the result.
// expectedRevision must be > 0 (public token path requires it).
func (s *Service) ApplyReviewLayoutFromScanResult(
	businessID, spaceID uint,
	expectedRevision int64,
	resultJSON, submittedJSON []byte,
) (*database.RestaurantSpace, ValidationResult, error) {
	if expectedRevision <= 0 {
		return nil, ValidationResult{}, fmt.Errorf("%w: expected_revision is required", ErrInvalidArgument)
	}
	if err := ValidateReviewLayoutAgainstResult(resultJSON, submittedJSON); err != nil {
		return nil, ValidationResult{Valid: false, Issues: []ValidationIssue{{
			Code: "review_layout_rejected", Message: err.Error(),
		}}}, fmt.Errorf("%w: %v", ErrInvalidArgument, err)
	}
	// Space-scope linked tables in the submitted payload.
	sub, err := ParseLayoutDocument(submittedJSON)
	if err != nil {
		return nil, ValidationResult{}, fmt.Errorf("%w: %v", ErrInvalidArgument, err)
	}
	if err := assertTablesBelongToSpaceOrUnassigned(businessID, spaceID, sub.Tables); err != nil {
		return nil, ValidationResult{Valid: false, Issues: []ValidationIssue{{
			Code: "tenant", Message: err.Error(),
		}}}, err
	}

	space, err := database.GetRestaurantSpaceByID(businessID, spaceID)
	if err != nil {
		if errors.Is(err, database.ErrSpaceNotFound) {
			return nil, ValidationResult{}, fmt.Errorf("%w: space %d", ErrNotFound, spaceID)
		}
		return nil, ValidationResult{}, err
	}
	// Fail closed: never full-replace an operator-owned draft via scan review.
	// Empty drafts and scan-auto-applied drafts (subset of result) are allowed.
	if !draftMayBeReplacedByScanReview([]byte(space.DraftLayoutJSON), resultJSON) {
		res := ValidationResult{Valid: false, Issues: []ValidationIssue{{
			Code:    "draft_has_content",
			Message: "space draft has operator content; re-apply from the authenticated editor (merge), not public token replace",
		}}}
		// Surface as validation so clients get code=layout_invalid / draft_has_content,
		// plus actual_revision via a ConflictError unwrap is not needed — attach revision
		// in the issue message path; handler can still read space.DraftRevision from GET.
		return nil, res, &ValidationError{Result: res}
	}
	// CAS pre-check before write so ActualRevision is accurate.
	if space.DraftRevision != expectedRevision {
		return nil, ValidationResult{}, &ConflictError{
			SpaceID:          spaceID,
			ExpectedRevision: expectedRevision,
			ActualRevision:   space.DraftRevision,
		}
	}
	return s.ApplyReviewLayoutToDraft(businessID, spaceID, expectedRevision, submittedJSON)
}

// CompleteScanSession revokes the scan token and marks the session completed.
func (s *Service) CompleteScanSession(businessID, sessionID uint) error {
	if err := database.CompleteSpaceScanSession(businessID, sessionID); err != nil {
		if errors.Is(err, database.ErrSpaceScanSessionNotFound) {
			return fmt.Errorf("%w: scan session %d", ErrNotFound, sessionID)
		}
		return err
	}
	return nil
}

// PublishLayout materializes candidates, validates, publishes layout JSON, and
// syncs relational positions/flags in ONE transaction. Failure returns an error
// without a successful published response and rolls back table creates.
func (s *Service) PublishLayout(businessID, spaceID uint, expectedRevision int64) (*database.RestaurantSpace, ValidationResult, error) {
	space, err := database.GetRestaurantSpaceByID(businessID, spaceID)
	if err != nil {
		if errors.Is(err, database.ErrSpaceNotFound) {
			return nil, ValidationResult{}, fmt.Errorf("%w: space %d", ErrNotFound, spaceID)
		}
		return nil, ValidationResult{}, err
	}
	if space.DraftRevision != expectedRevision {
		return nil, ValidationResult{}, &ConflictError{
			SpaceID:          spaceID,
			ExpectedRevision: expectedRevision,
			ActualRevision:   space.DraftRevision,
		}
	}

	doc, err := ParseLayoutDocument(space.DraftLayoutJSON)
	if err != nil {
		return nil, ValidationResult{}, err
	}
	cands := layoutCandidates(doc)

	// Pre-validate draft shape (sizes/polygons). Candidates allowed here.
	// Tenant + space-scope checks for already-linked tables run OUTSIDE the
	// publish transaction (global DB reads would deadlock SQLite MaxOpenConns=1
	// if done mid-tx). Newly materialized tables are always created for this
	// business+space inside the same tx.
	pre := ValidateLayoutDocumentMode(doc, ValidateDraft)
	if !pre.Valid {
		return nil, pre, &ValidationError{Result: pre}
	}
	if err := assertTablesBelongToSpaceOrUnassigned(businessID, spaceID, doc.Tables); err != nil {
		pre.add("tenant", "tables", err.Error())
		return nil, pre, &ValidationError{Result: pre}
	}

	var finalRes ValidationResult
	published, err := database.PublishSpaceLayoutAtomic(
		businessID, spaceID, expectedRevision,
		func(mat database.MaterializeResult) ([]byte, []database.TableLayoutPlacement, error) {
			applyMaterializeMap(doc, mat)
			// Publish validation requires real IDs (no external DB reads).
			finalRes = ValidateLayoutDocumentMode(doc, ValidatePublish)
			if !finalRes.Valid {
				return nil, nil, &ValidationError{Result: finalRes}
			}
			raw, err := MarshalLayoutDocument(doc)
			if err != nil {
				return nil, nil, err
			}
			return raw, placementsFromDoc(doc), nil
		},
		cands,
	)
	if err != nil {
		if errors.Is(err, database.ErrSpaceRevisionConflict) {
			// Re-fetch current revision so clients can recover (pre-tx read is stale).
			actual := expectedRevision
			if cur, rerr := database.GetRestaurantSpaceByID(businessID, spaceID); rerr == nil && cur != nil {
				actual = cur.DraftRevision
			}
			return nil, finalRes, &ConflictError{
				SpaceID:          spaceID,
				ExpectedRevision: expectedRevision,
				ActualRevision:   actual,
			}
		}
		var ve *ValidationError
		if errors.As(err, &ve) {
			return nil, ve.Result, ve
		}
		return nil, finalRes, err
	}

	_ = database.CreateSpaceLayoutAuditEvent(&database.SpaceLayoutAuditEvent{
		BusinessID: businessID,
		SpaceID:    &spaceID,
		Action:     "layout_published",
		DetailJSON: database.JSONRawMessage(fmt.Sprintf(
			`{"published_revision":%d,"table_count":%d}`, published.PublishedRevision, len(doc.Tables))),
	})
	return published, finalRes, nil
}

// DiscardDraft reverts draft layout to the last published layout.
func (s *Service) DiscardDraft(businessID, spaceID uint, expectedRevision int64) (*database.RestaurantSpace, error) {
	space, err := database.DiscardRestaurantSpaceDraft(businessID, spaceID, expectedRevision)
	if err != nil {
		if errors.Is(err, database.ErrSpaceRevisionConflict) {
			actual := expectedRevision
			if cur, rerr := database.GetRestaurantSpaceByID(businessID, spaceID); rerr == nil && cur != nil {
				actual = cur.DraftRevision
			}
			return nil, &ConflictError{
				SpaceID:          spaceID,
				ExpectedRevision: expectedRevision,
				ActualRevision:   actual,
			}
		}
		if errors.Is(err, database.ErrSpaceNotFound) {
			return nil, fmt.Errorf("%w: space %d", ErrNotFound, spaceID)
		}
		return nil, err
	}
	_ = database.CreateSpaceLayoutAuditEvent(&database.SpaceLayoutAuditEvent{
		BusinessID: businessID,
		SpaceID:    &spaceID,
		Action:     "draft_discarded",
		DetailJSON: database.JSONRawMessage(fmt.Sprintf(`{"new_revision":%d}`, space.DraftRevision)),
	})
	return space, nil
}

// DuplicateSpace clones a space (metadata + layout JSON) into a new draft.
// Tables are not cloned (QR identity is preserved on originals only).
func (s *Service) DuplicateSpace(businessID, spaceID uint, newName string) (*database.RestaurantSpace, error) {
	if newName == "" {
		return nil, fmt.Errorf("%w: newName is required", ErrInvalidArgument)
	}
	clone, err := database.DuplicateRestaurantSpace(businessID, spaceID, newName)
	if err != nil {
		if errors.Is(err, database.ErrSpaceNotFound) {
			return nil, fmt.Errorf("%w: space %d", ErrNotFound, spaceID)
		}
		return nil, err
	}
	// Clone draft layout elements.
	draftTrue := true
	els, err := database.ListSpaceLayoutElements(businessID, spaceID, &draftTrue)
	if err != nil {
		return clone, nil // space clone succeeded; elements best-effort
	}
	if len(els) > 0 {
		cloned := make([]database.SpaceLayoutElement, len(els))
		for i, el := range els {
			cloned[i] = el
			cloned[i].ID = 0
			cloned[i].SpaceID = clone.ID
			cloned[i].BusinessID = businessID
			cloned[i].IsDraft = true
		}
		_ = database.ReplaceDraftLayoutElements(businessID, clone.ID, cloned)
	}
	// Clone regions.
	regions, err := database.ListSpaceRegionsBySpace(businessID, spaceID)
	if err == nil && len(regions) > 0 {
		clonedR := make([]database.SpaceRegion, len(regions))
		for i, r := range regions {
			clonedR[i] = r
			clonedR[i].ID = 0
			clonedR[i].SpaceID = clone.ID
			clonedR[i].BusinessID = businessID
		}
		_ = database.ReplaceSpaceRegionsForSpace(businessID, clone.ID, clonedR)
	}

	_ = database.CreateSpaceLayoutAuditEvent(&database.SpaceLayoutAuditEvent{
		BusinessID: businessID,
		SpaceID:    &clone.ID,
		Action:     "space_duplicated",
		DetailJSON: database.JSONRawMessage(fmt.Sprintf(`{"source_space_id":%d}`, spaceID)),
	})
	return clone, nil
}

// SafeDeleteOrArchive archives a space (never hard-deletes tables with history).
// If any tables in the space have open bills or active reservations, returns
// ActivityBlockError. Tables are unassigned from the space; table rows and QR
// codes remain intact.
func (s *Service) SafeDeleteOrArchive(businessID, spaceID uint, hardSoftDelete bool) error {
	if _, err := database.GetRestaurantSpaceByID(businessID, spaceID); err != nil {
		if errors.Is(err, database.ErrSpaceNotFound) {
			return fmt.Errorf("%w: space %d", ErrNotFound, spaceID)
		}
		return err
	}
	tables, err := database.ListTablesBySpace(businessID, spaceID)
	if err != nil {
		return err
	}
	if len(tables) > 0 {
		ids := make([]uint, len(tables))
		for i, t := range tables {
			ids[i] = t.ID
		}
		openBills, err := database.CountOpenBillsForTables(businessID, ids)
		if err != nil {
			return err
		}
		openRes, err := database.CountActiveReservationsForTables(businessID, ids, s.now())
		if err != nil {
			return err
		}
		if openBills > 0 || openRes > 0 {
			return &ActivityBlockError{OpenBills: openBills, OpenReservations: openRes}
		}
	}

	if hardSoftDelete {
		if err := database.SoftDeleteRestaurantSpace(businessID, spaceID); err != nil {
			return err
		}
	} else {
		if err := database.ArchiveRestaurantSpace(businessID, spaceID); err != nil {
			return err
		}
	}
	_ = database.CreateSpaceLayoutAuditEvent(&database.SpaceLayoutAuditEvent{
		BusinessID: businessID,
		SpaceID:    &spaceID,
		Action:     "space_archived",
		DetailJSON: database.JSONRawMessage(fmt.Sprintf(`{"soft_delete":%v}`, hardSoftDelete)),
	})
	return nil
}

// AssignLegacyTables assigns unplaced (or specified) tables to a space so
// operators can place existing QR tables onto a floor plan without recreating them.
func (s *Service) AssignLegacyTables(businessID, spaceID uint, tableIDs []uint) error {
	if _, err := database.GetRestaurantSpaceByID(businessID, spaceID); err != nil {
		if errors.Is(err, database.ErrSpaceNotFound) {
			return fmt.Errorf("%w: space %d", ErrNotFound, spaceID)
		}
		return err
	}
	ids := tableIDs
	if len(ids) == 0 {
		unassigned, err := database.ListUnassignedTables(businessID)
		if err != nil {
			return err
		}
		ids = make([]uint, len(unassigned))
		for i, t := range unassigned {
			ids[i] = t.ID
		}
	}
	if err := database.AssignTablesToSpace(businessID, spaceID, ids); err != nil {
		return err
	}
	_ = database.CreateSpaceLayoutAuditEvent(&database.SpaceLayoutAuditEvent{
		BusinessID: businessID,
		SpaceID:    &spaceID,
		Action:     "legacy_tables_assigned",
		DetailJSON: database.JSONRawMessage(fmt.Sprintf(`{"table_count":%d}`, len(ids))),
	})
	return nil
}

func assertTablesBelongToBusiness(businessID uint, tables []LayoutTable) error {
	for _, tbl := range tables {
		if tbl.TableID == 0 {
			continue
		}
		t, err := database.GetTableByID(tbl.TableID)
		if err != nil {
			return fmt.Errorf("%w: table %d not found", ErrTenantIsolation, tbl.TableID)
		}
		if t.BusinessID != businessID {
			return fmt.Errorf("%w: table %d belongs to business %d, not %d",
				ErrTenantIsolation, tbl.TableID, t.BusinessID, businessID)
		}
	}
	return nil
}

// assertTablesBelongToSpaceOrUnassigned rejects layout table_ids that already
// belong to a different space (prevents cross-space reassignment via JSON).
func assertTablesBelongToSpaceOrUnassigned(businessID, spaceID uint, tables []LayoutTable) error {
	for _, tbl := range tables {
		if tbl.TableID == 0 {
			continue
		}
		t, err := database.GetTableByID(tbl.TableID)
		if err != nil {
			return fmt.Errorf("%w: table %d not found", ErrTenantIsolation, tbl.TableID)
		}
		if t.BusinessID != businessID {
			return fmt.Errorf("%w: table %d belongs to business %d, not %d",
				ErrTenantIsolation, tbl.TableID, t.BusinessID, businessID)
		}
		if t.SpaceID != nil && *t.SpaceID != 0 && *t.SpaceID != spaceID {
			return fmt.Errorf("%w: table %d belongs to space %d, not space %d",
				ErrTenantIsolation, tbl.TableID, *t.SpaceID, spaceID)
		}
	}
	return nil
}

// ValidateReviewLayoutAgainstResult ensures submitted is a safe filtered subset
// of the scan result — not an arbitrary full draft rewrite via the public token.
// Allowed: same (or empty) shell dimensions; tables that are a subset of result
// candidates (by client_key, or name+shape+geometry fingerprint); elements and
// regions that fingerprint-match result slots. Rejected: foreign real table_ids,
// invented elements/regions, expanded room size, invented tables.
func ValidateReviewLayoutAgainstResult(resultJSON, submittedJSON []byte) error {
	result, err := ParseLayoutDocument(resultJSON)
	if err != nil {
		return fmt.Errorf("parse result layout: %w", err)
	}
	sub, err := ParseLayoutDocument(submittedJSON)
	if err != nil {
		return fmt.Errorf("parse submitted layout: %w", err)
	}
	if sub == nil {
		return fmt.Errorf("submitted layout is empty")
	}
	if result == nil {
		result = &LayoutDocument{SchemaVersion: LayoutSchemaVersion}
	}

	// Shell: when result has dimensions, submitted must not expand.
	// When result dimensions are zero, submitted must also stay zero (no expansion
	// from an incomplete scan shell).
	if result.WidthMm > 0 {
		if sub.WidthMm > result.WidthMm {
			return fmt.Errorf("submitted width exceeds scan result")
		}
	} else if sub.WidthMm > 0 {
		return fmt.Errorf("submitted width not allowed when scan result has no width")
	}
	if result.HeightMm > 0 {
		if sub.HeightMm > result.HeightMm {
			return fmt.Errorf("submitted height exceeds scan result")
		}
	} else if sub.HeightMm > 0 {
		return fmt.Errorf("submitted height not allowed when scan result has no height")
	}

	// Structural elements: real subset by type+name+approx position (not count-only).
	if err := matchReviewElementSubset(result.Elements, sub.Elements); err != nil {
		return err
	}
	if err := matchReviewRegionSubset(result.Regions, sub.Regions); err != nil {
		return err
	}

	// Build allowlist of result table fingerprints.
	type fp struct {
		clientKey  string
		name       string
		shape      string
		x, y, w, h int
		tableID    uint
	}
	allowed := make([]fp, 0, len(result.Tables))
	for _, t := range result.Tables {
		allowed = append(allowed, fp{
			clientKey: t.ClientKey,
			name:      strings.TrimSpace(strings.ToLower(t.Name)),
			shape:     t.Shape,
			x:         t.XMm, y: t.YMm, w: t.WidthMm, h: t.HeightMm,
			tableID: t.TableID,
		})
	}
	// Track which result slots were consumed.
	used := make([]bool, len(allowed))

	for i, t := range sub.Tables {
		// Reject real foreign table_ids not present in the result.
		if t.TableID != 0 {
			matched := -1
			for j, a := range allowed {
				if used[j] {
					continue
				}
				if a.tableID == t.TableID {
					matched = j
					break
				}
			}
			if matched < 0 {
				return fmt.Errorf("submitted tables[%d] table_id %d is not in scan result", i, t.TableID)
			}
			used[matched] = true
			continue
		}
		// Candidate must match a result candidate (client_key preferred, else
		// name+shape+geometry — name alone is not enough when duplicates exist).
		matched := -1
		subName := strings.TrimSpace(strings.ToLower(t.Name))
		for j, a := range allowed {
			if used[j] {
				continue
			}
			if t.ClientKey != "" && a.clientKey != "" && t.ClientKey == a.clientKey {
				matched = j
				break
			}
			if a.tableID != 0 {
				continue // already a linked result table; candidates match candidate slots
			}
			// Geometry fingerprint within 50mm AND same shape (name optional helper).
			geoOK := absInt(t.XMm-a.x) <= 50 && absInt(t.YMm-a.y) <= 50 &&
				absInt(t.WidthMm-a.w) <= 50 && absInt(t.HeightMm-a.h) <= 50 &&
				t.Shape == a.shape
			if !geoOK {
				continue
			}
			if subName == "" || a.name == "" || subName == a.name {
				matched = j
				break
			}
		}
		if matched < 0 {
			return fmt.Errorf("submitted tables[%d] is not a subset of scan result candidates", i)
		}
		used[matched] = true
	}
	return nil
}

func optionalInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// matchReviewElementSubset ensures every submitted element fingerprints a unique
// result element (type + optional name + approx position within 50mm).
func matchReviewElementSubset(result, sub []LayoutElement) error {
	if len(sub) > len(result) {
		return fmt.Errorf("submitted adds structural elements beyond scan result")
	}
	used := make([]bool, len(result))
	for i, e := range sub {
		matched := -1
		subName := strings.TrimSpace(strings.ToLower(e.Name))
		ex, ey := optionalInt(e.XMm), optionalInt(e.YMm)
		for j, r := range result {
			if used[j] {
				continue
			}
			if e.ElementType != r.ElementType {
				continue
			}
			rx, ry := optionalInt(r.XMm), optionalInt(r.YMm)
			if absInt(ex-rx) > 50 || absInt(ey-ry) > 50 {
				continue
			}
			rName := strings.TrimSpace(strings.ToLower(r.Name))
			if subName != "" && rName != "" && subName != rName {
				continue
			}
			matched = j
			break
		}
		if matched < 0 {
			return fmt.Errorf("submitted elements[%d] is not a subset of scan result elements", i)
		}
		used[matched] = true
	}
	return nil
}

// matchReviewRegionSubset ensures every submitted region matches a unique result
// region by name (case-insensitive) or identical polygon point count + first point.
func matchReviewRegionSubset(result, sub []LayoutRegion) error {
	if len(sub) > len(result) {
		return fmt.Errorf("submitted adds regions beyond scan result")
	}
	used := make([]bool, len(result))
	for i, e := range sub {
		matched := -1
		subName := strings.TrimSpace(strings.ToLower(e.Name))
		for j, r := range result {
			if used[j] {
				continue
			}
			rName := strings.TrimSpace(strings.ToLower(r.Name))
			if subName != "" && rName != "" && subName == rName {
				matched = j
				break
			}
			// Fallback: same point count and first vertex within 50mm.
			if len(e.PolygonMm) > 0 && len(e.PolygonMm) == len(r.PolygonMm) {
				if absInt(int(e.PolygonMm[0].X)-int(r.PolygonMm[0].X)) <= 50 &&
					absInt(int(e.PolygonMm[0].Y)-int(r.PolygonMm[0].Y)) <= 50 {
					matched = j
					break
				}
			}
		}
		if matched < 0 {
			return fmt.Errorf("submitted regions[%d] is not a subset of scan result regions", i)
		}
		used[matched] = true
	}
	return nil
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
