package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"payverge/internal/database"
)

// Google Translate API structures
type GoogleTranslateRequest struct {
	Q      string `json:"q"`
	Source string `json:"source"`
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

// DetectLanguage detects the language of the given text
// For now, this is a stub that returns English
func (s *TranslationService) DetectLanguage(text string) (string, error) {
	if !s.enabled {
		return "", fmt.Errorf("translation service is not enabled")
	}

	if strings.TrimSpace(text) == "" {
		return "en", nil
	}

	// TODO: Implement actual language detection when Google Cloud is configured
	return "en", nil // Default to English for now
}

// TranslateText translates text to the specified target languages
func (s *TranslationService) TranslateText(text string, targetLanguages []string) (map[string]string, error) {
	if !s.enabled {
		return nil, fmt.Errorf("translation service is not enabled")
	}

	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("text cannot be empty")
	}

	result := make(map[string]string)
	for _, lang := range targetLanguages {
		translatedText := s.translateText(text, lang)
		result[lang] = translatedText
	}

	return result, nil
}

// TranslateMenuItem translates a menu item to all supported languages for a business
func (s *TranslationService) TranslateMenuItem(businessID uint, itemID uint, name, description string) error {
	// Get business supported languages
	businessLanguages, err := s.db.LanguageService.GetBusinessLanguages(businessID)
	if err != nil {
		return fmt.Errorf("failed to get business languages: %w", err)
	}

	if len(businessLanguages) <= 1 {
		return nil // No need to translate
	}

	// Find target languages (exclude default language)
	var targetLanguages []string

	for _, bl := range businessLanguages {
		if !bl.IsDefault {
			targetLanguages = append(targetLanguages, bl.LanguageCode)
		}
	}

	if len(targetLanguages) == 0 {
		return nil // No target languages to translate to
	}

	// Create mock translations for now (replace with actual translation service later)
	for _, langCode := range targetLanguages {
		// Create name translation
		if name != "" {
			nameTranslation := s.translateText(name, langCode)
			err := s.db.TranslationService.SaveTranslation(&database.Translation{
				EntityType:       "menu_item",
				EntityID:         itemID,
				FieldName:        "name",
				LanguageCode:     langCode,
				OriginalText:     name,
				TranslatedText:   nameTranslation,
				IsAutoTranslated: true,
			})
			if err != nil {
				log.Printf("Failed to save name translation for item %d: %v", itemID, err)
			}
		}

		// Create description translation
		if description != "" {
			descTranslation := s.translateText(description, langCode)
			err := s.db.TranslationService.SaveTranslation(&database.Translation{
				EntityType:       "menu_item",
				EntityID:         itemID,
				FieldName:        "description",
				LanguageCode:     langCode,
				OriginalText:     description,
				TranslatedText:   descTranslation,
				IsAutoTranslated: true,
			})
			if err != nil {
				log.Printf("Failed to save description translation for item %d: %v", itemID, err)
			}
		}
	}

	log.Printf("Created mock translations for menu item %d in %d languages", itemID, len(targetLanguages))
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

	// Find target languages (exclude default language)
	var targetLanguages []string

	for _, bl := range businessLanguages {
		if !bl.IsDefault {
			targetLanguages = append(targetLanguages, bl.LanguageCode)
		}
	}

	if len(targetLanguages) == 0 {
		return nil // No target languages to translate to
	}

	// Create mock translations for now (replace with actual translation service later)
	for _, langCode := range targetLanguages {
		// Create name translation
		if name != "" {
			nameTranslation := s.translateText(name, langCode)
			err := s.db.TranslationService.SaveTranslation(&database.Translation{
				EntityType:       "category",
				EntityID:         categoryID,
				FieldName:        "name",
				LanguageCode:     langCode,
				OriginalText:     name,
				TranslatedText:   nameTranslation,
				IsAutoTranslated: true,
			})
			if err != nil {
				log.Printf("Failed to save name translation for category %d: %v", categoryID, err)
			}
		}

		// Create description translation
		if description != "" {
			descTranslation := s.translateText(description, langCode)
			err := s.db.TranslationService.SaveTranslation(&database.Translation{
				EntityType:       "category",
				EntityID:         categoryID,
				FieldName:        "description",
				LanguageCode:     langCode,
				OriginalText:     description,
				TranslatedText:   descTranslation,
				IsAutoTranslated: true,
			})
			if err != nil {
				log.Printf("Failed to save description translation for category %d: %v", categoryID, err)
			}
		}
	}

	log.Printf("Created mock translations for category %d in %d languages", categoryID, len(targetLanguages))
	return nil
}

// TranslateExistingContent translates all existing menu content when new languages are added
func (s *TranslationService) TranslateExistingContent(businessID uint) error {
	log.Printf("Starting translation of existing content for business %d", businessID)

	// Get all categories for this business
	_, categories, err := database.GetMenuByBusinessID(businessID)
	if err != nil {
		return fmt.Errorf("failed to get menu: %w", err)
	}

	// Translate all categories
	for i, category := range categories {
		categoryID := uint(i + 1) // Use index as ID for now
		err := s.TranslateCategory(businessID, categoryID, category.Name, category.Description)
		if err != nil {
			log.Printf("Failed to translate category %d: %v", categoryID, err)
		}

		// Translate all items in this category
		for j, item := range category.Items {
			itemID := uint(j + 1) // Use index as ID for now
			err := s.TranslateMenuItem(businessID, itemID, item.Name, item.Description)
			if err != nil {
				log.Printf("Failed to translate menu item %d: %v", itemID, err)
			}
		}
	}

	log.Printf("Completed translation of existing content for business %d", businessID)
	return nil
}

