package services

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
)

type ReservationService struct {
	db  *gorm.DB
	now func() time.Time
}

func NewReservationService(db *gorm.DB) *ReservationService {
	return &ReservationService{db: db, now: time.Now}
}

const (
	// ReservationNearTermWindow is the period where live floor occupancy must
	// influence a table recommendation.
	ReservationNearTermWindow = 2 * time.Hour
	// StaleOccupiedTableAfter flags bills that likely need an operator to close
	// or reconcile them. A stale bill still occupies its table.
	StaleOccupiedTableAfter = 12 * time.Hour
)

// ErrReservationsDisabled is returned by public availability when the venue
// has reservations turned off.
var ErrReservationsDisabled = errors.New("reservations are not enabled")

// ErrGuestDurationNotAllowed is returned when a public booking payload includes
// duration. Online bookings always use settings.default_duration.
var ErrGuestDurationNotAllowed = errors.New("duration cannot be set for online bookings")

// ReservationSettingsValidationError is a client-safe settings validation
// failure. Error() is the validation sentence the API returns as-is.
type ReservationSettingsValidationError struct {
	msg string
}

func (e *ReservationSettingsValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.msg
}

// reservationConflictSelectColumns is the scheduling projection used by
// availability conflict reads. Guest PII (name/email/phone/requests/notes)
// is intentionally omitted — slot maths only needs assignment, time, and size.
var reservationConflictSelectColumns = []string{
	"id",
	"business_id",
	"table_id",
	"party_size",
	"reservation_time",
	"duration",
	"status",
}

type ReservationSettingsDTO struct {
	ID                    uint            `json:"id"`
	BusinessID            uint            `json:"business_id"`
	Enabled               bool            `json:"enabled"`
	MaxAdvanceDays        int             `json:"max_advance_days"`
	MinAdvanceMinutes     int             `json:"min_advance_minutes"`
	MinPartySize          int             `json:"min_party_size"`
	MaxPartySize          int             `json:"max_party_size"`
	DefaultDuration       int             `json:"default_duration"`
	SlotIntervalMinutes   int             `json:"slot_interval_minutes"`
	ServiceBufferMinutes  int             `json:"service_buffer_minutes"`
	MaxCoversPerSlot      int             `json:"max_covers_per_slot"`
	AutoAssignTables      bool            `json:"auto_assign_tables"`
	ApprovalMode          string          `json:"approval_mode"`
	AllowWaitlist         bool            `json:"allow_waitlist"`
	HoldDurationMinutes   int             `json:"hold_duration_minutes"`
	AllowCancellation     bool            `json:"allow_cancellation"`
	CancellationDeadline  int             `json:"cancellation_deadline"`
	NoShowGraceMinutes    int             `json:"no_show_grace_minutes"`
	SendConfirmationEmail bool            `json:"send_confirmation_email"`
	SendReminderEmail     bool            `json:"send_reminder_email"`
	ReminderHoursBefore   int             `json:"reminder_hours_before"`
	ExternalPartnerLinks  json.RawMessage `json:"external_partner_links"`
	NextAvailableSlot     *string         `json:"next_available_slot,omitempty"`
	AvailableSlotCount    int             `json:"available_slot_count,omitempty"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
}

type UpdateReservationSettingsInput struct {
	Enabled               *bool            `json:"enabled"`
	MaxAdvanceDays        *int             `json:"max_advance_days"`
	MinAdvanceMinutes     *int             `json:"min_advance_minutes"`
	MinPartySize          *int             `json:"min_party_size"`
	MaxPartySize          *int             `json:"max_party_size"`
	DefaultDuration       *int             `json:"default_duration"`
	SlotIntervalMinutes   *int             `json:"slot_interval_minutes"`
	ServiceBufferMinutes  *int             `json:"service_buffer_minutes"`
	MaxCoversPerSlot      *int             `json:"max_covers_per_slot"`
	AutoAssignTables      *bool            `json:"auto_assign_tables"`
	ApprovalMode          *string          `json:"approval_mode"`
	AllowWaitlist         *bool            `json:"allow_waitlist"`
	HoldDurationMinutes   *int             `json:"hold_duration_minutes"`
	AllowCancellation     *bool            `json:"allow_cancellation"`
	CancellationDeadline  *int             `json:"cancellation_deadline"`
	NoShowGraceMinutes    *int             `json:"no_show_grace_minutes"`
	SendConfirmationEmail *bool            `json:"send_confirmation_email"`
	SendReminderEmail     *bool            `json:"send_reminder_email"`
	ReminderHoursBefore   *int             `json:"reminder_hours_before"`
	ExternalPartnerLinks  *json.RawMessage `json:"external_partner_links"`
}

type ReservationAvailabilitySlotDTO struct {
	Time            string `json:"time"`
	AvailableTables int    `json:"available_tables"`
	Recommended     bool   `json:"recommended"`
	ReasonCode      string `json:"reason_code"`
}

type ReservationAvailabilityDTO struct {
	Date              string                           `json:"date"`
	PartySize         int                              `json:"party_size"`
	AvailableSlots    []ReservationAvailabilitySlotDTO `json:"available_slots"`
	TotalSlots        int                              `json:"total_slots"`
	NextAvailableSlot *string                          `json:"next_available_slot,omitempty"`
	WaitlistAvailable bool                             `json:"waitlist_available"`
}

type ReservationTableOptionDTO struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Capacity int    `json:"capacity"`
	// Optional space linkage for soft grouping in the operator table picker.
	// Omitted when the table is not assigned to a space (legacy / unplaced).
	SpaceID                   *uint                        `json:"space_id,omitempty"`
	SpaceName                 string                       `json:"space_name,omitempty"`
	OccupancyState            database.TableOccupancyState `json:"occupancy_state"`
	ActiveBillID              *uint                        `json:"active_bill_id,omitempty"`
	ActiveBillOpenedAt        *time.Time                   `json:"active_bill_opened_at,omitempty"`
	ActiveBillAgeMinutes      int64                        `json:"active_bill_age_minutes,omitempty"`
	ReservationAvailable      bool                         `json:"reservation_available"`
	Recommended               bool                         `json:"recommended"`
	RequiresOccupancyOverride bool                         `json:"requires_occupancy_override"`
	ConflictReason            string                       `json:"conflict_reason,omitempty"`
}

type CreateReservationInput struct {
	TableID         *uint  `json:"table_id"`
	CustomerName    string `json:"customer_name" binding:"required"`
	CustomerPhone   string `json:"customer_phone"`
	CustomerEmail   string `json:"customer_email"`
	PartySize       int    `json:"party_size" binding:"required"`
	ReservationTime string `json:"reservation_time" binding:"required"`
	Duration        *int   `json:"duration"`
	SpecialRequests string `json:"special_requests"`
	Notes           string `json:"notes"`
	Source          string `json:"source"`
	// Language is the guest locale at booking time (e.g. "fr", "es-AR").
	// Empty falls back to "en" via NormalizeGuestLang.
	Language                   string `json:"language"`
	AllowOccupiedTableOverride bool   `json:"allow_occupied_table_override"`
}

type UpdateReservationInput struct {
	TableID                    *uint   `json:"table_id"`
	ClearTable                 bool    `json:"clear_table"`
	CustomerName               *string `json:"customer_name"`
	CustomerPhone              *string `json:"customer_phone"`
	CustomerEmail              *string `json:"customer_email"`
	PartySize                  *int    `json:"party_size"`
	ReservationTime            *string `json:"reservation_time"`
	Duration                   *int    `json:"duration"`
	SpecialRequests            *string `json:"special_requests"`
	Notes                      *string `json:"notes"`
	Status                     *string `json:"status"`
	CancellationReason         *string `json:"cancellation_reason"`
	AllowOccupiedTableOverride bool    `json:"allow_occupied_table_override"`
}

type ReservationTransitionInput struct {
	TableID                    *uint  `json:"table_id"`
	Notes                      string `json:"notes"`
	Reason                     string `json:"reason"`
	AllowOccupiedTableOverride bool   `json:"allow_occupied_table_override"`
}

type PublicReservationRecordDTO struct {
	ID               uint       `json:"id"`
	CustomerName     string     `json:"customer_name"`
	CustomerEmail    string     `json:"customer_email,omitempty"`
	CustomerPhone    string     `json:"customer_phone,omitempty"`
	PartySize        int        `json:"party_size"`
	ReservationTime  time.Time  `json:"reservation_time"`
	Duration         int        `json:"duration"`
	Status           string     `json:"status"`
	Source           string     `json:"source,omitempty"`
	ConfirmationCode string     `json:"confirmation_code"`
	Language         string     `json:"language,omitempty"`
	SpecialRequests  string     `json:"special_requests,omitempty"`
	ConfirmedAt      *time.Time `json:"confirmed_at,omitempty"`
	AssignedAt       *time.Time `json:"assigned_at,omitempty"`
	SeatedAt         *time.Time `json:"seated_at,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	CancelledAt      *time.Time `json:"cancelled_at,omitempty"`
	WaitlistPosition *int       `json:"waitlist_position,omitempty"`
	TableName        string     `json:"table_name,omitempty"`
}

type PublicReservationDetailsDTO struct {
	Reservation       PublicReservationRecordDTO `json:"reservation"`
	BusinessName      string                     `json:"business_name"`
	BusinessPhone     string                     `json:"business_phone,omitempty"`
	BusinessAddress   string                     `json:"business_address,omitempty"`
	BusinessCustomURL string                     `json:"business_custom_url,omitempty"`
	// BusinessTimezone is the venue IANA TZ so guest confirmation pages format
	// reservation_time in restaurant wall-clock (not the guest device TZ).
	BusinessTimezone string `json:"business_timezone,omitempty"`
	CanCancel        bool   `json:"can_cancel"`
	// CanCancelReason explains a false CanCancel: status, not_allowed,
	// never_open (booked inside the deadline), or window_closed.
	CanCancelReason string `json:"can_cancel_reason,omitempty"`
	// CancellationDeadlineHours is the venue policy, surfaced so the guest
	// details page can explain a missing cancel button (#520).
	CancellationDeadlineHours int `json:"cancellation_deadline_hours,omitempty"`
}

type reservationAvailabilityContext struct {
	business *database.Business
	settings *database.ReservationSettings
	location *time.Location
	tables   []database.Table
	// Optional caches for multi-day settings summaries (and guest availability).
	// nil means "load on demand for this call".
	floorOccupancy    map[uint]database.ReservationTableOccupancy
	exceptions        map[string]*database.BusinessOperatingException
	operatingHours    map[int][]database.BusinessOperatingHours
	conflictCache     *reservationConflictSet
	conflictCacheFrom time.Time
	conflictCacheTo   time.Time
}

type slotCapacityResult struct {
	availableTables []database.Table
	reasonCode      string
}

type reservationConflictSet struct {
	all     []database.TableReservation
	byTable map[uint][]database.TableReservation
}

func (s *ReservationService) GetSettings(businessID uint) (*ReservationSettingsDTO, error) {
	settings, err := database.GetReservationSettings(businessID)
	if err != nil {
		return nil, err
	}
	return s.serializeReservationSettings(settings, true)
}

// GetPublicSettingsForBusiness is the public settings read that reuses the
// already-loaded storefront business (narrow public projection) instead of
// issuing another full-width businesses SELECT.
func (s *ReservationService) GetPublicSettingsForBusiness(business *database.Business) (*ReservationSettingsDTO, error) {
	if business == nil {
		return nil, fmt.Errorf("business not found")
	}
	settings, err := database.GetReservationSettingsForRead(business.ID)
	if err != nil {
		return nil, err
	}
	// A non-operational venue may keep an informational storefront, but public
	// reads must not advertise bookable slots the create path will refuse.
	if !database.IsBusinessOperational(business) {
		dto, err := s.serializeReservationSettingsForBusiness(settings, business, false)
		if err != nil {
			return nil, err
		}
		dto.Enabled = false
		dto.NextAvailableSlot = nil
		dto.AvailableSlotCount = 0
		return dto, nil
	}
	return s.serializeReservationSettingsForBusiness(settings, business, true)
}

