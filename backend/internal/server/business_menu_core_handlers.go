package server

import (
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
	"github.com/stdevmac/payverge/backend/internal/utils"

	"github.com/gin-gonic/gin"
)

// stampOnboardingAsync runs the server-side onboarding completion check
// off the request path after a menu mutation that may have completed setup.
var runStampOnboardingAsync = logger.SafeGo
var runMenuCategoryTranslationAsync = logger.SafeGo

func stampOnboardingAsync(businessID uint) {
	runStampOnboardingAsync(func() {
		if _, err := services.StampOnboardingCompletedIfReady(businessID); err != nil {
			log.Printf("onboarding stamp after menu write failed for business %d: %v", businessID, err)
		}
	})
}

// resolveBusinessForMenu reads the :id route parameter (numeric DB id or
// slug), looks up the business and returns it. Writes the appropriate
// JSON error and returns ok=false on failure so callers can early-return.
func resolveBusinessForMenu(c *gin.Context) (*database.Business, bool) {
	identifier := utils.BusinessIdentifierFromParam(c, "id")
	if identifier == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return nil, false
	}
	business, err := database.GetBusinessByIdOrBusinessId(identifier)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve business"})
		}
		return nil, false
	}
	return business, true
}

// Menu management

// CreateMenuRequest represents a request to create/update a menu
type CreateMenuRequest struct {
	Categories          []database.MenuCategory `json:"categories" binding:"required"`
	ConfirmSanitization bool                    `json:"confirm_sanitization"`
	// Version enables optimistic-concurrency control for the whole-menu write
	// used by drag-reorder. When present and a menu already exists, the write
	// goes through the version-checked path (rejecting a stale reorder snapshot)
	// instead of a blind Save. Absent for legacy callers.
	Version *uint `json:"version"`
}

// CreateMenu creates or updates a menu for a business
func CreateMenu(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	var req CreateMenuRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	sanitized, sanitization := services.SanitizeMenuCategories(req.Categories)
	if refuseDemoMenuImages(c, business.ID, sanitized) {
		return
	}
	if sanitization.HasDrops() && !req.ConfirmSanitization {
		respondMenuSanitizationReview(c, sanitization)
		return
	}

	// Check if menu already exists
	existingMenu, _, err := database.GetMenuByBusinessID(business.ID)
	if err != nil && !strings.Contains(err.Error(), "not found") {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check existing menu"})
		return
	}

	if existingMenu != nil {
		// Versioned path (drag-reorder): sanitize the incoming tree and apply it
		// under optimistic CAS so a stale reorder snapshot can't silently
		// clobber a concurrent edit — the same protection the ID-based item
		// routes and the AI-import path already have. Returns the bumped version
		// so the client can keep its optimistic-lock counter in sync (avoids a
		// false 409 on the next edit).
		if req.Version != nil {
			newVersion, err := database.ApplyMenuCategories(business.ID, sanitized, *req.Version)
			if err != nil {
				if errors.Is(err, database.ErrMenuVersionConflict) {
					respondVersionConflict(c)
					return
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update menu"})
				return
			}
			services.InvalidatePricingCache(business.ID)
			stampOnboardingAsync(business.ID)
			c.JSON(http.StatusOK, gin.H{
				"message":      "Menu updated successfully",
				"version":      newVersion,
				"sanitization": sanitization,
			})
			return
		}

		// Legacy blind update (no client version): sanitize then write.
		existingMenu.UpdatedAt = time.Now()
		if err := database.UpdateMenu(existingMenu, sanitized); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update menu"})
			return
		}
		services.InvalidatePricingCache(business.ID)
		stampOnboardingAsync(business.ID)
		c.JSON(http.StatusOK, struct {
			*database.Menu
			Sanitization services.GeneratedMenuSanitizeReport `json:"sanitization"`
		}{Menu: existingMenu, Sanitization: sanitization})
	} else {
		// Create new menu
		menu := &database.Menu{
			BusinessID: business.ID,
			IsActive:   true,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := database.CreateMenu(menu, sanitized); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create menu"})
			return
		}
		services.InvalidatePricingCache(business.ID)
		stampOnboardingAsync(business.ID)
		c.JSON(http.StatusCreated, struct {
			*database.Menu
			Sanitization services.GeneratedMenuSanitizeReport `json:"sanitization"`
		}{Menu: menu, Sanitization: sanitization})
	}
}

// ReorderMenuRequest is the body for the granular reorder endpoint (§3.7 fix 4).
// Scope is "category" (reorder categories, category_id ignored) or "item"
// (reorder items within category_id). A single drag sends only {scope, from,
// to, version} instead of re-uploading the whole menu.
type ReorderMenuRequest struct {
	Scope      string `json:"scope" binding:"required"`
	CategoryID string `json:"category_id"`
	From       int    `json:"from"`
	To         int    `json:"to"`
	Version    uint   `json:"version" binding:"required"`
}

// ReorderMenu applies a single category- or item-level move server-side under
// optimistic CAS, so a drag-reorder no longer uploads the entire menu document.
func ReorderMenu(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	var req ReorderMenuRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	var (
		newVersion uint
		err        error
	)
	switch req.Scope {
	case "category":
		newVersion, err = database.ReorderMenuCategory(business.ID, req.From, req.To, req.Version)
	case "item":
		if req.CategoryID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "category_id is required for item reorder"})
			return
		}
		newVersion, err = database.ReorderMenuItem(business.ID, req.CategoryID, req.From, req.To, req.Version)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "scope must be 'category' or 'item'"})
		return
	}

	if err != nil {
		if errors.Is(err, database.ErrMenuVersionConflict) {
			respondVersionConflict(c)
			return
		}
		if errors.Is(err, database.ErrCategoryNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Category not found"})
			return
		}
		if errors.Is(err, database.ErrReorderOutOfRange) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "reorder index out of range"})
			return
		}
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not reorder menu")
		return
	}

	services.InvalidatePricingCache(business.ID)
	c.JSON(http.StatusOK, gin.H{"message": "Menu reordered successfully", "version": newVersion})
}

