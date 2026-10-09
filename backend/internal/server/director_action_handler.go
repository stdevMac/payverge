package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// DirectorActionServiceAPI is the slice of *services.DirectorActionService the
// apply and undo handlers depend on, allowing test stubs to be injected.
type DirectorActionServiceAPI interface {
	Apply(ctx context.Context, in services.ApplyDirectorActionInput) (*services.ApplyDirectorActionResult, error)
	Undo(ctx context.Context, in services.UndoDirectorActionInput) (*services.ApplyDirectorActionResult, error)
	ProposePriceChange(ctx context.Context, in services.ProposePriceChangeInput) (*services.ProposePriceChangeResult, error)
}

// directorActionService is the package-level handle, set by main.go via
// SetDirectorActionService after service construction.
var directorActionService DirectorActionServiceAPI

// SetDirectorActionService wires the apply service into the handler layer.
// Must be called during application startup before serving requests.
func SetDirectorActionService(svc DirectorActionServiceAPI) {
	directorActionService = svc
}

// businessIDFromParam parses the ":id" route param into a uint business ID.
// On failure it writes a 400 response and returns (0, false).
func businessIDFromParam(c *gin.Context) (uint, bool) {
	raw := c.Param("id")
	id64, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		RespondWithError(c, http.StatusBadRequest, "", "Invalid business ID")
		return 0, false
	}
	return uint(id64), true
}

// directorActorUserID reads the authenticated owner's user_id from the gin
// context, tolerating the JWT float64 / direct uint / int shapes that the
// auth middleware uses (mirrors business_handlers.go staff_id reader).
func directorActorUserID(c *gin.Context) uint {
	raw, ok := c.Get("user_id")
	if !ok {
		return 0
	}
	switch v := raw.(type) {
	case uint:
		return v
	case int:
		return uint(v)
	case int64:
		return uint(v)
	case float64:
		return uint(v)
	}
	return 0
}

