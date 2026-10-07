package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services/director_actions"
)

var (
	// ErrDirectorActionNotFound is returned when the proposal public_id does not
	// exist for the given business.
	ErrDirectorActionNotFound = errors.New("director action not found")

	// ErrDirectorActionNotPending is returned when the proposal is not in pending state.
	ErrDirectorActionNotPending = errors.New("director action is not pending")

	// ErrDirectorActionExpired is returned when the proposal's ExpiresAt has passed.
	ErrDirectorActionExpired = errors.New("director action expired")

	// ErrDirectorActionVersionConflict is returned when the live menu version no
	// longer matches the version at preview time — the operator must re-preview.
	ErrDirectorActionVersionConflict = errors.New("menu changed since preview; re-preview required")

	// ErrDirectorActionReconfirmRequired is returned when the action carries a
	// large-swing flag and the caller did not set Reconfirm=true.
	ErrDirectorActionReconfirmRequired = errors.New("this action requires reconfirmation")

	// ErrDirectorActionUndoStale is returned when the menu version has advanced
	// beyond the post-apply version (something changed since apply).
	ErrDirectorActionUndoStale = errors.New("menu changed since apply; cannot undo")

	// ErrDirectorActionUndoExpired is returned when the 24h undo window has elapsed.
	ErrDirectorActionUndoExpired = errors.New("undo window elapsed")

	// ErrDirectorActionAlreadyUndone is returned when the audit row is already undone.
	ErrDirectorActionAlreadyUndone = errors.New("already undone")

	// ErrDirectorActionNoMatch is returned when a proposal matches zero menu
	// items at apply time (the exact-version gate means it also matched zero at
	// propose time). Applying it would be a lie — the proposal would be marked
	// applied with an un-undoable audit trail — so it is rejected outright and
	// left pending/dismissible (audit L4-17).
	ErrDirectorActionNoMatch = errors.New("proposal matches no menu items")

	// ErrInvalidProposedPrice is returned when a proposed absolute price target is
	// not strictly positive (handler maps to 400).
	ErrInvalidProposedPrice = errors.New("proposed price must be greater than zero")

	// ErrNoPriceIncrease is returned when the proposed price is at or below the
	// item's current price — the non-AI propose-price-change entry point only ever
	// raises a plowhorse toward its margin target (handler maps to 400).
	ErrNoPriceIncrease = errors.New("item is already at or above the suggested price")
)

// directorUndoWindow is the maximum time after apply that an undo is permitted.
const directorUndoWindow = 24 * time.Hour

// DirectorUndoWindow exposes the undo window so read-side handlers (the applied
// actions history) can compute undo eligibility with the same constant the
// write path enforces, instead of hard-coding 24h in two places.
const DirectorUndoWindow = directorUndoWindow

// DirectorActionService orchestrates the transactional apply path for
// director-proposed menu changes.  It deliberately holds no AI client — the
// AI only proposes; a human applies via this service.
type DirectorActionService struct {
	db *database.DB
}

// NewDirectorActionService constructs a DirectorActionService.
func NewDirectorActionService(db *database.DB) *DirectorActionService {
	return &DirectorActionService{db: db}
}

// ApplyDirectorActionInput carries the caller-supplied fields for an apply
// request.  Params (the mutation spec) are read from the stored proposal row —
// they are NEVER taken from the HTTP request body.
type ApplyDirectorActionInput struct {
	BusinessID  uint
	PublicID    string
	ActorUserID uint
	Reconfirm   bool
}

// ApplyDirectorActionResult is returned on a successful apply.
type ApplyDirectorActionResult struct {
	NewMenuVersion uint   `json:"new_menu_version"`
	ProposalID     string `json:"proposal_id"`
	AuditID        uint   `json:"audit_id"`
	Kind           string `json:"kind"`
}

