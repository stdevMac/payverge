package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/locales"
	"github.com/stdevmac/payverge/backend/internal/plugins"
	"github.com/stdevmac/payverge/backend/internal/services"
	operational_alerts "github.com/stdevmac/payverge/backend/internal/services/operational_alerts"

	"github.com/gin-gonic/gin"
)

func getActivePromotionsForBusiness(business *database.Business) ([]database.Offer, []database.Bundle) {
	if business == nil {
		return []database.Offer{}, []database.Bundle{}
	}

	offers, err := database.GetActiveOffersByBusinessIDAt(business.ID, database.ScheduleNow(), business.Timezone)
	if err != nil {
		log.Printf("Warning: Could not load active offers for business %d: %v", business.ID, err)
		offers = []database.Offer{}
	}

	bundles, err := database.GetActiveBundlesByBusinessID(business.ID)
	if err != nil {
		log.Printf("Warning: Could not load active bundles for business %d: %v", business.ID, err)
		bundles = []database.Bundle{}
	}

	return offers, bundles
}

// filterGuestOffers drops recurring offers that are outside OfferActiveAt in
// the business timezone. Applied at serve time so a pricing-cache snapshot
// loaded just before 15:00 cannot list a lunch-only deal at 15:14.
func filterGuestOffers(business *database.Business, offers []database.Offer) []database.Offer {
	if business == nil {
		return offers
	}
	return database.FilterOffersActiveAt(offers, database.ScheduleNow(), business.Timezone)
}

// guestOrderingEnabled gates GUEST ordering endpoints only (X-11, intentional):
// operator order routes ignore these flags so staff can always key in orders
// (phone orders, corrections) while guest self-ordering is switched off.
// ToggleKitchenAndOrders writes both flags together — they cannot diverge via
// the API even though the columns are separate.
func guestOrderingEnabled(business *database.Business) bool {
	return business != nil && business.KitchenEnabled && business.OrdersEnabled
}

func respondGuestOrderingDisabled(c *gin.Context) {
	c.JSON(http.StatusConflict, gin.H{
		"error": "Ordering is disabled for this business",
		"code":  services.OrderErrCodeOrderingDisabled,
	})
}

// Test seams: guest menu handlers schedule backfills through these so tests can
// count them without a translation provider.
var (
	scheduleGuestMenuTranslationBackfill      = scheduleMenuTranslationBackfill
	scheduleGuestPromotionTranslationBackfill = schedulePromotionTranslationBackfill
)

// resolveGuestMenuLanguage maps the anonymous ?language= value to the code the
// guest menu is served in. Only shipped guest locales are honoured
// (locales.IsGuestLocale, an exact registry match); anything else falls back to
// the venue default so a caller cannot mint unbounded translation keys.
// allowBackfill is true for any shipped guest locale other than the default:
// the guest picker offers the full storefront locale set (PG-14), so the paid
// backfill is bounded by the registry (one per venue and locale, deduped while
// in flight), not by the venue's configured business_languages rows.
func resolveGuestMenuLanguage(business *database.Business, requested string) (code string, defaultLanguage string, allowBackfill bool) {
	defaultLanguage = business.DefaultLanguage
	if defaultLanguage == "" {
		defaultLanguage = "en"
	}

	code = strings.TrimSpace(requested)
	if code == "" || !locales.IsGuestLocale(code) {
		code = defaultLanguage
	}
	return code, defaultLanguage, code != defaultLanguage
}

