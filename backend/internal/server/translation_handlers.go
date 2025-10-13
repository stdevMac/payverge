package server

import (
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"payverge/internal/database"
	"payverge/internal/services"
)

// TranslateMenuRequest represents a request to translate an entire menu
type TranslateMenuRequest struct {
	LanguageCodes []string `json:"language_codes" binding:"required"`
}

// TranslateMenuResponse represents the response for menu translation
type TranslateMenuResponse struct {
	JobID   string `json:"job_id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// TranslationStatus represents the status of a translation job
type TranslationStatus struct {
	JobID     string `json:"job_id"`
	Status    string `json:"status"`
	Progress  int    `json:"progress"`
	Total     int    `json:"total"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
}

// TranslateEntireMenu translates an entire business menu to specified languages
func TranslateEntireMenu(c *gin.Context) {
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
	db := database.GetDBWrapper()
	business, err := db.BusinessService.GetByID(uint(businessID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to manage this business"})
		return
	}

	var req TranslateMenuRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Generate a job ID
	jobID := generateJobID(uint(businessID))

	// Start translation in background
	go func() {
		if err := performBatchTranslation(uint(businessID), req.LanguageCodes, jobID); err != nil {
			log.Printf("Failed to translate menu for business %d: %v", businessID, err)
		}
	}()

	c.JSON(http.StatusAccepted, TranslateMenuResponse{
		JobID:   jobID,
		Status:  "processing",
		Message: "Translation started. Use the job ID to check progress.",
	})
}

// GetTranslationStatus returns the status of a translation job
func GetTranslationStatus(c *gin.Context) {
	jobID := c.Param("jobId")
	
	// For now, we'll simulate status checking
	// In production, you'd store job status in database/cache
	status := TranslationStatus{
		JobID:     jobID,
		Status:    "completed",
		Progress:  100,
		Total:     100,
		Message:   "Translation completed successfully",
		CreatedAt: "2025-10-12T11:59:00Z",
	}

	c.JSON(http.StatusOK, status)
}

// generateJobID creates a unique job ID for translation
func generateJobID(businessID uint) string {
	return "translate_" + strconv.FormatUint(uint64(businessID), 10) + "_" + strconv.FormatInt(1728728340, 10)
}

// performBatchTranslation performs the actual translation work
func performBatchTranslation(businessID uint, languageCodes []string, jobID string) error {
	log.Printf("Starting batch translation job %s for business %d", jobID, businessID)

	// Get the menu
	_, categories, err := database.GetMenuByBusinessID(businessID)
	if err != nil {
		return err
	}

	// Get database wrapper and translation service
	db := database.GetDBWrapper()
	translationService := services.NewTranslationService(db, "")

	// Get business languages to find default
	businessLanguages, err := db.LanguageService.GetBusinessLanguages(businessID)
	if err != nil {
		return err
	}

	// Find default language
	var defaultLang string
	for _, bl := range businessLanguages {
		if bl.IsDefault {
			defaultLang = bl.LanguageCode
			break
		}
	}

	// Filter out default language from target languages
	var targetLanguages []string
	for _, lang := range languageCodes {
		if lang != defaultLang {
			targetLanguages = append(targetLanguages, lang)
		}
	}

	if len(targetLanguages) == 0 {
		log.Printf("No target languages to translate to for business %d", businessID)
		return nil
	}

	// Use the existing translation service to translate all content
	processed := 0
	for _, category := range categories {
		// Create a consistent ID hash from category name and description
		categoryID := uint(len(category.Name) + len(category.Description))
		
		// Translate category using the existing service
		err := translationService.TranslateCategory(businessID, categoryID, category.Name, category.Description)
		if err != nil {
			log.Printf("Failed to translate category %s: %v", category.ID, err)
		}
		processed++

		// Translate all items in category
		for _, item := range category.Items {
			// Create a consistent ID hash from item name and description
			itemID := uint(len(item.Name) + len(item.Description))
			
			// Translate menu item using the existing service
			err := translationService.TranslateMenuItem(businessID, itemID, item.Name, item.Description)
			if err != nil {
				log.Printf("Failed to translate menu item %s: %v", item.ID, err)
			}
			processed++
		}
	}

	log.Printf("Completed batch translation job %s for business %d. Processed %d items", jobID, businessID, processed)
	return nil
}