// Apply re-validates server-side, recomputes the diff from stored params
// against the version-matched menu, and commits atomically:
//
//  1. Load proposal — must be pending and not expired.
//  2. Load menu — must match proposal.MenuVersion (409 otherwise).
//  3. Recompute mutation from stored params (never from request body).
//  4. If requiresReconfirm && !in.Reconfirm → 428.
//  5. Transaction: write menu (version-checked), insert audit, mark applied.
func (s *DirectorActionService) Apply(ctx context.Context, in ApplyDirectorActionInput) (*ApplyDirectorActionResult, error) {
	// Security invariant 1: load proposal from DB — caller supplies only public_id.
	proposal, err := database.GetDirectorProposedActionByPublicID(in.BusinessID, in.PublicID)
	if err != nil {
		return nil, ErrDirectorActionNotFound
	}

	// Security invariant 2: proposal must be pending and not expired.
	if proposal.Status != database.DirectorProposalPending {
		return nil, ErrDirectorActionNotPending
	}
	if !proposal.ExpiresAt.IsZero() && time.Now().After(proposal.ExpiresAt) {
		return nil, ErrDirectorActionExpired
	}

	// Security invariant 3: menu version must still match the preview version.
	menu, categories, err := database.GetMenuByBusinessID(in.BusinessID)
	if err != nil {
		return nil, fmt.Errorf("apply: load menu: %w", err)
	}
	if menu.Version != proposal.MenuVersion {
		return nil, ErrDirectorActionVersionConflict
	}

	// Security invariant 3 (continued) + 5: recompute from stored params so
	// the applied diff is provably identical to the previewed diff.
	mutated, _, _, reconfirm, before, after, err := s.recompute(proposal, categories)
	if err != nil {
		return nil, fmt.Errorf("apply: recompute: %w", err)
	}

	// Security invariant 5: large-swing confirmation gate.
	if reconfirm && !in.Reconfirm {
		return nil, ErrDirectorActionReconfirmRequired
	}

	// A proposal that matches zero items (e.g. a hallucinated scope) must not
	// "apply": marking it applied + auditing an empty diff produced a fake ✓
	// whose undo could never succeed (audit L4-17). The exact-version gate above
	// means zero-match here implies zero-match at propose time too — reject and
	// leave the proposal pending so the operator can dismiss it.
	if len(before) == 0 && len(after) == 0 {
		return nil, ErrDirectorActionNoMatch
	}

	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)

	var auditID uint
	var newVersion uint

	// Security invariant 4: one atomic transaction — any failure rolls back all writes.
	err = s.db.GetGorm().Transaction(func(tx *gorm.DB) error {
		v, werr := database.ApplyMenuCategoriesTx(tx, in.BusinessID, mutated, proposal.MenuVersion)
		if werr != nil {
			// ErrMenuVersionConflict bubbles; caller maps to 409.
			return werr
		}
		newVersion = v

		audit := &database.DirectorActionAudit{
			BusinessID:       in.BusinessID,
			ThreadID:         proposal.ThreadID,
			ProposedActionID: proposal.ID,
			ActorUserID:      in.ActorUserID,
			Kind:             proposal.Kind,
			ParamsJSON:       proposal.ParamsJSON,
			BeforeJSON:       string(beforeJSON),
			AfterJSON:        string(afterJSON),
			AppliedAt:        time.Now(),
		}
		if cerr := tx.Create(audit).Error; cerr != nil {
			return cerr
		}
		auditID = audit.ID

		// Status-CAS: only flip a still-pending row to applied. If a concurrent
		// applier already consumed this proposal between the line-95 read and
		// here, RowsAffected==0 and we abort (rolling back the menu write + audit)
		// rather than double-applying.
		res := tx.Model(&database.DirectorProposedAction{}).
			Where("id = ? AND status = ?", proposal.ID, database.DirectorProposalPending).
			Update("status", database.DirectorProposalApplied)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrDirectorActionNotPending
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, database.ErrMenuVersionConflict) {
			return nil, ErrDirectorActionVersionConflict
		}
		if errors.Is(err, ErrDirectorActionNotPending) {
			return nil, ErrDirectorActionNotPending
		}
		return nil, fmt.Errorf("apply: txn: %w", err)
	}

	return &ApplyDirectorActionResult{
		NewMenuVersion: newVersion,
		ProposalID:     proposal.PublicID,
		AuditID:        auditID,
		Kind:           proposal.Kind,
	}, nil
}

