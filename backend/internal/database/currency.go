package database

import (
	"time"
	"gorm.io/gorm"
)

// SupportedCurrency represents a currency that the platform supports
type SupportedCurrency struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Code        string    `json:"code" gorm:"uniqueIndex;size:3;not null"` // ISO 4217 code (USD, EUR, ARS, etc.)
	Name        string    `json:"name" gorm:"size:100;not null"`           // Full name (US Dollar, Euro, etc.)
	Symbol      string    `json:"symbol" gorm:"size:10;not null"`          // Currency symbol ($, €, etc.)
	IsActive    bool      `json:"is_active" gorm:"default:true"`           // Whether this currency is available
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ExchangeRate represents real-time exchange rates from Coinbase
type ExchangeRate struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	FromCurrency string    `json:"from_currency" gorm:"size:3;not null;index"` // Always USDC for our case
	ToCurrency   string    `json:"to_currency" gorm:"size:3;not null;index"`   // Target fiat currency
	Rate         float64   `json:"rate" gorm:"not null"`                       // Exchange rate (1 USDC = X fiat)
	Source       string    `json:"source" gorm:"size:50;default:'coinbase'"`   // Rate source (coinbase, etc.)
	FetchedAt    time.Time `json:"fetched_at" gorm:"not null"`                 // When this rate was fetched
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// BusinessCurrency represents which currencies a business supports for display
type BusinessCurrency struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	BusinessID       uint      `json:"business_id" gorm:"not null;index"`
	CurrencyCode     string    `json:"currency_code" gorm:"size:3;not null"`
	IsPreferred      bool      `json:"is_preferred" gorm:"default:false"` // One preferred currency per business
	DisplayOrder     int       `json:"display_order" gorm:"default:0"`    // Order in currency selector
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`

	// Relationships
	Business Business `json:"business" gorm:"foreignKey:BusinessID"`
}

// SupportedLanguage represents a language that the platform supports
type SupportedLanguage struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Code      string    `json:"code" gorm:"uniqueIndex;size:5;not null"` // ISO 639-1 code (en, es, fr, etc.)
	Name      string    `json:"name" gorm:"size:100;not null"`           // Full name (English, Spanish, etc.)
	NativeName string   `json:"native_name" gorm:"size:100;not null"`    // Native name (English, Español, etc.)
	IsActive  bool      `json:"is_active" gorm:"default:true"`           // Whether this language is available
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BusinessLanguage represents which languages a business supports
type BusinessLanguage struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	BusinessID   uint      `json:"business_id" gorm:"not null;index"`
	LanguageCode string    `json:"language_code" gorm:"size:5;not null"`
	IsDefault    bool      `json:"is_default" gorm:"default:false"` // One default language per business
	DisplayOrder int       `json:"display_order" gorm:"default:0"`  // Order in language selector
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// Relationships
	Business Business `json:"business" gorm:"foreignKey:BusinessID"`
}

// Translation represents translated content for menu items, business info, etc.
type Translation struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	EntityType   string    `json:"entity_type" gorm:"size:50;not null;index"` // 'menu_item', 'business', 'category', etc.
	EntityID     uint      `json:"entity_id" gorm:"not null;index"`           // ID of the entity being translated
	FieldName    string    `json:"field_name" gorm:"size:50;not null"`        // Field being translated ('name', 'description', etc.)
	LanguageCode string    `json:"language_code" gorm:"size:5;not null"`      // Target language
	OriginalText string    `json:"original_text" gorm:"type:text"`            // Original text (for reference)
	TranslatedText string  `json:"translated_text" gorm:"type:text;not null"` // Translated text
	IsAutoTranslated bool   `json:"is_auto_translated" gorm:"default:true"`   // Whether this was auto-translated
	TranslationSource string `json:"translation_source" gorm:"size:50;default:'google'"` // Translation service used
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CurrencyService handles currency-related database operations
type CurrencyService struct {
	db *gorm.DB
}

// NewCurrencyService creates a new currency service
func NewCurrencyService(db *gorm.DB) *CurrencyService {
	return &CurrencyService{db: db}
}

// GetSupportedCurrencies returns all active supported currencies
func (s *CurrencyService) GetSupportedCurrencies() ([]SupportedCurrency, error) {
	var currencies []SupportedCurrency
	err := s.db.Where("is_active = ?", true).Order("name").Find(&currencies).Error
	return currencies, err
}

// GetExchangeRate gets the latest exchange rate for a currency pair
func (s *CurrencyService) GetExchangeRate(fromCurrency, toCurrency string) (*ExchangeRate, error) {
	var rate ExchangeRate
	err := s.db.Where("from_currency = ? AND to_currency = ?", fromCurrency, toCurrency).
		Order("fetched_at DESC").
		First(&rate).Error
	return &rate, err
}

// UpdateExchangeRate updates or creates an exchange rate
func (s *CurrencyService) UpdateExchangeRate(rate *ExchangeRate) error {
	return s.db.Save(rate).Error
}

// GetBusinessCurrencies returns all currencies supported by a business
func (s *CurrencyService) GetBusinessCurrencies(businessID uint) ([]BusinessCurrency, error) {
	var currencies []BusinessCurrency
	err := s.db.Where("business_id = ?", businessID).
		Order("is_preferred DESC, display_order ASC").
		Find(&currencies).Error
	return currencies, err
}

// SetBusinessCurrencies sets the currencies supported by a business
func (s *CurrencyService) SetBusinessCurrencies(businessID uint, currencyCodes []string, preferredCode string) error {
	// Start transaction
	tx := s.db.Begin()
	
	// Delete existing currencies for this business
	if err := tx.Where("business_id = ?", businessID).Delete(&BusinessCurrency{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	
	// Add new currencies
	for i, code := range currencyCodes {
		businessCurrency := BusinessCurrency{
			BusinessID:   businessID,
			CurrencyCode: code,
			IsPreferred:  code == preferredCode,
			DisplayOrder: i,
		}
		if err := tx.Create(&businessCurrency).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	
	return tx.Commit().Error
}

// LanguageService handles language-related database operations
type LanguageService struct {
	db *gorm.DB
}

// NewLanguageService creates a new language service
func NewLanguageService(db *gorm.DB) *LanguageService {
	return &LanguageService{db: db}
}

// GetSupportedLanguages returns all active supported languages
func (s *LanguageService) GetSupportedLanguages() ([]SupportedLanguage, error) {
	var languages []SupportedLanguage
	err := s.db.Where("is_active = ?", true).Order("name").Find(&languages).Error
	return languages, err
}

// GetBusinessLanguages returns all languages supported by a business
func (s *LanguageService) GetBusinessLanguages(businessID uint) ([]BusinessLanguage, error) {
	var languages []BusinessLanguage
	err := s.db.Where("business_id = ?", businessID).
		Order("is_default DESC, display_order ASC").
		Find(&languages).Error
	return languages, err
}

// SetBusinessLanguages sets the languages supported by a business
func (s *LanguageService) SetBusinessLanguages(businessID uint, languageCodes []string, defaultCode string) error {
	// Start transaction
	tx := s.db.Begin()
	
	// Delete existing languages for this business
	if err := tx.Where("business_id = ?", businessID).Delete(&BusinessLanguage{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	
	// Add new languages
	for i, code := range languageCodes {
		businessLanguage := BusinessLanguage{
			BusinessID:   businessID,
			LanguageCode: code,
			IsDefault:    code == defaultCode,
			DisplayOrder: i,
		}
		if err := tx.Create(&businessLanguage).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	
	return tx.Commit().Error
}

// TranslationService handles translation-related database operations
type TranslationService struct {
	db *gorm.DB
}

// NewTranslationService creates a new translation service
func NewTranslationService(db *gorm.DB) *TranslationService {
	return &TranslationService{db: db}
}

// GetTranslation gets a specific translation
func (s *TranslationService) GetTranslation(entityType string, entityID uint, fieldName, languageCode string) (*Translation, error) {
	var translation Translation
	err := s.db.Where("entity_type = ? AND entity_id = ? AND field_name = ? AND language_code = ?",
		entityType, entityID, fieldName, languageCode).First(&translation).Error
	return &translation, err
}

// SaveTranslation saves or updates a translation
func (s *TranslationService) SaveTranslation(translation *Translation) error {
	return s.db.Save(translation).Error
}

// GetEntityTranslations gets all translations for a specific entity
func (s *TranslationService) GetEntityTranslations(entityType string, entityID uint) ([]Translation, error) {
	var translations []Translation
	err := s.db.Where("entity_type = ? AND entity_id = ?", entityType, entityID).Find(&translations).Error
	return translations, err
}

// DeleteEntityTranslations deletes all translations for a specific entity
func (s *TranslationService) DeleteEntityTranslations(entityType string, entityID uint) error {
	return s.db.Where("entity_type = ? AND entity_id = ?", entityType, entityID).Delete(&Translation{}).Error
}
