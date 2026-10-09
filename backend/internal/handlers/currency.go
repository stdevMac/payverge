package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/demomode"
	"github.com/stdevmac/payverge/backend/internal/locales"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

// CurrencyHandler handles currency and language related requests
type CurrencyHandler struct {
	db                  *database.DB
	exchangeRateService *services.ExchangeRateService
	translationService  *services.TranslationService
}

// NewCurrencyHandler creates a new currency handler
func NewCurrencyHandler(db *database.DB, exchangeRateService *services.ExchangeRateService, translationService *services.TranslationService) *CurrencyHandler {
	return &CurrencyHandler{
		db:                  db,
		exchangeRateService: exchangeRateService,
		translationService:  translationService,
	}
}

func (h *CurrencyHandler) authorizeBusinessAccess(c *gin.Context, businessID uint, requiredPermissions ...string) bool {
	return h.authorizeBusiness(c, businessID, false, requiredPermissions...)
}

// authorizeBusinessMutation is the write-path sibling of authorizeBusinessAccess.
// It additionally enforces the same admin-lifecycle lock the middleware applies
// to every other menu mutation (see RequireOperationalBusiness in main.go).
// The /inside/translations routes carry business_id in the request body rather than
// as an :id path param, so the RequireOperationalBusiness() middleware cannot be
// wired to them — the gate has to live inline in the mutating handler. Read paths
// stay untouched so a suspended business remains read-only, not blind.
func (h *CurrencyHandler) authorizeBusinessMutation(c *gin.Context, businessID uint, requiredPermissions ...string) bool {
	return h.authorizeBusiness(c, businessID, true, requiredPermissions...)
}

func (h *CurrencyHandler) authorizeBusiness(c *gin.Context, businessID uint, requireOperational bool, requiredPermissions ...string) bool {
	if h.db == nil || h.db.GetGorm() == nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Currency service unavailable")
		return false
	}

	var business database.Business
	if err := h.db.GetGorm().First(&business, businessID).Error; err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Business not found")
		return false
	}

	if !server.CheckBusinessAccess(c, &business) {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Access denied")
		return false
	}

	if requireOperational && server.RespondIfBusinessLocked(c, &business) {
		// Same 403 business_suspended / business_closed body the
		// RequireOperationalBusiness middleware returns for menu mutations.
		return false
	}

	if tokenType, _ := c.Get("token_type"); tokenType == "staff" && len(requiredPermissions) > 0 {
		server.NewRBACMiddleware(h.db).RequirePermissions(requiredPermissions...)(c)
		if c.IsAborted() {
			return false
		}
	}

	return true
}

// GetSupportedCurrencies returns all supported currencies
func (h *CurrencyHandler) GetSupportedCurrencies(c *gin.Context) {
	currencies, err := h.db.CurrencyService.GetSupportedCurrencies()
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to get supported currencies")
		return
	}

	c.JSON(http.StatusOK, gin.H{"currencies": currencies})
}

// GetSupportedLanguages returns all supported languages
func (h *CurrencyHandler) GetSupportedLanguages(c *gin.Context) {
	languages, err := h.db.LanguageService.GetSupportedLanguages()
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to get supported languages")
		return
	}

	c.JSON(http.StatusOK, gin.H{"languages": languages})
}

// GetExchangeRate returns the exchange rate between two currencies
func (h *CurrencyHandler) GetExchangeRate(c *gin.Context) {
	fromCurrency := c.Query("from")
	toCurrency := c.Query("to")

	if fromCurrency == "" || toCurrency == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Both 'from' and 'to' currency parameters are required")
		return
	}

	rate, err := h.exchangeRateService.GetExchangeRate(fromCurrency, toCurrency)
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Exchange rate not found")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"from_currency": fromCurrency,
		"to_currency":   toCurrency,
		"rate":          rate,
	})
}

// ConvertAmount converts an amount from one currency to another
func (h *CurrencyHandler) ConvertAmount(c *gin.Context) {
	amountStr := c.Query("amount")
	fromCurrency := c.Query("from")
	toCurrency := c.Query("to")

	if amountStr == "" || fromCurrency == "" || toCurrency == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Amount, from, and to parameters are required")
		return
	}

	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid amount format")
		return
	}

	convertedAmount, err := h.exchangeRateService.ConvertAmount(amount, fromCurrency, toCurrency)
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Failed to convert amount")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"original_amount":  amount,
		"from_currency":    fromCurrency,
		"converted_amount": convertedAmount,
		"to_currency":      toCurrency,
	})
}

