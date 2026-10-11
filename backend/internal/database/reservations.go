package database

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ReservationApprovalAuto   = "auto"
	ReservationApprovalManual = "manual"
)

// Machine-stamped cancellation reasons — never shown to guests as a personal
// note from the restaurant (see the decline-email blanklist in
// internal/server/reservation_handlers.go).
const (
	ReservationReasonApprovalTimeout     = "approval_timeout"
	ReservationReasonCancelledByBusiness = "cancelled_by_business"
	ReservationReasonNoShowTimeout       = "no_show_timeout"
)

var activeReservationStatuses = []string{"pending", "confirmed", "seated"}

// MaxReservationDurationMinutes is the longest booking the product accepts.
// Conflict reads rely on it: any booking that can still overlap a slot started
// at most this long (plus the service buffer) before the slot.
const MaxReservationDurationMinutes = 24 * 60

// MaxWaitlistPerSlot is the most waitlist rows one business may hold for a
// single reservation time.
const MaxWaitlistPerSlot = 200

// ErrWaitlistFull is returned when a slot already has MaxWaitlistPerSlot
// waitlist rows.
var ErrWaitlistFull = errors.New("the waitlist for this time is full")

// ReservationConflictLowerBound is the earliest reservation_time that can
// still overlap a slot starting at earliestStart.
func ReservationConflictLowerBound(earliestStart time.Time, serviceBufferMinutes int) time.Time {
	return earliestStart.Add(-time.Duration(MaxReservationDurationMinutes+serviceBufferMinutes) * time.Minute)
}

// ErrReservationSlotUnavailable indicates the slot was claimed between the
// initial (outside-transaction) availability check and the locked write: the
// assigned table, the covers cap, or the unassigned room hold.
var ErrReservationSlotUnavailable = errors.New("reservation slot is no longer available")

var validReservationStatuses = map[string]struct{}{
	"pending":   {},
	"confirmed": {},
	"waitlist":  {},
	"seated":    {},
	"completed": {},
	"cancelled": {},
	"no_show":   {},
}

func defaultReservationSettings(businessID uint) ReservationSettings {
	return ReservationSettings{
		BusinessID:            businessID,
		Enabled:               false,
		MaxAdvanceDays:        30,
		MinAdvanceMinutes:     30,
		MinPartySize:          1,
		MaxPartySize:          20,
		DefaultDuration:       120,
		SlotIntervalMinutes:   30,
		ServiceBufferMinutes:  15,
		MaxCoversPerSlot:      0,
		AutoAssignTables:      true,
		ApprovalMode:          ReservationApprovalAuto,
		AllowWaitlist:         true,
		HoldDurationMinutes:   15,
		AllowCancellation:     true,
		CancellationDeadline:  24,
		NoShowGraceMinutes:    15,
		SendConfirmationEmail: true,
		SendReminderEmail:     true,
		ReminderHoursBefore:   24,
		ExternalPartnerLinks:  JSONRawMessage("[]"),
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}
}