func (s *ReservationService) UpdateSettings(businessID uint, input UpdateReservationSettingsInput) (*ReservationSettingsDTO, error) {
	settings, err := database.GetReservationSettings(businessID)
	if err != nil {
		return nil, err
	}

	if input.Enabled != nil {
		settings.Enabled = *input.Enabled
	}
	if input.MaxAdvanceDays != nil {
		settings.MaxAdvanceDays = *input.MaxAdvanceDays
	}
	if input.MinAdvanceMinutes != nil {
		settings.MinAdvanceMinutes = *input.MinAdvanceMinutes
	}
	if input.MinPartySize != nil {
		settings.MinPartySize = *input.MinPartySize
	}
	if input.MaxPartySize != nil {
		settings.MaxPartySize = *input.MaxPartySize
	}
	if input.DefaultDuration != nil {
		settings.DefaultDuration = *input.DefaultDuration
	}
	if input.SlotIntervalMinutes != nil {
		settings.SlotIntervalMinutes = *input.SlotIntervalMinutes
	}
	if input.ServiceBufferMinutes != nil {
		settings.ServiceBufferMinutes = *input.ServiceBufferMinutes
	}
	if input.MaxCoversPerSlot != nil {
		settings.MaxCoversPerSlot = *input.MaxCoversPerSlot
	}
	if input.AutoAssignTables != nil {
		settings.AutoAssignTables = *input.AutoAssignTables
	}
	if input.ApprovalMode != nil {
		settings.ApprovalMode = strings.TrimSpace(strings.ToLower(*input.ApprovalMode))
	}
	if input.AllowWaitlist != nil {
		settings.AllowWaitlist = *input.AllowWaitlist
	}
	if input.HoldDurationMinutes != nil {
		settings.HoldDurationMinutes = *input.HoldDurationMinutes
	}
	if input.AllowCancellation != nil {
		settings.AllowCancellation = *input.AllowCancellation
	}
	if input.CancellationDeadline != nil {
		settings.CancellationDeadline = *input.CancellationDeadline
	}
	if input.NoShowGraceMinutes != nil {
		settings.NoShowGraceMinutes = *input.NoShowGraceMinutes
	}
	if input.SendConfirmationEmail != nil {
		settings.SendConfirmationEmail = *input.SendConfirmationEmail
	}
	if input.SendReminderEmail != nil {
		settings.SendReminderEmail = *input.SendReminderEmail
	}
	if input.ReminderHoursBefore != nil {
		settings.ReminderHoursBefore = *input.ReminderHoursBefore
	}
	if input.ExternalPartnerLinks != nil {
		settings.ExternalPartnerLinks = database.JSONRawMessage(*input.ExternalPartnerLinks)
	}

	if err := s.validateReservationSettings(settings); err != nil {
		return nil, &ReservationSettingsValidationError{msg: err.Error()}
	}
	if err := database.UpdateReservationSettings(settings); err != nil {
		return nil, err
	}
	return s.serializeReservationSettings(settings, true)
}

// GetPublicAvailability is the guest availability read: settings are loaded
// once via the pure-read helper, and the already-fetched public business is
// reused so the handler does not re-query settings or SELECT * the business.
func (s *ReservationService) GetPublicAvailability(business *database.Business, date time.Time, partySize int) (*ReservationAvailabilityDTO, error) {
	if business == nil {
		return nil, fmt.Errorf("business not found")
	}
	if !database.IsBusinessOperational(business) {
		return nil, ErrReservationsDisabled
	}
	settings, err := database.GetReservationSettingsForRead(business.ID)
	if err != nil {
		return nil, err
	}
	if !settings.Enabled {
		return nil, ErrReservationsDisabled
	}
	ctx, err := s.loadAvailabilityContextForSettings(settings, business)
	if err != nil {
		return nil, err
	}
	return s.buildAvailabilityResponse(ctx, date, partySize)
}

// GetTableOptions combines reservation overlap/capacity with a single live
// occupancy snapshot. The occupancy *override* guard only applies to near-term
// bookings; future reservations should not be blocked by the table's current
// service. Recommendations are stricter: a table occupied right now is never
// recommended at any horizon.
func (s *ReservationService) GetTableOptions(
	businessID uint,
	reservationTime time.Time,
	duration int,
	partySize int,
	excludeReservationID uint,
) ([]ReservationTableOptionDTO, error) {
	ctx, err := s.loadAvailabilityContext(businessID)
	if err != nil {
		return nil, err
	}
	if err := validateReservationDuration(duration); err != nil {
		return nil, err
	}
	if partySize < ctx.settings.MinPartySize || partySize > ctx.settings.MaxPartySize {
		return nil, fmt.Errorf(
			"party size must be between %d and %d",
			ctx.settings.MinPartySize,
			ctx.settings.MaxPartySize,
		)
	}

	capacity, err := s.evaluateSlotCapacity(
		ctx,
		reservationTime,
		duration,
		partySize,
		excludeReservationID,
	)
	if err != nil {
		return nil, err
	}
	availableIDs := make(map[uint]struct{}, len(capacity.availableTables))
	for _, table := range capacity.availableTables {
		availableIDs[table.ID] = struct{}{}
	}

	tableIDs := make([]uint, 0, len(ctx.tables))
	for _, table := range ctx.tables {
		tableIDs = append(tableIDs, table.ID)
	}
	now := s.currentTimeUTC()
	occupancy, err := database.GetReservationTableOccupancySnapshot(
		s.db,
		businessID,
		tableIDs,
		now,
		StaleOccupiedTableAfter,
	)
	if err != nil {
		return nil, err
	}
	nearTerm := !reservationTime.After(now.Add(ReservationNearTermWindow))

	// Best-effort space name map for soft grouping in the table picker.
	// Missing spaces (deleted/archived) leave SpaceName empty; picker still works flat.
	spaceNames := map[uint]string{}
	{
		var spaceIDs []uint
		seen := map[uint]struct{}{}
		for _, table := range ctx.tables {
			if table.SpaceID == nil {
				continue
			}
			if _, ok := seen[*table.SpaceID]; ok {
				continue
			}
			seen[*table.SpaceID] = struct{}{}
			spaceIDs = append(spaceIDs, *table.SpaceID)
		}
		if len(spaceIDs) > 0 {
			var spaces []database.RestaurantSpace
			if err := s.db.Select("id", "name").
				Where("business_id = ? AND id IN ?", businessID, spaceIDs).
				Find(&spaces).Error; err == nil {
				for _, sp := range spaces {
					spaceNames[sp.ID] = sp.Name
				}
			}
		}
	}

	options := make([]ReservationTableOptionDTO, 0, len(ctx.tables))
	for _, table := range ctx.tables {
		state := occupancy[table.ID]
		_, reservationAvailable := availableIDs[table.ID]
		occupiedNow := state.State != database.TableOccupancyAvailable
		requiresOverride := nearTerm && occupiedNow
		option := ReservationTableOptionDTO{
			ID:                   table.ID,
			Name:                 table.Name,
			Capacity:             tableSeatCapacity(table),
			SpaceID:              table.SpaceID,
			OccupancyState:       state.State,
			ActiveBillID:         state.ActiveBillID,
			ActiveBillOpenedAt:   state.ActiveBillOpenedAt,
			ActiveBillAgeMinutes: int64(state.ActiveBillAge / time.Minute),
			ReservationAvailable: reservationAvailable,
			// A table that is serving guests right now is never a
			// recommendation, even for a booking hours away — the host should
			// see free tables first and pick an occupied one deliberately.
			Recommended:               reservationAvailable && !occupiedNow,
			RequiresOccupancyOverride: requiresOverride,
		}
		if table.SpaceID != nil {
			option.SpaceName = spaceNames[*table.SpaceID]
		}
		fitsParty := tableSeatCapacity(table) >= partySize
		switch {
		case !fitsParty:
			option.ConflictReason = "capacity"
			option.ReservationAvailable = false
			option.Recommended = false
		case !reservationAvailable:
			option.ConflictReason = "reservation_conflict"
		case requiresOverride:
			option.ConflictReason = string(state.State)
		}
		options = append(options, option)
	}
	return options, nil
}

func (s *ReservationService) currentTimeUTC() time.Time {
	clock := s.now
	if clock == nil {
		clock = time.Now
	}
	return clock().UTC()
}

func (s *ReservationService) reservationWriteGuards(
	reservationTime time.Time,
	allowOccupiedOverride bool,
) database.ReservationWriteGuards {
	now := s.currentTimeUTC()
	if reservationTime.After(now.Add(ReservationNearTermWindow)) {
		return database.ReservationWriteGuards{}
	}
	return database.ReservationWriteGuards{
		OccupancyCheckAt:      &now,
		StaleAfter:            StaleOccupiedTableAfter,
		AllowOccupiedOverride: allowOccupiedOverride,
	}
}

func (s *ReservationService) filterAutoAssignmentCandidates(
	businessID uint,
	reservationTime time.Time,
	candidates []database.Table,
) ([]database.Table, error) {
	guards := s.reservationWriteGuards(reservationTime, false)
	if guards.OccupancyCheckAt == nil || len(candidates) == 0 {
		return candidates, nil
	}
	tableIDs := make([]uint, 0, len(candidates))
	for _, table := range candidates {
		tableIDs = append(tableIDs, table.ID)
	}
	snapshot, err := database.GetReservationTableOccupancySnapshot(
		s.db,
		businessID,
		tableIDs,
		*guards.OccupancyCheckAt,
		StaleOccupiedTableAfter,
	)
	if err != nil {
		return nil, err
	}
	available := make([]database.Table, 0, len(candidates))
	for _, table := range candidates {
		if snapshot[table.ID].State == database.TableOccupancyAvailable {
			available = append(available, table)
		}
	}
	return available, nil
}

func (s *ReservationService) buildAvailabilityResponse(ctx *reservationAvailabilityContext, date time.Time, partySize int) (*ReservationAvailabilityDTO, error) {
	date = date.In(ctx.location)
	if partySize < ctx.settings.MinPartySize || partySize > ctx.settings.MaxPartySize {
		return nil, fmt.Errorf("party size must be between %d and %d", ctx.settings.MinPartySize, ctx.settings.MaxPartySize)
	}

	slots, err := s.buildAvailabilitySlots(ctx, date, partySize)
	if err != nil {
		return nil, err
	}

	response := &ReservationAvailabilityDTO{
		Date:              date.Format("2006-01-02"),
		PartySize:         partySize,
		AvailableSlots:    slots,
		TotalSlots:        len(slots),
		WaitlistAvailable: ctx.settings.AllowWaitlist,
	}
	for _, slot := range slots {
		if slot.AvailableTables > 0 {
			response.NextAvailableSlot = &slot.Time
			break
		}
	}
	return response, nil
}

