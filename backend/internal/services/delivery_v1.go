package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/s3"
)

type DeliveryZoneDTO struct {
	ID                  uint            `json:"id"`
	Name                string          `json:"name"`
	Description         string          `json:"description"`
	DeliveryFee         float64         `json:"delivery_fee"`
	MinimumOrderAmount  float64         `json:"minimum_order_amount"`
	EstimatedTime       int             `json:"estimated_time"`
	Priority            int             `json:"priority"`
	CutoffBufferMinutes int             `json:"cutoff_buffer_minutes"`
	OperatingHours      string          `json:"operating_hours"`
	Boundaries          json.RawMessage `json:"boundaries,omitempty"`
	IsActive            bool            `json:"is_active"`
}

type DeliverySettingsDTO struct {
	BusinessID                  uint              `json:"business_id"`
	DeliveryEnabled             bool              `json:"delivery_enabled"`
	InHouseDeliveryEnabled      bool              `json:"in_house_delivery_enabled"`
	ThirdPartyEnabled           bool              `json:"third_party_enabled"`
	FlatDeliveryFee             float64           `json:"flat_delivery_fee"`
	FreeDeliveryMinimum         float64           `json:"free_delivery_minimum"`
	MinimumOrderAmount          float64           `json:"minimum_order_amount"`
	EstimatedPrepTime           int               `json:"estimated_prep_time"`
	MaxConcurrentDeliveries     int               `json:"max_concurrent_deliveries,omitempty"`
	DeliveryHoursSameAsBusiness bool              `json:"delivery_hours_same_as_business"`
	DeliveryStartTime           string            `json:"delivery_start_time"`
	DeliveryEndTime             string            `json:"delivery_end_time"`
	DeliveryInstructions        string            `json:"delivery_instructions,omitempty"`
	AutoAssignDrivers           bool              `json:"auto_assign_drivers,omitempty"`
	ExternalPartnerLinks        json.RawMessage   `json:"external_partner_links"`
	Zones                       []DeliveryZoneDTO `json:"zones"`
	LiveOrderCount              int64             `json:"live_order_count,omitempty"`
	PartnerFallbackAvailable    bool              `json:"partner_fallback_available"`
	EstimatedDeliveryMinutes    int               `json:"estimated_delivery_minutes,omitempty"`
	PaymentMode                 string            `json:"payment_mode"`
	OnlinePaymentAvailable      bool              `json:"online_payment_available,omitempty"`
}

type UpdateDeliveryZoneInput struct {
	ID                  *int            `json:"id,omitempty"`
	Name                string          `json:"name"`
	Description         string          `json:"description"`
	DeliveryFee         float64         `json:"delivery_fee"`
	MinimumOrderAmount  float64         `json:"minimum_order_amount"`
	EstimatedTime       int             `json:"estimated_time"`
	Priority            int             `json:"priority"`
	CutoffBufferMinutes int             `json:"cutoff_buffer_minutes"`
	OperatingHours      string          `json:"operating_hours"`
	Boundaries          json.RawMessage `json:"boundaries,omitempty"`
	IsActive            bool            `json:"is_active"`
}

type UpdateDeliverySettingsInput struct {
	DeliveryEnabled             *bool                      `json:"delivery_enabled"`
	InHouseDeliveryEnabled      *bool                      `json:"in_house_delivery_enabled"`
	ThirdPartyEnabled           *bool                      `json:"third_party_enabled"`
	FlatDeliveryFee             *float64                   `json:"flat_delivery_fee"`
	FreeDeliveryMinimum         *float64                   `json:"free_delivery_minimum"`
	MinimumOrderAmount          *float64                   `json:"minimum_order_amount"`
	EstimatedPrepTime           *int                       `json:"estimated_prep_time"`
	MaxConcurrentDeliveries     *int                       `json:"max_concurrent_deliveries"`
	DeliveryHoursSameAsBusiness *bool                      `json:"delivery_hours_same_as_business"`
	DeliveryStartTime           *string                    `json:"delivery_start_time"`
	DeliveryEndTime             *string                    `json:"delivery_end_time"`
	DeliveryInstructions        *string                    `json:"delivery_instructions"`
	AutoAssignDrivers           *bool                      `json:"auto_assign_drivers"`
	ExternalPartnerLinks        *json.RawMessage           `json:"external_partner_links"`
	Zones                       *[]UpdateDeliveryZoneInput `json:"zones"`
	PaymentMode                 *string                    `json:"payment_mode"`
}

type DeliveryQuoteRequest struct {
	OrderSubtotal   float64                  `json:"order_subtotal"`
	DeliveryAddress database.DeliveryAddress `json:"delivery_address" binding:"required"`
}

type DeliveryQuoteDTO struct {
	Eligible                 bool             `json:"eligible"`
	ReasonCode               string           `json:"reason_code"`
	Message                  string           `json:"message"`
	Zone                     *DeliveryZoneDTO `json:"zone,omitempty"`
	DeliveryFee              float64          `json:"delivery_fee"`
	MinimumOrderAmount       float64          `json:"minimum_order_amount"`
	FreeDeliveryMinimum      float64          `json:"free_delivery_minimum"`
	OrderSubtotal            float64          `json:"order_subtotal"`
	MeetsMinimum             bool             `json:"meets_minimum"`
	EstimatedPrepTime        int              `json:"estimated_prep_time"`
	EstimatedDeliveryMinutes int              `json:"estimated_delivery_minutes"`
	EstimatedTotalMinutes    int              `json:"estimated_total_minutes"`
	EstimatedDeliveryTime    *time.Time       `json:"estimated_delivery_time,omitempty"`
	CutoffAt                 *time.Time       `json:"cutoff_at,omitempty"`
	FulfillmentMode          string           `json:"fulfillment_mode"`
	PartnerFallbackAvailable bool             `json:"partner_fallback_available"`
	ExternalPartnerLinks     json.RawMessage  `json:"external_partner_links"`
}

type DeliveryCheckoutItemInput struct {
	MenuItemName    string                    `json:"menu_item_name" binding:"required"`
	MenuItemID      string                    `json:"menu_item_id"`
	Quantity        int                       `json:"quantity" binding:"required"`
	Price           float64                   `json:"price" binding:"required"`
	Options         []database.MenuItemOption `json:"options"`
	SpecialRequests string                    `json:"special_requests"`
	ItemType        string                    `json:"item_type"`
	BundleID        *uint                     `json:"bundle_id"`
	ParentBundleID  *uint                     `json:"parent_bundle_id"`
	SourceOfferID   *uint                     `json:"source_offer_id"`
}

// maxGuestDriverTipDollars is the absolute floor of the guest driver-tip cap:
// a tip is rejected when it exceeds max($500, 5x order subtotal) (DEL-PAY-01).
const maxGuestDriverTipDollars = 500.0

// Guest-typed delivery checkout fields are bounded in runes so a public order
// cannot park a paragraph of attacker text on a venue's records or in the
// owner notice. Limits match the reservation text rules for control and bidi
// characters (see validateReservationText).
const (
	maxGuestDeliveryCustomerNameRunes  = 100
	maxGuestDeliveryCustomerPhoneRunes = 32
	maxGuestDeliveryStreetRunes        = 200
	maxGuestDeliveryNotesRunes         = 500
	maxGuestDeliveryInstructionsRunes  = 500
)

type GuestDeliveryCheckoutRequest struct {
	CustomerID *uint `json:"-"`
	// ClientRequestID is the guest client's X-Request-Id, set by the handler
	// (never bound from the JSON body). Same id ⇒ replay of the original
	// checkout; empty ⇒ legacy non-idempotent behavior.
	ClientRequestID      string                      `json:"-"`
	CustomerName         string                      `json:"customer_name" binding:"required"`
	CustomerPhone        string                      `json:"customer_phone" binding:"required"`
	CustomerEmail        string                      `json:"customer_email"`
	CustomerLocale       string                      `json:"customer_locale"`
	DeliveryAddress      database.DeliveryAddress    `json:"delivery_address" binding:"required"`
	DeliveryInstructions string                      `json:"delivery_instructions"`
	ContactlessDelivery  bool                        `json:"contactless_delivery"`
	LeaveAtDoor          bool                        `json:"leave_at_door"`
	Notes                string                      `json:"notes"`
	PromoCode            string                      `json:"promo_code"`
	DriverTip            float64                     `json:"driver_tip"`
	Items                []DeliveryCheckoutItemInput `json:"items" binding:"required"`
}

type GuestDeliveryCheckoutDTO struct {
	Bill          *database.Bill          `json:"bill"`
	Order         *database.Order         `json:"order"`
	DeliveryOrder *database.DeliveryOrder `json:"delivery_order"`
	TrackingURL   string                  `json:"tracking_url"`
	// Duplicate is true when this response replays an earlier checkout for
	// the same X-Request-Id (nothing new was created). Mirrors the dine-in
	// replay contract (respondWithExistingGuestOrder's {"duplicate": true}).
	Duplicate bool `json:"duplicate,omitempty"`
}

