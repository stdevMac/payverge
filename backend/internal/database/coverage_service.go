package database

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ---- status / kind enums (string consts; see contracts §3) ----
const (
	OpenClaimStatusPending   = "pending"
	OpenClaimStatusApproved  = "approved"
	OpenClaimStatusDenied    = "denied"
	OpenClaimStatusWithdrawn = "withdrawn"

	SwapStatusOpen            = "open"
	SwapStatusAccepted        = "accepted"         // swap matched by a coworker, awaiting manager
	SwapStatusPendingApproval = "pending_approval" // giveup (or post-accept) awaiting manager
	SwapStatusApproved        = "approved"
	SwapStatusDenied          = "denied"
	SwapStatusCancelled       = "cancelled"

	SwapKindSwap   = "swap"
	SwapKindGiveup = "giveup"

	SwapTargetAllInRole = "all_in_role"
	SwapTargetSpecific  = "specific"
)

// RBAC audit actions for the coverage approval trail (contracts §9 lets a slice
// mint a domain-appropriate Action; RBACAction is a string alias).
const (
	RBACActionCoverageClaimed       RBACAction = "coverage_claimed"
	RBACActionCoverageSwapRequested RBACAction = "coverage_swap_requested"
	RBACActionCoverageAccepted      RBACAction = "coverage_accepted"
	RBACActionCoverageDecided       RBACAction = "coverage_decided"
	RBACActionCoverageCancelled     RBACAction = "coverage_cancelled" // staff retracts their own request
)

// OpenShiftClaim records a staff member's claim on an open (staff_id NULL) shift.
// The gorm tags encode the genesis indexes verbatim
// (idx_open_claims_business_shift, idx_open_claims_business_status); the model
// has an explicit TableName().
type OpenShiftClaim struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	BusinessID       uint       `gorm:"index:idx_open_claims_business_shift;index:idx_open_claims_business_status;not null" json:"business_id"`
	ShiftID          uint       `gorm:"index:idx_open_claims_business_shift;not null" json:"shift_id"`
	ClaimingStaffID  uint       `gorm:"not null" json:"claiming_staff_id"`
	Status           string     `gorm:"index:idx_open_claims_business_status;not null;default:'pending'" json:"status"`
	DecidedByStaffID *uint      `json:"decided_by_staff_id"`
	DecidedAt        *time.Time `json:"decided_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (OpenShiftClaim) TableName() string { return "open_shift_claims" }

// ShiftSwapRequest is the swap/giveup record + its approval state machine.
// State machine (slice 5): open → accepted(by an eligible coworker) →
// pending_approval → approved|denied. kind=giveup skips the accept step
// (straight to the manager queue). EVERY transition writes an RBACAuditLog row.
// Genesis-safety: the gorm tags encode the 000107 DDL indexes verbatim
// (idx_swaps_business_status, idx_swaps_business_shift).
type ShiftSwapRequest struct {
	ID                uint       `gorm:"primaryKey" json:"id"`
	BusinessID        uint       `gorm:"index:idx_swaps_business_status;index:idx_swaps_business_shift;not null" json:"business_id"`
	ShiftID           uint       `gorm:"index:idx_swaps_business_shift;not null" json:"shift_id"`
	RequestingStaffID uint       `gorm:"not null" json:"requesting_staff_id"`
	Kind              string     `gorm:"not null" json:"kind"`
	Target            string     `gorm:"not null;default:'all_in_role'" json:"target"`
	TargetStaffID     *uint      `json:"target_staff_id"`
	Status            string     `gorm:"index:idx_swaps_business_status;not null;default:'open'" json:"status"`
	AcceptingStaffID  *uint      `json:"accepting_staff_id"`
	ApprovedByStaffID *uint      `json:"approved_by_staff_id"`
	CreatedAt         time.Time  `json:"created_at"`
	ResolvedAt        *time.Time `json:"resolved_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func (ShiftSwapRequest) TableName() string { return "shift_swap_requests" }

// Sentinel errors mapped to HTTP by the handler.
var (
	ErrCoverageNotFound = errors.New("coverage record not found")
	// ErrCoverageExpired is returned when the underlying shift's ends_at is
	// already in the past. Normal Approve/Deny must fail closed after expiry.
	ErrCoverageExpired  = errors.New("coverage request has expired")
	ErrShiftNotOpen     = errors.New("shift is not open")
	ErrNotEligible      = errors.New("staff not eligible for this position")
	ErrSelfCoverage     = errors.New("cannot cover your own shift")
	ErrCoverageConflict = errors.New("coverage state changed concurrently") // double-accept/decide/fill, illegal transition
	ErrAlreadyClaimed   = errors.New("you already have a pending claim on this shift")
	ErrNotOwner         = errors.New("not the request owner") // only the requester/claimant may cancel their own
)