// GetTableByCodePublic retrieves table information by table code for guests
func GetTableByCodePublic(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Table code is required"})
		return
	}

	table, business, err := loadPublicGuestTableContext(code)
	if err != nil {
		respondPublicGuestTableLookupError(c, err)
		return
	}

	validatorKey := guestValidatorKey("table", code, strings.TrimSpace(c.Query("language")), business.DefaultLanguage)
	if answerGuestConditionalFromMemo(c, validatorKey, business, guestTableCacheControl) {
		emitGuestTableScanned(business.BusinessId, table.ID)
		return
	}

	// Get menu and promotion inputs through the same short-lived snapshot used
	// by the menu/order pricing paths; QR table loads are read-heavy.
	menu, categories, offers, bundles, err := menuDataForBusiness(table.BusinessID)
	if err != nil {
		// Menu might not exist yet, return empty menu
		menu = &database.Menu{
			BusinessID: table.BusinessID,
			Categories: "[]",
		}
		categories = []database.MenuCategory{}
		offers = []database.Offer{}
		bundles = []database.Bundle{}
	}
	offers = filterGuestOffers(business, offers)
	menuPayload, categories, offers, bundles := applyGuestLivePromotions(business, menu, categories, offers, bundles)

	languageCode, businessDefaultLanguage, allowBackfill := resolveGuestMenuLanguage(business, c.Query("language"))
	incomplete := false
	if languageCode != businessDefaultLanguage {
		translatedCategories, translatedOffers, translatedBundles, missingMenuTranslations, missingPromotionTranslations, _, translationErr :=
			translateGuestMenuForLanguage(business.ID, categories, offers, bundles, languageCode)
		if translationErr != nil {
			// Untranslated fallback: serve it, but never let it stick.
			incomplete = true
		} else {
			categories = translatedCategories
			offers = translatedOffers
			bundles = translatedBundles
			if allowBackfill && missingMenuTranslations {
				scheduleGuestMenuTranslationBackfill(business.ID, languageCode)
			}
			if allowBackfill && missingPromotionTranslations {
				scheduleGuestPromotionTranslationBackfill(business.ID, businessDefaultLanguage, languageCode, offers, bundles)
			}
			incomplete = missingMenuTranslations || missingPromotionTranslations
		}
	}

	writeGuestTableResponse(c, validatorKey, business, gin.H{
		"table":      buildPublicGuestTableResponse(table),
		"business":   buildPublicGuestBusinessResponse(business),
		"menu":       menuPayload,
		"categories": categories,
		"offers":     publicGuestOffers(offers),
		"bundles":    bundles,
	}, incomplete)

	// P2-19: activation instrumentation — throttled, async, zero extra queries
	// (business.BusinessId is already in the cached narrow projection).
	emitGuestTableScanned(business.BusinessId, table.ID)
}

// GetOpenBillByTableCode retrieves the active bill for a table by table code.
func GetOpenBillByTableCode(c *gin.Context) {
	applyGuestBillNoStore(c)
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Table code is required"})
		return
	}

	table, _, err := loadPublicGuestTableContext(code)
	if err != nil {
		respondPublicGuestTableLookupError(c, err)
		return
	}

	// Get active open or partially paid bill for this table.
	bill, items, err := database.GetPublicGuestOpenBillByTableID(table.ID)
	if err != nil {
		if errors.Is(err, database.ErrNoActiveBill) {
			// H3: guests poll this on every page load / ~10s tick. A missing
			// active bill is the normal empty state, not an error — return a
			// benign 200 so guest browsers never log a 404 per pageview.
			c.JSON(http.StatusOK, gin.H{"bill": nil, "items": []any{}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check for active bill"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bill":  buildPublicGuestBillResponse(bill),
		"items": items,
	})
}

// GetBusinessByTableCode retrieves business information by table code
func GetBusinessByTableCode(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Table code is required"})
		return
	}

	table, business, err := loadPublicGuestTableContext(code)
	if err != nil {
		respondPublicGuestTableLookupError(c, err)
		return
	}

	extras, err := services.CachedPublicGuestBusinessExtras(table.BusinessID, loadPublicGuestBusinessExtrasUncached)
	if err != nil {
		log.Printf("Warning: Could not load guest business metadata for business %d: %v", table.BusinessID, err)
		extras = services.PublicGuestBusinessExtras{}
	}

	c.JSON(http.StatusOK, gin.H{
		"business":              buildPublicGuestBusinessResponse(business),
		"business_languages":    extras.BusinessLanguages,
		"supported_languages":   extras.SupportedLanguages,
		"trustpilot_enabled":    extras.TrustpilotEnabled,
		"trustpilot_review_url": extras.TrustpilotReviewURL,
	})
}