func (s *DeliveryService) getPublicDeliveryBusiness(businessID uint) (*database.Business, error) {
	business, err := database.GetPublicBusinessByID(businessID)
	if err != nil {
		return nil, fmt.Errorf("%w: business not found", ErrDeliveryScopedNotFound)
	}
	if !business.IsActive || !business.BusinessPageEnabled {
		return nil, fmt.Errorf("%w: business not found", ErrDeliveryScopedNotFound)
	}
	return business, nil
}

func (s *DeliveryService) GetDeliverySettingsDTO(businessID uint, public bool) (*DeliverySettingsDTO, error) {
	var publicBusiness *database.Business
	if public {
		business, err := s.getPublicDeliveryBusiness(businessID)
		if err != nil {
			return nil, err
		}
		publicBusiness = business
	}
	var (
		settings *database.DeliverySettings
		zones    []DeliveryZoneDTO
		err      error
	)
	if public {
		settings, err = s.getPublicDeliverySettings(businessID)
		if err != nil {
			return nil, err
		}
		zones, err = s.getPublicDeliveryZones(businessID)
		if err != nil {
			return nil, err
		}
	} else {
		settings, err = s.GetDeliverySettings(businessID)
		if err != nil {
			return nil, err
		}
		zones, err = s.getDeliveryZones(businessID, false)
		if err != nil {
			return nil, err
		}
	}
	// Partner links are operator-configured URLs that guest surfaces render as
	// anchor hrefs. Re-filter on every read so a row that predates (or bypasses)
	// the write-time validator can never ship a javascript:/data: href to an
	// unauthenticated page (#897).
	safeLinks := SanitizeExternalPartnerLinks(json.RawMessage(settings.ExternalPartnerLinks))
	response := &DeliverySettingsDTO{
		BusinessID:                  businessID,
		DeliveryEnabled:             settings.DeliveryEnabled,
		InHouseDeliveryEnabled:      settings.InHouseDeliveryEnabled,
		ThirdPartyEnabled:           settings.ThirdPartyEnabled,
		FlatDeliveryFee:             float64(settings.FlatDeliveryFee) / 100.0,
		FreeDeliveryMinimum:         float64(settings.FreeDeliveryMinimum) / 100.0,
		MinimumOrderAmount:          float64(settings.MinimumOrderAmount) / 100.0,
		EstimatedPrepTime:           settings.EstimatedPrepTime,
		DeliveryHoursSameAsBusiness: settings.DeliveryHoursSameAsBusiness,
		DeliveryStartTime:           settings.DeliveryStartTime,
		DeliveryEndTime:             settings.DeliveryEndTime,
		ExternalPartnerLinks:        safeLinks,
		Zones:                       zones,
		PartnerFallbackAvailable:    hasJSONArrayValues(safeLinks),
	}
	// businessHasOnlinePayments is called on the public path too because
	// resolveDeliveryPaymentMode needs the result to return the effective mode
	// that the guest checkout will use. Ops-only fields (capacity, auto-assign,
	// dispatch instructions, online_payment_available, live_order_count) stay
	// off the public projection (#292). PaymentMode must still be populated
	// for guests.
	online := false
	if public && publicBusiness != nil && strings.TrimSpace(publicBusiness.SettlementAddr) != "" {
		online = true
	} else {
		online = s.businessHasOnlinePayments(businessID)
	}
	response.PaymentMode = resolveDeliveryPaymentMode(settings.PaymentMode, online)
	response.EstimatedDeliveryMinutes = settings.EstimatedPrepTime + s.defaultZoneTravelMinutes(zones)
	if public {
		// Administrator-closed venue: keep the informational storefront (and
		// partner links via delivery_enabled) but do not advertise in-house
		// checkout the quote/create path will refuse.
		if publicBusiness != nil && !database.IsBusinessOperational(publicBusiness) {
			response.InHouseDeliveryEnabled = false
		} else if response.InHouseDeliveryEnabled && !s.hasMatchableActiveZone(businessID) {
			// Don't advertise "entrega activa" when no active zone can match
			// an address (GeoJSON / empty postal+city). Partner fallback stays
			// available via delivery_enabled + third_party links (#714).
			response.InHouseDeliveryEnabled = false
		}
	} else {
		response.DeliveryInstructions = settings.DeliveryInstructions
		response.MaxConcurrentDeliveries = settings.MaxConcurrentDeliveries
		response.AutoAssignDrivers = settings.AutoAssignDrivers
		response.OnlinePaymentAvailable = online
		count, err := s.activeDeliveryCount(s.db, businessID)
		if err != nil {
			return nil, err
		}
		response.LiveOrderCount = count
	}
	return response, nil
}