// GetMenu retrieves a menu for a business with optional language translation
func GetMenu(c *gin.Context) {
	business, ok := resolveBusinessForMenu(c)
	if !ok {
		return
	}
	businessID := uint64(business.ID)

	businessDefaultLanguage := business.DefaultLanguage
	if businessDefaultLanguage == "" {
		businessDefaultLanguage = "en" // Fallback to English if not set
	}

	// Get optional language parameter
	languageCode := c.Query("language")
	if languageCode == "" {
		languageCode = businessDefaultLanguage
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

	// Apply translations if requested language is different from business default language
	if languageCode != businessDefaultLanguage {
		var missing bool
		categories, missing = applyTranslationsToMenu(uint(businessID), categories, languageCode)
		if missing {
			enqueueMenuTranslationBackfill(uint(businessID), languageCode)
		}
	}

	// Explicit response shape (§3.7 wire-contract slim). The old shape embedded
	// *database.Menu, which serialized BOTH the raw `categories` JSON string (a
	// second, redundant ~1MB copy of the menu the frontend never reads — it
	// consumes parsed_categories) AND a full zero-value `business` object
	// (Menu.Business has json:"business,omitempty", but struct omitempty is a
	// no-op so a ~200-field blank Business always serialized). We emit only the
	// menu metadata the client actually uses plus parsed_categories.
	orderability := services.ProjectOrderability(business, categories, services.OrderabilityContextOperator, true)
	// #727: the operator menu used to report the STORED is_available while
	// item_orderability on the same response said inventory_out — so Menu
	// Builder painted the dish sellable ("Mark 86") right next to its own
	// "Unavailable · inventory" pill, while guests reading the same dish got
	// is_available:false. The builder payload now carries the effective answer,
	// identical to guest, and the stored flag moves to manual_available for the
	// controls that still edit it.
	services.ApplyInventorySellability(categories, orderability, true)

	response := struct {
		ID               uint                             `json:"id"`
		BusinessID       uint                             `json:"business_id"`
		IsActive         bool                             `json:"is_active"`
		CreatedAt        time.Time                        `json:"created_at"`
		UpdatedAt        time.Time                        `json:"updated_at"`
		ParsedCategories []database.MenuCategory          `json:"parsed_categories"`
		ItemOrderability map[string]services.Orderability `json:"item_orderability"`
		Language         string                           `json:"language"`
		Version          uint                             `json:"version"`
	}{
		ID:               menu.ID,
		BusinessID:       menu.BusinessID,
		IsActive:         menu.IsActive,
		CreatedAt:        menu.CreatedAt,
		UpdatedAt:        menu.UpdatedAt,
		ParsedCategories: categories,
		ItemOrderability: orderability,
		Language:         languageCode,
		Version:          menu.Version,
	}

	c.JSON(http.StatusOK, response)
}

// applyTranslationsToMenu applies translations to menu categories and items
func applyTranslationsToMenu(businessID uint, categories []database.MenuCategory, languageCode string) ([]database.MenuCategory, bool) {
	return applyTranslationsToMenuWithLookup(
		newTranslationLookup(database.GetDBWrapper(), businessID, languageCode, menuTranslationLookupScope(categories)),
		categories,
	)
}

// applyTranslationsToMenuWithLookup overlays a shared translation snapshot on
// menu categories. Guest menu routes use the same snapshot for menu items and
// promotions so the response body and its cache revision are derived from one
// translation read.
func applyTranslationsToMenuWithLookup(lookup *translationLookup, categories []database.MenuCategory) ([]database.MenuCategory, bool) {
	translatedCategories := make([]database.MenuCategory, len(categories))
	missing := false

	for i, category := range categories {
		translatedCategory := category
		categoryID := uint(i)

		// getFresh (not get): only accept a stored translation when its
		// OriginalText still matches the current source text. A reorder or an
		// in-place edit reuses the same position-based key with different source
		// text; comparing OriginalText makes that mismatch read as MISSING so the
		// existing backfill regenerates it — no re-keying of stored rows needed.
		if value, ok := getFreshMenuTranslation(lookup, "category", categoryID, category.ID, "name", category.Name); ok {
			translatedCategory.Name = value
		} else if strings.TrimSpace(category.Name) != "" {
			missing = true
		}

		if value, ok := getFreshMenuTranslation(lookup, "category", categoryID, category.ID, "description", category.Description); ok {
			translatedCategory.Description = value
		} else if strings.TrimSpace(category.Description) != "" {
			missing = true
		}

		translatedItems := make([]database.MenuItem, len(category.Items))
		for j, item := range category.Items {
			translatedItem := item

			// Position-based ID: categoryIndex * 1000 + itemIndex. Must stay in
			// lockstep with the write side (performBatchTranslation / TranslateMenu).
			entityID := uint(i*1000 + j)

			if value, ok := getFreshMenuTranslation(lookup, "menu_item", entityID, item.ID, "name", item.Name); ok {
				translatedItem.Name = value
			} else if strings.TrimSpace(item.Name) != "" {
				missing = true
			}

			if value, ok := getFreshMenuTranslation(lookup, "menu_item", entityID, item.ID, "description", item.Description); ok {
				translatedItem.Description = value
			} else if strings.TrimSpace(item.Description) != "" {
				missing = true
			}

			// Options, allergens and dietary tags are best-effort. The batch
			// translator does not populate them, so a miss here must NOT flag the
			// menu as needing a backfill (that would loop forever); fall back to
			// the original text instead.
			translatedOptions := make([]database.MenuItemOption, len(item.Options))
			for k, option := range item.Options {
				translatedOption := option
				optionEntityID := entityID*1000 + uint(k)
				if value, ok := getMenuTranslation(lookup, "menu_item_option", optionEntityID, option.ID, "name"); ok {
					translatedOption.Name = value
				}
				translatedOptions[k] = translatedOption
			}
			translatedItem.Options = translatedOptions

			translatedAllergens := make([]string, len(item.Allergens))
			for k, allergen := range item.Allergens {
				if value, ok := lookup.get("allergen", entityID*10000+uint(k), "name"); ok {
					translatedAllergens[k] = value
				} else {
					translatedAllergens[k] = allergen
				}
			}
			translatedItem.Allergens = translatedAllergens

			translatedTags := make([]string, len(item.DietaryTags))
			for k, tag := range item.DietaryTags {
				if value, ok := lookup.get("dietary_tag", entityID*100000+uint(k), "name"); ok {
					translatedTags[k] = value
				} else {
					translatedTags[k] = tag
				}
			}
			translatedItem.DietaryTags = translatedTags

			translatedItems[j] = translatedItem
		}

		translatedCategory.Items = translatedItems
		translatedCategories[i] = translatedCategory
	}

	return translatedCategories, missing
}

// getFreshMenuTranslation accepts both identity schemes present in the
// translation table. Menu translation writers use a position-derived key;
// imported/persisted menu payloads may carry a numeric database ID and
// corresponding rows use that actual ID. Try the position key first, then
// fall back to the actual ID when it is different. A stale positional row must not prevent a fresh actual-ID row
// from applying.
func getFreshMenuTranslation(lookup *translationLookup, entityType string, positionID uint, actualID, fieldName, currentSource string) (string, bool) {
	if value, ok := lookup.getFresh(entityType, positionID, fieldName, currentSource); ok {
		return value, true
	}
	if actualEntityID, ok := numericMenuEntityID(actualID); ok && actualEntityID != positionID {
		return lookup.getFresh(entityType, actualEntityID, fieldName, currentSource)
	}
	return "", false
}

func getMenuTranslation(lookup *translationLookup, entityType string, positionID uint, actualID, fieldName string) (string, bool) {
	if value, ok := lookup.get(entityType, positionID, fieldName); ok {
		return value, true
	}
	if actualEntityID, ok := numericMenuEntityID(actualID); ok && actualEntityID != positionID {
		return lookup.get(entityType, actualEntityID, fieldName)
	}
	return "", false
}

// TranslateMenu translates the entire menu for a business into a specific language
func TranslateMenu(c *gin.Context) {
	// requireBusinessAccess (not resolve-only) so ownership is enforced even if
	// middleware composition changes; RBAC already gates menu:translate on the route.
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}
	businessID := uint64(business.ID)

	var req struct {
		LanguageCode string `json:"language_code" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	// Get business info to determine source language
	business, err := database.GetBusinessByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve business"})
		return
	}

	sourceLanguage := menuTranslationSourceLanguage(business.DefaultLanguage)

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
			translatedTexts, err := servicesTranslationService.TranslateTextWithSource(category.Name, sourceLanguage, []string{req.LanguageCode})
			if err == nil && persistableTranslation(category.Name, translatedTexts[req.LanguageCode]) {
				translation := &database.Translation{
					BusinessID:        uint(businessID),
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
				if err := db.TranslationService.SaveTranslation(translation); err != nil {
					log.Printf("WARNING: failed to save translation for category name: %v", err)
				}
			} else {
				log.Printf("❌ Failed to translate category name '%s': %v", category.Name, err)
			}
		}

		// Translate category description
		if category.Description != "" {
			translatedTexts, err := servicesTranslationService.TranslateTextWithSource(category.Description, sourceLanguage, []string{req.LanguageCode})
			if err == nil && persistableTranslation(category.Description, translatedTexts[req.LanguageCode]) {
				translation := &database.Translation{
					BusinessID:        uint(businessID),
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
				if err := db.TranslationService.SaveTranslation(translation); err != nil {
					log.Printf("WARNING: failed to save translation for category description: %v", err)
				}
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
				translatedTexts, err := servicesTranslationService.TranslateTextWithSource(item.Name, sourceLanguage, []string{req.LanguageCode})
				if err == nil && persistableTranslation(item.Name, translatedTexts[req.LanguageCode]) {
					translation := &database.Translation{
						BusinessID:        uint(businessID),
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
					if err := db.TranslationService.SaveTranslation(translation); err != nil {
						log.Printf("WARNING: failed to save translation for item name: %v", err)
					}
				} else {
					log.Printf("❌ Failed to translate item name '%s' to %s: %v", item.Name, req.LanguageCode, err)
				}
			}

			// Translate item description
			if item.Description != "" {
				translatedTexts, err := servicesTranslationService.TranslateTextWithSource(item.Description, sourceLanguage, []string{req.LanguageCode})
				if err == nil && persistableTranslation(item.Description, translatedTexts[req.LanguageCode]) {
					translation := &database.Translation{
						BusinessID:        uint(businessID),
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
					if err := db.TranslationService.SaveTranslation(translation); err != nil {
						log.Printf("WARNING: failed to save translation for item description: %v", err)
					}
				} else {
					log.Printf("❌ Failed to translate item description '%s': %v", item.Description, err)
				}
			}

			// Translate item options
			for k, option := range item.Options {
				if option.Name != "" {
					optionEntityID := entityID*1000 + k // Unique ID for each option
					translatedTexts, err := servicesTranslationService.TranslateTextWithSource(option.Name, sourceLanguage, []string{req.LanguageCode})
					if err == nil && persistableTranslation(option.Name, translatedTexts[req.LanguageCode]) {
						translation := &database.Translation{
							BusinessID:        uint(businessID),
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
						if err := db.TranslationService.SaveTranslation(translation); err != nil {
							log.Printf("WARNING: failed to save translation for option: %v", err)
						}
					} else {
						log.Printf("❌ Failed to translate option '%s': %v", option.Name, err)
					}
				}
			}

			// Allergens and dietary tags are canonical IDs (celery, vegan, …)
			// rendered through localized chip maps on both tiers — machine-
			// translating them detached them from the ID→label/icon maps (D4d).
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Menu translated successfully",
		"language_code": req.LanguageCode,
		"business_id":   businessID,
	})
}

// Phase 2: Enhanced Menu Management API Endpoints

// Menu category request structures
type AddCategoryRequest struct {
	Name                string              `json:"name" binding:"required"`
	Description         string              `json:"description"`
	Items               []database.MenuItem `json:"items"`
	Version             *uint               `json:"version"` // Menu version for optimistic concurrency control
	ConfirmSanitization bool                `json:"confirm_sanitization"`
}

type UpdateCategoryRequest struct {
	Name                string              `json:"name"`
	Description         string              `json:"description"`
	Items               []database.MenuItem `json:"items"`
	Version             *uint               `json:"version"` // Menu version for optimistic concurrency control
	ConfirmSanitization bool                `json:"confirm_sanitization"`
}

// DeleteCategoryRequest is the JSON body for ID-based category deletion
type DeleteCategoryRequest struct {
	CategoryID string `json:"category_id" binding:"required"`
	Version    uint   `json:"version" binding:"required"`
}

// Menu item request structures
type AddMenuItemRequest struct {
	CategoryIndex       int               `json:"category_index" binding:"min=0"` // Deprecated: use CategoryID
	CategoryID          string            `json:"category_id"`                    // ID-based addressing
	Item                database.MenuItem `json:"item" binding:"required"`
	Version             *uint             `json:"version"` // Menu version for optimistic concurrency control
	ConfirmSanitization bool              `json:"confirm_sanitization"`
}

type UpdateMenuItemRequest struct {
	CategoryIndex       int               `json:"category_index" binding:"min=0"` // Deprecated: use CategoryID + ItemID
	ItemIndex           int               `json:"item_index" binding:"min=0"`     // Deprecated: use CategoryID + ItemID
	CategoryID          string            `json:"category_id"`                    // ID-based addressing
	ItemID              string            `json:"item_id"`                        // ID-based addressing
	Item                database.MenuItem `json:"item" binding:"required"`
	Version             *uint             `json:"version"` // Menu version for optimistic concurrency control
	ConfirmSanitization bool              `json:"confirm_sanitization"`
}

// DeleteMenuItemRequest is the JSON body for ID-based item deletion
type DeleteMenuItemRequest struct {
	CategoryID string `json:"category_id" binding:"required"`
	ItemID     string `json:"item_id" binding:"required"`
	Version    uint   `json:"version" binding:"required"`
}

// respondVersionConflict sends a standardized version conflict response
func respondVersionConflict(c *gin.Context) {
	c.JSON(http.StatusConflict, gin.H{
		"error": "Menu was modified by another user. Please refresh and try again.",
		"code":  "MENU_VERSION_CONFLICT",
	})
}

func respondMenuSanitizationReview(c *gin.Context, report services.GeneratedMenuSanitizeReport) {
	c.JSON(http.StatusOK, gin.H{
		"message":               "Review dropped menu fields before saving",
		"requires_confirmation": true,
		"sanitization":          report,
	})
}

// AddMenuCategory adds a new category to a business menu
func AddMenuCategory(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	var req AddCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	category := database.MenuCategory{
		Name:        req.Name,
		Description: req.Description,
		Items:       req.Items,
	}
	sanitizedCategories, sanitization := services.SanitizeMenuCategories([]database.MenuCategory{category})
	category = sanitizedCategories[0]
	if refuseDemoMenuImages(c, business.ID, sanitizedCategories) {
		return
	}
	if sanitization.HasDrops() && !req.ConfirmSanitization {
		respondMenuSanitizationReview(c, sanitization)
		return
	}

	// The DB write returns the bumped version, the created category (with its
	// server-assigned ID), and its index — so we neither re-read the whole menu
	// for the version (§3.7 fix 8) nor force the client to reload the full menu
	// (§3.7 fix 3: it patches state in place from the echoed category).
	var (
		version       uint
		created       *database.MenuCategory
		categoryIndex int
		err           error
	)
	if req.Version != nil {
		version, created, categoryIndex, err = database.AddMenuCategoryVersioned(business.ID, category, *req.Version)
	} else {
		// Unversioned append: no optimistic-concurrency check.
		version, created, categoryIndex, err = database.AddMenuCategory(business.ID, category)
	}
	if err != nil {
		if errors.Is(err, database.ErrMenuVersionConflict) {
			respondVersionConflict(c)
			return
		}
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not add menu category")
		return
	}

	services.InvalidatePricingCache(business.ID)
	stampOnboardingAsync(business.ID)

	// Auto-translate the new category. Uses the index returned by the write —
	// no post-write re-read of the full document.
	runMenuCategoryTranslationAsync(func() {
		if err := autoTranslateCategory(business.ID, categoryIndex, category.Name, category.Description); err != nil {
			log.Printf("Failed to translate category: %v", err)
		}
	})

	c.JSON(http.StatusCreated, gin.H{
		"message":               "Category added successfully",
		"version":               version,
		"category":              created,
		"requires_confirmation": false,
		"sanitization":          sanitization,
	})
}

// UpdateMenuCategory updates a specific category in a business menu.
// Supports both legacy index-based (URL param) and new ID-based (URL param) addressing.
func UpdateMenuCategory(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	// Determine if this is an ID-based or index-based request via URL param
	categoryParam := c.Param("category_id")
	if categoryParam == "" {
		categoryParam = c.Param("category_index")
	}

	var req UpdateCategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	category := database.MenuCategory{
		Name:        req.Name,
		Description: req.Description,
		Items:       req.Items,
	}
	sanitizedCategories, sanitization := services.SanitizeMenuCategories([]database.MenuCategory{category})
	category = sanitizedCategories[0]
	if refuseDemoMenuImages(c, business.ID, sanitizedCategories) {
		return
	}
	if sanitization.HasDrops() && !req.ConfirmSanitization {
		respondMenuSanitizationReview(c, sanitization)
		return
	}

	// Try ID-based path first: if param is not a pure integer, treat as UUID
	categoryIndex, indexErr := strconv.Atoi(categoryParam)
	isIDBasedRoute := indexErr != nil

	var (
		version      uint
		mutated      *database.MenuCategory
		translateIdx = categoryIndex
		err          error
	)
	switch {
	case isIDBasedRoute:
		// ID-based with version check
		if req.Version == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "version is required for ID-based updates"})
			return
		}
		version, mutated, translateIdx, err = database.UpdateMenuCategoryByID(business.ID, categoryParam, category, *req.Version)
	case req.Version != nil:
		// Index-addressed but version-carrying: the CAS is still enforced — a
		// client-supplied version must never be silently ignored, and this used
		// to be misrouted into the ID lookup with the index string as a UUID
		// (guaranteed 404) (L3-9).
		version, mutated, err = database.UpdateMenuCategoryVersioned(business.ID, categoryIndex, category, *req.Version)
	default:
		// Legacy index-based path (no client version)
		version, mutated, err = database.UpdateMenuCategory(business.ID, categoryIndex, category)
	}
	if err != nil {
		if errors.Is(err, database.ErrMenuVersionConflict) {
			respondVersionConflict(c)
			return
		}
		if errors.Is(err, database.ErrCategoryNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Category not found"})
			return
		}
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not update menu category")
		return
	}

	services.InvalidatePricingCache(business.ID)

	// Auto-translate the updated category. Uses the index returned by the write —
	// no post-write re-read of the full document (§3.7 fix 8).
	runMenuCategoryTranslationAsync(func() {
		if err := autoTranslateCategory(business.ID, translateIdx, category.Name, category.Description); err != nil {
			log.Printf("Failed to translate updated category: %v", err)
		}
	})

	c.JSON(http.StatusOK, gin.H{
		"message":               "Category updated successfully",
		"version":               version,
		"category":              mutated,
		"requires_confirmation": false,
		"sanitization":          sanitization,
	})
}

// DeleteMenuCategory removes a category from a business menu.
// Supports both legacy index-based (URL param) and new ID-based (URL param) addressing.
func DeleteMenuCategory(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	// Determine if this is an ID-based or index-based request via URL param
	categoryParam := c.Param("category_id")
	if categoryParam == "" {
		categoryParam = c.Param("category_index")
	}

	categoryIndex, indexErr := strconv.Atoi(categoryParam)
	isIDBasedRoute := indexErr != nil

	// Resolve the translation-cleanup index before the delete (the ID→index
	// lookup needs the category to still exist), but only fire the cleanup after
	// the delete actually lands — a version-conflicted delete must not strip the
	// surviving category's translations.
	cleanupIdx := categoryIndex
	if isIDBasedRoute {
		cleanupIdx = -1
		_, cats, _ := database.GetMenuByBusinessID(business.ID)
		for i, cat := range cats {
			if cat.ID == categoryParam {
				cleanupIdx = i
				break
			}
		}
	}

	// The DB delete returns the bumped version directly — no post-write re-read
	// of the full document for the version (§3.7 fix 8).
	var (
		newVersion uint
		err        error
	)
	if isIDBasedRoute {
		// ID-based with version check - read version from query param or body
		var version uint
		if vStr := c.Query("version"); vStr != "" {
			v, perr := strconv.ParseUint(vStr, 10, 32)
			if perr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid version parameter"})
				return
			}
			version = uint(v)
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "version query parameter is required for ID-based deletes"})
			return
		}
		newVersion, err = database.DeleteMenuCategoryByID(business.ID, categoryParam, version)
	} else if vStr := c.Query("version"); vStr != "" {
		// Index-addressed but version-carrying: the CAS is still enforced — a
		// client-supplied version must never be silently ignored (L3-9).
		v, perr := strconv.ParseUint(vStr, 10, 32)
		if perr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid version parameter"})
			return
		}
		newVersion, err = database.DeleteMenuCategoryVersioned(business.ID, categoryIndex, uint(v))
	} else {
		// Legacy index-based path (no client version)
		newVersion, err = database.DeleteMenuCategory(business.ID, categoryIndex)
	}
	if err != nil {
		if errors.Is(err, database.ErrMenuVersionConflict) {
			respondVersionConflict(c)
			return
		}
		if errors.Is(err, database.ErrCategoryNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Category not found"})
			return
		}
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not delete menu category")
		return
	}

	if cleanupIdx >= 0 {
		logger.SafeGo(func() {
			if err := deleteTranslationsForCategory(business.ID, cleanupIdx); err != nil {
				log.Printf("Failed to delete category translations: %v", err)
			}
		})
	}

	services.InvalidatePricingCache(business.ID)

	c.JSON(http.StatusOK, gin.H{"message": "Category deleted successfully", "version": newVersion})
}

// validateMenuItemWrite rejects blank names and negative prices before any
// menu document mutation. Nested MenuItem has no gin binding tags (JSON blob
// in the menu document), so empty POSTs previously created nameless $0 rows
// under category_index 0 (FIND-031).
func validateMenuItemWrite(item database.MenuItem) (string, bool) {
	if strings.TrimSpace(item.Name) == "" {
		return "Menu item name is required", false
	}
	if item.Price < 0 {
		return "Menu item price cannot be negative", false
	}
	return "", true
}

func sanitizeMenuItemWrite(item database.MenuItem) (database.MenuItem, services.GeneratedMenuSanitizeReport, bool) {
	categories, report := services.SanitizeMenuCategories([]database.MenuCategory{{Items: []database.MenuItem{item}}})
	if len(categories) == 0 || len(categories[0].Items) == 0 {
		return database.MenuItem{}, report, false
	}
	return categories[0].Items[0], report, true
}

// AddMenuItem adds a new item to a menu category
func AddMenuItem(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	var req AddMenuItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	if msg, ok := validateMenuItemWrite(req.Item); !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": msg,
			"code":  "VALIDATION_INVALID_INPUT",
		})
		return
	}
	req.Item.Name = strings.TrimSpace(req.Item.Name)
	var sanitization services.GeneratedMenuSanitizeReport
	var retained bool
	req.Item, sanitization, retained = sanitizeMenuItemWrite(req.Item)
	if refuseDemoMenuImages(c, business.ID, []database.MenuCategory{{Items: []database.MenuItem{req.Item}}}) {
		return
	}
	if sanitization.HasDrops() && !req.ConfirmSanitization {
		respondMenuSanitizationReview(c, sanitization)
		return
	}
	if !retained {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":        "Menu item price must be greater than 0 and less than 10000",
			"code":         "VALIDATION_INVALID_INPUT",
			"sanitization": sanitization,
		})
		return
	}

	// The DB write returns the bumped version, the created item (with its
	// server-assigned ID), and its (category, item) position — so we neither
	// re-read for the version (§3.7 fix 8) nor force a full-menu reload on the
	// client (§3.7 fix 3), and the async translate keys off the returned index.
	var (
		version uint
		created *database.MenuItem
		catIdx  int
		itemIdx int
		err     error
	)
	switch {
	case req.CategoryID != "":
		if req.Version == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "version is required for ID-based updates"})
			return
		}
		version, created, catIdx, itemIdx, err = database.AddMenuItemByID(business.ID, req.CategoryID, req.Item, *req.Version)
	case req.Version != nil:
		// Index-addressed but version-carrying: the CAS is still enforced — a
		// client-supplied version must never be silently ignored (L3-9).
		catIdx = req.CategoryIndex
		version, created, itemIdx, err = database.AddMenuItemVersioned(business.ID, req.CategoryIndex, req.Item, *req.Version)
	default:
		// Legacy index-based path (no client version)
		catIdx = req.CategoryIndex
		version, created, itemIdx, err = database.AddMenuItem(business.ID, req.CategoryIndex, req.Item)
	}
	if err != nil {
		if errors.Is(err, database.ErrMenuVersionConflict) {
			respondVersionConflict(c)
			return
		}
		if errors.Is(err, database.ErrCategoryNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Category not found"})
			return
		}
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not add menu item")
		return
	}

	services.InvalidatePricingCache(business.ID)
	stampOnboardingAsync(business.ID)

	// Auto-translate the new menu item using the returned position — no
	// post-write re-read of the full document (§3.7 fix 8).
	logger.SafeGo(func() {
		// Bail before any DB read when translation is unconfigured: this keeps the
		// goroutine from touching the package-global DB handle in that case (a no-op
		// in prod, but it removes a test-only data race with SetTestDB).
		if GetTranslationService() == nil {
			return
		}
		if err := autoTranslateMenuItem(business.ID, catIdx, itemIdx, req.Item.Name, req.Item.Description, req.Item.Options); err != nil {
			log.Printf("Failed to translate menu item: %v", err)
		}
	})

	c.JSON(http.StatusCreated, gin.H{
		"message":               "Menu item added successfully",
		"version":               version,
		"item":                  created,
		"requires_confirmation": false,
		"sanitization":          sanitization,
	})
}

// UpdateMenuItem updates a specific menu item
func UpdateMenuItem(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	var req UpdateMenuItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	if msg, ok := validateMenuItemWrite(req.Item); !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": msg,
			"code":  "VALIDATION_INVALID_INPUT",
		})
		return
	}
	req.Item.Name = strings.TrimSpace(req.Item.Name)
	var sanitization services.GeneratedMenuSanitizeReport
	var retained bool
	req.Item, sanitization, retained = sanitizeMenuItemWrite(req.Item)
	if refuseDemoMenuImages(c, business.ID, []database.MenuCategory{{Items: []database.MenuItem{req.Item}}}) {
		return
	}
	if sanitization.HasDrops() && !req.ConfirmSanitization {
		respondMenuSanitizationReview(c, sanitization)
		return
	}
	if !retained {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":        "Menu item price must be greater than 0 and less than 10000",
			"code":         "VALIDATION_INVALID_INPUT",
			"sanitization": sanitization,
		})
		return
	}

	// The DB write returns the bumped version, the mutated item, and its
	// (category, item) position — so we neither re-read for the version (§3.7
	// fix 8) nor force a full-menu reload on the client (§3.7 fix 3).
	var (
		version uint
		mutated *database.MenuItem
		catIdx  int
		itemIdx int
		err     error
	)
	switch {
	case req.CategoryID != "" && req.ItemID != "":
		if req.Version == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "version is required for ID-based updates"})
			return
		}
		version, mutated, catIdx, itemIdx, err = database.UpdateMenuItemByID(business.ID, req.CategoryID, req.ItemID, req.Item, *req.Version)
	case req.Version != nil:
		// Index-addressed but version-carrying: the CAS is still enforced — a
		// client-supplied version must never be silently ignored (L3-9).
		catIdx = req.CategoryIndex
		itemIdx = req.ItemIndex
		version, mutated, err = database.UpdateMenuItemVersioned(business.ID, req.CategoryIndex, req.ItemIndex, req.Item, *req.Version)
	default:
		// Legacy index-based path (no client version)
		catIdx = req.CategoryIndex
		itemIdx = req.ItemIndex
		version, mutated, err = database.UpdateMenuItem(business.ID, req.CategoryIndex, req.ItemIndex, req.Item)
	}
	if err != nil {
		if errors.Is(err, database.ErrMenuVersionConflict) {
			respondVersionConflict(c)
			return
		}
		if errors.Is(err, database.ErrCategoryNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Category not found"})
			return
		}
		if errors.Is(err, database.ErrItemNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Menu item not found"})
			return
		}
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not update menu item")
		return
	}

	services.InvalidatePricingCache(business.ID)

	// Auto-translate the updated menu item using the returned position — no
	// post-write re-read of the full document (§3.7 fix 8).
	logger.SafeGo(func() {
		// Bail before any DB read when translation is unconfigured: this keeps the
		// goroutine from touching the package-global DB handle in that case (a no-op
		// in prod, but it removes a test-only data race with SetTestDB).
		if GetTranslationService() == nil {
			return
		}
		if err := autoTranslateMenuItem(business.ID, catIdx, itemIdx, req.Item.Name, req.Item.Description, req.Item.Options); err != nil {
			log.Printf("Failed to translate updated menu item: %v", err)
		}
	})

	c.JSON(http.StatusOK, gin.H{
		"message":               "Menu item updated successfully",
		"version":               version,
		"item":                  mutated,
		"requires_confirmation": false,
		"sanitization":          sanitization,
	})
}

// DeleteMenuItem removes an item from a menu category.
// Supports both legacy index-based (URL params) and new ID-based (URL params) addressing.
func DeleteMenuItem(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	// Determine if this is ID-based or index-based by checking URL params
	categoryIDParam := c.Param("category_id")
	itemIDParam := c.Param("item_id")

	// The DB delete returns the bumped version directly — no post-write re-read
	// of the full document for the version (§3.7 fix 8).
	var newVersion uint
	if categoryIDParam != "" && itemIDParam != "" {
		// ID-based path - version from query param
		var version uint
		if vStr := c.Query("version"); vStr != "" {
			v, err := strconv.ParseUint(vStr, 10, 32)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid version parameter"})
				return
			}
			version = uint(v)
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "version query parameter is required for ID-based deletes"})
			return
		}

		translationCat, translationItem := -1, -1
		if _, cats, err := database.GetMenuByBusinessID(business.ID); err == nil {
			for i, cat := range cats {
				if cat.ID != categoryIDParam {
					continue
				}
				for j, item := range cat.Items {
					if item.ID == itemIDParam {
						translationCat, translationItem = i, j
						break
					}
				}
				break
			}
		}

		v, err := database.DeleteMenuItemByID(business.ID, categoryIDParam, itemIDParam, version)
		if err != nil {
			if errors.Is(err, database.ErrMenuVersionConflict) {
				respondVersionConflict(c)
				return
			}
			if errors.Is(err, database.ErrCategoryNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "Category not found"})
				return
			}
			if errors.Is(err, database.ErrItemNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "Menu item not found"})
				return
			}
			RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not delete menu item")
			return
		}
		newVersion = v
		if translationCat >= 0 && translationItem >= 0 {
			catIdx, itemIdx := translationCat, translationItem
			logger.SafeGo(func() {
				if err := deleteTranslationsForMenuItem(business.ID, catIdx, itemIdx); err != nil {
					log.Printf("Failed to delete menu item translations: %v", err)
				}
			})
		}
	} else {
		// Legacy index-based path
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

		var v uint
		if vStr := c.Query("version"); vStr != "" {
			// Index-addressed but version-carrying: the CAS is still enforced — a
			// client-supplied version must never be silently ignored (L3-9).
			parsed, perr := strconv.ParseUint(vStr, 10, 32)
			if perr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid version parameter"})
				return
			}
			v, err = database.DeleteMenuItemVersioned(business.ID, categoryIndex, itemIndex, uint(parsed))
		} else {
			v, err = database.DeleteMenuItem(business.ID, categoryIndex, itemIndex)
		}
		if err != nil {
			if errors.Is(err, database.ErrMenuVersionConflict) {
				respondVersionConflict(c)
				return
			}
			RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not delete menu item")
			return
		}
		newVersion = v

		// Cleanup runs only after the delete actually landed: a version-conflicted
		// delete must not strip the surviving item's translations.
		logger.SafeGo(func() {
			if err := deleteTranslationsForMenuItem(business.ID, categoryIndex, itemIndex); err != nil {
				log.Printf("Failed to delete menu item translations: %v", err)
			}
		})
	}

	services.InvalidatePricingCache(business.ID)

	c.JSON(http.StatusOK, gin.H{"message": "Menu item deleted successfully", "version": newVersion})
}

// Language and Currency Management Handlers

// Translation helper functions for menu management

// autoTranslateCategory translates a category to all business languages
func autoTranslateCategory(businessID uint, categoryIndex int, name, description string) error {
	db := database.GetDBWrapper()
	translationService := GetTranslationService()

	if translationService == nil {
		return errors.New("translation service not available")
	}

	// Get business languages
	businessLanguages, err := db.LanguageService.GetBusinessLanguages(businessID)
	if err != nil {
		return fmt.Errorf("failed to get business languages: %v", err)
	}

	// Skip if no languages configured
	if len(businessLanguages) == 0 {
		return nil
	}

	// Translate to each business language
	for _, bl := range businessLanguages {
		// Skip default language (usually English)
		if bl.IsDefault {
			continue
		}

		// Translate category name
		if name != "" {
			translatedTexts, err := translationService.TranslateText(name, []string{bl.LanguageCode})
			if err == nil && translatedTexts[bl.LanguageCode] != "" {
				translation := &database.Translation{
					BusinessID:        businessID,
					EntityType:        "category",
					EntityID:          uint(categoryIndex),
					FieldName:         "name",
					LanguageCode:      bl.LanguageCode,
					OriginalText:      name,
					TranslatedText:    translatedTexts[bl.LanguageCode],
					IsAutoTranslated:  true,
					TranslationSource: "google_translate",
				}
				if err := db.TranslationService.SaveTranslation(translation); err != nil {
					log.Printf("WARNING: failed to save translation: %v", err)
				}
			}
		}

		// Translate category description
		if description != "" {
			translatedTexts, err := translationService.TranslateText(description, []string{bl.LanguageCode})
			if err == nil && translatedTexts[bl.LanguageCode] != "" {
				translation := &database.Translation{
					BusinessID:        businessID,
					EntityType:        "category",
					EntityID:          uint(categoryIndex),
					FieldName:         "description",
					LanguageCode:      bl.LanguageCode,
					OriginalText:      description,
					TranslatedText:    translatedTexts[bl.LanguageCode],
					IsAutoTranslated:  true,
					TranslationSource: "google_translate",
				}
				if err := db.TranslationService.SaveTranslation(translation); err != nil {
					log.Printf("WARNING: failed to save translation: %v", err)
				}
			}
		}
	}

	return nil
}

// autoTranslateMenuItem translates a menu item to all business languages.
// Allergens and dietary tags are canonical IDs localized at render time by
// both tiers (menu.allergenNames.* / items.allergenNames.*) — they are
// deliberately NOT machine-translated (D4d).
func autoTranslateMenuItem(businessID uint, categoryIndex, itemIndex int, name, description string, options []database.MenuItemOption) error {
	db := database.GetDBWrapper()
	translationService := GetTranslationService()

	if translationService == nil {
		return errors.New("translation service not available")
	}

	// Get business languages
	businessLanguages, err := db.LanguageService.GetBusinessLanguages(businessID)
	if err != nil {
		return fmt.Errorf("failed to get business languages: %v", err)
	}

	// Skip if no languages configured
	if len(businessLanguages) == 0 {
		return nil
	}

	// Calculate entity ID using same logic as TranslateMenu
	entityID := categoryIndex*1000 + itemIndex

	// Translate to each business language
	for _, bl := range businessLanguages {
		// Skip default language (usually English)
		if bl.IsDefault {
			continue
		}

		// Translate item name
		if name != "" {
			translatedTexts, err := translationService.TranslateText(name, []string{bl.LanguageCode})
			if err == nil && translatedTexts[bl.LanguageCode] != "" {
				translation := &database.Translation{
					BusinessID:        businessID,
					EntityType:        "menu_item",
					EntityID:          uint(entityID),
					FieldName:         "name",
					LanguageCode:      bl.LanguageCode,
					OriginalText:      name,
					TranslatedText:    translatedTexts[bl.LanguageCode],
					IsAutoTranslated:  true,
					TranslationSource: "google_translate",
				}
				if err := db.TranslationService.SaveTranslation(translation); err != nil {
					log.Printf("WARNING: failed to save translation: %v", err)
				}
			}
		}

		// Translate item description
		if description != "" {
			translatedTexts, err := translationService.TranslateText(description, []string{bl.LanguageCode})
			if err == nil && translatedTexts[bl.LanguageCode] != "" {
				translation := &database.Translation{
					BusinessID:        businessID,
					EntityType:        "menu_item",
					EntityID:          uint(entityID),
					FieldName:         "description",
					LanguageCode:      bl.LanguageCode,
					OriginalText:      description,
					TranslatedText:    translatedTexts[bl.LanguageCode],
					IsAutoTranslated:  true,
					TranslationSource: "google_translate",
				}
				if err := db.TranslationService.SaveTranslation(translation); err != nil {
					log.Printf("WARNING: failed to save translation: %v", err)
				}
			}
		}

		// Translate options
		for k, option := range options {
			if option.Name != "" {
				optionEntityID := entityID*1000 + k
				translatedTexts, err := translationService.TranslateText(option.Name, []string{bl.LanguageCode})
				if err == nil && translatedTexts[bl.LanguageCode] != "" {
					translation := &database.Translation{
						BusinessID:        businessID,
						EntityType:        "menu_item_option",
						EntityID:          uint(optionEntityID),
						FieldName:         "name",
						LanguageCode:      bl.LanguageCode,
						OriginalText:      option.Name,
						TranslatedText:    translatedTexts[bl.LanguageCode],
						IsAutoTranslated:  true,
						TranslationSource: "google_translate",
					}
					if err := db.TranslationService.SaveTranslation(translation); err != nil {
						log.Printf("WARNING: failed to save translation: %v", err)
					}
				}
			}
		}
	}

	return nil
}

// deleteTranslationsForCategory removes all translations for a category
func deleteTranslationsForCategory(businessID uint, categoryIndex int) error {
	db := database.GetDBWrapper()

	// Delete category name and description translations
	if err := db.GetGorm().Where("business_id = ? AND entity_type = ? AND entity_id = ?", businessID, "category", categoryIndex).Delete(&database.Translation{}).Error; err != nil {
		return fmt.Errorf("failed to delete category translations: %v", err)
	}

	log.Printf("🗑️ Deleted translations for category %d", categoryIndex)
	return nil
}

// deleteTranslationsForMenuItem removes all translations for a menu item
func deleteTranslationsForMenuItem(businessID uint, categoryIndex, itemIndex int) error {
	db := database.GetDBWrapper()

	// Calculate entity ID using same logic as TranslateMenu
	entityID := categoryIndex*1000 + itemIndex

	// Delete menu item name and description translations
	if err := db.GetGorm().Where("business_id = ? AND entity_type = ? AND entity_id = ?", businessID, "menu_item", entityID).Delete(&database.Translation{}).Error; err != nil {
		return fmt.Errorf("failed to delete menu item translations: %v", err)
	}

	// Delete option translations (entity_id starts with entityID*1000)
	if err := db.GetGorm().Where("business_id = ? AND entity_type = ? AND entity_id >= ? AND entity_id < ?", businessID, "menu_item_option", entityID*1000, (entityID+1)*1000).Delete(&database.Translation{}).Error; err != nil {
		return fmt.Errorf("failed to delete option translations: %v", err)
	}

	// Delete allergen translations (entity_id starts with entityID*10000)
	if err := db.GetGorm().Where("business_id = ? AND entity_type = ? AND entity_id >= ? AND entity_id < ?", businessID, "allergen", entityID*10000, (entityID+1)*10000).Delete(&database.Translation{}).Error; err != nil {
		return fmt.Errorf("failed to delete allergen translations: %v", err)
	}

	// Delete dietary tag translations (entity_id starts with entityID*100000)
	if err := db.GetGorm().Where("business_id = ? AND entity_type = ? AND entity_id >= ? AND entity_id < ?", businessID, "dietary_tag", entityID*100000, (entityID+1)*100000).Delete(&database.Translation{}).Error; err != nil {
		return fmt.Errorf("failed to delete dietary tag translations: %v", err)
	}

	log.Printf("🗑️ Deleted translations for menu item %d in category %d", itemIndex, categoryIndex)
	return nil
}