func (s *ReservationService) CreateReservation(businessID uint, input CreateReservationInput, actor string, isGuest bool) (*database.TableReservation, error) {
	if err := validateCreateReservationFields(
		strings.TrimSpace(input.CustomerName),
		strings.TrimSpace(input.CustomerPhone),
		strings.TrimSpace(input.SpecialRequests),
	); err != nil {
		return nil, err
	}
	ctx, err := s.loadAvailabilityContext(businessID)
	if err != nil {
		return nil, err
	}
	if isGuest {
		input.TableID = nil
		input.Notes = ""
		input.Source = "customer"
		input.AllowOccupiedTableOverride = false
		if input.Duration != nil {
			return nil, ErrGuestDurationNotAllowed
		}
	}
	reservationTime, err := ParseReservationDateTime(input.ReservationTime, ctx.location)
	if err != nil {
		return nil, fmt.Errorf("invalid reservation_time")
	}
	reservationTime = reservationTime.In(ctx.location)
	duration := ctx.settings.DefaultDuration
	if !isGuest && input.Duration != nil && *input.Duration > 0 {
		duration = *input.Duration
	}

	candidateTables, err := s.validateReservationRequest(ctx, input.TableID, 0, reservationTime, duration, input.PartySize, isGuest)
	if err != nil {
		return nil, err
	}
	if input.TableID == nil {
		candidateTables, err = s.filterAutoAssignmentCandidates(businessID, reservationTime, candidateTables)
		if err != nil {
			return nil, err
		}
	}

	status := "confirmed"
	createdBy := actor
	source := strings.TrimSpace(input.Source)
	if source == "" {
		if isGuest {
			source = "customer"
		} else {
			source = actor
		}
	}
	if isGuest {
		createdBy = "customer"
		source = "customer"
		if ctx.settings.ApprovalMode == database.ReservationApprovalManual {
			status = "pending"
		}
	}

	var assignedTableID *uint
	if input.TableID != nil {
		if !tableFitsParty(candidateTables, *input.TableID, input.PartySize) {
			return nil, fmt.Errorf("selected table is too small for this party")
		}
		assignedTableID = input.TableID
	} else if ctx.settings.AutoAssignTables {
		if picked := firstTableFittingParty(candidateTables, input.PartySize); picked != nil {
			assignedTableID = &picked.ID
		}
	}

	if assignedTableID == nil && len(candidateTables) == 0 {
		if ctx.settings.AllowWaitlist {
			status = "waitlist"
		} else {
			return nil, fmt.Errorf("no availability for the requested slot")
		}
	}

	confirmationCode, err := uniqueReservationCode()
	if err != nil {
		return nil, err
	}

	reservation := &database.TableReservation{
		BusinessID:       businessID,
		TableID:          assignedTableID,
		CustomerName:     strings.TrimSpace(input.CustomerName),
		CustomerPhone:    strings.TrimSpace(input.CustomerPhone),
		CustomerEmail:    strings.TrimSpace(input.CustomerEmail),
		PartySize:        input.PartySize,
		ReservationTime:  reservationTime.UTC(),
		Duration:         duration,
		Status:           status,
		Source:           source,
		ConfirmationCode: confirmationCode,
		Language:         ResolveBookingLanguage(input.Language, ctx.business),
		SpecialRequests:  strings.TrimSpace(input.SpecialRequests),
		Notes:            strings.TrimSpace(input.Notes),
		CreatedBy:        createdBy,
	}
	if status == "confirmed" {
		now := time.Now().UTC()
		reservation.ConfirmedAt = &now
		if assignedTableID != nil {
			reservation.AssignedAt = &now
		}
	}

	// Create inside a transaction with row locking to prevent overbooking
	// races. The status-history write and the aggregate reload run inside the
	// same transaction so any post-insert failure rolls the reservation back —
	// a guest must never end up with a phantom row and no confirmation.
	// The business lock is taken before the waitlist position and the insert:
	// covers and unassigned holds are rechecked under it, because the check
	// above ran outside this transaction.
	var created *database.TableReservation
	err = database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := database.LockReservationBusinessTx(tx, businessID); err != nil {
			return err
		}
		if status != "waitlist" {
			if err := s.recheckSlotCapacityTx(tx, ctx, reservation.ReservationTime, duration, reservation.PartySize, assignedTableID, 0); err != nil {
				return err
			}
		} else {
			position, err := database.GetNextWaitlistPositionTx(tx, businessID, reservation.ReservationTime)
			if err != nil {
				return err
			}
			reservation.WaitlistPosition = &position
		}
		if err := database.CreateReservationTx(
			tx,
			reservation,
			ctx.settings.ServiceBufferMinutes,
			s.reservationWriteGuards(reservation.ReservationTime, input.AllowOccupiedTableOverride),
		); err != nil {
			return err
		}
		if err := database.CreateReservationStatusHistoryTx(tx, &database.ReservationStatusHistory{
			ReservationID: reservation.ID,
			Status:        reservation.Status,
			TableID:       reservation.TableID,
			Notes:         initialReservationHistoryNote(reservation),
			ChangedBy:     actor,
		}); err != nil {
			return err
		}
		loaded, err := database.GetReservationByBusinessAndIDTx(tx, businessID, reservation.ID)
		if err != nil {
			return err
		}
		created = loaded
		return nil
	})
	if err != nil {
		if errors.Is(err, database.ErrReservationSlotUnavailable) {
			return nil, fmt.Errorf("the selected time is no longer available")
		}
		return nil, err
	}

	// External side effect stays outside the transaction: it is queue-based
	// and failure-tolerant, and must only fire once the row is committed.
	enqueueTelegramReservationCreated(created, isGuest)
	return created, nil
}

// UpdateReservation applies the input and returns the updated row plus a
// snapshot of the reservation as loaded for this mutation. Callers that
// branch on pre-update state (e.g. the handler's one-email-per-outcome
// logic) must use the returned prior, not a read of their own — a separate
// earlier read races with concurrent updates.
func (s *ReservationService) UpdateReservation(businessID, reservationID uint, input UpdateReservationInput, actor string) (*database.TableReservation, *database.TableReservation, error) {
	reservation, err := database.GetReservationByBusinessAndID(businessID, reservationID)
	if err != nil {
		return nil, nil, err
	}
	priorSnapshot := *reservation
	prior := &priorSnapshot
	ctx, err := s.loadAvailabilityContext(businessID)
	if err != nil {
		return nil, nil, err
	}

	originalTime := reservation.ReservationTime
	originalDuration := reservation.Duration
	originalPartySize := reservation.PartySize
	originalTableID := reservation.TableID

	// The dashboard resends every field on edit, so only a value that actually
	// changes is validated: a legacy row that predates the field caps stays
	// editable for unrelated fields.
	if input.CustomerName != nil {
		if name := strings.TrimSpace(*input.CustomerName); name != reservation.CustomerName {
			if err := validateReservationCustomerName(name); err != nil {
				return nil, nil, err
			}
			reservation.CustomerName = name
		}
	}
	if input.CustomerPhone != nil {
		if phone := strings.TrimSpace(*input.CustomerPhone); phone != reservation.CustomerPhone {
			if err := validateReservationCustomerPhone(phone); err != nil {
				return nil, nil, err
			}
			reservation.CustomerPhone = phone
		}
	}
	if input.CustomerEmail != nil {
		reservation.CustomerEmail = strings.TrimSpace(*input.CustomerEmail)
	}
	if input.SpecialRequests != nil {
		if requests := strings.TrimSpace(*input.SpecialRequests); requests != reservation.SpecialRequests {
			if err := validateReservationSpecialRequests(requests); err != nil {
				return nil, nil, err
			}
			reservation.SpecialRequests = requests
		}
	}
	if input.Notes != nil {
		reservation.Notes = strings.TrimSpace(*input.Notes)
	}
	if input.PartySize != nil {
		reservation.PartySize = *input.PartySize
	}
	if input.Duration != nil && *input.Duration > 0 {
		reservation.Duration = *input.Duration
	}
	if input.ClearTable {
		reservation.TableID = nil
	}
	if input.TableID != nil {
		reservation.TableID = input.TableID
	}
	if input.ReservationTime != nil {
		parsedTime, err := ParseReservationDateTime(*input.ReservationTime, ctx.location)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid reservation_time")
		}
		reservation.ReservationTime = parsedTime.In(ctx.location).UTC()
	}

	// The dashboard resends every field on edit, so compare effective values
	// instead of trusting which pointers are set: an "edit phone number"
	// request must not re-run the booking-window validation that the
	// original (possibly past or near-term) reservation time can no longer
	// satisfy.
	timingChanged := !reservation.ReservationTime.Equal(originalTime) || reservation.Duration != originalDuration
	tableChanged := (originalTableID == nil) != (reservation.TableID == nil) ||
		(originalTableID != nil && reservation.TableID != nil && *originalTableID != *reservation.TableID)
	availabilityChanged := timingChanged || tableChanged || reservation.PartySize != originalPartySize
	// Position is assigned inside the write transaction, under the business
	// lock. Computing it here races with another update and hands out duplicates.
	needsWaitlistPosition := false

	if availabilityChanged {
		var candidateTables []database.Table
		if timingChanged {
			candidateTables, err = s.validateReservationRequest(ctx, reservation.TableID, reservation.ID, reservation.ReservationTime.In(ctx.location), reservation.Duration, reservation.PartySize, false)
		} else {
			candidateTables, err = s.validateReservationCapacity(ctx, reservation.TableID, reservation.ID, reservation.ReservationTime.In(ctx.location), reservation.Duration, reservation.PartySize)
		}
		if err != nil {
			return nil, nil, err
		}
		if reservation.TableID == nil {
			candidateTables, err = s.filterAutoAssignmentCandidates(
				businessID,
				reservation.ReservationTime,
				candidateTables,
			)
			if err != nil {
				return nil, nil, err
			}
		}
		if reservation.TableID == nil && len(candidateTables) == 0 && !ctx.settings.AllowWaitlist {
			return nil, nil, fmt.Errorf("no tables are available for the requested time")
		}
		if reservation.TableID == nil && len(candidateTables) == 0 && ctx.settings.AllowWaitlist {
			reservation.Status = "waitlist"
			if reservation.WaitlistPosition == nil {
				needsWaitlistPosition = true
			}
		}
		if reservation.TableID == nil && ctx.settings.AutoAssignTables && reservation.Status != "waitlist" {
			if picked := firstTableFittingParty(candidateTables, reservation.PartySize); picked != nil {
				reservation.TableID = &picked.ID
				now := time.Now().UTC()
				reservation.AssignedAt = &now
			}
		}
	}

	if input.Status != nil && strings.TrimSpace(*input.Status) != reservation.Status {
		normalized := normalizeReservationTransition(strings.TrimSpace(*input.Status))
		if normalized == "" {
			return nil, nil, fmt.Errorf("unsupported reservation transition")
		}
		transitionInput := ReservationTransitionInput{
			TableID:                    reservation.TableID,
			Notes:                      reservation.Notes,
			Reason:                     stringValue(input.CancellationReason),
			AllowOccupiedTableOverride: input.AllowOccupiedTableOverride,
		}
		updated, err := s.transitionReservation(ctx, reservation, normalized, transitionInput, actor)
		if err != nil {
			return nil, nil, err
		}
		return updated, prior, nil
	}

	writeGuards := database.ReservationWriteGuards{}
	if availabilityChanged {
		writeGuards = s.reservationWriteGuards(
			reservation.ReservationTime,
			input.AllowOccupiedTableOverride,
		)
	}
	if err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := database.LockReservationBusinessTx(tx, businessID); err != nil {
			return err
		}
		if needsWaitlistPosition {
			position, err := database.GetNextWaitlistPositionTx(tx, businessID, reservation.ReservationTime)
			if err != nil {
				return err
			}
			reservation.WaitlistPosition = &position
		}
		// The availability check above ran outside this transaction. Recheck
		// covers and unassigned holds under the business lock, or two
		// concurrent edits can each pass the stale read and overbook the slot.
		if availabilityChanged && reservationStatusHoldsCapacity(reservation.Status) {
			if err := s.recheckSlotCapacityTx(
				tx,
				ctx,
				reservation.ReservationTime,
				reservation.Duration,
				reservation.PartySize,
				reservation.TableID,
				reservation.ID,
			); err != nil {
				return err
			}
		}
		return database.UpdateReservationTx(
			tx,
			reservation,
			ctx.settings.ServiceBufferMinutes,
			writeGuards,
		)
	}); err != nil {
		if errors.Is(err, database.ErrReservationSlotUnavailable) {
			return nil, nil, fmt.Errorf("the selected time is no longer available")
		}
		return nil, nil, err
	}
	updated, err := database.GetReservationByBusinessAndID(businessID, reservationID)
	if err != nil {
		return nil, nil, err
	}
	return updated, prior, nil
}

