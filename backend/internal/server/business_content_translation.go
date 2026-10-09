package server

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/locales"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

const (
	specialFeatureTranslationEntityType = "special_feature"
	businessTranslationEntityType       = "business"
	galleryImageTranslationEntityType   = "gallery_image"
)

// businessProseFields are the operator-entered free-text fields on the business
// record that the storefront renders verbatim (hero subtitle, about copy). Brand
// fields (name, address, currency) are intentionally left untranslated.
var businessProseFields = []string{"description", "welcome_message", "about_story"}

// businessContentBackfillInFlight de-duplicates concurrent storefront backfills
// of business-page prose + special features. Key is "businessID:languageCode".
var businessContentBackfillInFlight sync.Map

// applyBusinessContentTranslations overlays stored translations onto the business
// prose fields, special features, and gallery captions for languageCode, mutating
// the passed copies in place. It returns whether any non-empty source field was
// missing a translation so the storefront read path can schedule a backfill —
// mirroring the menu/offers/bundles self-healing pattern.
func applyBusinessContentTranslations(business *database.Business, features []database.BusinessSpecialFeature, galleryImages []database.BusinessGalleryImage, languageCode string) bool {
	dbw := database.GetDBWrapper()
	if dbw == nil || business == nil || strings.TrimSpace(languageCode) == "" {
		return false
	}

	// One batched read of all this business's translations for the language,
	// resolved in-memory — instead of a per-field getLatestTranslationText query
	// for each prose field and each special-feature title/description (an N+1 on
	// the guest storefront page load). Mirrors applyTranslationsToMenu.
	lookup := newTranslationLookup(dbw, business.ID, languageCode)
	missing := false

	if strings.TrimSpace(business.Description) != "" {
		if value, ok := lookup.get(businessTranslationEntityType, business.ID, "description"); ok {
			business.Description = value
		} else {
			missing = true
		}
	}
	if strings.TrimSpace(business.WelcomeMessage) != "" {
		if value, ok := lookup.get(businessTranslationEntityType, business.ID, "welcome_message"); ok {
			business.WelcomeMessage = value
		} else {
			missing = true
		}
	}
	if strings.TrimSpace(business.AboutStory) != "" {
		if value, ok := lookup.get(businessTranslationEntityType, business.ID, "about_story"); ok {
			business.AboutStory = value
		} else {
			missing = true
		}
	}

	for i := range features {
		feature := &features[i]
		if strings.TrimSpace(feature.Title) != "" {
			if value, ok := lookup.get(specialFeatureTranslationEntityType, feature.ID, "title"); ok {
				feature.Title = value
			} else {
				missing = true
			}
		}
		if strings.TrimSpace(feature.Description) != "" {
			if value, ok := lookup.get(specialFeatureTranslationEntityType, feature.ID, "description"); ok {
				feature.Description = value
			} else {
				missing = true
			}
		}
	}

	for i := range galleryImages {
		image := &galleryImages[i]
		if strings.TrimSpace(image.Caption) != "" {
			if value, ok := lookup.get(galleryImageTranslationEntityType, image.ID, "caption"); ok {
				image.Caption = value
			} else {
				missing = true
			}
		}
	}

	return missing
}

// TranslateBusinessContentForLanguages translates the business prose fields and
// special features into the given guest languages (the business default language
// is skipped). It is idempotent at the field level and safe to call repeatedly.
// Exported so the languages-update handler can fire it in the background when an
// operator adds a guest language.
func TranslateBusinessContentForLanguages(businessID uint, languageCodes []string) error {
	return translateBusinessContentIntoLanguages(businessID, languageCodes)
}

