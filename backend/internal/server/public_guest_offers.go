package server

import (
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// publicGuestOffer is the unauthenticated menu projection of database.Offer.
// It matches the offer JSON shape except promo code, which is never a public
// field. Coded offers are omitted entirely by publicGuestOffers.
type publicGuestOffer struct {
	ID            uint       `json:"id"`
	BusinessID    uint       `json:"business_id"`
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	Image         string     `json:"image"`
	DiscountType  string     `json:"discount_type"`
	DiscountValue float64    `json:"discount_value"`
	StartDate     *time.Time `json:"start_date"`
	EndDate       *time.Time `json:"end_date"`
	WeekdayMask   int16      `json:"weekday_mask"`
	StartMinute   *int       `json:"start_minute,omitempty"`
	EndMinute     *int       `json:"end_minute,omitempty"`
	IsActive      bool       `json:"is_active"`
	ApplicableTo  string     `json:"applicable_to"`
	TargetID      *string    `json:"target_id"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// publicGuestOffers drops offers that carry a promo code and projects the
// rest. A nil or empty input returns an empty non-nil slice so the guest
// JSON is [] rather than null.
func publicGuestOffers(offers []database.Offer) []publicGuestOffer {
	out := make([]publicGuestOffer, 0, len(offers))
	for _, offer := range offers {
		if offer.Code != nil && strings.TrimSpace(*offer.Code) != "" {
			continue
		}
		out = append(out, publicGuestOffer{
			ID:            offer.ID,
			BusinessID:    offer.BusinessID,
			Name:          offer.Name,
			Description:   offer.Description,
			Image:         offer.Image,
			DiscountType:  offer.DiscountType,
			DiscountValue: offer.DiscountValue,
			StartDate:     offer.StartDate,
			EndDate:       offer.EndDate,
			WeekdayMask:   offer.WeekdayMask,
			StartMinute:   offer.StartMinute,
			EndMinute:     offer.EndMinute,
			IsActive:      offer.IsActive,
			ApplicableTo:  offer.ApplicableTo,
			TargetID:      offer.TargetID,
			CreatedAt:     offer.CreatedAt,
			UpdatedAt:     offer.UpdatedAt,
		})
	}
	return out
}