func normalizeReservationSettings(settings *ReservationSettings) bool {
	needsUpdate := false
	defaults := defaultReservationSettings(settings.BusinessID)

	if settings.MaxAdvanceDays < 1 {
		settings.MaxAdvanceDays = defaults.MaxAdvanceDays
		needsUpdate = true
	}
	if settings.MinAdvanceMinutes < 0 {
		settings.MinAdvanceMinutes = defaults.MinAdvanceMinutes
		needsUpdate = true
	}
	if settings.MinPartySize < 1 {
		settings.MinPartySize = defaults.MinPartySize
		needsUpdate = true
	}
	if settings.MaxPartySize < settings.MinPartySize {
		settings.MaxPartySize = defaults.MaxPartySize
		needsUpdate = true
	}
	if settings.DefaultDuration < 15 {
		settings.DefaultDuration = defaults.DefaultDuration
		needsUpdate = true
	}
	if settings.SlotIntervalMinutes < 5 {
		settings.SlotIntervalMinutes = defaults.SlotIntervalMinutes
		needsUpdate = true
	}
	if settings.ServiceBufferMinutes < 0 {
		settings.ServiceBufferMinutes = defaults.ServiceBufferMinutes
		needsUpdate = true
	}
	if settings.HoldDurationMinutes < 0 {
		settings.HoldDurationMinutes = defaults.HoldDurationMinutes
		needsUpdate = true
	}
	if settings.NoShowGraceMinutes < 0 {
		settings.NoShowGraceMinutes = defaults.NoShowGraceMinutes
		needsUpdate = true
	}
	if settings.ReminderHoursBefore < 0 {
		settings.ReminderHoursBefore = defaults.ReminderHoursBefore
		needsUpdate = true
	}
	if len(settings.ExternalPartnerLinks) == 0 || string(settings.ExternalPartnerLinks) == "null" {
		settings.ExternalPartnerLinks = JSONRawMessage("[]")
		needsUpdate = true
	}
	if settings.ApprovalMode != ReservationApprovalAuto && settings.ApprovalMode != ReservationApprovalManual {
		settings.ApprovalMode = defaults.ApprovalMode
		needsUpdate = true
	}
	if !settings.AutoAssignTables && settings.ID == 0 {
		settings.AutoAssignTables = defaults.AutoAssignTables
		needsUpdate = true
	}
	if !settings.AllowWaitlist && settings.ID == 0 {
		settings.AllowWaitlist = defaults.AllowWaitlist
		needsUpdate = true
	}

	return needsUpdate
}

// GetReservationSettings retrieves reservation settings for a business
func GetReservationSettings(businessID uint) (*ReservationSettings, error) {
	var settings ReservationSettings
	err := db.Where("business_id = ?", businessID).First(&settings).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			settings = defaultReservationSettings(businessID)
			if err := db.Create(&settings).Error; err != nil {
				return nil, fmt.Errorf("failed to create default settings: %w", err)
			}
			return &settings, nil
		}
		return nil, fmt.Errorf("failed to get reservation settings: %w", err)
	}

	if normalizeReservationSettings(&settings) {
		if err := db.Omit(clause.Associations).Save(&settings).Error; err != nil {
			return nil, fmt.Errorf("failed to update settings with defaults: %w", err)
		}
	}

	return &settings, nil
}

// GetReservationSettingsForRead retrieves reservation settings as a PURE READ —
// it never writes to the database. Unlike GetReservationSettings (the persist
// variant kept for the settings editor and other callers), this:
//   - returns in-memory defaults on not-found instead of db.Create-ing a row, and
//   - normalizes the loaded row in memory but ignores normalizeReservationSettings'
//     needsUpdate result instead of db.Save-ing it back.
//
// Use this on public GET reservation paths, the hourly reminder sweep, and any
// other hot/background read where a hidden INSERT/UPDATE per call is
// unacceptable. The returned settings carry the same normalized values the
// persist variant would expose; only the side-effecting write is dropped.
func GetReservationSettingsForRead(businessID uint) (*ReservationSettings, error) {
	var settings ReservationSettings
	err := db.Where("business_id = ?", businessID).First(&settings).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			settings = defaultReservationSettings(businessID)
			return &settings, nil
		}
		return nil, fmt.Errorf("failed to get reservation settings: %w", err)
	}

	// Normalize in memory only — deliberately discard the needsUpdate result so
	// no Save is issued on this read path.
	_ = normalizeReservationSettings(&settings)

	return &settings, nil
}

// UpdateReservationSettings updates reservation settings for a business
func UpdateReservationSettings(settings *ReservationSettings) error {
	settings.UpdatedAt = time.Now()
	return db.Omit(clause.Associations).Save(settings).Error
}

// LockReservationBusinessTx serializes reservation writes for one business.
// Covers and unassigned-table holds have no row to lock, so concurrent guest
// bookings would otherwise both pass the outside-transaction capacity check.
// The lock is transaction-scoped and released on commit or rollback. Other
// dialects no-op; SQLite tests keep the single-connection behavior they have.
func LockReservationBusinessTx(tx *gorm.DB, businessID uint) error {
	if tx.Dialector.Name() != "postgres" {
		return nil
	}
	return tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", fmt.Sprintf("res:%d", businessID)).Error
}

