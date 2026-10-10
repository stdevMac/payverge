package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

type OfferRequest struct {
	Name          string  `json:"name" binding:"required"`
	Description   string  `json:"description"`
	Image         string  `json:"image"`
	DiscountType  string  `json:"discount_type" binding:"required"`
	DiscountValue float64 `json:"discount_value" binding:"required"`
	StartDate     *string `json:"start_date"`
	EndDate       *string `json:"end_date"`
	WeekdayMask   *int16  `json:"weekday_mask"`
	StartMinute   *int    `json:"start_minute"`
	EndMinute     *int    `json:"end_minute"`
	IsActive      *bool   `json:"is_active"`
	ApplicableTo  string  `json:"applicable_to"`
	TargetID      *string `json:"target_id"`
}

var errInvalidOfferSchedule = errors.New("invalid offer schedule")

func validateOfferSchedule(req OfferRequest) error {
	if req.WeekdayMask != nil && (*req.WeekdayMask < 1 || *req.WeekdayMask > 127) {
		return fmt.Errorf("%w: weekday_mask must be between 1 and 127", errInvalidOfferSchedule)
	}
	if (req.StartMinute == nil) != (req.EndMinute == nil) {
		return fmt.Errorf("%w: start_minute and end_minute must be provided together", errInvalidOfferSchedule)
	}
	for field, minute := range map[string]*int{"start_minute": req.StartMinute, "end_minute": req.EndMinute} {
		if minute != nil && (*minute < 0 || *minute > 1439) {
			return fmt.Errorf("%w: %s must be between 0 and 1439", errInvalidOfferSchedule, field)
		}
	}
	return nil
}

func writeOfferValidationError(c *gin.Context, err error) {
	if errors.Is(err, errInvalidOfferSchedule) {
		// Schedule validation is hand-written product copy (wraps errInvalidOfferSchedule).
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_offer_schedule", "error": err.Error()})
		return
	}
	// Defensive: validateOfferRequest returns domain strings only today, but if a
	// future path wraps a driver failure the client still must not see it (CO-1).
	c.JSON(http.StatusBadRequest, gin.H{"error": LogAndClientSafeErrorMessage("offer validation", err)})
}

type BundleRequest struct {
	Name        string          `json:"name" binding:"required"`
	Description string          `json:"description"`
	Price       float64         `json:"price" binding:"required"`
	Currency    string          `json:"currency"`
	Image       string          `json:"image"`
	Items       json.RawMessage `json:"items" binding:"required"`
	IsActive    *bool           `json:"is_active"`
}

// errBundleCurrencyMismatch is returned when a write tries to persist a
// bundle denomination other than the business default. Bundle.Price is majors
// in DefaultCurrency (same as menu items); Currency is a stamp, not a source.
var errBundleCurrencyMismatch = errors.New("bundle currency must match the business default currency")

func resolveBundleCurrency(defaultCurrency, requested string) (string, error) {
	def := strings.ToUpper(strings.TrimSpace(defaultCurrency))
	if def == "" {
		def = "USD"
	}
	req := strings.ToUpper(strings.TrimSpace(requested))
	if req == "" || req == def {
		return def, nil
	}
	return "", errBundleCurrencyMismatch
}

func parseBusinessIDParam(c *gin.Context) (uint, error) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		return 0, errors.New("invalid business ID")
	}
	return uint(businessID), nil
}

func parseUintParam(c *gin.Context, key string) (uint, error) {
	value := c.Param(key)
	parsed, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid %s", key)
	}
	return uint(parsed), nil
}

func parseOptionalDate(dateStr *string) (*time.Time, error) {
	if dateStr == nil || strings.TrimSpace(*dateStr) == "" {
		return nil, nil
	}

	candidate := strings.TrimSpace(*dateStr)
	layouts := []string{time.RFC3339, "2006-01-02"}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, candidate); err == nil {
			return &parsed, nil
		}
	}

	return nil, fmt.Errorf("invalid date format: %s", candidate)
}

func normalizeApplicableTo(value string) string {
	normalized := strings.TrimSpace(strings.ToLower(value))
	if normalized == "" {
		return "all"
	}
	return normalized
}