// GetBusinessCurrencies returns currencies supported by a business
func (h *CurrencyHandler) GetBusinessCurrencies(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	// Access control is handled by HybridAuthenticationMiddleware
	// Business access validation is done in middleware

	currencies, err := h.db.CurrencyService.GetBusinessCurrencies(uint(businessID))
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to get business currencies")
		return
	}

	c.JSON(http.StatusOK, gin.H{"currencies": currencies})
}

// UpdateBusinessCurrencies updates currencies supported by a business
func (h *CurrencyHandler) UpdateBusinessCurrencies(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	var request struct {
		CurrencyCodes []string `json:"currency_codes" binding:"required"`
		PreferredCode string   `json:"preferred_code" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		server.RespondBindError(c, err)
		return
	}

	// Access control is handled by HybridAuthenticationMiddleware + RoleBasedAccessMiddleware
	// Business access validation and permission checking is done in middleware

	// Validate that preferred currency is in the list
	preferredFound := false
	for _, code := range request.CurrencyCodes {
		if code == request.PreferredCode {
			preferredFound = true
			break
		}
	}

	if !preferredFound {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Preferred currency must be in the currency list")
		return
	}

	// Update business currencies
	if err := h.db.CurrencyService.SetBusinessCurrencies(uint(businessID), request.CurrencyCodes, request.PreferredCode); err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to update business currencies")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Business currencies updated successfully"})
}

// GetBusinessLanguages returns languages supported by a business
func (h *CurrencyHandler) GetBusinessLanguages(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	// Verify business exists and user has access
	var business database.Business
	if err := h.db.GetGorm().First(&business, businessID).Error; err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Business not found")
		return
	}

	// Access control is enforced by this route's middleware chain (see main.go):
	//   - HybridAuthenticationMiddleware (group-level on /inside) enforces business
	//     scope INLINE: web3/OAuth tokens must match the business owner; staff
	//     tokens must have staff.BusinessID == the :id route business (else 403).
	//   - RoleBasedAccessMiddleware("languages:read") enforces the RBAC permission.
	// There is no separate staff/business middleware: the staff/business match
	// is done inline in HybridAuthenticationMiddleware, so a business-scoped
	// route must sit under that group to be scoped.

	languages, err := h.db.LanguageService.GetBusinessLanguages(uint(businessID))
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to get business languages")
		return
	}

	c.JSON(http.StatusOK, gin.H{"languages": languages})
}

// UpdateBusinessLanguages updates languages supported by a business
func (h *CurrencyHandler) UpdateBusinessLanguages(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	var request struct {
		LanguageCodes []string `json:"language_codes" binding:"required"`
		DefaultCode   string   `json:"default_code" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		server.RespondBindError(c, err)
		return
	}

	// Verify business exists (needed for translation service later)
	var business database.Business
	if err := h.db.GetGorm().First(&business, businessID).Error; err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Business not found")
		return
	}

	// Access control is handled by HybridAuthenticationMiddleware + RoleBasedAccessMiddleware
	// Business access validation and permission checking is done in middleware

	for _, code := range request.LanguageCodes {
		if !locales.IsGuestLocale(code) {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, fmt.Sprintf("Unsupported language: %s", code))
			return
		}
	}

	if !locales.IsGuestLocale(request.DefaultCode) {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, fmt.Sprintf("Unsupported language: %s", request.DefaultCode))
		return
	}

	// Validate that default language is in the list
	defaultFound := false
	for _, code := range request.LanguageCodes {
		if code == request.DefaultCode {
			defaultFound = true
			break
		}
	}

	if !defaultFound {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Default language must be in the language list")
		return
	}

	// Update business languages
	if err := h.db.LanguageService.SetBusinessLanguages(uint(businessID), request.LanguageCodes, request.DefaultCode); err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to update business languages")
		return
	}
	services.InvalidatePublicGuestBusiness(uint(businessID))

	// Auto-translate the storefront menu into the configured guest languages so
	// guests immediately see a translated menu — without the operator having to
	// open the menu builder and click "Sync Translations". This mirrors the
	// per-item auto-translation that already runs when a menu item is added or
	// edited; here we cover the case of adding a whole language after the fact.
	// performBatchTranslation filters out the default language and is idempotent
	// at the field level, so re-saving the same language set is cheap.
	if server.IsBatchTranslationEnabled() {
		languageCodes := append([]string(nil), request.LanguageCodes...)
		businessIDForTranslation := uint(businessID)
		logger.SafeGo(func() {
			if err := server.TranslateBusinessMenuForLanguages(businessIDForTranslation, languageCodes); err != nil {
				log.Printf("background menu translation failed for business %d: %v", businessIDForTranslation, err)
			}
			// Also translate the storefront prose (hero/about copy) and the
			// "Why choose us" special features so the whole guest page — not just
			// the menu — follows the newly configured languages.
			if err := server.TranslateBusinessContentForLanguages(businessIDForTranslation, languageCodes); err != nil {
				log.Printf("background business content translation failed for business %d: %v", businessIDForTranslation, err)
			}
			// Offers/bundles were the one storefront surface language-add
			// skipped — guests in a new language saw source-language promo
			// names until a first-hit backfill. Translate them proactively,
			// like the menu and the business prose. (D4b)
			if err := server.TranslateExistingPromotionContent(businessIDForTranslation); err != nil {
				log.Printf("background promotion translation failed for business %d: %v", businessIDForTranslation, err)
			}
		})
	}

	c.JSON(http.StatusOK, gin.H{"message": "Business languages updated successfully"})
}