func (s *DeliveryService) UpdateDeliverySettingsDTO(businessID uint, input UpdateDeliverySettingsInput) (*DeliverySettingsDTO, error) {
	settings, err := s.GetDeliverySettings(businessID)
	if err != nil {
		return nil, err
	}
	if input.DeliveryEnabled != nil {
		settings.DeliveryEnabled = *input.DeliveryEnabled
	}
	if input.InHouseDeliveryEnabled != nil {
		settings.InHouseDeliveryEnabled = *input.InHouseDeliveryEnabled
	}
	if input.ThirdPartyEnabled != nil {
		settings.ThirdPartyEnabled = *input.ThirdPartyEnabled
	}
	if input.FlatDeliveryFee != nil {
		settings.FlatDeliveryFee = int64(math.Round(*input.FlatDeliveryFee * 100))
	}
	if input.FreeDeliveryMinimum != nil {
		settings.FreeDeliveryMinimum = int64(math.Round(*input.FreeDeliveryMinimum * 100))
	}
	if input.MinimumOrderAmount != nil {
		settings.MinimumOrderAmount = int64(math.Round(*input.MinimumOrderAmount * 100))
	}
	if input.EstimatedPrepTime != nil {
		settings.EstimatedPrepTime = *input.EstimatedPrepTime
	}
	if input.MaxConcurrentDeliveries != nil {
		settings.MaxConcurrentDeliveries = *input.MaxConcurrentDeliveries
	}
	if input.DeliveryHoursSameAsBusiness != nil {
		settings.DeliveryHoursSameAsBusiness = *input.DeliveryHoursSameAsBusiness
	}
	if input.DeliveryStartTime != nil {
		settings.DeliveryStartTime = strings.TrimSpace(*input.DeliveryStartTime)
	}
	if input.DeliveryEndTime != nil {
		settings.DeliveryEndTime = strings.TrimSpace(*input.DeliveryEndTime)
	}
	if input.DeliveryInstructions != nil {
		settings.DeliveryInstructions = strings.TrimSpace(*input.DeliveryInstructions)
	}
	if input.AutoAssignDrivers != nil {
		settings.AutoAssignDrivers = *input.AutoAssignDrivers
	}
	if input.ExternalPartnerLinks != nil {
		// Write-time scheme gate: only http/https partner URLs are ever stored,
		// so an unsafe href cannot be persisted through any service caller (#897).
		settings.ExternalPartnerLinks = database.JSONRawMessage(SanitizeExternalPartnerLinks(*input.ExternalPartnerLinks))
	}
	if input.PaymentMode != nil {
		settings.PaymentMode = strings.TrimSpace(*input.PaymentMode)
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		// Sync zones first so validateDeliverySettings sees the post-sync snapshot.
		// A failed validation rolls back the zone rows with the rest of the tx (R1).
		if input.Zones != nil {
			serializedZones, syncErr := s.syncDeliveryZones(tx, businessID, *input.Zones)
			if syncErr != nil {
				return syncErr
			}
			settings.DeliveryZones = database.JSONRawMessage(normalizeJSONRaw(serializedZones, "[]"))
		}
		if err := s.validateDeliverySettings(settings); err != nil {
			return err
		}
		if err := tx.Where("business_id = ?", businessID).Omit(clause.Associations).Save(settings).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return s.GetDeliverySettingsDTO(businessID, false)
}

func (s *DeliveryService) QuoteDelivery(businessID uint, request DeliveryQuoteRequest) (*DeliveryQuoteDTO, error) {
	business, err := s.getPublicDeliveryBusiness(businessID)
	if err != nil {
		return nil, err
	}
	settings, err := s.GetDeliverySettings(businessID)
	if err != nil {
		return nil, err
	}
	fallbackLinks := SanitizeExternalPartnerLinks(json.RawMessage(settings.ExternalPartnerLinks))
	quote := &DeliveryQuoteDTO{
		Eligible:            false,
		FulfillmentMode:     "delivery",
		OrderSubtotal:       request.OrderSubtotal,
		FreeDeliveryMinimum: float64(settings.FreeDeliveryMinimum) / 100.0,
		MinimumOrderAmount:  float64(settings.MinimumOrderAmount) / 100.0,
		// EstimatedPrepTime intentionally omitted here: it is set only on the
		// eligible path (after all checks pass) so that ineligible quotes never
		// carry fabricated timing numbers (R14).
		PartnerFallbackAvailable: hasJSONArrayValues(fallbackLinks),
		ExternalPartnerLinks:     fallbackLinks,
	}

	if !settings.DeliveryEnabled || !settings.InHouseDeliveryEnabled {
		quote.ReasonCode = "delivery_disabled"
		quote.Message = "In-house delivery is not available for this business"
		return quote, nil
	}
	if err := s.validateDeliveryWindow(business, settings, quote); err != nil {
		quote.ReasonCode = "outside_delivery_hours"
		quote.Message = err.Error()
		return quote, nil
	}
	count, err := s.activeDeliveryCount(s.db, businessID)
	if err != nil {
		return nil, err
	}
	if settings.MaxConcurrentDeliveries > 0 && int(count) >= settings.MaxConcurrentDeliveries {
		// Do not advertise ops capacity state on the public quote surface (#292).
		quote.ReasonCode = "delivery_unavailable"
		quote.Message = "Delivery is temporarily unavailable"
		return quote, nil
	}

	zones, err := s.getDeliveryZones(businessID, true)
	if err != nil {
		return nil, err
	}
	matchedZone := resolveDeliveryZone(zones, request.DeliveryAddress)
	if matchedZone == nil && addressMatchesVenue(request.DeliveryAddress, business) {
		matchedZone = venueFallbackZone(zones, request.DeliveryAddress)
	}
	if len(zones) > 0 && matchedZone == nil {
		quote.ReasonCode = "zone_unavailable"
		quote.Message = "The delivery address is outside this business's delivery area"
		return quote, nil
	}
	if matchedZone != nil {
		// Public quote: expose fee/ETA fields without zone polygon internals (#292).
		zoneCopy := *matchedZone
		zoneCopy.Boundaries = nil
		quote.Zone = &zoneCopy
		quote.MinimumOrderAmount = maxFloat(quote.MinimumOrderAmount, matchedZone.MinimumOrderAmount)
	}

	// Apply per-zone CutoffBufferMinutes: pull the effective cutoff back by the
	// zone's buffer so late orders that the kitchen cannot fulfil are blocked.
	// This is evaluated after zone resolution because zones can carry different
	// buffer values and the matched zone is authoritative (R-cutoff-buffer).
	if matchedZone != nil && matchedZone.CutoffBufferMinutes > 0 && quote.CutoffAt != nil {
		buffered := quote.CutoffAt.Add(-time.Duration(matchedZone.CutoffBufferMinutes) * time.Minute)
		quote.CutoffAt = &buffered
		if time.Now().After(buffered) {
			quote.ReasonCode = "outside_delivery_hours"
			quote.Message = "Delivery is no longer accepting orders for this time window"
			return quote, nil
		}
	}

	quote.MeetsMinimum = request.OrderSubtotal >= quote.MinimumOrderAmount
	if !quote.MeetsMinimum {
		quote.ReasonCode = "below_minimum"
		// DEL-GT-5: carry the business currency — a bare amount reads as the
		// wrong currency for every non-USD business. The frontend localizes by
		// the stable reason_code; this message is the readable fallback.
		quote.Message = fmt.Sprintf("Minimum delivery order is %s %.2f", deliveryDisplayCurrency(business), quote.MinimumOrderAmount)
	}

	// Address is in a valid zone and capacity is available — from here on it is
	// safe to populate timing and fee. EstimatedPrepTime is set here (not at
	// struct initialisation) so that ineligible early-returns above carry zero
	// numbers rather than fabricated values (R14).
	quote.EstimatedPrepTime = settings.EstimatedPrepTime
	quote.DeliveryFee = s.computeDeliveryFee(settings, matchedZone, request.OrderSubtotal)
	quote.EstimatedDeliveryMinutes = s.computeDeliveryMinutes(matchedZone)
	quote.EstimatedTotalMinutes = quote.EstimatedPrepTime + quote.EstimatedDeliveryMinutes
	estimatedTime := time.Now().UTC().Add(time.Duration(quote.EstimatedTotalMinutes) * time.Minute)
	quote.EstimatedDeliveryTime = &estimatedTime
	if !quote.MeetsMinimum {
		return quote, nil
	}
	quote.Eligible = true
	quote.ReasonCode = "quote_available"
	quote.Message = "Delivery is available"
	return quote, nil
}

// validateGuestDeliveryCheckoutFields bounds guest-typed text after the
// public-business check and before idempotency, pricing, or the checkout
// transaction. Blank name or phone keeps the existing validation message.
// Other failures reuse validateReservationText and wrap its field error so
// the handler maps them to HTTP 400.
func validateGuestDeliveryCheckoutFields(request GuestDeliveryCheckoutRequest) error {
	name := strings.TrimSpace(request.CustomerName)
	phone := strings.TrimSpace(request.CustomerPhone)
	if name == "" || phone == "" {
		return fmt.Errorf("%w: customer name and phone are required", ErrDeliveryValidation)
	}
	checks := []struct {
		field     string
		value     string
		maxRunes  int
		multiline bool
	}{
		{"customer_name", name, maxGuestDeliveryCustomerNameRunes, false},
		{"customer_phone", phone, maxGuestDeliveryCustomerPhoneRunes, false},
		{"delivery_address.street", strings.TrimSpace(request.DeliveryAddress.Street), maxGuestDeliveryStreetRunes, false},
		{"notes", strings.TrimSpace(request.Notes), maxGuestDeliveryNotesRunes, true},
		{"delivery_instructions", strings.TrimSpace(request.DeliveryInstructions), maxGuestDeliveryInstructionsRunes, true},
	}
	for _, check := range checks {
		err := validateReservationText(check.field, check.value, check.maxRunes, check.multiline)
		if err == nil {
			continue
		}
		var ferr *ReservationFieldError
		if errors.As(err, &ferr) {
			return fmt.Errorf("%w: %s", ErrDeliveryValidation, ferr.Error())
		}
		return fmt.Errorf("%w: %s", ErrDeliveryValidation, err.Error())
	}
	return nil
}

func (s *DeliveryService) GuestDeliveryCheckout(businessID uint, request GuestDeliveryCheckoutRequest) (*GuestDeliveryCheckoutDTO, error) {
	if _, err := s.getPublicDeliveryBusiness(businessID); err != nil {
		return nil, err
	}
	if err := validateGuestDeliveryCheckoutFields(request); err != nil {
		return nil, err
	}
	// Idempotency (Wave 4): a network retry / double-tap carries the same
	// X-Request-Id. Replay the original checkout instead of minting a second
	// Bill/Order/DeliveryOrder. Empty id keeps legacy non-idempotent behavior.
	requestID := strings.TrimSpace(request.ClientRequestID)
	if requestID != "" {
		if existing, err := s.findExistingGuestDeliveryCheckout(businessID, requestID); err != nil {
			return nil, err
		} else if existing != nil {
			return existing, nil
		}
	}
	if _, err := mail.ParseAddress(strings.TrimSpace(request.CustomerEmail)); err != nil {
		return nil, fmt.Errorf("%w: a valid customer email is required for delivery status updates", ErrDeliveryValidation)
	}
	if len(request.Items) == 0 {
		return nil, fmt.Errorf("%w: at least one item is required", ErrDeliveryValidation)
	}
	// DEL-PAY-01: driver_tip arrives as an unbounded float64 on a public,
	// unauthenticated route. Non-finite values survive JSON binding and huge
	// magnitudes overflow the int64 cents conversion below (undefined per the
	// Go spec — negative on amd64), so reject them before any money math.
	if math.IsNaN(request.DriverTip) || math.IsInf(request.DriverTip, 0) {
		return nil, fmt.Errorf("%w: driver tip must be a valid amount", ErrDeliveryValidation)
	}
	if request.DriverTip < 0 {
		return nil, fmt.Errorf("%w: driver tip cannot be negative", ErrDeliveryValidation)
	}
	if err := s.validateGuestDeliveryCustomer(request.CustomerID, businessID); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrDeliveryValidation, err.Error())
	}

	promotionInput := make([]PromotionInputLine, 0, len(request.Items))
	for _, itemReq := range request.Items {
		promotionInput = append(promotionInput, PromotionInputLine{
			Name:            itemReq.MenuItemName,
			MenuItemID:      itemReq.MenuItemID,
			Quantity:        itemReq.Quantity,
			UnitPrice:       itemReq.Price,
			Options:         itemReq.Options,
			SpecialRequests: itemReq.SpecialRequests,
			ItemType:        itemReq.ItemType,
			BundleID:        itemReq.BundleID,
			ParentBundleID:  itemReq.ParentBundleID,
			SourceOfferID:   itemReq.SourceOfferID,
		})
	}

	pricedOrder, business, err := PriceOrderInputsByBusinessIDWithPromo(businessID, promotionInput, strings.TrimSpace(request.PromoCode))
	if err != nil {
		// Pricing errors are user-facing (unknown items, invalid quantities, etc.)
		// and safe to surface as validation failures.
		return nil, fmt.Errorf("%w: %s", ErrDeliveryValidation, err.Error())
	}

	subtotal := PromotionLinesSubtotal(pricedOrder.Lines)
	billItems := PromotionLinesToBillItems(pricedOrder.Lines)

	// DEL-PAY-01 cap rule: a tip above max($500, 5x order subtotal) is treated
	// as input abuse, not generosity. The subtotal-relative arm lets very large
	// orders carry proportionally large tips; the $500 floor covers small ones.
	if request.DriverTip > maxFloat(maxGuestDriverTipDollars, subtotal*5) {
		return nil, fmt.Errorf("%w: driver tip exceeds the maximum allowed amount", ErrDeliveryValidation)
	}

	quote, err := s.QuoteDelivery(businessID, DeliveryQuoteRequest{
		OrderSubtotal:   subtotal,
		DeliveryAddress: request.DeliveryAddress,
	})
	if err != nil {
		return nil, err
	}
	if !quote.Eligible {
		return nil, fmt.Errorf("%w: %s", ErrDeliveryValidation, quote.Message)
	}

	taxAmount, serviceFeeAmount, totalAmount := ComputeBillTotals(subtotal, business.TaxRate, business.ServiceFeeRate)
	totalAmount += quote.DeliveryFee + request.DriverTip

	// DEL-OP-2 follow-up: snapshot the EFFECTIVE payment mode in force at
	// checkout time onto the order row, so operators can later tell which
	// in-flight orders were placed under a mode that differs from the current
	// settings. Resolved before the transaction (it reads settings + plugin
	// state, not transactional data).
	effectivePaymentMode := s.EffectiveDeliveryPaymentMode(businessID)

	// DELIV-NOTIF-2 follow-up: gate the Telegram outbox write before opening
	// the transaction (a cached eligibility read) so the notification row can
	// be enqueued INSIDE the same transaction as the order — a true
	// transactional outbox, mirroring the dine-in paths in orders.go /
	// guest_handlers.go. A crash between the checkout commit and a separate
	// post-commit enqueue can no longer lose the operator notification.
	//
	// The venue-notice cap is counted BEFORE insert. recent >= max means this
	// checkout would be the 201st order in the window, so it enqueues nothing.
	// A count error fails closed (no telegram). The post-insert email cap
	// lives in notifyBusinessNewDelivery, where the new row is already included.
	recentOrders, recentErr := countRecentDeliveryOrders(s.noticeDB(), businessID, time.Now())
	if recentErr != nil {
		log.Printf("guest delivery notice cap: business_id=%d: %v", businessID, recentErr)
	}
	notifyTelegram := recentErr == nil &&
		recentOrders < int64(maxVenueDeliveryNoticesPerDay) &&
		ShouldEnqueueTelegramNotification(businessID, PluginEventOrderCreated)

	result := &GuestDeliveryCheckoutDTO{}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Re-check the cap under a row lock on this business's settings.
		// QuoteDelivery counted outside the transaction, so two checkouts at
		// cap-1 could both pass and both insert. FOR UPDATE serializes those
		// checkouts on Postgres; SQLite ignores the lock clause. No settings
		// row means no cap.
		var lockedSettings database.DeliverySettings
		lockErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("business_id = ?", businessID).
			First(&lockedSettings).Error
		if lockErr != nil && !errors.Is(lockErr, gorm.ErrRecordNotFound) {
			return lockErr
		}
		if lockErr == nil && lockedSettings.MaxConcurrentDeliveries > 0 {
			active, countErr := s.activeDeliveryCount(tx, businessID)
			if countErr != nil {
				return countErr
			}
			if int(active) >= lockedSettings.MaxConcurrentDeliveries {
				return fmt.Errorf("%w: Delivery is temporarily unavailable", ErrDeliveryValidation)
			}
		}

		bill := &database.Bill{
			BusinessID:       businessID,
			BillNumber:       fmt.Sprintf("B%d-%d", businessID, time.Now().UnixNano()),
			Notes:            strings.TrimSpace(request.Notes),
			Subtotal:         int64(math.Round(subtotal * 100)),
			TaxAmount:        int64(math.Round(taxAmount * 100)),
			ServiceFeeAmount: int64(math.Round(serviceFeeAmount * 100)),
			TotalAmount:      int64(math.Round(totalAmount * 100)),
			Status:           database.BillStatusOpen,
			SettlementAddr:   business.SettlementAddr,
			TippingAddr:      business.TippingAddr,
		}
		itemsJSON, marshalErr := json.Marshal(billItems)
		if marshalErr != nil {
			return marshalErr
		}
		bill.Items = string(itemsJSON)
		// Guest delivery bills have no table — Bill.TableID is uint (not *uint)
		// so GORM would insert 0, which violates fk_bills_table. Omit the column
		// so Postgres stores NULL.
		if err := tx.Omit("table_id").Create(bill).Error; err != nil {
			return err
		}
		for i := range billItems {
			billItems[i].BillID = bill.ID
		}
		if len(billItems) > 0 {
			if err := tx.Create(&billItems).Error; err != nil {
				return err
			}
		}

		orderItems := pricedOrder.Lines
		order := &database.Order{
			BillID:      bill.ID,
			BusinessID:  businessID,
			OrderNumber: fmt.Sprintf("O%d-%d", businessID, time.Now().UnixNano()),
			Status:      database.OrderStatusPending,
			CreatedBy:   "guest_delivery",
			Notes:       strings.TrimSpace(request.Notes),
		}
		if requestID != "" {
			order.ClientRequestID = &requestID
		}
		orderJSON, marshalErr := json.Marshal(orderItems)
		if marshalErr != nil {
			return marshalErr
		}
		order.Items = string(orderJSON)
		if err := tx.Create(order).Error; err != nil {
			return err
		}
		if notifyTelegram {
			// Atomic with the order create: order committed ⇒ notification row
			// committed. A failed insert rolls back the checkout, which is
			// acceptable — both live in the same DB transaction, so the only
			// realistic failure is a DB fault that would fail the commit anyway.
			if err := enqueueDeliveryTelegramOrderCreatedTx(tx, *order, *bill, business, orderItems); err != nil {
				return err
			}
		}

		var zoneID *uint
		if quote.Zone != nil {
			zoneID = &quote.Zone.ID
		}
		quoteJSON, marshalErr := json.Marshal(quote)
		if marshalErr != nil {
			return marshalErr
		}
		// Enrich quote metadata with bag size so dispatch cards can show an
		// item count without joining orders/bills on every list poll.
		var quoteMeta map[string]interface{}
		if err := json.Unmarshal(quoteJSON, &quoteMeta); err == nil {
			itemCount := 0
			for _, item := range request.Items {
				itemCount += item.Quantity
			}
			quoteMeta["item_count"] = itemCount
			if enriched, err := json.Marshal(quoteMeta); err == nil {
				quoteJSON = enriched
			}
		}
		delivery := &database.DeliveryOrder{
			BusinessID:            businessID,
			BillID:                bill.ID,
			OrderID:               &order.ID,
			CustomerID:            request.CustomerID,
			ZoneID:                zoneID,
			DeliveryNumber:        s.generateDeliveryNumber(),
			DeliveryType:          database.DeliveryTypeInHouse,
			Status:                database.DeliveryStatusPending,
			Priority:              database.PriorityNormal,
			FulfillmentMode:       quote.FulfillmentMode,
			CustomerName:          strings.TrimSpace(request.CustomerName),
			CustomerPhone:         strings.TrimSpace(request.CustomerPhone),
			CustomerEmail:         strings.TrimSpace(request.CustomerEmail),
			CustomerLocale:        strings.TrimSpace(request.CustomerLocale),
			DeliveryAddress:       request.DeliveryAddress,
			DeliveryFee:           int64(math.Round(quote.DeliveryFee * 100)),
			DriverTip:             int64(math.Round(request.DriverTip * 100)),
			PlatformFee:           0,
			DeliveryInstructions:  strings.TrimSpace(request.DeliveryInstructions),
			ContactlessDelivery:   request.ContactlessDelivery,
			LeaveAtDoor:           request.LeaveAtDoor,
			QuoteMetadata:         quoteJSON,
			CutoffAt:              quote.CutoffAt,
			EstimatedDeliveryTime: quote.EstimatedDeliveryTime,
			PaymentModeStored:     effectivePaymentMode,
		}
		if quote.EstimatedPrepTime > 0 {
			estimatedPickup := time.Now().UTC().Add(time.Duration(quote.EstimatedPrepTime) * time.Minute)
			delivery.EstimatedPickupTime = &estimatedPickup
		}
		if err := tx.Create(delivery).Error; err != nil {
			return err
		}
		history := &database.DeliveryStatusHistory{
			DeliveryOrderID: delivery.ID,
			Status:          database.DeliveryStatusPending,
			Notes:           "Guest delivery checkout created",
			ChangedBy:       "customer",
			CreatedAt:       time.Now().UTC(),
		}
		if err := tx.Create(history).Error; err != nil {
			return err
		}
		result.Bill = bill
		result.Order = order
		result.DeliveryOrder = delivery
		result.TrackingURL = fmt.Sprintf("/delivery/%s/track", delivery.DeliveryNumber)
		return nil
	})
	if err != nil {
		// Unique-violation on idx_orders_delivery_request_identity means a
		// concurrent submit with the same X-Request-Id won the race — replay it.
		if requestID != "" && isDuplicateKeyError(err) {
			if existing, replayErr := s.findExistingGuestDeliveryCheckout(businessID, requestID); replayErr == nil && existing != nil {
				return existing, nil
			}
		}
		return nil, err
	}

	// Post-commit: business email (mirrors staff-created delivery path) +
	// guest "order received" confirmation (R10).
	// Reuse the business resolved by PriceOrderInputsByBusinessID above —
	// no second DB round-trip needed.
	s.notifyBusinessNewDelivery(result.DeliveryOrder)
	guestLocale := guestNotificationLocale(result.DeliveryOrder, business)
	title, body := localizedReceivedMessage(
		result.DeliveryOrder.DeliveryNumber,
		guestLocale,
	)
	// The recipient was typed by an anonymous caller: the confirmation goes
	// through the guest lane of the venue's tenant mail budget.
	s.sendDeliveryNotificationToEmail(
		deliveryCustomerMailOrigin(result.DeliveryOrder, emails.MailPurposeGuestDeliveryRequest),
		result.DeliveryOrder.CustomerEmail, result.DeliveryOrder.CustomerName, title, body, guestLocale)

	return result, nil
}