func validateOfferRequest(req OfferRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return errors.New("name is required")
	}

	if req.DiscountType != "percentage" && req.DiscountType != "fixed" {
		return errors.New("discount_type must be 'percentage' or 'fixed'")
	}
	if req.DiscountValue <= 0 {
		return errors.New("discount_value must be greater than 0")
	}
	if req.DiscountType == "percentage" && req.DiscountValue > 100 {
		return errors.New("percentage discounts cannot exceed 100")
	}

	applicableTo := normalizeApplicableTo(req.ApplicableTo)
	switch applicableTo {
	case "all", "category", "item", "bundle":
	default:
		return errors.New("applicable_to must be one of: all, category, item, bundle")
	}

	if applicableTo != "all" && (req.TargetID == nil || strings.TrimSpace(*req.TargetID) == "") {
		return errors.New("target_id is required when applicable_to is not 'all'")
	}

	startDate, err := parseOptionalDate(req.StartDate)
	if err != nil {
		return err
	}
	endDate, err := parseOptionalDate(req.EndDate)
	if err != nil {
		return err
	}
	if startDate != nil && endDate != nil && startDate.After(*endDate) {
		return errors.New("start_date must be before or equal to end_date")
	}
	if err := validateOfferSchedule(req); err != nil {
		return err
	}

	return nil
}

func parseBundleItems(raw json.RawMessage) ([]database.BundleItemRef, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, errors.New("items is required")
	}

	// Support legacy string payloads where "items" is a JSON-encoded string.
	if strings.HasPrefix(trimmed, "\"") {
		var encoded string
		if err := json.Unmarshal(raw, &encoded); err != nil {
			return nil, errors.New("invalid items payload")
		}
		trimmed = strings.TrimSpace(encoded)
	}

	var refs []database.BundleItemRef
	if err := json.Unmarshal([]byte(trimmed), &refs); err == nil {
		for i := range refs {
			if refs[i].Quantity <= 0 {
				refs[i].Quantity = 1
			}
		}
		return refs, nil
	}

	// Support legacy array of IDs.
	var ids []string
	if err := json.Unmarshal([]byte(trimmed), &ids); err == nil {
		converted := make([]database.BundleItemRef, 0, len(ids))
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			converted = append(converted, database.BundleItemRef{
				MenuItemID: id,
				Quantity:   1,
			})
		}
		return converted, nil
	}

	return nil, errors.New("items must be an array of bundle item refs or menu item IDs")
}

// validateBundleItemsWithTotal normalizes refs and returns the sum of
// component menu prices (× quantity). Used by Create/Update to reject
// anti-deals where bundle price is at or above the à-la-carte total (L3-18).
func validateBundleItemsWithTotal(businessID uint, refs []database.BundleItemRef) ([]database.BundleItemRef, float64, error) {
	if len(refs) == 0 {
		return nil, 0, errors.New("bundle must include at least one item")
	}

	_, categories, err := database.GetMenuByBusinessID(businessID)
	if err != nil {
		return nil, 0, errors.New("failed to load business menu for bundle validation")
	}

	type menuLookup struct {
		ID    string
		Name  string
		Price float64
	}
	byID := map[string]menuLookup{}
	byName := map[string]menuLookup{}
	for _, category := range categories {
		for _, item := range category.Items {
			normalizedName := strings.ToLower(strings.TrimSpace(item.Name))
			lookup := menuLookup{ID: item.ID, Name: item.Name, Price: item.Price}
			if item.ID != "" {
				byID[item.ID] = lookup
			}
			if normalizedName != "" {
				byName[normalizedName] = lookup
			}
		}
	}

	normalized := make([]database.BundleItemRef, 0, len(refs))
	var regularTotal float64
	for _, ref := range refs {
		ref.MenuItemID = strings.TrimSpace(ref.MenuItemID)
		ref.Name = strings.TrimSpace(ref.Name)
		if ref.Quantity <= 0 {
			ref.Quantity = 1
		}

		var matched menuLookup
		var found bool
		if ref.MenuItemID != "" {
			matched, found = byID[ref.MenuItemID]
		}
		if !found && ref.Name != "" {
			matched, found = byName[strings.ToLower(ref.Name)]
		}
		if !found {
			return nil, 0, fmt.Errorf("bundle item '%s' is not a valid menu item", ref.Name)
		}
		if ref.MenuItemID == "" {
			ref.MenuItemID = matched.ID
		}
		if ref.Name == "" {
			ref.Name = matched.Name
		}
		regularTotal += matched.Price * float64(ref.Quantity)
		normalized = append(normalized, ref)
	}

	return normalized, regularTotal, nil
}