// swapTransitions is the legal directed edge set for ShiftSwapRequest.Status.
var swapTransitions = map[string]map[string]bool{
	SwapStatusOpen:            {SwapStatusAccepted: true, SwapStatusCancelled: true},
	SwapStatusAccepted:        {SwapStatusPendingApproval: true, SwapStatusCancelled: true},
	SwapStatusPendingApproval: {SwapStatusApproved: true, SwapStatusDenied: true, SwapStatusCancelled: true},
	// approved/denied/cancelled are terminal (no outgoing edges).
}

func validSwapTransition(from, to string) bool { return swapTransitions[from][to] }

// claimTransitions is the legal edge set for OpenShiftClaim.Status.
var claimTransitions = map[string]map[string]bool{
	OpenClaimStatusPending: {OpenClaimStatusApproved: true, OpenClaimStatusDenied: true, OpenClaimStatusWithdrawn: true},
}

func validClaimTransition(from, to string) bool { return claimTransitions[from][to] }

// IsStaffEligibleForPosition is true iff a StaffPosition row links staff↔position
// within the business — the role-eligibility gate for claiming/accepting.
func (d *DB) IsStaffEligibleForPosition(businessID, staffID, positionID uint) (bool, error) {
	var n int64
	err := d.GetGorm().Model(&StaffPosition{}).
		Where("business_id = ? AND staff_id = ? AND position_id = ?", businessID, staffID, positionID).
		Limit(1).Count(&n).Error
	return n > 0, err
}

// writeCoverageAudit appends an RBACAuditLog row inside the caller's transaction,
// so the audit trail commits ATOMICALLY with the state transition — it is NOT
// best-effort: on Postgres a failed audit INSERT aborts the whole transaction and
// rolls the transition back. The returned error is discarded only to keep the
// call sites terse; the atomicity is enforced by the surrounding tx, not here.
func writeCoverageAudit(tx *gorm.DB, businessID, staffID uint, action RBACAction, changedBy, reason string) {
	_ = tx.Create(&RBACAuditLog{
		StaffID: staffID, BusinessID: businessID, Action: action,
		ChangedBy: changedBy, Reason: reason, CreatedAt: time.Now().UTC(),
	}).Error
}

// loadShiftForCoverage loads a shift tenant-scoped, or ErrCoverageNotFound.
func loadShiftForCoverage(tx *gorm.DB, businessID, shiftID uint) (*Shift, error) {
	var s Shift
	if err := tx.Where("id = ? AND business_id = ?", shiftID, businessID).First(&s).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCoverageNotFound
		}
		return nil, err
	}
	return &s, nil
}

// txStaffEligible is IsStaffEligibleForPosition bound to an open transaction.
func txStaffEligible(tx *gorm.DB, businessID, staffID, positionID uint) (bool, error) {
	var n int64
	err := tx.Model(&StaffPosition{}).
		Where("business_id = ? AND staff_id = ? AND position_id = ?", businessID, staffID, positionID).
		Limit(1).Count(&n).Error
	return n > 0, err
}