// findExistingGuestDeliveryCheckout rebuilds the original checkout DTO for a
// replayed X-Request-Id, or returns (nil, nil) when no prior checkout exists.
// The lookup is a single indexed probe on
// (business_id, client_request_id) WHERE created_by = 'guest_delivery'.
func (s *DeliveryService) findExistingGuestDeliveryCheckout(businessID uint, clientRequestID string) (*GuestDeliveryCheckoutDTO, error) {
	var order database.Order
	err := s.db.
		Where("business_id = ? AND created_by = ? AND client_request_id = ?",
			businessID, "guest_delivery", clientRequestID).
		First(&order).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	var bill database.Bill
	if err := s.db.First(&bill, order.BillID).Error; err != nil {
		return nil, err
	}
	var delivery database.DeliveryOrder
	if err := s.db.Where("order_id = ?", order.ID).First(&delivery).Error; err != nil {
		return nil, err
	}
	return &GuestDeliveryCheckoutDTO{
		Bill:          &bill,
		Order:         &order,
		DeliveryOrder: &delivery,
		TrackingURL:   fmt.Sprintf("/delivery/%s/track", delivery.DeliveryNumber),
		Duplicate:     true,
	}, nil
}

// isDuplicateKeyError reports whether err is a unique/duplicate-key violation.
// Same lowercase heuristic as guest_handlers.go's guestOrderIsDuplicateError.
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate")
}

