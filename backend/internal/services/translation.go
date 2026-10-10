package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/httpclientx"
	"github.com/stdevmac/payverge/backend/internal/locales"
)

// Google Translate API structures
type GoogleTranslateRequest struct {
	Q      string `json:"q"`
	Source string `json:"source,omitempty"`
	Target string `json:"target"`
	Format string `json:"format"`
}

type GoogleTranslateResponse struct {
	Data struct {
		Translations []struct {
			TranslatedText string `json:"translatedText"`
		} `json:"translations"`
	} `json:"data"`
}

// TranslationService handles automatic translation of content
type TranslationService struct {
	db         *database.DB
	enabled    bool
	apiKey     string
	apiBaseURL string
}

// NewTranslationService creates a new translation service
func NewTranslationService(db *database.DB, apiKey string) *TranslationService {

	service := &TranslationService{
		db:         db,
		enabled:    apiKey != "",
		apiKey:     apiKey,
		apiBaseURL: "https://translation.googleapis.com/language/translate/v2",
	}

	if apiKey != "" {
		log.Println("Translation service enabled with Google Translate API")
	} else {
		log.Println("Translation service disabled - GOOGLE_TRANSLATE_API_KEY not set")
	}

	return service
}

// IsEnabled returns whether the translation service is available
func (s *TranslationService) IsEnabled() bool {
	return s.enabled
}

// TranslateText translates text to the specified target languages.
// Source is omitted so Google Translate v2 can detect the language.
func (s *TranslationService) TranslateText(text string, targetLanguages []string) (map[string]string, error) {
	return s.TranslateTextWithSource(text, "", targetLanguages)
}

// TranslateTextWithSource translates text from a specific source language to target languages
func (s *TranslationService) TranslateTextWithSource(text, sourceLang string, targetLanguages []string) (map[string]string, error) {
	if !s.enabled {
		return nil, fmt.Errorf("translation service is not enabled")
	}

	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("text cannot be empty")
	}

	result := make(map[string]string)
	for _, lang := range targetLanguages {
		if !locales.IsGuestLocale(lang) {
			return nil, fmt.Errorf("unsupported guest target language: %s", lang)
		}
		translatedText, err := s.translateTextWithSource(text, sourceLang, lang)
		if err != nil {
			return nil, err
		}
		result[lang] = translatedText
	}

	return result, nil
}

// hasFreshTranslation reports whether a non-empty translation for
// (entityType, entityID, field, lang) already exists whose stored OriginalText
// equals the current source text. When true, the batch/job path skips the
// external translate call + save — the §3.7 fix-1 unchanged-string skip that
// turns a no-op re-sync from thousands of external calls into near-zero.
func (s *TranslationService) hasFreshTranslation(businessID uint, entityType string, entityID uint, field, lang, source string) bool {
	var existing database.Translation
	// Latest row wins, mirroring the read-path (applyTranslationsToMenu) semantics.
	err := s.db.GetGorm().
		Where("business_id = ? AND entity_type = ? AND entity_id = ? AND field_name = ? AND language_code = ?",
			businessID, entityType, entityID, field, lang).
		Order("id DESC").
		First(&existing).Error
	if err != nil {
		return false
	}
	if existing.TranslatedText == "" || existing.OriginalText != source {
		return false
	}
	// Identity/echo rows are leftover provider failures (or proper-noun
	// echoes). They must not be treated as a completed translation.
	return !isIdentityTranslation(existing.OriginalText, existing.TranslatedText)
}

// TranslateMenuItemToLanguages translates a menu item to explicit target languages.
func (s *TranslationService) TranslateMenuItemToLanguages(businessID uint, itemID uint, name, description string, targetLanguages []string) error {
	targetLanguages = supportedTranslationTargets(targetLanguages)

	if len(targetLanguages) == 0 {
		return nil
	}

	var errs []error
	for _, langCode := range targetLanguages {
		if name != "" && !s.hasFreshTranslation(businessID, "menu_item", itemID, "name", langCode, name) {
			nameTranslation, err := s.translateTextWithSource(name, "", langCode)
			if err != nil {
				errs = append(errs, fmt.Errorf("item %d name -> %s: %w", itemID, langCode, err))
			} else if err := s.saveAutoTranslation(businessID, "menu_item", itemID, "name", langCode, name, nameTranslation); err != nil {
				log.Printf("Failed to save name translation for item %d: %v", itemID, err)
				errs = append(errs, err)
			}
		}

		if description != "" && !s.hasFreshTranslation(businessID, "menu_item", itemID, "description", langCode, description) {
			descTranslation, err := s.translateTextWithSource(description, "", langCode)
			if err != nil {
				errs = append(errs, fmt.Errorf("item %d description -> %s: %w", itemID, langCode, err))
			} else if err := s.saveAutoTranslation(businessID, "menu_item", itemID, "description", langCode, description, descTranslation); err != nil {
				log.Printf("Failed to save description translation for item %d: %v", itemID, err)
				errs = append(errs, err)
			}
		}
	}

	if err := errors.Join(errs...); err != nil {
		return err
	}

	log.Printf("Created translations for menu item %d in %d languages", itemID, len(targetLanguages))
	return nil
}

