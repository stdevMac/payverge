package database

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// defaultMaxRateStepFactor bounds how far a single exchange-rate observation may
// move from the prior accepted rate before it is treated as corrupt and
// rejected. A factor of 10 catches the realistic failure mode (an
// order-of-magnitude garbage value from a malformed upstream payload) while
// never rejecting a genuine fiat move, which is far below 10× per fetch cycle.
// Tunable via EXCHANGE_RATE_MAX_STEP_FACTOR.
const defaultMaxRateStepFactor = 10.0

func maxRateStepFactor() float64 {
	if raw := strings.TrimSpace(os.Getenv("EXCHANGE_RATE_MAX_STEP_FACTOR")); raw != "" {
		if f, err := strconv.ParseFloat(raw, 64); err == nil && f > 1 {
			return f
		}
	}
	return defaultMaxRateStepFactor
}

// plausibleRateStep reports whether moving from prior to next is within the
// configured band. The first observation (prior <= 0) is always plausible.
func plausibleRateStep(prior, next float64) bool {
	if prior <= 0 {
		return true
	}
	if next <= 0 {
		return false
	}
	factor := maxRateStepFactor()
	return next <= prior*factor && next >= prior/factor
}

// SupportedCurrency represents a currency that the platform supports
type SupportedCurrency struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Code      string    `json:"code" gorm:"uniqueIndex;size:3;not null"` // ISO 4217 code (USD, EUR, ARS, etc.)
	Name      string    `json:"name" gorm:"size:100;not null"`           // Full name (US Dollar, Euro, etc.)
	Symbol    string    `json:"symbol" gorm:"size:10;not null"`          // Currency symbol ($, €, etc.)
	IsActive  bool      `json:"is_active" gorm:"default:true"`           // Whether this currency is available
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ExchangeRate represents real-time exchange rates from Coinbase.
//
// Storage is a store-on-change step function: a row is written only when a
// pair's rate actually changes, so each row means "this rate held from
// FetchedAt onward." This keeps GetExchangeRateAtOrBefore (as-of accounting
// lookups) lossless while bounding row growth to real volatility rather than
// poll frequency. FetchedAt marks when the value began; LastSeenAt marks when
// the value was last confirmed (advanced in place on every unchanged fetch).
// Staleness checks key off LastSeenAt; as-of history keys off FetchedAt.
//
// The composite index (from_currency, to_currency, fetched_at DESC) backs both
// the latest-rate lookup and the as-of lookup. The legacy single-column indexes
// were useless because every row shares from_currency='USDC'.
type ExchangeRate struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	FromCurrency string    `json:"from_currency" gorm:"size:10;not null;index:idx_exchange_rates_pair_fetched,priority:1"` // USDC or other crypto codes
	ToCurrency   string    `json:"to_currency" gorm:"size:10;not null;index:idx_exchange_rates_pair_fetched,priority:2"`   // Target currency (fiat or crypto)
	Rate         float64   `json:"rate" gorm:"not null"`                                                                   // Exchange rate (1 USDC = X target)
	Source       string    `json:"source" gorm:"size:50;default:'coinbase'"`                                               // Rate source (coinbase, etc.)
	FetchedAt    time.Time `json:"fetched_at" gorm:"not null;index:idx_exchange_rates_pair_fetched,priority:3,sort:desc"`  // When this rate value began
	LastSeenAt   time.Time `json:"last_seen_at" gorm:"index"`                                                              // When this rate was last confirmed current
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// EffectiveSeenAt returns the timestamp at which this rate was last confirmed
// current, used for staleness decisions. It falls back to FetchedAt for legacy
// rows whose LastSeenAt predates the store-on-change migration backfill.
func (r ExchangeRate) EffectiveSeenAt() time.Time {
	if r.LastSeenAt.IsZero() {
		return r.FetchedAt
	}
	return r.LastSeenAt
}

// BusinessCurrency represents which currencies a business supports for display
type BusinessCurrency struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	BusinessID   uint      `json:"business_id" gorm:"not null;index"`
	CurrencyCode string    `json:"currency_code" gorm:"size:3;not null"`
	IsPreferred  bool      `json:"is_preferred" gorm:"default:false"` // One preferred currency per business
	DisplayOrder int       `json:"display_order" gorm:"default:0"`    // Order in currency selector
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// Relationships
	Business Business `json:"business" gorm:"foreignKey:BusinessID"`
}

// SupportedLanguage represents a language that the platform supports
type SupportedLanguage struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	Code       string    `json:"code" gorm:"uniqueIndex;size:16;not null"` // ISO 639-1 / BCP-47 code (en, es, es-AR, fr, etc.)
	Name       string    `json:"name" gorm:"size:100;not null"`            // Full name (English, Spanish, etc.)
	NativeName string    `json:"native_name" gorm:"size:100;not null"`     // Native name (English, Español, etc.)
	IsActive   bool      `json:"is_active" gorm:"default:true"`            // Whether this language is available
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// BusinessLanguage represents which languages a business supports
type BusinessLanguage struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	BusinessID   uint      `json:"business_id" gorm:"not null;index"`
	LanguageCode string    `json:"language_code" gorm:"size:16;not null"`
	IsDefault    bool      `json:"is_default" gorm:"default:false"` // One default language per business
	DisplayOrder int       `json:"display_order" gorm:"default:0"`  // Order in language selector
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// Relationships. Never serialized: the public storefront and guest table
	// extras return []BusinessLanguage raw. omitempty is a no-op on a struct
	// value, so a zero Business was ~2.8KB of padding per language (#523/#566).
	Business Business `json:"-" gorm:"foreignKey:BusinessID"`
}