// enqueueDeliveryTelegramOrderCreatedTx writes the order.created Telegram
// outbox row on the guest-delivery checkout transaction (transactional
// outbox, DELIV-NOTIF-2 follow-up). The payload mirrors the dine-in builders
// (orders.go enqueueTelegramOrderCreatedNotificationTx / guest_handlers.go
// enqueueGuestTelegramOrderCreatedNotificationTx) with source "delivery";
// delivery bills have no table, so the table fields carry the zero/placeholder
// values the renderer already handles. The EventID "order:<id>" plus the
// (business_id, plugin_name, event_type, event_id) ON CONFLICT DO NOTHING
// unique index make any replayed enqueue a no-op — exactly one send per order.
func enqueueDeliveryTelegramOrderCreatedTx(tx *gorm.DB, order database.Order, bill database.Bill, business *database.Business, items []database.OrderItem) error {
	var itemCount int
	for _, item := range items {
		itemCount += item.Quantity
	}
	// Bill.TotalAmount is already canonical cents (items + tax/service +
	// delivery fee + driver tip + platform fee). The shared Telegram template
	// labels this field "Total", so do not substitute the items-only subtotal
	// and do not scale the bill again.
	totalCents := bill.TotalAmount
	currency := ""
	if business != nil {
		currency = business.DisplayCurrency
		if currency == "" {
			currency = business.DefaultCurrency
		}
	}
	payload := map[string]interface{}{
		"order_id":     order.ID,
		"order_number": order.OrderNumber,
		"bill_id":      order.BillID,
		"bill_number":  bill.BillNumber,
		"table_id":     bill.TableID,
		"table_name":   "Unassigned", // guest delivery bills never have a table
		"item_count":   itemCount,
		"total_cents":  totalCents,
		"currency":     currency,
		"notes":        order.Notes,
		"source":       "delivery",
	}

	if _, _, err := EnqueuePluginNotificationTx(tx, PluginNotificationEvent{
		BusinessID: order.BusinessID,
		EventType:  PluginEventOrderCreated,
		EventID:    fmt.Sprintf("order:%d", order.ID),
		Payload:    payload,
		CreatedAt:  time.Now().UTC(),
	}, "telegram"); err != nil {
		return fmt.Errorf("enqueue telegram delivery order notification (business_id=%d order_id=%d): %w", order.BusinessID, order.ID, err)
	}
	return nil
}

func (s *DeliveryService) validateGuestDeliveryCustomer(customerID *uint, businessID uint) error {
	if customerID == nil {
		return nil
	}

	var customer database.Customer
	if err := s.db.Select("id", "is_active").First(&customer, *customerID).Error; err != nil || !customer.IsActive {
		return fmt.Errorf("customer is not connected to this business")
	}

	var connection database.CustomerBusiness
	if err := s.db.Where("customer_id = ? AND business_id = ? AND is_active = ?", *customerID, businessID, true).
		First(&connection).Error; err != nil {
		return fmt.Errorf("customer is not connected to this business")
	}

	return nil
}

func (s *DeliveryService) GetDeliveryOrderByBusiness(businessID, deliveryID uint) (*database.DeliveryOrder, error) {
	order, err := s.GetDeliveryOrder(deliveryID)
	if err != nil {
		return nil, err
	}
	if order.BusinessID != businessID {
		return nil, fmt.Errorf("delivery order not found")
	}
	return order, nil
}

// assertDeliveryOrderOwnership verifies the delivery order belongs to the
// business with a narrow projection (DELIV-PERF-3 / DEL-SM-3). The mutation
// wrappers below only need an ownership answer — the operations themselves
// re-read the row under FOR UPDATE (UpdateDeliveryStatus / AssignDriver /
// cancelDeliveryLinked) — so hydrating the full dispatch aggregate
// (Business + Driver + Customer + unbounded StatusHistory preloads) here was
// 4 wasted queries per status click on the hot dispatch path.
func (s *DeliveryService) assertDeliveryOrderOwnership(businessID, deliveryID uint) error {
	var order database.DeliveryOrder
	if err := s.db.Select("id").
		Where("id = ? AND business_id = ?", deliveryID, businessID).
		First(&order).Error; err != nil {
		return fmt.Errorf("delivery order not found")
	}
	return nil
}

func (s *DeliveryService) UpdateDeliveryStatusByBusiness(businessID, deliveryID uint, status database.DeliveryStatus, location *database.Location, changedBy string) error {
	if err := s.assertDeliveryOrderOwnership(businessID, deliveryID); err != nil {
		return err
	}
	return s.UpdateDeliveryStatus(deliveryID, status, location, changedBy)
}

func (s *DeliveryService) AssignDriverByBusiness(businessID, deliveryID, driverID uint) error {
	if err := s.assertDeliveryOrderOwnership(businessID, deliveryID); err != nil {
		return err
	}
	driver, err := s.GetDriverByBusiness(businessID, driverID)
	if err != nil {
		return err
	}
	return s.AssignDriver(deliveryID, driver.ID)
}

func (s *DeliveryService) CancelDeliveryOrderByBusiness(businessID, deliveryID uint, reason, cancelledBy string) error {
	if err := s.assertDeliveryOrderOwnership(businessID, deliveryID); err != nil {
		return err
	}
	return s.CancelDeliveryOrder(deliveryID, reason, cancelledBy)
}

func rawZonesToDTO(zones []database.DeliveryZone) []DeliveryZoneDTO {
	result := make([]DeliveryZoneDTO, 0, len(zones))
	for _, zone := range zones {
		result = append(result, DeliveryZoneDTO{
			ID:                  zone.ID,
			Name:                zone.Name,
			Description:         zone.Description,
			DeliveryFee:         float64(zone.DeliveryFee) / 100.0,
			MinimumOrderAmount:  float64(zone.MinimumOrderAmount) / 100.0,
			EstimatedTime:       zone.EstimatedTime,
			Priority:            zone.Priority,
			CutoffBufferMinutes: zone.CutoffBufferMinutes,
			OperatingHours:      zone.OperatingHours,
			Boundaries:          normalizeJSONRaw(json.RawMessage(zone.Boundaries), "{}"),
			IsActive:            zone.IsActive,
		})
	}
	return result
}