// CreateReservationTx creates a reservation inside a transaction with optional
// row locking on the assigned table to prevent overbooking races.
func CreateReservationTx(tx *gorm.DB, reservation *TableReservation, bufferMinutes int, guardOptions ...ReservationWriteGuards) error {
	reservation.CreatedAt = time.Now()
	reservation.UpdatedAt = time.Now()
	guards := ReservationWriteGuards{}
	if len(guardOptions) > 0 {
		guards = guardOptions[0]
	}
	if reservation.TableID != nil && *reservation.TableID > 0 {
		// Lock the table row to serialize concurrent reservations for the same table.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&Table{}, *reservation.TableID).Error; err != nil {
			return fmt.Errorf("failed to lock table: %w", err)
		}
		// Re-validate availability UNDER the lock. The initial availability check
		// runs outside this transaction, so without this re-check two concurrent
		// bookings for the same table+slot both pass it and then double-book once
		// the table-row lock serializes their inserts.
		available, err := checkTableAvailabilityWithBufferTx(tx, *reservation.TableID, reservation.ReservationTime, reservation.Duration, bufferMinutes, 0)
		if err != nil {
			return err
		}
		if !available {
			return ErrReservationSlotUnavailable
		}
		if err := guardReservationTableOccupancyTx(tx, *reservation.TableID, guards); err != nil {
			return err
		}
	}
	return tx.Create(reservation).Error
}

// GetNextWaitlistPositionTx computes the next waitlist position atomically
// inside the given transaction, preventing duplicate positions under concurrency.
// A slot that already holds MaxWaitlistPerSlot waitlist rows returns ErrWaitlistFull.
func GetNextWaitlistPositionTx(tx *gorm.DB, businessID uint, reservationTime time.Time) (int, error) {
	var count int64
	if err := tx.Model(&TableReservation{}).
		Where("business_id = ? AND status = ? AND reservation_time = ?", businessID, "waitlist", reservationTime).
		Count(&count).Error; err != nil {
		return 0, err
	}
	if count >= MaxWaitlistPerSlot {
		return 0, ErrWaitlistFull
	}
	var maxPosition int
	if err := tx.Model(&TableReservation{}).
		Where("business_id = ? AND status = ? AND reservation_time = ?", businessID, "waitlist", reservationTime).
		Select("COALESCE(MAX(waitlist_position), 0)").
		Scan(&maxPosition).Error; err != nil {
		return 0, err
	}
	return maxPosition + 1, nil
}

// UpdateReservationTx updates a reservation inside a transaction with row
// locking. It MUST be called within a transaction (e.g. database.GetDB().
// Transaction(...)) for the locks to hold across the read/validate/save.
//
// When a table is assigned, it locks the table row and re-validates the slot
// under that lock (excluding this reservation itself) so that reassigning a
// reservation's table/time cannot double-book a table that a concurrent
// booking just claimed — the same protection CreateReservationTx applies.
func UpdateReservationTx(tx *gorm.DB, reservation *TableReservation, bufferMinutes int, guardOptions ...ReservationWriteGuards) error {
	reservation.UpdatedAt = time.Now()
	guards := ReservationWriteGuards{}
	if len(guardOptions) > 0 {
		guards = guardOptions[0]
	}
	if reservation.TableID != nil && *reservation.TableID > 0 {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&Table{}, *reservation.TableID).Error; err != nil {
			return fmt.Errorf("failed to lock table: %w", err)
		}
		available, err := checkTableAvailabilityWithBufferTx(tx, *reservation.TableID, reservation.ReservationTime, reservation.Duration, bufferMinutes, reservation.ID)
		if err != nil {
			return err
		}
		if !available {
			return ErrReservationSlotUnavailable
		}
		if err := guardReservationTableOccupancyTx(tx, *reservation.TableID, guards); err != nil {
			return err
		}
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&TableReservation{}, reservation.ID).Error; err != nil {
		return fmt.Errorf("failed to lock reservation: %w", err)
	}
	// Omit associations: the aggregate carries the partial-projection Business
	// preload (no owner_address/user_id), and letting Save upsert it emits an
	// owner-less businesses row that businesses_has_canonical_owner rejects.
	return tx.Omit(clause.Associations).Save(reservation).Error
}

