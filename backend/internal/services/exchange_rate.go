package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
	"log"

	"payverge/internal/database"
)

// CoinbaseExchangeRateResponse represents the response from Coinbase API
type CoinbaseExchangeRateResponse struct {
	Data struct {
		Currency string            `json:"currency"`
		Rates    map[string]string `json:"rates"`
	} `json:"data"`
}

// ExchangeRateService handles fetching and caching exchange rates
type ExchangeRateService struct {
	db           *database.DB
	coinbaseURL  string
	httpClient   *http.Client
	lastFetched  time.Time
	cacheDuration time.Duration
}

// NewExchangeRateService creates a new exchange rate service
func NewExchangeRateService(db *database.DB) *ExchangeRateService {
	return &ExchangeRateService{
		db:           db,
		coinbaseURL:  "https://api.coinbase.com/v2/exchange-rates?currency=USDC",
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		cacheDuration: 5 * time.Minute, // Cache rates for 5 minutes
	}
}

// FetchLatestRates fetches the latest exchange rates from Coinbase
func (s *ExchangeRateService) FetchLatestRates() error {
	// Check if we need to fetch (rate limiting)
	if time.Since(s.lastFetched) < s.cacheDuration {
		log.Println("Exchange rates recently fetched, skipping...")
		return nil
	}

	log.Println("Fetching latest exchange rates from Coinbase...")

	resp, err := s.httpClient.Get(s.coinbaseURL)
	if err != nil {
		return fmt.Errorf("failed to fetch exchange rates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("coinbase API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	var coinbaseResp CoinbaseExchangeRateResponse
	if err := json.Unmarshal(body, &coinbaseResp); err != nil {
		return fmt.Errorf("failed to parse coinbase response: %w", err)
	}

	// Update rates in database
	fetchedAt := time.Now()
	for currency, rateStr := range coinbaseResp.Data.Rates {
		// Parse rate as float
		var rate float64
		if _, err := fmt.Sscanf(rateStr, "%f", &rate); err != nil {
			log.Printf("Failed to parse rate for %s: %v", currency, err)
			continue
		}

		// Skip if rate is 0 or invalid
		if rate <= 0 {
			continue
		}

		exchangeRate := &database.ExchangeRate{
			FromCurrency: "USDC",
			ToCurrency:   currency,
			Rate:         rate,
			Source:       "coinbase",
			FetchedAt:    fetchedAt,
		}

		if err := s.db.CurrencyService.UpdateExchangeRate(exchangeRate); err != nil {
			log.Printf("Failed to update exchange rate for %s: %v", currency, err)
		}
	}

	s.lastFetched = fetchedAt
	log.Printf("Successfully updated %d exchange rates", len(coinbaseResp.Data.Rates))
	return nil
}

// GetExchangeRate gets the exchange rate for a currency pair
func (s *ExchangeRateService) GetExchangeRate(fromCurrency, toCurrency string) (float64, error) {
	// If converting to the same currency, return 1
	if fromCurrency == toCurrency {
		return 1.0, nil
	}

	rate, err := s.db.CurrencyService.GetExchangeRate(fromCurrency, toCurrency)
	if err != nil {
		// Try to fetch latest rates if not found
		if fetchErr := s.FetchLatestRates(); fetchErr != nil {
			return 0, fmt.Errorf("exchange rate not found and failed to fetch: %w", fetchErr)
		}

		// Try again after fetching
		rate, err = s.db.CurrencyService.GetExchangeRate(fromCurrency, toCurrency)
		if err != nil {
			return 0, fmt.Errorf("exchange rate not found for %s to %s", fromCurrency, toCurrency)
		}
	}

	// Check if rate is too old (more than 1 hour)
	if time.Since(rate.FetchedAt) > time.Hour {
		log.Printf("Exchange rate for %s to %s is stale, fetching new rates...", fromCurrency, toCurrency)
		if err := s.FetchLatestRates(); err != nil {
			log.Printf("Failed to fetch fresh rates: %v", err)
		}
	}

	return rate.Rate, nil
}

// ConvertAmount converts an amount from one currency to another
func (s *ExchangeRateService) ConvertAmount(amount float64, fromCurrency, toCurrency string) (float64, error) {
	rate, err := s.GetExchangeRate(fromCurrency, toCurrency)
	if err != nil {
		return 0, err
	}

	return amount * rate, nil
}

// GetSupportedCurrencies returns all currencies we have rates for
func (s *ExchangeRateService) GetSupportedCurrencies() ([]string, error) {
	// First ensure we have fresh rates
	if err := s.FetchLatestRates(); err != nil {
		log.Printf("Failed to fetch latest rates: %v", err)
	}

	// Get all currencies from database
	currencies, err := s.db.CurrencyService.GetSupportedCurrencies()
	if err != nil {
		return nil, err
	}

	var codes []string
	for _, currency := range currencies {
		codes = append(codes, currency.Code)
	}

	return codes, nil
}

// StartPeriodicFetch starts a background goroutine to fetch rates periodically
func (s *ExchangeRateService) StartPeriodicFetch() {
	go func() {
		// Fetch immediately
		if err := s.FetchLatestRates(); err != nil {
			log.Printf("Initial exchange rate fetch failed: %v", err)
		}

		// Then fetch every 5 minutes
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			if err := s.FetchLatestRates(); err != nil {
				log.Printf("Periodic exchange rate fetch failed: %v", err)
			}
		}
	}()
}

// InitializeDefaultCurrencies adds default supported currencies to the database
func (s *ExchangeRateService) InitializeDefaultCurrencies() error {
	defaultCurrencies := []database.SupportedCurrency{
		{Code: "USD", Name: "US Dollar", Symbol: "$", IsActive: true},
		{Code: "EUR", Name: "Euro", Symbol: "€", IsActive: true},
		{Code: "GBP", Name: "British Pound", Symbol: "£", IsActive: true},
		{Code: "JPY", Name: "Japanese Yen", Symbol: "¥", IsActive: true},
		{Code: "AUD", Name: "Australian Dollar", Symbol: "A$", IsActive: true},
		{Code: "CAD", Name: "Canadian Dollar", Symbol: "C$", IsActive: true},
		{Code: "CHF", Name: "Swiss Franc", Symbol: "CHF", IsActive: true},
		{Code: "CNY", Name: "Chinese Yuan", Symbol: "¥", IsActive: true},
		{Code: "ARS", Name: "Argentine Peso", Symbol: "$", IsActive: true},
		{Code: "AED", Name: "UAE Dirham", Symbol: "د.إ", IsActive: true},
		{Code: "BRL", Name: "Brazilian Real", Symbol: "R$", IsActive: true},
		{Code: "MXN", Name: "Mexican Peso", Symbol: "$", IsActive: true},
		{Code: "INR", Name: "Indian Rupee", Symbol: "₹", IsActive: true},
		{Code: "KRW", Name: "South Korean Won", Symbol: "₩", IsActive: true},
		{Code: "SGD", Name: "Singapore Dollar", Symbol: "S$", IsActive: true},
		{Code: "HKD", Name: "Hong Kong Dollar", Symbol: "HK$", IsActive: true},
		{Code: "NOK", Name: "Norwegian Krone", Symbol: "kr", IsActive: true},
		{Code: "SEK", Name: "Swedish Krona", Symbol: "kr", IsActive: true},
		{Code: "DKK", Name: "Danish Krone", Symbol: "kr", IsActive: true},
		{Code: "PLN", Name: "Polish Zloty", Symbol: "zł", IsActive: true},
	}

	for _, currency := range defaultCurrencies {
		// Check if currency already exists
		var existing database.SupportedCurrency
		err := s.db.GetGorm().Where("code = ?", currency.Code).First(&existing).Error
		
		if err != nil {
			// Currency doesn't exist, create it
			if err := s.db.GetGorm().Create(&currency).Error; err != nil {
				log.Printf("Failed to create currency %s: %v", currency.Code, err)
			} else {
				log.Printf("Created currency: %s", currency.Code)
			}
		}
	}

	return nil
}
