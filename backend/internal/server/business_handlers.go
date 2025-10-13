package server

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"payverge/internal/database"
	"payverge/internal/services"

	"github.com/gin-gonic/gin"
)

// Business creation request
type CreateBusinessRequest struct {
	Name             string                      `json:"name" binding:"required"`
	Logo             string                      `json:"logo"`
	Address          database.BusinessAddress   `json:"address"`
	SettlementAddr   string                      `json:"settlement_address" binding:"required"`
	TippingAddr      string                      `json:"tipping_address" binding:"required"`
	TaxRate          float64                     `json:"tax_rate"`
	ServiceFeeRate   float64                     `json:"service_fee_rate"`
	TaxInclusive     bool                        `json:"tax_inclusive"`
	ServiceInclusive bool                        `json:"service_inclusive"`
	// New fields for enhanced business features
	Description          string                  `json:"description"`
	CustomURL            string                  `json:"custom_url"`
	Phone                string                  `json:"phone"`
	Email                string                  `json:"email"`
	Website              string                  `json:"website"`
	SocialMedia          string                  `json:"social_media"`
	BannerImages         string                  `json:"banner_images"`
	BusinessPageEnabled  bool                    `json:"business_page_enabled"`
	ShowReviews          bool                    `json:"show_reviews"`
	GoogleReviewsEnabled bool                    `json:"google_reviews_enabled"`
	ReferredByCode       string                  `json:"referred_by_code"` // Referral code used during registration
}

// Business update request
type UpdateBusinessRequest struct {
	Name             string                      `json:"name"`
	Logo             string                      `json:"logo"`
	Address          database.BusinessAddress   `json:"address"`
	SettlementAddr   string                      `json:"settlement_address"`
	TippingAddr      string                      `json:"tipping_address"`
	TaxRate          float64                     `json:"tax_rate"`
	ServiceFeeRate   float64                     `json:"service_fee_rate"`
	TaxInclusive     bool                        `json:"tax_inclusive"`
	ServiceInclusive bool                        `json:"service_inclusive"`
	// New fields for enhanced business features
	Description          string                  `json:"description"`
	CustomURL            string                  `json:"custom_url"`
	Phone                string                  `json:"phone"`
	Website              string                  `json:"website"`
	SocialMedia          string                  `json:"social_media"`
	BannerImages         string                  `json:"banner_images"`
	BusinessPageEnabled  bool                    `json:"business_page_enabled"`
	ShowReviews          bool                    `json:"show_reviews"`
	GoogleReviewsEnabled bool                    `json:"google_reviews_enabled"`
}

// CreateBusiness creates a new business for the authenticated user
func CreateBusiness(c *gin.Context) {
	// Get user address from authentication middleware
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var req CreateBusinessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate Ethereum addresses
	if !isValidEthereumAddress(req.SettlementAddr) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid settlement address"})
		return
	}
	if !isValidEthereumAddress(req.TippingAddr) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tipping address"})
		return
	}

	// Validate custom URL if provided
	if req.CustomURL != "" {
		if err := validateCustomURL(req.CustomURL, 0); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	business := &database.Business{
		OwnerAddress:         userAddress.(string),
		Name:                 req.Name,
		Logo:                 req.Logo,
		Address:              req.Address,
		SettlementAddr:       req.SettlementAddr,
		TippingAddr:          req.TippingAddr,
		TaxRate:              req.TaxRate,
		ServiceFeeRate:       req.ServiceFeeRate,
		TaxInclusive:         req.TaxInclusive,
		ServiceInclusive:     req.ServiceInclusive,
		IsActive:             true,
		// New fields
		Description:          req.Description,
		CustomURL:            req.CustomURL,
		Phone:                req.Phone,
		Website:              req.Website,
		SocialMedia:          req.SocialMedia,
		BannerImages:         req.BannerImages,
		BusinessPageEnabled:  req.BusinessPageEnabled,
		ShowReviews:          req.ShowReviews,
		GoogleReviewsEnabled: req.GoogleReviewsEnabled,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}

	if err := database.CreateBusiness(business); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create business"})
		return
	}

	c.JSON(http.StatusCreated, business)
}

// GetMyBusinesses retrieves all businesses owned by the authenticated user
func GetMyBusinesses(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businesses, err := database.GetBusinessByOwnerAddress(userAddress.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve businesses"})
		return
	}

	c.JSON(http.StatusOK, businesses)
}

// GetBusiness retrieves a specific business by ID
func GetBusiness(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve business"})
		return
	}

	c.JSON(http.StatusOK, business)
}

// UpdateBusiness updates an existing business
func UpdateBusiness(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Check if business exists and user owns it
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve business"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You don't own this business"})
		return
	}

	var req UpdateBusinessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update business fields
	if req.Name != "" {
		business.Name = req.Name
	}
	if req.Logo != "" {
		business.Logo = req.Logo
	}
	business.Address = req.Address
	if req.SettlementAddr != "" {
		if !isValidEthereumAddress(req.SettlementAddr) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid settlement address"})
			return
		}
		business.SettlementAddr = req.SettlementAddr
	}
	if req.TippingAddr != "" {
		if !isValidEthereumAddress(req.TippingAddr) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tipping address"})
			return
		}
		business.TippingAddr = req.TippingAddr
	}
	business.TaxRate = req.TaxRate
	business.ServiceFeeRate = req.ServiceFeeRate
	business.TaxInclusive = req.TaxInclusive
	business.ServiceInclusive = req.ServiceInclusive
	
	// Update new fields
	if req.Description != "" {
		business.Description = req.Description
	}
	if req.CustomURL != "" {
		// Check if custom URL is already taken by another business
		if err := validateCustomURL(req.CustomURL, uint(businessID)); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		business.CustomURL = req.CustomURL
	}
	if req.Phone != "" {
		business.Phone = req.Phone
	}
	if req.Website != "" {
		business.Website = req.Website
	}
	if req.SocialMedia != "" {
		business.SocialMedia = req.SocialMedia
	}
	if req.BannerImages != "" {
		business.BannerImages = req.BannerImages
	}
	business.BusinessPageEnabled = req.BusinessPageEnabled
	business.ShowReviews = req.ShowReviews
	business.GoogleReviewsEnabled = req.GoogleReviewsEnabled
	
	business.UpdatedAt = time.Now()

	if err := database.UpdateBusiness(business); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update business"})
		return
	}

	c.JSON(http.StatusOK, business)
}