func (s *DeliveryService) hasMatchableActiveZone(businessID uint) bool {
	// Narrow coverage probe: guests never receive boundaries (#567), but the
	// public projection needs to know whether in-house checkout can match
	// any address. SELECT only the matcher columns — never SELECT *.
	var rows []struct {
		Boundaries string `gorm:"column:boundaries"`
		IsActive   bool   `gorm:"column:is_active"`
	}
	if err := s.db.Table("delivery_zones").
		Select("boundaries", "is_active").
		Where("business_id = ? AND is_active = ?", businessID, true).
		Limit(publicDeliveryZoneLimit).
		Find(&rows).Error; err != nil {
		return false
	}
	for _, row := range rows {
		if zoneCoverageReady(DeliveryZoneDTO{
			IsActive:   row.IsActive,
			Boundaries: json.RawMessage(row.Boundaries),
		}) {
			return true
		}
	}
	return false
}

func (s *DeliveryService) getDeliveryZones(businessID uint, activeOnly bool) ([]DeliveryZoneDTO, error) {
	var zones []database.DeliveryZone
	query := s.db.Where("business_id = ?", businessID)
	if activeOnly {
		query = query.Where("is_active = ?", true)
	}
	if err := query.Order("priority DESC, estimated_time ASC, id ASC").Find(&zones).Error; err != nil {
		return nil, err
	}
	return rawZonesToDTO(zones), nil
}

// publicDeliverySettingsColumns is the guest GET projection. Partner API keys
// and the unused delivery_zones JSON snapshot stay off an anonymous read (#567).
var publicDeliverySettingsColumns = []string{
	"id",
	"business_id",
	"delivery_enabled",
	"in_house_delivery_enabled",
	"third_party_enabled",
	"payment_mode",
	"default_delivery_fee",
	"free_delivery_threshold",
	"minimum_order_amount",
	"estimated_prep_time",
	"delivery_hours_same_as_business",
	"delivery_start_time",
	"delivery_end_time",
	"external_partner_links",
}

func (s *DeliveryService) getPublicDeliverySettings(businessID uint) (*database.DeliverySettings, error) {
	var settings database.DeliverySettings
	if err := s.db.Select(publicDeliverySettingsColumns).
		Where("business_id = ?", businessID).
		First(&settings).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return defaultDeliverySettings(businessID), nil
		}
		return nil, fmt.Errorf("failed to get delivery settings: %w", err)
	}
	if len(settings.ExternalPartnerLinks) == 0 || string(settings.ExternalPartnerLinks) == "null" {
		settings.ExternalPartnerLinks = database.JSONRawMessage("[]")
	}
	return &settings, nil
}

// publicDeliveryZoneLimit bounds the guest settings list. Quote matching still
// uses the unbounded operator-style read on the quote path.
const publicDeliveryZoneLimit = 50

// publicDeliveryZoneColumns omits GeoJSON boundaries, priority, and cutoff —
// guests never receive those fields, so they must not be selected (#567).
var publicDeliveryZoneColumns = []string{
	"id",
	"business_id",
	"name",
	"description",
	"delivery_fee",
	"minimum_order_amount",
	"estimated_time",
	"operating_hours",
	"is_active",
}

func (s *DeliveryService) getPublicDeliveryZones(businessID uint) ([]DeliveryZoneDTO, error) {
	var zones []database.DeliveryZone
	if err := s.db.Select(publicDeliveryZoneColumns).
		Where("business_id = ? AND is_active = ?", businessID, true).
		Order("priority DESC, estimated_time ASC, id ASC").
		Limit(publicDeliveryZoneLimit).
		Find(&zones).Error; err != nil {
		return nil, err
	}
	return rawZonesToPublicDTO(zones), nil
}

func rawZonesToPublicDTO(zones []database.DeliveryZone) []DeliveryZoneDTO {
	result := make([]DeliveryZoneDTO, 0, len(zones))
	for _, zone := range zones {
		result = append(result, DeliveryZoneDTO{
			ID:                 zone.ID,
			Name:               zone.Name,
			Description:        zone.Description,
			DeliveryFee:        float64(zone.DeliveryFee) / 100.0,
			MinimumOrderAmount: float64(zone.MinimumOrderAmount) / 100.0,
			EstimatedTime:      zone.EstimatedTime,
			OperatingHours:     zone.OperatingHours,
			IsActive:           zone.IsActive,
		})
	}
	return result
}

func (s *DeliveryService) syncDeliveryZones(tx *gorm.DB, businessID uint, inputs []UpdateDeliveryZoneInput) (json.RawMessage, error) {
	keepIDs := make([]uint, 0, len(inputs))
	for _, input := range inputs {
		zone := database.DeliveryZone{BusinessID: businessID}
		if input.ID != nil && *input.ID > 0 {
			if err := tx.Where("business_id = ? AND id = ?", businessID, uint(*input.ID)).First(&zone).Error; err != nil {
				return nil, err
			}
		}
		zone.Name = strings.TrimSpace(input.Name)
		zone.Description = strings.TrimSpace(input.Description)
		zone.DeliveryFee = int64(math.Round(input.DeliveryFee * 100))
		zone.MinimumOrderAmount = int64(math.Round(input.MinimumOrderAmount * 100))
		zone.EstimatedTime = input.EstimatedTime
		zone.Priority = input.Priority
		zone.CutoffBufferMinutes = input.CutoffBufferMinutes
		zone.OperatingHours = strings.TrimSpace(input.OperatingHours)
		zone.Boundaries = string(normalizeJSONRaw(input.Boundaries, "{}"))
		zone.IsActive = input.IsActive
		if zone.ID == 0 {
			if err := tx.Create(&zone).Error; err != nil {
				return nil, err
			}
		} else {
			if err := tx.Omit(clause.Associations).Save(&zone).Error; err != nil {
				return nil, err
			}
		}
		keepIDs = append(keepIDs, zone.ID)
	}
	cleanup := tx.Where("business_id = ?", businessID)
	if len(keepIDs) > 0 {
		cleanup = cleanup.Where("id NOT IN ?", keepIDs)
	}
	if err := cleanup.Delete(&database.DeliveryZone{}).Error; err != nil {
		return nil, err
	}
	// Use tx for the final read so the serialized snapshot reflects the
	// in-progress transaction writes (avoids SQLite "table is locked" when
	// syncDeliveryZones is called inside a db.Transaction block).
	var rawZones []database.DeliveryZone
	if err := tx.Where("business_id = ?", businessID).Order("priority DESC, estimated_time ASC, id ASC").Find(&rawZones).Error; err != nil {
		return nil, err
	}
	serialized, err := json.Marshal(rawZonesToDTO(rawZones))
	if err != nil {
		return nil, err
	}
	return serialized, nil
}

// zoneBoundaries holds the geographic boundary data for a delivery zone.
type zoneBoundaries struct {
	PostalCodes []string `json:"postal_codes"`
	Cities      []string `json:"cities"`
}

// zoneShape is used exclusively by the validator to decode raw zone JSON without
// depending on the full DeliveryZoneDTO (which has Boundaries as json.RawMessage).
type zoneShape struct {
	Name               string         `json:"name"`
	Boundaries         zoneBoundaries `json:"boundaries"`
	Priority           int            `json:"priority"`
	EstimatedTime      int            `json:"estimated_time"`
	DeliveryFee        float64        `json:"delivery_fee"`
	MinimumOrderAmount float64        `json:"minimum_order_amount"`
	IsActive           bool           `json:"is_active"`
}

func decodeZoneShapes(raw json.RawMessage) ([]zoneShape, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var zones []zoneShape
	if err := json.Unmarshal(raw, &zones); err != nil {
		return nil, err
	}
	return zones, nil
}

func countPartnerLinks(raw json.RawMessage) (int, error) {
	if len(raw) == 0 {
		return 0, nil
	}
	var links []map[string]interface{}
	if err := json.Unmarshal(raw, &links); err != nil {
		return 0, err
	}
	count := 0
	for _, l := range links {
		name, _ := l["name"].(string)
		linkURL, _ := l["url"].(string)
		// A link whose URL is not a real web URL is never rendered on a guest
		// surface, so it cannot satisfy third_party_enabled either (#897).
		if strings.TrimSpace(name) != "" && isSafePartnerLinkURL(linkURL) {
			count++
		}
	}
	return count, nil
}

// PublicPartnerLinks returns the business's courier partner links for guest
// tracking surfaces, already filtered to real web URLs. It reads through the
// service's injected handle rather than the global one so tests and callers
// share one DB (#897).
func (s *DeliveryService) PublicPartnerLinks(businessID uint) json.RawMessage {
	var raw string
	if err := s.db.Model(&database.DeliverySettings{}).
		Select("external_partner_links").
		Where("business_id = ?", businessID).
		Limit(1).
		Scan(&raw).Error; err != nil {
		return json.RawMessage("[]")
	}
	return SanitizeExternalPartnerLinks(json.RawMessage(raw))
}

