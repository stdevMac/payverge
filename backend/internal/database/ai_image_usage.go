package database

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AIImageUsage is the per-business generation counter for AI images. It is a
// fair-use backstop, not a quota: the daily window blocks runaway loops, and
// the monthly window only counts so an operator crossing the alert threshold
// can be noticed. Nothing here is customer-visible until the daily cap trips.
type AIImageUsage struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	BusinessID         uint       `gorm:"uniqueIndex;not null" json:"business_id"`
	DailyUsed          int        `gorm:"not null;default:0" json:"daily_used"`
	DailyPeriodStart   time.Time  `gorm:"not null" json:"daily_period_start"`
	MonthlyUsed        int        `gorm:"not null;default:0" json:"monthly_used"`
	MonthlyAnchorDay   int        `gorm:"not null;default:1" json:"monthly_anchor_day"` // 1..31
	MonthlyPeriodStart time.Time  `gorm:"not null" json:"monthly_period_start"`
	MonthlyAlertSentAt *time.Time `json:"monthly_alert_sent_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (AIImageUsage) TableName() string { return "ai_image_usage" }

// ImageDailyLimitError is returned when the daily backstop is hit. It carries
// everything the HTTP layer needs so the response is self-contained and the
// client never has to fetch a separate usage snapshot.
type ImageDailyLimitError struct {
	Limit    int
	ResetsAt time.Time
}

func (e ImageDailyLimitError) Error() string {
	return fmt.Sprintf("daily AI image limit of %d reached", e.Limit)
}

// ResetsInSeconds is a relative duration on purpose: the operator never sees a
// clock, so the window can stay UTC without ever looking wrong in another zone.
func (e ImageDailyLimitError) ResetsInSeconds() int {
	secs := int(time.Until(e.ResetsAt).Seconds())
	if secs < 0 {
		return 0
	}
	return secs
}

// ImageUsageReservation records a consumed generation so it can be refunded if
// the provider call fails. AlertTriggered is true exactly once per monthly
// period, on the call that crosses the threshold. MonthlyPeriodStart pins which
// period that claim belongs to, so a refund can only release its own latch.
type ImageUsageReservation struct {
	BusinessID         uint
	AlertTriggered     bool
	MonthlyUsed        int
	MonthlyPeriodStart time.Time
}

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func clampDayUTC(year int, month time.Month, day int) time.Time {
	if last := daysInMonth(year, month); day > last {
		day = last
	}
	if day < 1 {
		day = 1
	}
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// computeImageUsagePeriodStart returns the UTC-midnight start of the monthly
// window containing `now`, anchored to anchorDay (clamped per month).
func computeImageUsagePeriodStart(now time.Time, anchorDay int) time.Time {
	now = now.UTC()
	year, month := now.Year(), now.Month()
	candidate := clampDayUTC(year, month, anchorDay)
	if candidate.After(now) {
		if month == time.January {
			year, month = year-1, time.December
		} else {
			month--
		}
		candidate = clampDayUTC(year, month, anchorDay)
	}
	return candidate
}

func deriveAnchorDay(b *Business) int {
	switch {
	case b == nil:
		return 1
	case !b.CreatedAt.IsZero():
		return b.CreatedAt.UTC().Day()
	default:
		return 1
	}
}

func startOfUTCDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func nextUTCDay(t time.Time) time.Time {
	return startOfUTCDay(t).AddDate(0, 0, 1)
}

// EnsureImageUsageRow creates the per-business row on first use (idempotent).
// The insert is an atomic upsert, not a read-then-write: two concurrent first
// generations for the same business would both miss a SELECT and one would then
// trip the business_id unique index, surfacing a duplicate-key error on a brand
// new business's very first request. ON CONFLICT DO NOTHING makes the losing
// racer a no-op, and the unconditional re-read returns the stored row whichever
// call won — so a re-read failure is reported on its own terms rather than
// standing in for the insert's.
func EnsureImageUsageRow(b *Business) (*AIImageUsage, error) {
	if b == nil || b.ID == 0 {
		return nil, errors.New("business required")
	}
	now := time.Now().UTC()
	anchor := deriveAnchorDay(b)
	if err := GetDB().Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "business_id"}},
		DoNothing: true,
	}).Create(&AIImageUsage{
		BusinessID:         b.ID,
		DailyPeriodStart:   startOfUTCDay(now),
		MonthlyAnchorDay:   anchor,
		MonthlyPeriodStart: computeImageUsagePeriodStart(now, anchor),
	}).Error; err != nil {
		return nil, err
	}

	var row AIImageUsage
	if err := GetDB().Where("business_id = ?", b.ID).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// resetStaleWindows zeroes whichever counters have rolled over. The monthly
// reset also clears the alert latch so the next period can alert again.
// Period advance and latch null MUST stay in one Updates map: the refund
// release scopes on monthly_period_start equality under the assumption that a
// period mismatch means the latch was already cleared with that same write.
// See TestResetStaleWindows_MonthlyRollIsOneStatement and RefundImageGeneration.
func resetStaleWindows(tx *gorm.DB, businessID uint, anchorDay int) error {
	now := time.Now().UTC()

	dayStart := startOfUTCDay(now)
	if err := tx.Model(&AIImageUsage{}).
		Where("business_id = ? AND daily_period_start < ?", businessID, dayStart).
		Updates(map[string]interface{}{
			"daily_used":         0,
			"daily_period_start": dayStart,
		}).Error; err != nil {
		return err
	}

	monthStart := computeImageUsagePeriodStart(now, anchorDay)
	return tx.Model(&AIImageUsage{}).
		Where("business_id = ? AND monthly_period_start < ?", businessID, monthStart).
		Updates(map[string]interface{}{
			"monthly_used":          0,
			"monthly_period_start":  monthStart,
			"monthly_alert_sent_at": nil,
		}).Error
}

// ReserveImageGeneration rolls stale windows, then atomically consumes one
// generation against the daily cap. The conditional UPDATE is the whole
// concurrency story: no app-level locking, and a losing racer sees
// RowsAffected == 0 rather than an over-issued credit.
func ReserveImageGeneration(b *Business, dailyLimit, monthlyAlert int) (ImageUsageReservation, error) {
	row, err := EnsureImageUsageRow(b)
	if err != nil {
		return ImageUsageReservation{}, err
	}
	res := ImageUsageReservation{BusinessID: b.ID}

	err = GetDB().Transaction(func(tx *gorm.DB) error {
		if e := resetStaleWindows(tx, b.ID, row.MonthlyAnchorDay); e != nil {
			return e
		}

		upd := tx.Model(&AIImageUsage{}).
			Where("business_id = ? AND daily_used < ?", b.ID, dailyLimit).
			UpdateColumns(map[string]interface{}{
				"daily_used":   gorm.Expr("daily_used + 1"),
				"monthly_used": gorm.Expr("monthly_used + 1"),
			})
		if upd.Error != nil {
			return upd.Error
		}
		if upd.RowsAffected == 0 {
			return ImageDailyLimitError{Limit: dailyLimit, ResetsAt: nextUTCDay(time.Now())}
		}

		var fresh AIImageUsage
		if e := tx.Where("business_id = ?", b.ID).First(&fresh).Error; e != nil {
			return e
		}
		res.MonthlyUsed = fresh.MonthlyUsed
		res.MonthlyPeriodStart = fresh.MonthlyPeriodStart

		// Claim the alert exactly once per period. A concurrent crosser loses
		// the conditional UPDATE and reports AlertTriggered=false.
		if monthlyAlert > 0 && fresh.MonthlyUsed >= monthlyAlert {
			claim := tx.Model(&AIImageUsage{}).
				Where("business_id = ? AND monthly_alert_sent_at IS NULL", b.ID).
				UpdateColumn("monthly_alert_sent_at", time.Now().UTC())
			if claim.Error != nil {
				return claim.Error
			}
			res.AlertTriggered = claim.RowsAffected == 1
		}
		return nil
	})
	if err != nil {
		return ImageUsageReservation{}, err
	}
	return res, nil
}

// RefundImageGeneration reverses a reservation after a provider failure. The
// counter guards clamp at zero so a double refund is a no-op rather than a
// negative counter.
//
// The counter decrement is intentionally period-agnostic: it does not require
// daily_period_start / monthly_period_start to still match the reservation's
// windows. A refund's job is to return the slot. If the UTC day rolled while
// the provider was in flight, yesterday's daily bucket is already gone (reset
// to zero on the next reserve); crediting today's counter is the user-favorable
// reading — scoping the decrement to the old day would mean a generation that
// fails across midnight silently forfeits its slot. The monthly counter is ±1
// against a 2,000 internal alert threshold, so the same window-unaware ±1 is
// noise, not a correctness hazard. Two adversarial review rounds have flagged
// this as a bug; it is accepted gap O1, not an oversight.
//
// A reservation that claimed the monthly alert also releases it, so a later
// crossing can alert again — the generation it alerted on never happened.
// Refunds that did not claim the alert leave the latch alone: the period has
// genuinely alerted, and re-arming on every oscillation around the threshold
// would only add noise.
//
// The release is a separate UPDATE on purpose. Sharing the counter predicate
// would couple it to state it has nothing to do with: a slow provider spanning
// UTC midnight can leave a claiming reservation outstanding while the daily
// window rolls and a sibling refund drains the counters to zero, and the
// claimer's own refund would then find `daily_used > 0` false and skip the
// release — silently, since the statement still succeeds. The latch would stay
// set with nothing above the threshold, which is the exact silence the release
// exists to prevent. Both statements share a transaction so a refund never
// half-applies.
//
// It is scoped to the claiming period instead. A monthly roll zeroes the count
// and clears the latch, so a reservation outstanding from the previous period
// has nothing of its own left to release; without the scope its late refund
// would free whatever latch the *new* period had legitimately armed and let
// that period alert twice. monthly_period_start is safe to match by equality —
// it is always a UTC midnight from computeImageUsagePeriodStart, so it carries
// no sub-second component for a driver round trip to truncate.
//
// The period-scope safety rests on one more invariant: after row creation, the
// only writer of monthly_period_start is resetStaleWindows, which nulls
// monthly_alert_sent_at in the *same* UPDATE. A period mismatch at refund time
// therefore always means the latch was already cleared with the roll, and the
// scope can never suppress a release that was still needed. If those two writes
// are ever split, the scope becomes unsafe — pinned by
// TestResetStaleWindows_MonthlyRollIsOneStatement.
func RefundImageGeneration(r ImageUsageReservation) error {
	if r.BusinessID == 0 {
		return nil
	}
	return GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&AIImageUsage{}).
			Where("business_id = ? AND daily_used > 0 AND monthly_used > 0", r.BusinessID).
			UpdateColumns(map[string]interface{}{
				"daily_used":   gorm.Expr("daily_used - 1"),
				"monthly_used": gorm.Expr("monthly_used - 1"),
			}).Error; err != nil {
			return err
		}
		if !r.AlertTriggered {
			return nil
		}
		return tx.Model(&AIImageUsage{}).
			Where("business_id = ? AND monthly_period_start = ? AND monthly_alert_sent_at IS NOT NULL",
				r.BusinessID, r.MonthlyPeriodStart).
			UpdateColumn("monthly_alert_sent_at", nil).Error
	})
}