// reservationBusinessColumns is the projection for the Business embedded on a
// reservation read. Consumers of the reservation readers only touch a handful
// of Business fields — hydrating the full ~94-column row (incl. Stripe IDs and
// the onboarding blob) is pure over-fetch and a needless exposure surface.
//   - id           — primary key / FK integrity
//   - name         — cancel-by-code handler response (business_name)
//   - custom_url   — cancel-by-code handler response (business_custom_url)
//   - timezone     — reservationNotificationPayload (Telegram status changes)
//   - default_currency / display_currency — money-rendering floor for any
//     downstream consumer that formats amounts off the reservation's business
var reservationBusinessColumns = []string{
	"id", "name", "custom_url", "default_currency", "display_currency", "timezone",
}

func preloadReservationBusinessSummary(tx *gorm.DB) *gorm.DB {
	return tx.Preload("Business", func(preload *gorm.DB) *gorm.DB {
		return preload.Select(reservationBusinessColumns)
	})
}

// createReservationBaseQueryOn is the operator-detail read shape bound to the
// given handle: the projected Business plus the table and the full status
// timeline (rendered by the operator reservation detail view).
func createReservationBaseQueryOn(handle *gorm.DB) *gorm.DB {
	return preloadReservationBusinessSummary(handle).
		Preload("Table").
		Preload("StatusHistory", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("created_at ASC")
		}).
		Preload("StatusHistory.Table")
}

// createGuestReservationQuery is the public confirmation-code read shape. The
// guest confirmation page and cancel-by-code path never render the status
// timeline (publicReservationPayload strips status_history; the public details
// DTO and CancelReservationByCode read only the projected Business + table), so
// StatusHistory is dropped here to avoid two extra preload queries.
func createGuestReservationQuery() *gorm.DB {
	return preloadReservationBusinessSummary(db).Preload("Table")
}

func preloadReservationTableSummary(tx *gorm.DB) *gorm.DB {
	return tx.Preload("Table", func(preload *gorm.DB) *gorm.DB {
		return preload.Select("id", "name", "table_code", "capacity")
	})
}

// ErrReservationNotFound lets handlers map missing reservations to 404 with
// errors.Is instead of comparing error strings.
var ErrReservationNotFound = errors.New("reservation not found")

// GetReservationByBusinessAndID retrieves a reservation scoped to a business.
func GetReservationByBusinessAndID(businessID, reservationID uint) (*TableReservation, error) {
	return getReservationByBusinessAndIDOn(db, businessID, reservationID)
}

// GetReservationByBusinessAndIDTx is GetReservationByBusinessAndID inside the
// caller's transaction, so a create flow can reload the aggregate before
// committing and roll the insert back if the reload fails.
func GetReservationByBusinessAndIDTx(tx *gorm.DB, businessID, reservationID uint) (*TableReservation, error) {
	return getReservationByBusinessAndIDOn(tx, businessID, reservationID)
}

func getReservationByBusinessAndIDOn(handle *gorm.DB, businessID, reservationID uint) (*TableReservation, error) {
	var reservation TableReservation
	if err := createReservationBaseQueryOn(handle).Where("business_id = ? AND id = ?", businessID, reservationID).First(&reservation).Error; err != nil {
		// Only a missing row is "not found"; a driver/connection failure must
		// surface as a server fault rather than a misleading 404.
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrReservationNotFound
		}
		return nil, fmt.Errorf("load reservation %d for business %d: %w", reservationID, businessID, err)
	}
	return &reservation, nil
}

// NormalizeConfirmationCode trims surrounding whitespace and uppercases a
// public confirmation code. Generated codes are 12-char uppercase hex; guests
// may type lowercase or paste with spaces.
func NormalizeConfirmationCode(confirmationCode string) string {
	return strings.ToUpper(strings.TrimSpace(confirmationCode))
}