// TranslateCategory translates a category to all supported languages for a business
func (s *TranslationService) TranslateCategory(businessID uint, categoryID uint, name, description string) error {
	// Get business supported languages
	businessLanguages, err := s.db.LanguageService.GetBusinessLanguages(businessID)
	if err != nil {
		return fmt.Errorf("failed to get business languages: %w", err)
	}

	if len(businessLanguages) <= 1 {
		return nil // No need to translate
	}

	targetLanguages := targetLanguagesFromBusinessLanguages(businessLanguages)

	return s.TranslateCategoryToLanguages(businessID, categoryID, name, description, targetLanguages)
}

// TranslateCategoryToLanguages translates a category to explicit target languages.
func (s *TranslationService) TranslateCategoryToLanguages(businessID uint, categoryID uint, name, description string, targetLanguages []string) error {
	targetLanguages = supportedTranslationTargets(targetLanguages)

	if len(targetLanguages) == 0 {
		return nil
	}

	var errs []error
	for _, langCode := range targetLanguages {
		if name != "" && !s.hasFreshTranslation(businessID, "category", categoryID, "name", langCode, name) {
			nameTranslation, err := s.translateTextWithSource(name, "", langCode)
			if err != nil {
				errs = append(errs, fmt.Errorf("category %d name -> %s: %w", categoryID, langCode, err))
			} else if err := s.saveAutoTranslation(businessID, "category", categoryID, "name", langCode, name, nameTranslation); err != nil {
				log.Printf("Failed to save name translation for category %d: %v", categoryID, err)
				errs = append(errs, err)
			}
		}

		if description != "" && !s.hasFreshTranslation(businessID, "category", categoryID, "description", langCode, description) {
			descTranslation, err := s.translateTextWithSource(description, "", langCode)
			if err != nil {
				errs = append(errs, fmt.Errorf("category %d description -> %s: %w", categoryID, langCode, err))
			} else if err := s.saveAutoTranslation(businessID, "category", categoryID, "description", langCode, description, descTranslation); err != nil {
				log.Printf("Failed to save description translation for category %d: %v", categoryID, err)
				errs = append(errs, err)
			}
		}
	}

	if err := errors.Join(errs...); err != nil {
		return err
	}

	log.Printf("Created translations for category %d in %d languages", categoryID, len(targetLanguages))
	return nil
}

func (s *TranslationService) saveAutoTranslation(businessID uint, entityType string, entityID uint, field, lang, original, translated string) error {
	if isIdentityTranslation(original, translated) {
		return nil
	}
	return s.db.TranslationService.SaveTranslation(&database.Translation{
		BusinessID:       businessID,
		EntityType:       entityType,
		EntityID:         entityID,
		FieldName:        field,
		LanguageCode:     lang,
		OriginalText:     original,
		TranslatedText:   translated,
		IsAutoTranslated: true,
	})
}

func isIdentityTranslation(original, translated string) bool {
	o := strings.TrimSpace(original)
	t := strings.TrimSpace(translated)
	return o != "" && t != "" && o == t
}

func targetLanguagesFromBusinessLanguages(businessLanguages []database.BusinessLanguage) []string {
	targetLanguages := make([]string, 0, len(businessLanguages))
	for _, bl := range businessLanguages {
		if !bl.IsDefault {
			targetLanguages = append(targetLanguages, bl.LanguageCode)
		}
	}

	return targetLanguages
}

func supportedTranslationTargets(targetLanguages []string) []string {
	supported := make([]string, 0, len(targetLanguages))
	seen := make(map[string]struct{}, len(targetLanguages))
	for _, lang := range targetLanguages {
		if !locales.IsGuestLocale(lang) {
			log.Printf("Skipping unsupported guest target language: %s", lang)
			continue
		}
		if _, ok := seen[lang]; ok {
			continue
		}
		seen[lang] = struct{}{}
		supported = append(supported, lang)
	}

	return supported
}

// googleTranslateSource returns a v2 language code, or empty to omit source
// (auto-detect). "auto" is not a valid v2 code and must never be sent.
func googleTranslateSource(sourceLang string) string {
	sourceLang = strings.TrimSpace(sourceLang)
	if sourceLang == "" || strings.EqualFold(sourceLang, "auto") {
		return ""
	}
	if locale, ok := locales.Lookup(sourceLang); ok {
		if locale.TranslationProviderTarget != "" {
			return locale.TranslationProviderTarget
		}
		return locale.Canonical
	}
	return ""
}

