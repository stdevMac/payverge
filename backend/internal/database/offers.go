package database

import (
	"strings"
	"sync"
	"time"

	"gorm.io/gorm/clause"
)

var (
	scheduleNowMu sync.RWMutex
	scheduleNow   = time.Now
)

// ScheduleNow is the clock used when guest menu/list/apply evaluate recurring
// offer windows. Tests pin it to a business-local instant.
func ScheduleNow() time.Time {
	scheduleNowMu.RLock()
	fn := scheduleNow
	scheduleNowMu.RUnlock()
	return fn()
}

// SetScheduleNow overrides ScheduleNow. Pass nil to restore time.Now.
func SetScheduleNow(fn func() time.Time) {
	scheduleNowMu.Lock()
	defer scheduleNowMu.Unlock()
	if fn == nil {
		scheduleNow = time.Now
		return
	}
	scheduleNow = fn
}

// ResetScheduleNow restores the real wall clock.
func ResetScheduleNow() { SetScheduleNow(nil) }

// FilterOffersActiveAt keeps offers whose weekday/minute window is active at
// `at` in the business timezone. Empty/invalid timezones fall back to UTC.
func FilterOffersActiveAt(offers []Offer, at time.Time, timezone string) []Offer {
	if len(offers) == 0 {
		return offers
	}
	ref := offerScheduleReference(at, timezone)
	out := make([]Offer, 0, len(offers))
	for _, offer := range offers {
		if OfferActiveAt(offer, ref) {
			out = append(out, offer)
		}
	}
	return out
}

// OfferActiveAt evaluates an offer's recurring schedule in business-local time.
// Weekday bits follow time.Weekday (Sunday bit 0 through Saturday bit 6). An
// overnight window belongs to the day on which it starts.
func OfferActiveAt(offer Offer, local time.Time) bool {
	mask := offer.WeekdayMask
	if mask == 0 {
		mask = 127
	}
	activeOn := func(day time.Weekday) bool {
		return mask&int16(1<<uint(day)) != 0
	}

	if offer.StartMinute == nil && offer.EndMinute == nil {
		return activeOn(local.Weekday())
	}
	if offer.StartMinute == nil || offer.EndMinute == nil {
		return false
	}

	minute := local.Hour()*60 + local.Minute()
	start, end := *offer.StartMinute, *offer.EndMinute
	weekday := local.Weekday()
	if start < end {
		return activeOn(weekday) && minute >= start && minute < end
	}
	if minute < end {
		weekday = time.Weekday((int(weekday) + 6) % 7)
	}
	return activeOn(weekday) && (minute >= start || minute < end)
}

// CreateOffer creates a new offer
func CreateOffer(offer *Offer) error {
	return db.Create(offer).Error
}

// GetOffersByBusinessID retrieves all offers for a business
func GetOffersByBusinessID(businessID uint) ([]Offer, error) {
	var offers []Offer
	err := db.Where("business_id = ?", businessID).Find(&offers).Error
	return offers, err
}

// offerScheduleReference converts an instant into the business-local calendar
// used by recurring offer schedules. Empty and invalid IANA timezone values use
// the platform-wide UTC fallback defined by ResolveLocation.
func offerScheduleReference(at time.Time, timezone string) time.Time {
	return at.In(ResolveLocation(strings.TrimSpace(timezone)))
}

// GetActiveOffersByBusinessIDAt retrieves active offers for a business at a
// specific instant. Recurring weekday/minute windows are evaluated in the
// supplied business timezone; empty or invalid timezones fall back to UTC.
func GetActiveOffersByBusinessIDAt(businessID uint, at time.Time, timezone string) ([]Offer, error) {
	var offers []Offer
	reference := offerScheduleReference(at, timezone)

	err := db.Where(
		"business_id = ? AND is_active = ? AND (start_date IS NULL OR start_date <= ?) AND (end_date IS NULL OR end_date >= ?)",
		businessID,
		true,
		reference,
		reference,
	).Find(&offers).Error
	if err != nil {
		return nil, err
	}

	active := make([]Offer, 0, len(offers))
	for _, offer := range offers {
		if OfferActiveAt(offer, reference) {
			active = append(active, offer)
		}
	}
	return active, nil
}

// GetOfferByID retrieves an offer by ID
func GetOfferByID(id uint) (*Offer, error) {
	var offer Offer
	err := db.First(&offer, id).Error
	if err != nil {
		return nil, err
	}
	return &offer, nil
}

// UpdateOffer updates an existing offer
func UpdateOffer(offer *Offer) error {
	return db.Omit(clause.Associations).Save(offer).Error
}

// DeleteOffer deletes an offer
func DeleteOffer(id uint) error {
	return db.Delete(&Offer{}, id).Error
}