// ApplyDirectorAction commits a previously-previewed director proposal.
// POST /api/v1/inside/businesses/:id/ai/director/actions/apply
//
// Security: params are read ONLY from the stored proposal row; the client
// supplies only proposal_id + reconfirm.
func ApplyDirectorAction(c *gin.Context) {
	if directorActionService == nil {
		RespondWithError(c, http.StatusInternalServerError, "", "director action service unavailable")
		return
	}

	businessID, ok := businessIDFromParam(c)
	if !ok {
		return
	}

	var req struct {
		ProposalID string `json:"proposal_id" binding:"required"`
		Reconfirm  bool   `json:"reconfirm"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondWithError(c, http.StatusBadRequest, "", "proposal_id is required")
		return
	}

	res, err := directorActionService.Apply(c.Request.Context(), services.ApplyDirectorActionInput{
		BusinessID:  businessID,
		PublicID:    req.ProposalID,
		ActorUserID: directorActorUserID(c),
		Reconfirm:   req.Reconfirm,
	})
	if err != nil {
		writeDirectorActionError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"applied": true, "result": res})
}

// appliedActionDTO is one row of the "Applied changes" history returned to the
// client. can_undo reflects the SERVER's undo eligibility (window still open,
// not already undone, and the menu hasn't advanced past the applied version) so
// the client renders an Undo button only when the undo would actually succeed.
type appliedActionDTO struct {
	ProposalID string  `json:"proposal_id"`
	Kind       string  `json:"kind"`
	Title      string  `json:"title"`
	AppliedAt  string  `json:"applied_at"`
	UndoneAt   *string `json:"undone_at"`
	CanUndo    bool    `json:"can_undo"`
}

// ListAppliedDirectorActions returns the newest applied director actions for a
// business so the operator can inspect and undo them after a refresh — the
// previous UI stranded that history in ephemeral React state.
// GET /api/v1/inside/businesses/:id/ai/director/actions/applied
func ListAppliedDirectorActions(c *gin.Context) {
	if !ensureOwnerToken(c) {
		return
	}
	businessID, ok := businessIDFromParam(c)
	if !ok {
		return
	}
	if !ensureDirectorFeatureAvailable(c, businessID) {
		return
	}

	rows, err := database.ListAppliedDirectorActions(businessID, 25)
	if err != nil {
		RespondWithError(c, http.StatusInternalServerError, "", "failed to load applied actions")
		return
	}

	// Load the current menu version once; an undo is only eligible while the
	// menu hasn't advanced past the POST-apply version (apply bumps the
	// proposal's previewed version by exactly 1 — mirrors the write-path
	// staleness check) AND we're still inside the undo window AND it isn't
	// already undone. Legacy no-op audits (empty before-snapshot) undo as a
	// safe heal at any version, so they stay eligible.
	var currentMenuVersion uint
	if menu, _, merr := database.GetMenuByBusinessID(businessID); merr == nil && menu != nil {
		currentMenuVersion = menu.Version
	}

	now := time.Now()
	out := make([]appliedActionDTO, 0, len(rows))
	for _, r := range rows {
		canUndo := r.UndoneAt == nil &&
			now.Sub(r.AppliedAt) <= services.DirectorUndoWindow &&
			(r.IsNoOp || r.ProposalMenuVer+1 == currentMenuVersion)
		var undoneAt *string
		if r.UndoneAt != nil {
			s := r.UndoneAt.UTC().Format(time.RFC3339)
			undoneAt = &s
		}
		out = append(out, appliedActionDTO{
			ProposalID: r.PublicID,
			Kind:       r.Kind,
			Title:      directorActionTitle(r.Kind),
			AppliedAt:  r.AppliedAt.UTC().Format(time.RFC3339),
			UndoneAt:   undoneAt,
			CanUndo:    canUndo,
		})
	}

	c.JSON(http.StatusOK, gin.H{"actions": out})
}

// directorActionTitle maps a proposal kind to a short, stable label. The client
// localizes the human copy; this is a machine-friendly fallback so a row is
// never blank if the client lacks a translation for a kind.
func directorActionTitle(kind string) string {
	switch kind {
	case "menu.adjust_prices":
		return "Price change"
	case "menu.set_availability":
		return "Availability change"
	case "menu.edit_content":
		return "Content edit"
	default:
		return kind
	}
}

// ProposeDirectorPriceChange stages a single-item price-change proposal from the
// menu-engineering matrix's "Propose new price" button — a NON-AI entry point
// onto the SAME governed apply/undo rail as the AI director.
// POST /api/v1/inside/businesses/:id/ai/director/actions/propose-price-change
//
// Body: { "menu_item_id": string, "new_price": number } where new_price is an
// absolute DOLLAR target. The service converts it to a flat upward delta,
// computes the preview, and persists a threadless pending proposal. It writes no
// menu data — apply (version-checked) does that later.
func ProposeDirectorPriceChange(c *gin.Context) {
	if directorActionService == nil {
		RespondWithError(c, http.StatusInternalServerError, "", "director action service unavailable")
		return
	}

	businessID, ok := businessIDFromParam(c)
	if !ok {
		return
	}

	var req struct {
		MenuItemID string  `json:"menu_item_id" binding:"required"`
		NewPrice   float64 `json:"new_price" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondWithError(c, http.StatusBadRequest, "", "menu_item_id and new_price are required")
		return
	}

	res, err := directorActionService.ProposePriceChange(c.Request.Context(), services.ProposePriceChangeInput{
		BusinessID: businessID,
		MenuItemID: req.MenuItemID,
		NewPrice:   req.NewPrice,
	})
	if err != nil {
		writeDirectorActionError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"proposal": gin.H{
		"public_id":          res.PublicID,
		"kind":               res.Kind,
		"preview":            res.Preview,
		"warnings":           res.Warnings,
		"requires_reconfirm": res.RequiresReconfirm,
		"title":              res.Title,
		"description":        res.Description,
	}}})
}

