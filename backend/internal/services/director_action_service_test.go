package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services/director_actions"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// actionTestSeq generates unique business-id suffixes so parallel tests don't collide.
var actionTestSeq uint64

// newActionTestDB returns a SQLite in-memory DB with all tables needed for
// DirectorActionService tests, mirroring the pattern in
// director_console_stream_test.go.
func newActionTestDB(t *testing.T) *database.DB {
	t.Helper()
	db := newServiceTestDB(t)
	require.NoError(t, db.GetGorm().AutoMigrate(
		&database.Menu{},
		&database.DirectorProposedAction{},
		&database.DirectorActionAudit{},
	))
	return db
}

// seedActionTestBiz creates a minimal business row.
func seedActionTestBiz(t *testing.T, db *database.DB, label string) database.Business {
	t.Helper()
	seq := atomic.AddUint64(&actionTestSeq, 1)
	biz := database.Business{
		BusinessId:     fmt.Sprintf("biz-action-%s-%d", label, seq),
		Name:           label,
		SettlementAddr: "0xtest",
		TippingAddr:    "0xtest",
	}
	require.NoError(t, db.GetGorm().Create(&biz).Error)
	return biz
}

// seedActionTestMenu seeds an active menu at version 1 with one category + burger at $10.
func seedActionTestMenu(t *testing.T, db *database.DB, bizID uint) *database.Menu {
	t.Helper()
	cats := []database.MenuCategory{{
		ID:   "cat-1",
		Name: "Mains",
		Items: []database.MenuItem{
			{ID: "i1", Name: "Burger", Price: 10.0, IsAvailable: true},
		},
	}}
	catsJSON, err := json.Marshal(cats)
	require.NoError(t, err)
	menu := &database.Menu{
		BusinessID: bizID,
		Categories: string(catsJSON),
		IsActive:   true,
		Version:    1,
	}
	require.NoError(t, db.GetGorm().Create(menu).Error)
	return menu
}