func loadPublicGuestBusinessExtrasUncached(businessID uint) (services.PublicGuestBusinessExtras, error) {
	extras := services.PublicGuestBusinessExtras{}

	db := database.GetDBWrapper()
	businessLanguages, err := db.LanguageService.GetBusinessLanguages(businessID)
	if err != nil {
		log.Printf("Warning: Could not load business languages for business %d: %v", businessID, err)
	} else {
		extras.BusinessLanguages = businessLanguages
	}

	supportedLanguages, err := db.LanguageService.GetSupportedLanguages()
	if err != nil {
		log.Printf("Warning: Could not load supported languages: %v", err)
	} else {
		extras.SupportedLanguages = supportedLanguages
	}

	if enabled, err := database.IsPluginEnabledForBusiness(businessID, "trustpilot"); err == nil && enabled {
		extras.TrustpilotEnabled = true
		if trustpilotPlugin, exists := plugins.GlobalRegistry.GetPlugin("trustpilot"); exists {
			if tp, ok := trustpilotPlugin.(interface {
				GenerateReviewLink(uint) (string, error)
			}); ok {
				if reviewURL, err := tp.GenerateReviewLink(businessID); err == nil {
					extras.TrustpilotReviewURL = reviewURL
				}
			}
		}
	}

	return extras, nil
}

// CreateBillByTableCode allows guests to create a new bill for their table
func CreateBillByTableCode(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Table code is required"})
		return
	}

	// Get table information
	table, business, err := loadPublicGuestTableContext(code)
	if err != nil {
		respondPublicGuestTableLookupError(c, err)
		return
	}

	// Reject orders against suspended or closed businesses so a
	// scraped QR code cannot keep driving bills on a lapsed account.
	if !database.IsBusinessOperational(business) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "This business is not currently accepting orders",
			"code":  services.OrderErrCodeBusinessUnavailable,
		})
		return
	}

	if !guestOrderingEnabled(business) {
		respondGuestOrderingDisabled(c)
		return
	}

	// Check if there's already an active open or partially paid bill for this
	// table without hydrating the existing bill aggregate.
	hasActiveBill, err := database.HasActiveBillForTableID(table.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check for active bill"})
		return
	}
	if hasActiveBill {
		c.JSON(http.StatusConflict, gin.H{"error": "Table already has an open bill"})
		return
	}

	// Snapshot the owner settlement/tipping wallets onto the new bill so guest
	// USDC settlement has a destination. The QR table context is loaded via
	// publicBusinessColumns, which now includes these wallet columns precisely
	// because this path needs them — without them the guest bill is born
	// walletless and guest crypto-payment later fails 503 "not configured a
	// settlement wallet" even when the business has one configured.
	bill := &database.Bill{
		BusinessID:       table.BusinessID,
		TableID:          table.ID,
		BillNumber:       "", // Will be auto-generated
		Subtotal:         0,
		TaxAmount:        0,
		ServiceFeeAmount: 0,
		TotalAmount:      0,
		PaidAmount:       0,
		TipAmount:        0,
		Status:           database.BillStatusOpen,
		SettlementAddr:   business.SettlementAddr,
		TippingAddr:      business.TippingAddr,
		// Bill.Currency is gorm:"-", so a bill built in memory carries whatever
		// the constructor puts there — nothing. Without this stamp the 201 an
		// ARS venue serves on the first QR scan of an empty table claims USD,
		// because publicBillCurrency finds neither bill.Currency nor a
		// preloaded Business and falls through to its default (#852).
		Currency: database.BusinessDisplayCurrency(business),
	}

	err = database.CreateBill(bill, []database.BillItem{})
	if err != nil {
		// Lost the race: another concurrent scan of the same table created the
		// active bill between our pre-check and insert. The partial unique index
		// rejects the duplicate; surface the same 409 as the pre-check.
		if errors.Is(err, database.ErrActiveBillExists) {
			c.JSON(http.StatusConflict, gin.H{"error": "Table already has an open bill"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create bill"})
		return
	}

	if customer, ok := OptionalCustomerFromRequest(c); ok {
		attached, attachErr := database.AttachCRMCustomerIDToBillIfEmpty(bill.ID, customer.ID)
		if attachErr != nil {
			log.Printf("Warning: Could not attach CRM customer %d to guest bill %d: %v", customer.ID, bill.ID, attachErr)
		} else if attached != nil {
			bill.CRMCustomerID = attached.CRMCustomerID
		}
	}

	billResponse := buildPublicGuestBillResponse(bill)
	if bill.CRMCustomerID != nil {
		billResponse["crm_customer_id"] = *bill.CRMCustomerID
	}

	c.JSON(http.StatusCreated, gin.H{
		"bill":  billResponse,
		"items": []database.BillItem{}, // Empty items array
	})
}

// GetMenuByTableCode retrieves menu information by table code with optional language translation
func GetMenuByTableCode(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Table code is required"})
		return
	}

	table, business, err := loadPublicGuestTableContext(code)
	if err != nil {
		respondPublicGuestTableLookupError(c, err)
		return
	}

	languageCode, businessDefaultLanguage, allowBackfill := resolveGuestMenuLanguage(business, c.Query("language"))

	validatorKey := guestValidatorKey("menu-table", code, languageCode, businessDefaultLanguage)
	if answerGuestConditionalFromMemo(c, validatorKey, business, guestMenuCacheControl) {
		return
	}

	menu, categories, offers, bundles, err := menuDataForBusiness(table.BusinessID)
	if err != nil {
		// Menu might not exist yet, return empty menu
		menu = &database.Menu{
			BusinessID: table.BusinessID,
			Categories: "[]",
		}
		categories = []database.MenuCategory{}
		offers = []database.Offer{}
		bundles = []database.Bundle{}
	}
	offers = filterGuestOffers(business, offers)
	menuPayload, categories, offers, bundles := applyGuestLivePromotions(business, menu, categories, offers, bundles)

	// Apply translations if requested language is different from business default language
	if languageCode != businessDefaultLanguage {
		translatedCategories, translatedOffers, translatedBundles, missingMenuTranslations, missingPromotionTranslations, translationRevision, translationErr :=
			translateGuestMenuForLanguage(business.ID, categories, offers, bundles, languageCode)
		if translationErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load menu translations"})
			return
		}
		if allowBackfill && missingMenuTranslations {
			scheduleGuestMenuTranslationBackfill(business.ID, languageCode)
		}
		if allowBackfill && missingPromotionTranslations {
			scheduleGuestPromotionTranslationBackfill(business.ID, businessDefaultLanguage, languageCode, offers, bundles)
		}
		incomplete := missingMenuTranslations || missingPromotionTranslations
		etag := guestMenuCacheValidator(business, menu, languageCode, translationRevision, offers, bundles, translatedCategories, menuPayload)
		if applyRememberedGuestMenuCaching(c, validatorKey, business, etag, incomplete) {
			return
		}
		c.JSON(http.StatusOK, buildTranslatedMenuResponse(menu, translatedCategories, translatedOffers, translatedBundles, languageCode, business))
		return
	}

	etag := guestMenuCacheValidator(business, menu, languageCode, "", offers, bundles, categories, menuPayload)
	if applyRememberedGuestMenuCaching(c, validatorKey, business, etag, false) {
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"menu":       menuPayload,
		"categories": categories,
		"offers":     publicGuestOffers(offers),
		"bundles":    bundles,
	})
}

