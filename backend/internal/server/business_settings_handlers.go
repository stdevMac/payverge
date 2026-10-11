package server

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/demomode"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/reporting"
	"github.com/stdevmac/payverge/backend/internal/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PaymentHistoryItem struct {
	ID           uint    `json:"id"`
	BillID       uint    `json:"bill_id"`
	BillNumber   string  `json:"bill_number"`
	TableName    string  `json:"table_name"`
	PayerAddress string  `json:"payer_address"`
	Amount       float64 `json:"amount"`
	TipAmount    float64 `json:"tip_amount"`
	Currency     string  `json:"currency"`
	TxHash       string  `json:"tx_hash"`
	Status       string  `json:"status"`
	// Method is the canonical payment-method vocabulary value
	// (reporting.Method: crypto|cross_chain|card|cash|wallet|other).
	Method    string    `json:"method,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func parsePaymentHistoryDate(value string, location *time.Location) (time.Time, error) {
	parsed, err := time.ParseInLocation("2006-01-02", value, location)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date format %q, use YYYY-MM-DD", value)
	}

	return parsed, nil
}

// paymentHistoryMaxRangeDays bounds any explicit start/end range so a
// hand-edited URL can't pull thousands of days (mirrors the analytics
// timeseries clamp in handlers/analytics.go).
const paymentHistoryMaxRangeDays = 366

// clampPaymentHistoryRange caps [start,end) to at most paymentHistoryMaxRangeDays,
// keeping the newest end and walking start forward. Applied to explicit ranges
// only — the fixed presets are already bounded.
func clampPaymentHistoryRange(start, end time.Time) time.Time {
	if start.Before(end.AddDate(0, 0, -paymentHistoryMaxRangeDays)) {
		return end.AddDate(0, 0, -paymentHistoryMaxRangeDays)
	}
	return start
}

// resolvePaymentHistoryRange turns a period preset or an explicit start/end into
// a half-open [start,end) window. The window is interpreted in the business's
// own timezone (location) — not the server clock — so that a date the operator
// picked lines up with their local midnight boundaries. Explicit ranges are
// clamped to paymentHistoryMaxRangeDays.
func resolvePaymentHistoryRange(period, startDateStr, endDateStr string, now time.Time, location *time.Location) (time.Time, time.Time, error) {
	if location == nil {
		location = time.UTC
	}
	// Anchor "now" in the business timezone so preset windows and the open-ended
	// upper bound are computed against the business's local clock.
	now = now.In(location)

	if startDateStr != "" || endDateStr != "" {
		var (
			startDate time.Time
			endDate   time.Time
			err       error
		)

		if startDateStr != "" {
			startDate, err = parsePaymentHistoryDate(startDateStr, location)
			if err != nil {
				return time.Time{}, time.Time{}, err
			}
		}

		if endDateStr != "" {
			endDate, err = parsePaymentHistoryDate(endDateStr, location)
			if err != nil {
				return time.Time{}, time.Time{}, err
			}
			endDate = endDate.Add(24 * time.Hour)
		}

		switch {
		case startDateStr == "" && endDateStr != "":
			startDate = endDate.Add(-24 * time.Hour)
		case startDateStr != "" && endDateStr == "":
			endDate = now
		}

		if !endDate.After(startDate) {
			return time.Time{}, time.Time{}, fmt.Errorf("end date must be on or after start date")
		}

		startDate = clampPaymentHistoryRange(startDate, endDate)
		return startDate, endDate, nil
	}

	switch period {
	case "week":
		return now.AddDate(0, 0, -7), now, nil
	case "month":
		return now.AddDate(0, -1, 0), now, nil
	case "quarter":
		return now.AddDate(0, -3, 0), now, nil
	case "year":
		return now.AddDate(-1, 0, 0), now, nil
	case "yesterday":
		// L6-2: full local calendar day before today (matches analytics period).
		todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
		return todayStart.AddDate(0, 0, -1), todayStart, nil
	case "", "today":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
		return start, now, nil
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("unsupported period: %s", period)
	}
}

// paymentHistoryMaxPageSize caps the server-side page so a single request never
// returns more than this many rows. 100 matches the other dashboard list caps.
const paymentHistoryMaxPageSize = 100

// PaymentHistoryFilter carries the optional server-side search/filter/pagination
// controls. The zero value (Page == 0) is the legacy "return every row in the
// window" behavior so existing callers and the CSV export stay unchanged.
type PaymentHistoryFilter struct {
	// Search matches bill number, table name, method, external_ref (tx hash /
	// card auth), or payer_ref — escaped LIKE (same discipline as bills search).
	Search string
	// Method is a canonical reporting.Method key (crypto|cross_chain|card|cash|
	// wallet|other). Expanded to every known raw alias so stripe/card rows both
	// match method=card. Non-canonical keys (stripe, manual, Online) match zero
	// rows — the FE only offers keys from available_methods.
	Method string
	// Status filters on payment_events.status (e.g. "confirmed", "pending").
	Status string
	// Page is 1-based. Page == 0 disables pagination (legacy shape).
	Page int
	// PageSize is clamped to [1, paymentHistoryMaxPageSize]; ignored when Page == 0.
	PageSize int
}

// paymentEventsMethodNorm is the SQL expression that normalizes
// payment_events.method the same way reporting.normalizeRaw does (lower +
// hyphen→underscore) so filter aliases match both "cross-chain" and
// "cross_chain".
const paymentEventsMethodNorm = "LOWER(REPLACE(TRIM(COALESCE(payment_events.method, '')), '-', '_'))"

// applyCanonicalMethodFilter expands a canonical Method into a SQL predicate
// over payment_events.method. Non-canonical keys force an empty result so a
// stale "stripe"/"manual" filter cannot silently match nothing useful while
// looking like a real filter.
func applyCanonicalMethodFilter(base *gorm.DB, method string) *gorm.DB {
	m, ok := reporting.ParseMethod(method)
	if !ok {
		return base.Where("1 = 0")
	}
	include, excludeOther := reporting.FilterMatchArgs(m)
	// Normalize aliases the same way as the SQL expression (drop empty; empty
	// is matched separately for crypto's historical default).
	aliases := make([]string, 0, len(include))
	matchEmpty := false
	for _, a := range include {
		n := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(a, "-", "_")))
		if n == "" {
			matchEmpty = true
			continue
		}
		aliases = append(aliases, n)
	}

	if excludeOther {
		// other = known-other aliases OR anything not belonging to the other five.
		nonOther := make([]string, 0, 16)
		for _, a := range reporting.NonOtherAliases() {
			n := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(a, "-", "_")))
			if n == "" {
				continue
			}
			nonOther = append(nonOther, n)
		}
		if len(aliases) == 0 {
			return base.Where(paymentEventsMethodNorm+" NOT IN ?", nonOther)
		}
		return base.Where(
			"("+paymentEventsMethodNorm+" IN ? OR ("+paymentEventsMethodNorm+" NOT IN ? AND TRIM(COALESCE(payment_events.method, '')) <> ''))",
			aliases, nonOther,
		)
	}

	if len(aliases) == 0 && matchEmpty {
		return base.Where("TRIM(COALESCE(payment_events.method, '')) = ''")
	}
	if matchEmpty {
		return base.Where(
			"("+paymentEventsMethodNorm+" IN ? OR TRIM(COALESCE(payment_events.method, '')) = '')",
			aliases,
		)
	}
	return base.Where(paymentEventsMethodNorm+" IN ?", aliases)
}

// loadPaymentHistoryAvailableMethods returns the canonical Methods that have at
// least one row in the business+window. Order follows reporting.AllMethods.
// The FE builds the filter dropdown from this list only.
func loadPaymentHistoryAvailableMethods(businessID uint, startDate, endDate time.Time) ([]string, error) {
	var raws []string
	err := database.PaymentEventsQuery(nil).
		Where("payment_events.business_id = ? AND payment_events.created_at >= ? AND payment_events.created_at < ?", businessID, startDate, endDate).
		Distinct("payment_events.method").
		Pluck("payment_events.method", &raws).Error
	if err != nil {
		return nil, err
	}
	// source_table is mixed in the view; Canonicalize only uses it for the
	// empty-string default, and empty raws on either table still land in the set.
	methods := reporting.DistinctMethods("", raws)
	out := make([]string, len(methods))
	for i, m := range methods {
		out[i] = string(m)
	}
	return out, nil
}

// escapePaymentHistoryLike escapes LIKE wildcards so an operator's search string
// is matched literally (mirrors the private escapeSQLLike in the database pkg,
// which isn't exported).
func escapePaymentHistoryLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return value
}

// loadPaymentHistoryItemsFiltered runs the projected payment-history join with
// optional server-side search/method/status filters and pagination. It returns
// the page of items plus the total row count matching the window+filters (so the
// client can render an honest "showing X of N"). When filter.Page == 0 the full
// filtered window is returned (legacy behavior) and total == len(items).
//
// Reads from the payment_events view so crypto rows in
// `payments` and cash/card rows in `alternative_payments` share one ledger.
func loadPaymentHistoryItemsFiltered(businessID uint, startDate, endDate time.Time, filter PaymentHistoryFilter) ([]PaymentHistoryItem, int64, error) {
	type paymentHistoryRow struct {
		ID           uint
		BillID       uint
		BillNumber   string
		TableName    string
		PayerAddress string
		Amount       int64
		TipAmount    int64
		Currency     string
		TxHash       string
		Status       string
		RawMethod    string
		SourceTable  string
		CreatedAt    time.Time
		UpdatedAt    time.Time
	}

	base := database.PaymentEventsQuery(nil).
		Joins("JOIN bills ON payment_events.bill_id = bills.id").
		Joins("JOIN businesses ON businesses.id = payment_events.business_id").
		Joins("LEFT JOIN tables ON bills.table_id = tables.id").
		Where("payment_events.business_id = ? AND payment_events.created_at >= ? AND payment_events.created_at < ?", businessID, startDate, endDate)

	if method := strings.TrimSpace(filter.Method); method != "" {
		base = applyCanonicalMethodFilter(base, method)
	}
	if status := strings.TrimSpace(filter.Status); status != "" {
		base = base.Where("payment_events.status = ?", status)
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		like := "%" + escapePaymentHistoryLike(strings.ToLower(strings.TrimPrefix(search, "#"))) + "%"
		base = base.Where(
			"(LOWER(COALESCE(bills.bill_number, '')) LIKE ? ESCAPE '\\' OR "+
				"LOWER(COALESCE(tables.name, '')) LIKE ? ESCAPE '\\' OR "+
				"LOWER(COALESCE(payment_events.method, '')) LIKE ? ESCAPE '\\' OR "+
				"LOWER(COALESCE(payment_events.external_ref, '')) LIKE ? ESCAPE '\\' OR "+
				"LOWER(COALESCE(payment_events.payer_ref, '')) LIKE ? ESCAPE '\\')",
			like, like, like, like, like,
		)
	}

	var total int64
	if filter.Page > 0 {
		// Count only when paginating — the legacy full-window path derives the
		// total from len(items) and doesn't need a second query.
		if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			return nil, 0, err
		}
	}

	query := base.Session(&gorm.Session{}).
		Select(`
			payment_events.id,
			payment_events.bill_id,
			bills.bill_number,
			COALESCE(tables.name, 'Unknown') AS table_name,
			payment_events.payer_ref AS payer_address,
			payment_events.amount_cents AS amount,
			payment_events.tip_cents AS tip_amount,
			COALESCE(NULLIF(businesses.display_currency, ''), NULLIF(businesses.default_currency, ''), 'USD') AS currency,
			payment_events.external_ref AS tx_hash,
			payment_events.status,
			payment_events.method AS raw_method,
			payment_events.source_table AS source_table,
			payment_events.created_at,
			payment_events.updated_at
		`).
		// Newest first — without an explicit order the list surfaces in
		// insertion order, which reads as randomly sorted on the dashboard.
		Order("payment_events.created_at DESC, payment_events.id DESC")

	if filter.Page > 0 {
		pageSize := filter.PageSize
		if pageSize <= 0 {
			pageSize = paymentHistoryMaxPageSize
		}
		if pageSize > paymentHistoryMaxPageSize {
			pageSize = paymentHistoryMaxPageSize
		}
		query = query.Limit(pageSize).Offset((filter.Page - 1) * pageSize)
	}

	var rows []paymentHistoryRow
	if err := query.Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	paymentHistory := make([]PaymentHistoryItem, 0, len(rows))
	for _, row := range rows {
		paymentHistory = append(paymentHistory, PaymentHistoryItem{
			ID:           row.ID,
			BillID:       row.BillID,
			BillNumber:   row.BillNumber,
			TableName:    row.TableName,
			PayerAddress: row.PayerAddress,
			Amount:       float64(row.Amount) / 100.0,
			TipAmount:    float64(row.TipAmount) / 100.0,
			Currency:     row.Currency,
			TxHash:       row.TxHash,
			Status:       row.Status,
			Method:       string(reporting.Canonicalize(row.SourceTable, row.RawMethod)),
			CreatedAt:    row.CreatedAt,
			UpdatedAt:    row.UpdatedAt,
		})
	}

	if filter.Page == 0 {
		total = int64(len(paymentHistory))
	}

	return paymentHistory, total, nil
}

func exportPaymentHistoryCSV(items []PaymentHistoryItem, lang string) ([]byte, error) {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)

	headers := []string{
		"Date",
		"Bill Number",
		"Table",
		"Amount",
		"Tip Amount",
		"Currency",
		"Status",
		"Payer Address",
		"Transaction Hash",
	}
	if utils.NormalizeOperatorLang(lang) == "es" {
		headers = []string{
			"Fecha",
			"Número de cuenta",
			"Mesa",
			"Monto",
			"Propina",
			"Moneda",
			"Estado",
			"Dirección del pagador",
			"Hash de transacción",
		}
	}
	if err := writer.Write(headers); err != nil {
		return nil, err
	}

	for _, item := range items {
		// TableName (staff/owner-set) and PayerAddress (free-form on some payment
		// types) are the free-text fields here; sanitize against CSV formula
		// injection (CWE-1236) so a crafted value can't run as a formula in the
		// owner's spreadsheet. Generated/enum/hex fields are left as-is.
		if err := writer.Write([]string{
			item.CreatedAt.Format(time.RFC3339),
			item.BillNumber,
			utils.SanitizeCSVField(item.TableName),
			fmt.Sprintf("%.2f", item.Amount),
			fmt.Sprintf("%.2f", item.TipAmount),
			item.Currency,
			item.Status,
			utils.SanitizeCSVField(item.PayerAddress),
			item.TxHash,
		}); err != nil {
			return nil, err
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

// errCustomURLLookupFailed is returned when the availability query itself
// fails. Callers must map it to HTTP 500 and must not treat the slug as free.
var errCustomURLLookupFailed = errors.New("failed to check custom URL availability")

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

	if database.IsReservedPublicSlug(customURL) {
		return errors.New("this custom URL is reserved")
	}

	// Check if URL is already taken by another business — case-insensitive,
	// matching the lower(custom_url) unique index and the public
	// lookup, closing the MiCafe/micafe squat.
	var existingBusiness database.Business
	result := database.GetDB().
		Select("id").
		Where("custom_url <> '' AND LOWER(custom_url) = LOWER(?) AND id != ?", customURL, excludeBusinessID).
		Take(&existingBusiness)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil
	}
	if result.Error == nil {
		return errors.New("this custom URL is already taken")
	}
	log.Printf("custom URL availability lookup failed: %v", result.Error)
	return errCustomURLLookupFailed
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
		if errors.Is(err, errCustomURLLookupFailed) {
			RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Failed to check custom URL availability")
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"available": false,
			"error":     err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"available": true,
		"url":       customURL,
	})
}

// Counter management handlers

// UpdateCounterSettingsRequest represents the request to update counter settings.
// counter_prefix is NOT binding-max validated here: max must apply after
// TrimSpace (L2-34) so "  BAR  " normalizes to "BAR" rather than 400 on len=7.
type UpdateCounterSettingsRequest struct {
	CounterEnabled bool   `json:"counter_enabled"`
	CounterCount   int    `json:"counter_count" binding:"min=1,max=20"`
	CounterPrefix  string `json:"counter_prefix"`
}

// UpdateCounterSettings updates counter configuration for a business
func UpdateCounterSettings(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	var req UpdateCounterSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	// L2-34: trim first, then non-empty + max=5 (shared pure helper).
	prefix, err := normalizeAndValidateCounterPrefix(req.CounterPrefix)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.CounterPrefix = prefix

	// Update counter settings
	if err := database.UpdateBusinessCounters(business.ID, req.CounterEnabled, req.CounterCount, req.CounterPrefix); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update counter settings"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Counter settings updated successfully"})
}

// GetBusinessCounters retrieves all counters for a business
func GetBusinessCounters(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	// Get counters
	counters, err := database.GetBusinessCounters(business.ID)
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
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	// Check if counters are enabled
	if !business.CounterEnabled {
		c.JSON(http.StatusOK, gin.H{"counters": []database.Counter{}})
		return
	}

	// Get available counters
	counters, err := database.GetAvailableCounters(business.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get available counters"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"counters": counters})
}

// Google Places API integration

// GoogleBusinessSearchRequest represents a request to search for Google businesses
type GoogleBusinessSearchRequest struct {
	Query string `json:"query" binding:"required"`
}

// GoogleBusinessUpdateRequest represents a request to update Google business info
type GoogleBusinessUpdateRequest struct {
	PlaceID      string `json:"place_id" binding:"required"`
	BusinessName string `json:"business_name" binding:"required"`
}

// SearchGoogleBusinesses searches for businesses using Google Places API
func SearchGoogleBusinesses(c *gin.Context) {
	var req GoogleBusinessSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	// Get Google Places service
	placesService := GetGooglePlacesService()
	if placesService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Google Places API not configured"})
		return
	}

	// Search for businesses
	results, err := placesService.SearchBusinesses(req.Query)
	if err != nil {
		log.Printf("Error searching Google businesses: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to search businesses"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"results": results,
		"count":   len(results),
	})
}

// UpdateBusinessGoogleInfo updates the Google business information for a business
func UpdateBusinessGoogleInfo(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	var req GoogleBusinessUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	// Get Google Places service and validate Place ID
	placesService := GetGooglePlacesService()
	if placesService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Google Places API not configured"})
		return
	}

	isValid, err := placesService.ValidatePlaceID(req.PlaceID)
	if err != nil {
		log.Printf("Error validating Place ID: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to validate Google business"})
		return
	}

	if !isValid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Google Place ID"})
		return
	}

	// Generate review link and business URL
	reviewLink := placesService.GenerateReviewLink(req.PlaceID)
	businessURL := placesService.GenerateBusinessURL(req.PlaceID)

	// Update business with Google information
	business.GooglePlaceID = req.PlaceID
	business.GoogleBusinessName = req.BusinessName
	business.GoogleReviewLink = reviewLink
	business.GoogleBusinessURL = businessURL
	business.GoogleReviewsEnabled = true
	business.UpdatedAt = time.Now()

	if err := database.UpdateBusiness(business); err != nil {
		log.Printf("Error updating business Google info: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update business"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":              "Google business information updated successfully",
		"google_place_id":      business.GooglePlaceID,
		"google_business_name": business.GoogleBusinessName,
		"google_review_link":   business.GoogleReviewLink,
		"google_business_url":  business.GoogleBusinessURL,
	})
}

// RemoveBusinessGoogleInfo removes Google business integration from a business
func RemoveBusinessGoogleInfo(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	// Clear Google business information
	business.GooglePlaceID = ""
	business.GoogleBusinessName = ""
	business.GoogleReviewLink = ""
	business.GoogleBusinessURL = ""
	business.GoogleReviewsEnabled = false
	business.UpdatedAt = time.Now()

	if err := database.UpdateBusiness(business); err != nil {
		log.Printf("Error removing business Google info: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update business"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Google business integration removed successfully",
	})
}

// GetPublicBusinessGoogleReviews fetches Google reviews for a business by custom URL (public endpoint)
func GetPublicBusinessGoogleReviews(c *gin.Context) {
	customUrl := c.Param("customUrl")
	if customUrl == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Custom URL is required"})
		return
	}

	// Get the public business by custom URL
	business, err := loadPublicBusinessByCustomURL(customUrl)
	if err != nil {
		respondPublicBusinessLookupError(c, err)
		return
	}

	// Check if Google reviews are enabled and place ID exists
	if !business.GoogleReviewsEnabled || business.GooglePlaceID == "" {
		c.JSON(http.StatusOK, gin.H{
			"reviews": []interface{}{},
			"count":   0,
			"message": "Google reviews not enabled for this business",
		})
		return
	}

	// Get Google Places service (interface — swappable for tests / nil when
	// the API key is not configured at startup).
	placesService := googlePlacesService
	if placesService == nil {
		c.JSON(http.StatusOK, gin.H{
			"reviews":             []interface{}{},
			"count":               0,
			"google_place_id":     business.GooglePlaceID,
			"business_name":       business.GoogleBusinessName,
			"google_business_url": business.GoogleBusinessURL,
			"google_review_link":  business.GoogleReviewLink,
			"error_code":          "places_not_configured",
		})
		return
	}

	// Get language parameter from query string (optional)
	language := c.Query("language")

	// Fetch reviews from Google Places API with language parameter
	reviews, err := placesService.GetPlaceReviews(business.GooglePlaceID, language)
	if err != nil {
		log.Printf("[google-reviews] fail-soft for %s: %v", customUrl, err)
		c.JSON(http.StatusOK, gin.H{
			"reviews":             []interface{}{},
			"count":               0,
			"google_place_id":     business.GooglePlaceID,
			"business_name":       business.GoogleBusinessName,
			"google_business_url": business.GoogleBusinessURL,
			"google_review_link":  business.GoogleReviewLink,
			"error_code":          "upstream_unavailable",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"reviews":             reviews,
		"count":               len(reviews),
		"google_place_id":     business.GooglePlaceID,
		"business_name":       business.GoogleBusinessName,
		"google_business_url": business.GoogleBusinessURL,
		"google_review_link":  business.GoogleReviewLink,
	})
}

// GetPublicBusinessGoogleDetails fetches detailed Google business information including reviews by custom URL (public endpoint)
func GetPublicBusinessGoogleDetails(c *gin.Context) {
	customUrl := c.Param("customUrl")
	if customUrl == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Custom URL is required"})
		return
	}

	// Get the public business by custom URL
	business, err := loadPublicBusinessByCustomURL(customUrl)
	if err != nil {
		respondPublicBusinessLookupError(c, err)
		return
	}

	// Check if Google reviews are enabled and place ID exists
	if !business.GoogleReviewsEnabled || business.GooglePlaceID == "" {
		c.JSON(http.StatusOK, gin.H{
			"place_details": nil,
			"message":       "Google integration not enabled for this business",
		})
		return
	}

	// Get Google Places service (interface — swappable for tests / nil when
	// the API key is not configured at startup).
	placesService := googlePlacesService
	if placesService == nil {
		c.JSON(http.StatusOK, gin.H{
			"place_details":       nil,
			"google_place_id":     business.GooglePlaceID,
			"business_name":       business.GoogleBusinessName,
			"google_business_url": business.GoogleBusinessURL,
			"google_review_link":  business.GoogleReviewLink,
			"error_code":          "places_not_configured",
		})
		return
	}

	// Fetch place details from Google Places API
	placeDetails, err := placesService.GetPlaceDetails(business.GooglePlaceID)
	if err != nil {
		log.Printf("[google-details] fail-soft for %s: %v", customUrl, err)
		c.JSON(http.StatusOK, gin.H{
			"place_details":       nil,
			"google_place_id":     business.GooglePlaceID,
			"business_name":       business.GoogleBusinessName,
			"google_business_url": business.GoogleBusinessURL,
			"google_review_link":  business.GoogleReviewLink,
			"error_code":          "upstream_unavailable",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"place_details":       placeDetails,
		"google_place_id":     business.GooglePlaceID,
		"business_name":       business.GoogleBusinessName,
		"google_business_url": business.GoogleBusinessURL,
		"google_review_link":  business.GoogleReviewLink,
	})
}

// GetPaymentHistory gets payment history for a business
func GetPaymentHistory(c *gin.Context) {
	business, ok := fetchBusinessByParam(c, "id")
	if !ok {
		return
	}
	businessID := business.ID

	if !CheckBusinessAccess(c, business) {
		RespondWithError(c, http.StatusForbidden, ErrCodeForbidden, "Access denied")
		return
	}

	period := c.DefaultQuery("period", "month")
	startDate, endDate, err := resolvePaymentHistoryRange(
		period,
		c.Query("start_date"),
		c.Query("end_date"),
		time.Now(),
		database.ResolveBusinessLocation(business),
	)
	if err != nil {
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, err.Error())
		return
	}

	filter := PaymentHistoryFilter{
		Search: c.Query("q"),
		Method: c.Query("method"),
		Status: c.Query("status"),
	}

	// Pagination is opt-in. When ?page is absent we return the legacy bare
	// array so existing clients (and any BE-first deploy window) keep working.
	pageStr := c.Query("page")
	paginated := pageStr != ""
	if paginated {
		page, perr := strconv.Atoi(pageStr)
		if perr != nil || page < 1 {
			page = 1
		}
		pageSize := paymentHistoryMaxPageSize
		if ps, psErr := strconv.Atoi(c.Query("page_size")); psErr == nil && ps > 0 {
			pageSize = ps
		}
		if pageSize > paymentHistoryMaxPageSize {
			pageSize = paymentHistoryMaxPageSize
		}
		filter.Page = page
		filter.PageSize = pageSize
	}

	paymentHistory, total, err := loadPaymentHistoryItemsFiltered(uint(businessID), startDate, endDate, filter)
	if err != nil {
		log.Printf("Error getting payment history: %v", err)
		RespondWithError(c, http.StatusInternalServerError, "", "Failed to get payment history")
		return
	}

	// Available methods for the business+window (unfiltered by method/status/q)
	// so the FE can only offer filter keys that match ≥1 row.
	availableMethods, availErr := loadPaymentHistoryAvailableMethods(uint(businessID), startDate, endDate)
	if availErr != nil {
		log.Printf("Error loading payment history available methods: %v", availErr)
		availableMethods = []string{}
	}

	if !paginated {
		// Legacy shape: bare array of items. available_methods is only on the
		// paginated envelope the operator UI consumes.
		c.JSON(http.StatusOK, paymentHistory)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items":             paymentHistory,
		"total":             total,
		"page":              filter.Page,
		"page_size":         filter.PageSize,
		"available_methods": availableMethods,
	})
}

// ExportPaymentHistory exports payment history for a business.
func ExportPaymentHistory(c *gin.Context) {
	business, ok := fetchBusinessByParam(c, "id")
	if !ok {
		return
	}
	businessID := business.ID

	if !CheckBusinessAccess(c, business) {
		RespondWithError(c, http.StatusForbidden, ErrCodeForbidden, "Access denied")
		return
	}

	period := c.DefaultQuery("period", "month")
	startDate, endDate, err := resolvePaymentHistoryRange(
		period,
		c.Query("start_date"),
		c.Query("end_date"),
		time.Now(),
		database.ResolveBusinessLocation(business),
	)
	if err != nil {
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, err.Error())
		return
	}

	// Export honors the same search/method/status filters as the list so the two
	// stay in sync (the audit's "rows in CSV but not the table" bug). The range
	// clamp above bounds the export payload identically to the list.
	items, _, err := loadPaymentHistoryItemsFiltered(uint(businessID), startDate, endDate, PaymentHistoryFilter{
		Search: c.Query("q"),
		Method: c.Query("method"),
		Status: c.Query("status"),
	})
	if err != nil {
		log.Printf("Error exporting payment history: %v", err)
		RespondWithError(c, http.StatusInternalServerError, "", "Failed to export payment history")
		return
	}

	format := c.DefaultQuery("format", "csv")
	lang := c.DefaultQuery("lang", "en")

	var (
		content     []byte
		contentType string
	)

	switch format {
	case "csv":
		content, err = exportPaymentHistoryCSV(items, lang)
		contentType = "text/csv"
	case "json":
		content, err = json.Marshal(items)
		contentType = "application/json"
	default:
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "Unsupported format")
		return
	}

	if err != nil {
		log.Printf("Error encoding payment history export: %v", err)
		RespondWithError(c, http.StatusInternalServerError, "", "Failed to export payment history")
		return
	}

	filenameScope := period
	if c.Query("start_date") != "" || c.Query("end_date") != "" {
		filenameScope = "custom"
	}

	filename := fmt.Sprintf("payment_history_%s.%s", filenameScope, format)
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Data(http.StatusOK, contentType, content)
}

// UpdateBusinessDesignSettings updates business design customization settings
func UpdateBusinessDesignSettings(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	var updateData database.BusinessDesignSettings
	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	applyDesignSettingsDefaults(&updateData)
	if err := validateBusinessDesignSettings(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update design settings
	business.DesignSettings = updateData
	if err := database.UpdateBusinessDesignOnly(business); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update design settings"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Design settings updated successfully"})
}

// UpdateBusinessHospitalitySettings updates business hospitality features
func UpdateBusinessHospitalitySettings(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	var updateData struct {
		WelcomeMessage      string `json:"welcome_message"`
		AboutStory          string `json:"about_story"`
		ShowWelcomeMessage  bool   `json:"show_welcome_message"`
		ShowAboutStory      bool   `json:"show_about_story"`
		ShowGallery         bool   `json:"show_gallery"`
		ShowOperatingHours  bool   `json:"show_operating_hours"`
		ShowSpecialFeatures bool   `json:"show_special_features"`
	}

	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}
	if config.DemoModeEnabled() && demoHospitalityTextRefusal(updateData.WelcomeMessage, updateData.AboutStory) {
		demomode.Refuse(c, demomode.KindStorefront, demoStorefrontTextAction)
		return
	}

	// Apply updates to business struct
	business.WelcomeMessage = updateData.WelcomeMessage
	business.AboutStory = updateData.AboutStory
	business.ShowWelcomeMessage = updateData.ShowWelcomeMessage
	business.ShowAboutStory = updateData.ShowAboutStory
	business.ShowGallery = updateData.ShowGallery
	business.ShowOperatingHours = updateData.ShowOperatingHours
	business.ShowSpecialFeatures = updateData.ShowSpecialFeatures

	if err := database.UpdateBusiness(business); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update hospitality settings"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Hospitality settings updated successfully"})
}

// GetBusinessGalleryImages retrieves gallery images for a business.
// Optional ?limit= (clamped 1..100) bounds the list; omit for legacy full set.
func GetBusinessGalleryImages(c *gin.Context) {
	business, ok := fetchBusinessByParam(c, "id")
	if !ok {
		return
	}

	var images []database.BusinessGalleryImage
	var err error
	if limStr := c.Query("limit"); limStr != "" {
		lim, _ := strconv.Atoi(limStr)
		images, err = database.GetBusinessGalleryImages(business.ID, lim)
	} else {
		images, err = database.GetBusinessGalleryImages(business.ID)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve gallery images"})
		return
	}

	c.JSON(http.StatusOK, images)
}

// UpdateBusinessGalleryImages updates gallery images for a business
func UpdateBusinessGalleryImages(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	var images []database.BusinessGalleryImage
	if err := c.ShouldBindJSON(&images); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	// Upsert path preserves row IDs for unchanged images so ID-keyed caption
	// translations stay valid. Skip the translation fan-out when no caption
	// text changed (typo-free reorder / enable flips must not re-translate
	// into up to 21 locales).
	result, err := database.UpdateBusinessGalleryImages(business.ID, images)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update gallery images"})
		return
	}

	if ts := GetTranslationService(); ts != nil && ts.IsEnabled() {
		businessID := business.ID
		deletedIDs := append([]uint(nil), result.DeletedIDs...)
		captionsChanged := result.CaptionsChanged
		logger.SafeGo(func() {
			// Always drop translations for deleted rows (orphan cleanup).
			if len(deletedIDs) > 0 {
				if err := deleteGalleryImageTranslationsForIDs(businessID, deletedIDs); err != nil {
					log.Printf("Failed clearing deleted gallery caption translations for business %d: %v", businessID, err)
				}
			}
			if !captionsChanged {
				return
			}
			// Captions changed on preserved or new rows: re-translate the live set.
			// Prefer scoped clear of all gallery translations for the business so
			// stale text for edited captions cannot linger under the same ID.
			if err := deleteGalleryImageTranslations(businessID); err != nil {
				log.Printf("Failed clearing stale gallery caption translations for business %d: %v", businessID, err)
			}
			if languageCodes := businessGuestLanguageCodes(businessID); len(languageCodes) > 0 {
				if err := translateBusinessContentIntoLanguages(businessID, languageCodes); err != nil {
					log.Printf("Background gallery caption translation failed for business %d: %v", businessID, err)
				}
			}
		})
	}

	c.JSON(http.StatusOK, gin.H{"message": "Gallery images updated successfully"})
}

// GetBusinessOperatingHours retrieves operating hours for a business
func GetBusinessOperatingHours(c *gin.Context) {
	resolved, ok := fetchBusinessByParam(c, "id")
	if !ok {
		return
	}
	businessID := uint64(resolved.ID)

	hours, err := database.GetBusinessOperatingHours(uint(businessID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve operating hours"})
		return
	}

	c.JSON(http.StatusOK, hours)
}

// UpdateBusinessOperatingHours updates operating hours for a business
func UpdateBusinessOperatingHours(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	var hours []database.BusinessOperatingHours
	if err := c.ShouldBindJSON(&hours); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}
	if err := database.ValidateBusinessOperatingHours(hours); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update operating hours using database function
	if err := database.UpdateBusinessOperatingHours(business.ID, hours); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update operating hours"})
		return
	}
	invalidateReservationAvailability(business.ID)

	c.JSON(http.StatusOK, gin.H{"message": "Operating hours updated successfully"})
}

// GetBusinessOperatingExceptions retrieves holiday / one-off closures.
func GetBusinessOperatingExceptions(c *gin.Context) {
	resolved, ok := fetchBusinessByParam(c, "id")
	if !ok {
		return
	}
	rows, err := database.GetBusinessOperatingExceptions(resolved.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve operating exceptions"})
		return
	}
	c.JSON(http.StatusOK, rows)
}

// UpdateBusinessOperatingExceptions replaces holiday / one-off closures.
func UpdateBusinessOperatingExceptions(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}
	var rows []database.BusinessOperatingException
	if err := c.ShouldBindJSON(&rows); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}
	if err := database.ValidateBusinessOperatingExceptions(rows); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := database.UpdateBusinessOperatingExceptions(business.ID, rows); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update operating exceptions"})
		return
	}
	invalidateReservationAvailability(business.ID)
	c.JSON(http.StatusOK, gin.H{"message": "Operating exceptions updated successfully"})
}

// GetBusinessSpecialFeatures retrieves special features for a business
func GetBusinessSpecialFeatures(c *gin.Context) {
	resolved, ok := fetchBusinessByParam(c, "id")
	if !ok {
		return
	}
	businessID := uint64(resolved.ID)

	features, err := database.GetBusinessSpecialFeatures(uint(businessID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve special features"})
		return
	}

	c.JSON(http.StatusOK, features)
}

// UpdateBusinessSpecialFeatures updates special features for a business
func UpdateBusinessSpecialFeatures(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	var features []database.BusinessSpecialFeature
	if err := c.ShouldBindJSON(&features); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}
	if config.DemoModeEnabled() && demoSpecialFeaturesTextRefusal(features) {
		demomode.Refuse(c, demomode.KindStorefront, demoStorefrontTextAction)
		return
	}

	// Update special features using database function
	if err := database.UpdateBusinessSpecialFeatures(business.ID, features); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update special features"})
		return
	}

	// UpdateBusinessSpecialFeatures replaces rows wholesale, so the new features
	// carry fresh IDs and any prior ID-keyed translations are orphaned. Clear
	// them and re-translate the new set into the business's guest languages so
	// the storefront "Why choose us" section tracks the selected language.
	if ts := GetTranslationService(); ts != nil && ts.IsEnabled() {
		businessID := business.ID
		logger.SafeGo(func() {
			if err := deleteSpecialFeatureTranslations(businessID); err != nil {
				log.Printf("Failed clearing stale special-feature translations for business %d: %v", businessID, err)
			}
			if languageCodes := businessGuestLanguageCodes(businessID); len(languageCodes) > 0 {
				if err := translateBusinessContentIntoLanguages(businessID, languageCodes); err != nil {
					log.Printf("Background special-feature translation failed for business %d: %v", businessID, err)
				}
			}
		})
	}

	c.JSON(http.StatusOK, gin.H{"message": "Special features updated successfully"})
}

// businessGuestLanguageCodes returns the business's configured guest language
// codes (including the default; translation helpers skip the default themselves).
func businessGuestLanguageCodes(businessID uint) []string {
	dbw := database.GetDBWrapper()
	if dbw == nil {
		return nil
	}
	businessLanguages, err := dbw.LanguageService.GetBusinessLanguages(businessID)
	if err != nil {
		log.Printf("Failed loading guest languages for business %d: %v", businessID, err)
		return nil
	}
	codes := make([]string, 0, len(businessLanguages))
	for _, bl := range businessLanguages {
		if code := strings.TrimSpace(bl.LanguageCode); code != "" {
			codes = append(codes, code)
		}
	}
	return codes
}

// ToggleKitchenAndOrdersRequest represents the request to toggle kitchen/orders features
type ToggleKitchenAndOrdersRequest struct {
	Enabled bool `json:"enabled"`
}

// ToggleKitchenAndOrders enables or disables kitchen and orders features for a business
func ToggleKitchenAndOrders(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	var req ToggleKitchenAndOrdersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	// A suspended or closed business cannot re-enable guest ordering; any
	// error surfaced after this check is an internal failure.
	if RespondIfBusinessLocked(c, business) {
		return
	}
	if err := database.ToggleKitchenAndOrders(business.ID, req.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to toggle kitchen and orders"})
		return
	}

	// Get updated business data
	updatedBusiness, err := database.GetBusinessByID(business.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve updated business"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "Kitchen and orders features updated successfully",
		"kitchen_enabled": updatedBusiness.KitchenEnabled,
		"orders_enabled":  updatedBusiness.OrdersEnabled,
	})
}

// GetKitchenOrdersStatus returns the current status of kitchen and orders features
func GetKitchenOrdersStatus(c *gin.Context) {
	businessIdentifier := c.Param("id")
	business, err := database.GetBusinessByIdOrBusinessId(businessIdentifier)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve business"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"kitchen_enabled": business.KitchenEnabled,
		"orders_enabled":  business.OrdersEnabled,
	})
}

// GetBusinessByCustomURL handles public business page requests by custom URL
func GetBusinessByCustomURL(c *gin.Context) {
	customURL := c.Param("customUrl")
	if customURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Custom URL is required"})
		return
	}

	// Get public business by custom URL
	business, err := loadPublicBusinessByCustomURL(customURL)
	if err != nil {
		log.Printf("Error getting public business by custom URL '%s': %v", customURL, err)
		respondPublicBusinessLookupError(c, err)
		return
	}

	galleryImages, err := database.GetPublicBusinessGalleryImages(business.ID)
	if err != nil {
		log.Printf("Warning: Could not load gallery images for business %d: %v", business.ID, err)
		galleryImages = []database.BusinessGalleryImage{}
	}

	operatingHours, err := database.GetPublicBusinessOperatingHours(business.ID)
	if err != nil {
		log.Printf("Warning: Could not load operating hours for business %d: %v", business.ID, err)
		operatingHours = []database.BusinessOperatingHours{}
	}

	operatingExceptions, err := database.GetPublicBusinessOperatingExceptions(business.ID)
	if err != nil {
		log.Printf("Warning: Could not load operating exceptions for business %d: %v", business.ID, err)
		operatingExceptions = []database.BusinessOperatingException{}
	}

	specialFeatures, err := database.GetPublicBusinessSpecialFeatures(business.ID)
	if err != nil {
		log.Printf("Warning: Could not load special features for business %d: %v", business.ID, err)
		specialFeatures = []database.BusinessSpecialFeature{}
	}

	businessLanguages, err := database.GetPublicBusinessLanguages(business.ID)
	if err != nil {
		log.Printf("Warning: Could not load business languages for business %d: %v", business.ID, err)
		businessLanguages = []database.BusinessLanguage{}
	}

	supportedLanguages, err := database.GetCachedPublicSupportedLanguages()
	if err != nil {
		log.Printf("Warning: Could not load supported languages: %v", err)
		supportedLanguages = []database.PublicSupportedLanguage{}
	}

	// Translate operator prose (hero/about copy) and special features into the
	// requested guest language. Like the menu, untranslated content self-heals:
	// a miss schedules a background backfill so the next load is translated.
	defaultLanguage := business.DefaultLanguage
	if defaultLanguage == "" {
		defaultLanguage = "en"
	}
	if languageCode := strings.TrimSpace(c.Query("language")); languageCode != "" && languageCode != defaultLanguage {
		if applyBusinessContentTranslations(business, specialFeatures, galleryImages, languageCode) {
			scheduleBusinessContentTranslationBackfill(business.ID, languageCode)
		}
	}

	// Return business data (excluding sensitive information). Base fields come from
	// the shared projection so this storefront response and GetBusiness cannot drift.
	publicBusiness := publicBusinessProjection(business)
	publicBusiness["gallery_images"] = galleryImages
	publicBusiness["operating_hours"] = operatingHours
	publicBusiness["operating_exceptions"] = operatingExceptions
	publicBusiness["special_features"] = specialFeatures
	publicBusiness["business_languages"] = businessLanguages
	publicBusiness["supported_languages"] = supportedLanguages

	c.JSON(http.StatusOK, publicBusiness)
}

// ListPublishedStorefronts serves the public sitemap's slug list: published,
// active storefront slugs, bounded. Non-sensitive (slug + updated_at).
// kind=test fixtures are omitted; published demo showrooms are included.
func ListPublishedStorefronts(c *gin.Context) {
	const maxStorefronts = 5000 // bounded; paginate if this is ever hit (see plan Open questions)
	slugs, err := database.ListPublishedStorefrontSlugs(maxStorefronts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list storefronts"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"storefronts": slugs, "count": len(slugs)})
}