func translateBusinessContentIntoLanguages(businessID uint, languageCodes []string) error {
	dbw := database.GetDBWrapper()
	if dbw == nil {
		return nil
	}
	translationService := GetTranslationService()
	if translationService == nil || !translationService.IsEnabled() {
		return fmt.Errorf("translation service is not enabled")
	}

	business, err := database.GetBusinessByID(businessID)
	if err != nil {
		return err
	}
	source := strings.TrimSpace(business.DefaultLanguage)
	if source == "" {
		source = "en"
	}

	features, err := database.GetBusinessSpecialFeatures(businessID)
	if err != nil {
		log.Printf("business content translation: load special features for business %d: %v", businessID, err)
		features = nil
	}

	galleryImages, err := database.GetBusinessGalleryImages(businessID)
	if err != nil {
		log.Printf("business content translation: load gallery images for business %d: %v", businessID, err)
		galleryImages = nil
	}

	proseValues := map[string]string{
		"description":     business.Description,
		"welcome_message": business.WelcomeMessage,
		"about_story":     business.AboutStory,
	}

	for _, lang := range languageCodes {
		target := strings.TrimSpace(lang)
		if target == "" || target == source || !locales.IsGuestLocale(target) {
			continue
		}

		for _, field := range businessProseFields {
			if err := translatePromotionField(dbw, translationService, businessID, businessTranslationEntityType, business.ID, field, proseValues[field], source, target); err != nil {
				log.Printf("business %s translation failed (business %d, %s): %v", field, businessID, target, err)
			}
		}

		for _, feature := range features {
			if err := translatePromotionField(dbw, translationService, businessID, specialFeatureTranslationEntityType, feature.ID, "title", feature.Title, source, target); err != nil {
				log.Printf("special feature title translation failed (feature %d, %s): %v", feature.ID, target, err)
			}
			if err := translatePromotionField(dbw, translationService, businessID, specialFeatureTranslationEntityType, feature.ID, "description", feature.Description, source, target); err != nil {
				log.Printf("special feature description translation failed (feature %d, %s): %v", feature.ID, target, err)
			}
		}

		for _, image := range galleryImages {
			if err := translatePromotionField(dbw, translationService, businessID, galleryImageTranslationEntityType, image.ID, "caption", image.Caption, source, target); err != nil {
				log.Printf("gallery caption translation failed (image %d, %s): %v", image.ID, target, err)
			}
		}
	}

	return nil
}

// scheduleBusinessContentTranslationBackfill kicks off a one-shot, de-duplicated
// background translation of the business prose + special features into
// languageCode when the storefront read path found missing translations.
func scheduleBusinessContentTranslationBackfill(businessID uint, languageCode string) {
	translationService := GetTranslationService()
	if translationService == nil || !translationService.IsEnabled() {
		return
	}

	code := strings.TrimSpace(languageCode)
	if code == "" {
		return
	}

	key := fmt.Sprintf("%d:%s", businessID, code)
	if _, alreadyRunning := businessContentBackfillInFlight.LoadOrStore(key, struct{}{}); alreadyRunning {
		return
	}

	logger.SafeGo(func() {
		defer businessContentBackfillInFlight.Delete(key)
		if err := translateBusinessContentIntoLanguages(businessID, []string{code}); err != nil {
			log.Printf("Business content translation backfill failed for business %d language %s: %v", businessID, code, err)
		}
	})
}

// deleteSpecialFeatureTranslations clears stored special-feature translations for
// a business. UpdateBusinessSpecialFeatures replaces the feature rows wholesale
// (new IDs each save), so old ID-keyed translations are orphaned; clearing them
// keeps the table bounded and lets the storefront read path regenerate fresh
// translations against the new feature IDs.
func deleteSpecialFeatureTranslations(businessID uint) error {
	dbw := database.GetDBWrapper()
	if dbw == nil {
		return nil
	}
	return dbw.GetGorm().
		Where("business_id = ? AND entity_type = ?", businessID, specialFeatureTranslationEntityType).
		Delete(&database.Translation{}).Error
}

// deleteGalleryImageTranslations clears stored gallery-caption translations for
// a business. Used when caption text changed so storefront captions re-translate
// against the live image set (IDs are now preserved by the gallery upsert).
func deleteGalleryImageTranslations(businessID uint) error {
	dbw := database.GetDBWrapper()
	if dbw == nil {
		return nil
	}
	return dbw.GetGorm().
		Where("business_id = ? AND entity_type = ?", businessID, galleryImageTranslationEntityType).
		Delete(&database.Translation{}).Error
}

// deleteGalleryImageTranslationsForIDs clears caption translations for specific
// gallery image rows (e.g. after those rows were deleted from the gallery).
func deleteGalleryImageTranslationsForIDs(businessID uint, entityIDs []uint) error {
	if len(entityIDs) == 0 {
		return nil
	}
	dbw := database.GetDBWrapper()
	if dbw == nil {
		return nil
	}
	return dbw.GetGorm().
		Where("business_id = ? AND entity_type = ? AND entity_id IN ?", businessID, galleryImageTranslationEntityType, entityIDs).
		Delete(&database.Translation{}).Error
}
