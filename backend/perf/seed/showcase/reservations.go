package main

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// seedReservations installs reservation settings + 12 reservations (past +
// upcoming) so the Reservation Manager view is meaningful in screenshots.
// Idempotent on confirmation_code.
func seedReservations(ctx context.Context, db *gorm.DB, bizID uint) error {
	if err := seedReservationSettings(ctx, db, bizID); err != nil {
		return err
	}

	tables, err := loadTables(ctx, db, bizID)
	if err != nil {
		return err
	}
	if len(tables) == 0 {
		return fmt.Errorf("no tables to attach reservations to")
	}

	now := time.Now().UTC()

	type resSpec struct {
		Code       string
		Name       string
		Phone      string
		Email      string
		Party      int
		At         time.Time
		Status     string
		Notes      string
		Special    string
		TableIndex int // index into showcase tables; -1 = unassigned
	}

	specs := []resSpec{
		// Upcoming — tonight and tomorrow
		{"BV-RES-001", "Jennifer Park", "+1 (415) 555-0101", "jpark@example.com", 2, now.Add(2 * time.Hour), "confirmed", "Anniversary — quiet table requested", "anniversary, wine pairing", 8},
		{"BV-RES-002", "David Chen", "+1 (415) 555-0102", "dchen@example.com", 4, now.Add(3 * time.Hour), "confirmed", "Regular guest, prefers booth", "", 14},
		{"BV-RES-003", "Sarah Johnson", "+1 (415) 555-0103", "sarah.j@example.com", 6, now.Add(5 * time.Hour), "confirmed", "Birthday — bringing cake", "birthday, allergies: nuts", 12},
		{"BV-RES-004", "Robert Martinez", "+1 (415) 555-0104", "rmartinez@example.com", 2, now.Add(24 * time.Hour), "pending", "First visit", "", -1},
		{"BV-RES-005", "Aisha Patel", "+1 (415) 555-0105", "aisha.p@example.com", 8, now.Add(28 * time.Hour), "confirmed", "Corporate dinner", "vegetarian options needed", 16},
		{"BV-RES-006", "Thomas O'Brien", "+1 (415) 555-0106", "tobrien@example.com", 3, now.Add(48 * time.Hour), "confirmed", "", "", 5},
		{"BV-RES-007", "Maria Rodriguez", "+1 (415) 555-0107", "mrodriguez@example.com", 12, now.Add(72 * time.Hour), "pending", "Engagement dinner", "private dining preferred", 18},
		{"BV-RES-008", "Kevin Wu", "+1 (415) 555-0108", "kwu@example.com", 2, now.Add(96 * time.Hour), "confirmed", "Window seat if possible", "", 7},

		// Recent past
		{"BV-RES-101", "Lisa Brown", "+1 (415) 555-0201", "lbrown@example.com", 4, now.Add(-24 * time.Hour), "completed", "", "", 9},
		{"BV-RES-102", "Michael Smith", "+1 (415) 555-0202", "msmith@example.com", 2, now.Add(-48 * time.Hour), "completed", "", "", 0},
		{"BV-RES-103", "Daniel Kim", "+1 (415) 555-0203", "dkim@example.com", 6, now.Add(-72 * time.Hour), "no_show", "Called 45min late, no show", "", 4},
		{"BV-RES-104", "Olivia White", "+1 (415) 555-0204", "owhite@example.com", 3, now.Add(-96 * time.Hour), "cancelled", "Cancelled morning of", "", 1},
	}

	rows := make([]database.TableReservation, 0, len(specs))
	for _, s := range specs {
		var tableID *uint
		if s.TableIndex >= 0 && s.TableIndex < len(tables) {
			id := tables[s.TableIndex].ID
			tableID = &id
		}
		var confirmedAt *time.Time
		if s.Status == "confirmed" || s.Status == "completed" {
			t := s.At.Add(-24 * time.Hour)
			confirmedAt = &t
		}
		rows = append(rows, database.TableReservation{
			BusinessID:       bizID,
			TableID:          tableID,
			CustomerName:     s.Name,
			CustomerPhone:    s.Phone,
			CustomerEmail:    s.Email,
			PartySize:        s.Party,
			ReservationTime:  s.At,
			Duration:         120,
			Status:           s.Status,
			Source:           "customer",
			ConfirmationCode: s.Code,
			SpecialRequests:  s.Special,
			Notes:            s.Notes,
			CreatedBy:        "customer",
			ConfirmedAt:      confirmedAt,
		})
	}

	// confirmation_code is indexed but NOT unique on the model — we can't
	// use ON CONFLICT here. Filter rows whose codes already exist instead.
	codes := make([]string, 0, len(rows))
	for _, r := range rows {
		codes = append(codes, r.ConfirmationCode)
	}
	var existingCodes []string
	if err := db.WithContext(ctx).
		Model(&database.TableReservation{}).
		Where("business_id = ? AND confirmation_code IN ?", bizID, codes).
		Pluck("confirmation_code", &existingCodes).Error; err != nil {
		return fmt.Errorf("query existing reservations: %w", err)
	}
	existing := make(map[string]struct{}, len(existingCodes))
	for _, c := range existingCodes {
		existing[c] = struct{}{}
	}
	fresh := make([]database.TableReservation, 0, len(rows))
	for _, r := range rows {
		if _, dup := existing[r.ConfirmationCode]; dup {
			continue
		}
		fresh = append(fresh, r)
	}
	if len(fresh) == 0 {
		return nil
	}
	if err := db.WithContext(ctx).
		CreateInBatches(fresh, 50).Error; err != nil {
		return fmt.Errorf("create reservations: %w", err)
	}
	return nil
}

// seedReservationSettings ensures one ReservationSettings row exists per
// business with sensible defaults for a casual dinner-service restaurant.
func seedReservationSettings(ctx context.Context, db *gorm.DB, bizID uint) error {
	var existing database.ReservationSettings
	err := db.WithContext(ctx).
		Where("business_id = ?", bizID).
		First(&existing).Error
	if err == nil {
		return nil
	}
	if err != gorm.ErrRecordNotFound {
		return fmt.Errorf("query reservation settings: %w", err)
	}
	settings := database.ReservationSettings{
		BusinessID:            bizID,
		Enabled:               true,
		MaxAdvanceDays:        60,
		MinAdvanceMinutes:     60,
		MinPartySize:          1,
		MaxPartySize:          18,
		DefaultDuration:       120,
		SlotIntervalMinutes:   30,
		ServiceBufferMinutes:  15,
		MaxCoversPerSlot:      30,
		AutoAssignTables:      true,
		ApprovalMode:          database.ReservationApprovalAuto,
		AllowWaitlist:         true,
		HoldDurationMinutes:   15,
		AllowCancellation:     true,
		CancellationDeadline:  24,
		NoShowGraceMinutes:    20,
		SendConfirmationEmail: true,
		SendReminderEmail:     true,
		ReminderHoursBefore:   24,
		ExternalPartnerLinks:  database.JSONRawMessage([]byte(`[]`)),
	}
	if err := db.WithContext(ctx).Create(&settings).Error; err != nil {
		return fmt.Errorf("create reservation settings: %w", err)
	}
	return nil
}