// DeleteBusiness soft deletes a business
func DeleteBusiness(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Check if business exists and user owns it
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve business"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You don't own this business"})
		return
	}

	if err := database.DeleteBusiness(uint(businessID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete business"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Business deleted successfully"})
}

// Menu management

// CreateMenuRequest represents a request to create/update a menu
type CreateMenuRequest struct {
	Categories []database.MenuCategory `json:"categories" binding:"required"`
}

// CreateMenu creates or updates a menu for a business
func CreateMenu(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Check if business exists and user owns it
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve business"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You don't own this business"})
		return
	}

	var req CreateMenuRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if menu already exists
	existingMenu, _, err := database.GetMenuByBusinessID(uint(businessID))
	if err != nil && !strings.Contains(err.Error(), "not found") {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check existing menu"})
		return
	}

	if existingMenu != nil {
		// Update existing menu
		existingMenu.UpdatedAt = time.Now()
		if err := database.UpdateMenu(existingMenu, req.Categories); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update menu"})
			return
		}
		c.JSON(http.StatusOK, existingMenu)
	} else {
		// Create new menu
		menu := &database.Menu{
			BusinessID: uint(businessID),
			IsActive:   true,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := database.CreateMenu(menu, req.Categories); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create menu"})
			return
		}
		c.JSON(http.StatusCreated, menu)
	}
}