// errBundleAntiDeal is returned when a bundle is priced at or above the sum of
// its component menu prices (L3-18). A deal must save the guest money.
func errBundleAntiDeal(price, regularTotal float64) error {
	return fmt.Errorf(
		"bundle price (%.2f) must be less than the sum of component prices (%.2f)",
		price,
		regularTotal,
	)
}

func rejectAntiDealBundlePrice(price, regularTotal float64) error {
	// Only enforce when we have a positive component total; free/zero-priced
	// components would make any positive bundle price look like an anti-deal.
	if regularTotal > 0 && price >= regularTotal {
		return errBundleAntiDeal(price, regularTotal)
	}
	return nil
}

// --- Offers Handlers ---

func CreateOffer(c *gin.Context) {
	var req OfferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	if err := validateOfferRequest(req); err != nil {
		writeOfferValidationError(c, err)
		return
	}
	if refuseDemoImage(c, req.Image, "") {
		return
	}

	businessID, err := parseBusinessIDParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	startDate, _ := parseOptionalDate(req.StartDate)
	endDate, _ := parseOptionalDate(req.EndDate)
	applicableTo := normalizeApplicableTo(req.ApplicableTo)

	var targetID *string
	if req.TargetID != nil && strings.TrimSpace(*req.TargetID) != "" {
		value := strings.TrimSpace(*req.TargetID)
		targetID = &value
	}
	if applicableTo == "all" {
		targetID = nil
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	weekdayMask := int16(127)
	if req.WeekdayMask != nil {
		weekdayMask = *req.WeekdayMask
	}

	offer := database.Offer{
		BusinessID:    businessID,
		Name:          strings.TrimSpace(req.Name),
		Description:   strings.TrimSpace(req.Description),
		Image:         strings.TrimSpace(req.Image),
		DiscountType:  req.DiscountType,
		DiscountValue: req.DiscountValue,
		StartDate:     startDate,
		EndDate:       endDate,
		WeekdayMask:   weekdayMask,
		StartMinute:   req.StartMinute,
		EndMinute:     req.EndMinute,
		IsActive:      isActive,
		ApplicableTo:  applicableTo,
		TargetID:      targetID,
	}

	if err := database.CreateOffer(&offer); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create offer"})
		return
	}
	services.InvalidatePricingCache(businessID)
	if GetTranslationService() != nil {
		createdOffer := offer
		logger.SafeGo(func() {
			if err := autoTranslateOffer(createdOffer.BusinessID, createdOffer.ID, createdOffer.Name, createdOffer.Description); err != nil {
				log.Printf("Failed to auto-translate offer %d: %v", createdOffer.ID, err)
			}
		})
	}

	c.JSON(http.StatusCreated, annotateOfferWithInventory(businessID, offer))
}

// operatorOfferView decorates an offer row with the serve-time sellability
// verdict of its target dish for the operator Offers list (#835). An Active
// "$5 Off the Steak Plate" while beef is at zero is already hidden guest-side
// (filterGuestLivePromotions); the operator list must say so instead of
// showing a clean Active chip.
type operatorOfferView struct {
	database.Offer
	// InventoryBlocked is true for an item-targeted offer whose target dish is
	// not currently sellable (inventory 86 or manual 86) — the same hidden set
	// that suppresses the offer on guest surfaces.
	InventoryBlocked bool `json:"inventory_blocked"`
	// ManualActive mirrors the STORED is_active column. Operator reads report
	// is_active as the effective answer — whether the offer can actually fire —
	// so the surfaces that EDIT the switch (the offer modal) must read this
	// instead, or saving a blocked offer would turn it off for good and it
	// would stay off after the restock (#835). Serve-time only: no write path
	// accepts it.
	ManualActive bool `json:"manual_active"`
}