// translateTextWithSource translates text using Google Translate API with specified source language
func (s *TranslationService) translateTextWithSource(text, sourceLang, targetLang string) (string, error) {
	if !s.enabled {
		return "", fmt.Errorf("translation service is not enabled")
	}
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("text cannot be empty")
	}

	locale, ok := locales.Lookup(targetLang)
	if !ok {
		return "", fmt.Errorf("unsupported translation target language: %s", targetLang)
	}

	providerTargetLang := locale.TranslationProviderTarget
	providerSourceLang := googleTranslateSource(sourceLang)
	if providerSourceLang != "" && (providerSourceLang == targetLang || providerSourceLang == providerTargetLang) {
		return text, nil
	}

	request := GoogleTranslateRequest{
		Q:      text,
		Source: providerSourceLang,
		Target: providerTargetLang,
		Format: "text",
	}

	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("marshal translation request: %w", err)
	}

	// The API key rides in the query string, so every error that can embed the
	// URL goes through RedactURLError before it is wrapped or logged.
	url := fmt.Sprintf("%s?key=%s", s.apiBaseURL, s.apiKey)
	req, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("create translation request: %w", httpclientx.RedactURLError(err))
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("call Google Translate API: %w", httpclientx.RedactURLError(err))
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read translation response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("Google Translate API error (status %d): %s", resp.StatusCode, string(body))
		return "", fmt.Errorf("Google Translate API error (status %d)", resp.StatusCode)
	}

	var response GoogleTranslateResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("parse translation response: %w", err)
	}

	if len(response.Data.Translations) == 0 {
		return "", fmt.Errorf("translation provider returned no translations")
	}

	translatedText := response.Data.Translations[0].TranslatedText
	log.Printf("Translated '%s' to '%s' (%s)", text, translatedText, targetLang)
	return translatedText, nil
}

// GetTranslatedContent gets translated content for an entity
func (s *TranslationService) GetTranslatedContent(businessID uint, entityType string, entityID uint, fieldName, languageCode string) (string, error) {
	translation, err := s.db.TranslationService.GetTranslation(businessID, entityType, entityID, fieldName, languageCode)
	if err != nil {
		return "", err
	}
	return translation.TranslatedText, nil
}

// InitializeDefaultLanguages populates supported_languages from the canonical
// locale registry. Every locale flagged GuestLocale becomes an active row —
// both operator locales (en/es/es-AR, full dashboard UI) and guest-only
// menu-translation targets (ar, de, fr, hi, it, ja, ko, nl, pt, ru, th, tr,
// zh). Existing rows are kept in sync; rows for codes the registry no longer
// recognizes are left alone so an operator who hand-added an experimental
// locale doesn't lose it on the next boot.
//
// History: a 2026-05-14 fix-up commit narrowed the seed to the 3 operator
// locales AND added a UPDATE … WHERE code NOT IN (operator-set) sweep that
// silently disabled the other 18 every backend boot. That wiped guest-language
// menu support in production. Do not reintroduce a global "deactivate the
// rest" sweep here.
func (s *TranslationService) InitializeDefaultLanguages() error {
	for _, locale := range locales.GuestLocales() {
		seed := database.SupportedLanguage{
			Code:       locale.Canonical,
			Name:       locale.DisplayName,
			NativeName: locale.NativeName,
			IsActive:   true,
		}

		var existing database.SupportedLanguage
		err := s.db.GetGorm().Where("code = ?", seed.Code).First(&existing).Error
		if err != nil {
			if err := s.db.GetGorm().Create(&seed).Error; err != nil {
				log.Printf("Failed to create language %s: %v", seed.Code, err)
			} else {
				log.Printf("Created language: %s", seed.Code)
			}
			continue
		}

		// Self-heal stale metadata. Production seed wrote ASCII-only native
		// names ("Espanol", "Francais") at one point; reactivating rows that
		// the rogue 2026-05-14 sweep marked inactive also flows through here.
		if existing.NativeName != seed.NativeName || existing.Name != seed.Name || !existing.IsActive {
			if err := s.db.GetGorm().
				Model(&database.SupportedLanguage{}).
				Where("code = ?", seed.Code).
				Updates(map[string]interface{}{
					"name":        seed.Name,
					"native_name": seed.NativeName,
					"is_active":   true,
				}).Error; err != nil {
				log.Printf("Failed to sync language %s: %v", seed.Code, err)
			} else {
				log.Printf("Synced canonical names for language: %s", seed.Code)
			}
		}
	}

	database.InvalidatePublicSupportedLanguages()
	return nil
}

// Close closes the translation client (no-op for simplified version)
func (s *TranslationService) Close() error {
	return nil
}
