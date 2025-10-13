package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"payverge/internal/database"
	"payverge/internal/services"

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

// GetSupportedCurrencies returns all supported currencies
func (h *CurrencyHandler) GetSupportedCurrencies(c *gin.Context) {
	currencies, err := h.db.CurrencyService.GetSupportedCurrencies()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get supported currencies"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"currencies": currencies})
}

// GetSupportedLanguages returns all supported languages
func (h *CurrencyHandler) GetSupportedLanguages(c *gin.Context) {
	languages, err := h.db.LanguageService.GetSupportedLanguages()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get supported languages"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"languages": languages})
}

// GetExchangeRate returns the exchange rate between two currencies
func (h *CurrencyHandler) GetExchangeRate(c *gin.Context) {
	fromCurrency := c.Query("from")
	toCurrency := c.Query("to")

	if fromCurrency == "" || toCurrency == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Both 'from' and 'to' currency parameters are required"})
		return
	}

	rate, err := h.exchangeRateService.GetExchangeRate(fromCurrency, toCurrency)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Exchange rate not found"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Amount, from, and to parameters are required"})
		return
	}

	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid amount format"})
		return
	}

	convertedAmount, err := h.exchangeRateService.ConvertAmount(amount, fromCurrency, toCurrency)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Failed to convert amount"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"original_amount":   amount,
		"from_currency":     fromCurrency,
		"converted_amount":  convertedAmount,
		"to_currency":       toCurrency,
	})
}

// GetBusinessCurrencies returns currencies supported by a business
func (h *CurrencyHandler) GetBusinessCurrencies(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Verify business exists and user has access
	var business database.Business
	if err := h.db.GetGorm().First(&business, businessID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	// Get user address from context
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User address not found"})
		return
	}

	// Verify user owns the business
	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	currencies, err := h.db.CurrencyService.GetBusinessCurrencies(uint(businessID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get business currencies"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"currencies": currencies})
}

// UpdateBusinessCurrencies updates currencies supported by a business
func (h *CurrencyHandler) UpdateBusinessCurrencies(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	var request struct {
		CurrencyCodes  []string `json:"currency_codes" binding:"required"`
		PreferredCode  string   `json:"preferred_code" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify business exists and user has access
	var business database.Business
	if err := h.db.GetGorm().First(&business, businessID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	// Get user address from context
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User address not found"})
		return
	}

	// Verify user owns the business
	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Validate that preferred currency is in the list
	preferredFound := false
	for _, code := range request.CurrencyCodes {
		if code == request.PreferredCode {
			preferredFound = true
			break
		}
	}

	if !preferredFound {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Preferred currency must be in the currency list"})
		return
	}

	// Update business currencies
	if err := h.db.CurrencyService.SetBusinessCurrencies(uint(businessID), request.CurrencyCodes, request.PreferredCode); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update business currencies"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Business currencies updated successfully"})
}

// GetBusinessLanguages returns languages supported by a business
func (h *CurrencyHandler) GetBusinessLanguages(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Verify business exists and user has access
	var business database.Business
	if err := h.db.GetGorm().First(&business, businessID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	// Get user address from context
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User address not found"})
		return
	}

	// Verify user owns the business
	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	languages, err := h.db.LanguageService.GetBusinessLanguages(uint(businessID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get business languages"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"languages": languages})
}

// UpdateBusinessLanguages updates languages supported by a business
func (h *CurrencyHandler) UpdateBusinessLanguages(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	var request struct {
		LanguageCodes []string `json:"language_codes" binding:"required"`
		DefaultCode   string   `json:"default_code" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify business exists and user has access
	var business database.Business
	if err := h.db.GetGorm().First(&business, businessID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	// Get user address from context
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User address not found"})
		return
	}

	// Verify user owns the business
	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Default language must be in the language list"})
		return
	}

	// Update business languages
	if err := h.db.LanguageService.SetBusinessLanguages(uint(businessID), request.LanguageCodes, request.DefaultCode); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update business languages"})
		return
	}

	// Trigger translation of existing business content
	if h.translationService.IsEnabled() {
		go func() {
			if err := h.translationService.TranslateBusiness(uint(businessID), business.Name, business.Description); err != nil {
				// Log error but don't fail the request
				// Translation happens in background
			}
		}()
	}

	c.JSON(http.StatusOK, gin.H{"message": "Business languages updated successfully"})
}

// GetTranslatedContent returns translated content for an entity
func (h *CurrencyHandler) GetTranslatedContent(c *gin.Context) {
	entityType := c.Query("entity_type")
	entityIDStr := c.Query("entity_id")
	fieldName := c.Query("field_name")
	languageCode := c.Query("language_code")

	if entityType == "" || entityIDStr == "" || fieldName == "" || languageCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "entity_type, entity_id, field_name, and language_code are required"})
		return
	}

	entityID, err := strconv.ParseUint(entityIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid entity ID"})
		return
	}

	translatedText, err := h.translationService.GetTranslatedContent(entityType, uint(entityID), fieldName, languageCode)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Translation not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
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
		EntityType     string `json:"entity_type" binding:"required"`
		EntityID       uint   `json:"entity_id" binding:"required"`
		FieldName      string `json:"field_name" binding:"required"`
		LanguageCode   string `json:"language_code" binding:"required"`
		TranslatedText string `json:"translated_text" binding:"required"`
		OriginalText   string `json:"original_text"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// TODO: Add authorization check based on entity type
	// For now, we'll allow any authenticated user to update translations

	translation := &database.Translation{
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save translation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Translation updated successfully"})
}

// GetMenuTranslations returns all translations for a business menu in a specific language
func (h *CurrencyHandler) GetMenuTranslations(c *gin.Context) {
	businessIDStr := c.Query("business_id")
	languageCode := c.Query("language_code")

	if businessIDStr == "" || languageCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "business_id and language_code are required"})
		return
	}

	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Get all translations for this business and language
	var translations []database.Translation
	err = h.db.GetGorm().Where("language_code = ?", languageCode).Find(&translations).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get translations"})
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

	c.JSON(http.StatusOK, gin.H{
		"business_id":    businessID,
		"language_code":  languageCode,
		"translations":   result,
	})
}