// annotateOffersWithInventory flags item-targeted offers whose target dish the
// guest surfaces currently refuse to sell. Fail-open: any lookup error leaves
// every flag false — an inventory hiccup must never take down the Offers tab.
func annotateOffersWithInventory(businessID uint, offers []database.Offer) []operatorOfferView {
	views := make([]operatorOfferView, len(offers))
	for i := range offers {
		views[i] = operatorOfferView{Offer: offers[i], ManualActive: offers[i].IsActive}
	}
	hasItemTarget := false
	for i := range offers {
		if offers[i].ApplicableTo == "item" && offers[i].TargetID != nil {
			hasItemTarget = true
			break
		}
	}
	if !hasItemTarget {
		return views
	}
	business, err := database.GetBusinessByID(businessID)
	if err != nil || business == nil {
		return views
	}
	_, categories, err := database.GetMenuByBusinessID(businessID)
	if err != nil {
		return views
	}
	projection := services.ProjectOrderability(business, categories, services.OrderabilityContextOperator, true)
	hidden := unsellableGuestMenuItemIDs(categories, projection)
	if len(hidden) == 0 {
		return views
	}
	for i := range views {
		offer := &views[i]
		if offer.ApplicableTo == "item" && offer.TargetID != nil && hidden[strings.TrimSpace(*offer.TargetID)] {
			offer.InventoryBlocked = true
			// #835: "AR$ 3.000 menos en el bife" came back is_active:true AND
			// inventory_blocked:true on the same row, while guest surfaces had
			// already dropped the offer. An offer that cannot fire is not
			// active — the flag now says so, and the operator's own switch
			// moves to manual_active so the modal still edits the real column.
			offer.IsActive = false
		}
	}
	return views
}

// annotateOfferWithInventory is the single-row form, used by the create/update
// echoes so a client that trusts the response body sees the same effective
// is_active the list will report a second later. Non-item offers skip the menu
// read entirely.
func annotateOfferWithInventory(businessID uint, offer database.Offer) operatorOfferView {
	views := annotateOffersWithInventory(businessID, []database.Offer{offer})
	if len(views) == 0 {
		return operatorOfferView{Offer: offer, ManualActive: offer.IsActive}
	}
	return views[0]
}

func GetOffers(c *gin.Context) {
	businessID, err := parseBusinessIDParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	offers, err := database.GetOffersByBusinessID(businessID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve offers"})
		return
	}

	c.JSON(http.StatusOK, annotateOffersWithInventory(businessID, offers))
}

func UpdateOffer(c *gin.Context) {
	businessID, err := parseBusinessIDParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	offerID, err := parseUintParam(c, "offerId")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	existing, err := database.GetOfferByID(offerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Offer not found"})
		return
	}
	if existing.BusinessID != businessID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Offer does not belong to this business"})
		return
	}

	var req OfferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	if err := validateOfferRequest(req); err != nil {
		writeOfferValidationError(c, err)
		return
	}
	if refuseDemoImage(c, req.Image, existing.Image) {
		return
	}

	startDate, _ := parseOptionalDate(req.StartDate)
	endDate, _ := parseOptionalDate(req.EndDate)
	applicableTo := normalizeApplicableTo(req.ApplicableTo)

	var targetID *string
	if req.TargetID != nil && strings.TrimSpace(*req.TargetID) != "" {
		value := strings.TrimSpace(*req.TargetID)
		targetID = &value
	}
	if applicableTo == "all" {
		targetID = nil
	}

	existing.Name = strings.TrimSpace(req.Name)
	existing.Description = strings.TrimSpace(req.Description)
	existing.Image = strings.TrimSpace(req.Image)
	existing.DiscountType = req.DiscountType
	existing.DiscountValue = req.DiscountValue
	existing.StartDate = startDate
	existing.EndDate = endDate
	if req.WeekdayMask != nil {
		existing.WeekdayMask = *req.WeekdayMask
	} else if existing.WeekdayMask == 0 {
		existing.WeekdayMask = 127
	}
	existing.StartMinute = req.StartMinute
	existing.EndMinute = req.EndMinute
	existing.ApplicableTo = applicableTo
	existing.TargetID = targetID
	if req.IsActive != nil {
		existing.IsActive = *req.IsActive
	}

	if err := database.UpdateOffer(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update offer"})
		return
	}
	services.InvalidatePricingCache(businessID)
	if GetTranslationService() != nil {
		updatedOffer := existing
		logger.SafeGo(func() {
			if err := autoTranslateOffer(updatedOffer.BusinessID, updatedOffer.ID, updatedOffer.Name, updatedOffer.Description); err != nil {
				log.Printf("Failed to auto-translate updated offer %d: %v", updatedOffer.ID, err)
			}
		})
	}

	c.JSON(http.StatusOK, annotateOfferWithInventory(businessID, *existing))
}