func (s *ReservationService) TransitionReservation(businessID, reservationID uint, transition string, input ReservationTransitionInput, actor string) (*database.TableReservation, error) {
	reservation, err := database.GetReservationByBusinessAndID(businessID, reservationID)
	if err != nil {
		return nil, err
	}
	ctx, err := s.loadAvailabilityContext(businessID)
	if err != nil {
		return nil, err
	}
	normalized := normalizeReservationTransition(transition)
	if normalized == "" {
		return nil, fmt.Errorf("unsupported reservation transition")
	}

	return s.transitionReservation(ctx, reservation, normalized, input, actor)
}

func (s *ReservationService) GetPublicReservationDetailsByCode(confirmationCode string) (*PublicReservationDetailsDTO, error) {
	reservation, err := database.GetReservationByConfirmationCode(confirmationCode)
	if err != nil {
		return nil, err
	}

	ctx, err := s.loadAvailabilityContext(reservation.BusinessID)
	if err != nil {
		return nil, err
	}

	return s.buildPublicReservationDetails(ctx, reservation), nil
}

func (s *ReservationService) CancelReservationByCode(confirmationCode, actor string) (*database.TableReservation, bool, error) {
	reservation, err := database.GetReservationByConfirmationCode(confirmationCode)
	if err != nil {
		return nil, false, err
	}

	switch reservation.Status {
	case "cancelled":
		return reservation, false, nil
	case "no_show", "seated", "completed":
		return nil, false, fmt.Errorf("reservation can no longer be cancelled")
	}

	ctx, err := s.loadAvailabilityContext(reservation.BusinessID)
	if err != nil {
		return nil, false, err
	}

	if !ctx.settings.AllowCancellation {
		return nil, false, fmt.Errorf("cancellations are not allowed for this business")
	}

	if ctx.settings.CancellationDeadline > 0 {
		reservationTime := reservation.ReservationTime.In(ctx.location)
		cutoff := reservationTime.Add(-time.Duration(ctx.settings.CancellationDeadline) * time.Hour)
		now := s.currentTimeUTC().In(ctx.location)
		if !reservation.CreatedAt.In(ctx.location).Before(cutoff) {
			return nil, false, fmt.Errorf("this reservation cannot be cancelled online")
		}
		if now.After(cutoff) {
			return nil, false, fmt.Errorf("cancellation window has closed")
		}
	}

	updatedReservation, err := s.transitionReservation(ctx, reservation, "cancel", ReservationTransitionInput{
		Reason: reservationNoteCancelledByCustomer,
	}, actor)
	if err != nil {
		return nil, false, err
	}

	return updatedReservation, true, nil
}

func (s *ReservationService) buildPublicReservationDetails(ctx *reservationAvailabilityContext, reservation *database.TableReservation) *PublicReservationDetailsDTO {
	canCancel := ctx.settings.AllowCancellation
	canCancelReason := ""

	switch reservation.Status {
	case "cancelled", "no_show", "seated", "completed":
		canCancel = false
		canCancelReason = "status"
	}

	if !canCancel && canCancelReason == "" && !ctx.settings.AllowCancellation {
		canCancelReason = "not_allowed"
	}

	if canCancel && ctx.settings.CancellationDeadline > 0 {
		reservationTime := reservation.ReservationTime.In(ctx.location)
		cutoff := reservationTime.Add(-time.Duration(ctx.settings.CancellationDeadline) * time.Hour)
		now := s.currentTimeUTC().In(ctx.location)
		if !reservation.CreatedAt.In(ctx.location).Before(cutoff) {
			canCancel = false
			canCancelReason = "never_open"
		} else if now.After(cutoff) {
			canCancel = false
			canCancelReason = "window_closed"
		}
	}

	record := PublicReservationRecordDTO{
		ID:               reservation.ID,
		CustomerName:     reservation.CustomerName,
		CustomerEmail:    reservation.CustomerEmail,
		CustomerPhone:    reservation.CustomerPhone,
		PartySize:        reservation.PartySize,
		ReservationTime:  reservation.ReservationTime,
		Duration:         reservation.Duration,
		Status:           reservation.Status,
		Source:           reservation.Source,
		ConfirmationCode: reservation.ConfirmationCode,
		Language:         reservation.Language,
		SpecialRequests:  reservation.SpecialRequests,
		ConfirmedAt:      reservation.ConfirmedAt,
		AssignedAt:       reservation.AssignedAt,
		SeatedAt:         reservation.SeatedAt,
		CompletedAt:      reservation.CompletedAt,
		CancelledAt:      reservation.CancelledAt,
		WaitlistPosition: reservation.WaitlistPosition,
	}

	if reservation.Table != nil {
		record.TableName = reservation.Table.Name
	}

	return &PublicReservationDetailsDTO{
		Reservation:               record,
		BusinessName:              ctx.business.Name,
		BusinessPhone:             ctx.business.Phone,
		BusinessAddress:           FormatBusinessAddress(ctx.business),
		BusinessCustomURL:         ctx.business.CustomURL,
		BusinessTimezone:          ctx.business.Timezone,
		CanCancel:                 canCancel,
		CanCancelReason:           canCancelReason,
		CancellationDeadlineHours: ctx.settings.CancellationDeadline,
	}
}

func (s *ReservationService) transitionReservation(ctx *reservationAvailabilityContext, reservation *database.TableReservation, normalized string, input ReservationTransitionInput, actor string) (*database.TableReservation, error) {
	now := s.currentTimeUTC()
	oldStatus := reservation.Status
	shouldEnsureBill := false

	availableCandidates := func() ([]database.Table, error) {
		candidateTables, err := s.validateReservationCapacity(
			ctx,
			nil,
			reservation.ID,
			reservation.ReservationTime.In(ctx.location),
			reservation.Duration,
			reservation.PartySize,
		)
		if err != nil {
			return nil, err
		}
		return s.filterAutoAssignmentCandidates(
			reservation.BusinessID,
			reservation.ReservationTime,
			candidateTables,
		)
	}

	if input.TableID != nil {
		candidateTables, err := s.validateReservationCapacity(ctx, input.TableID, reservation.ID, reservation.ReservationTime.In(ctx.location), reservation.Duration, reservation.PartySize)
		if err != nil {
			return nil, err
		}
		if len(candidateTables) == 0 || !tableFitsParty(candidateTables, *input.TableID, reservation.PartySize) {
			return nil, fmt.Errorf("selected table is too small for this party")
		}
		reservation.TableID = input.TableID
		reservation.AssignedAt = &now
	}

	switch normalized {
	case "assign_table":
		if reservation.TableID == nil {
			candidateTables, err := availableCandidates()
			if err != nil {
				return nil, err
			}
			picked := firstTableFittingParty(candidateTables, reservation.PartySize)
			if picked == nil {
				return nil, fmt.Errorf("no available table for this reservation")
			}
			reservation.TableID = &picked.ID
		}
		reservation.AssignedAt = &now
		if reservation.Status == "waitlist" {
			reservation.Status = "pending"
			reservation.WaitlistPosition = nil
		}
	case "confirm":
		if reservation.TableID == nil {
			candidateTables, err := availableCandidates()
			if err != nil {
				return nil, err
			}
			if ctx.settings.AutoAssignTables {
				picked := firstTableFittingParty(candidateTables, reservation.PartySize)
				if picked == nil {
					return nil, fmt.Errorf("no available table for this reservation")
				}
				reservation.TableID = &picked.ID
				reservation.AssignedAt = &now
			} else if len(candidateTables) == 0 {
				return nil, fmt.Errorf("no available table for this reservation")
			}
		}
		reservation.Status = "confirmed"
		reservation.ConfirmedAt = &now
		reservation.WaitlistPosition = nil
	case "check_in", "seat", "seated":
		// L1-13: reject seat/check-in before the reservation's temporal window.
		// R2-B5: seating honors the shared early-arrival grace so a guest who
		// shows up 15 minutes early can still be seated.
		if !CanSeatReservation(reservation.ReservationTime, now) {
			return nil, fmt.Errorf("cannot seat a future-dated reservation")
		}
		if reservation.TableID == nil {
			candidateTables, err := availableCandidates()
			if err != nil {
				return nil, err
			}
			picked := firstTableFittingParty(candidateTables, reservation.PartySize)
			if picked == nil {
				return nil, fmt.Errorf("no available table for this reservation")
			}
			reservation.TableID = &picked.ID
			reservation.AssignedAt = &now
		}
		reservation.Status = "seated"
		if reservation.ConfirmedAt == nil {
			reservation.ConfirmedAt = &now
		}
		reservation.SeatedAt = &now
		reservation.WaitlistPosition = nil
		shouldEnsureBill = true
	case "complete":
		reservation.Status = "completed"
		reservation.CompletedAt = &now
	case "cancel":
		reservation.Status = "cancelled"
		reservation.CancelledAt = &now
		reservation.CancelledBy = actor
		reservation.CancellationReason = strings.TrimSpace(input.Reason)
	case "no_show":
		// L1-13: reject no-show marks on future-dated rows. No early grace —
		// a guest is not a no-show until the reservation has started.
		if !CanMarkNoShow(reservation.ReservationTime, now) {
			return nil, fmt.Errorf("cannot mark no-show on a future-dated reservation")
		}
		reservation.Status = "no_show"
		reservation.CancelledAt = &now
		reservation.CancelledBy = actor
		reservation.CancellationReason = strings.TrimSpace(input.Reason)
	case "promote_waitlist":
		candidateTables, err := availableCandidates()
		if err != nil {
			return nil, err
		}
		if len(candidateTables) == 0 {
			return nil, fmt.Errorf("no available table to promote waitlist reservation")
		}
		if reservation.TableID == nil {
			reservation.TableID = &candidateTables[0].ID
			reservation.AssignedAt = &now
		}
		reservation.Status = "confirmed"
		reservation.ConfirmedAt = &now
		reservation.WaitlistPosition = nil
	default:
		reservation.Status = normalized
	}

	// Guard the write against a concurrent status change (the auto-decline
	// sweeper, a guest cancel-by-code, or another operator). Without this, the
	// full-row Save would silently clobber whatever the other actor committed —
	// e.g. resurrecting a swept-cancelled reservation to confirmed and firing
	// contradictory guest emails. On conflict we abort and the caller re-reads.
	guardedTransition := input.TableID != nil || normalized == "assign_table" ||
		normalized == "confirm" || normalized == "check_in" ||
		normalized == "seat" || normalized == "seated" || normalized == "promote_waitlist"
	guards := database.ReservationWriteGuards{}
	if guardedTransition {
		guards = s.reservationWriteGuards(
			reservation.ReservationTime,
			input.AllowOccupiedTableOverride,
		)
	}
	// Validate an existing active bill before persisting the seated status.
	// GetOpenBillByTableID also validates its authoritative item snapshot; if
	// that data is malformed, leaving the reservation confirmed is safer than
	// recording a seated guest whose bill cannot be opened by operators.
	if shouldEnsureBill {
		if err := s.preflightBillForReservation(reservation); err != nil {
			return nil, err
		}
	}
	// Reactivating a non-active row (waitlist, cancelled, no-show, completed)
	// consumes covers that the outside-transaction check did not hold a lock
	// for. Recheck under the same business lock as create, excluding this row.
	activating := reservationStatusHoldsCapacity(reservation.Status) && !reservationStatusHoldsCapacity(oldStatus)
	if err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		if activating {
			if err := database.LockReservationBusinessTx(tx, reservation.BusinessID); err != nil {
				return err
			}
			if err := s.recheckSlotCapacityTx(
				tx,
				ctx,
				reservation.ReservationTime,
				reservation.Duration,
				reservation.PartySize,
				reservation.TableID,
				reservation.ID,
			); err != nil {
				return err
			}
		}
		return database.UpdateReservationStatusGuardedTx(
			tx,
			reservation,
			oldStatus,
			ctx.settings.ServiceBufferMinutes,
			guards,
		)
	}); err != nil {
		return nil, err
	}
	if shouldEnsureBill {
		if err := s.ensureBillForReservation(reservation); err != nil {
			return nil, err
		}
	}
	var historyID uint
	if reservation.Status != oldStatus || normalized == "assign_table" || input.TableID != nil {
		hist := &database.ReservationStatusHistory{
			ReservationID: reservation.ID,
			Status:        reservation.Status,
			TableID:       reservation.TableID,
			Notes:         buildReservationTransitionNote(oldStatus, reservation, normalized, input),
			ChangedBy:     actor,
		}
		_ = database.CreateReservationStatusHistory(hist)
		historyID = hist.ID
	}
	updated, err := database.GetReservationByBusinessAndID(reservation.BusinessID, reservation.ID)
	if err != nil {
		return nil, err
	}
	if updated.Status != oldStatus || normalized == "assign_table" || input.TableID != nil {
		enqueueTelegramReservationStatusChanged(updated, historyID)
	}
	return updated, nil
}