// translateText translates text using Google Translate API
func (s *TranslationService) translateText(text, targetLang string) string {
	if !s.enabled || strings.TrimSpace(text) == "" {
		return text
	}

	// Create request payload
	request := GoogleTranslateRequest{
		Q:      text,
		Source: "en", // Assume source is English
		Target: targetLang,
		Format: "text",
	}

	// Convert to JSON
	jsonData, err := json.Marshal(request)
	if err != nil {
		log.Printf("Failed to marshal translation request: %v", err)
		return text
	}

	// Create HTTP request
	url := fmt.Sprintf("%s?key=%s", s.apiBaseURL, s.apiKey)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Failed to create translation request: %v", err)
		return text
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	// Make the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Failed to call Google Translate API: %v", err)
		return text
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read translation response: %v", err)
		return text
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("Google Translate API error (status %d): %s", resp.StatusCode, string(body))
		return text
	}

	// Parse response
	var response GoogleTranslateResponse
	if err := json.Unmarshal(body, &response); err != nil {
		log.Printf("Failed to parse translation response: %v", err)
		return text
	}

	if len(response.Data.Translations) > 0 {
		translatedText := response.Data.Translations[0].TranslatedText
		log.Printf("Translated '%s' to '%s' (%s)", text, translatedText, targetLang)
		return translatedText
	}

	return text
}

// TranslateBusiness translates business information to all supported languages
func (s *TranslationService) TranslateBusiness(businessID uint, name, description string) error {
	if !s.enabled {
		log.Println("Translation service disabled, skipping business translation")
		return nil
	}

	// Get business supported languages
	businessLanguages, err := s.db.LanguageService.GetBusinessLanguages(businessID)
	if err != nil {
		return fmt.Errorf("failed to get business languages: %w", err)
	}

	if len(businessLanguages) <= 1 {
		return nil
	}

	// For now, just log that translation would happen here
	log.Printf("Would translate business %d to %d languages", businessID, len(businessLanguages))
	return nil
}

// GetTranslatedContent gets translated content for an entity
func (s *TranslationService) GetTranslatedContent(entityType string, entityID uint, fieldName, languageCode string) (string, error) {
	translation, err := s.db.TranslationService.GetTranslation(entityType, entityID, fieldName, languageCode)
	if err != nil {
		return "", err
	}
	return translation.TranslatedText, nil
}

// InitializeDefaultLanguages adds default supported languages to the database
func (s *TranslationService) InitializeDefaultLanguages() error {
	defaultLanguages := []database.SupportedLanguage{
		{Code: "en", Name: "English", NativeName: "English", IsActive: true},
		{Code: "es", Name: "Spanish", NativeName: "Español", IsActive: true},
		{Code: "fr", Name: "French", NativeName: "Français", IsActive: true},
		{Code: "de", Name: "German", NativeName: "Deutsch", IsActive: true},
		{Code: "it", Name: "Italian", NativeName: "Italiano", IsActive: true},
		{Code: "pt", Name: "Portuguese", NativeName: "Português", IsActive: true},
		{Code: "ru", Name: "Russian", NativeName: "Русский", IsActive: true},
		{Code: "ja", Name: "Japanese", NativeName: "日本語", IsActive: true},
		{Code: "ko", Name: "Korean", NativeName: "한국어", IsActive: true},
		{Code: "zh", Name: "Chinese", NativeName: "中文", IsActive: true},
		{Code: "ar", Name: "Arabic", NativeName: "العربية", IsActive: true},
		{Code: "hi", Name: "Hindi", NativeName: "हिन्दी", IsActive: true},
		{Code: "th", Name: "Thai", NativeName: "ไทย", IsActive: true},
		{Code: "vi", Name: "Vietnamese", NativeName: "Tiếng Việt", IsActive: true},
		{Code: "tr", Name: "Turkish", NativeName: "Türkçe", IsActive: true},
		{Code: "pl", Name: "Polish", NativeName: "Polski", IsActive: true},
		{Code: "nl", Name: "Dutch", NativeName: "Nederlands", IsActive: true},
		{Code: "sv", Name: "Swedish", NativeName: "Svenska", IsActive: true},
		{Code: "da", Name: "Danish", NativeName: "Dansk", IsActive: true},
		{Code: "no", Name: "Norwegian", NativeName: "Norsk", IsActive: true},
	}

	for _, language := range defaultLanguages {
		// Check if language already exists
		var existing database.SupportedLanguage
		err := s.db.GetGorm().Where("code = ?", language.Code).First(&existing).Error

		if err != nil {
			// Language doesn't exist, create it
			if err := s.db.GetGorm().Create(&language).Error; err != nil {
				log.Printf("Failed to create language %s: %v", language.Code, err)
			} else {
				log.Printf("Created language: %s", language.Code)
			}
		}
	}

	return nil
}

// Close closes the translation client (no-op for simplified version)
func (s *TranslationService) Close() error {
	return nil
}