func DeleteOffer(c *gin.Context) {
	businessID, err := parseBusinessIDParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	offerID, err := parseUintParam(c, "offerId")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	existing, err := database.GetOfferByID(offerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Offer not found"})
		return
	}
	if existing.BusinessID != businessID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Offer does not belong to this business"})
		return
	}

	if err := database.DeleteOffer(offerID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete offer"})
		return
	}
	services.InvalidatePricingCache(businessID)
	id := offerID
	logger.SafeGo(func() {
		if err := deleteTranslationsForOffer(businessID, id); err != nil {
			log.Printf("Failed to delete offer translations for %d: %v", id, err)
		}
	})

	c.JSON(http.StatusOK, gin.H{"message": "Offer deleted successfully"})
}

// --- Bundles Handlers ---

func CreateBundle(c *gin.Context) {
	var req BundleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if req.Price <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "price must be greater than 0"})
		return
	}
	if refuseDemoImage(c, req.Image, "") {
		return
	}

	businessID, err := parseBusinessIDParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	business, err := database.GetBusinessByID(businessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}
	currency, err := resolveBundleCurrency(business.DefaultCurrency, req.Currency)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "bundle_currency_mismatch"})
		return
	}

	refs, err := parseBundleItems(req.Items)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	normalizedRefs, regularTotal, err := validateBundleItemsWithTotal(businessID, refs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// L3-18: reject anti-deals (bundle price ≥ component sum).
	if err := rejectAntiDealBundlePrice(req.Price, regularTotal); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "bundle_anti_deal"})
		return
	}
	refsJSON, err := json.Marshal(normalizedRefs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to serialize bundle items"})
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	bundle := database.Bundle{
		BusinessID:  businessID,
		Name:        strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description),
		Price:       req.Price,
		Currency:    currency,
		Image:       strings.TrimSpace(req.Image),
		Items:       string(refsJSON),
		IsActive:    isActive,
	}

	if err := database.CreateBundle(&bundle); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create bundle"})
		return
	}
	services.InvalidatePricingCache(businessID)
	createdBundle := bundle
	translateCreatedBundle := func() {
		if err := autoTranslateBundle(createdBundle.BusinessID, createdBundle.ID, createdBundle.Name, createdBundle.Description); err != nil {
			log.Printf("Failed to auto-translate bundle %d: %v", createdBundle.ID, err)
		}
	}
	if gin.Mode() == gin.TestMode {
		translateCreatedBundle()
	} else {
		logger.SafeGo(translateCreatedBundle)
	}

	c.JSON(http.StatusCreated, bundle)
}

func GetBundles(c *gin.Context) {
	businessID, err := parseBusinessIDParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	bundles, err := database.GetBundlesByBusinessID(businessID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve bundles"})
		return
	}

	c.JSON(http.StatusOK, bundles)
}

func UpdateBundle(c *gin.Context) {
	businessID, err := parseBusinessIDParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	bundleID, err := parseUintParam(c, "bundleId")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	existing, err := database.GetBundleByID(bundleID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bundle not found"})
		return
	}
	if existing.BusinessID != businessID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bundle does not belong to this business"})
		return
	}

	var req BundleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if req.Price <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "price must be greater than 0"})
		return
	}
	if refuseDemoImage(c, req.Image, existing.Image) {
		return
	}

	business, err := database.GetBusinessByID(businessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}
	currency, err := resolveBundleCurrency(business.DefaultCurrency, req.Currency)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "bundle_currency_mismatch"})
		return
	}

	refs, err := parseBundleItems(req.Items)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	normalizedRefs, regularTotal, err := validateBundleItemsWithTotal(businessID, refs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	refsJSON, err := json.Marshal(normalizedRefs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to serialize bundle items"})
		return
	}
	// L3-18 / M5: reject anti-deals only when price or items change — description-only
	// edits of legacy at/above-sum bundles must still save.
	priceChanged := req.Price != existing.Price
	itemsChanged := string(refsJSON) != existing.Items
	if priceChanged || itemsChanged {
		if err := rejectAntiDealBundlePrice(req.Price, regularTotal); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "bundle_anti_deal"})
			return
		}
	}

	existing.Name = strings.TrimSpace(req.Name)
	existing.Description = strings.TrimSpace(req.Description)
	existing.Price = req.Price
	existing.Currency = currency
	existing.Image = strings.TrimSpace(req.Image)
	existing.Items = string(refsJSON)
	if req.IsActive != nil {
		existing.IsActive = *req.IsActive
	}

	if err := database.UpdateBundle(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update bundle"})
		return
	}
	services.InvalidatePricingCache(businessID)
	updatedBundle := existing
	logger.SafeGo(func() {
		if err := autoTranslateBundle(updatedBundle.BusinessID, updatedBundle.ID, updatedBundle.Name, updatedBundle.Description); err != nil {
			log.Printf("Failed to auto-translate updated bundle %d: %v", updatedBundle.ID, err)
		}
	})

	c.JSON(http.StatusOK, existing)
}