func enqueueTelegramReservationCreated(reservation *database.TableReservation, isGuest bool) {
	if reservation == nil {
		return
	}
	// Same eligibility gate as guest orders / manual payments / inventory:
	// don't enqueue into the void for businesses without a connected Telegram
	// config or with this event disabled.
	if !ShouldEnqueueTelegramNotification(reservation.BusinessID, PluginEventReservationCreated) {
		return
	}
	payload := reservationNotificationPayload(reservation)
	payload["is_guest"] = isGuest
	if _, _, err := EnqueuePluginNotification(PluginNotificationEvent{
		BusinessID: reservation.BusinessID,
		EventType:  PluginEventReservationCreated,
		EventID:    fmt.Sprintf("reservation:%d", reservation.ID),
		Payload:    payload,
		CreatedAt:  time.Now().UTC(),
	}, "telegram"); err != nil {
		log.Printf("Failed to enqueue Telegram reservation notification for business_id=%d reservation_id=%d: %v", reservation.BusinessID, reservation.ID, err)
	}
}

func enqueueTelegramReservationStatusChanged(reservation *database.TableReservation, transitionID uint) {
	if reservation == nil {
		return
	}
	// Parity with enqueueTelegramReservationCreated: gate before writing the
	// outbox row (see comment there).
	if !ShouldEnqueueTelegramNotification(reservation.BusinessID, PluginEventReservationStatusChanged) {
		return
	}
	payload := reservationNotificationPayload(reservation)
	// Key the dedup by the status-history row id (unique per transition), not by
	// the status value. Keying on status silently drops a legitimate re-entry of a
	// status seen before — e.g. cancel → reinstate back to "confirmed" collides
	// with the earlier "confirmed" and the operator's Telegram gets nothing. The
	// history id still dedups retries of THIS transition (same id) while letting a
	// re-entered status produce a fresh notification.
	eventID := fmt.Sprintf("reservation:%d:status:%s:%d", reservation.ID, reservation.Status, transitionID)
	if transitionID == 0 {
		// History row id unavailable (creation failed): fall back to the write
		// timestamp so distinct transitions still get distinct keys.
		eventID = fmt.Sprintf("reservation:%d:status:%s:t%d", reservation.ID, reservation.Status, reservation.UpdatedAt.UnixNano())
	}
	if _, _, err := EnqueuePluginNotification(PluginNotificationEvent{
		BusinessID: reservation.BusinessID,
		EventType:  PluginEventReservationStatusChanged,
		EventID:    eventID,
		Payload:    payload,
		CreatedAt:  time.Now().UTC(),
	}, "telegram"); err != nil {
		log.Printf("Failed to enqueue Telegram reservation status notification for business_id=%d reservation_id=%d status=%s: %v", reservation.BusinessID, reservation.ID, reservation.Status, err)
	}
}

func reservationNotificationPayload(reservation *database.TableReservation) map[string]interface{} {
	tableName := ""
	var tableID interface{}
	if reservation.TableID != nil {
		tableID = *reservation.TableID
	}
	if reservation.Table != nil && strings.TrimSpace(reservation.Table.Name) != "" {
		tableName = reservation.Table.Name
	}
	timezone := "UTC"
	if strings.TrimSpace(reservation.Business.Timezone) != "" {
		timezone = reservation.Business.Timezone
	}
	return map[string]interface{}{
		"reservation_id":    reservation.ID,
		"confirmation_code": reservation.ConfirmationCode,
		"customer_name":     reservation.CustomerName,
		"party_size":        reservation.PartySize,
		"reservation_time":  reservation.ReservationTime.Format(time.RFC3339),
		"timezone":          timezone,
		"status":            reservation.Status,
		"table_id":          tableID,
		"table_name":        tableName,
		// Deep link into the operator dashboard so the Telegram notification
		// lands where the reservation can be approved/declined/seated.
		"dashboard_url": fmt.Sprintf("%s/business/%d/dashboard?tab=reservations", config.FrontendBaseURL(), reservation.BusinessID),
	}
}

func (s *ReservationService) serializeReservationSettings(settings *database.ReservationSettings, includeMetrics bool) (*ReservationSettingsDTO, error) {
	return s.serializeReservationSettingsForBusiness(settings, nil, includeMetrics)
}

func (s *ReservationService) serializeReservationSettingsForBusiness(settings *database.ReservationSettings, business *database.Business, includeMetrics bool) (*ReservationSettingsDTO, error) {
	response := &ReservationSettingsDTO{
		ID:                    settings.ID,
		BusinessID:            settings.BusinessID,
		Enabled:               settings.Enabled,
		MaxAdvanceDays:        settings.MaxAdvanceDays,
		MinAdvanceMinutes:     settings.MinAdvanceMinutes,
		MinPartySize:          settings.MinPartySize,
		MaxPartySize:          settings.MaxPartySize,
		DefaultDuration:       settings.DefaultDuration,
		SlotIntervalMinutes:   settings.SlotIntervalMinutes,
		ServiceBufferMinutes:  settings.ServiceBufferMinutes,
		MaxCoversPerSlot:      settings.MaxCoversPerSlot,
		AutoAssignTables:      settings.AutoAssignTables,
		ApprovalMode:          settings.ApprovalMode,
		AllowWaitlist:         settings.AllowWaitlist,
		HoldDurationMinutes:   settings.HoldDurationMinutes,
		AllowCancellation:     settings.AllowCancellation,
		CancellationDeadline:  settings.CancellationDeadline,
		NoShowGraceMinutes:    settings.NoShowGraceMinutes,
		SendConfirmationEmail: settings.SendConfirmationEmail,
		SendReminderEmail:     settings.SendReminderEmail,
		ReminderHoursBefore:   settings.ReminderHoursBefore,
		ExternalPartnerLinks:  json.RawMessage(settings.ExternalPartnerLinks),
		CreatedAt:             settings.CreatedAt,
		UpdatedAt:             settings.UpdatedAt,
	}
	if !includeMetrics {
		return response, nil
	}

	ctx, err := s.loadAvailabilityContextForSettings(settings, business)
	if err != nil {
		return response, nil
	}
	// Warm caches once so the 7-day lookahead does not re-query exceptions,
	// operating hours, floor occupancy, or reservation conflicts per day.
	_ = s.ensureAvailabilityCaches(ctx)
	today := time.Now().UTC()
	prefetchFrom := today.AddDate(0, 0, -2)
	prefetchTo := today.AddDate(0, 0, 8).Add(36 * time.Hour)
	_ = s.prefetchReservationConflicts(ctx, prefetchFrom, prefetchTo)
	for offset := 0; offset < 7; offset++ {
		availability, err := s.buildAvailabilityResponse(ctx, today.AddDate(0, 0, offset), max(settings.MinPartySize, 2))
		if err != nil {
			continue
		}
		dayAvailableCount := 0
		for _, slot := range availability.AvailableSlots {
			if slot.AvailableTables > 0 {
				dayAvailableCount++
			}
		}
		response.AvailableSlotCount += dayAvailableCount
		if response.NextAvailableSlot == nil && availability.NextAvailableSlot != nil {
			response.NextAvailableSlot = availability.NextAvailableSlot
		}
	}
	return response, nil
}

func (s *ReservationService) loadAvailabilityContext(businessID uint) (*reservationAvailabilityContext, error) {
	settings, err := database.GetReservationSettingsForRead(businessID)
	if err != nil {
		return nil, err
	}
	return s.loadAvailabilityContextForSettings(settings, nil)
}

