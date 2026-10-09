package accounting

import (
	"errors"
	"fmt"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"gorm.io/gorm"
)

// ErrPeriodLocked is returned when a mutation targets a closed accounting period.
// Handlers map it to HTTP 409 with code period_locked.
var ErrPeriodLocked = errors.New("accounting period is locked")

// PeriodLockedError carries the locked_through date for client payloads.
type PeriodLockedError struct {
	LockedThrough *time.Time
}

func (e *PeriodLockedError) Error() string {
	if e.LockedThrough == nil {
		return ErrPeriodLocked.Error()
	}
	return fmt.Sprintf("%s through %s", ErrPeriodLocked.Error(), e.LockedThrough.Format("2006-01-02"))
}

func (e *PeriodLockedError) Unwrap() error { return ErrPeriodLocked }

// GetLockedThrough returns the authoritative locked_through for a business
// (latest append-only row). Nil means books are fully open.
//
// NOTE: Fiscal receipt issuance and credit notes are intentionally NOT gated
// by this helper — AFIP is its own legal ledger; blocking it would break
// compliance operations.
func GetLockedThrough(db *gorm.DB, businessID uint) (*time.Time, error) {
	var row database.AccountingPeriodLock
	err := db.Where("business_id = ?", businessID).
		Order("created_at DESC, id DESC").
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return row.LockedThrough, nil
}

// IsDateLocked reports whether day (date-only) is on or before locked_through.
func IsDateLocked(lockedThrough *time.Time, day time.Time) bool {
	if lockedThrough == nil {
		return false
	}
	lt := time.Date(lockedThrough.Year(), lockedThrough.Month(), lockedThrough.Day(), 0, 0, 0, 0, time.UTC)
	d := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	return !d.After(lt)
}

// CheckPeriodUnlocked returns *PeriodLockedError if day is locked.
//
// When a lock exists, day is interpreted in the business IANA timezone
// (businesses.timezone). An empty or invalid zone falls back to UTC, so a
// late local evening is not treated as the next UTC calendar date.
// locked_through is a date: its year, month, and day are compared as stored.
// A nil lock means the books are open. Database errors are returned as-is.
func CheckPeriodUnlocked(db *gorm.DB, businessID uint, day time.Time) error {
	lt, err := GetLockedThrough(db, businessID)
	if err != nil {
		return err
	}
	if lt == nil {
		return nil
	}
	loc, err := businessLocation(db, businessID)
	if err != nil {
		return err
	}
	if IsDateLocked(lt, day.In(loc)) {
		return &PeriodLockedError{LockedThrough: lt}
	}
	return nil
}

// CheckDateUnlocked returns *PeriodLockedError if the calendar date day is
// locked. day is date-only: its year, month, and day are compared as stored,
// with no timezone shift. Use it for values that are already a business-local
// date at UTC midnight (a payroll period_end, a recurring template run day).
// Shifting those into a zone west of UTC would read the previous day and lock
// the first open day. Instants (occurred_at, paid_at) use CheckPeriodUnlocked.
func CheckDateUnlocked(db *gorm.DB, businessID uint, day time.Time) error {
	lt, err := GetLockedThrough(db, businessID)
	if err != nil {
		return err
	}
	if IsDateLocked(lt, day) {
		return &PeriodLockedError{LockedThrough: lt}
	}
	return nil
}

// businessLocation loads businesses.timezone for the period-lock comparison.
// Lookup failures are returned; an empty or unknown name falls back to UTC.
func businessLocation(db *gorm.DB, businessID uint) (*time.Location, error) {
	var tz string
	if err := db.Model(&database.Business{}).Where("id = ?", businessID).Pluck("timezone", &tz).Error; err != nil {
		return nil, err
	}
	if tz == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC, nil
	}
	return loc, nil
}

// LockBooksTx serializes, for one business, appending a period lock against
// the writes it guards. A guarded write that takes this lock and then checks
// the period inside the same transaction cannot commit into a period that a
// concurrent close has just locked. Postgres only: a transaction-scoped
// advisory lock. On other dialects (SQLite tests) it is a no-op.
func LockBooksTx(tx *gorm.DB, businessID uint) error {
	if tx.Dialector == nil || tx.Dialector.Name() != "postgres" {
		return nil
	}
	return tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", fmt.Sprintf("books:%d", businessID)).Error
}

// checkPayrollRunPeriodTx gates a payroll mutation on the run's period_end
// (a calendar date), its stored paid_at (an instant: voiding removes the
// expense from that period) and any extra instants the mutation writes.
func checkPayrollRunPeriodTx(tx *gorm.DB, businessID uint, run *database.PayrollRun, extra ...time.Time) error {
	if err := CheckDateUnlocked(tx, businessID, run.PeriodEnd); err != nil {
		return err
	}
	days := append([]time.Time{}, extra...)
	if run.PaidAt != nil {
		days = append(days, *run.PaidAt)
	}
	for _, day := range days {
		if err := CheckPeriodUnlocked(tx, businessID, day); err != nil {
			return err
		}
	}
	return nil
}
