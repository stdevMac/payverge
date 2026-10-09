package services

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/services/operational_alerts"

	"gorm.io/gorm"
)

// ReservationNoShowSweeper ages past-due Confirmed bookings to no_show after
// each business's no_show_grace_minutes so NEXT ARRIVAL / table status / the
// operator chip share one clock.
type ReservationNoShowSweeper struct{}

// NewReservationNoShowSweeper creates a no-show aging sweeper.
func NewReservationNoShowSweeper() *ReservationNoShowSweeper {
	return &ReservationNoShowSweeper{}
}

// ProcessExpiredConfirmedArrivals marks confirmed reservations whose no-show
// grace has elapsed. Safe to overlap with itself and with operator actions:
// each row is claimed via a status-guarded UPDATE before side effects.
func (s *ReservationNoShowSweeper) ProcessExpiredConfirmedArrivals() (int, error) {
	db := database.GetDB()
	now := time.Now().UTC()

	var candidates []database.TableReservation
	if err := expiredConfirmedArrivalQuery(db, now).Find(&candidates).Error; err != nil {
		return 0, fmt.Errorf("load expired confirmed arrivals: %w", err)
	}

	graceByBusiness := map[uint]int{}
	aged := 0
	for i := range candidates {
		res := &candidates[i]
		grace, ok := graceByBusiness[res.BusinessID]
		if !ok {
			grace = DefaultNoShowGraceMinutes
			if settings, err := database.GetReservationSettingsForRead(res.BusinessID); err == nil && settings != nil {
				grace = NormalizeNoShowGraceMinutes(settings.NoShowGraceMinutes)
			}
			graceByBusiness[res.BusinessID] = grace
		}
		if !ShouldAutoNoShow(res.ReservationTime, res.Status, now, grace) {
			continue
		}
		if tableHasOpenCheck(res) {
			continue
		}

		claim := db.Model(&database.TableReservation{}).
			Where("id = ? AND status = ?", res.ID, "confirmed").
			Updates(map[string]any{
				"status":              "no_show",
				"cancelled_at":        now,
				"cancelled_by":        "system",
				"cancellation_reason": database.ReservationReasonNoShowTimeout,
				"updated_at":          now,
			})
		if claim.Error != nil {
			log.Printf("no-show sweeper: claim failed for reservation %d: %v", res.ID, claim.Error)
			continue
		}
		if claim.RowsAffected != 1 {
			continue
		}
		aged++

		if err := database.CreateReservationStatusHistory(&database.ReservationStatusHistory{
			ReservationID: res.ID,
			Status:        "no_show",
			TableID:       res.TableID,
			Notes:         "token:auto_no_show_grace",
			ChangedBy:     "system",
		}); err != nil {
			log.Printf("no-show sweeper: failed to write status history for reservation %d: %v", res.ID, err)
		}

		if err := operational_alerts.NewService(db).ResolveAlertForResource(
			context.Background(),
			res.BusinessID,
			database.OperationalAlertResourceTypeReservation,
			res.ID,
			operational_alerts.Actor{Name: "system"},
			"no_show",
		); err != nil {
			log.Printf("no-show sweeper: failed to resolve operational alert for reservation %d: %v", res.ID, err)
		}

		events.GetHub().PublishJSON(res.BusinessID, "reservation.updated", map[string]any{
			"id":                  res.ID,
			"business_id":         res.BusinessID,
			"status":              "no_show",
			"cancellation_reason": database.ReservationReasonNoShowTimeout,
		})

		s.notifyNoShow(res, now, grace)
	}

	if aged > 0 {
		log.Printf("no-show sweeper: aged %d confirmed reservation(s) to no_show", aged)
	}
	return aged, nil
}

// SQL prefilter: confirmed rows whose start is already in the past. The exact
// per-business grace is re-checked in Go so SQLite and Postgres share one path.
// tableHasOpenCheck is true when the reserved table already has an open bill.
// A seated-late party that the host never marked arrived must not be flipped
// to no_show (and must not free the table) while they are still on a check.
// Lookup errors fail closed so a transient DB blip cannot age a live cover.
func tableHasOpenCheck(res *database.TableReservation) bool {
	if res == nil || res.TableID == nil || *res.TableID == 0 {
		return false
	}
	bills, err := database.GetActiveBillsByTableID(*res.TableID)
	if err != nil {
		log.Printf("no-show sweeper: open-check lookup failed for reservation %d table %d: %v", res.ID, *res.TableID, err)
		return true
	}
	return len(bills) > 0
}

func expiredConfirmedArrivalQuery(db *gorm.DB, now time.Time) *gorm.DB {
	return db.
		Model(&database.TableReservation{}).
		Where("status = ?", "confirmed").
		Where("reservation_time <= ?", now).
		Order("reservation_time ASC").
		Limit(500)
}

// shouldEmailAutoNoShow emails only when the guest left an address and the
// slot is recent. Stale demo / legacy rows are still aged, but a hours-late
// "you were a no-show" inbox is worse than silence.
func shouldEmailAutoNoShow(res *database.TableReservation, now time.Time, graceMinutes int) bool {
	if res == nil || res.CustomerEmail == "" {
		return false
	}
	cutoff := NoShowCutoff(res.ReservationTime, graceMinutes)
	return !now.After(cutoff.Add(2 * time.Hour))
}

func (s *ReservationNoShowSweeper) notifyNoShow(res *database.TableReservation, now time.Time, graceMinutes int) {
	if !shouldEmailAutoNoShow(res, now, graceMinutes) {
		return
	}
	if emails.EmailServerInstance == nil {
		return
	}

	business, err := database.GetBusinessByID(res.BusinessID)
	if err != nil {
		log.Printf("no-show sweeper: failed to load business %d for email: %v", res.BusinessID, err)
		return
	}

	language := ResolveReservationEmailLanguage(res, business)
	reservationDate, reservationTime := FormatReservationEmailDateTime(res.ReservationTime, business.Timezone, language)

	if err := emails.EmailServerInstance.ForReservationFollowUp(res.BusinessID, res.ID, res.CreatedBy).SendReservationNoShowEmail(
		[]string{res.CustomerEmail},
		res.CustomerName,
		business.Name,
		reservationDate,
		reservationTime,
		business.Phone,
		language,
	); err != nil {
		log.Printf("no-show sweeper: failed to send email for reservation %d: %v", res.ID, err)
	}
}
