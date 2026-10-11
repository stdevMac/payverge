package server

import (
	"fmt"
	"log"
	"net/http"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/locales"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/services"
	"github.com/stdevmac/payverge/backend/internal/utils"

	"github.com/gin-gonic/gin"
)

var batchTranslationService *services.TranslationService

func SetBatchTranslationService(service *services.TranslationService) {
	batchTranslationService = service
}

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
	// Verify business ownership
	business, err := database.GetBusinessByIdOrBusinessId(utils.BusinessIdentifierFromParam(c, "id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if !CheckBusinessAccess(c, business) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to manage this business"})
		return
	}

	var req TranslateMenuRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	jobID := registerTranslationJob(business.ID)

	// Start translation in background
	logger.SafeGo(func() {
		if err := performBatchTranslation(business.ID, req.LanguageCodes, jobID); err != nil {
			updateTranslationJob(jobID, func(status *TranslationStatus) {
				status.Status = "failed"
				status.Message = err.Error()
			})
			log.Printf("Failed to translate menu for business %d: %v", business.ID, err)
		}
	})

	c.JSON(http.StatusAccepted, TranslateMenuResponse{
		JobID:   jobID,
		Status:  "processing",
		Message: "Translation started. Use the job ID to check progress.",
	})
}

// GetTranslationStatus returns the status of a translation job
func GetTranslationStatus(c *gin.Context) {
	jobID := c.Param("jobId")

	record, ok := getTranslationJob(jobID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Translation job not found"})
		return
	}

	business, err := database.GetBusinessByID(record.BusinessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if !CheckBusinessAccess(c, business) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to view this translation job"})
		return
	}

	c.JSON(http.StatusOK, record.Status)
}

// IsBatchTranslationEnabled reports whether the Google Translate-backed batch
// translation service is configured. Callers use it to avoid spawning no-op
// background work when no API key is set.
func IsBatchTranslationEnabled() bool {
	return batchTranslationService != nil && batchTranslationService.IsEnabled()
}

// TranslateBusinessMenuForLanguages translates the whole storefront menu into the
// given guest languages (the business default language is filtered out). It is
// job-less — callers wrap it in a goroutine. This is the automatic counterpart
// to the per-item translation that already runs on menu add/edit, used when an
// operator adds or changes the business's guest languages so the storefront menu
// is translated without a manual "Sync Translations" click.
func TranslateBusinessMenuForLanguages(businessID uint, languageCodes []string) error {
	return translateMenuIntoLanguages(businessID, languageCodes, nil)
}

// translateMenuIntoLanguages is the shared core behind the job-based
// TranslateEntireMenu endpoint and the job-less automatic paths (language-add +
// storefront backfill). progress may be nil. Entity IDs are kept in lockstep
// with applyTranslationsToMenu so the storefront read path finds what is written.
func translateMenuIntoLanguages(businessID uint, languageCodes []string, progress func(processed, total int)) error {
	translationService := batchTranslationService
	if translationService == nil || !translationService.IsEnabled() {
		return fmt.Errorf("translation service is not enabled")
	}

	_, categories, err := database.GetMenuByBusinessID(businessID)
	if err != nil {
		return err
	}

	total := 0
	for _, category := range categories {
		total++
		total += len(category.Items)
	}
	if progress != nil {
		progress(0, total)
	}

	db := database.GetDBWrapper()
	businessLanguages, err := db.LanguageService.GetBusinessLanguages(businessID)
	if err != nil {
		return err
	}

	var defaultLang string
	for _, bl := range businessLanguages {
		if bl.IsDefault {
			defaultLang = bl.LanguageCode
			break
		}
	}

	// Filter out the default language. Use IsGuestLocale — these codes describe
	// the menu-translation target for guest storefront rendering, which can
	// extend well beyond the operator dashboard's locale set (en/es/es-AR).
	var targetLanguages []string
	for _, lang := range languageCodes {
		if lang != defaultLang {
			if !locales.IsGuestLocale(lang) {
				return fmt.Errorf("unsupported guest target language: %s", lang)
			}
			targetLanguages = append(targetLanguages, lang)
		}
	}

	if len(targetLanguages) == 0 {
		log.Printf("No target languages to translate to for business %d", businessID)
		return nil
	}

	processed := 0
	for i, category := range categories {
		categoryID := uint(i)
		if err := translationService.TranslateCategoryToLanguages(businessID, categoryID, category.Name, category.Description, targetLanguages); err != nil {
			log.Printf("Failed to translate category %s: %v", category.ID, err)
		}
		processed++
		if progress != nil {
			progress(processed, total)
		}

		for j, item := range category.Items {
			// Keep menu-item IDs aligned with applyTranslationsToMenu.
			itemID := uint(i*1000 + j)
			if err := translationService.TranslateMenuItemToLanguages(businessID, itemID, item.Name, item.Description, targetLanguages); err != nil {
				log.Printf("Failed to translate menu item %s: %v", item.ID, err)
			}
			processed++
			if progress != nil {
				progress(processed, total)
			}
		}
	}

	return nil
}

// performBatchTranslation runs translateMenuIntoLanguages while reporting
// progress into the polling job record used by TranslateEntireMenu.
func performBatchTranslation(businessID uint, languageCodes []string, jobID string) error {
	log.Printf("Starting batch translation job %s for business %d", jobID, businessID)

	err := translateMenuIntoLanguages(businessID, languageCodes, func(processed, total int) {
		updateTranslationJob(jobID, func(status *TranslationStatus) {
			status.Total = total
			status.Progress = processed
			if total == 0 {
				status.Message = "No menu content found to translate"
			}
		})
	})
	if err != nil {
		return err
	}

	updateTranslationJob(jobID, func(status *TranslationStatus) {
		status.Status = "completed"
		status.Message = "Translation completed successfully"
	})

	log.Printf("Completed batch translation job %s for business %d", jobID, businessID)
	return nil
}