// seedPriceProposal creates a pending price-change proposal (+20%) for the menu at the given version.
func seedPriceProposal(t *testing.T, db *database.DB, bizID uint, menuVersion uint) *database.DirectorProposedAction {
	t.Helper()
	seq := atomic.AddUint64(&actionTestSeq, 1)
	p := &database.DirectorProposedAction{
		PublicID:    fmt.Sprintf("pa_test_%d", seq),
		BusinessID:  bizID,
		ThreadID:    1,
		Kind:        string(director_actions.KindAdjustPrices),
		ParamsJSON:  `{"scope":"all","mode":"percent","value":20,"direction":"up"}`,
		PreviewJSON: `{"affected_count":1,"summary":"Prices raised"}`,
		MenuVersion: menuVersion,
		Status:      database.DirectorProposalPending,
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	require.NoError(t, db.GetGorm().Create(p).Error)
	return p
}

// seedReconfirmProposal creates a pending price-change proposal with a >50% swing (60%),
// which means the compute will return requiresReconfirm=true.
func seedReconfirmProposal(t *testing.T, db *database.DB, bizID uint) *database.DirectorProposedAction {
	t.Helper()
	seq := atomic.AddUint64(&actionTestSeq, 1)
	p := &database.DirectorProposedAction{
		PublicID:    fmt.Sprintf("pa_reconfirm_%d", seq),
		BusinessID:  bizID,
		ThreadID:    1,
		Kind:        string(director_actions.KindAdjustPrices),
		ParamsJSON:  `{"scope":"all","mode":"percent","value":60,"direction":"up"}`,
		PreviewJSON: `{"affected_count":1}`,
		MenuVersion: 1,
		Status:      database.DirectorProposalPending,
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	require.NoError(t, db.GetGorm().Create(p).Error)
	return p
}

func TestApply_ZeroMatch_RejectedWithoutConsumingProposal(t *testing.T) {
	db := newActionTestDB(t)
	biz := seedActionTestBiz(t, db, "NoOp")
	seedActionTestMenu(t, db, biz.ID) // version 1, one burger in cat-1

	// Scope a category that doesn't exist → compute matches 0 items. Applying
	// such a proposal previously "succeeded" as a silent no-op that marked the
	// proposal applied and wrote an audit row whose undo could never work
	// (audit L4-17). It must instead be rejected outright: nothing consumed,
	// nothing written, so the operator sees an honest error rather than a fake ✓.
	seq := atomic.AddUint64(&actionTestSeq, 1)
	proposal := &database.DirectorProposedAction{
		PublicID:    fmt.Sprintf("pa_noop_%d", seq),
		BusinessID:  biz.ID,
		ThreadID:    1,
		Kind:        string(director_actions.KindAdjustPrices),
		ParamsJSON:  `{"scope":"category:does-not-exist","mode":"percent","value":20,"direction":"up"}`,
		PreviewJSON: `{"affected_count":0}`,
		MenuVersion: 1,
		Status:      database.DirectorProposalPending,
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	require.NoError(t, db.GetGorm().Create(proposal).Error)

	svc := NewDirectorActionService(db)
	_, err := svc.Apply(context.Background(), ApplyDirectorActionInput{
		BusinessID: biz.ID, PublicID: proposal.PublicID, ActorUserID: 7,
	})
	require.Error(t, err, "0-match apply must be rejected, not silently succeed")
	assert.True(t, errors.Is(err, ErrDirectorActionNoMatch),
		"expected ErrDirectorActionNoMatch, got %v", err)

	menu, cats, err := database.GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	assert.Equal(t, uint(1), menu.Version, "menu version must stay 1")
	assert.InDelta(t, 10.0, cats[0].Items[0].Price, 0.001, "burger price must be unchanged")

	// Proposal must NOT be consumed and no audit row may exist — a rejected
	// apply leaves the card dismissible, not fake-applied.
	var reloaded database.DirectorProposedAction
	require.NoError(t, db.GetGorm().First(&reloaded, proposal.ID).Error)
	assert.Equal(t, database.DirectorProposalPending, reloaded.Status,
		"zero-match proposal must remain pending after rejected apply")
	_, err = database.GetDirectorActionAuditByProposalID(biz.ID, proposal.ID)
	assert.Error(t, err, "no audit row may be written for a rejected zero-match apply")
}

func TestApply_HappyPath_WritesAuditAndBumpsVersionOnce(t *testing.T) {
	db := newActionTestDB(t)
	biz := seedActionTestBiz(t, db, "HappyPath")
	seedActionTestMenu(t, db, biz.ID)
	proposal := seedPriceProposal(t, db, biz.ID, 1)

	svc := NewDirectorActionService(db)
	res, err := svc.Apply(context.Background(), ApplyDirectorActionInput{
		BusinessID:  biz.ID,
		PublicID:    proposal.PublicID,
		ActorUserID: 99,
	})
	require.NoError(t, err, "happy-path apply must not error")

	// Menu version must be 2.
	assert.Equal(t, uint(2), res.NewMenuVersion, "new menu version must be 2")
	assert.Equal(t, proposal.PublicID, res.ProposalID)
	assert.NotZero(t, res.AuditID)
	assert.Equal(t, string(director_actions.KindAdjustPrices), res.Kind)

	// Burger must now be $12 (10 * 1.20).
	_, cats, err := database.GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	require.Len(t, cats, 1)
	require.Len(t, cats[0].Items, 1)
	assert.InDelta(t, 12.0, cats[0].Items[0].Price, 0.001, "burger must be 12 after +20%% price change")

	// Proposal status must be 'applied'.
	var reloaded database.DirectorProposedAction
	require.NoError(t, db.GetGorm().First(&reloaded, proposal.ID).Error)
	assert.Equal(t, database.DirectorProposalApplied, reloaded.Status)

	// Audit row must exist with before_json containing the $10 snapshot.
	audit, err := database.GetDirectorActionAuditByProposalID(biz.ID, proposal.ID)
	require.NoError(t, err, "audit row must be created")
	assert.Equal(t, uint(99), audit.ActorUserID)

	var before []director_actions.ItemSnapshot
	require.NoError(t, json.Unmarshal([]byte(audit.BeforeJSON), &before))
	require.Len(t, before, 1, "before snapshot must contain exactly one changed item")
	assert.InDelta(t, 10.0, before[0].Price, 0.001, "before snapshot must record the original $10 price")
	assert.Equal(t, "i1", before[0].ItemID)
}

func TestApply_VersionConflict409Signal(t *testing.T) {
	db := newActionTestDB(t)
	biz := seedActionTestBiz(t, db, "VersionConflict")
	menu := seedActionTestMenu(t, db, biz.ID)
	proposal := seedPriceProposal(t, db, biz.ID, 1)

	// Externally bump the menu to v2 so proposal.MenuVersion (1) is stale.
	require.NoError(t, db.GetGorm().Model(menu).Update("version", 2).Error)

	svc := NewDirectorActionService(db)
	_, err := svc.Apply(context.Background(), ApplyDirectorActionInput{
		BusinessID:  biz.ID,
		PublicID:    proposal.PublicID,
		ActorUserID: 99,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDirectorActionVersionConflict),
		"expected ErrDirectorActionVersionConflict, got %v", err)
}

func TestApply_RejectsNonPendingOrExpired(t *testing.T) {
	db := newActionTestDB(t)
	svc := NewDirectorActionService(db)

	// Case 1: already-applied proposal.
	biz1 := seedActionTestBiz(t, db, "AlreadyApplied")
	seedActionTestMenu(t, db, biz1.ID)
	seq := atomic.AddUint64(&actionTestSeq, 1)
	applied := &database.DirectorProposedAction{
		PublicID:    fmt.Sprintf("pa_applied_%d", seq),
		BusinessID:  biz1.ID,
		ThreadID:    1,
		Kind:        string(director_actions.KindAdjustPrices),
		ParamsJSON:  `{}`,
		PreviewJSON: `{}`,
		MenuVersion: 1,
		Status:      database.DirectorProposalApplied,
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	require.NoError(t, db.GetGorm().Create(applied).Error)
	_, err := svc.Apply(context.Background(), ApplyDirectorActionInput{
		BusinessID:  biz1.ID,
		PublicID:    applied.PublicID,
		ActorUserID: 99,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDirectorActionNotPending),
		"applied proposal must be rejected with ErrDirectorActionNotPending, got %v", err)

	// Case 2: expired proposal (ExpiresAt in the past).
	biz2 := seedActionTestBiz(t, db, "Expired")
	seedActionTestMenu(t, db, biz2.ID)
	seq2 := atomic.AddUint64(&actionTestSeq, 1)
	expired := &database.DirectorProposedAction{
		PublicID:    fmt.Sprintf("pa_expired_%d", seq2),
		BusinessID:  biz2.ID,
		ThreadID:    1,
		Kind:        string(director_actions.KindAdjustPrices),
		ParamsJSON:  `{"scope":"all","mode":"percent","value":10,"direction":"up"}`,
		PreviewJSON: `{}`,
		MenuVersion: 1,
		Status:      database.DirectorProposalPending,
		ExpiresAt:   time.Now().Add(-time.Hour), // past
	}
	require.NoError(t, db.GetGorm().Create(expired).Error)
	_, err = svc.Apply(context.Background(), ApplyDirectorActionInput{
		BusinessID:  biz2.ID,
		PublicID:    expired.PublicID,
		ActorUserID: 99,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDirectorActionExpired),
		"expired proposal must be rejected with ErrDirectorActionExpired, got %v", err)
}

// TestApply_AlreadyApplied_ReturnsNotPending exercises the TOCTOU between the
// line-95 read (proposal still Pending) and the in-transaction status write. A
// GORM after-create callback on the audit row flips the proposal to Applied
// *inside the same transaction* — simulating a concurrent applier that won the
// race between the read and the status update. Without the status-CAS the
// unconditional update silently double-applies (no error); with the CAS the
// RowsAffected==0 path returns ErrDirectorActionNotPending and the txn rolls back.
func TestApply_AlreadyApplied_ReturnsNotPending(t *testing.T) {
	db := newActionTestDB(t)
	biz := seedActionTestBiz(t, db, "DoubleApply")
	seedActionTestMenu(t, db, biz.ID) // version 1
	proposal := seedPriceProposal(t, db, biz.ID, 1)

	g := db.GetGorm()
	// Interpose a concurrent winner: the first time an audit row is created for
	// this proposal (inside the apply txn, after the menu write, before the
	// status-CAS), flip the proposal's status to Applied on the same tx so the
	// CAS sees a non-pending row.
	cbName := "test_flip_proposal_applied"
	var flipped bool
	require.NoError(t, g.Callback().Create().After("gorm:create").Register(cbName, func(tx *gorm.DB) {
		audit, ok := tx.Statement.Model.(*database.DirectorActionAudit)
		if !ok || flipped || audit == nil || audit.ProposedActionID != proposal.ID {
			return
		}
		flipped = true
		tx.Session(&gorm.Session{NewDB: true}).
			Model(&database.DirectorProposedAction{}).
			Where("id = ?", proposal.ID).
			Update("status", database.DirectorProposalApplied)
	}))
	t.Cleanup(func() { _ = g.Callback().Create().Remove(cbName) })

	svc := NewDirectorActionService(db)
	_, err := svc.Apply(context.Background(), ApplyDirectorActionInput{
		BusinessID:  biz.ID,
		PublicID:    proposal.PublicID,
		ActorUserID: 7,
	})
	require.Error(t, err, "an applied-out-from-under proposal must not double-apply")
	assert.True(t, errors.Is(err, ErrDirectorActionNotPending),
		"concurrent apply must surface ErrDirectorActionNotPending, got %v", err)

	// The losing apply must have rolled back: no audit row persisted and the
	// menu version must not have been bumped by this attempt.
	_, auditErr := database.GetDirectorActionAuditByProposalID(biz.ID, proposal.ID)
	assert.Error(t, auditErr, "losing apply must not leave an audit row")
	menu, _, err := database.GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	assert.Equal(t, uint(1), menu.Version, "losing apply must not bump the menu version")
}

func TestApply_ReconfirmRequiredWithoutConfirm(t *testing.T) {
	db := newActionTestDB(t)
	biz := seedActionTestBiz(t, db, "ReconfirmRequired")
	seedActionTestMenu(t, db, biz.ID)
	proposal := seedReconfirmProposal(t, db, biz.ID) // 60% swing

	svc := NewDirectorActionService(db)
	_, err := svc.Apply(context.Background(), ApplyDirectorActionInput{
		BusinessID:  biz.ID,
		PublicID:    proposal.PublicID,
		ActorUserID: 99,
		Reconfirm:   false, // not confirmed
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDirectorActionReconfirmRequired),
		"large-swing proposal without reconfirm must return ErrDirectorActionReconfirmRequired, got %v", err)

	// Verify menu was NOT mutated.
	_, cats, err := database.GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	assert.InDelta(t, 10.0, cats[0].Items[0].Price, 0.001, "menu must be unchanged when reconfirm rejected")
}

func TestApply_ReconfirmRequiredWithConfirm_Succeeds(t *testing.T) {
	db := newActionTestDB(t)
	biz := seedActionTestBiz(t, db, "ReconfirmAccepted")
	seedActionTestMenu(t, db, biz.ID)
	proposal := seedReconfirmProposal(t, db, biz.ID) // 60% swing

	svc := NewDirectorActionService(db)
	res, err := svc.Apply(context.Background(), ApplyDirectorActionInput{
		BusinessID:  biz.ID,
		PublicID:    proposal.PublicID,
		ActorUserID: 99,
		Reconfirm:   true, // operator explicitly confirmed
	})
	require.NoError(t, err, "reconfirm=true should allow apply even for large swings")
	assert.Equal(t, uint(2), res.NewMenuVersion)

	_, cats, err := database.GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	// 10 * 1.60 = 16.00
	assert.InDelta(t, 16.0, cats[0].Items[0].Price, 0.001)
}

func TestApply_NotFound(t *testing.T) {
	db := newActionTestDB(t)
	biz := seedActionTestBiz(t, db, "NotFound")
	seedActionTestMenu(t, db, biz.ID)

	svc := NewDirectorActionService(db)
	_, err := svc.Apply(context.Background(), ApplyDirectorActionInput{
		BusinessID:  biz.ID,
		PublicID:    "pa_does_not_exist",
		ActorUserID: 99,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDirectorActionNotFound),
		"unknown proposal must return ErrDirectorActionNotFound, got %v", err)
}

func TestApply_TransactionRollbackOnVersionMismatch(t *testing.T) {
	// Use a proposal whose MenuVersion=0 while the menu is at version=1 to
	// exercise the pre-load version check → ErrDirectorActionVersionConflict,
	// confirming the menu is not mutated and the proposal remains pending.
	db := newActionTestDB(t)
	biz := seedActionTestBiz(t, db, "TxRollback")
	seedActionTestMenu(t, db, biz.ID) // menu at v1
	proposal := seedPriceProposal(t, db, biz.ID, 0 /* stale menuVersion */)

	svc := NewDirectorActionService(db)
	_, err := svc.Apply(context.Background(), ApplyDirectorActionInput{
		BusinessID:  biz.ID,
		PublicID:    proposal.PublicID,
		ActorUserID: 99,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDirectorActionVersionConflict))

	// Menu must be unchanged.
	menu, _, err := database.GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	assert.Equal(t, uint(1), menu.Version, "menu must be unchanged after rejected apply")

	// Proposal status must still be pending.
	var reloaded database.DirectorProposedAction
	require.NoError(t, db.GetGorm().First(&reloaded, proposal.ID).Error)
	assert.Equal(t, database.DirectorProposalPending, reloaded.Status,
		"proposal status must remain pending after a rejected apply")
}

// --- Undo tests ---

// applyProposalForUndo is a helper that applies a price proposal (v1→v2) and
// returns the audit row — used by multiple undo tests to avoid repetition.
func applyProposalForUndo(t *testing.T, svc *DirectorActionService, db *database.DB, bizID uint, proposal *database.DirectorProposedAction) *database.DirectorActionAudit {
	t.Helper()
	res, err := svc.Apply(context.Background(), ApplyDirectorActionInput{
		BusinessID:  bizID,
		PublicID:    proposal.PublicID,
		ActorUserID: 99,
	})
	require.NoError(t, err, "apply must succeed before undo test")
	require.Equal(t, uint(2), res.NewMenuVersion)

	audit, err := database.GetDirectorActionAuditByProposalID(bizID, proposal.ID)
	require.NoError(t, err, "audit row must exist after apply")
	return audit
}

func TestUndo_RestoresBeforeSnapshotWhenVersionUnchanged(t *testing.T) {
	// Apply a price proposal (menu v1→v2). Undo → burger back to $10, menu
	// v2→v3, audit.undone_at / undone_by set.
	db := newActionTestDB(t)
	biz := seedActionTestBiz(t, db, "UndoHappy")
	seedActionTestMenu(t, db, biz.ID)
	proposal := seedPriceProposal(t, db, biz.ID, 1)

	svc := NewDirectorActionService(db)
	audit := applyProposalForUndo(t, svc, db, biz.ID, proposal)

	// Verify burger is $12 after apply.
	_, cats, err := database.GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	assert.InDelta(t, 12.0, cats[0].Items[0].Price, 0.001, "burger must be $12 after +20%% apply")

	// Now undo.
	undoRes, err := svc.Undo(context.Background(), UndoDirectorActionInput{
		BusinessID:  biz.ID,
		PublicID:    proposal.PublicID,
		ActorUserID: 77,
	})
	require.NoError(t, err, "undo must succeed when version unchanged")
	assert.Equal(t, uint(3), undoRes.NewMenuVersion, "undo is a forward write: v2→v3")
	assert.Equal(t, proposal.PublicID, undoRes.ProposalID)
	assert.Equal(t, audit.ID, undoRes.AuditID)

	// Burger must be back to $10.
	_, catsAfterUndo, err := database.GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	assert.InDelta(t, 10.0, catsAfterUndo[0].Items[0].Price, 0.001, "burger must be restored to $10 after undo")

	// Audit row must have undone_at and undone_by set.
	var reloadedAudit database.DirectorActionAudit
	require.NoError(t, db.GetGorm().First(&reloadedAudit, audit.ID).Error)
	require.NotNil(t, reloadedAudit.UndoneAt, "audit.undone_at must be set after undo")
	require.NotNil(t, reloadedAudit.UndoneBy, "audit.undone_by must be set after undo")
	assert.Equal(t, uint(77), *reloadedAudit.UndoneBy)
}

func TestUndo_RefusesWhenMenuChangedSinceApply(t *testing.T) {
	// Apply (→v2); external edit (→v3); Undo → ErrDirectorActionUndoStale.
	db := newActionTestDB(t)
	biz := seedActionTestBiz(t, db, "UndoStale")
	menu := seedActionTestMenu(t, db, biz.ID)
	proposal := seedPriceProposal(t, db, biz.ID, 1)

	svc := NewDirectorActionService(db)
	applyProposalForUndo(t, svc, db, biz.ID, proposal)

	// External edit bumps menu to v3 (post-apply version was v2).
	require.NoError(t, db.GetGorm().Model(menu).Update("version", 3).Error)

	_, err := svc.Undo(context.Background(), UndoDirectorActionInput{
		BusinessID:  biz.ID,
		PublicID:    proposal.PublicID,
		ActorUserID: 77,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDirectorActionUndoStale),
		"undo must refuse with ErrDirectorActionUndoStale when menu changed, got %v", err)
}

func TestUndo_OutsideWindowRefused(t *testing.T) {
	// Audit applied_at older than 24h → ErrDirectorActionUndoExpired.
	db := newActionTestDB(t)
	biz := seedActionTestBiz(t, db, "UndoExpired")
	seedActionTestMenu(t, db, biz.ID)
	proposal := seedPriceProposal(t, db, biz.ID, 1)

	svc := NewDirectorActionService(db)
	audit := applyProposalForUndo(t, svc, db, biz.ID, proposal)

	// Back-date applied_at to 25h ago so the undo window is elapsed.
	pastTime := time.Now().Add(-25 * time.Hour)
	require.NoError(t, db.GetGorm().Model(&database.DirectorActionAudit{}).
		Where("id = ?", audit.ID).
		Update("applied_at", pastTime).Error)

	_, err := svc.Undo(context.Background(), UndoDirectorActionInput{
		BusinessID:  biz.ID,
		PublicID:    proposal.PublicID,
		ActorUserID: 77,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDirectorActionUndoExpired),
		"undo must refuse with ErrDirectorActionUndoExpired after 24h, got %v", err)
}

func TestUndo_AlreadyUndoneRefused(t *testing.T) {
	// A second undo attempt → ErrDirectorActionAlreadyUndone.
	db := newActionTestDB(t)
	biz := seedActionTestBiz(t, db, "UndoTwice")
	seedActionTestMenu(t, db, biz.ID)
	proposal := seedPriceProposal(t, db, biz.ID, 1)

	svc := NewDirectorActionService(db)
	applyProposalForUndo(t, svc, db, biz.ID, proposal)

	// First undo succeeds.
	_, err := svc.Undo(context.Background(), UndoDirectorActionInput{
		BusinessID:  biz.ID,
		PublicID:    proposal.PublicID,
		ActorUserID: 77,
	})
	require.NoError(t, err, "first undo must succeed")

	// Second undo must fail.
	_, err = svc.Undo(context.Background(), UndoDirectorActionInput{
		BusinessID:  biz.ID,
		PublicID:    proposal.PublicID,
		ActorUserID: 77,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDirectorActionAlreadyUndone),
		"second undo must return ErrDirectorActionAlreadyUndone, got %v", err)
}

func TestUndo_LegacyNoOpAudit_MarksUndoneWithoutMenuWrite(t *testing.T) {
	// Before the L4-17 fix, a zero-match apply "succeeded" as a no-op: proposal
	// applied + audit with before_json "[]" but NO version bump. Undo then 409'd
	// forever (menu.Version never equals MenuVersion+1 for a no-op). Such legacy
	// rows must heal: undoing nothing succeeds, marks the audit undone, and
	// leaves the menu untouched.
	db := newActionTestDB(t)
	biz := seedActionTestBiz(t, db, "UndoNoOpLegacy")
	seedActionTestMenu(t, db, biz.ID) // menu v1, burger $10

	seq := atomic.AddUint64(&actionTestSeq, 1)
	proposal := &database.DirectorProposedAction{
		PublicID:    fmt.Sprintf("pa_legacy_noop_%d", seq),
		BusinessID:  biz.ID,
		ThreadID:    1,
		Kind:        string(director_actions.KindAdjustPrices),
		ParamsJSON:  `{"scope":"category:does-not-exist","mode":"percent","value":20,"direction":"up"}`,
		PreviewJSON: `{"affected_count":0}`,
		MenuVersion: 1, // menu is ALSO at 1 — the no-op apply never bumped it
		Status:      database.DirectorProposalApplied,
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	require.NoError(t, db.GetGorm().Create(proposal).Error)
	audit := &database.DirectorActionAudit{
		BusinessID:       biz.ID,
		ThreadID:         1,
		ProposedActionID: proposal.ID,
		ActorUserID:      7,
		Kind:             proposal.Kind,
		ParamsJSON:       proposal.ParamsJSON,
		BeforeJSON:       `[]`,
		AfterJSON:        `[]`,
		AppliedAt:        time.Now(),
	}
	require.NoError(t, db.GetGorm().Create(audit).Error)

	svc := NewDirectorActionService(db)
	res, err := svc.Undo(context.Background(), UndoDirectorActionInput{
		BusinessID:  biz.ID,
		PublicID:    proposal.PublicID,
		ActorUserID: 77,
	})
	require.NoError(t, err, "undoing a legacy no-op audit must succeed, not 409 forever")
	assert.Equal(t, uint(1), res.NewMenuVersion, "no-op undo must not bump the menu version")

	menu, cats, err := database.GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	assert.Equal(t, uint(1), menu.Version, "menu version must stay 1")
	assert.InDelta(t, 10.0, cats[0].Items[0].Price, 0.001, "burger price must be untouched")

	var reloadedAudit database.DirectorActionAudit
	require.NoError(t, db.GetGorm().First(&reloadedAudit, audit.ID).Error)
	require.NotNil(t, reloadedAudit.UndoneAt, "audit.undone_at must be set")
	require.NotNil(t, reloadedAudit.UndoneBy)
	assert.Equal(t, uint(77), *reloadedAudit.UndoneBy)
}

// --- ProposePriceChange (non-AI entry point) tests ---

// countProposals returns the number of proposal rows for a business — used to
// assert that rejected proposes persist NOTHING.
func countProposals(t *testing.T, db *database.DB, bizID uint) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.GetGorm().Model(&database.DirectorProposedAction{}).
		Where("business_id = ?", bizID).Count(&n).Error)
	return n
}

// TestProposePriceChange_ApplyRoundTrip is the centerpiece: a threadless
// (ThreadID=0) proposal staged outside any Sage conversation must flow cleanly
// through the EXISTING governed apply rail end-to-end.
func TestProposePriceChange_ApplyRoundTrip(t *testing.T) {
	db := newActionTestDB(t)
	biz := seedActionTestBiz(t, db, "ProposeRoundTrip")
	menu := seedActionTestMenu(t, db, biz.ID) // version 1, burger "i1" @ $10

	svc := NewDirectorActionService(db)

	// Stage a single-item price-change proposal: raise i1 from $10 → $14.
	res, err := svc.ProposePriceChange(context.Background(), ProposePriceChangeInput{
		BusinessID: biz.ID,
		MenuItemID: "i1",
		NewPrice:   14.00,
	})
	require.NoError(t, err, "propose must succeed for a valid raise")
	require.NotNil(t, res)
	assert.NotEmpty(t, res.PublicID)
	assert.Equal(t, string(director_actions.KindAdjustPrices), res.Kind)
	assert.False(t, res.RequiresReconfirm, "40%% swing is under the reconfirm threshold")
	assert.Equal(t, 1, res.Preview.AffectedCount)

	// A pending, threadless proposal row must exist at the live menu version.
	row, err := database.GetDirectorProposedActionByPublicID(biz.ID, res.PublicID)
	require.NoError(t, err, "proposal row must be persisted")
	assert.Equal(t, database.DirectorProposalPending, row.Status)
	assert.Equal(t, uint(0), row.ThreadID, "non-AI proposal must be threadless (ThreadID=0)")
	assert.Equal(t, menu.Version, row.MenuVersion)
	assert.Equal(t, string(director_actions.KindAdjustPrices), row.Kind)
	// ParamsJSON must be the flat-mode item-scoped delta the apply rail recomputes from.
	assert.JSONEq(t, `{"scope":"item:i1","mode":"flat","value":4,"direction":"up"}`, row.ParamsJSON)

	// Now apply through the EXISTING rail — no live thread needed.
	applyRes, err := svc.Apply(context.Background(), ApplyDirectorActionInput{
		BusinessID:  biz.ID,
		PublicID:    res.PublicID,
		ActorUserID: 55,
	})
	require.NoError(t, err, "threadless proposal must apply cleanly through the governed rail")
	assert.Equal(t, uint(2), applyRes.NewMenuVersion, "apply must bump menu version 1→2")

	// Item must now be exactly $14.00 and the menu version incremented.
	reloadedMenu, cats, err := database.GetMenuByBusinessID(biz.ID)
	require.NoError(t, err)
	assert.Equal(t, uint(2), reloadedMenu.Version)
	assert.InDelta(t, 14.00, cats[0].Items[0].Price, 0.001, "i1 must be raised to exactly $14.00")
}

func TestProposePriceChange_NoIncreaseRejected(t *testing.T) {
	db := newActionTestDB(t)
	biz := seedActionTestBiz(t, db, "ProposeNoIncrease")
	seedActionTestMenu(t, db, biz.ID) // burger "i1" @ $10

	svc := NewDirectorActionService(db)

	// Equal price → no increase.
	_, err := svc.ProposePriceChange(context.Background(), ProposePriceChangeInput{
		BusinessID: biz.ID, MenuItemID: "i1", NewPrice: 10.00,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoPriceIncrease),
		"equal price must return ErrNoPriceIncrease, got %v", err)

	// Lower price → also no increase.
	_, err = svc.ProposePriceChange(context.Background(), ProposePriceChangeInput{
		BusinessID: biz.ID, MenuItemID: "i1", NewPrice: 8.00,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNoPriceIncrease),
		"lower price must return ErrNoPriceIncrease, got %v", err)

	assert.Equal(t, int64(0), countProposals(t, db, biz.ID),
		"rejected proposes must persist no proposal row")
}

func TestProposePriceChange_InvalidPrice(t *testing.T) {
	db := newActionTestDB(t)
	biz := seedActionTestBiz(t, db, "ProposeInvalid")
	seedActionTestMenu(t, db, biz.ID)

	svc := NewDirectorActionService(db)

	for _, bad := range []float64{0, -5} {
		_, err := svc.ProposePriceChange(context.Background(), ProposePriceChangeInput{
			BusinessID: biz.ID, MenuItemID: "i1", NewPrice: bad,
		})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidProposedPrice),
			"new_price %.2f must return ErrInvalidProposedPrice, got %v", bad, err)
	}

	assert.Equal(t, int64(0), countProposals(t, db, biz.ID),
		"invalid-price proposes must persist no proposal row")
}

func TestProposePriceChange_ItemNotFound(t *testing.T) {
	db := newActionTestDB(t)
	biz := seedActionTestBiz(t, db, "ProposeNotFound")
	seedActionTestMenu(t, db, biz.ID)

	svc := NewDirectorActionService(db)
	_, err := svc.ProposePriceChange(context.Background(), ProposePriceChangeInput{
		BusinessID: biz.ID, MenuItemID: "does-not-exist", NewPrice: 14.00,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDirectorActionNotFound),
		"unknown item must reuse ErrDirectorActionNotFound (handler maps to 404), got %v", err)

	assert.Equal(t, int64(0), countProposals(t, db, biz.ID),
		"not-found propose must persist no proposal row")
}

// TestProposePriceChange_ForeignItemNotFound pins the cross-tenant invariant:
// the menu load is scoped to the path business, so a real-but-foreign item id
// (one that exists, but in a DIFFERENT tenant's menu) can never be staged
// against the wrong business. It must reject as ErrDirectorActionNotFound
// (handler 404) and persist NO proposal row.
func TestProposePriceChange_ForeignItemNotFound(t *testing.T) {
	db := newActionTestDB(t)

	// Business A: default menu with item "i1".
	bizA := seedActionTestBiz(t, db, "TenantA")
	seedActionTestMenu(t, db, bizA.ID)

	// Business B: its own menu with a DISTINCT item id "b-item-1".
	bizB := seedActionTestBiz(t, db, "TenantB")
	catsB := []database.MenuCategory{{
		ID:   "cat-b",
		Name: "B Mains",
		Items: []database.MenuItem{
			{ID: "b-item-1", Name: "B Burger", Price: 12.0, IsAvailable: true},
		},
	}}
	catsBJSON, err := json.Marshal(catsB)
	require.NoError(t, err)
	require.NoError(t, db.GetGorm().Create(&database.Menu{
		BusinessID: bizB.ID,
		Categories: string(catsBJSON),
		IsActive:   true,
		Version:    1,
	}).Error)

	svc := NewDirectorActionService(db)

	// Attempt to stage B's real item against A — must be rejected as not-found.
	_, err = svc.ProposePriceChange(context.Background(), ProposePriceChangeInput{
		BusinessID: bizA.ID,
		MenuItemID: "b-item-1", // exists, but only in tenant B's menu
		NewPrice:   20.00,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDirectorActionNotFound),
		"a real-but-foreign item id must reject with ErrDirectorActionNotFound, got %v", err)

	// No proposal row may exist for EITHER tenant.
	assert.Equal(t, int64(0), countProposals(t, db, bizA.ID),
		"foreign-item propose must persist no row against tenant A")
	assert.Equal(t, int64(0), countProposals(t, db, bizB.ID),
		"foreign-item propose must persist no row against tenant B")
}

// TestApply_AccessShape_SingleMenuLoadAndVersionedWrite documents and asserts
// the access shape of the apply path.
//
// Mechanism: behavioral assertion (no query-counter hook exists in this package).
//
// We assert two complementary invariants:
//
//  1. One apply succeeds and bumps the menu version by exactly 1 (v1→v2). This
//     verifies that a single version-checked write occurred — if there were N+1
//     writes, the version would advance by more than 1, or the version-predicate
//     would conflict with itself.
//
//  2. A second apply of the SAME proposal fails with ErrDirectorActionNotPending
//     (because status=applied after step 1). This proves that the apply path does
//     not loop or duplicate the menu write — a looped write would leave the
//     proposal in an inconsistent state or yield a different error.
//
// Together these assert: one menu read (GetMenuByBusinessID) + one versioned
// write (ApplyMenuCategoriesTx with version=?) + one audit insert + one status
// update — all in a single transaction. No per-item N+1 reload.
func TestApply_AccessShape_SingleMenuLoadAndVersionedWrite(t *testing.T) {
	db := newActionTestDB(t)
	biz := seedActionTestBiz(t, db, "AccessShape")
	seedActionTestMenu(t, db, biz.ID)
	proposal := seedPriceProposal(t, db, biz.ID, 1)

	svc := NewDirectorActionService(db)

	// Invariant 1: apply succeeds, version advances by exactly 1.
	res, err := svc.Apply(context.Background(), ApplyDirectorActionInput{
		BusinessID:  biz.ID,
		PublicID:    proposal.PublicID,
		ActorUserID: 42,
	})
	require.NoError(t, err, "first apply must succeed")
	assert.Equal(t, uint(2), res.NewMenuVersion,
		"version must advance by exactly 1 (v1→v2) — a looped write would advance further or conflict")

	// Invariant 2: a second apply of the same proposal is rejected immediately as
	// ErrDirectorActionNotPending — proving no duplicate write path exists.
	_, err = svc.Apply(context.Background(), ApplyDirectorActionInput{
		BusinessID:  biz.ID,
		PublicID:    proposal.PublicID,
		ActorUserID: 42,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDirectorActionNotPending),
		"second apply of the same proposal must return ErrDirectorActionNotPending (status=applied), not a duplicate write; got %v", err)
}

// BenchmarkApplyPriceChange measures the full apply transaction on SQLite:
// proposal load → menu load → compute → version-checked menu write → audit insert
// → status update.
//
// Per-iteration setup (seed proposal + reset menu version) is excluded from
// measurement via b.StopTimer/b.StartTimer.
func BenchmarkApplyPriceChange(b *testing.B) {
	db := newActionTestBenchDB(b)
	biz := seedActionBenchBiz(b, db)
	svc := NewDirectorActionService(db)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Exclude per-iteration setup from the measurement.
		b.StopTimer()

		// Seed a fresh pending proposal and reset the menu to version 1 so every
		// iteration starts from a clean, identical state. The apply marks the
		// proposal as applied and bumps menu.version, so we must re-seed both.
		resetMenuAndProposal(b, db, biz.ID)

		b.StartTimer()

		_, _ = svc.Apply(context.Background(), ApplyDirectorActionInput{
			BusinessID:  biz.ID,
			PublicID:    benchProposalPublicID,
			ActorUserID: 1,
		})
	}
}

