package spaces

import (
	"errors"
	"fmt"
)

// Sentinel errors for the spaces domain.
var (
	ErrNotFound         = errors.New("spaces: not found")
	ErrRevisionConflict = errors.New("spaces: draft revision conflict")
	ErrValidation       = errors.New("spaces: validation failed")
	ErrHasOpenActivity  = errors.New("spaces: open bills or reservations prevent operation")
	ErrTenantIsolation  = errors.New("spaces: tenant isolation violation")
	ErrInvalidArgument  = errors.New("spaces: invalid argument")
	// ErrScanDraftSkipped means ApplyScan left the draft unchanged intentionally
	// (non-empty draft that is not safe to auto-replace with this scan result).
	// Callers must NOT treat this as draft_apply_failed.
	ErrScanDraftSkipped = errors.New("spaces: scan draft apply skipped")
	// ErrScanDraftOperatorContent means ApplyScan refused because the draft has
	// operator-owned (or linked) content that must not be clobbered.
	ErrScanDraftOperatorContent = errors.New("spaces: draft has operator content")
)

// ConflictError is returned when optimistic concurrency (draft_revision) mismatches.
type ConflictError struct {
	SpaceID          uint
	ExpectedRevision int64
	ActualRevision   int64
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("space %d draft revision conflict: expected %d, actual %d",
		e.SpaceID, e.ExpectedRevision, e.ActualRevision)
}

func (e *ConflictError) Unwrap() error { return ErrRevisionConflict }

// ValidationError wraps layout validation issues.
type ValidationError struct {
	Result ValidationResult
}

func (e *ValidationError) Error() string {
	if len(e.Result.Issues) == 0 {
		return "layout validation failed"
	}
	return fmt.Sprintf("layout validation failed: %s (%s)", e.Result.Issues[0].Code, e.Result.Issues[0].Message)
}

func (e *ValidationError) Unwrap() error { return ErrValidation }

// ActivityBlockError is returned when open bills/reservations block archive/delete.
type ActivityBlockError struct {
	OpenBills        int64
	OpenReservations int64
}

func (e *ActivityBlockError) Error() string {
	return fmt.Sprintf("cannot archive/delete: %d open bill(s), %d active reservation(s)",
		e.OpenBills, e.OpenReservations)
}

func (e *ActivityBlockError) Unwrap() error { return ErrHasOpenActivity }