// isSafePartnerLinkURL reports whether an operator-configured partner URL is a
// real web URL. Guest surfaces render these values straight into anchor hrefs,
// so anything that is not http/https (javascript:, data:, relative garbage) is
// treated as unusable rather than trusted (#897).
func isSafePartnerLinkURL(value string) bool {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return false
	}
	return parsed.Host != ""
}

// SanitizeExternalPartnerLinks drops every partner row whose url is missing or
// uses a non-web scheme and strips unsafe icon_url values, preserving the rest
// of each row verbatim. It is the single read/write chokepoint for partner
// links leaving the delivery service (#897).
func SanitizeExternalPartnerLinks(raw json.RawMessage) json.RawMessage {
	normalized := normalizeJSONRaw(raw, "[]")

	var links []map[string]json.RawMessage
	if err := json.Unmarshal(normalized, &links); err != nil {
		return json.RawMessage("[]")
	}

	kept := make([]map[string]json.RawMessage, 0, len(links))
	for _, link := range links {
		if !isSafePartnerLinkURL(rawJSONString(link["url"])) {
			continue
		}
		if icon, ok := link["icon_url"]; ok && !isSafePartnerLinkURL(rawJSONString(icon)) && !s3.IsOwnMediaURL(rawJSONString(icon)) {
			delete(link, "icon_url")
		}
		kept = append(kept, link)
	}

	out, err := json.Marshal(kept)
	if err != nil {
		return json.RawMessage("[]")
	}
	return out
}

func rawJSONString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}

func (s *DeliveryService) validateDeliverySettings(settings *database.DeliverySettings) error {
	// Server-side normalization: when delivery is off, sub-flags must be off too.
	if !settings.DeliveryEnabled {
		settings.InHouseDeliveryEnabled = false
		settings.ThirdPartyEnabled = false
	}

	// Money sanity (fields are int64 cents after Tasks 1-3).
	if settings.FlatDeliveryFee < 0 ||
		settings.FreeDeliveryMinimum < 0 || settings.MinimumOrderAmount < 0 {
		return fmt.Errorf("delivery fees and minimums cannot be negative")
	}
	if settings.EstimatedPrepTime < 0 || settings.MaxConcurrentDeliveries < 0 {
		return fmt.Errorf("timing and capacity values cannot be negative")
	}

	if settings.PaymentMode != "" && !database.DeliveryPaymentMode(settings.PaymentMode).IsValid() {
		return fmt.Errorf("invalid payment_mode %q", settings.PaymentMode)
	}

	// DEL-SM-4 / empty-custom fail-closed: when operators opt out of business
	// hours they must set an explicit window. Empty custom start/end used to
	// persist and then fail OPEN at quote time (24/7 delivery, including on
	// days the restaurant is closed). Require both sides as valid HH:MM.
	if !settings.DeliveryHoursSameAsBusiness {
		start := strings.TrimSpace(settings.DeliveryStartTime)
		end := strings.TrimSpace(settings.DeliveryEndTime)
		if start == "" || end == "" {
			return fmt.Errorf("delivery_start_time and delivery_end_time are required when delivery_hours_same_as_business is false")
		}
		if _, _, err := parseClockParts(start); err != nil {
			return fmt.Errorf("delivery_start_time must be HH:MM (24-hour), got %q", settings.DeliveryStartTime)
		}
		if _, _, err := parseClockParts(end); err != nil {
			return fmt.Errorf("delivery_end_time must be HH:MM (24-hour), got %q", settings.DeliveryEndTime)
		}
	}

	// Skip fulfillment-route checks when delivery is off after normalization.
	if !settings.DeliveryEnabled {
		return nil
	}

	// Decode zones for structural checks.
	zones, err := decodeZoneShapes(json.RawMessage(settings.DeliveryZones))
	if err != nil {
		return fmt.Errorf("invalid delivery_zones: %w", err)
	}
	activeZoneCount := 0
	priorities := map[int]bool{}
	for i, z := range zones {
		if z.IsActive {
			activeZoneCount++
		}
		if strings.TrimSpace(z.Name) == "" {
			return fmt.Errorf("zone %d: name required", i+1)
		}
		if len(z.Boundaries.PostalCodes) == 0 && len(z.Boundaries.Cities) == 0 {
			return fmt.Errorf("zone %d (%s): zone requires at least one postal code or city", i+1, z.Name)
		}
		if z.Priority < 1 {
			return fmt.Errorf("zone %d (%s): priority must be >= 1", i+1, z.Name)
		}
		if z.IsActive {
			if priorities[z.Priority] {
				return fmt.Errorf("duplicate active zone priority: %d", z.Priority)
			}
			priorities[z.Priority] = true
		}
		if z.EstimatedTime <= 0 {
			return fmt.Errorf("zone %d (%s): estimated_time must be > 0", i+1, z.Name)
		}
		if z.DeliveryFee < 0 || z.MinimumOrderAmount < 0 {
			return fmt.Errorf("zone %d (%s): fee and minimum cannot be negative", i+1, z.Name)
		}
	}

	// Partner links presence.
	partnerCount, err := countPartnerLinks(json.RawMessage(settings.ExternalPartnerLinks))
	if err != nil {
		return fmt.Errorf("invalid external_partner_links: %w", err)
	}

	if settings.InHouseDeliveryEnabled && activeZoneCount == 0 {
		return fmt.Errorf("in_house_delivery_enabled requires at least one active zone")
	}
	if settings.ThirdPartyEnabled && partnerCount == 0 {
		return fmt.Errorf("third_party_enabled requires at least one external_partner_link")
	}
	// delivery_enabled with no fulfillment route is allowed as a "setup in
	// progress" state. The public-page gate (useBusinessPageData) will render
	// nothing until the merchant configures partners or zones, and the toggle
	// can flip the master flag without forcing the merchant to pre-configure.

	return nil
}

