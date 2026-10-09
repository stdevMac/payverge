package marketing

import "strings"

// Activity handoff statuses (S2-E). No due dates, no schedule, no queue clock.
//
// Wire values live in marketing_activities.status (string column — no migration).
//
//	draft is implicit: no row yet (or creative still open in the editor).
//	ready      — creator finished craft; waiting owner/manager review
//	approved   — owner OK to post externally
//	posted     — mark_posted (starts 30-day suggestion cooldown)
//	dismissed  — hidden from feed
const (
	ActivityStatusReady     = "ready"
	ActivityStatusApproved  = "approved"
	ActivityStatusPosted    = "posted"
	ActivityStatusDismissed = "dismissed"
)

// Activity actions for RecordMarketingActivityHandler.
const (
	ActivityActionMarkReady = "mark_ready"
	ActivityActionApprove   = "approve"
	ActivityActionPost      = "post"
	ActivityActionDismiss   = "dismiss"
	ActivityActionRestore   = "restore"
)

// HandoffTransitionError is a stable code for illegal status moves.
type HandoffTransitionError struct {
	Code    string
	Message string
}

func (e *HandoffTransitionError) Error() string { return e.Message }

// CanMarkReady reports whether marketing:write may set status ready.
// Allowed from: no row (""), ready (re-save), approved (rework).
// Not from: posted (use restore path first), dismissed (restore first).
func CanMarkReady(current string) bool {
	switch stringsTrim(current) {
	case "", ActivityStatusReady, ActivityStatusApproved:
		return true
	default:
		return false
	}
}

// CanApprove reports whether an owner may set status approved.
// Allowed from: ready only (must have been handed up).
func CanApprove(current string) bool {
	return stringsTrim(current) == ActivityStatusReady
}

// CanMarkPosted reports whether mark_posted is allowed for this actor.
//
// Policy (S2-E):
//   - Owners may post from ready, approved, or a fresh craft ("").
//   - Staff with marketing:write may post only from approved (owner already OK'd).
//   - Never from dismissed without restore.
func CanMarkPosted(current string, isOwner bool) bool {
	cur := stringsTrim(current)
	if cur == ActivityStatusDismissed {
		return false
	}
	if isOwner {
		switch cur {
		case "", ActivityStatusReady, ActivityStatusApproved, ActivityStatusPosted:
			return true
		default:
			return false
		}
	}
	// Staff: only after owner approval.
	return cur == ActivityStatusApproved
}

// CanDismiss is allowed for any non-empty lifecycle except we allow always
// (dismiss is a soft hide). Empty current still creates a dismissed row.
func CanDismiss(current string) bool {
	_ = current
	return true
}

// ValidateHandoffTransition returns nil if action may proceed given current
// status and actor. current is "" when no activity row exists yet.
func ValidateHandoffTransition(action, current string, isOwner bool) error {
	switch action {
	case ActivityActionMarkReady:
		if !CanMarkReady(current) {
			return &HandoffTransitionError{
				Code:    "invalid_handoff_transition",
				Message: "cannot mark ready from status " + stringsTrim(current),
			}
		}
		return nil
	case ActivityActionApprove:
		if !isOwner {
			return &HandoffTransitionError{
				Code:    "owner_required",
				Message: "only the business owner can approve creatives",
			}
		}
		if !CanApprove(current) {
			return &HandoffTransitionError{
				Code:    "invalid_handoff_transition",
				Message: "can only approve a creative marked ready",
			}
		}
		return nil
	case ActivityActionPost:
		if !CanMarkPosted(current, isOwner) {
			if !isOwner && stringsTrim(current) != ActivityStatusApproved {
				return &HandoffTransitionError{
					Code:    "approval_required",
					Message: "staff may mark posted only after owner approval",
				}
			}
			return &HandoffTransitionError{
				Code:    "invalid_handoff_transition",
				Message: "cannot mark posted from status " + stringsTrim(current),
			}
		}
		return nil
	case ActivityActionDismiss:
		if !CanDismiss(current) {
			return &HandoffTransitionError{
				Code:    "invalid_handoff_transition",
				Message: "cannot dismiss from status " + stringsTrim(current),
			}
		}
		return nil
	case ActivityActionRestore:
		if stringsTrim(current) != ActivityStatusDismissed && current != "" {
			// Restore is only meaningful for dismissed rows; handler still
			// no-ops safely when row missing.
			return nil
		}
		return nil
	default:
		return &HandoffTransitionError{
			Code:    "invalid_action",
			Message: "invalid action",
		}
	}
}

// StatusForAction maps a valid action onto the status column value.
func StatusForAction(action string) string {
	switch action {
	case ActivityActionMarkReady:
		return ActivityStatusReady
	case ActivityActionApprove:
		return ActivityStatusApproved
	case ActivityActionPost:
		return ActivityStatusPosted
	case ActivityActionDismiss:
		return ActivityStatusDismissed
	default:
		return ""
	}
}

func stringsTrim(s string) string { return strings.TrimSpace(s) }