// GetMenu retrieves a menu for a business with optional language translation
func GetMenu(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Get optional language parameter
	languageCode := c.Query("language")
	if languageCode == "" {
		languageCode = "en" // Default to English
	}

	menu, categories, err := database.GetMenuByBusinessID(uint(businessID))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			// Return empty menu structure instead of error
			c.JSON(http.StatusOK, gin.H{
				"id":          0,
				"business_id": businessID,
				"categories":  []interface{}{},
				"is_active":   true,
				"created_at":  "",
				"updated_at":  "",
				"language":    languageCode,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve menu"})
		return
	}

	// If language is not English, apply translations
	if languageCode != "en" {
		categories = applyTranslationsToMenu(categories, languageCode)
	}

	response := struct {
		*database.Menu
		ParsedCategories []database.MenuCategory `json:"parsed_categories"`
		Language         string                  `json:"language"`
	}{
		Menu:             menu,
		ParsedCategories: categories,
		Language:         languageCode,
	}

	c.JSON(http.StatusOK, response)
}

// applyTranslationsToMenu applies translations to menu categories and items
func applyTranslationsToMenu(categories []database.MenuCategory, languageCode string) []database.MenuCategory {
	db := database.GetDBWrapper()
	translatedCategories := make([]database.MenuCategory, len(categories))
	
	log.Printf("🔍 Starting translation retrieval for language: %s", languageCode)
	
	for i, category := range categories {
		translatedCategory := category
		log.Printf("🔄 Retrieving translations for category %d: '%s'", i, category.Name)
		
		// Translate category name
		var categoryNameTranslation database.Translation
		if err := db.GetGorm().Where("entity_type = ? AND entity_id = ? AND field_name = ? AND language_code = ?", 
			"category", i, "name", languageCode).Order("id DESC").First(&categoryNameTranslation).Error; err == nil {
			log.Printf("✅ Found category name translation: '%s' -> '%s'", category.Name, categoryNameTranslation.TranslatedText)
			translatedCategory.Name = categoryNameTranslation.TranslatedText
		} else {
			log.Printf("❌ No category name translation found for entity_id=%d, field=name, language=%s", i, languageCode)
		}
		
		// Translate category description
		var categoryDescTranslation database.Translation
		if err := db.GetGorm().Where("entity_type = ? AND entity_id = ? AND field_name = ? AND language_code = ?", 
			"category", i, "description", languageCode).Order("id DESC").First(&categoryDescTranslation).Error; err == nil {
			log.Printf("✅ Found category description translation: '%s' -> '%s'", category.Description, categoryDescTranslation.TranslatedText)
			translatedCategory.Description = categoryDescTranslation.TranslatedText
		} else {
			log.Printf("❌ No category description translation found for entity_id=%d, field=description, language=%s", i, languageCode)
		}
		
		// Translate menu items
		translatedItems := make([]database.MenuItem, len(category.Items))
		for j, item := range category.Items {
			translatedItem := item
			
			// Use position-based ID: categoryIndex * 1000 + itemIndex
			entityID := i*1000 + j
			log.Printf("🍽️ Retrieving translations for item %d in category %d: '%s' (entity_id=%d)", j, i, item.Name, entityID)
			
			// Translate item name
			var itemNameTranslation database.Translation
			if err := db.GetGorm().Where("entity_type = ? AND entity_id = ? AND field_name = ? AND language_code = ?", 
				"menu_item", entityID, "name", languageCode).Order("id DESC").First(&itemNameTranslation).Error; err == nil {
				log.Printf("✅ Found item name translation: '%s' -> '%s'", item.Name, itemNameTranslation.TranslatedText)
				translatedItem.Name = itemNameTranslation.TranslatedText
			} else {
				log.Printf("❌ No item name translation found for entity_id=%d, field=name, language=%s", entityID, languageCode)
			}
			
			// Translate item description
			var itemDescTranslation database.Translation
			if err := db.GetGorm().Where("entity_type = ? AND entity_id = ? AND field_name = ? AND language_code = ?", 
				"menu_item", entityID, "description", languageCode).Order("id DESC").First(&itemDescTranslation).Error; err == nil {
				log.Printf("✅ Found item description translation: '%s' -> '%s'", item.Description, itemDescTranslation.TranslatedText)
				translatedItem.Description = itemDescTranslation.TranslatedText
			} else {
				log.Printf("❌ No item description translation found for entity_id=%d, field=description, language=%s", entityID, languageCode)
			}

			// Translate item options
			translatedOptions := make([]database.MenuItemOption, len(item.Options))
			for k, option := range item.Options {
				translatedOption := option
				optionEntityID := entityID*1000 + k
				
				var optionTranslation database.Translation
				if err := db.GetGorm().Where("entity_type = ? AND entity_id = ? AND field_name = ? AND language_code = ?", 
					"menu_item_option", optionEntityID, "name", languageCode).Order("id DESC").First(&optionTranslation).Error; err == nil {
					log.Printf("✅ Found option translation: '%s' -> '%s'", option.Name, optionTranslation.TranslatedText)
					translatedOption.Name = optionTranslation.TranslatedText
				} else {
					log.Printf("❌ No option translation found for entity_id=%d, field=name, language=%s", optionEntityID, languageCode)
				}
				translatedOptions[k] = translatedOption
			}
			translatedItem.Options = translatedOptions

			// Translate allergens
			translatedAllergens := make([]string, len(item.Allergens))
			for k, allergen := range item.Allergens {
				allergenEntityID := entityID*10000 + k
				
				var allergenTranslation database.Translation
				if err := db.GetGorm().Where("entity_type = ? AND entity_id = ? AND field_name = ? AND language_code = ?", 
					"allergen", allergenEntityID, "name", languageCode).Order("id DESC").First(&allergenTranslation).Error; err == nil {
					log.Printf("✅ Found allergen translation: '%s' -> '%s'", allergen, allergenTranslation.TranslatedText)
					translatedAllergens[k] = allergenTranslation.TranslatedText
				} else {
					log.Printf("❌ No allergen translation found for entity_id=%d, field=name, language=%s", allergenEntityID, languageCode)
					translatedAllergens[k] = allergen // Keep original if no translation
				}
			}
			translatedItem.Allergens = translatedAllergens

			// Translate dietary tags
			translatedTags := make([]string, len(item.DietaryTags))
			for k, tag := range item.DietaryTags {
				tagEntityID := entityID*100000 + k
				
				var tagTranslation database.Translation
				if err := db.GetGorm().Where("entity_type = ? AND entity_id = ? AND field_name = ? AND language_code = ?", 
					"dietary_tag", tagEntityID, "name", languageCode).Order("id DESC").First(&tagTranslation).Error; err == nil {
					log.Printf("✅ Found dietary tag translation: '%s' -> '%s'", tag, tagTranslation.TranslatedText)
					translatedTags[k] = tagTranslation.TranslatedText
				} else {
					log.Printf("❌ No dietary tag translation found for entity_id=%d, field=name, language=%s", tagEntityID, languageCode)
					translatedTags[k] = tag // Keep original if no translation
				}
			}
			translatedItem.DietaryTags = translatedTags
			
			translatedItems[j] = translatedItem
		}
		
		translatedCategory.Items = translatedItems
		translatedCategories[i] = translatedCategory
	}
	
	return translatedCategories
}

// TranslateMenu translates the entire menu for a business into a specific language
func TranslateMenu(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	var req struct {
		LanguageCode string `json:"language_code" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get the menu
	_, categories, err := database.GetMenuByBusinessID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Menu not found"})
		return
	}

	// Get database wrapper and translation service
	db := database.GetDBWrapper()
	servicesTranslationService := GetTranslationService()
	
	if servicesTranslationService == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Translation service not available"})
		return
	}

	// Translate all categories and items
	for i, category := range categories {
		log.Printf("🔄 Processing category %d: '%s'", i, category.Name)
		
		// Translate category name
		if category.Name != "" {
			// Get translation using Google Translate
			translatedTexts, err := servicesTranslationService.TranslateText(category.Name, []string{req.LanguageCode})
			if err == nil && translatedTexts[req.LanguageCode] != "" {
				translation := &database.Translation{
					EntityType:        "category",
					EntityID:          uint(i),
					FieldName:         "name",
					LanguageCode:      req.LanguageCode,
					OriginalText:      category.Name,
					TranslatedText:    translatedTexts[req.LanguageCode],
					IsAutoTranslated:  true,
					TranslationSource: "google_translate",
				}
				log.Printf("💾 Storing category name translation: entity_id=%d, original='%s', translated='%s'", 
					i, category.Name, translatedTexts[req.LanguageCode])
				db.TranslationService.SaveTranslation(translation)
			} else {
				log.Printf("❌ Failed to translate category name '%s': %v", category.Name, err)
			}
		}

		// Translate category description
		if category.Description != "" {
			translatedTexts, err := servicesTranslationService.TranslateText(category.Description, []string{req.LanguageCode})
			if err == nil && translatedTexts[req.LanguageCode] != "" {
				translation := &database.Translation{
					EntityType:        "category",
					EntityID:          uint(i),
					FieldName:         "description",
					LanguageCode:      req.LanguageCode,
					OriginalText:      category.Description,
					TranslatedText:    translatedTexts[req.LanguageCode],
					IsAutoTranslated:  true,
					TranslationSource: "google_translate",
				}
				log.Printf("💾 Storing category description translation: entity_id=%d, original='%s', translated='%s'", 
					i, category.Description, translatedTexts[req.LanguageCode])
				db.TranslationService.SaveTranslation(translation)
			} else {
				log.Printf("❌ Failed to translate category description '%s': %v", category.Description, err)
			}
		}

		// Translate menu items
		for j, item := range category.Items {
			// Use position-based ID: categoryIndex * 1000 + itemIndex
			entityID := i*1000 + j
			log.Printf("🍽️ Processing item %d in category %d: '%s' (entity_id=%d)", j, i, item.Name, entityID)

			// Translate item name
			if item.Name != "" {
				translatedTexts, err := servicesTranslationService.TranslateText(item.Name, []string{req.LanguageCode})
				if err == nil && translatedTexts[req.LanguageCode] != "" {
					translation := &database.Translation{
						EntityType:        "menu_item",
						EntityID:          uint(entityID),
						FieldName:         "name",
						LanguageCode:      req.LanguageCode,
						OriginalText:      item.Name,
						TranslatedText:    translatedTexts[req.LanguageCode],
						IsAutoTranslated:  true,
						TranslationSource: "google_translate",
					}
					log.Printf("💾 Storing item name translation: entity_id=%d, original='%s', translated='%s'", 
						entityID, item.Name, translatedTexts[req.LanguageCode])
					db.TranslationService.SaveTranslation(translation)
				} else {
					log.Printf("❌ Failed to translate item name '%s' to %s: %v", item.Name, req.LanguageCode, err)
				}
			}

			// Translate item description
			if item.Description != "" {
				translatedTexts, err := servicesTranslationService.TranslateText(item.Description, []string{req.LanguageCode})
				if err == nil && translatedTexts[req.LanguageCode] != "" {
					translation := &database.Translation{
						EntityType:        "menu_item",
						EntityID:          uint(entityID),
						FieldName:         "description",
						LanguageCode:      req.LanguageCode,
						OriginalText:      item.Description,
						TranslatedText:    translatedTexts[req.LanguageCode],
						IsAutoTranslated:  true,
						TranslationSource: "google_translate",
					}
					log.Printf("💾 Storing item description translation: entity_id=%d, original='%s', translated='%s'", 
						entityID, item.Description, translatedTexts[req.LanguageCode])
					db.TranslationService.SaveTranslation(translation)
				} else {
					log.Printf("❌ Failed to translate item description '%s': %v", item.Description, err)
				}
			}

			// Translate item options
			for k, option := range item.Options {
				if option.Name != "" {
					optionEntityID := entityID*1000 + k // Unique ID for each option
					translatedTexts, err := servicesTranslationService.TranslateText(option.Name, []string{req.LanguageCode})
					if err == nil && translatedTexts[req.LanguageCode] != "" {
						translation := &database.Translation{
							EntityType:        "menu_item_option",
							EntityID:          uint(optionEntityID),
							FieldName:         "name",
							LanguageCode:      req.LanguageCode,
							OriginalText:      option.Name,
							TranslatedText:    translatedTexts[req.LanguageCode],
							IsAutoTranslated:  true,
							TranslationSource: "google_translate",
						}
						log.Printf("💾 Storing option translation: entity_id=%d, original='%s', translated='%s'", 
							optionEntityID, option.Name, translatedTexts[req.LanguageCode])
						db.TranslationService.SaveTranslation(translation)
					} else {
						log.Printf("❌ Failed to translate option '%s': %v", option.Name, err)
					}
				}
			}

			// Translate allergens
			for k, allergen := range item.Allergens {
				if allergen != "" {
					allergenEntityID := entityID*10000 + k // Unique ID for each allergen
					translatedTexts, err := servicesTranslationService.TranslateText(allergen, []string{req.LanguageCode})
					if err == nil && translatedTexts[req.LanguageCode] != "" {
						translation := &database.Translation{
							EntityType:        "allergen",
							EntityID:          uint(allergenEntityID),
							FieldName:         "name",
							LanguageCode:      req.LanguageCode,
							OriginalText:      allergen,
							TranslatedText:    translatedTexts[req.LanguageCode],
							IsAutoTranslated:  true,
							TranslationSource: "google_translate",
						}
						log.Printf("💾 Storing allergen translation: entity_id=%d, original='%s', translated='%s'", 
							allergenEntityID, allergen, translatedTexts[req.LanguageCode])
						db.TranslationService.SaveTranslation(translation)
					} else {
						log.Printf("❌ Failed to translate allergen '%s': %v", allergen, err)
					}
				}
			}

			// Translate dietary tags
			for k, tag := range item.DietaryTags {
				if tag != "" {
					tagEntityID := entityID*100000 + k // Unique ID for each dietary tag
					translatedTexts, err := servicesTranslationService.TranslateText(tag, []string{req.LanguageCode})
					if err == nil && translatedTexts[req.LanguageCode] != "" {
						translation := &database.Translation{
							EntityType:        "dietary_tag",
							EntityID:          uint(tagEntityID),
							FieldName:         "name",
							LanguageCode:      req.LanguageCode,
							OriginalText:      tag,
							TranslatedText:    translatedTexts[req.LanguageCode],
							IsAutoTranslated:  true,
							TranslationSource: "google_translate",
						}
						log.Printf("💾 Storing dietary tag translation: entity_id=%d, original='%s', translated='%s'", 
							tagEntityID, tag, translatedTexts[req.LanguageCode])
						db.TranslationService.SaveTranslation(translation)
					} else {
						log.Printf("❌ Failed to translate dietary tag '%s': %v", tag, err)
					}
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Menu translated successfully",
		"language_code": req.LanguageCode,
		"business_id": businessID,
	})
}

// Helper function to validate Ethereum addresses
func isValidEthereumAddress(address string) bool {
	// Basic validation - starts with 0x and is 42 characters long
	if len(address) != 42 {
		return false
	}
	if !strings.HasPrefix(address, "0x") {
		return false
	}
	// Additional validation could be added here (checksum, etc.)
	return true
}

// Phase 2: Enhanced Menu Management API Endpoints

// Menu category request structures
type AddCategoryRequest struct {
	Name        string                `json:"name" binding:"required"`
	Description string                `json:"description"`
	Items       []database.MenuItem   `json:"items"`
}

type UpdateCategoryRequest struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Items       []database.MenuItem   `json:"items"`
}

// Menu item request structures
type AddMenuItemRequest struct {
	CategoryIndex int                 `json:"category_index" binding:"min=0"`
	Item          database.MenuItem   `json:"item" binding:"required"`
}

type UpdateMenuItemRequest struct {
	CategoryIndex int                 `json:"category_index" binding:"min=0"`
	ItemIndex     int                 `json:"item_index" binding:"min=0"`
	Item          database.MenuItem   `json:"item" binding:"required"`
}

// AddMenuCategory adds a new category to a business menu
func AddMenuCategory(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this business"})
		return
	}

	var req AddCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	category := database.MenuCategory{
		Name:        req.Name,
		Description: req.Description,
		Items:       req.Items,
	}

	if err := database.AddMenuCategory(uint(businessID), category); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Auto-translate the new category
	go func() {
		if err := autoTranslateCategory(uint(businessID), category.Name, category.Description); err != nil {
			log.Printf("Failed to translate category: %v", err)
		}
	}()

	c.JSON(http.StatusCreated, gin.H{"message": "Category added successfully"})
}

// UpdateMenuCategory updates a specific category in a business menu
func UpdateMenuCategory(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	categoryIndex, err := strconv.Atoi(c.Param("category_index"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category index"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this business"})
		return
	}

	var req UpdateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	category := database.MenuCategory{
		Name:        req.Name,
		Description: req.Description,
		Items:       req.Items,
	}

	if err := database.UpdateMenuCategory(uint(businessID), categoryIndex, category); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Category updated successfully"})
}

// DeleteMenuCategory removes a category from a business menu
func DeleteMenuCategory(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	categoryIndex, err := strconv.Atoi(c.Param("category_index"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category index"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this business"})
		return
	}

	if err := database.DeleteMenuCategory(uint(businessID), categoryIndex); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Category deleted successfully"})
}

// AddMenuItem adds a new item to a menu category
func AddMenuItem(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this business"})
		return
	}

	var req AddMenuItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := database.AddMenuItem(uint(businessID), req.CategoryIndex, req.Item); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Auto-translate the new menu item
	go func() {
		if err := autoTranslateMenuItem(uint(businessID), req.Item.Name, req.Item.Description); err != nil {
			log.Printf("Failed to translate menu item: %v", err)
		}
	}()

	c.JSON(http.StatusCreated, gin.H{"message": "Menu item added successfully"})
}

// UpdateMenuItem updates a specific menu item
func UpdateMenuItem(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this business"})
		return
	}

	var req UpdateMenuItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := database.UpdateMenuItem(uint(businessID), req.CategoryIndex, req.ItemIndex, req.Item); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Menu item updated successfully"})
}

// DeleteMenuItem removes an item from a menu category
func DeleteMenuItem(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	categoryIndex, err := strconv.Atoi(c.Param("category_index"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid category index"})
		return
	}

	itemIndex, err := strconv.Atoi(c.Param("item_index"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid item index"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this business"})
		return
	}

	if err := database.DeleteMenuItem(uint(businessID), categoryIndex, itemIndex); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Menu item deleted successfully"})
}

// Bill management handlers

// CreateBillRequest represents the request to create a new bill
type CreateBillRequest struct {
	TableID   *uint                `json:"table_id"`
	CounterID *uint                `json:"counter_id"`
	Notes     string               `json:"notes"`
	Items     []database.BillItem  `json:"items"`
}

// UpdateBillRequest represents the request to update a bill
type UpdateBillRequest struct {
	Items []database.BillItem `json:"items"`
}

// AddBillItemRequest represents the request to add an item to a bill
type AddBillItemRequest struct {
	MenuItemID string                     `json:"menu_item_id" binding:"required"`
	Name       string                     `json:"name" binding:"required"`
	Price      float64                    `json:"price" binding:"required"`
	Quantity   int                        `json:"quantity" binding:"required"`
	Options    []database.MenuItemOption  `json:"options"`
}

// CreateBill creates a new bill for a business
func CreateBill(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	var req CreateBillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to access this business"})
		return
	}

	// Validate that either table_id or counter_id is provided, but not both
	if req.TableID == nil && req.CounterID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Either table_id or counter_id must be provided"})
		return
	}

	if req.TableID != nil && req.CounterID != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot specify both table_id and counter_id"})
		return
	}

	// Validate table if provided
	if req.TableID != nil {
		table, err := database.GetTableByID(*req.TableID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Table not found"})
			return
		}

		if table.BusinessID != uint(businessID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Table does not belong to this business"})
			return
		}

		// Check if table already has an open bill
		existingBill, _, err := database.GetOpenBillByTableID(*req.TableID)
		if err == nil && existingBill != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "Table already has an open bill"})
			return
		}
	}

	// Validate counter if provided
	if req.CounterID != nil {
		counter, err := database.GetCounterByID(*req.CounterID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Counter not found"})
			return
		}

		if counter.BusinessID != uint(businessID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Counter does not belong to this business"})
			return
		}

		if !counter.IsActive {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Counter is not active"})
			return
		}
	}

	// Calculate totals
	subtotal := 0.0
	for i := range req.Items {
		req.Items[i].Subtotal = req.Items[i].Price * float64(req.Items[i].Quantity)
		subtotal += req.Items[i].Subtotal
	}

	taxAmount := subtotal * business.TaxRate / 100
	serviceFeeAmount := subtotal * business.ServiceFeeRate / 100
	totalAmount := subtotal + taxAmount + serviceFeeAmount

	// Generate bill number
	billNumber := fmt.Sprintf("B%d-%d", businessID, time.Now().Unix())

	// Create bill
	bill := &database.Bill{
		BusinessID:       uint(businessID),
		CounterID:        req.CounterID,
		BillNumber:       billNumber,
		Notes:            req.Notes,
		Subtotal:         subtotal,
		TaxAmount:        taxAmount,
		ServiceFeeAmount: serviceFeeAmount,
		TotalAmount:      totalAmount,
		Status:           database.BillStatusOpen,
		SettlementAddr:   business.SettlementAddr,
		TippingAddr:      business.TippingAddr,
	}

	// Set TableID if provided
	if req.TableID != nil {
		bill.TableID = *req.TableID
	}

	if err := database.CreateBill(bill, req.Items); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}


	c.JSON(http.StatusCreated, gin.H{
		"bill":  bill,
		"items": req.Items,
	})
}

// GetBusinessBills retrieves all bills for a business
func GetBusinessBills(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to access this business"})
		return
	}

	bills, err := database.GetAllBillsByBusinessID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"bills": bills})
}

// GetOpenBusinessBills retrieves only open bills for a business (for table filtering)
func GetOpenBusinessBills(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	businessID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to access this business"})
		return
	}

	bills, err := database.GetOpenBillsByBusinessID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"bills": bills})
}

// GetBill retrieves a specific bill by ID
func GetBill(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	billID, err := strconv.ParseUint(c.Param("bill_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	bill, items, err := database.GetBillByID(uint(billID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(bill.BusinessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to access this bill"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bill":  bill,
		"items": items,
	})
}

// UpdateBill updates an existing bill
func UpdateBill(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	billID, err := strconv.ParseUint(c.Param("bill_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	var req UpdateBillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	bill, _, err := database.GetBillByID(uint(billID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(bill.BusinessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this bill"})
		return
	}

	if bill.Status != database.BillStatusOpen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot modify closed bill"})
		return
	}

	// Recalculate totals
	subtotal := 0.0
	for i := range req.Items {
		req.Items[i].Subtotal = req.Items[i].Price * float64(req.Items[i].Quantity)
		subtotal += req.Items[i].Subtotal
	}

	taxAmount := subtotal * business.TaxRate / 100
	serviceFeeAmount := subtotal * business.ServiceFeeRate / 100
	totalAmount := subtotal + taxAmount + serviceFeeAmount

	bill.Subtotal = subtotal
	bill.TaxAmount = taxAmount
	bill.ServiceFeeAmount = serviceFeeAmount
	bill.TotalAmount = totalAmount

	if err := database.UpdateBill(bill, req.Items); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bill":  bill,
		"items": req.Items,
	})
}

// AddBillItem adds an item to an existing bill
func AddBillItem(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	billID, err := strconv.ParseUint(c.Param("bill_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	var req AddBillItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	bill, items, err := database.GetBillByID(uint(billID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(bill.BusinessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this bill"})
		return
	}

	if bill.Status != database.BillStatusOpen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot modify closed bill"})
		return
	}

	// Create new bill item
	newItem := database.BillItem{
		ID:         fmt.Sprintf("item_%d", time.Now().UnixNano()),
		MenuItemID: req.MenuItemID,
		Name:       req.Name,
		Price:      req.Price,
		Quantity:   req.Quantity,
		Options:    req.Options,
		Subtotal:   req.Price * float64(req.Quantity),
	}

	// Add to existing items
	items = append(items, newItem)

	// Recalculate totals
	subtotal := 0.0
	for _, item := range items {
		subtotal += item.Subtotal
	}

	taxAmount := subtotal * business.TaxRate / 100
	serviceFeeAmount := subtotal * business.ServiceFeeRate / 100
	totalAmount := subtotal + taxAmount + serviceFeeAmount

	bill.Subtotal = subtotal
	bill.TaxAmount = taxAmount
	bill.ServiceFeeAmount = serviceFeeAmount
	bill.TotalAmount = totalAmount

	if err := database.UpdateBill(bill, items); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bill":  bill,
		"items": items,
	})
}

// RemoveBillItem removes an item from a bill
func RemoveBillItem(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	billID, err := strconv.ParseUint(c.Param("bill_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	itemID := c.Param("item_id")
	if itemID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Item ID is required"})
		return
	}

	bill, items, err := database.GetBillByID(uint(billID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(bill.BusinessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this bill"})
		return
	}

	if bill.Status != database.BillStatusOpen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot modify closed bill"})
		return
	}

	// Remove item from items slice
	var updatedItems []database.BillItem
	itemFound := false
	for _, item := range items {
		if item.ID != itemID {
			updatedItems = append(updatedItems, item)
		} else {
			itemFound = true
		}
	}

	if !itemFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "Item not found in bill"})
		return
	}

	// Recalculate totals
	subtotal := 0.0
	for _, item := range updatedItems {
		subtotal += item.Subtotal
	}

	taxAmount := subtotal * business.TaxRate / 100
	serviceFeeAmount := subtotal * business.ServiceFeeRate / 100
	totalAmount := subtotal + taxAmount + serviceFeeAmount

	bill.Subtotal = subtotal
	bill.TaxAmount = taxAmount
	bill.ServiceFeeAmount = serviceFeeAmount
	bill.TotalAmount = totalAmount

	if err := database.UpdateBill(bill, updatedItems); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bill":  bill,
		"items": updatedItems,
	})
}

// CloseBill closes a bill
func CloseBill(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	billID, err := strconv.ParseUint(c.Param("bill_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	bill, items, err := database.GetBillByID(uint(billID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(bill.BusinessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this bill"})
		return
	}

	if bill.Status != database.BillStatusOpen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Bill is already closed"})
		return
	}

	if err := database.CloseBill(uint(billID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get updated bill
	updatedBill, _, err := database.GetBillByID(uint(billID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve updated bill"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bill":  updatedBill,
		"items": items,
	})
}

// validateCustomURL checks if a custom URL is available for use
func validateCustomURL(customURL string, excludeBusinessID uint) error {
	if customURL == "" {
		return nil
	}

	// Basic validation - only allow alphanumeric characters and hyphens
	if matched, _ := regexp.MatchString("^[a-zA-Z0-9-]+$", customURL); !matched {
		return errors.New("custom URL can only contain letters, numbers, and hyphens")
	}

	// Check minimum length
	if len(customURL) < 2 {
		return errors.New("custom URL must be at least 2 characters long")
	}

	// Check if URL is already taken by another business
	var existingBusiness database.Business
	result := database.GetDB().Where("custom_url = ? AND id != ?", customURL, excludeBusinessID).First(&existingBusiness)
	if result.Error == nil {
		return errors.New("this custom URL is already taken")
	}

	return nil
}

// CheckCustomURLAvailability checks if a custom URL is available
func CheckCustomURLAvailability(c *gin.Context) {
	customURL := c.Query("url")
	businessIDStr := c.Query("exclude_business_id")
	
	if customURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "URL parameter is required"})
		return
	}

	var excludeBusinessID uint = 0
	if businessIDStr != "" {
		if id, err := strconv.ParseUint(businessIDStr, 10, 32); err == nil {
			excludeBusinessID = uint(id)
		}
	}

	// Validate the URL format and availability
	if err := validateCustomURL(customURL, excludeBusinessID); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"available": false,
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"available": true,
		"url": customURL,
	})
}

// Counter management handlers

// UpdateCounterSettingsRequest represents the request to update counter settings
type UpdateCounterSettingsRequest struct {
	CounterEnabled bool   `json:"counter_enabled"`
	CounterCount   int    `json:"counter_count" binding:"min=1,max=20"`
	CounterPrefix  string `json:"counter_prefix" binding:"required,max=5"`
}

// UpdateCounterSettings updates counter configuration for a business
func UpdateCounterSettings(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Get user address from context
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this business"})
		return
	}

	var req UpdateCounterSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update counter settings
	if err := database.UpdateBusinessCounters(uint(businessID), req.CounterEnabled, req.CounterCount, req.CounterPrefix); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update counter settings"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Counter settings updated successfully"})
}

// GetBusinessCounters retrieves all counters for a business
func GetBusinessCounters(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Get user address from context
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to access this business"})
		return
	}

	// Get counters
	counters, err := database.GetBusinessCounters(uint(businessID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get counters"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"counters": counters,
		"business": gin.H{
			"counter_enabled": business.CounterEnabled,
			"counter_count":   business.CounterCount,
			"counter_prefix":  business.CounterPrefix,
		},
	})
}

// GetAvailableCounters retrieves available counters for bill creation
func GetAvailableCounters(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Get user address from context
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to access this business"})
		return
	}

	// Check if counters are enabled
	if !business.CounterEnabled {
		c.JSON(http.StatusOK, gin.H{"counters": []database.Counter{}})
		return
	}

	// Get available counters
	counters, err := database.GetAvailableCounters(uint(businessID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get available counters"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"counters": counters})
}

// Payment control handlers

// MarkPaidRequest represents the request to mark a bill as paid
type MarkPaidRequest struct {
	PaymentMethod string  `json:"payment_method" binding:"required,oneof=cash card crypto"`
	AmountPaid    float64 `json:"amount_paid" binding:"required,min=0"`
	TipAmount     float64 `json:"tip_amount,omitempty"`
	Notes         string  `json:"notes,omitempty"`
}

// MarkBillAsPaid allows staff to mark a bill as paid
func MarkBillAsPaid(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	billID, err := strconv.ParseUint(c.Param("bill_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	var req MarkPaidRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get bill and verify ownership
	bill, _, err := database.GetBillByID(uint(billID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(bill.BusinessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to access this bill"})
		return
	}

	// Check if bill is already paid
	if bill.Status == database.BillStatusPaid || bill.Status == database.BillStatusClosed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Bill is already paid or closed"})
		return
	}

	// Update bill status and payment details
	err = database.MarkBillAsPaid(uint(billID), req.AmountPaid, req.TipAmount, req.PaymentMethod, req.Notes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to mark bill as paid"})
		return
	}

	// Get updated bill
	updatedBill, items, err := database.GetBillByID(uint(billID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve updated bill"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bill":  updatedBill,
		"items": items,
	})
}

// ApproveCashPayment allows staff to approve cash payments
func ApproveCashPayment(c *gin.Context) {
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	billID, err := strconv.ParseUint(c.Param("bill_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	var req MarkPaidRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Force payment method to cash for this endpoint
	req.PaymentMethod = "cash"

	// Get bill and verify ownership
	bill, _, err := database.GetBillByID(uint(billID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Verify business ownership
	business, err := database.GetBusinessByID(bill.BusinessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to access this bill"})
		return
	}

	// Check if bill is already paid
	if bill.Status == database.BillStatusPaid || bill.Status == database.BillStatusClosed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Bill is already paid or closed"})
		return
	}

	// Update bill status and payment details with cash approval
	notes := fmt.Sprintf("Cash payment approved by staff. %s", req.Notes)
	err = database.MarkBillAsPaid(uint(billID), req.AmountPaid, req.TipAmount, "cash", notes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to approve cash payment"})
		return
	}

	// Get updated bill
	updatedBill, items, err := database.GetBillByID(uint(billID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve updated bill"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bill":  updatedBill,
		"items": items,
	})
}

// Language and Currency Management Handlers

// GetSupportedLanguages returns all supported languages
func GetSupportedLanguages(c *gin.Context) {
	db := database.GetDBWrapper()
	languages, err := db.LanguageService.GetSupportedLanguages()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get supported languages"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"languages": languages})
}

// GetSupportedCurrencies returns all supported currencies
func GetSupportedCurrencies(c *gin.Context) {
	db := database.GetDBWrapper()
	currencies, err := db.CurrencyService.GetSupportedCurrencies()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get supported currencies"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"currencies": currencies})
}

// GetBusinessLanguages returns languages configured for a business
func GetBusinessLanguages(c *gin.Context) {
	businessID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Verify business ownership
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	db := database.GetDBWrapper()
	languages, err := db.LanguageService.GetBusinessLanguages(uint(businessID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get business languages"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"languages": languages})
}

// GetBusinessCurrencies returns currencies configured for a business
func GetBusinessCurrencies(c *gin.Context) {
	businessID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Verify business ownership
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	db := database.GetDBWrapper()
	currencies, err := db.CurrencyService.GetBusinessCurrencies(uint(businessID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get business currencies"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"currencies": currencies})
}

// SetBusinessLanguagesRequest represents the request to set business languages
type SetBusinessLanguagesRequest struct {
	LanguageCodes []string `json:"language_codes" binding:"required"`
	DefaultCode   string   `json:"default_code" binding:"required"`
}

// SetBusinessLanguages sets the languages for a business
func SetBusinessLanguages(c *gin.Context) {
	businessID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Verify business ownership
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	var req SetBusinessLanguagesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate that default language is in the list
	defaultFound := false
	for _, code := range req.LanguageCodes {
		if code == req.DefaultCode {
			defaultFound = true
			break
		}
	}
	if !defaultFound {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Default language must be in the language codes list"})
		return
	}

	db := database.GetDBWrapper()
	err = db.LanguageService.SetBusinessLanguages(uint(businessID), req.LanguageCodes, req.DefaultCode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set business languages"})
		return
	}

	// Auto-translate existing menu content when languages are updated
	go func() {
		if err := translateExistingMenuContent(uint(businessID)); err != nil {
			log.Printf("Failed to translate existing menu content for business %d: %v", businessID, err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"message": "Business languages updated successfully"})
}

// autoTranslateMenuItem creates translations for a menu item using the translation service
func autoTranslateMenuItem(businessID uint, name, description string) error {
	db := database.GetDBWrapper()
	translationService := services.NewTranslationService(db, "")
	
	// Use a hash of the name as ID to ensure consistency
	itemID := uint(len(name) + len(description)) // Simple but consistent ID
	
	// Use the translation service to handle the translation
	return translationService.TranslateMenuItem(businessID, itemID, name, description)
}

// autoTranslateCategory creates translations for a category using the translation service
func autoTranslateCategory(businessID uint, name, description string) error {
	db := database.GetDBWrapper()
	translationService := services.NewTranslationService(db, "")
	
	// Use a hash of the name as ID to ensure consistency
	categoryID := uint(len(name) + len(description)) // Simple but consistent ID
	
	// Use the translation service to handle the translation
	return translationService.TranslateCategory(businessID, categoryID, name, description)
}


// translateExistingMenuContent translates all existing menu content for a business
func translateExistingMenuContent(businessID uint) error {
	log.Printf("Starting translation of existing menu content for business %d", businessID)

	// Get the menu for this business
	_, categories, err := database.GetMenuByBusinessID(businessID)
	if err != nil {
		return fmt.Errorf("failed to get menu: %w", err)
	}

	// Translate all categories and their items
	for categoryIndex, category := range categories {
		categoryID := uint(categoryIndex + 1) // Use index + 1 as ID
		
		// Translate category
		if err := autoTranslateCategory(businessID, category.Name, category.Description); err != nil {
			log.Printf("Failed to translate category %d: %v", categoryID, err)
		}

		// Translate all items in this category
		for itemIndex, item := range category.Items {
			itemID := uint(categoryIndex*100 + itemIndex + 1) // Create unique item ID
			if err := autoTranslateMenuItem(businessID, item.Name, item.Description); err != nil {
				log.Printf("Failed to translate menu item %d: %v", itemID, err)
			}
		}
	}

	log.Printf("Completed translation of existing menu content for business %d", businessID)
	return nil
}

// SetBusinessCurrenciesRequest represents the request to set business currencies
type SetBusinessCurrenciesRequest struct {
	CurrencyCodes []string `json:"currency_codes" binding:"required"`
	PreferredCode string   `json:"preferred_code" binding:"required"`
}

// SetBusinessCurrencies sets the currencies for a business
func SetBusinessCurrencies(c *gin.Context) {
	businessID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Verify business ownership
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	var req SetBusinessCurrenciesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate that preferred currency is in the list
	preferredFound := false
	for _, code := range req.CurrencyCodes {
		if code == req.PreferredCode {
			preferredFound = true
			break
		}
	}
	if !preferredFound {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Preferred currency must be in the currency codes list"})
		return
	}

	db := database.GetDBWrapper()
	err = db.CurrencyService.SetBusinessCurrencies(uint(businessID), req.CurrencyCodes, req.PreferredCode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set business currencies"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Business currencies updated successfully"})
}