func (s *ReservationService) loadAvailabilityContextForSettings(settings *database.ReservationSettings, business *database.Business) (*reservationAvailabilityContext, error) {
	if business == nil {
		var err error
		business, err = database.GetPublicBusinessByID(settings.BusinessID)
		if err != nil {
			return nil, err
		}
	}
	location, err := loadBusinessLocation(business.Timezone)
	if err != nil {
		return nil, err
	}
	tables, err := database.GetTablesByBusinessID(settings.BusinessID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(tables, func(i, j int) bool {
		if tables[i].Capacity == tables[j].Capacity {
			return tables[i].ID < tables[j].ID
		}
		return tables[i].Capacity < tables[j].Capacity
	})
	return &reservationAvailabilityContext{business: business, settings: settings, location: location, tables: tables}, nil
}

func (s *ReservationService) ensureAvailabilityCaches(ctx *reservationAvailabilityContext) error {
	if ctx.exceptions == nil {
		rows, err := database.GetPublicBusinessOperatingExceptions(ctx.business.ID)
		if err != nil {
			msg := strings.ToLower(err.Error())
			if !strings.Contains(msg, "no such table") &&
				!strings.Contains(msg, "does not exist") &&
				!strings.Contains(msg, "undefined_table") {
				return err
			}
			rows = nil
		}
		ctx.exceptions = make(map[string]*database.BusinessOperatingException, len(rows))
		for i := range rows {
			key := rows[i].ExceptionDate.UTC().Format("2006-01-02")
			row := rows[i]
			ctx.exceptions[key] = &row
		}
	}
	if ctx.floorOccupancy == nil {
		tableIDs := make([]uint, 0, len(ctx.tables))
		for _, table := range ctx.tables {
			tableIDs = append(tableIDs, table.ID)
		}
		occupancy, err := database.GetReservationTableOccupancySnapshot(
			s.db,
			ctx.business.ID,
			tableIDs,
			s.currentTimeUTC(),
			StaleOccupiedTableAfter,
		)
		if err != nil {
			msg := strings.ToLower(err.Error())
			if !strings.Contains(msg, "no such table") &&
				!strings.Contains(msg, "does not exist") &&
				!strings.Contains(msg, "undefined_table") {
				return err
			}
			occupancy = map[uint]database.ReservationTableOccupancy{}
		}
		ctx.floorOccupancy = occupancy
	}
	if ctx.operatingHours == nil {
		rows, err := database.GetBusinessOperatingHours(ctx.business.ID)
		if err != nil {
			msg := strings.ToLower(err.Error())
			if !strings.Contains(msg, "no such table") &&
				!strings.Contains(msg, "does not exist") &&
				!strings.Contains(msg, "undefined_table") {
				return err
			}
			rows = nil
		}
		ctx.operatingHours = make(map[int][]database.BusinessOperatingHours, 7)
		for i := range rows {
			day := rows[i].DayOfWeek
			ctx.operatingHours[day] = append(ctx.operatingHours[day], rows[i])
		}
	}
	return nil
}

func (s *ReservationService) validateReservationSettings(settings *database.ReservationSettings) error {
	if settings.MinPartySize < 1 {
		return fmt.Errorf("min_party_size must be at least 1")
	}
	if settings.MaxPartySize < settings.MinPartySize {
		return fmt.Errorf("max_party_size must be greater than or equal to min_party_size")
	}
	if settings.MaxAdvanceDays < 1 {
		return fmt.Errorf("max_advance_days must be at least 1")
	}
	if settings.MinAdvanceMinutes < 0 {
		return fmt.Errorf("min_advance_minutes cannot be negative")
	}
	if settings.DefaultDuration < 15 {
		return fmt.Errorf("default_duration must be at least 15 minutes")
	}
	if settings.DefaultDuration > database.MaxReservationDurationMinutes {
		return fmt.Errorf("default_duration must be at most %d minutes", database.MaxReservationDurationMinutes)
	}
	if settings.SlotIntervalMinutes < 5 {
		return fmt.Errorf("slot_interval_minutes must be at least 5 minutes")
	}
	if settings.MaxCoversPerSlot < 0 {
		return fmt.Errorf("max_covers_per_slot cannot be negative")
	}
	if settings.ServiceBufferMinutes < 0 || settings.NoShowGraceMinutes < 0 || settings.HoldDurationMinutes < 0 {
		return fmt.Errorf("buffer and timing controls cannot be negative")
	}
	if settings.DefaultDuration+settings.ServiceBufferMinutes > database.MaxReservationDurationMinutes {
		return fmt.Errorf("default_duration plus service_buffer_minutes must be at most %d minutes", database.MaxReservationDurationMinutes)
	}
	if settings.CancellationDeadline < 0 {
		return fmt.Errorf("cancellation_deadline cannot be negative")
	}
	if settings.ReminderHoursBefore < 0 {
		return fmt.Errorf("reminder_hours_before cannot be negative")
	}
	if settings.ApprovalMode != database.ReservationApprovalAuto && settings.ApprovalMode != database.ReservationApprovalManual {
		return fmt.Errorf("approval_mode must be auto or manual")
	}
	if len(settings.ExternalPartnerLinks) == 0 {
		settings.ExternalPartnerLinks = database.JSONRawMessage("[]")
	}
	return nil
}

func (s *ReservationService) validateReservationRequest(ctx *reservationAvailabilityContext, tableID *uint, excludeReservationID uint, reservationTime time.Time, duration, partySize int, enforceMinAdvance bool) ([]database.Table, error) {
	if partySize < ctx.settings.MinPartySize || partySize > ctx.settings.MaxPartySize {
		return nil, fmt.Errorf("party size must be between %d and %d", ctx.settings.MinPartySize, ctx.settings.MaxPartySize)
	}
	if err := validateReservationDuration(duration); err != nil {
		return nil, err
	}
	if duration+ctx.settings.ServiceBufferMinutes > database.MaxReservationDurationMinutes {
		return nil, fmt.Errorf("duration plus the %d-minute service buffer must be at most %d minutes", ctx.settings.ServiceBufferMinutes, database.MaxReservationDurationMinutes)
	}
	if err := s.validateReservationWindow(ctx, reservationTime, duration, enforceMinAdvance); err != nil {
		return nil, err
	}

	return s.validateReservationCapacity(ctx, tableID, excludeReservationID, reservationTime, duration, partySize)
}

func (s *ReservationService) validateReservationCapacity(ctx *reservationAvailabilityContext, tableID *uint, excludeReservationID uint, reservationTime time.Time, duration, partySize int) ([]database.Table, error) {
	if partySize < ctx.settings.MinPartySize || partySize > ctx.settings.MaxPartySize {
		return nil, fmt.Errorf("party size must be between %d and %d", ctx.settings.MinPartySize, ctx.settings.MaxPartySize)
	}
	if err := validateReservationDuration(duration); err != nil {
		return nil, err
	}
	if duration+ctx.settings.ServiceBufferMinutes > database.MaxReservationDurationMinutes {
		return nil, fmt.Errorf("duration plus the %d-minute service buffer must be at most %d minutes", ctx.settings.ServiceBufferMinutes, database.MaxReservationDurationMinutes)
	}

	capacity, err := s.evaluateSlotCapacity(ctx, reservationTime, duration, partySize, excludeReservationID)
	if err != nil {
		return nil, err
	}
	if len(capacity.availableTables) == 0 {
		if tableID != nil {
			return nil, fmt.Errorf("%s", humanizeReservationReason(capacity.reasonCode))
		}
		return []database.Table{}, nil
	}

	if tableID != nil {
		for _, table := range capacity.availableTables {
			if table.ID == *tableID {
				return []database.Table{table}, nil
			}
		}
		return nil, fmt.Errorf("selected table is not available")
	}
	return capacity.availableTables, nil
}

func (s *ReservationService) validateReservationWindow(ctx *reservationAvailabilityContext, reservationTime time.Time, duration int, enforceMinAdvance bool) error {
	reservationTime = reservationTime.In(ctx.location)
	now := s.currentTimeUTC().In(ctx.location)
	// Staff book walk-ins and near-term parties at the door; the advance
	// window only constrains guest self-service bookings.
	if enforceMinAdvance {
		minAdvance := now.Add(time.Duration(ctx.settings.MinAdvanceMinutes) * time.Minute)
		if reservationTime.Before(minAdvance) {
			return fmt.Errorf("reservations must be made at least %d minutes in advance", ctx.settings.MinAdvanceMinutes)
		}
	}
	// Even at the door, a slot whose entire window has already elapsed can
	// only be an input error: the row would be invisible under every horizon
	// filter and occupy the table's history for nothing (L1-1). A party seated
	// minutes ago — slot still running — stays bookable.
	if !reservationTime.Add(time.Duration(duration) * time.Minute).After(now) {
		return fmt.Errorf("reservation time is in the past")
	}
	maxAdvance := now.AddDate(0, 0, ctx.settings.MaxAdvanceDays)
	if reservationTime.After(maxAdvance) {
		return fmt.Errorf("reservations can only be made up to %d days in advance", ctx.settings.MaxAdvanceDays)
	}
	periods, isClosed, err := s.getOperatingPeriods(ctx, reservationTime)
	if err != nil {
		return err
	}
	if isClosed || len(periods) == 0 {
		return fmt.Errorf("business is closed for the selected date")
	}
	for _, period := range periods {
		if !reservationTime.Before(period.start) && reservationFitsOperatingWindow(reservationTime, duration, ctx.settings.ServiceBufferMinutes, period.serviceEnd) {
			return nil
		}
	}
	return fmt.Errorf("reservation must fall within business operating hours")
}

// reservationFitsOperatingWindow is the single source of truth for whether a
// reservation starting at slot can finish (including the service buffer)
// before the operating window closes. Availability and create/update
// validation must always agree on this rule.
func reservationFitsOperatingWindow(slot time.Time, duration, serviceBufferMinutes int, windowEnd time.Time) bool {
	return !slot.Add(time.Duration(duration+serviceBufferMinutes) * time.Minute).After(windowEnd)
}

func (s *ReservationService) buildAvailabilitySlots(ctx *reservationAvailabilityContext, date time.Time, partySize int) ([]ReservationAvailabilitySlotDTO, error) {
	if err := s.ensureAvailabilityCaches(ctx); err != nil {
		return nil, err
	}
	periods, isClosed, err := s.getOperatingPeriods(ctx, date)
	if err != nil {
		return nil, err
	}
	if isClosed || len(periods) == 0 {
		return []ReservationAvailabilitySlotDTO{}, nil
	}
	interval := max(ctx.settings.SlotIntervalMinutes, 5)
	now := s.currentTimeUTC().In(ctx.location)
	minAdvance := now.Add(time.Duration(ctx.settings.MinAdvanceMinutes) * time.Minute)
	maxAdvance := now.AddDate(0, 0, ctx.settings.MaxAdvanceDays)

	windowStart := periods[0].start
	windowEnd := periods[0].end
	for _, period := range periods[1:] {
		if period.start.Before(windowStart) {
			windowStart = period.start
		}
		if period.end.After(windowEnd) {
			windowEnd = period.end
		}
	}
	conflictEnd := windowEnd.Add(time.Duration(ctx.settings.DefaultDuration+ctx.settings.ServiceBufferMinutes) * time.Minute)
	conflicts, err := s.loadReservationConflicts(ctx, windowStart, conflictEnd, 0)
	if err != nil {
		return nil, err
	}

	occupancy := ctx.floorOccupancy
	if occupancy == nil {
		occupancy = map[uint]database.ReservationTableOccupancy{}
	}

	slots := make([]ReservationAvailabilitySlotDTO, 0)
	for _, period := range periods {
		for slot := period.start; !slot.After(period.serviceEnd); slot = slot.Add(time.Duration(interval) * time.Minute) {
			if slot.Equal(period.serviceEnd) {
				break
			}
			if slot.Before(minAdvance) {
				slots = append(slots, ReservationAvailabilitySlotDTO{
					Time:            slot.UTC().Format(time.RFC3339),
					AvailableTables: 0,
					Recommended:     false,
					ReasonCode:      "outside_advance_window",
				})
				continue
			}
			if slot.After(maxAdvance) {
				slots = append(slots, ReservationAvailabilitySlotDTO{
					Time:            slot.UTC().Format(time.RFC3339),
					AvailableTables: 0,
					Recommended:     false,
					ReasonCode:      "reservation_max_advance",
				})
				continue
			}
			if !reservationFitsOperatingWindow(slot, ctx.settings.DefaultDuration, ctx.settings.ServiceBufferMinutes, period.serviceEnd) {
				slots = append(slots, ReservationAvailabilitySlotDTO{
					Time:            slot.UTC().Format(time.RFC3339),
					AvailableTables: 0,
					Recommended:     false,
					ReasonCode:      "outside_operating_window",
				})
				continue
			}
			applyOccupancy := !slot.After(s.currentTimeUTC().Add(ReservationNearTermWindow))
			capacity := s.evaluateSlotCapacityFromConflicts(
				ctx, conflicts, slot, ctx.settings.DefaultDuration, partySize, occupancy, applyOccupancy,
			)
			availableTables := len(capacity.availableTables)
			slots = append(slots, ReservationAvailabilitySlotDTO{
				Time:            slot.UTC().Format(time.RFC3339),
				AvailableTables: availableTables,
				Recommended:     false,
				ReasonCode:      capacity.reasonCode,
			})
		}
	}
	applySparseRecommendations(slots)
	return slots, nil
}

// applySparseRecommendations marks at most one open slot as recommended.
// Equivalent-capacity days get no badge; mixed capacity keeps the soonest
// slot that still has the most tables. Waitlist and closed windows stay unmarked.
func applySparseRecommendations(slots []ReservationAvailabilitySlotDTO) {
	open := make([]int, 0, len(slots))
	minTables := 0
	maxTables := 0
	for i := range slots {
		slots[i].Recommended = false
		tables := slots[i].AvailableTables
		if tables <= 0 {
			continue
		}
		if len(open) == 0 {
			minTables = tables
			maxTables = tables
		} else {
			if tables < minTables {
				minTables = tables
			}
			if tables > maxTables {
				maxTables = tables
			}
		}
		open = append(open, i)
	}
	if len(open) < 2 || minTables == maxTables {
		return
	}

	best := -1
	var bestTime time.Time
	hasBestTime := false
	for _, i := range open {
		if slots[i].AvailableTables != maxTables {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, slots[i].Time)
		if err != nil {
			if best < 0 || slots[i].Time < slots[best].Time {
				best = i
			}
			continue
		}
		if !hasBestTime || parsed.Before(bestTime) {
			best = i
			bestTime = parsed
			hasBestTime = true
		}
	}
	if best >= 0 {
		slots[best].Recommended = true
	}
}

func (s *ReservationService) evaluateSlotCapacity(ctx *reservationAvailabilityContext, reservationTime time.Time, duration, partySize int, excludeReservationID uint) (*slotCapacityResult, error) {
	startTime := reservationTime.In(ctx.location)
	conflictEnd := startTime.Add(time.Duration(duration+ctx.settings.ServiceBufferMinutes) * time.Minute)
	conflicts, err := s.loadReservationConflicts(ctx, startTime, conflictEnd, excludeReservationID)
	if err != nil {
		return nil, err
	}
	// Write-path / table-options capacity stays reservation-conflict based.
	// Live floor occupancy is applied in buildAvailabilitySlots (guest counts)
	// and filterAutoAssignmentCandidates / write guards (assignment).
	return s.evaluateSlotCapacityFromConflicts(
		ctx, conflicts, reservationTime, duration, partySize, nil, false,
	), nil
}

func (s *ReservationService) prefetchReservationConflicts(ctx *reservationAvailabilityContext, earliestStart, latestEnd time.Time) error {
	conflicts, err := s.loadReservationConflicts(ctx, earliestStart, latestEnd, 0)
	if err != nil {
		return err
	}
	ctx.conflictCache = conflicts
	ctx.conflictCacheFrom = earliestStart.UTC()
	ctx.conflictCacheTo = latestEnd.UTC()
	return nil
}

func (s *ReservationService) reservationConflictsFromCache(
	ctx *reservationAvailabilityContext,
	earliestStart, latestEnd time.Time,
	excludeReservationID uint,
) *reservationConflictSet {
	if ctx.conflictCache == nil || ctx.conflictCacheFrom.IsZero() {
		return nil
	}
	if earliestStart.UTC().Before(ctx.conflictCacheFrom) || latestEnd.UTC().After(ctx.conflictCacheTo) {
		return nil
	}
	return filterReservationConflicts(ctx.conflictCache, earliestStart, latestEnd, excludeReservationID, ctx.settings.ServiceBufferMinutes)
}

func filterReservationConflicts(
	src *reservationConflictSet,
	earliestStart, latestEnd time.Time,
	excludeReservationID uint,
	bufferMinutes int,
) *reservationConflictSet {
	earliestUTC := earliestStart.UTC()
	latestUTC := latestEnd.UTC()
	out := &reservationConflictSet{
		all:     make([]database.TableReservation, 0, len(src.all)),
		byTable: make(map[uint][]database.TableReservation),
	}
	for _, reservation := range src.all {
		if excludeReservationID > 0 && reservation.ID == excludeReservationID {
			continue
		}
		if !reservationOverlapsWindow(reservation, earliestUTC, latestUTC, bufferMinutes) {
			continue
		}
		out.all = append(out.all, reservation)
		if reservation.TableID != nil {
			out.byTable[*reservation.TableID] = append(out.byTable[*reservation.TableID], reservation)
		}
	}
	return out
}

func (s *ReservationService) loadReservationConflicts(ctx *reservationAvailabilityContext, earliestStart, latestEnd time.Time, excludeReservationID uint) (*reservationConflictSet, error) {
	if cached := s.reservationConflictsFromCache(ctx, earliestStart, latestEnd, excludeReservationID); cached != nil {
		return cached, nil
	}
	return s.loadReservationConflictsTx(s.db, ctx, earliestStart, latestEnd, excludeReservationID)
}

// loadReservationConflictsTx reads overlapping active reservations through tx
// and never consults ctx.conflictCache. The cached path is for availability
// reads; a capacity recheck inside a write transaction must see rows committed
// after that cache was filled.
func (s *ReservationService) loadReservationConflictsTx(tx *gorm.DB, ctx *reservationAvailabilityContext, earliestStart, latestEnd time.Time, excludeReservationID uint) (*reservationConflictSet, error) {
	var reservations []database.TableReservation
	earliestUTC := earliestStart.UTC()
	latestUTC := latestEnd.UTC()
	lowerBoundUTC := reservationConflictLowerBound(earliestStart, ctx.settings.ServiceBufferMinutes).UTC()
	query := tx.Select(reservationConflictSelectColumns).Where(
		"business_id = ? AND status IN ? AND reservation_time < ? AND reservation_time >= ?",
		ctx.business.ID,
		[]string{"pending", "confirmed", "seated"},
		latestUTC,
		lowerBoundUTC,
	)
	if excludeReservationID > 0 {
		query = query.Where("id != ?", excludeReservationID)
	}
	if err := query.Order("reservation_time ASC").Find(&reservations).Error; err != nil {
		return nil, err
	}

	conflicts := &reservationConflictSet{
		all:     make([]database.TableReservation, 0, len(reservations)),
		byTable: make(map[uint][]database.TableReservation),
	}
	for _, reservation := range reservations {
		if !reservationOverlapsWindow(reservation, earliestUTC, latestUTC, ctx.settings.ServiceBufferMinutes) {
			continue
		}
		conflicts.all = append(conflicts.all, reservation)
		if reservation.TableID != nil {
			conflicts.byTable[*reservation.TableID] = append(conflicts.byTable[*reservation.TableID], reservation)
		}
	}
	return conflicts, nil
}

// recheckSlotCapacityTx is the in-transaction capacity gate. Covers apply
// whether or not a table is pinned. An unassigned booking also needs a free
// table after unassigned holds, because nothing else locks that inventory.
func (s *ReservationService) recheckSlotCapacityTx(
	tx *gorm.DB,
	ctx *reservationAvailabilityContext,
	reservationTime time.Time,
	duration, partySize int,
	assignedTableID *uint,
	excludeReservationID uint,
) error {
	startTime := reservationTime.In(ctx.location)
	conflictEnd := startTime.Add(time.Duration(duration+ctx.settings.ServiceBufferMinutes) * time.Minute)
	conflicts, err := s.loadReservationConflictsTx(tx, ctx, startTime, conflictEnd, excludeReservationID)
	if err != nil {
		return err
	}
	if ctx.settings.MaxCoversPerSlot > 0 {
		covers := 0
		coverStart := startTime.UTC()
		coverEnd := coverStart.Add(time.Duration(duration) * time.Minute)
		for _, reservation := range conflicts.all {
			if reservationOverlapsWindow(reservation, coverStart, coverEnd, 0) {
				covers += reservation.PartySize
			}
		}
		if covers+partySize > ctx.settings.MaxCoversPerSlot {
			return database.ErrReservationSlotUnavailable
		}
	}
	if assignedTableID == nil {
		capacity := s.evaluateSlotCapacityFromConflicts(
			ctx, conflicts, reservationTime, duration, partySize, nil, false,
		)
		if len(capacity.availableTables) == 0 {
			return database.ErrReservationSlotUnavailable
		}
	}
	return nil
}

func reservationStatusHoldsCapacity(status string) bool {
	switch status {
	case "pending", "confirmed", "seated":
		return true
	default:
		return false
	}
}

func (s *ReservationService) evaluateSlotCapacityFromConflicts(
	ctx *reservationAvailabilityContext,
	conflicts *reservationConflictSet,
	reservationTime time.Time,
	duration, partySize int,
	occupancy map[uint]database.ReservationTableOccupancy,
	applyOccupancy bool,
) *slotCapacityResult {
	startTime := reservationTime.In(ctx.location).UTC()
	endTime := startTime.Add(time.Duration(duration) * time.Minute)
	if ctx.settings.MaxCoversPerSlot > 0 {
		covers := 0
		for _, reservation := range conflicts.all {
			if reservationOverlapsWindow(reservation, startTime, endTime, 0) {
				covers += reservation.PartySize
			}
		}
		if covers+partySize > ctx.settings.MaxCoversPerSlot {
			return &slotCapacityResult{availableTables: []database.Table{}, reasonCode: "covers_limit"}
		}
	}

	tableEndTime := startTime.Add(time.Duration(duration+ctx.settings.ServiceBufferMinutes) * time.Minute)
	freePool := make([]database.Table, 0, len(ctx.tables))
	for _, table := range ctx.tables {
		if !table.IsActive {
			continue
		}
		if applyOccupancy {
			if state, ok := occupancy[table.ID]; ok && state.State != database.TableOccupancyAvailable {
				continue
			}
		}
		tableConflicts := conflicts.byTable[table.ID]
		tableAvailable := true
		for _, reservation := range tableConflicts {
			if reservationOverlapsWindow(reservation, startTime, tableEndTime, ctx.settings.ServiceBufferMinutes) {
				tableAvailable = false
				break
			}
		}
		if !tableAvailable {
			continue
		}
		freePool = append(freePool, table)
	}

	// Unassigned overlapping bookings still consume covers of inventory even
	// without a pinned table — greedily hold the smallest fitting free tables
	// so guest "tables ready" never overpromises the room.
	unassigned := make([]database.TableReservation, 0)
	for _, reservation := range conflicts.all {
		if reservation.TableID != nil {
			continue
		}
		if reservationOverlapsWindow(reservation, startTime, tableEndTime, ctx.settings.ServiceBufferMinutes) {
			unassigned = append(unassigned, reservation)
		}
	}
	sort.SliceStable(unassigned, func(i, j int) bool {
		if unassigned[i].PartySize == unassigned[j].PartySize {
			return unassigned[i].ID < unassigned[j].ID
		}
		return unassigned[i].PartySize > unassigned[j].PartySize
	})
	for _, reservation := range unassigned {
		heldIdx := -1
		for i, table := range freePool {
			if tableSeatCapacity(table) >= reservation.PartySize {
				heldIdx = i
				break
			}
		}
		if heldIdx < 0 {
			return &slotCapacityResult{availableTables: []database.Table{}, reasonCode: "table_unavailable"}
		}
		freePool = append(freePool[:heldIdx], freePool[heldIdx+1:]...)
	}

	available := make([]database.Table, 0, len(freePool))
	for _, table := range freePool {
		if tableSeatCapacity(table) >= partySize {
			available = append(available, table)
		}
	}
	if len(available) == 0 {
		reason := "table_unavailable"
		if applyOccupancy {
			reason = "table_unavailable"
		}
		return &slotCapacityResult{availableTables: []database.Table{}, reasonCode: reason}
	}
	return &slotCapacityResult{availableTables: available, reasonCode: "available"}
}

func reservationConflictLowerBound(earliestStart time.Time, serviceBufferMinutes int) time.Time {
	return database.ReservationConflictLowerBound(earliestStart, serviceBufferMinutes)
}

// validateReservationDuration enforces the duration range conflict reads rely
// on (see database.MaxReservationDurationMinutes).
func validateReservationDuration(duration int) error {
	if duration <= 0 {
		return fmt.Errorf("duration must be greater than zero")
	}
	if duration > database.MaxReservationDurationMinutes {
		return fmt.Errorf("duration must be at most %d minutes", database.MaxReservationDurationMinutes)
	}
	return nil
}

func reservationOverlapsWindow(reservation database.TableReservation, startTime, endTime time.Time, bufferMinutes int) bool {
	reservationStart := reservation.ReservationTime.UTC()
	reservationEnd := reservationStart.Add(time.Duration(reservation.Duration+bufferMinutes) * time.Minute)
	return reservationStart.Before(endTime.UTC()) && reservationEnd.After(startTime.UTC())
}

func (s *ReservationService) preflightBillForReservation(reservation *database.TableReservation) error {
	if reservation.TableID == nil {
		return fmt.Errorf("reservation must have an assigned table before seating")
	}
	_, _, err := database.GetOpenBillByTableID(*reservation.TableID)
	if err != nil && !errors.Is(err, database.ErrNoActiveBill) {
		return fmt.Errorf("failed to check active bill for reservation table: %w", err)
	}
	// Refuse before writing seated. Walk-in Liberar/seat already block leftover
	// kitchen/pending; reservation check-in must not CreateBill over the same
	// 1132/T5 leftover (#704 ticket 1123).
	if err := database.RefuseUnfinishedTableServiceTx(database.GetDB(), *reservation.TableID); err != nil {
		return err
	}
	return nil
}

type operatingPeriod struct {
	start      time.Time
	end        time.Time // door / bar close
	serviceEnd time.Time // kitchen close when set; otherwise door close
}

// getOperatingPeriods returns each open service window for the calendar day.
// Split lunch/dinner periods stay separate so the afternoon gap is not sold.
// Holiday exceptions override the weekly grid for that date.
func (s *ReservationService) getOperatingPeriods(ctx *reservationAvailabilityContext, date time.Time) ([]operatingPeriod, bool, error) {
	loc := ctx.location
	localDate := date.In(loc)
	dayStart := time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 0, 0, 0, 0, loc)
	// Exception rows are stored as calendar dates (UTC midnight of that civil day),
	// not as venue-local instants — do not shift via dayStart.UTC().
	civilKey := localDate.Format("2006-01-02")

	var exception *database.BusinessOperatingException
	if ctx.exceptions != nil {
		exception = ctx.exceptions[civilKey]
	} else {
		civilDay := time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 0, 0, 0, 0, time.UTC)
		row, err := database.GetBusinessOperatingExceptionForDate(ctx.business.ID, civilDay)
		if err == nil {
			exception = row
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, err
		}
	}
	if exception != nil {
		if exception.IsClosed {
			return nil, true, nil
		}
		openClock := ""
		closeClock := ""
		if exception.OpenTime != nil {
			openClock = *exception.OpenTime
		}
		if exception.CloseTime != nil {
			closeClock = *exception.CloseTime
		}
		period, err := buildOperatingPeriod(dayStart, openClock, closeClock, exception.KitchenCloseTime, loc)
		if err != nil {
			return nil, false, err
		}
		return []operatingPeriod{period}, false, nil
	}

	var rows []database.BusinessOperatingHours
	if ctx.operatingHours != nil {
		rows = ctx.operatingHours[int(localDate.Weekday())]
	} else {
		var err error
		rows, err = database.GetBusinessOperatingHoursByDay(ctx.business.ID, int(localDate.Weekday()))
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				rows = nil
			} else {
				return nil, false, err
			}
		}
	}
	if len(rows) == 0 {
		// No business_operating_hours row for this weekday means closed.
		// Matches delivery: a day the venue never configured is not bookable.
		return nil, true, nil
	}

	periods := make([]operatingPeriod, 0, len(rows))
	for _, hours := range rows {
		if hours.IsClosed {
			return nil, true, nil
		}
		period, err := buildOperatingPeriod(dayStart, hours.OpenTime, hours.CloseTime, hours.KitchenCloseTime, loc)
		if err != nil {
			return nil, false, err
		}
		periods = append(periods, period)
	}
	if len(periods) == 0 {
		return nil, true, nil
	}
	sort.SliceStable(periods, func(i, j int) bool {
		return periods[i].start.Before(periods[j].start)
	})
	return periods, false, nil
}

