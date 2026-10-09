package server

import (
	"errors"
	"net/http"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

var errPublicBusinessNotFound = errors.New("public business not found")
var getPublicBusinessByCustomURL = database.GetBusinessByCustomURL

// menuDataForBusiness is the test seam for services.MenuDataForBusiness.
// The guest menu/bill endpoints fetch the cached pricing snapshot
// (menu + categories + offers + bundles) through this var so unit tests
// can inject hand-crafted slices (e.g. an Offer whose embedded
// `Business` is densely populated) to exercise the JSON serializer
// end-to-end. The default points at the real services implementation;
// tests in the `server` package reassign it.
var menuDataForBusiness = services.MenuDataForBusiness

func loadPublicBusinessByCustomURL(customURL string) (*database.Business, error) {
	// services.BusinessByCustomURL caches the result of getPublicBusinessByCustomURL
	// for PricingCacheTTL — trims the last DB hit off the menu read path.
	// We pass the lookup function explicitly to avoid a database→services
	// import cycle, and to keep the test seam (getPublicBusinessByCustomURL
	// is overridden in unit tests).
	business, err := services.BusinessByCustomURL(customURL, getPublicBusinessByCustomURL)
	if err != nil {
		if errors.Is(err, database.ErrBusinessNotFound) {
			return nil, errPublicBusinessNotFound
		}
		return nil, err
	}
	return gatePublicBusiness(business)
}

// gatePublicBusiness applies the storefront publish gates: only published (business_page_enabled) AND active
// businesses are publicly resolvable.
func gatePublicBusiness(business *database.Business) (*database.Business, error) {
	if !business.BusinessPageEnabled || !business.IsActive {
		return nil, errPublicBusinessNotFound
	}
	return business, nil
}

func respondPublicBusinessLookupError(c *gin.Context, err error) {
	if errors.Is(err, errPublicBusinessNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load business"})
}

// publicBusinessProjection returns the non-sensitive business-row fields safe to
// expose to non-owners. It is the single source of truth for which Business fields
// are public; GetBusiness (non-owner branch) and GetBusinessByCustomURL both use it
// so the safe field set cannot drift. Separately-queried sub-data (gallery, hours,
// features, languages) is added by the caller, not here.
func publicBusinessProjection(business *database.Business) gin.H {
	return gin.H{
		"id":                     business.ID,
		"name":                   business.Name,
		"logo":                   business.Logo,
		"address":                business.Address,
		"description":            business.Description,
		"custom_url":             business.CustomURL,
		"phone":                  business.Phone,
		"website":                business.Website,
		"social_media":           business.SocialMedia,
		"banner_images":          business.BannerImages,
		"show_reviews":           business.ShowReviews,
		"google_reviews_enabled": business.GoogleReviewsEnabled,
		"google_place_id":        business.GooglePlaceID,
		"google_business_name":   business.GoogleBusinessName,
		"google_review_link":     business.GoogleReviewLink,
		"google_business_url":    business.GoogleBusinessURL,
		"default_currency":       business.DefaultCurrency,
		"display_currency":       business.DisplayCurrency,
		"default_language":       business.DefaultLanguage,
		"design_settings":        business.DesignSettings,
		"welcome_message":        business.WelcomeMessage,
		"about_story":            business.AboutStory,
		"show_welcome_message":   business.ShowWelcomeMessage,
		"show_about_story":       business.ShowAboutStory,
		"show_gallery":           business.ShowGallery,
		"show_operating_hours":   business.ShowOperatingHours,
		"show_special_features":  business.ShowSpecialFeatures,
		"timezone":               business.Timezone,
		"ai_settings": gin.H{
			"ai_enabled":               business.AiSettings.AiEnabled,
			"ai_name":                  business.AiSettings.AiName,
			"ai_priority":              business.AiSettings.AiPriority,
			"business_page_ai_enabled": business.AiSettings.BusinessPageAiEnabled,
		},
		"tax_rate":         business.TaxRate,
		"service_fee_rate": business.ServiceFeeRate,
		"created_at":       business.CreatedAt,
		"updated_at":       business.UpdatedAt,
		// Existing demo flags — public so /b pages can noindex seed venues
		// without guessing slugs. Non-sensitive classification.
		"is_demo": business.IsDemo,
		"kind":    business.Kind,
		// Authoritative guest AI gate. Operational + ai_enabled is the same
		// rule as QR/table. The storefront also
		// honours the operator's business-page opt-out, which is storefront
		// only (gorm default:false) and does not apply to the QR/table path.
		"ai_available": database.IsAIWaiterAvailable(business) &&
			business.AiSettings.BusinessPageAiEnabled,
		// "llm" when an LLM provider is configured, "basic" when the waiter
		// answers from the deterministic menu snapshot (no-key self-host).
		"ai_waiter_mode": aiWaiterMode(),
	}
}

// staffBusinessProjection returns business fields safe for staff members who
// are authenticated to this business but are NOT the owner. It extends the
// public projection with operational dashboard fields (feature toggles, QR
// defaults, counter config, coordinates) while still omitting owner wallets
// (settlement/tipping/owner address), owner email, owner_name and Stripe IDs.
// (RBAC-BIZRECORD-LEAK-01)
func staffBusinessProjection(business *database.Business) gin.H {
	result := publicBusinessProjection(business)
	result["business_id"] = business.BusinessId
	result["is_active"] = business.IsActive
	// closed_at is the other half of the admin lock; the dashboard derives its
	// read-only state from is_active + closed_at.
	result["closed_at"] = business.ClosedAt
	result["tax_inclusive"] = business.TaxInclusive
	result["service_inclusive"] = business.ServiceInclusive
	result["timezone"] = business.Timezone
	result["counter_enabled"] = business.CounterEnabled
	result["counter_count"] = business.CounterCount
	result["counter_prefix"] = business.CounterPrefix
	result["kitchen_enabled"] = business.KitchenEnabled
	result["orders_enabled"] = business.OrdersEnabled
	result["crm_enabled"] = business.CRMEnabled
	result["business_page_enabled"] = business.BusinessPageEnabled
	result["source_language"] = business.SourceLanguage
	result["latitude"] = business.Latitude
	result["longitude"] = business.Longitude
	result["is_demo"] = business.IsDemo
	// QR defaults.
	result["default_qr_logo_url"] = business.DefaultQRLogoURL
	result["default_qr_foreground_color"] = business.DefaultQRForegroundColor
	result["default_qr_background_color"] = business.DefaultQRBackgroundColor
	result["default_qr_logo_size"] = business.DefaultQRLogoSize
	result["default_qr_show_business_name"] = business.DefaultQRShowBusinessName
	result["default_qr_show_table_name"] = business.DefaultQRShowTableName
	result["default_qr_text_font"] = business.DefaultQRTextFont
	// Onboarding state (needed by the staff dashboard's OnboardingHub).
	if business.OnboardingCompletedAt != nil {
		result["onboarding_completed_at"] = business.OnboardingCompletedAt
	}
	return result
}