func (s *DeliveryService) activeDeliveryCount(db *gorm.DB, businessID uint) (int64, error) {
	var count int64
	// assigned, picked_up, in_transit, nearby, confirmed, preparing, and ready
	// always occupy a driver, including when the ETA has already passed. A
	// late in_transit order must not free capacity. Only pending keeps the
	// self-healing cutoff (R5): a stuck unpaid checkout stops counting once
	// its ETA is more than 45 minutes past, or, when no ETA was recorded,
	// once it is more than 3 hours old. The lifecycle service and expiry job
	// drive real orders to terminal states long before that cutoff; this is
	// the last-resort guard against one abandoned pending row bricking
	// delivery for the whole business.
	alwaysCount := []database.DeliveryStatus{
		database.DeliveryStatusAssigned,
		database.DeliveryStatusPickedUp,
		database.DeliveryStatusInTransit,
		database.DeliveryStatusNearby,
		database.DeliveryStatusConfirmed,
		database.DeliveryStatusPreparing,
		database.DeliveryStatusReady,
	}
	cutoff := time.Now().Add(-3 * time.Hour)
	etaGraceCutoff := time.Now().Add(-45 * time.Minute)
	err := db.Model(&database.DeliveryOrder{}).
		Where("business_id = ?", businessID).
		Where(
			"status IN ? OR (status = ? AND ((estimated_delivery_time IS NOT NULL AND estimated_delivery_time > ?) OR (estimated_delivery_time IS NULL AND created_at > ?)))",
			alwaysCount,
			database.DeliveryStatusPending,
			etaGraceCutoff,
			cutoff,
		).
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (s *DeliveryService) validateDeliveryWindow(business *database.Business, settings *database.DeliverySettings, quote *DeliveryQuoteDTO) error {
	return s.validateDeliveryWindowAt(business, settings, quote, time.Now())
}

// validateDeliveryWindowAt is the testable core of validateDeliveryWindow: it
// accepts an explicit "now" so unit tests can pin the clock without patching
// global state.
func (s *DeliveryService) validateDeliveryWindowAt(business *database.Business, settings *database.DeliverySettings, quote *DeliveryQuoteDTO, now time.Time) error {
	loc, err := time.LoadLocation(defaultString(business.Timezone, "UTC"))
	if err != nil {
		loc = time.UTC
	}
	now = now.In(loc)
	if settings.DeliveryHoursSameAsBusiness {
		// The after-midnight tail belongs to yesterday's overnight row. Reading
		// it off today's row accepts Friday 00:30 against a Friday 22:00–02:00
		// window, and rejects Sunday 01:30 when Saturday ran 22:00–02:00 and
		// Sunday is closed. A missing or unreadable yesterday is simply no tail.
		// Malformed yesterday rows are skipped; they must not fail the quote closed.
		yesterday := now.AddDate(0, 0, -1)
		if yesterdayPeriods, yErr := database.GetBusinessOperatingHoursByDay(business.ID, int(yesterday.Weekday())); yErr == nil {
			midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
			for _, hours := range yesterdayPeriods {
				if hours.IsClosed {
					continue
				}
				openTime, err := parseSimpleClock(yesterday, hours.OpenTime, loc)
				if err != nil {
					continue
				}
				closeTime, err := parseSimpleClock(yesterday, hours.CloseTime, loc)
				if err != nil {
					continue
				}
				if closeTime.After(openTime) {
					continue
				}
				// Tail ends at today's date on the close clock (close + 24h).
				// now is already on today's date, so it is not before midnight.
				tailEnd := closeTime.Add(24 * time.Hour)
				if !now.Before(midnight) && !now.After(tailEnd) {
					end := tailEnd
					quote.CutoffAt = &end
					return nil
				}
			}
		}

		periods, err := database.GetBusinessOperatingHoursByDay(business.ID, int(now.Weekday()))
		if err != nil {
			// Missing weekday row (partial seed, accidental delete) fails CLOSED.
			// Matches FE useBusinessOpenStatus (missing day → closed) and the
			// is_closed / after-hours honesty path (LOG-015 / FIND-012).
			return fmt.Errorf("delivery is currently unavailable")
		}
		// Split shifts: open if now falls inside ANY period. A closed row fails closed.
		// An overnight row covers [open, close+24h] only — the after-midnight
		// tail is yesterday's row, checked above.
		for _, hours := range periods {
			if hours.IsClosed {
				return fmt.Errorf("delivery is currently unavailable")
			}
			// DEL-SM-4: a malformed configured time fails CLOSED. Treating a parse
			// failure as "no window configured" silently accepted 3 a.m. orders for
			// a business whose operator believes delivery closes at 22:00.
			openTime, err := parseSimpleClock(now, hours.OpenTime, loc)
			if err != nil {
				return fmt.Errorf("delivery is currently unavailable")
			}
			closeTime, err := parseSimpleClock(now, hours.CloseTime, loc)
			if err != nil {
				return fmt.Errorf("delivery is currently unavailable")
			}
			if !closeTime.After(openTime) {
				closeTime = closeTime.Add(24 * time.Hour)
			}
			if !now.Before(openTime) && !now.After(closeTime) {
				quote.CutoffAt = &closeTime
				return nil
			}
		}
		return fmt.Errorf("delivery is outside active hours")
	}
	// Empty custom window fails CLOSED. Operators who uncheck "use business
	// hours" without setting start/end used to get unrestricted 24/7 delivery
	// (including on is_closed business days). Settings-save now rejects empty
	// custom windows; this guards legacy rows still missing times.
	if strings.TrimSpace(settings.DeliveryStartTime) == "" || strings.TrimSpace(settings.DeliveryEndTime) == "" {
		return fmt.Errorf("delivery is currently unavailable")
	}
	// DEL-SM-4: malformed configured hours fail CLOSED (see the same rule in
	// the business-hours branch above). The settings-save path rejects new
	// malformed windows; this guards legacy rows.
	startTime, err := parseSimpleClock(now, settings.DeliveryStartTime, loc)
	if err != nil {
		return fmt.Errorf("delivery is currently unavailable")
	}
	endTime, err := parseSimpleClock(now, settings.DeliveryEndTime, loc)
	if err != nil {
		return fmt.Errorf("delivery is currently unavailable")
	}
	overnight := !endTime.After(startTime)
	if overnight {
		// After-midnight tail: "now" is before today's start but before the
		// unadjusted end — the window from last night is still active.
		if now.Before(startTime) && !now.After(endTime) {
			quote.CutoffAt = &endTime
			return nil
		}
		endTime = endTime.Add(24 * time.Hour)
	}
	if now.Before(startTime) || now.After(endTime) {
		// DEL-SM-6 / R14: an ineligible outside-hours quote must not carry a
		// fabricated CutoffAt — set the cutoff only on the eligible path,
		// mirroring the business-hours branch.
		return fmt.Errorf("delivery is outside active hours")
	}
	quote.CutoffAt = &endTime
	return nil
}

func (s *DeliveryService) computeDeliveryFee(settings *database.DeliverySettings, zone *DeliveryZoneDTO, subtotal float64) float64 {
	baseFee := float64(settings.FlatDeliveryFee) / 100.0
	if zone != nil {
		// A matched zone fee is authoritative ("lowest fee wins"), including a
		// deliberately configured $0 free-delivery zone (validated as >= 0).
		// Guarding on `> 0` here silently billed the flat fee in free zones.
		baseFee = zone.DeliveryFee
	}
	freeMinimum := float64(settings.FreeDeliveryMinimum) / 100.0
	if freeMinimum > 0 && subtotal >= freeMinimum {
		return 0
	}
	return baseFee
}

func (s *DeliveryService) computeDeliveryMinutes(zone *DeliveryZoneDTO) int {
	if zone != nil && zone.EstimatedTime > 0 {
		return zone.EstimatedTime
	}
	return 30
}

func (s *DeliveryService) defaultZoneTravelMinutes(zones []DeliveryZoneDTO) int {
	if len(zones) == 0 {
		return 30
	}
	minutes := 0
	for _, zone := range zones {
		if zone.IsActive && (minutes == 0 || zone.EstimatedTime < minutes) {
			minutes = zone.EstimatedTime
		}
	}
	if minutes == 0 {
		return 30
	}
	return minutes
}

func normalizeJSONRaw(raw json.RawMessage, fallback string) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage(fallback)
	}
	return raw
}

func hasJSONArrayValues(raw json.RawMessage) bool {
	normalized := normalizeJSONRaw(raw, "[]")

	var values []json.RawMessage
	if err := json.Unmarshal(normalized, &values); err == nil {
		return len(values) > 0
	}

	return false
}

// parseClockParts parses a 24-hour "HH:MM" string with range validation
// (0-23 / 0-59). It is the single format authority shared by the quote-time
// window check (parseSimpleClock) and the settings-save validator, so a value
// that saves is guaranteed to parse at quote time (DEL-SM-4).
func parseClockParts(raw string) (hour, minute int, err error) {
	parts := strings.Split(strings.TrimSpace(raw), ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid time")
	}
	hour, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}
	minute, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, err
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("time out of range")
	}
	return hour, minute, nil
}

func parseSimpleClock(date time.Time, raw string, loc *time.Location) (time.Time, error) {
	hour, minute, err := parseClockParts(raw)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, loc), nil
}

// deliveryDisplayCurrency resolves the currency code guest-facing delivery
// copy should carry: display currency when set, else the business default,
// else USD (matching the platform-wide display-currency fallback).
func deliveryDisplayCurrency(business *database.Business) string {
	if code := strings.TrimSpace(business.DisplayCurrency); code != "" {
		return code
	}
	if code := strings.TrimSpace(business.DefaultCurrency); code != "" {
		return code
	}
	return "USD"
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func maxFloat(values ...float64) float64 {
	maxValue := 0.0
	for _, value := range values {
		if value > maxValue {
			maxValue = value
		}
	}
	return maxValue
}

// resolveDeliveryPaymentMode returns the effective payment mode: the stored
// value when set, otherwise derived from online-payment availability. A
// stored "online" with no online method falls back to COD so checkouts
// never dead-end (the settings UI flags the mismatch).
func resolveDeliveryPaymentMode(stored string, onlineAvailable bool) string {
	if stored == string(database.DeliveryPaymentOnline) && onlineAvailable {
		return string(database.DeliveryPaymentOnline)
	}
	if stored == string(database.DeliveryPaymentCashOnDelivery) {
		return string(database.DeliveryPaymentCashOnDelivery)
	}
	if onlineAvailable {
		return string(database.DeliveryPaymentOnline)
	}
	return string(database.DeliveryPaymentCashOnDelivery)
}

// EffectiveDeliveryPaymentMode resolves the guest-facing payment mode for a
// business without hydrating the settings DTO (business aggregate + zones +
// live counts). Used by the public tracking endpoint — a polled hot path that
// only needs the mode. Read errors degrade to "" stored mode, which
// resolveDeliveryPaymentMode derives from online-payment availability.
func (s *DeliveryService) EffectiveDeliveryPaymentMode(businessID uint) string {
	stored := ""
	var settings database.DeliverySettings
	if err := s.db.Select("payment_mode").Where("business_id = ?", businessID).First(&settings).Error; err == nil {
		stored = settings.PaymentMode
	}
	return resolveDeliveryPaymentMode(stored, s.businessHasOnlinePayments(businessID))
}

// businessHasOnlinePayments reports whether the business can take a guest
// payment online: a crypto settlement address, or any enabled payment-category
// plugin. (Plugin "configured" checks live at the handler layer; an enabled
// but unconfigured plugin over-counts here, which only affects the derived
// default — the guest pay page itself uses the real plugin endpoint.)
func (s *DeliveryService) businessHasOnlinePayments(businessID uint) bool {
	// Projected read: this also runs on the polled public tracking path, so do
	// not hydrate the business aggregate just to inspect the settlement address.
	var business database.Business
	if err := s.db.Select("settlement_addr").Where("id = ?", businessID).First(&business).Error; err == nil &&
		strings.TrimSpace(business.SettlementAddr) != "" {
		return true
	}
	has, err := database.HasEnabledPaymentPlugin(s.db, businessID)
	return err == nil && has
}