// ReservationConfirmationCodeExists reports whether any reservation already
// uses the given confirmation code (codes are globally unique). Comparison is
// case-insensitive so a lowercase generator output still collides with the
// stored uppercase row.
func ReservationConfirmationCodeExists(confirmationCode string) (bool, error) {
	code := NormalizeConfirmationCode(confirmationCode)
	if code == "" {
		return false, nil
	}
	var count int64
	if err := db.Model(&TableReservation{}).
		Where("confirmation_code = ?", code).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetReservationByConfirmationCode retrieves a reservation by its public confirmation code.
func GetReservationByConfirmationCode(confirmationCode string) (*TableReservation, error) {
	code := NormalizeConfirmationCode(confirmationCode)
	if code == "" {
		return nil, ErrReservationNotFound
	}
	var reservation TableReservation
	if err := createGuestReservationQuery().
		Where("confirmation_code = ?", code).
		First(&reservation).Error; err != nil {
		return nil, ErrReservationNotFound
	}
	return &reservation, nil
}

// ErrReservationStatusConflict is returned by UpdateReservationStatusGuardedTx when
// the reservation's status changed out from under the caller between the read that
// planned the transition and the write — e.g. the auto-decline sweeper flipped a
// pending reservation to cancelled while an operator was approving it. The caller
// must abort (not clobber) and re-read.
var ErrReservationStatusConflict = errors.New("reservation status changed concurrently")

// UpdateReservationStatusGuardedTx protects a transition with a consistent
// lock order: table, slot/occupancy checks, then reservation status. This keeps
// table assignment transitions from bypassing the same race checks used by
// create/update writes.
func UpdateReservationStatusGuardedTx(
	tx *gorm.DB,
	reservation *TableReservation,
	expectedStatus string,
	bufferMinutes int,
	guards ReservationWriteGuards,
) error {
	reservation.UpdatedAt = time.Now()
	if reservation.TableID != nil && *reservation.TableID > 0 {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&Table{}, *reservation.TableID).Error; err != nil {
			return fmt.Errorf("failed to lock table: %w", err)
		}
		available, err := checkTableAvailabilityWithBufferTx(
			tx,
			*reservation.TableID,
			reservation.ReservationTime,
			reservation.Duration,
			bufferMinutes,
			reservation.ID,
		)
		if err != nil {
			return err
		}
		if !available {
			return ErrReservationSlotUnavailable
		}
		if err := guardReservationTableOccupancyTx(tx, *reservation.TableID, guards); err != nil {
			return err
		}
	}

	var current TableReservation
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&current, reservation.ID).Error; err != nil {
		return err
	}
	if current.Status != expectedStatus {
		return ErrReservationStatusConflict
	}
	// Omit associations for the same reason as UpdateReservationTx.
	return tx.Omit(clause.Associations).Save(reservation).Error
}

func ParseReservationStatusFilter(status string) ([]string, error) {
	status = strings.TrimSpace(status)
	if status == "" || status == "all" {
		return nil, nil
	}
	statuses := strings.Split(status, ",")
	for i, s := range statuses {
		normalized := strings.TrimSpace(s)
		if normalized == "" {
			return nil, fmt.Errorf("invalid reservation status: %s", s)
		}
		if _, ok := validReservationStatuses[normalized]; !ok {
			return nil, fmt.Errorf("invalid reservation status: %s", normalized)
		}
		statuses[i] = normalized
	}
	return statuses, nil
}

// reservationsByBusinessBaseQueryWithSearch builds the list filter query.
// When search is empty/whitespace the SQL shape matches the legacy (pre-search)
// path exactly so callers that omit q keep identical behavior.
func reservationsByBusinessBaseQueryWithSearch(businessID uint, startDate, endDate time.Time, status, search string) *gorm.DB {
	query := db.Model(&TableReservation{}).Where("business_id = ? AND reservation_time BETWEEN ? AND ?", businessID, startDate, endDate)
	if statuses, err := ParseReservationStatusFilter(status); err == nil && len(statuses) > 0 {
		query = query.Where("status IN ?", statuses)
	}
	search = strings.ToLower(strings.TrimSpace(search))
	if search != "" {
		like := "%" + escapeSQLLike(search) + "%"
		query = query.Where(
			"(LOWER(COALESCE(customer_name,'')) LIKE ? ESCAPE '\\' OR LOWER(COALESCE(customer_phone,'')) LIKE ? ESCAPE '\\' OR LOWER(COALESCE(customer_email,'')) LIKE ? ESCAPE '\\')",
			like, like, like,
		)
	}
	return query
}