// GetTranslatedContent returns translated content for an entity
func (h *CurrencyHandler) GetTranslatedContent(c *gin.Context) {
	businessIDStr := c.Query("business_id")
	entityType := c.Query("entity_type")
	entityIDStr := c.Query("entity_id")
	fieldName := c.Query("field_name")
	languageCode := c.Query("language_code")

	if businessIDStr == "" || entityType == "" || entityIDStr == "" || fieldName == "" || languageCode == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "business_id, entity_type, entity_id, field_name, and language_code are required")
		return
	}

	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}
	if !h.authorizeBusinessAccess(c, uint(businessID), string(server.PermMenuRead)) {
		return
	}

	entityID, err := strconv.ParseUint(entityIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid entity ID")
		return
	}

	translatedText, err := h.translationService.GetTranslatedContent(uint(businessID), entityType, uint(entityID), fieldName, languageCode)
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Translation not found")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"business_id":     businessID,
		"entity_type":     entityType,
		"entity_id":       entityID,
		"field_name":      fieldName,
		"language_code":   languageCode,
		"translated_text": translatedText,
	})
}

// UpdateTranslation manually updates a translation
func (h *CurrencyHandler) UpdateTranslation(c *gin.Context) {
	var request struct {
		BusinessID     uint   `json:"business_id" binding:"required"`
		EntityType     string `json:"entity_type" binding:"required"`
		EntityID       uint   `json:"entity_id" binding:"required"`
		FieldName      string `json:"field_name" binding:"required"`
		LanguageCode   string `json:"language_code" binding:"required"`
		TranslatedText string `json:"translated_text" binding:"required"`
		OriginalText   string `json:"original_text"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		server.RespondBindError(c, err)
		return
	}

	if !h.authorizeBusinessMutation(c, request.BusinessID, string(server.PermMenuTranslate)) {
		return
	}
	// Manual translations render on the storefront in place of the source
	// text, so the public demo caps them like the source fields.
	if config.DemoModeEnabled() && (demomode.TextTooLong(request.TranslatedText) || demomode.TextTooLong(request.OriginalText)) {
		demomode.Refuse(c, demomode.KindStorefront, demomode.StorefrontTextAction)
		return
	}

	translation := &database.Translation{
		BusinessID:        request.BusinessID,
		EntityType:        request.EntityType,
		EntityID:          request.EntityID,
		FieldName:         request.FieldName,
		LanguageCode:      request.LanguageCode,
		OriginalText:      request.OriginalText,
		TranslatedText:    request.TranslatedText,
		IsAutoTranslated:  false, // Manual translation
		TranslationSource: "manual",
	}

	if err := h.db.TranslationService.SaveTranslation(translation); err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to save translation")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Translation updated successfully"})
}

// GetMenuTranslations returns all translations for a business menu in a specific language
func (h *CurrencyHandler) GetMenuTranslations(c *gin.Context) {
	businessIDStr := c.Query("business_id")
	languageCode := c.Query("language_code")

	if businessIDStr == "" || languageCode == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "business_id and language_code are required")
		return
	}

	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	if !h.authorizeBusinessAccess(c, uint(businessID), string(server.PermMenuRead)) {
		return
	}

	h.respondWithMenuTranslations(c, uint(businessID), languageCode, nil)
}

// GetGuestMenuTranslations is the public guest-accessible equivalent of
// GetMenuTranslations. It requires a table code to resolve the business so
// unauthenticated guests can view translated menus on the table page without
// leaking bulk access to other businesses' data.
func (h *CurrencyHandler) GetGuestMenuTranslations(c *gin.Context) {
	tableCode := strings.TrimSpace(c.Param("code"))
	if tableCode == "" {
		tableCode = strings.TrimSpace(c.Query("table_code"))
	}
	languageCode := strings.TrimSpace(c.Query("language_code"))

	if tableCode == "" || languageCode == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "table_code and language_code are required")
		return
	}

	var table database.Table
	if err := h.db.GetGorm().Where("table_code = ? AND is_active = ?", tableCode, true).First(&table).Error; err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Table not found")
		return
	}

	h.respondWithMenuTranslations(c, table.BusinessID, languageCode, guestMenuTranslationEntityTypes)
}

// guestMenuTranslationEntityTypes are the only entity types the guest bill
// renders (GuestBill reads translations.menu_item and translations.bundle).
var guestMenuTranslationEntityTypes = []string{"menu_item", "bundle"}

// respondWithMenuTranslations groups a venue's translations for one language
// by entity type and id. entityTypes, when non-empty, limits the read to those
// types so the (business_id, language_code, entity_type, ...) index prefix
// serves it. Rows are read oldest first so the newest row for a field wins.
func (h *CurrencyHandler) respondWithMenuTranslations(c *gin.Context, businessID uint, languageCode string, entityTypes []string) {
	var translations []database.Translation
	q := h.db.GetGorm().
		Select("id", "entity_type", "entity_id", "field_name", "translated_text").
		Where("business_id = ? AND language_code = ?", businessID, languageCode)
	if len(entityTypes) > 0 {
		q = q.Where("entity_type IN ?", entityTypes)
	}
	err := q.Order("id ASC").Find(&translations).Error
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to get translations")
		return
	}

	// Group translations by entity type and ID
	result := make(map[string]map[string]map[string]string)

	for _, translation := range translations {
		entityType := translation.EntityType
		entityID := fmt.Sprintf("%d", translation.EntityID)
		fieldName := translation.FieldName

		if result[entityType] == nil {
			result[entityType] = make(map[string]map[string]string)
		}
		if result[entityType][entityID] == nil {
			result[entityType][entityID] = make(map[string]string)
		}

		result[entityType][entityID][fieldName] = translation.TranslatedText
	}

	// Dual-key menu_item rows by the catalog MenuItem.ID string so guest bills
	// (which store menu_item_id = "demo-tacos", not position entity id "2") can
	// resolve the same translated names as the table menu (GUEST-002).
	dualKeyMenuItemTranslationsByCatalogID(h.db, businessID, result)

	c.JSON(http.StatusOK, gin.H{
		"business_id":   businessID,
		"language_code": languageCode,
		"translations":  result,
	})
}

// dualKeyMenuItemTranslationsByCatalogID copies position-based menu_item
// translation maps onto each item's stable catalog id (MenuItem.ID). Position
// keys (categoryIndex*1000+itemIndex) stay for legacy callers.
func dualKeyMenuItemTranslationsByCatalogID(dbw *database.DB, businessID uint, result map[string]map[string]map[string]string) {
	if dbw == nil || result == nil {
		return
	}
	menuItems := result["menu_item"]
	if len(menuItems) == 0 {
		return
	}
	_, categories, err := database.GetMenuByBusinessID(businessID)
	if err != nil || len(categories) == 0 {
		return
	}
	for i, category := range categories {
		for j, item := range category.Items {
			catalogID := strings.TrimSpace(item.ID)
			if catalogID == "" {
				continue
			}
			entityID := fmt.Sprintf("%d", i*1000+j)
			fields := menuItems[entityID]
			if len(fields) == 0 {
				continue
			}
			// Do not overwrite an existing catalog-id key with different fields
			// unless it is absent — keeps intentional dual writes intact.
			if existing := menuItems[catalogID]; len(existing) > 0 {
				continue
			}
			copied := make(map[string]string, len(fields))
			for k, v := range fields {
				copied[k] = v
			}
			menuItems[catalogID] = copied
		}
	}
}
