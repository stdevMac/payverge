package server

import (
	"encoding/json"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

// guestKitchenOrdersAdvertised returns the kitchen/orders flags guests should
// see. A suspended or closed business keeps the published storefront but
// must not advertise orderable flows the mutation path will reject with 403.
func guestKitchenOrdersAdvertised(business *database.Business) (kitchen, orders bool) {
	if business == nil || !database.IsBusinessOperational(business) {
		return false, false
	}
	return business.KitchenEnabled, business.OrdersEnabled
}

// guestFallbackBusiness is the synthetic "no business row passed" stand-in
// the guest menu projections fall back to: orderable, and not admin-locked.
// IsActive must be set explicitly: Business.IsActive is a DB default (true),
// so a bare literal reads false, and is_active=false IS the
// admin-suspension lock — the fallback would project every dish as
// "ordering paused".
func guestFallbackBusiness(businessID uint) *database.Business {
	return &database.Business{ID: businessID, IsActive: true, KitchenEnabled: true, OrdersEnabled: true}
}

func buildPublicGuestTableResponse(table *database.Table) gin.H {
	return gin.H{
		"table_code": table.TableCode,
		"name":       table.Name,
		"capacity":   table.Capacity,
		"is_active":  table.IsActive,
	}
}

// buildPublicGuestMenuResponse keeps every configured dish visible and returns
// one authoritative orderability decision per item. Guests can therefore see
// what the restaurant offers while unavailable controls are disabled with a
// stable reason (manual, inventory, hours, or ordering toggle). A malformed
// menu blob still serves an empty array rather than breaking the guest page.
// publicGuestMenuMeta is the guest menu payload without the category tree.
// Callers that also emit a top-level "categories" key must use this so the
// tree is not serialized twice (#513).
func publicGuestMenuMeta(menu *database.Menu, businesses ...*database.Business) gin.H {
	pub := buildPublicGuestMenuResponse(menu, businesses...)
	delete(pub, "categories")
	return pub
}

func buildPublicGuestMenuResponse(menu *database.Menu, businesses ...*database.Business) gin.H {
	categories, _ := publicMenuCategories(menu.Categories)
	business := guestFallbackBusiness(menu.BusinessID)
	if len(businesses) > 0 && businesses[0] != nil {
		business = businesses[0]
	}
	projection := projectGuestOrderability(business, categories)
	encoded, err := json.Marshal(categories)
	if err != nil {
		encoded = []byte("[]")
	}
	return gin.H{"categories": json.RawMessage(encoded), "item_orderability": projection}
}

// projectGuestOrderability stamps catalog inventory_status in place (hours do
// not wipe it) and returns the guest item_orderability map.
func projectGuestOrderability(business *database.Business, categories []database.MenuCategory) map[string]services.Orderability {
	if business == nil {
		business = guestFallbackBusiness(0)
	}
	businessOpen := true
	if business.ID != 0 {
		businessOpen = services.BusinessOpenAt(database.GetDB(), business, time.Now())
	}
	projection := services.ProjectOrderability(
		business,
		categories,
		services.OrderabilityContextGuest,
		businessOpen,
	)
	applyGuestInventorySellability(categories, projection)
	return projection
}

// applyGuestInventorySellability flips guest catalog is_available off for
// inventory-86'd dishes. The guest card historically only read is_available +
// allergen tags, so a depleted recipe still rendered as a normal sellable dish
// (#728). Hours may remap item_orderability to business_closed;
// inventory_status survives that.
//
// The operator menu applies the same rule (#727) — one shared implementation so
// the two audiences cannot drift back apart. Guests never edit the manual flag,
// so it is not stamped here.
func applyGuestInventorySellability(
	categories []database.MenuCategory,
	projection map[string]services.Orderability,
) {
	services.ApplyInventorySellability(categories, projection, false)
}

func publicMenuCategories(raw string) ([]database.MenuCategory, json.RawMessage) {
	var categories []database.MenuCategory
	if err := json.Unmarshal([]byte(raw), &categories); err != nil {
		return []database.MenuCategory{}, json.RawMessage("[]")
	}
	encoded, err := json.Marshal(categories)
	if err != nil {
		return []database.MenuCategory{}, json.RawMessage("[]")
	}
	return categories, encoded
}

func publicBillCurrency(bill *database.Bill) string {
	if bill == nil {
		return "USD"
	}
	if bill.Currency != "" {
		return bill.Currency
	}
	if bill.Business.DisplayCurrency != "" {
		return bill.Business.DisplayCurrency
	}
	if bill.Business.DefaultCurrency != "" {
		return bill.Business.DefaultCurrency
	}
	return "USD"
}

func publicGuestBillRemainingDollars(bill *database.Bill) float64 {
	if bill == nil {
		return 0
	}
	remainingCents := bill.TotalAmount - bill.PaidAmount
	if remainingCents < 0 {
		remainingCents = 0
	}
	return float64(remainingCents) / 100.0
}

func buildPublicGuestBillResponse(bill *database.Bill) gin.H {
	return gin.H{
		"id":          bill.ID,
		"bill_number": bill.BillNumber,
		// The table code is already a guest capability. Return the independent,
		// unguessable bill capability so subsequent payment/split/detail routes
		// never fall back to the human-readable bill number.
		"public_token":       bill.PublicToken,
		"subtotal":           float64(bill.Subtotal) / 100.0,
		"tax_amount":         float64(bill.TaxAmount) / 100.0,
		"service_fee_amount": float64(bill.ServiceFeeAmount) / 100.0,
		"total_amount":       float64(bill.TotalAmount) / 100.0,
		"paid_amount":        float64(bill.PaidAmount) / 100.0,
		"remaining":          publicGuestBillRemainingDollars(bill),
		"tip_amount":         float64(bill.TipAmount) / 100.0,
		// Same cents→dollars contract as the other money fields above and as
		// operator Bill.MarshalJSON. Always present (0 when none) so the guest
		// breakdown reconciles after a loyalty redemption and reload.
		"loyalty_discount":   float64(bill.LoyaltyDiscountCents) / 100.0,
		"currency":           publicBillCurrency(bill),
		"status":             bill.Status,
		"settlement_address": bill.SettlementAddr,
		"tipping_address":    bill.TippingAddr,
		"created_at":         bill.CreatedAt,
		"updated_at":         bill.UpdatedAt,
		"closed_at":          bill.ClosedAt,
	}
}

func buildPublicGuestBusinessResponse(business *database.Business) gin.H {
	kitchenEnabled, ordersEnabled := guestKitchenOrdersAdvertised(business)
	resp := gin.H{
		"id":                     business.ID,
		"business_id":            business.BusinessId,
		"name":                   business.Name,
		"logo":                   business.Logo,
		"address":                business.Address,
		"description":            business.Description,
		"custom_url":             business.CustomURL,
		"phone":                  business.Phone,
		"website":                business.Website,
		"social_media":           business.SocialMedia,
		"default_currency":       business.DefaultCurrency,
		"display_currency":       business.DisplayCurrency,
		"default_language":       business.DefaultLanguage,
		"tax_rate":               business.TaxRate,
		"service_fee_rate":       business.ServiceFeeRate,
		"tax_inclusive":          business.TaxInclusive,
		"service_inclusive":      business.ServiceInclusive,
		"show_reviews":           business.ShowReviews,
		"google_reviews_enabled": business.GoogleReviewsEnabled,
		"google_place_id":        business.GooglePlaceID,
		"google_business_name":   business.GoogleBusinessName,
		"google_review_link":     business.GoogleReviewLink,
		"google_business_url":    business.GoogleBusinessURL,
		"timezone":               business.Timezone,
		"kitchen_enabled":        kitchenEnabled,
		"orders_enabled":         ordersEnabled,
		"crm_enabled":            business.CRMEnabled,
		"counter_enabled":        business.CounterEnabled,
		"ai_settings": gin.H{
			"ai_enabled":               business.AiSettings.AiEnabled,
			"ai_name":                  business.AiSettings.AiName,
			"ai_priority":              business.AiSettings.AiPriority,
			"business_page_ai_enabled": business.AiSettings.BusinessPageAiEnabled,
		},
		// Authoritative guest AI gate: operational business + AI toggle.
		// Computed from the already-loaded business, so no extra DB access.
		"ai_available": database.IsAIWaiterAvailable(business),
		// "llm" | "basic" (deterministic menu-snapshot fallback, no LLM key).
		"ai_waiter_mode": aiWaiterMode(),
	}
	// Additive weekly hours so guest OpenClosedPill can render without a
	// second fetch. Fail open (omit key) when none are configured or load fails.
	if business != nil && business.ID != 0 {
		if hours, err := database.GetBusinessOperatingHours(business.ID); err == nil && len(hours) > 0 {
			publicHours := make([]gin.H, 0, len(hours))
			for _, h := range hours {
				publicHours = append(publicHours, gin.H{
					"day_of_week": h.DayOfWeek,
					"open_time":   h.OpenTime,
					"close_time":  h.CloseTime,
					"is_closed":   h.IsClosed,
				})
			}
			resp["hours"] = publicHours
		}
	}
	return resp
}