func buildOperatingPeriod(dayStart time.Time, openClock, closeClock string, kitchenClose *string, loc *time.Location) (operatingPeriod, error) {
	openTime, err := parseClockTime(dayStart, openClock, loc)
	if err != nil {
		return operatingPeriod{}, err
	}
	closeTime, err := parseClockTime(dayStart, closeClock, loc)
	if err != nil {
		return operatingPeriod{}, err
	}
	if !closeTime.After(openTime) {
		closeTime = closeTime.Add(24 * time.Hour)
	}
	serviceEnd := closeTime
	if kitchenClose != nil && strings.TrimSpace(*kitchenClose) != "" {
		kitchenEnd, err := parseClockTime(dayStart, strings.TrimSpace(*kitchenClose), loc)
		if err != nil {
			return operatingPeriod{}, err
		}
		if !kitchenEnd.After(openTime) {
			kitchenEnd = kitchenEnd.Add(24 * time.Hour)
		}
		// Kitchen close may be earlier than door close; never later than door.
		if kitchenEnd.Before(closeTime) {
			serviceEnd = kitchenEnd
		}
	}
	return operatingPeriod{start: openTime, end: closeTime, serviceEnd: serviceEnd}, nil
}

func (s *ReservationService) ensureBillForReservation(reservation *database.TableReservation) error {
	if reservation.TableID == nil {
		return fmt.Errorf("reservation must have an assigned table before seating")
	}
	// Active bill lookup treats open and partially paid bills as already seated.
	existingBill, _, err := database.GetOpenBillByTableID(*reservation.TableID)
	if err == nil && existingBill != nil {
		return nil
	}
	if err != nil && !errors.Is(err, database.ErrNoActiveBill) {
		return fmt.Errorf("failed to check active bill for reservation table: %w", err)
	}
	business, err := database.GetBusinessByID(reservation.BusinessID)
	if err != nil {
		return err
	}
	bill := &database.Bill{
		BusinessID:     reservation.BusinessID,
		TableID:        *reservation.TableID,
		Notes:          fmt.Sprintf("Reservation bill for %s (%d guests)", reservation.CustomerName, reservation.PartySize),
		Status:         database.BillStatusOpen,
		SettlementAddr: business.SettlementAddr,
		TippingAddr:    business.TippingAddr,
	}
	// Same leftover-service door as SeatWalkIn. Refuse + CreateBill share one
	// transaction so a host cannot sit a new party on T5 while 1123 is in the pass.
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := database.RefuseUnfinishedTableServiceTx(tx, *reservation.TableID); err != nil {
			return err
		}
		return database.CreateBillTx(tx, bill, []database.BillItem{})
	})
}