func DeleteBundle(c *gin.Context) {
	businessID, err := parseBusinessIDParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	bundleID, err := parseUintParam(c, "bundleId")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	existing, err := database.GetBundleByID(bundleID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bundle not found"})
		return
	}
	if existing.BusinessID != businessID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bundle does not belong to this business"})
		return
	}

	if err := database.DeleteBundle(bundleID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete bundle"})
		return
	}
	services.InvalidatePricingCache(businessID)
	id := bundleID
	logger.SafeGo(func() {
		if err := deleteTranslationsForBundle(businessID, id); err != nil {
			log.Printf("Failed to delete bundle translations for %d: %v", id, err)
		}
	})

	c.JSON(http.StatusOK, gin.H{"message": "Bundle deleted successfully"})
}

// --- AI Image Generation ---

type GenerateImageRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Ingredients string `json:"ingredients"`
	// DietaryTags are the item's dietary tag ids (e.g. "vegetarian", "vegan")
	// from Menu Builder. Used only to select a pre-written hard-negative
	// sentence in the prompt builder — see MenuImagePrompt.DietaryTags.
	DietaryTags      []string `json:"dietary_tags"`
	EntityType       string   `json:"entity_type"`
	OfferStampText   string   `json:"offer_stamp_text"`
	OfferScope       string   `json:"offer_scope"`
	RelatedItemNames []string `json:"related_item_names"`
	BundleItems      []struct {
		Name     string `json:"name"`
		Quantity int    `json:"quantity"`
	} `json:"bundle_items"`
	BundlePrice float64 `json:"bundle_price"`
	Currency    string  `json:"currency"`
}

// Bounds for menu image generation requests (AI-15).
const (
	menuImageNameMaxRunes        = 120
	menuImageDescriptionMaxRunes = 1000
	menuImageIngredientsMaxRunes = 1000
	menuImageStampMaxRunes       = 80
	menuImageRelatedNameMaxRunes = 120
	menuImageRelatedMax          = 30
	menuImageBundleRowsMax       = 50
	menuImageBundleQtyMin        = 1
	menuImageBundleQtyMax        = 100
	menuImageBundlePriceMax      = 1_000_000
	menuImageDietaryTagsMax      = 12
	menuImageDietaryTagMaxRunes  = 40
)

var allowedMenuImageEntities = map[string]struct{}{
	"menu_item": {},
	"offer":     {},
	"bundle":    {},
	"":          {}, // treated as menu_item
}

var allowedMenuImageOfferScopes = map[string]struct{}{
	"":         {},
	"item":     {},
	"category": {},
	"menu":     {},
	"all":      {},
}

// payvergeImageCurrencies is the closed ISO-4217 set Payverge seeds by default.
var payvergeImageCurrencies = map[string]struct{}{
	"USD": {}, "EUR": {}, "GBP": {}, "JPY": {}, "AUD": {}, "CAD": {}, "CHF": {},
	"CNY": {}, "ARS": {}, "AED": {}, "BRL": {}, "MXN": {}, "INR": {}, "KRW": {},
	"SGD": {}, "HKD": {}, "NOK": {}, "SEK": {}, "DKK": {}, "PLN": {}, "CLP": {},
	"COP": {}, "PEN": {}, "UYU": {}, "PYG": {}, "BOB": {}, "CRC": {}, "DOP": {},
}

func runeLen(s string) int { return len([]rune(s)) }

// normalizeImageDietaryTags bounds and trims a dietary_tags request list.
// Shared by the generate, regenerate, and enhance image endpoints (#588/#601).
// The tags only ever select pre-written prompt sentences server-side (see
// services.dietaryConstraintDirective); the tag text never reaches a prompt.
func normalizeImageDietaryTags(tags []string) ([]string, error) {
	if len(tags) > menuImageDietaryTagsMax {
		return nil, fmt.Errorf("dietary_tags exceeds %d entries", menuImageDietaryTagsMax)
	}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		tag := strings.TrimSpace(t)
		if tag == "" {
			continue
		}
		if runeLen(tag) > menuImageDietaryTagMaxRunes {
			return nil, fmt.Errorf("dietary tag exceeds %d characters", menuImageDietaryTagMaxRunes)
		}
		out = append(out, tag)
	}
	return out, nil
}