// GetBillByNumberPublic retrieves a bill by its public_token capability (public guest endpoint).
func GetBillByNumberPublic(c *gin.Context) {
	applyGuestBillNoStore(c)
	token := strings.TrimSpace(c.Param("bill_token"))
	if token == "" {
		// Empty/blank capability → canonical not-found envelope (no enumeration).
		respondPublicGuestBillLookupError(c, errPublicGuestBillNotFound)
		return
	}

	bill, items, err := loadPublicGuestBillByToken(token)
	if err != nil {
		respondPublicGuestBillLookupError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bill":  buildPublicGuestBillResponse(bill),
		"items": items,
	})
}

// GetMenuByBusinessCustomUrl retrieves menu information by business custom URL with optional language translation
func GetMenuByBusinessCustomUrl(c *gin.Context) {
	c.Set("perf_flow", "menu_browse")
	customUrl := c.Param("customUrl")
	if customUrl == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Business custom URL is required"})
		return
	}

	// Get public business by custom URL
	business, err := loadPublicBusinessByCustomURL(customUrl)
	if err != nil {
		respondPublicBusinessLookupError(c, err)
		return
	}

	languageCode, businessDefaultLanguage, allowBackfill := resolveGuestMenuLanguage(business, c.Query("language"))

	validatorKey := guestValidatorKey("menu-custom-url", customUrl, languageCode, businessDefaultLanguage)
	if answerGuestConditionalFromMemo(c, validatorKey, business, guestMenuCacheControl) {
		return
	}

	// Pull menu + offers + bundles from the in-process pricing cache. This
	// collapses three DB roundtrips (GetMenuByBusinessID, GetActiveOffersByBusinessIDAt,
	// GetActiveBundlesByBusinessID) into a single cached snapshot when the
	// same business is hit repeatedly — exactly the menu-browse traffic
	// pattern under load. Cache misses fall back to fresh DB reads.
	menu, categories, offers, bundles, err := menuDataForBusiness(business.ID)
	if err != nil {
		// Should not happen — MenuDataForBusiness already handles
		// missing-menu by returning an empty Menu. Treat any unexpected
		// error as an empty menu so guests still see the page.
		menu = &database.Menu{
			BusinessID: business.ID,
			Categories: "[]",
		}
		categories = []database.MenuCategory{}
		offers = []database.Offer{}
		bundles = []database.Bundle{}
	}
	offers = filterGuestOffers(business, offers)
	menuPayload, categories, offers, bundles := applyGuestLivePromotions(business, menu, categories, offers, bundles)

	// Apply translations if requested language is different from business default language
	if languageCode != businessDefaultLanguage {
		translatedCategories, translatedOffers, translatedBundles, missingMenuTranslations, missingPromotionTranslations, translationRevision, translationErr :=
			translateGuestMenuForLanguage(business.ID, categories, offers, bundles, languageCode)
		if translationErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load menu translations"})
			return
		}
		if allowBackfill && missingMenuTranslations {
			scheduleGuestMenuTranslationBackfill(business.ID, languageCode)
		}
		if allowBackfill && missingPromotionTranslations {
			scheduleGuestPromotionTranslationBackfill(business.ID, businessDefaultLanguage, languageCode, offers, bundles)
		}
		incomplete := missingMenuTranslations || missingPromotionTranslations
		etag := guestMenuCacheValidator(business, menu, languageCode, translationRevision, offers, bundles, translatedCategories, menuPayload)
		if applyRememberedGuestMenuCaching(c, validatorKey, business, etag, incomplete) {
			return
		}
		c.JSON(http.StatusOK, buildTranslatedMenuResponse(menu, translatedCategories, translatedOffers, translatedBundles, languageCode, business))
		return
	}

	etag := guestMenuCacheValidator(business, menu, languageCode, "", offers, bundles, categories, menuPayload)
	if applyRememberedGuestMenuCaching(c, validatorKey, business, etag, false) {
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"menu":       menuPayload,
		"categories": categories,
		"offers":     publicGuestOffers(offers),
		"bundles":    bundles,
	})
}