// UndoDirectorActionInput carries the caller-supplied fields for an undo
// request.  Only public_id and actor identity come from the HTTP layer.
type UndoDirectorActionInput struct {
	BusinessID  uint
	PublicID    string
	ActorUserID uint
}

// Undo reverses an applied action while (a) within the 24h window and (b) the
// menu version still equals the post-apply version (nothing changed since). It
// restores the affected items from before_json and stamps undone_at/undone_by.
// The restore is a versioned write (menu version bumps again) — undo is a
// forward correction, not a "rollback to N", so a later re-apply still sees a
// fresh version.
func (s *DirectorActionService) Undo(ctx context.Context, in UndoDirectorActionInput) (*ApplyDirectorActionResult, error) {
	// Security invariant 1: load proposal scoped to business.
	proposal, err := database.GetDirectorProposedActionByPublicID(in.BusinessID, in.PublicID)
	if err != nil {
		return nil, ErrDirectorActionNotFound
	}

	// Security invariant 2: load audit (must exist — proposal was applied).
	audit, err := database.GetDirectorActionAuditByProposalID(in.BusinessID, proposal.ID)
	if err != nil {
		return nil, ErrDirectorActionNotFound
	}

	// Security invariant 3: not already undone.
	if audit.UndoneAt != nil {
		return nil, ErrDirectorActionAlreadyUndone
	}

	// Security invariant 4: within the undo window.
	if time.Since(audit.AppliedAt) > directorUndoWindow {
		return nil, ErrDirectorActionUndoExpired
	}

	menu, categories, err := database.GetMenuByBusinessID(in.BusinessID)
	if err != nil {
		return nil, fmt.Errorf("undo: load menu: %w", err)
	}

	// Decode the before-snapshot from the audit row.
	var before []director_actions.ItemSnapshot
	if err := json.Unmarshal([]byte(audit.BeforeJSON), &before); err != nil {
		return nil, fmt.Errorf("undo: decode snapshot: %w", err)
	}

	// Legacy no-op audits (pre-L4-17 zero-match applies) changed nothing and
	// never bumped the menu version, so the invariant-5 check below can never
	// pass for them. Undoing nothing is always safe: mark the audit undone
	// without touching the menu.
	if len(before) == 0 {
		err = s.db.GetGorm().Transaction(func(tx *gorm.DB) error {
			return database.MarkAuditUndoneTx(tx, audit.ID, in.ActorUserID)
		})
		if err != nil {
			return nil, fmt.Errorf("undo: txn: %w", err)
		}
		return &ApplyDirectorActionResult{
			NewMenuVersion: menu.Version,
			ProposalID:     proposal.PublicID,
			AuditID:        audit.ID,
			Kind:           proposal.Kind,
		}, nil
	}

	// Security invariant 5: menu version must equal proposal.MenuVersion+1
	// (the post-apply version). If anything changed since apply, refuse.
	if menu.Version != proposal.MenuVersion+1 {
		return nil, ErrDirectorActionUndoStale
	}

	// Restore: apply before-values onto a copy of the current categories.
	restored := director_actions.RestoreSnapshots(categories, before)

	var newVersion uint
	err = s.db.GetGorm().Transaction(func(tx *gorm.DB) error {
		v, werr := database.ApplyMenuCategoriesTx(tx, in.BusinessID, restored, menu.Version)
		if werr != nil {
			return werr
		}
		newVersion = v
		return database.MarkAuditUndoneTx(tx, audit.ID, in.ActorUserID)
	})
	if err != nil {
		if errors.Is(err, database.ErrMenuVersionConflict) {
			return nil, ErrDirectorActionUndoStale
		}
		return nil, fmt.Errorf("undo: txn: %w", err)
	}

	return &ApplyDirectorActionResult{
		NewMenuVersion: newVersion,
		ProposalID:     proposal.PublicID,
		AuditID:        audit.ID,
		Kind:           proposal.Kind,
	}, nil
}