// normalizeGenerateImageRequest validates and bounds a GenerateImageRequest into
// a MenuImagePrompt. Rejects blank names, unsupported enums, invalid currency,
// absurd prices/quantities, and oversized free text before any credit reservation.
func normalizeGenerateImageRequest(req GenerateImageRequest) (services.MenuImagePrompt, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return services.MenuImagePrompt{}, fmt.Errorf("name is required")
	}
	if runeLen(name) > menuImageNameMaxRunes {
		return services.MenuImagePrompt{}, fmt.Errorf("name exceeds %d characters", menuImageNameMaxRunes)
	}
	desc := strings.TrimSpace(req.Description)
	if runeLen(desc) > menuImageDescriptionMaxRunes {
		return services.MenuImagePrompt{}, fmt.Errorf("description exceeds %d characters", menuImageDescriptionMaxRunes)
	}
	ingredients := strings.TrimSpace(req.Ingredients)
	if runeLen(ingredients) > menuImageIngredientsMaxRunes {
		return services.MenuImagePrompt{}, fmt.Errorf("ingredients exceeds %d characters", menuImageIngredientsMaxRunes)
	}
	stamp := strings.TrimSpace(req.OfferStampText)
	if runeLen(stamp) > menuImageStampMaxRunes {
		return services.MenuImagePrompt{}, fmt.Errorf("offer_stamp_text exceeds %d characters", menuImageStampMaxRunes)
	}

	entity := strings.ToLower(strings.TrimSpace(req.EntityType))
	if entity == "" {
		entity = "menu_item"
	}
	if _, ok := allowedMenuImageEntities[entity]; !ok {
		return services.MenuImagePrompt{}, fmt.Errorf("unsupported entity_type %q", req.EntityType)
	}

	scope := strings.ToLower(strings.TrimSpace(req.OfferScope))
	if _, ok := allowedMenuImageOfferScopes[scope]; !ok {
		return services.MenuImagePrompt{}, fmt.Errorf("unsupported offer_scope %q", req.OfferScope)
	}

	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if currency != "" {
		if len(currency) != 3 {
			return services.MenuImagePrompt{}, fmt.Errorf("currency must be a 3-letter ISO code")
		}
		if _, ok := payvergeImageCurrencies[currency]; !ok {
			return services.MenuImagePrompt{}, fmt.Errorf("unsupported currency %q", currency)
		}
	}

	if req.BundlePrice < 0 || req.BundlePrice > menuImageBundlePriceMax {
		return services.MenuImagePrompt{}, fmt.Errorf("bundle_price out of range")
	}

	dietaryTags, tagErr := normalizeImageDietaryTags(req.DietaryTags)
	if tagErr != nil {
		return services.MenuImagePrompt{}, tagErr
	}

	if len(req.RelatedItemNames) > menuImageRelatedMax {
		return services.MenuImagePrompt{}, fmt.Errorf("related_item_names exceeds %d entries", menuImageRelatedMax)
	}
	related := make([]string, 0, len(req.RelatedItemNames))
	for _, r := range req.RelatedItemNames {
		n := strings.TrimSpace(r)
		if n == "" {
			continue
		}
		if runeLen(n) > menuImageRelatedNameMaxRunes {
			return services.MenuImagePrompt{}, fmt.Errorf("related item name exceeds %d characters", menuImageRelatedNameMaxRunes)
		}
		related = append(related, n)
	}

	if len(req.BundleItems) > menuImageBundleRowsMax {
		return services.MenuImagePrompt{}, fmt.Errorf("bundle_items exceeds %d rows", menuImageBundleRowsMax)
	}
	bundleItems := make([]services.BundleImagePromptItem, 0, len(req.BundleItems))
	for _, item := range req.BundleItems {
		n := strings.TrimSpace(item.Name)
		if n == "" {
			continue
		}
		if runeLen(n) > menuImageRelatedNameMaxRunes {
			return services.MenuImagePrompt{}, fmt.Errorf("bundle item name exceeds %d characters", menuImageRelatedNameMaxRunes)
		}
		q := item.Quantity
		if q < menuImageBundleQtyMin || q > menuImageBundleQtyMax {
			return services.MenuImagePrompt{}, fmt.Errorf("bundle item quantity must be between %d and %d", menuImageBundleQtyMin, menuImageBundleQtyMax)
		}
		bundleItems = append(bundleItems, services.BundleImagePromptItem{Name: n, Quantity: q})
	}

	return services.MenuImagePrompt{
		Name:             name,
		Description:      desc,
		Ingredients:      ingredients,
		DietaryTags:      dietaryTags,
		EntityType:       entity,
		OfferStampText:   stamp,
		OfferScope:       scope,
		RelatedItemNames: related,
		BundleItems:      bundleItems,
		BundlePrice:      req.BundlePrice,
		Currency:         currency,
	}, nil
}