// writeDirectorActionError maps service-layer errors to the appropriate HTTP
// status codes and machine-readable error codes.
func writeDirectorActionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrInvalidProposedPrice),
		errors.Is(err, services.ErrNoPriceIncrease):
		// 400: invalid propose-price-change input (non-positive target or no raise).
		RespondWithError(c, http.StatusBadRequest, "", err.Error())

	case errors.Is(err, services.ErrDirectorActionVersionConflict):
		// 409: menu changed since preview — operator must re-preview.
		RespondWithErrorParams(c, http.StatusConflict, "menu_changed",
			"The menu changed since this was previewed — review again.", nil)

	case errors.Is(err, services.ErrDirectorActionNoMatch):
		// 409: the proposal matches no menu items — applying it would be a lie.
		// Distinct code so the client shows an honest message instead of the
		// misleading "menu changed" copy (audit L4-17).
		RespondWithErrorParams(c, http.StatusConflict, "no_items_match",
			"This proposal doesn't match any menu items — there is nothing to apply.", nil)

	case errors.Is(err, services.ErrDirectorActionReconfirmRequired):
		// 428: large swing needs explicit reconfirmation.
		RespondWithErrorParams(c, http.StatusPreconditionRequired, "reconfirm_required",
			"This change is large and needs reconfirmation.", nil)

	case errors.Is(err, services.ErrDirectorActionNotFound):
		RespondWithError(c, http.StatusNotFound, "", "Proposal not found")

	case errors.Is(err, services.ErrDirectorActionNotPending),
		errors.Is(err, services.ErrDirectorActionExpired):
		// 410: proposal is no longer available (already applied, dismissed, or expired).
		RespondWithError(c, http.StatusGone, "", "This proposal is no longer available")

	default:
		RespondWithError(c, http.StatusInternalServerError, "", "Failed to apply action")
	}
}

// UndoDirectorAction reverses a previously-applied action within the window.
// POST /api/v1/inside/businesses/:id/ai/director/actions/undo
//
// Security: refuses if menu version changed since apply (stale) or if the 24h
// window has elapsed.
func UndoDirectorAction(c *gin.Context) {
	if directorActionService == nil {
		RespondWithError(c, http.StatusInternalServerError, "", "director action service unavailable")
		return
	}

	businessID, ok := businessIDFromParam(c)
	if !ok {
		return
	}

	var req struct {
		ProposalID string `json:"proposal_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondWithError(c, http.StatusBadRequest, "", "proposal_id is required")
		return
	}

	res, err := directorActionService.Undo(c.Request.Context(), services.UndoDirectorActionInput{
		BusinessID:  businessID,
		PublicID:    req.ProposalID,
		ActorUserID: directorActorUserID(c),
	})
	if err != nil {
		writeDirectorUndoError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"undone": true, "result": res})
}

// writeDirectorUndoError maps undo service-layer errors to HTTP status codes.
func writeDirectorUndoError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrDirectorActionUndoStale):
		// 409: menu changed since apply — cannot undo safely.
		RespondWithErrorParams(c, http.StatusConflict, "menu_changed_since_apply",
			"The menu changed since this action was applied — undo is no longer safe.", nil)

	case errors.Is(err, services.ErrDirectorActionUndoExpired):
		// 410: 24h window elapsed.
		RespondWithError(c, http.StatusGone, "", "The undo window has elapsed")

	case errors.Is(err, services.ErrDirectorActionAlreadyUndone):
		// 409: already undone.
		RespondWithErrorParams(c, http.StatusConflict, "already_undone",
			"This action has already been undone.", nil)

	case errors.Is(err, services.ErrDirectorActionNotFound):
		RespondWithError(c, http.StatusNotFound, "", "Proposal not found")

	default:
		RespondWithError(c, http.StatusInternalServerError, "", "Failed to undo action")
	}
}