// Translation represents translated content for menu items, business info, etc.
type Translation struct {
	ID                uint      `json:"id" gorm:"primaryKey"`
	BusinessID        uint      `json:"business_id" gorm:"not null;index"`
	EntityType        string    `json:"entity_type" gorm:"size:50;not null;index"`          // 'menu_item', 'business', 'category', etc.
	EntityID          uint      `json:"entity_id" gorm:"not null;index"`                    // ID of the entity being translated
	FieldName         string    `json:"field_name" gorm:"size:50;not null"`                 // Field being translated ('name', 'description', etc.)
	LanguageCode      string    `json:"language_code" gorm:"size:16;not null"`              // Target language
	OriginalText      string    `json:"original_text" gorm:"type:text"`                     // Original text (for reference)
	TranslatedText    string    `json:"translated_text" gorm:"type:text;not null"`          // Translated text
	IsAutoTranslated  bool      `json:"is_auto_translated" gorm:"default:true"`             // Whether this was auto-translated
	TranslationSource string    `json:"translation_source" gorm:"size:50;default:'google'"` // Translation service used
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
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

// GetExchangeRateAtOrBefore gets the latest exchange rate for a currency pair
// that was available at or before the provided timestamp.
func (s *CurrencyService) GetExchangeRateAtOrBefore(fromCurrency, toCurrency string, at time.Time) (*ExchangeRate, error) {
	var rate ExchangeRate
	query := s.db.Where("from_currency = ? AND to_currency = ?", fromCurrency, toCurrency)
	if !at.IsZero() {
		query = query.Where("fetched_at <= ?", at.UTC())
	}

	err := query.Order("fetched_at DESC").First(&rate).Error
	return &rate, err
}

// RecordRateObservation records an observed rate for a pair using store-on-change
// semantics. If the newest stored rate for the pair differs from the observed
// rate (or no row exists), a new row is inserted with FetchedAt = LastSeenAt =
// at. If it is unchanged, the existing newest row's LastSeenAt is advanced to at
// in place — no new row. This is the true upsert that the misnamed insert-only
// UpdateExchangeRate never was.
func (s *CurrencyService) RecordRateObservation(fromCurrency, toCurrency string, rate float64, source string, at time.Time) error {
	at = at.UTC()
	if source == "" {
		source = "coinbase"
	}

	var latest ExchangeRate
	err := s.db.Where("from_currency = ? AND to_currency = ?", fromCurrency, toCurrency).
		Order("fetched_at DESC").
		First(&latest).Error

	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		// No history for this pair yet — insert the first observation.
		return s.db.Create(&ExchangeRate{
			FromCurrency: fromCurrency,
			ToCurrency:   toCurrency,
			Rate:         rate,
			Source:       source,
			FetchedAt:    at,
			LastSeenAt:   at,
		}).Error
	case err != nil:
		return err
	case latest.Rate != rate:
		// Reject an implausible single-step move (corrupt upstream value) so a
		// garbage rate is never stored and locked into quotes; keep the prior.
		if !plausibleRateStep(latest.Rate, rate) {
			log.Printf("exchange-rate: rejecting implausible %s->%s observation %.6f (prior %.6f, max step %.0fx)",
				fromCurrency, toCurrency, rate, latest.Rate, maxRateStepFactor())
			return nil
		}
		// Rate changed — append a new step.
		return s.db.Create(&ExchangeRate{
			FromCurrency: fromCurrency,
			ToCurrency:   toCurrency,
			Rate:         rate,
			Source:       source,
			FetchedAt:    at,
			LastSeenAt:   at,
		}).Error
	default:
		// Rate unchanged — confirm the current step in place. Guard against an
		// out-of-order observation regressing last_seen_at.
		if at.Before(latest.LastSeenAt) {
			return nil
		}
		return s.db.Model(&ExchangeRate{}).
			Where("id = ?", latest.ID).
			Update("last_seen_at", at).Error
	}
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
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

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
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

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

	// Update the business's default_language field
	if err := tx.Model(&Business{}).Where("id = ?", businessID).Update("default_language", defaultCode).Error; err != nil {
		tx.Rollback()
		return err
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
func (s *TranslationService) GetTranslation(businessID uint, entityType string, entityID uint, fieldName, languageCode string) (*Translation, error) {
	var translation Translation
	err := s.db.Where("business_id = ? AND entity_type = ? AND entity_id = ? AND field_name = ? AND language_code = ?",
		businessID, entityType, entityID, fieldName, languageCode).First(&translation).Error
	return &translation, err
}

// SaveTranslation saves or updates a translation
func (s *TranslationService) SaveTranslation(translation *Translation) error {
	return s.db.Save(translation).Error
}