// menuImageGuardrailBrief builds a delimited text summary of the normalized
// request for the image-prompt classifier (not instruction text).
func menuImageGuardrailBrief(p services.MenuImagePrompt) string {
	var b strings.Builder
	b.WriteString("name: ")
	b.WriteString(p.Name)
	b.WriteString("\ndescription: ")
	b.WriteString(p.Description)
	b.WriteString("\ningredients: ")
	b.WriteString(p.Ingredients)
	b.WriteString("\nentity_type: ")
	b.WriteString(p.EntityType)
	b.WriteString("\noffer_stamp: ")
	b.WriteString(p.OfferStampText)
	if len(p.RelatedItemNames) > 0 {
		b.WriteString("\nrelated: ")
		b.WriteString(strings.Join(p.RelatedItemNames, ", "))
	}
	for _, it := range p.BundleItems {
		b.WriteString(fmt.Sprintf("\nbundle_item: %dx %s", it.Quantity, it.Name))
	}
	return b.String()
}

func GenerateMenuImage(c *gin.Context) {
	business, ok := getAIMenuRouteBusiness(c)
	if !ok {
		return
	}

	var req GenerateImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	prompt, normErr := normalizeGenerateImageRequest(req)
	if normErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": normErr.Error(), "code": "invalid_image_request"})
		return
	}
	prompt.BusinessID = business.ID

	// Screen the complete bounded brief BEFORE singleflight / credit reservation.
	if allowed, category := evaluateImagePromptGuardrail(
		imagePromptClassifier,
		c.Request.Context(),
		business.ID,
		determineBusinessOwnerLanguage(business),
		menuImageGuardrailBrief(prompt),
	); !allowed {
		log.Printf("GenerateMenuImage: prompt blocked (%s) for business %s", category, business.BusinessId)
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "That request can't be used for an image. Try describing the dish itself.",
			"code":  "prompt_rejected",
		})
		return
	}

	service := GetAIService()
	if service == nil {
		respondAINotConfigured(c)
		return
	}

	log.Printf("GenerateMenuImage: starting business_id=%d", business.ID)

	key := imageJobBriefKey(briefFromMenuImagePrompt(business.ID, "generate", prompt))
	v, genErr := doSharedImageJob(c.Request.Context(), key, func(jobCtx context.Context) (interface{}, error) {
		// Route through reserveGenerateRefundImage so a panic inside the provider
		// call refunds the reserved credit before re-raising (parity with the
		// regenerate/enhance handlers). A bare reserve/refund here would leak
		// the paid credit if GenerateMenuImage panicked mid-flight.
		result, gErr := reserveGenerateRefundImage(jobCtx, business, func() (*services.GeneratedImage, error) {
			return service.GenerateMenuImage(jobCtx, prompt)
		})
		if gErr != nil {
			return nil, gErr
		}
		if recErr := database.RecordAIGeneratedImage(business.ID, result.URL, "generate", result.Model); recErr != nil {
			log.Printf("GenerateMenuImage: provenance record failed for business %s: %v", business.BusinessId, recErr)
		}
		return result, nil
	})
	if respondImageDailyLimit(c, genErr) {
		return
	}
	if genErr != nil {
		log.Printf("GenerateMenuImage: failed to generate image for business %s: %v", business.BusinessId, genErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate image"})
		return
	}
	image := v.(*services.GeneratedImage)
	c.JSON(http.StatusOK, gin.H{
		"url":       image.URL,
		"credit":    "Generated by AI",
		"mime_type": image.MIMEType,
		"model":     image.Model,
	})
}