// ClaimOpenShift records a pending claim on an OPEN shift by an eligible staff
// member. Guards: shift must exist + be open, staff must hold the shift's
// position, and at most one PENDING claim per (business, shift, staff). The
// double-claim guard is ATOMIC: rather than a racy count-then-insert, the insert
// rides the partial unique index idx_open_claims_one_pending via ON CONFLICT DO
// NOTHING — a concurrent duplicate inserts zero rows (RowsAffected==0) →
// ErrAlreadyClaimed, with no window for two same-staff claims to both succeed
// under Postgres Read Committed. Writes an audit row.
func (d *DB) ClaimOpenShift(businessID, shiftID, staffID uint, changedBy string) (*OpenShiftClaim, error) {
	var out OpenShiftClaim
	err := d.GetGorm().Transaction(func(tx *gorm.DB) error {
		shift, err := loadShiftForCoverage(tx, businessID, shiftID)
		if err != nil {
			return err
		}
		if shift.Status != ShiftStatusOpen {
			return ErrShiftNotOpen
		}
		ok, err := txStaffEligible(tx, businessID, staffID, shift.PositionID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrNotEligible
		}
		out = OpenShiftClaim{BusinessID: businessID, ShiftID: shiftID, ClaimingStaffID: staffID, Status: OpenClaimStatusPending}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&out)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// The partial unique index rejected the duplicate pending claim.
			return ErrAlreadyClaimed
		}
		writeCoverageAudit(tx, businessID, staffID, RBACActionCoverageClaimed, changedBy,
			fmt.Sprintf("open-shift claim: shift %d by staff %d", shiftID, staffID))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// RequestSwap creates a swap (kind=swap → status open, awaiting an eligible
// coworker) or a giveup (kind=giveup → status pending_approval, straight to the
// manager queue). The requester must currently own the (filled) shift.
func (d *DB) RequestSwap(businessID, shiftID, staffID uint, kind, target string, targetStaffID *uint, changedBy string) (*ShiftSwapRequest, error) {
	if kind != SwapKindSwap && kind != SwapKindGiveup {
		return nil, ErrCoverageConflict
	}
	if target != SwapTargetAllInRole && target != SwapTargetSpecific {
		target = SwapTargetAllInRole
	}
	var out ShiftSwapRequest
	err := d.GetGorm().Transaction(func(tx *gorm.DB) error {
		shift, err := loadShiftForCoverage(tx, businessID, shiftID)
		if err != nil {
			return err
		}
		if shift.StaffID == nil || *shift.StaffID != staffID {
			return ErrSelfCoverage // only the assigned owner may give up / swap their shift
		}
		status := SwapStatusOpen
		if kind == SwapKindGiveup {
			status = SwapStatusPendingApproval // skips the accept step
		}
		out = ShiftSwapRequest{
			BusinessID: businessID, ShiftID: shiftID, RequestingStaffID: staffID,
			Kind: kind, Target: target, TargetStaffID: targetStaffID, Status: status,
		}
		if err := tx.Create(&out).Error; err != nil {
			return err
		}
		writeCoverageAudit(tx, businessID, staffID, RBACActionCoverageSwapRequested, changedBy,
			fmt.Sprintf("%s requested: shift %d by staff %d", kind, shiftID, staffID))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// CancelSwap lets the REQUESTER retract their own swap/giveup while it is still
// non-terminal (open / accepted / pending_approval). It never touches the shift: a
// non-approved request never moved it (reassignment happens only in
// applySwapApproval), so cancelling simply closes the request. Only the requester
// may cancel — the acceptor of a swap is not yet bound and has no cancel path. The
// atomic UPDATE precondition (requester + non-terminal status) makes a race with a
// concurrent manager decision lose cleanly (RowsAffected != 1 → ErrCoverageConflict).
func (d *DB) CancelSwap(businessID, swapID, staffID uint, changedBy string) (*ShiftSwapRequest, error) {
	var out ShiftSwapRequest
	err := d.GetGorm().Transaction(func(tx *gorm.DB) error {
		swap, err := loadSwap(tx, businessID, swapID)
		if err != nil {
			return err
		}
		if swap.RequestingStaffID != staffID {
			return ErrNotOwner // only the requester may cancel their own request
		}
		if !validSwapTransition(swap.Status, SwapStatusCancelled) {
			return ErrCoverageConflict // terminal source — nothing to cancel
		}
		now := time.Now().UTC()
		res := tx.Model(&ShiftSwapRequest{}).
			Where("id = ? AND business_id = ? AND requesting_staff_id = ? AND status IN ?",
				swapID, businessID, staffID,
				[]string{SwapStatusOpen, SwapStatusAccepted, SwapStatusPendingApproval}).
			Updates(map[string]interface{}{"status": SwapStatusCancelled, "resolved_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrCoverageConflict // raced a manager decision or another cancel
		}
		writeCoverageAudit(tx, businessID, staffID, RBACActionCoverageCancelled, changedBy,
			fmt.Sprintf("%s cancelled: swap %d shift %d by staff %d", swap.Kind, swapID, swap.ShiftID, staffID))
		return tx.Where("id = ?", swapID).First(&out).Error
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// WithdrawClaim lets the CLAIMANT retract their own still-pending open-shift claim.
// The shift is never touched (a pending claim never filled it). Only the claimant
// may withdraw; the atomic precondition (claimant + status=pending) loses cleanly
// to a concurrent manager decision (RowsAffected != 1 → ErrCoverageConflict).
func (d *DB) WithdrawClaim(businessID, claimID, staffID uint, changedBy string) (*OpenShiftClaim, error) {
	var out OpenShiftClaim
	err := d.GetGorm().Transaction(func(tx *gorm.DB) error {
		var claim OpenShiftClaim
		if err := tx.Where("id = ? AND business_id = ?", claimID, businessID).First(&claim).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCoverageNotFound
			}
			return err
		}
		if claim.ClaimingStaffID != staffID {
			return ErrNotOwner
		}
		if !validClaimTransition(claim.Status, OpenClaimStatusWithdrawn) {
			return ErrCoverageConflict // only a pending claim can be withdrawn
		}
		now := time.Now().UTC()
		res := tx.Model(&OpenShiftClaim{}).
			Where("id = ? AND business_id = ? AND claiming_staff_id = ? AND status = ?",
				claimID, businessID, staffID, OpenClaimStatusPending).
			Updates(map[string]interface{}{"status": OpenClaimStatusWithdrawn, "decided_by_staff_id": staffID, "decided_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrCoverageConflict
		}
		writeCoverageAudit(tx, businessID, staffID, RBACActionCoverageCancelled, changedBy,
			fmt.Sprintf("open-claim withdrawn: claim %d shift %d by staff %d", claimID, claim.ShiftID, staffID))
		return tx.Where("id = ?", claimID).First(&out).Error
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// loadSwap loads a swap tenant-scoped, or ErrCoverageNotFound.
func loadSwap(tx *gorm.DB, businessID, swapID uint) (*ShiftSwapRequest, error) {
	var s ShiftSwapRequest
	if err := tx.Where("id = ? AND business_id = ?", swapID, businessID).First(&s).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCoverageNotFound
		}
		return nil, err
	}
	return &s, nil
}

// AcceptSwap is an eligible coworker taking an OPEN swap. It performs the two
// validated edges open→accepted→pending_approval in one transaction; the
// open→accepted UPDATE carries the status precondition that makes a concurrent
// double-accept lose (RowsAffected==0 → ErrCoverageConflict).
func (d *DB) AcceptSwap(businessID, swapID, acceptingStaffID uint, changedBy string) (*ShiftSwapRequest, error) {
	var out ShiftSwapRequest
	err := d.GetGorm().Transaction(func(tx *gorm.DB) error {
		swap, err := loadSwap(tx, businessID, swapID)
		if err != nil {
			return err
		}
		if swap.Kind != SwapKindSwap {
			return ErrCoverageConflict // giveups have no accept step
		}
		if swap.RequestingStaffID == acceptingStaffID {
			return ErrSelfCoverage
		}
		// A swap directed at a SPECIFIC coworker may be accepted only by that
		// coworker. Other staff in the same role are position-eligible but must not
		// be able to hijack a swap that was offered to someone else.
		if swap.Target == SwapTargetSpecific {
			if swap.TargetStaffID == nil || *swap.TargetStaffID != acceptingStaffID {
				return ErrNotEligible
			}
		}
		shift, err := loadShiftForCoverage(tx, businessID, swap.ShiftID)
		if err != nil {
			return err
		}
		ok, err := txStaffEligible(tx, businessID, acceptingStaffID, shift.PositionID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrNotEligible
		}
		if !validSwapTransition(swap.Status, SwapStatusAccepted) {
			return ErrCoverageConflict
		}
		// Edge 1: open → accepted, atomic precondition = still open (double-accept guard).
		res := tx.Model(&ShiftSwapRequest{}).
			Where("id = ? AND business_id = ? AND status = ?", swapID, businessID, SwapStatusOpen).
			Updates(map[string]interface{}{"status": SwapStatusAccepted, "accepting_staff_id": acceptingStaffID})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrCoverageConflict
		}
		// Edge 2: accepted → pending_approval (auto-advance into the manager queue).
		if err := tx.Model(&ShiftSwapRequest{}).
			Where("id = ? AND business_id = ? AND status = ?", swapID, businessID, SwapStatusAccepted).
			Update("status", SwapStatusPendingApproval).Error; err != nil {
			return err
		}
		writeCoverageAudit(tx, businessID, swap.RequestingStaffID, RBACActionCoverageAccepted, changedBy,
			fmt.Sprintf("swap accepted: shift %d staff %d→%d", swap.ShiftID, swap.RequestingStaffID, acceptingStaffID))
		return tx.Where("id = ?", swapID).First(&out).Error
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DecideSwap approves/denies a swap (source accepted) or giveup (source
// pending_approval). On approve it reassigns the shift: a swap moves it to the
// acceptor; a giveup converts it to open. The status UPDATE precondition guards
// concurrent double-decision; the shift UPDATE precondition guards stale ownership.
func (d *DB) DecideSwap(businessID, swapID, deciderStaffID uint, approve bool, changedBy string) (*ShiftSwapRequest, error) {
	var out ShiftSwapRequest
	err := d.GetGorm().Transaction(func(tx *gorm.DB) error {
		swap, err := loadSwap(tx, businessID, swapID)
		if err != nil {
			return err
		}
		target := SwapStatusDenied
		if approve {
			target = SwapStatusApproved
		}
		// A swap is decidable by a manager from EITHER accepted (a coworker
		// matched) OR pending_approval (a giveup, or a post-accept auto-advance) —
		// design decision #1. This source-decidability gate mirrors the SQL
		// precondition below; open/terminal sources are rejected. We do NOT use
		// validSwapTransition here because accepted→approved is intentionally not a
		// modeled edge (accepted only advances to pending_approval in the locked
		// transition table), yet an accepted swap is still a valid decision source.
		if swap.Status != SwapStatusAccepted && swap.Status != SwapStatusPendingApproval {
			return ErrCoverageConflict // open/terminal sources are not decidable
		}
		shift, err := loadShiftForCoverage(tx, businessID, swap.ShiftID)
		if err != nil {
			return err
		}
		if InstantElapsed(shift.EndsAt, time.Now().UTC()) {
			return ErrCoverageExpired
		}
		now := time.Now().UTC()
		res := tx.Model(&ShiftSwapRequest{}).
			Where("id = ? AND business_id = ? AND status IN ?", swapID, businessID,
				[]string{SwapStatusAccepted, SwapStatusPendingApproval}).
			Updates(map[string]interface{}{"status": target, "approved_by_staff_id": deciderStaffID, "resolved_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrCoverageConflict // double-decision
		}
		if approve {
			if err := applySwapApproval(tx, businessID, swap); err != nil {
				return err
			}
		}
		writeCoverageAudit(tx, businessID, swap.RequestingStaffID, RBACActionCoverageDecided, changedBy,
			fmt.Sprintf("%s %s: shift %d (req staff %d)", swap.Kind, target, swap.ShiftID, swap.RequestingStaffID))
		return tx.Where("id = ?", swapID).First(&out).Error
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// applySwapApproval mutates the shift for an approved swap/giveup, guarded by a
// stale-ownership precondition (still owned by the requester).
func applySwapApproval(tx *gorm.DB, businessID uint, swap *ShiftSwapRequest) error {
	q := tx.Model(&Shift{}).
		Where("id = ? AND business_id = ? AND staff_id = ?", swap.ShiftID, businessID, swap.RequestingStaffID)
	var res *gorm.DB
	if swap.Kind == SwapKindGiveup {
		res = q.Updates(map[string]interface{}{"staff_id": nil, "status": ShiftStatusOpen})
	} else { // swap → acceptor
		if swap.AcceptingStaffID == nil {
			return ErrCoverageConflict
		}
		res = q.Updates(map[string]interface{}{"staff_id": *swap.AcceptingStaffID, "status": ShiftStatusFilled})
	}
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrCoverageConflict // shift changed hands underneath us
	}
	return nil
}

// DecideClaim approves/denies an open-shift claim. On approve it fills the shift
// (precondition: still open → double-fill guard) and auto-denies sibling pending
// claims on the same shift.
func (d *DB) DecideClaim(businessID, claimID, deciderStaffID uint, approve bool, changedBy string) (*OpenShiftClaim, error) {
	var out OpenShiftClaim
	err := d.GetGorm().Transaction(func(tx *gorm.DB) error {
		var claim OpenShiftClaim
		if err := tx.Where("id = ? AND business_id = ?", claimID, businessID).First(&claim).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCoverageNotFound
			}
			return err
		}
		target := OpenClaimStatusDenied
		if approve {
			target = OpenClaimStatusApproved
		}
		if !validClaimTransition(claim.Status, target) {
			return ErrCoverageConflict
		}
		shift, err := loadShiftForCoverage(tx, businessID, claim.ShiftID)
		if err != nil {
			return err
		}
		if InstantElapsed(shift.EndsAt, time.Now().UTC()) {
			return ErrCoverageExpired
		}
		now := time.Now().UTC()
		res := tx.Model(&OpenShiftClaim{}).
			Where("id = ? AND business_id = ? AND status = ?", claimID, businessID, OpenClaimStatusPending).
			Updates(map[string]interface{}{"status": target, "decided_by_staff_id": deciderStaffID, "decided_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrCoverageConflict // double-decision
		}
		if approve {
			fill := tx.Model(&Shift{}).
				Where("id = ? AND business_id = ? AND status = ?", claim.ShiftID, businessID, ShiftStatusOpen).
				Updates(map[string]interface{}{"staff_id": claim.ClaimingStaffID, "status": ShiftStatusFilled})
			if fill.Error != nil {
				return fill.Error
			}
			if fill.RowsAffected != 1 {
				return ErrCoverageConflict // shift already filled by another approval
			}
			// auto-deny sibling pending claims on the now-filled shift
			if err := tx.Model(&OpenShiftClaim{}).
				Where("business_id = ? AND shift_id = ? AND status = ? AND id <> ?",
					businessID, claim.ShiftID, OpenClaimStatusPending, claimID).
				Updates(map[string]interface{}{"status": OpenClaimStatusDenied, "decided_by_staff_id": deciderStaffID, "decided_at": now}).Error; err != nil {
				return err
			}
		}
		writeCoverageAudit(tx, businessID, claim.ClaimingStaffID, RBACActionCoverageDecided, changedBy,
			fmt.Sprintf("open-claim %s: shift %d staff %d", target, claim.ShiftID, claim.ClaimingStaffID))
		return tx.Where("id = ?", claimID).First(&out).Error
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

const coverageListMax = 200 // bounded result set (perf gate)

type OpenShiftItem struct {
	ShiftID      uint      `json:"shift_id"`
	PositionID   uint      `json:"position_id"`
	StartsAt     time.Time `json:"starts_at"`
	EndsAt       time.Time `json:"ends_at"`
	BreakMinutes int       `json:"break_minutes"`
}

type SwapInboxItem struct {
	SwapID            uint      `json:"swap_id"`
	ShiftID           uint      `json:"shift_id"`
	RequestingStaffID uint      `json:"requesting_staff_id"`
	PositionID        uint      `json:"position_id"`
	StartsAt          time.Time `json:"starts_at"`
	EndsAt            time.Time `json:"ends_at"`
}

type PendingApprovalItem struct {
	ID                uint      `json:"id"`
	ShiftID           uint      `json:"shift_id"`
	PositionID        uint      `json:"position_id"`
	Kind              string    `json:"kind"`
	Status            string    `json:"status"`
	RequestingStaffID uint      `json:"requesting_staff_id"`
	AcceptingStaffID  *uint     `json:"accepting_staff_id"`
	ClaimingStaffID   *uint     `json:"claiming_staff_id"`
	StartsAt          time.Time `json:"starts_at"`
	EndsAt            time.Time `json:"ends_at"`
}

type OpenCoverageResult struct {
	OpenShifts       []OpenShiftItem `json:"open_shifts"`
	SwapInbox        []SwapInboxItem `json:"swap_inbox"`
	PendingApprovals struct {
		Swaps  []PendingApprovalItem `json:"swaps"`
		Claims []PendingApprovalItem `json:"claims"`
	} `json:"pending_approvals"`
}

// ListOpenCoverage returns the staff member's eligible open shifts + swap inbox.
// When isApprover is true it ALSO returns the business-wide pending-approval
// queue (manager surface). All reads are narrow-projection joins, bounded.
func (d *DB) ListOpenCoverage(businessID, staffID uint, isApprover bool) (*OpenCoverageResult, error) {
	g := d.GetGorm()
	out := &OpenCoverageResult{OpenShifts: []OpenShiftItem{}, SwapInbox: []SwapInboxItem{}}
	out.PendingApprovals.Swaps = []PendingApprovalItem{}
	out.PendingApprovals.Claims = []PendingApprovalItem{}

	if err := g.Table("shifts AS s").
		Select("s.id AS shift_id, s.position_id, s.starts_at, s.ends_at, s.break_minutes").
		Joins("JOIN staff_positions sp ON sp.position_id = s.position_id AND sp.staff_id = ? AND sp.business_id = s.business_id", staffID).
		Where("s.business_id = ? AND s.status = ?", businessID, ShiftStatusOpen).
		Order("s.starts_at ASC").Limit(coverageListMax).Scan(&out.OpenShifts).Error; err != nil {
		return nil, err
	}

	if err := g.Table("shift_swap_requests AS sw").
		Select("sw.id AS swap_id, sw.shift_id, sw.requesting_staff_id, s.position_id, s.starts_at, s.ends_at").
		Joins("JOIN shifts s ON s.id = sw.shift_id AND s.business_id = sw.business_id").
		Joins("JOIN staff_positions sp ON sp.position_id = s.position_id AND sp.staff_id = ? AND sp.business_id = sw.business_id", staffID).
		Where("sw.business_id = ? AND sw.kind = ? AND sw.status = ? AND sw.requesting_staff_id <> ?",
			businessID, SwapKindSwap, SwapStatusOpen, staffID).
		Order("s.starts_at ASC").Limit(coverageListMax).Scan(&out.SwapInbox).Error; err != nil {
		return nil, err
	}

	if !isApprover {
		return out, nil
	}
	if err := g.Table("shift_swap_requests AS sw").
		Select("sw.id, sw.shift_id, s.position_id, sw.kind, sw.status, sw.requesting_staff_id, sw.accepting_staff_id, s.starts_at, s.ends_at").
		Joins("JOIN shifts s ON s.id = sw.shift_id AND s.business_id = sw.business_id").
		Where("sw.business_id = ? AND sw.status IN ?", businessID, []string{SwapStatusAccepted, SwapStatusPendingApproval}).
		Order("sw.created_at ASC").Limit(coverageListMax).Scan(&out.PendingApprovals.Swaps).Error; err != nil {
		return nil, err
	}
	if err := g.Table("open_shift_claims AS c").
		Select("c.id, c.shift_id, s.position_id, 'open_claim' AS kind, c.status, c.claiming_staff_id, s.starts_at, s.ends_at").
		Joins("JOIN shifts s ON s.id = c.shift_id AND s.business_id = c.business_id").
		Where("c.business_id = ? AND c.status = ?", businessID, OpenClaimStatusPending).
		Order("c.created_at ASC").Limit(coverageListMax).Scan(&out.PendingApprovals.Claims).Error; err != nil {
		return nil, err
	}
	return out, nil
}

type MyCoverageResult struct {
	Claims []OpenShiftClaim   `json:"claims"`
	Swaps  []ShiftSwapRequest `json:"swaps"`
}

// ListMyCoverage returns the caller's own claims + swaps they requested or
// accepted, tenant-scoped and bounded.
func (d *DB) ListMyCoverage(businessID, staffID uint) (*MyCoverageResult, error) {
	g := d.GetGorm()
	out := &MyCoverageResult{Claims: []OpenShiftClaim{}, Swaps: []ShiftSwapRequest{}}
	if err := g.Where("business_id = ? AND claiming_staff_id = ?", businessID, staffID).
		Order("created_at DESC").Limit(coverageListMax).Find(&out.Claims).Error; err != nil {
		return nil, err
	}
	if err := g.Where("business_id = ? AND (requesting_staff_id = ? OR accepting_staff_id = ?)", businessID, staffID, staffID).
		Order("created_at DESC").Limit(coverageListMax).Find(&out.Swaps).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// terminalSwapStatuses / terminalClaimStatuses are the resolved (non-actionable)
// states that make up the coverage history feed.
var terminalSwapStatuses = []string{SwapStatusApproved, SwapStatusDenied, SwapStatusCancelled}
var terminalClaimStatuses = []string{OpenClaimStatusApproved, OpenClaimStatusDenied, OpenClaimStatusWithdrawn}

// CoverageHistoryItem is one resolved coverage event — an approved/denied/cancelled
// swap or giveup, or an approved/denied/withdrawn open-shift claim — flattened
// across the two backing tables for the operator history view. Staff are carried
// as ids (the operator surface resolves names from its roster, like the other
// coverage lists). Money-free.
type CoverageHistoryItem struct {
	Kind             string    `json:"kind"` // "swap" | "giveup" | "open_claim"
	RequestID        uint      `json:"request_id"`
	ShiftID          uint      `json:"shift_id"`
	PositionID       uint      `json:"position_id"`
	StartsAt         time.Time `json:"starts_at"`
	EndsAt           time.Time `json:"ends_at"`
	Status           string    `json:"status"`
	RequesterStaffID uint      `json:"requester_staff_id"`
	DeciderStaffID   *uint     `json:"decider_staff_id"` // nil for self-cancel/withdraw
	ResolvedAt       time.Time `json:"resolved_at"`
}

// ListCoverageHistory returns a business's resolved coverage events, newest
// resolution first, capped at limit. TWO bounded, narrow-projection joins (one per
// backing table, each riding its (business_id, status) index) merged and sorted in
// memory — no N+1, no SELECT *. The per-table LIMIT is a safe upper bound: the
// global newest-`limit` is a subset of (newest-`limit` of each table), so the
// post-merge truncation yields the correct page.
func (d *DB) ListCoverageHistory(businessID uint, limit int) ([]CoverageHistoryItem, error) {
	if limit <= 0 || limit > coverageListMax {
		limit = coverageListMax
	}
	g := d.GetGorm()

	type swapRow struct {
		ID                uint
		ShiftID           uint
		Kind              string
		Status            string
		RequestingStaffID uint
		ApprovedByStaffID *uint
		ResolvedAt        *time.Time
		PositionID        uint
		StartsAt          time.Time
		EndsAt            time.Time
	}
	var swaps []swapRow
	if err := g.Table("shift_swap_requests AS sw").
		Select("sw.id, sw.shift_id, sw.kind, sw.status, sw.requesting_staff_id, sw.approved_by_staff_id, sw.resolved_at, s.position_id, s.starts_at, s.ends_at").
		Joins("JOIN shifts s ON s.id = sw.shift_id AND s.business_id = sw.business_id").
		Where("sw.business_id = ? AND sw.status IN ?", businessID, terminalSwapStatuses).
		Order("sw.resolved_at DESC").Limit(limit).Scan(&swaps).Error; err != nil {
		return nil, err
	}

	type claimRow struct {
		ID               uint
		ShiftID          uint
		Status           string
		ClaimingStaffID  uint
		DecidedByStaffID *uint
		DecidedAt        *time.Time
		PositionID       uint
		StartsAt         time.Time
		EndsAt           time.Time
	}
	var claims []claimRow
	if err := g.Table("open_shift_claims AS c").
		Select("c.id, c.shift_id, c.status, c.claiming_staff_id, c.decided_by_staff_id, c.decided_at, s.position_id, s.starts_at, s.ends_at").
		Joins("JOIN shifts s ON s.id = c.shift_id AND s.business_id = c.business_id").
		Where("c.business_id = ? AND c.status IN ?", businessID, terminalClaimStatuses).
		Order("c.decided_at DESC").Limit(limit).Scan(&claims).Error; err != nil {
		return nil, err
	}

	out := make([]CoverageHistoryItem, 0, len(swaps)+len(claims))
	for _, r := range swaps {
		resolved := time.Time{}
		if r.ResolvedAt != nil {
			resolved = *r.ResolvedAt
		}
		out = append(out, CoverageHistoryItem{
			Kind: r.Kind, RequestID: r.ID, ShiftID: r.ShiftID, PositionID: r.PositionID,
			StartsAt: r.StartsAt, EndsAt: r.EndsAt, Status: r.Status,
			RequesterStaffID: r.RequestingStaffID, DeciderStaffID: r.ApprovedByStaffID, ResolvedAt: resolved,
		})
	}
	for _, r := range claims {
		resolved := time.Time{}
		if r.DecidedAt != nil {
			resolved = *r.DecidedAt
		}
		out = append(out, CoverageHistoryItem{
			Kind: "open_claim", RequestID: r.ID, ShiftID: r.ShiftID, PositionID: r.PositionID,
			StartsAt: r.StartsAt, EndsAt: r.EndsAt, Status: r.Status,
			RequesterStaffID: r.ClaimingStaffID, DeciderStaffID: r.DecidedByStaffID, ResolvedAt: resolved,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ResolvedAt.After(out[j].ResolvedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ListEligibleStaffIDsForShift returns the active staff ids eligible to cover a
// shift — those with a StaffPosition link to the shift's position. One join over
// shifts → staff_positions → staff; the set form of IsStaffEligibleForPosition.
// Caller excludes the requester.
func (d *DB) ListEligibleStaffIDsForShift(businessID, shiftID uint) ([]uint, error) {
	var ids []uint
	err := d.GetGorm().
		Table("shifts AS sh").
		Select("sp.staff_id").
		Joins("JOIN staff_positions sp ON sp.position_id = sh.position_id AND sp.business_id = sh.business_id").
		Joins("JOIN staff s ON s.id = sp.staff_id AND s.business_id = sp.business_id").
		Where("sh.id = ? AND sh.business_id = ? AND s.is_active = ?", shiftID, businessID, true).
		Scan(&ids).Error
	return ids, err
}