func loadBusinessLocation(timezone string) (*time.Location, error) {
	if strings.TrimSpace(timezone) == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("invalid business timezone")
	}
	return loc, nil
}

// ParseReservationDateTime stores venue-local wall times in UTC.
// RFC3339 instants keep their offset. Naive "2026-08-19T19:00" is 7pm in loc
// (America/New_York → 23:00Z EDT / 00:00Z EST), never 19:00Z.
func ParseReservationDateTime(raw string, loc *time.Location) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("invalid reservation_time")
	}
	if loc == nil {
		loc = time.UTC
	}
	if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return parsed, nil
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed, nil
	}
	for _, layout := range []string{
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
	} {
		if parsed, err := time.ParseInLocation(layout, raw, loc); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid reservation_time")
}

// tableSeatCapacity is the host-visible seat count used for assignment.
// Spaces layout may pin MaxCapacity / VisibleSeatCount below Table.Capacity.
func tableSeatCapacity(table database.Table) int {
	cap := table.Capacity
	tighten := func(value *int) {
		if value == nil || *value <= 0 {
			return
		}
		if cap <= 0 || *value < cap {
			cap = *value
		}
	}
	tighten(table.MaxCapacity)
	tighten(table.VisibleSeatCount)
	if cap < 0 {
		return 0
	}
	return cap
}

func firstTableFittingParty(tables []database.Table, partySize int) *database.Table {
	for i := range tables {
		if tableSeatCapacity(tables[i]) >= partySize {
			return &tables[i]
		}
	}
	return nil
}

func tableFitsParty(tables []database.Table, tableID uint, partySize int) bool {
	for i := range tables {
		if tables[i].ID == tableID {
			return tableSeatCapacity(tables[i]) >= partySize
		}
	}
	return false
}

func parseClockTime(date time.Time, raw string, loc *time.Location) (time.Time, error) {
	parts := strings.Split(strings.TrimSpace(raw), ":")
	if len(parts) != 2 {
		return time.Time{}, fmt.Errorf("invalid operating hours")
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid operating hours")
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid operating hours")
	}
	return time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, loc), nil
}

// reservationCodeGenerator is a seam for tests to force collisions.
var reservationCodeGenerator = generateReservationCode

// generateReservationCode returns a 12-char uppercase hex code. The code is
// the only credential a guest needs to view/cancel a reservation, so
// rand failures are an error rather than a predictable timestamp fallback.
func generateReservationCode() (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate confirmation code: %w", err)
	}
	return strings.ToUpper(hex.EncodeToString(buf)), nil
}

// uniqueReservationCode generates a confirmation code that is not already in
// use. Codes are looked up globally (guests open them without business
// context), so collisions across businesses must be retried. The unique index
// added by the genesis schema backstops the race between check and insert.
func uniqueReservationCode() (string, error) {
	for attempt := 0; attempt < 5; attempt++ {
		code, err := reservationCodeGenerator()
		if err != nil {
			return "", err
		}
		code = database.NormalizeConfirmationCode(code)
		if code == "" {
			return "", fmt.Errorf("failed to generate confirmation code: empty")
		}
		exists, err := database.ReservationConfirmationCodeExists(code)
		if err != nil {
			return "", err
		}
		if !exists {
			return code, nil
		}
	}
	return "", fmt.Errorf("could not allocate a unique confirmation code")
}

func normalizeReservationTransition(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "assign_table":
		return "assign_table"
	case "check_in", "seat", "seated":
		return "check_in"
	case "complete", "completed":
		return "complete"
	case "cancel", "cancelled":
		return "cancel"
	case "no_show":
		return "no_show"
	case "promote_waitlist", "promote":
		return "promote_waitlist"
	case "confirm", "confirmed":
		return "confirm"
	case "pending", "waitlist":
		return strings.TrimSpace(strings.ToLower(raw))
	default:
		return ""
	}
}

// Stable history-note tokens (L1-21). Persisted in the existing text column so
// the UI can map them to locale strings without a schema migration. Free-text
// notes pass through unchanged.
const (
	reservationNoteTokenPrefix            = "token:"
	reservationNoteAssignedTable          = "token:assigned_table"
	reservationNotePromotedWaitlist       = "token:promoted_waitlist"
	reservationNoteAddedWaitlist          = "token:added_waitlist"
	reservationNoteCreatedWithTable       = "token:created_with_table"
	reservationNoteCreated                = "token:created"
	reservationNoteCancelledByCustomer    = "token:cancelled_by_customer"
	reservationNoteAutoDeclinedNoResponse = "token:auto_declined_no_response"
	reservationNoteCancelledByBusiness    = "token:cancelled_by_business"
)

func buildReservationTransitionNote(oldStatus string, reservation *database.TableReservation, transition string, input ReservationTransitionInput) string {
	if reason := strings.TrimSpace(input.Reason); reason != "" {
		if reason == database.ReservationReasonCancelledByBusiness {
			return reservationNoteCancelledByBusiness
		}
		return reason
	}
	if strings.TrimSpace(input.Notes) != "" {
		return strings.TrimSpace(input.Notes)
	}
	switch transition {
	case "assign_table":
		if reservation.Table != nil {
			return reservationNoteTokenPrefix + "assigned_to:" + reservation.Table.Name
		}
		return reservationNoteAssignedTable
	case "promote_waitlist":
		return reservationNotePromotedWaitlist
	default:
		return fmt.Sprintf("%sstatus_changed:%s:%s", reservationNoteTokenPrefix, oldStatus, reservation.Status)
	}
}

func initialReservationHistoryNote(reservation *database.TableReservation) string {
	if reservation.Status == "waitlist" {
		return reservationNoteAddedWaitlist
	}
	if reservation.TableID != nil {
		return reservationNoteCreatedWithTable
	}
	return reservationNoteCreated
}

func humanizeReservationReason(reasonCode string) string {
	switch reasonCode {
	case "covers_limit":
		return "slot capacity has been reached"
	case "table_unavailable":
		return "no tables are available for the requested time"
	case "outside_advance_window":
		return "selected time is too soon to book"
	case "outside_operating_window":
		return "selected time is too close to closing for a full reservation"
	default:
		return "requested slot is unavailable"
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