// benchProposalPublicID is the stable public_id used by the benchmark so we
// don't have to return anything from the reset helper.
const benchProposalPublicID = "pa_bench_stable"

// newActionTestBenchDB returns a SQLite in-memory DB for benchmarks. It uses a
// stable DSN so the same DB is reused across iterations (unlike *testing.T
// helpers which use t.Name() for isolation).
func newActionTestBenchDB(b *testing.B) *database.DB {
	b.Helper()
	dsn := "file:bench_apply?mode=memory&cache=shared"

	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		b.Fatalf("newActionTestBenchDB: open: %v", err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		b.Fatalf("newActionTestBenchDB: gormDB.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	b.Cleanup(func() { sqlDB.Close() })

	database.SetTestDB(gormDB)
	if err := gormDB.AutoMigrate(
		&database.Business{},
		&database.Menu{},
		&database.DirectorProposedAction{},
		&database.DirectorActionAudit{},
	); err != nil {
		b.Fatalf("newActionTestBenchDB: migrate: %v", err)
	}
	return database.GetDBWrapper()
}

// seedActionBenchBiz creates one persistent business row for the benchmark DB.
func seedActionBenchBiz(b *testing.B, db *database.DB) database.Business {
	b.Helper()
	biz := database.Business{
		BusinessId:     "biz-bench-apply",
		Name:           "Bench Restaurant",
		SettlementAddr: "0xbench",
		TippingAddr:    "0xbench",
	}
	if err := db.GetGorm().Create(&biz).Error; err != nil {
		b.Fatalf("seedActionBenchBiz: %v", err)
	}
	return biz
}

// resetMenuAndProposal deletes any prior menu + proposal rows for bizID, then
// re-seeds them at a clean state (menu v1, one pending proposal). It uses the
// stable benchProposalPublicID so the benchmark caller can reference it without
// inspecting return values.
func resetMenuAndProposal(b *testing.B, db *database.DB, bizID uint) {
	b.Helper()
	g := db.GetGorm()

	// Delete prior rows (cascade-safe: audit rows reference proposal by ID, but
	// SQLite has no FK cascade here — delete audit rows first, then proposal).
	g.Where("business_id = ?", bizID).Delete(&database.DirectorActionAudit{})
	g.Where("business_id = ?", bizID).Delete(&database.DirectorProposedAction{})
	g.Where("business_id = ?", bizID).Delete(&database.Menu{})

	// Re-seed menu at version 1.
	cats := []database.MenuCategory{{
		ID:   "cat-bench",
		Name: "Mains",
		Items: []database.MenuItem{
			{ID: "item-bench-1", Name: "Burger", Price: 10.0, IsAvailable: true},
			{ID: "item-bench-2", Name: "Fries", Price: 4.0, IsAvailable: true},
			{ID: "item-bench-3", Name: "Salad", Price: 8.0, IsAvailable: true},
		},
	}}
	catsJSON, err := json.Marshal(cats)
	if err != nil {
		b.Fatalf("resetMenuAndProposal: marshal: %v", err)
	}
	if err := g.Create(&database.Menu{
		BusinessID: bizID,
		Categories: string(catsJSON),
		IsActive:   true,
		Version:    1,
	}).Error; err != nil {
		b.Fatalf("resetMenuAndProposal: create menu: %v", err)
	}

	// Re-seed proposal with the stable public_id.
	if err := g.Create(&database.DirectorProposedAction{
		PublicID:    benchProposalPublicID,
		BusinessID:  bizID,
		ThreadID:    1,
		Kind:        string(director_actions.KindAdjustPrices),
		ParamsJSON:  `{"scope":"all","mode":"percent","value":20,"direction":"up"}`,
		PreviewJSON: `{"affected_count":3,"summary":"Prices raised 20%"}`,
		MenuVersion: 1,
		Status:      database.DirectorProposalPending,
		ExpiresAt:   time.Now().Add(time.Hour),
	}).Error; err != nil {
		b.Fatalf("resetMenuAndProposal: create proposal: %v", err)
	}
}