// guestOrderItemRequest is one requested line on a guest order. Price is
// gte=0 (not required): zero-price comp items are valid and the server
// re-prices from the menu regardless (G-8).
type guestOrderItemRequest struct {
	MenuItemName    string                    `json:"menu_item_name" binding:"required"`
	MenuItemID      string                    `json:"menu_item_id"`
	Quantity        int                       `json:"quantity" binding:"required"`
	Price           float64                   `json:"price" binding:"gte=0"`
	Options         []database.MenuItemOption `json:"options"` // Add-ons/modifiers
	SpecialRequests string                    `json:"special_requests"`
	ItemType        string                    `json:"item_type"` // menu_item|bundle|bundle_item|discount
	BundleID        *uint                     `json:"bundle_id"`
	ParentBundleID  *uint                     `json:"parent_bundle_id"`
	SourceOfferID   *uint                     `json:"source_offer_id"`
}

type guestCreateOrderRequest struct {
	BillID    uint                    `json:"bill_id"`
	Items     []guestOrderItemRequest `json:"items" binding:"required"`
	Notes     string                  `json:"notes"`
	PromoCode string                  `json:"promo_code"`
}

// CreateGuestOrder creates an order from a guest (public endpoint)
func CreateGuestOrder(c *gin.Context) {
	tableCode := c.Param("code")

	// Get table by code
	_, business, err := loadPublicGuestTableContext(tableCode)
	if err != nil {
		respondPublicGuestTableLookupError(c, err)
		return
	}

	// Reject orders against suspended or closed businesses.
	if !database.IsBusinessOperational(business) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "This business is not currently accepting orders",
			"code":  services.OrderErrCodeBusinessUnavailable,
		})
		return
	}

	if !guestOrderingEnabled(business) {
		respondGuestOrderingDisabled(c)
		return
	}

	// Parse request body
	var req guestCreateOrderRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	if utf8.RuneCountInString(req.Notes) > services.MaxOrderTextLen {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("Notes must be %d characters or fewer", services.MaxOrderTextLen),
			"code":  services.OrderErrCodeTextTooLong,
		})
		return
	}

	// Atomic checkout supersedes the legacy two-step bill/order path below.
	// Keep this block scoped while the old path is removed after all callers
	// have migrated; request identity is intentionally the client-supplied
	// header, not the middleware-generated trace id.
	{
		requestID, ok := guestOrderRequestID(c)
		if !ok {
			return
		}
		var requestedBillID *uint
		if req.BillID != 0 {
			requestedBillID = &req.BillID
		}
		input := services.GuestCheckoutInput{
			TableCode: tableCode, BillID: requestedBillID,
			ClientRequestID: requestID, Notes: req.Notes, PromoCode: req.PromoCode,
			Items: make([]services.PromotionInputLine, 0, len(req.Items)),
		}
		for _, item := range req.Items {
			input.Items = append(input.Items, services.PromotionInputLine{
				Name: item.MenuItemName, MenuItemID: item.MenuItemID, Quantity: item.Quantity,
				UnitPrice: item.Price, Options: item.Options, SpecialRequests: item.SpecialRequests,
				ItemType: item.ItemType, BundleID: item.BundleID, ParentBundleID: item.ParentBundleID,
				SourceOfferID: item.SourceOfferID,
			})
		}
		result, checkoutErr := services.NewGuestCheckoutService(database.GetDB()).Checkout(c.Request.Context(), input)
		if checkoutErr != nil {
			var replayConflict *services.GuestCheckoutReplayConflictError
			if errors.As(checkoutErr, &replayConflict) {
				c.JSON(http.StatusConflict, gin.H{
					"code":    services.GuestCheckoutReplayConflictCode,
					"error":   "This request was already used for another table",
					"message": "This request was already used for another table",
				})
				return
			}
			var unavailable *services.ItemNotOrderableError
			if errors.As(checkoutErr, &unavailable) {
				c.JSON(http.StatusConflict, gin.H{
					"code":       "item_not_orderable",
					"error":      "One or more items are unavailable",
					"message":    "One or more items are unavailable",
					"request_id": requestID,
					"details":    gin.H{"items": unavailable.Items},
				})
				return
			}
			if strings.Contains(strings.ToLower(checkoutErr.Error()), "bill is not open") {
				c.JSON(http.StatusConflict, gin.H{
					"code":  services.OrderErrCodeBillNotOpen,
					"error": "Bill is not open for new orders",
				})
				return
			}
			respondGuestOrderValidationError(c, checkoutErr)
			return
		}

		if !result.Replay {
			_ = events.NotifyTableBillChangedByBillID(database.GetDB(), result.Bill.ID)
			if herr := database.HydrateOrderSnapshotForWire(&result.Order); herr != nil {
				log.Printf("guest order.created snapshot hydrate failed: order_id=%d error=%v", result.Order.ID, herr)
				result.Order.Items = ""
			}
			orderBytes, marshalErr := json.Marshal(result.Order)
			if marshalErr == nil {
				events.GetHub().Publish(events.BusinessEvent{
					BusinessID: result.Order.BusinessID, Type: "order.created", Data: orderBytes, Timestamp: time.Now(),
				})
			}
			if alertErr := operational_alerts.NewService(database.GetDB()).CreateOrderNewAlert(c.Request.Context(), result.Order); alertErr != nil {
				log.Printf("failed to create guest order operational alert: business_id=%d order_id=%d error=%v", result.Order.BusinessID, result.Order.ID, alertErr)
			}
			emitGuestOrderPlaced(business.BusinessId, len(result.Quote.Lines))
		}

		// GuestCheckoutService loads the bill with a plain read on both the
		// fresh and the replay path, and Bill.Currency is gorm:"-", so the
		// serialized bill would claim USD on an ARS venue. The business this
		// route already resolved owns the bill by construction (checkout
		// rejects a bill from another business), so stamp the label here
		// rather than widening the checkout read (#852).
		if result.Bill.Currency == "" {
			result.Bill.Currency = database.BusinessDisplayCurrency(business)
		}

		status := http.StatusCreated
		if result.Replay {
			status = http.StatusOK
		}
		c.JSON(status, gin.H{
			"bill":      buildPublicGuestBillResponse(&result.Bill),
			"order":     result.Order,
			"quote":     serializeOrderQuote(services.OrderQuoteProjection{Quote: result.Quote}),
			"replay":    result.Replay,
			"duplicate": result.Replay,
		})
		return
	}

}