// GetReservationsByBusinessIDPaginatedWithSearch is the filtered list path.
// search is optional; empty/whitespace applies no customer-field LIKE clause.
func GetReservationsByBusinessIDPaginatedWithSearch(businessID uint, startDate, endDate time.Time, status, search string, p PaginationParams) (*PaginatedResult[TableReservation], error) {
	p = p.Normalize()
	base := reservationsByBusinessBaseQueryWithSearch(businessID, startDate, endDate, status, search)

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count reservations: %w", err)
	}

	var reservations []TableReservation
	if err := base.Order("reservation_time ASC").
		Offset(p.Offset()).
		Limit(p.PageSize).
		Scopes(preloadReservationTableSummary).
		Find(&reservations).Error; err != nil {
		return nil, fmt.Errorf("failed to get reservations: %w", err)
	}

	totalPages := int(total) / p.PageSize
	if int(total)%p.PageSize != 0 {
		totalPages++
	}
	return &PaginatedResult[TableReservation]{
		Data:       reservations,
		Total:      total,
		Page:       p.Page,
		PageSize:   p.PageSize,
		TotalPages: totalPages,
	}, nil
}

const (
	defaultUpcomingReservationLimit = 50
	maxUpcomingReservationLimit     = 500
)

// clampUpcomingReservationLimit bounds a client-supplied limit so a hand-edited
// ?limit can neither request an unbounded scan (limit<=0 used to skip the LIMIT
// clause entirely) nor an absurd cap. Same default-and-cap convention as the
// inventory movement list.
func clampUpcomingReservationLimit(limit int) int {
	if limit <= 0 {
		return defaultUpcomingReservationLimit
	}
	if limit > maxUpcomingReservationLimit {
		return maxUpcomingReservationLimit
	}
	return limit
}

// reservationHoldCutoff is now minus the business no-show grace. Upcoming
// and late-within-grace bookings stay on the host clock; past-grace
// Confirmed rows do not (they age to no_show).
//
// The cutoff is always UTC. SQLite DATETIME has no zone, so a local now
// compared against reservation_time written as UTC drops still-valid
// bookings and paints the table Available.
func reservationHoldCutoff(businessID uint, now time.Time) time.Time {
	grace := 15
	if settings, err := GetReservationSettingsForRead(businessID); err == nil && settings != nil {
		if settings.NoShowGraceMinutes >= 0 {
			grace = settings.NoShowGraceMinutes
		}
	}
	return now.UTC().Add(-time.Duration(grace) * time.Minute)
}

// GetUpcomingReservations retrieves upcoming reservations for a business
func GetUpcomingReservations(businessID uint, limit int) ([]TableReservation, error) {
	var reservations []TableReservation
	now := time.Now()
	cutoff := reservationHoldCutoff(businessID, now)

	query := db.Where("business_id = ? AND reservation_time >= ? AND status IN ?",
		businessID, cutoff, []string{"pending", "confirmed", "waitlist"}).
		Order("reservation_time ASC").
		Scopes(preloadReservationTableSummary).
		Limit(clampUpcomingReservationLimit(limit))

	if err := query.Find(&reservations).Error; err != nil {
		return nil, err
	}
	return reservations, nil
}

func reservationOverlaps(reservation TableReservation, startTime, endTime time.Time, bufferMinutes int) bool {
	reservationStart := reservation.ReservationTime
	reservationEnd := reservation.ReservationTime.Add(time.Duration(reservation.Duration+bufferMinutes) * time.Minute)
	return reservationStart.Before(endTime) && reservationEnd.After(startTime)
}