// ProposePriceChangeInput carries the caller-supplied fields for a non-AI,
// single-item price-change proposal (the menu-engineering matrix "Propose new
// price" button). NewPrice is an absolute DOLLAR target.
type ProposePriceChangeInput struct {
	BusinessID uint
	MenuItemID string
	NewPrice   float64
}

// ProposePriceChangeResult mirrors the data the AI propose tool surfaces so the
// frontend can consume one shape regardless of entry point.
type ProposePriceChangeResult struct {
	PublicID          string                         `json:"public_id"`
	Kind              string                         `json:"kind"`
	Preview           director_actions.ActionPreview `json:"preview"`
	Warnings          []string                       `json:"warnings"`
	RequiresReconfirm bool                           `json:"requires_reconfirm"`
	Title             string                         `json:"title"`
	Description       string                         `json:"description"`
}

// ProposePriceChange stages a single-item price-change proposal directly,
// OUTSIDE any Sage conversation, that the operator then previews/applies/undoes
// through the EXISTING apply/undo rail. It deliberately reuses the same
// ComputePriceChange + persisted-proposal machinery as the AI propose tool —
// the only difference is the threadless (ThreadID=0) origin and the absolute
// dollar target (which it converts to a flat upward delta).
//
// It writes NO menu data — apply (a separate, version-checked, human-initiated
// endpoint) recomputes the mutation from the stored ParamsJSON.
func (s *DirectorActionService) ProposePriceChange(ctx context.Context, in ProposePriceChangeInput) (*ProposePriceChangeResult, error) {
	menu, categories, err := database.GetMenuByBusinessID(in.BusinessID)
	if err != nil {
		return nil, fmt.Errorf("propose price change: load menu: %w", err)
	}

	// Locate the target item across all categories. Its current price is the
	// float64 DOLLAR baseline ComputePriceChange operates on.
	current, found := 0.0, false
	for ci := range categories {
		for ii := range categories[ci].Items {
			if categories[ci].Items[ii].ID == in.MenuItemID {
				current = categories[ci].Items[ii].Price
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		// Reuse the existing sentinel so the handler maps it to 404.
		return nil, ErrDirectorActionNotFound
	}

	// Round the absolute target to cents, then derive the exact flat increment so
	// ComputePriceChange yields current+delta == target after its own 2dp rounding.
	target := math.Round(in.NewPrice*100) / 100
	if target <= 0 {
		return nil, ErrInvalidProposedPrice
	}
	delta := math.Round((target-current)*100) / 100
	if delta <= 0 {
		// We only ever propose raising a plowhorse toward its margin target.
		return nil, ErrNoPriceIncrease
	}

	params := director_actions.PriceChangeParams{
		Scope:     "item:" + in.MenuItemID,
		Mode:      "flat",
		Value:     delta,
		Direction: "up",
	}

	// Resolve the business currency exactly like the AI tool so the proposal copy
	// reads in the business's own currency. Best-effort → USD. (audit C6)
	currency := "USD"
	if biz, berr := database.GetBusinessByID(in.BusinessID); berr == nil && biz != nil {
		if biz.DisplayCurrency != "" {
			currency = biz.DisplayCurrency
		} else if biz.DefaultCurrency != "" {
			currency = biz.DefaultCurrency
		}
	}

	_, preview, warnings, reconfirm, err := director_actions.ComputePriceChange(categories, params, currency)
	if err != nil {
		return nil, err // e.g. would zero a price — surfaced as-is.
	}

	title := director_actions.PriceChangeTitle(params, currency)
	desc := director_actions.PriceChangeDescription(preview)

	// Persist a pending proposal. ParamsJSON is apply-critical: Apply recomputes
	// the mutation from it, so it MUST be byte-identical in shape to what the AI
	// tool's persistProposal stores. PreviewJSON mirrors that same anonymous
	// blob (ActionPreview + title/description/warnings/requires_reconfirm).
	paramsJSON, _ := json.Marshal(params)
	previewBlob, _ := json.Marshal(struct {
		director_actions.ActionPreview
		Title             string   `json:"title"`
		Description       string   `json:"description"`
		Warnings          []string `json:"warnings"`
		RequiresReconfirm bool     `json:"requires_reconfirm"`
	}{preview, title, desc, warnings, reconfirm})

	row := database.DirectorProposedAction{
		BusinessID:  in.BusinessID,
		ThreadID:    0, // threadless: staged outside any Sage conversation.
		Kind:        string(director_actions.KindAdjustPrices),
		MenuVersion: menu.Version,
		ParamsJSON:  string(paramsJSON),
		PreviewJSON: string(previewBlob),
		ExpiresAt:   time.Now().Add(director_actions.ProposalTTL),
	}
	if err := database.CreateDirectorProposedAction(&row); err != nil {
		return nil, fmt.Errorf("propose price change: persist proposal: %w", err)
	}

	return &ProposePriceChangeResult{
		PublicID:          row.PublicID,
		Kind:              string(director_actions.KindAdjustPrices),
		Preview:           preview,
		Warnings:          warnings,
		RequiresReconfirm: reconfirm,
		Title:             title,
		Description:       desc,
	}, nil
}

// recompute dispatches by kind, returning the mutated tree and before/after
// snapshots of only the affected items.  Params are read exclusively from the
// stored proposal row — never from the request body.
func (s *DirectorActionService) recompute(
	p *database.DirectorProposedAction,
	categories []database.MenuCategory,
) (
	mutated []database.MenuCategory,
	preview director_actions.ActionPreview,
	warnings []string,
	reconfirm bool,
	before, after []director_actions.ItemSnapshot,
	err error,
) {
	// Defense-in-depth: ParamsJSON is always machine-marshalled at propose time,
	// so a decode failure means a corrupted row — abort rather than proceed with
	// zero-value params (which would silently consume the proposal on a no-op).
	switch director_actions.Kind(p.Kind) {
	case director_actions.KindAdjustPrices:
		var params director_actions.PriceChangeParams
		if uerr := json.Unmarshal([]byte(p.ParamsJSON), &params); uerr != nil {
			err = fmt.Errorf("apply: decode price params: %w", uerr)
			return
		}
		mutated, preview, warnings, reconfirm, err = director_actions.ComputePriceChange(categories, params)

	case director_actions.KindSetAvailability:
		var params director_actions.AvailabilityParams
		if uerr := json.Unmarshal([]byte(p.ParamsJSON), &params); uerr != nil {
			err = fmt.Errorf("apply: decode availability params: %w", uerr)
			return
		}
		mutated, preview, warnings, reconfirm, err = director_actions.ComputeAvailabilityChange(categories, params)

	case director_actions.KindEditContent:
		var params director_actions.ContentEditParams
		if uerr := json.Unmarshal([]byte(p.ParamsJSON), &params); uerr != nil {
			err = fmt.Errorf("apply: decode content params: %w", uerr)
			return
		}
		mutated, preview, warnings, reconfirm, err = director_actions.ComputeContentEdit(categories, params)

	default:
		err = fmt.Errorf("apply: unknown kind %q", p.Kind)
	}
	if err != nil {
		return
	}

	// Build before/after snapshots from items that actually changed.
	before, after = director_actions.DiffSnapshots(categories, mutated)
	return
}