// checkTableAvailabilityWithBufferTx is the transaction-aware core of the table
// availability check. Run inside the reservation-create transaction after the
// table row is locked, it re-validates the slot under the lock to close the
// check-then-insert double-booking race.
func checkTableAvailabilityWithBufferTx(tx *gorm.DB, tableID uint, reservationTime time.Time, duration, bufferMinutes int, excludeReservationID uint) (bool, error) {
	startTime := reservationTime
	endTime := reservationTime.Add(time.Duration(duration+bufferMinutes) * time.Minute)

	var reservations []TableReservation
	// Bounded scan: only bookings that can still overlap (started at most the
	// longest allowed duration plus buffer before the slot), scheduling
	// columns only. Runs under the table row lock, so it must stay cheap.
	query := tx.Select("id", "table_id", "reservation_time", "duration", "status").
		Where("table_id = ? AND status IN ? AND reservation_time < ? AND reservation_time >= ?",
			tableID, activeReservationStatuses, endTime, ReservationConflictLowerBound(startTime, bufferMinutes))
	if excludeReservationID > 0 {
		query = query.Where("id != ?", excludeReservationID)
	}
	if err := query.Find(&reservations).Error; err != nil {
		return false, err
	}

	for _, reservation := range reservations {
		if reservationOverlaps(reservation, startTime, endTime, bufferMinutes) {
			return false, nil
		}
	}

	return true, nil
}

// GetReservationStats retrieves statistics for reservations.
// covers/needs_table ride the same single-scan aggregate (additive map keys —
// BE-first safe for clients that only read the status counts):
//   - covers: SUM(party_size) over non-cancelled/non-no-show rows — the KPI
//     label says "Covers" (guests), so a booking count would be dishonest.
//   - needs_table: active bookings (pending/confirmed/waitlist) with no table.
func GetReservationStats(businessID uint, startDate, endDate time.Time) (map[string]interface{}, error) {
	var counts struct {
		Total      int64
		Pending    int64
		Confirmed  int64
		Waitlist   int64
		Seated     int64
		Completed  int64
		Cancelled  int64
		NoShow     int64 `gorm:"column:no_show"`
		Covers     int64
		NeedsTable int64 `gorm:"column:needs_table"`
	}

	if err := db.Model(&TableReservation{}).
		Select(`
			COUNT(*) AS total,
			SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END) AS pending,
			SUM(CASE WHEN status = 'confirmed' THEN 1 ELSE 0 END) AS confirmed,
			SUM(CASE WHEN status = 'waitlist' THEN 1 ELSE 0 END) AS waitlist,
			SUM(CASE WHEN status = 'seated' THEN 1 ELSE 0 END) AS seated,
			SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS completed,
			SUM(CASE WHEN status = 'cancelled' THEN 1 ELSE 0 END) AS cancelled,
			SUM(CASE WHEN status = 'no_show' THEN 1 ELSE 0 END) AS no_show,
			COALESCE(SUM(CASE WHEN status NOT IN ('cancelled','no_show') THEN party_size ELSE 0 END), 0) AS covers,
			COALESCE(SUM(CASE WHEN (table_id IS NULL OR table_id = 0) AND status IN ('pending','confirmed','waitlist') THEN 1 ELSE 0 END), 0) AS needs_table
		`).
		Where("business_id = ? AND reservation_time BETWEEN ? AND ?", businessID, startDate, endDate).
		Scan(&counts).Error; err != nil {
		return nil, fmt.Errorf("failed to count reservation stats: %w", err)
	}

	return map[string]interface{}{
		"total": counts.Total, "pending": counts.Pending, "confirmed": counts.Confirmed, "waitlist": counts.Waitlist,
		"seated": counts.Seated, "completed": counts.Completed, "cancelled": counts.Cancelled, "no_show": counts.NoShow,
		"covers": counts.Covers, "needs_table": counts.NeedsTable,
	}, nil
}

func CreateReservationStatusHistory(history *ReservationStatusHistory) error {
	return CreateReservationStatusHistoryTx(db, history)
}

// CreateReservationStatusHistoryTx writes the status-history row on the given
// handle so create flows can record it inside the same transaction as the
// reservation insert and roll everything back together on failure.
func CreateReservationStatusHistoryTx(tx *gorm.DB, history *ReservationStatusHistory) error {
	if history.CreatedAt.IsZero() {
		history.CreatedAt = time.Now()
	}
	return tx.Create(history).Error
}
