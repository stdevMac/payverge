package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/money"
	"github.com/stdevmac/payverge/backend/internal/s3"
	"github.com/stdevmac/payverge/backend/internal/services"
	operational_alerts "github.com/stdevmac/payverge/backend/internal/services/operational_alerts"
	"github.com/stdevmac/payverge/backend/internal/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// staffIDFromContext extracts the staff_id from the Gin context, returning nil if not present or not staff.
func staffIDFromContext(c *gin.Context) *uint {
	tokenType, _ := c.Get("token_type")
	if ts, ok := tokenType.(string); !ok || ts != "staff" {
		return nil
	}
	raw, exists := c.Get("staff_id")
	if !exists {
		return nil
	}
	switch v := raw.(type) {
	case uint:
		id := v
		return &id
	case int:
		id := uint(v)
		return &id
	case int64:
		id := uint(v)
		return &id
	case float64:
		id := uint(v)
		return &id
	}
	return nil
}

func getBusinessActionActor(c *gin.Context) string {
	for _, key := range []string{"staff_email", "email", "address", "token_type"} {
		if value, exists := c.Get(key); exists {
			if actor, ok := value.(string); ok && strings.TrimSpace(actor) != "" {
				return actor
			}
		}
	}

	return ""
}

func businessOperationalAlertActor(c *gin.Context) operational_alerts.Actor {
	return operational_alerts.Actor{
		StaffID: staffIDFromContext(c),
		Name:    getBusinessActionActor(c),
	}
}

func resolveBillOperationalAlert(c *gin.Context, bill *database.Bill, reason string) {
	if bill == nil {
		return
	}
	if err := operational_alerts.NewService(database.GetDB()).ResolveAlertForResource(
		c.Request.Context(),
		bill.BusinessID,
		database.OperationalAlertResourceTypeBill,
		bill.ID,
		businessOperationalAlertActor(c),
		reason,
	); err != nil {
		log.Printf("failed to resolve bill operational alert: business_id=%d bill_id=%d reason=%s error=%v", bill.BusinessID, bill.ID, reason, err)
	}
}

// publishBillUpdatedEvent announces an operator-visible bill mutation to the
// business's SSE stream (bill.updated, gated on bills:read) so dashboards
// refetch immediately instead of waiting for the next poll. The embedded bill
// row serializes money as dollars via its MarshalJSON (money wire contract).
// Call AFTER the DB write has committed.
func publishBillUpdatedEvent(bill *database.Bill) {
	if bill == nil {
		return
	}
	if err := database.HydrateBillSnapshotForWire(bill); err != nil {
		log.Printf("bill.updated snapshot hydrate failed: bill_id=%d error=%v", bill.ID, err)
		bill.Items = ""
	}
	events.GetHub().PublishJSON(bill.BusinessID, "bill.updated", gin.H{
		"bill_id": bill.ID,
		"status":  string(bill.Status),
		"bill":    bill,
	})
	_ = events.NotifyTableBillChangedByBillID(database.GetDB(), bill.ID)
}

func parseDatabasePagination(c *gin.Context) database.PaginationParams {
	p := database.DefaultPagination()
	if raw := strings.TrimSpace(c.Query("page")); raw != "" {
		if page, err := strconv.Atoi(raw); err == nil {
			p.Page = page
		}
	}
	if raw := strings.TrimSpace(c.Query("page_size")); raw != "" {
		if pageSize, err := strconv.Atoi(raw); err == nil {
			p.PageSize = pageSize
		}
	}
	return p.Normalize()
}

func parseBillListFilters(c *gin.Context, businessID uint) (database.BillListFilters, error) {
	filters := database.BillListFilters{
		Status: strings.TrimSpace(c.Query("status")),
		Search: strings.TrimSpace(c.Query("search")),
	}
	dateLocation := (*time.Location)(nil)

	if raw := strings.TrimSpace(c.Query("date_from")); raw != "" {
		if dateLocation == nil {
			dateLocation = businessBillListDateLocation(businessID)
		}
		parsed, err := parseBillListDateBound(raw, false, dateLocation)
		if err != nil {
			return database.BillListFilters{}, fmt.Errorf("invalid date_from")
		}
		filters.CreatedFrom = &parsed
	}
	if raw := strings.TrimSpace(c.Query("date_to")); raw != "" {
		if dateLocation == nil {
			dateLocation = businessBillListDateLocation(businessID)
		}
		parsed, err := parseBillListDateBound(raw, true, dateLocation)
		if err != nil {
			return database.BillListFilters{}, fmt.Errorf("invalid date_to")
		}
		filters.CreatedTo = &parsed
	}
	if raw := strings.TrimSpace(c.Query("customer")); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 32)
		if err != nil || id == 0 {
			return database.BillListFilters{}, fmt.Errorf("invalid customer")
		}
		customerID := uint(id)
		filters.CustomerID = &customerID
	}

	return filters, nil
}

func businessBillListDateLocation(businessID uint) *time.Location {
	location := time.UTC
	db := database.GetDB()
	if db == nil {
		return location
	}

	var business database.Business
	if err := db.Select("timezone").First(&business, businessID).Error; err != nil {
		return location
	}
	if loadedLocation, err := time.LoadLocation(strings.TrimSpace(business.Timezone)); err == nil {
		return loadedLocation
	}
	return location
}

func parseBillListDateBound(raw string, exclusiveEnd bool, location *time.Location) (time.Time, error) {
	if location == nil {
		location = time.UTC
	}
	if parsed, err := time.ParseInLocation("2006-01-02", raw, location); err == nil {
		if exclusiveEnd {
			return parsed.AddDate(0, 0, 1), nil
		}
		return parsed, nil
	}

	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, err
	}
	return parsed, nil
}

type billListResponseBill struct {
	ID         uint  `json:"id"`
	BusinessID uint  `json:"business_id"`
	TableID    uint  `json:"table_id"`
	CounterID  *uint `json:"counter_id"`
	// TableName carries the joined tables.name from the BillListRow projection.
	// Dropping it here regresses list UIs to the raw-ID fallback ("Table 237").
	TableName  string `json:"table_name,omitempty"`
	BillNumber string `json:"bill_number"`
	Notes      string `json:"notes"`
	// Items is the legacy JSON snapshot as a line array (never a string; #771).
	// Omitted when empty/"[]" so list polls do not invent blank carts beside
	// item_count (FIND-057). Non-empty snapshots still ship for pre-bill_items
	// legacy rows (FE G-03 fallback).
	Items                json.RawMessage `json:"items,omitempty"`
	ItemCount            int64           `json:"item_count"`
	PhysicalItemQuantity int64           `json:"physical_item_quantity"`
	Subtotal             float64         `json:"subtotal"`
	TaxAmount            float64         `json:"tax_amount"`
	ServiceFeeAmount     float64         `json:"service_fee_amount"`
	TotalAmount          float64         `json:"total_amount"`
	PaidAmount           float64         `json:"paid_amount"`
	Remaining            float64         `json:"remaining"`
	TipAmount            float64         `json:"tip_amount"`
	// Currency labels the money above. The list projection never loads the
	// business, so before #852 every row arrived without a currency key and the
	// operator UI rendered peso totals as USD. Stamped from the business the
	// handler already resolved — no extra query, no per-row lookup.
	Currency string              `json:"currency"`
	Status   database.BillStatus `json:"status"`
	// Settlement/tipping wallets intentionally omitted from list responses
	// (FIND-045). Operator bill list/table UIs only need money totals + status;
	// wallets stay on dedicated bill detail / guest pay endpoints.
	CreatedByStaffID    *uint      `json:"created_by_staff_id,omitempty"`
	ClosedByStaffID     *uint      `json:"closed_by_staff_id,omitempty"`
	CRMCustomerID       *uint      `json:"crm_customer_id,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	ClosedAt            *time.Time `json:"closed_at"`
	FeedbackEmailSentAt *time.Time `json:"feedback_email_sent_at,omitempty"`
}

// billListItemsSnapshot keeps only non-empty legacy snapshots. Empty string,
// "[]", and "null" are treated as absent so the list wire does not claim an
// empty cart when item_count is the real source of truth (FIND-057).
func billListItemsSnapshot(items string) string {
	switch strings.TrimSpace(items) {
	case "", "[]", "null":
		return ""
	default:
		return items
	}
}

func newBillListResponseBill(row database.BillListRow, currency string) billListResponseBill {
	remainingCents := row.TotalAmount - row.PaidAmount
	if remainingCents < 0 {
		remainingCents = 0
	}
	return billListResponseBill{
		ID:                   row.ID,
		BusinessID:           row.BusinessID,
		TableID:              row.TableID,
		CounterID:            row.CounterID,
		TableName:            row.TableName,
		BillNumber:           row.BillNumber,
		Notes:                row.Notes,
		Items:                database.JSONItemsArrayWire(billListItemsSnapshot(row.Items)),
		ItemCount:            row.ItemCount,
		PhysicalItemQuantity: row.PhysicalItemQuantity,
		Subtotal:             float64(row.Subtotal) / 100.0,
		TaxAmount:            float64(row.TaxAmount) / 100.0,
		ServiceFeeAmount:     float64(row.ServiceFeeAmount) / 100.0,
		TotalAmount:          float64(row.TotalAmount) / 100.0,
		PaidAmount:           float64(row.PaidAmount) / 100.0,
		Remaining:            float64(remainingCents) / 100.0,
		TipAmount:            float64(row.TipAmount) / 100.0,
		Currency:             currency,
		Status:               row.Status,
		CreatedByStaffID:     row.CreatedByStaffID,
		ClosedByStaffID:      row.ClosedByStaffID,
		CRMCustomerID:        row.CRMCustomerID,
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
		ClosedAt:             row.ClosedAt,
		FeedbackEmailSentAt:  row.FeedbackEmailSentAt,
	}
}

// shapeBillListResponse stamps one already-resolved currency onto every row.
// All rows in a bill list belong to the same business by construction, so the
// label is resolved once by the caller instead of per row (#852).
func shapeBillListResponse(rows []database.BillListRow, currency string) []billListResponseBill {
	responseBills := make([]billListResponseBill, len(rows))
	for i, row := range rows {
		responseBills[i] = newBillListResponseBill(row, currency)
	}
	return responseBills
}

func respondWithPaginatedBills(c *gin.Context, business *database.Business, result *database.PaginatedResult[database.BillListRow]) {
	c.JSON(http.StatusOK, gin.H{
		"bills":       shapeBillListResponse(result.Data, database.BusinessDisplayCurrency(business)),
		"total":       result.Total,
		"page":        result.Page,
		"page_size":   result.PageSize,
		"total_pages": result.TotalPages,
	})
}

func businessFromBusinessIdentifierParam(c *gin.Context) (*database.Business, bool) {
	businessIdentifier := utils.BusinessIdentifierFromParam(c, "id")
	business, err := database.GetBusinessByIdOrBusinessId(businessIdentifier)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return nil, false
	}
	return business, true
}

// determineBusinessOwnerLanguage determines the best language to communicate with the business owner
// Priority: User Profile Language > Business Default Language > English
func determineBusinessOwnerLanguage(business *database.Business) string {
	return services.DetermineBusinessOwnerLanguage(business)
}

// Compiled once at init rather than per call: these run on business
// create/update handler paths.
var (
	businessEthAddressRegex = regexp.MustCompile("^0x[0-9a-fA-F]{40}$")
	businessSlugStripRegex  = regexp.MustCompile(`[^a-z0-9-]`)
)

// payoutWalletsMustDiffer reports whether both payout wallets are set and equal
// (case-insensitive). Tips and sales must not share one address — otherwise tip
// pools cannot be reconciled against staff payouts.
func payoutWalletsMustDiffer(settlement, tipping string) bool {
	s := strings.TrimSpace(settlement)
	t := strings.TrimSpace(tipping)
	return s != "" && t != "" && strings.EqualFold(s, t)
}

// isValidEthereumAddress validates if a string is a valid Ethereum address
func isValidEthereumAddress(address string) bool {
	return businessEthAddressRegex.MatchString(address)
}

func businessCreationHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// Business creation request
type CreateBusinessRequest struct {
	BusinessId       string                   `json:"business_id"`
	OwnerName        string                   `json:"owner_name"`
	Name             string                   `json:"name" binding:"required"`
	Logo             string                   `json:"logo"`
	Address          database.BusinessAddress `json:"address"`
	SettlementAddr   string                   `json:"settlement_address"`
	TippingAddr      string                   `json:"tipping_address"`
	TaxRate          float64                  `json:"tax_rate"`
	ServiceFeeRate   float64                  `json:"service_fee_rate"`
	TaxInclusive     bool                     `json:"tax_inclusive"`
	ServiceInclusive bool                     `json:"service_inclusive"`
	// New fields for enhanced business features
	Description          string `json:"description"`
	CustomURL            string `json:"custom_url"`
	Phone                string `json:"phone"`
	Email                string `json:"email"`
	Website              string `json:"website"`
	SocialMedia          string `json:"social_media"`
	BannerImages         string `json:"banner_images"`
	BusinessPageEnabled  bool   `json:"business_page_enabled"`
	ShowReviews          bool   `json:"show_reviews"`
	GoogleReviewsEnabled bool   `json:"google_reviews_enabled"`
	// Counter settings
	CounterEnabled bool   `json:"counter_enabled"`
	CounterCount   int    `json:"counter_count"`
	CounterPrefix  string `json:"counter_prefix"`
	// Currency and language settings
	DefaultCurrency string `json:"default_currency"`
	DisplayCurrency string `json:"display_currency"`
	DefaultLanguage string `json:"default_language"`
	SourceLanguage  string `json:"source_language"`
	// Locale + segmentation captured at registration
	Timezone     string `json:"timezone"` // IANA timezone identifier
	BusinessType string `json:"business_type"`
	// Hospitality features
	WelcomeMessage      string `json:"welcome_message"`
	AboutStory          string `json:"about_story"`
	ShowWelcomeMessage  bool   `json:"show_welcome_message"`
	ShowAboutStory      bool   `json:"show_about_story"`
	ShowGallery         bool   `json:"show_gallery"`
	ShowOperatingHours  bool   `json:"show_operating_hours"`
	ShowSpecialFeatures bool   `json:"show_special_features"`
}

var sendCreateBusinessOnboardingEmail = services.SendBusinessOnboardingEmail

var sendAdminNewSignupEmailHook = services.SendAdminNewSignupEmailForBusiness

// Business update request
type UpdateBusinessRequest struct {
	Name             string                    `json:"name"`
	Logo             string                    `json:"logo"`
	Address          *database.BusinessAddress `json:"address"`
	SettlementAddr   string                    `json:"settlement_address"`
	TippingAddr      string                    `json:"tipping_address"`
	TaxRate          *float64                  `json:"tax_rate"`
	ServiceFeeRate   *float64                  `json:"service_fee_rate"`
	TaxInclusive     *bool                     `json:"tax_inclusive"`
	ServiceInclusive *bool                     `json:"service_inclusive"`
	// New fields for enhanced business features
	Description          *string `json:"description"`
	CustomURL            *string `json:"custom_url"`
	Phone                *string `json:"phone"`
	Website              *string `json:"website"`
	SocialMedia          *string `json:"social_media"`
	BannerImages         *string `json:"banner_images"`
	BusinessPageEnabled  *bool   `json:"business_page_enabled"`
	ShowReviews          *bool   `json:"show_reviews"`
	GoogleReviewsEnabled *bool   `json:"google_reviews_enabled"`
	// Currency settings
	DefaultCurrency *string `json:"default_currency"`
	DisplayCurrency *string `json:"display_currency"`
	Timezone        *string `json:"timezone"` // IANA timezone identifier
	// ServiceDayStartMinute is minutes after local midnight (0–1439) when the
	// venue service day begins. Nil leaves the stored value untouched.
	ServiceDayStartMinute *int    `json:"service_day_start_minute"`
	BusinessType          *string `json:"business_type"`
	// Hospitality features
	WelcomeMessage      *string `json:"welcome_message"`
	AboutStory          *string `json:"about_story"`
	ShowWelcomeMessage  *bool   `json:"show_welcome_message"`
	ShowAboutStory      *bool   `json:"show_about_story"`
	ShowGallery         *bool   `json:"show_gallery"`
	ShowOperatingHours  *bool   `json:"show_operating_hours"`
	ShowSpecialFeatures *bool   `json:"show_special_features"`
	// AI Waiter Settings
	AiEnabled                 *bool   `json:"ai_enabled"`
	AiName                    *string `json:"ai_name"`
	AiPriority                *string `json:"ai_priority"`
	AiSpecialInstructions     *string `json:"ai_special_instructions"`
	BusinessPageAiEnabled     *bool   `json:"business_page_ai_enabled"`
	DefaultQRLogoURL          *string `json:"default_qr_logo_url"`
	DefaultQRForegroundColor  *string `json:"default_qr_foreground_color"`
	DefaultQRBackgroundColor  *string `json:"default_qr_background_color"`
	DefaultQRLogoSize         *int    `json:"default_qr_logo_size"`
	DefaultQRShowBusinessName *bool   `json:"default_qr_show_business_name"`
	DefaultQRShowTableName    *bool   `json:"default_qr_show_table_name"`
	DefaultQRTextFont         *string `json:"default_qr_text_font"`
}

// GenerateBusinessIdRequest represents a request to generate a business ID
type GenerateBusinessIdRequest struct {
	Name string `json:"name" binding:"required"`
}

// GenerateBusinessIdResponse represents the response with a generated business ID
type GenerateBusinessIdResponse struct {
	BusinessId string `json:"business_id"`
}

// generateUniqueBusinessId generates a unique business ID based on name and random bytes
func generateUniqueBusinessId(name string) (string, error) {
	// Create a base from the business name (lowercase, replace spaces with hyphens)
	nameBase := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), " ", "-"))
	// Remove any non-alphanumeric characters except hyphens
	nameBase = businessSlugStripRegex.ReplaceAllString(nameBase, "")
	// Limit to 20 characters
	if len(nameBase) > 20 {
		nameBase = nameBase[:20]
	}

	// Generate 4 random bytes for uniqueness
	randomBytes := make([]byte, 4)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	randomSuffix := hex.EncodeToString(randomBytes)

	// Combine name base with random suffix
	businessId := fmt.Sprintf("%s-%s", nameBase, randomSuffix)
	return businessId, nil
}

// GenerateBusinessId generates a unique business ID for registration
func GenerateBusinessId(c *gin.Context) {
	// Check authentication - accept both Web3 users (address) and OAuth users (user_id)
	_, hasAddress := c.Get("address")
	_, hasUserID := c.Get("user_id")

	if !hasAddress && !hasUserID {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var req GenerateBusinessIdRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	// Generate unique business ID
	businessId, err := generateUniqueBusinessId(req.Name)
	if err != nil {
		log.Printf("Failed to generate business ID: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate business ID"})
		return
	}

	// Return the generated business ID
	c.JSON(http.StatusOK, GenerateBusinessIdResponse{
		BusinessId: businessId,
	})
}

// validateStorefrontImageURL accepts only our own public media: a same-origin
// "/media/<public key>" URL, or https on the configured public asset host.
// A trimmed empty string is allowed and clears the field.
func validateStorefrontImageURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if s3.IsOwnMediaURL(raw) {
		if key, err := s3.KeyFromURL(raw); err == nil && !s3.IsProtectedMediaKey(key) {
			return nil
		}
		return errors.New("invalid storefront image URL")
	}
	if s3.ValidatePublicAssetURL(raw, s3.PublicHost()) == nil {
		return nil
	}
	return errors.New("invalid storefront image URL")
}

// validateBannerImagesJSON accepts an empty value, an empty list, or JSON null.
// Any other value must be a JSON array of at most 20 storefront image URLs.
func validateBannerImagesJSON(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" || raw == "null" {
		return nil
	}
	var urls []string
	if err := json.Unmarshal([]byte(raw), &urls); err != nil {
		return errors.New("invalid banner images")
	}
	if len(urls) > 20 {
		return errors.New("invalid banner images")
	}
	for _, rawURL := range urls {
		if err := validateStorefrontImageURL(rawURL); err != nil {
			return err
		}
	}
	return nil
}

// CreateBusiness creates a new business for the authenticated user
func CreateBusiness(c *gin.Context) {
	// Get user authentication - support both Web3 (address) and OAuth (user_id + email)
	userAddress, hasAddress := c.Get("address")
	userID, hasUserID := c.Get("user_id")
	userEmail, hasEmail := c.Get("email")

	if !hasAddress && !hasUserID {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// For OAuth users without wallet address, use empty string
	var ownerAddress string
	if hasAddress {
		ownerAddress = userAddress.(string)
	} else {
		ownerAddress = "" // OAuth users don't have wallet addresses
	}

	var req CreateBusinessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if len(idempotencyKey) > 255 {
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "Idempotency-Key is too long")
		return
	}
	// Hash the decoded request before generated IDs/defaults are applied. JSON
	// object key order and omitted zero values therefore resolve to one stable,
	// canonical payload for replay/conflict decisions.
	canonicalPayload, err := json.Marshal(req)
	if err != nil {
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Failed to prepare business creation")
		return
	}
	payloadHash := businessCreationHash(string(canonicalPayload))

	// Validate Ethereum addresses
	if req.SettlementAddr != "" && !isValidEthereumAddress(req.SettlementAddr) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid settlement address"})
		return
	}
	if req.TippingAddr != "" && !isValidEthereumAddress(req.TippingAddr) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tipping address"})
		return
	}
	if payoutWalletsMustDiffer(req.SettlementAddr, req.TippingAddr) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Tip wallet must be different from the sales settlement wallet so tips can be paid out separately",
		})
		return
	}

	// Validate custom URL if provided
	if req.CustomURL != "" {
		if err := validateCustomURL(req.CustomURL, 0); err != nil {
			if errors.Is(err, errCustomURLLookupFailed) {
				RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Failed to check custom URL availability")
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	if err := validateStorefrontImageURL(req.Logo); err != nil {
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "Logo must be an uploaded image")
		return
	}
	if err := validateBannerImagesJSON(req.BannerImages); err != nil {
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "Banner images must be uploaded images")
		return
	}

	if req.BusinessId == "" {
		generatedBusinessID, err := generateUniqueBusinessId(req.Name)
		if err != nil {
			log.Printf("Failed to generate fallback business ID: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate business ID"})
			return
		}
		req.BusinessId = generatedBusinessID
	}

	// Set UserID for OAuth users
	var businessUserID *uint
	if hasUserID {
		if uid, ok := extractContextUint(userID); ok && uid != 0 {
			businessUserID = &uid
		}
	}

	// Set Email for OAuth users if not already in request
	businessEmail := req.Email
	if businessEmail == "" && hasEmail {
		if email, ok := userEmail.(string); ok {
			businessEmail = email
		}
	}

	// Seed currency + timezone from the country (explicit values win), set the
	// business type, and default counter mode for counter-style types.
	seedCurrency, seedTimezone, seedBusinessType, seedCounter := applyCreateSeed(
		req.Address.Country, req.DefaultCurrency, req.Timezone, req.BusinessType, req.CounterEnabled,
	)
	seedDisplayCurrency := req.DisplayCurrency
	if seedDisplayCurrency == "" {
		seedDisplayCurrency = seedCurrency
	}

	business := &database.Business{
		OwnerAddress:     ownerAddress,
		UserID:           businessUserID, // Set for OAuth users
		OwnerName:        req.OwnerName,
		BusinessId:       req.BusinessId,
		Name:             req.Name,
		Logo:             req.Logo,
		Address:          req.Address,
		SettlementAddr:   req.SettlementAddr,
		TippingAddr:      req.TippingAddr,
		TaxRate:          req.TaxRate,
		ServiceFeeRate:   req.ServiceFeeRate,
		TaxInclusive:     req.TaxInclusive,
		ServiceInclusive: req.ServiceInclusive,
		IsActive:         true,
		// New fields
		Description:          req.Description,
		CustomURL:            req.CustomURL,
		Phone:                req.Phone,
		Email:                businessEmail, // Use email from context for OAuth users
		Website:              req.Website,
		SocialMedia:          req.SocialMedia,
		BannerImages:         req.BannerImages,
		BusinessPageEnabled:  req.BusinessPageEnabled,
		ShowReviews:          req.ShowReviews,
		GoogleReviewsEnabled: req.GoogleReviewsEnabled,
		// Counter settings
		CounterEnabled: seedCounter,
		CounterCount:   req.CounterCount,
		CounterPrefix:  req.CounterPrefix,
		// Currency and language settings
		DefaultCurrency: seedCurrency,
		DisplayCurrency: seedDisplayCurrency,
		DefaultLanguage: req.DefaultLanguage,
		SourceLanguage:  req.SourceLanguage,
		// Locale + segmentation
		Timezone:     seedTimezone,
		BusinessType: seedBusinessType,
		// Design settings with defaults
		DesignSettings: database.BusinessDesignSettings{
			PrimaryColor:      "#1f2937",
			SecondaryColor:    "#3b82f6",
			FontFamily:        "Inter",
			Theme:             "light",
			MenuLayout:        "grid",
			ShowImages:        true,
			ShowDescriptions:  true,
			HeaderStyle:       "banner",
			CornerRadius:      "medium",
			ShadowIntensity:   "subtle",
			BackgroundPattern: "none",
			PatternOpacity:    0.1,
		},
		// Hospitality features
		WelcomeMessage:      req.WelcomeMessage,
		AboutStory:          req.AboutStory,
		ShowWelcomeMessage:  req.ShowWelcomeMessage,
		ShowAboutStory:      req.ShowAboutStory,
		ShowGallery:         req.ShowGallery,
		ShowOperatingHours:  req.ShowOperatingHours,
		ShowSpecialFeatures: req.ShowSpecialFeatures,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
	}

	if idempotencyKey != "" {
		ownerScope := "wallet:" + strings.ToLower(strings.TrimSpace(ownerAddress))
		if businessUserID != nil {
			ownerScope = "user:" + strconv.FormatUint(uint64(*businessUserID), 10)
		}
		resolved, replay, err := database.CreateBusinessIdempotently(
			business,
			businessCreationHash(ownerScope),
			businessCreationHash(idempotencyKey),
			payloadHash,
		)
		if errors.Is(err, database.ErrBusinessCreationIdempotencyConflict) {
			RespondWithError(c, http.StatusConflict, ErrCodeConflict,
				"Idempotency-Key was already used with a different business payload")
			return
		}
		if err != nil {
			log.Printf("Error creating business idempotently: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create business"})
			return
		}
		business = resolved
		if replay {
			c.JSON(http.StatusCreated, business)
			return
		}
	} else if err := database.CreateBusiness(business); err != nil {
		log.Printf("Error creating business: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create business"})
		return
	}

	// Crypto rails are opt-in under Plugins (never auto-enabled). Card/checkout
	// paths like Mercado Pago lead the dinner-service payment story.

	// Alert the admin inbox of every new signup. Non-fatal.
	if err := sendAdminNewSignupEmailHook(business); err != nil {
		log.Printf("Warning: failed to send admin new-signup alert for business %d: %v", business.ID, err)
	}

	if business.Email != "" {
		if err := sendCreateBusinessOnboardingEmail(business); err != nil {
			log.Printf("Failed to send business onboarding email: %v", err)
		}
	}

	c.JSON(http.StatusCreated, business)
}

// GetMyBusinesses retrieves businesses the actor may open. Owners and demo
// owners see their venues; platform admins also see active demos so list and
// GET /inside/businesses/:id share one rule.
func GetMyBusinesses(c *gin.Context) {
	// Check token type to determine how to look up businesses
	tokenType, _ := c.Get("token_type")

	var businesses []database.Business
	var err error

	switch tokenType {
	case "user":
		// Email/OAuth users are linked canonically through businesses.user_id.
		// A restaurant's public contact email is not an ownership credential.
		userIDInterface, exists := c.Get("user_id")
		userID, validUserID := extractContextUint(userIDInterface)
		if !exists || !validUserID || userID == 0 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}
		businesses, err = database.ListBusinessesForInsideUser(userID, hasPlatformAdminRole(c))
	case "web3":
		// Web3 user - look up by wallet address
		userAddress, exists := c.Get("address")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			return
		}
		businesses, err = database.GetBusinessByOwnerAddress(userAddress.(string))
	default:
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve businesses"})
		return
	}

	c.JSON(http.StatusOK, businesses)
}

// GetBusiness retrieves a specific business by ID or businessId
func GetBusiness(c *gin.Context) {
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

	// Owners and platform admins (impersonate / admin read) get the full record
	// (dashboard settlement UIs need it). Staff of this business get a
	// scoped projection that omits owner wallets, contact email, Stripe IDs,
	// and billing transactions. Any other authenticated user gets the public
	// projection. (RBAC-BIZRECORD-LEAK-01)
	if CheckBusinessOwnership(c, business) || hasPlatformAdminRole(c) {
		c.JSON(http.StatusOK, business)
		return
	}
	if CheckBusinessAccess(c, business) {
		c.JSON(http.StatusOK, staffBusinessProjection(business))
		return
	}
	c.JSON(http.StatusOK, publicBusinessProjection(business))
}

// stripOwnerOnlyFieldsForStaff zeros out wallet/owner-only fields when the
// actor is a staff member and returns the JSON names of the fields that were
// actually submitted and dropped, so UpdateBusiness can disclose the drop
// instead of returning a silent 200 (P1-12). Manager can edit profile fields
// (name, address, description, etc.) but must not be able to move the
// owner's wallets.
//
// If UpdateBusinessRequest grows new wallet/owner fields later, update this
// function — the matrix test does not currently catch silent wallet writes
// (the Postgres integration test in staff_settings_pg_test.go does, when T8 lands).
func stripOwnerOnlyFieldsForStaff(req *UpdateBusinessRequest, c *gin.Context) []string {
	tokenType, _ := c.Get("token_type")
	ts, ok := tokenType.(string)
	if !ok || ts != "staff" {
		return nil
	}
	var skipped []string
	if strings.TrimSpace(req.SettlementAddr) != "" {
		skipped = append(skipped, "settlement_address")
	}
	req.SettlementAddr = ""
	if strings.TrimSpace(req.TippingAddr) != "" {
		skipped = append(skipped, "tipping_address")
	}
	req.TippingAddr = ""
	// AI configuration is owner-only. Managers can handle live conversations
	// (see ai_waiter:reply) but must not change the AI's identity/behavior.
	if req.AiEnabled != nil {
		skipped = append(skipped, "ai_enabled")
	}
	req.AiEnabled = nil
	if req.AiName != nil {
		skipped = append(skipped, "ai_name")
	}
	req.AiName = nil
	if req.AiPriority != nil {
		skipped = append(skipped, "ai_priority")
	}
	req.AiPriority = nil
	if req.AiSpecialInstructions != nil {
		skipped = append(skipped, "ai_special_instructions")
	}
	req.AiSpecialInstructions = nil
	if req.BusinessPageAiEnabled != nil {
		skipped = append(skipped, "business_page_ai_enabled")
	}
	req.BusinessPageAiEnabled = nil
	return skipped
}

// UpdateBusiness updates an existing business
func UpdateBusiness(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	var req UpdateBusinessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	// Strip wallet/owner-only fields if actor is staff; remember what was
	// dropped so the response can disclose it (P1-12).
	skippedFields := stripOwnerOnlyFieldsForStaff(&req, c)

	// Public demo: the showroom's identity, links and payout addresses are
	// frozen; everything else stays editable (business_demo_freeze.go).
	if config.DemoModeEnabled() {
		if kind, action, refused := demoStorefrontRefusal(&req, business); refused {
			demomode.Refuse(c, kind, action)
			return
		}
	}

	if req.Logo != "" {
		if err := validateStorefrontImageURL(req.Logo); err != nil {
			RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "Logo must be an uploaded image")
			return
		}
	}
	if req.BannerImages != nil {
		if err := validateBannerImagesJSON(*req.BannerImages); err != nil {
			RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "Banner images must be uploaded images")
			return
		}
	}

	// Update business fields
	if req.Name != "" {
		business.Name = req.Name
	}
	if req.Logo != "" {
		business.Logo = req.Logo
	}
	if req.Address != nil {
		business.Address = *req.Address
	}

	// Track if wallet addresses changed for email notification. The two
	// rotation flags are separate: a settlement change must not rewrite
	// tipping_addr on open bills, and the reverse.
	walletChanged := false
	settlementChanged := false
	tippingChanged := false

	if req.SettlementAddr != "" {
		if business.SettlementAddr != req.SettlementAddr {
			if !isValidEthereumAddress(req.SettlementAddr) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid settlement address"})
				return
			}
			walletChanged = true
			settlementChanged = true
		}
		business.SettlementAddr = req.SettlementAddr
	}
	if req.TippingAddr != "" {
		if business.TippingAddr != req.TippingAddr {
			if !isValidEthereumAddress(req.TippingAddr) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tipping address"})
				return
			}
			walletChanged = true
			tippingChanged = true
		}
		business.TippingAddr = req.TippingAddr
	}
	// Only enforce tip≠settlement when this request mutates a wallet. Legacy
	// rows may already share one USDC address; profile-only saves must not
	// lock those venues out of UpdateBusiness.
	if walletChanged && payoutWalletsMustDiffer(business.SettlementAddr, business.TippingAddr) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Tip wallet must be different from the sales settlement wallet so tips can be paid out separately",
		})
		return
	}
	if req.TaxRate != nil {
		business.TaxRate = *req.TaxRate
	}
	if req.ServiceFeeRate != nil {
		business.ServiceFeeRate = *req.ServiceFeeRate
	}
	if req.TaxInclusive != nil {
		business.TaxInclusive = *req.TaxInclusive
	}
	if req.ServiceInclusive != nil {
		business.ServiceInclusive = *req.ServiceInclusive
	}

	// Capture the old slug before it is overwritten so we can evict
	// the BusinessByCustomURL cache entry for the old key after the write.
	oldCustomURL := business.CustomURL

	// Update new fields
	if req.Description != nil {
		business.Description = *req.Description
	}
	if req.CustomURL != nil {
		if *req.CustomURL != business.CustomURL {
			// Validate only when the value actually changes.
			if err := validateCustomURL(*req.CustomURL, business.ID); err != nil {
				if errors.Is(err, errCustomURLLookupFailed) {
					RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Failed to check custom URL availability")
					return
				}
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}
		business.CustomURL = *req.CustomURL
	}
	if req.Phone != nil {
		business.Phone = *req.Phone
	}
	if req.Website != nil {
		business.Website = *req.Website
	}
	if req.SocialMedia != nil {
		business.SocialMedia = *req.SocialMedia
	}
	if req.BannerImages != nil {
		business.BannerImages = *req.BannerImages
	}
	if req.BusinessPageEnabled != nil {
		business.BusinessPageEnabled = *req.BusinessPageEnabled
	}
	if req.ShowReviews != nil {
		business.ShowReviews = *req.ShowReviews
	}
	if req.GoogleReviewsEnabled != nil {
		business.GoogleReviewsEnabled = *req.GoogleReviewsEnabled
	}
	// Update currency settings
	if req.DefaultCurrency != nil {
		business.DefaultCurrency = *req.DefaultCurrency
	}
	if req.DisplayCurrency != nil {
		business.DisplayCurrency = *req.DisplayCurrency
	}
	if req.Timezone != nil {
		// Reject at the door instead of storing verbatim: every reader resolves
		// an unknown zone to UTC silently, so a typo here shifts analytics,
		// payroll, and report day boundaries with nothing on screen to explain
		// it. It is also the only route by which unbounded operator free text
		// reaches the process-lifetime location cache in database.ResolveLocation.
		tz := strings.TrimSpace(*req.Timezone)
		if tz != "" {
			if _, err := time.LoadLocation(tz); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid timezone"})
				return
			}
		}
		business.Timezone = tz
	}
	if req.ServiceDayStartMinute != nil {
		minute := *req.ServiceDayStartMinute
		if minute < 0 || minute >= 24*60 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid service_day_start_minute"})
			return
		}
		business.ServiceDayStartMinute = minute
	}
	if req.BusinessType != nil {
		business.BusinessType = normalizeBusinessType(*req.BusinessType)
	}

	// Update hospitality features
	if req.WelcomeMessage != nil {
		business.WelcomeMessage = *req.WelcomeMessage
	}
	if req.AboutStory != nil {
		business.AboutStory = *req.AboutStory
	}
	if req.ShowWelcomeMessage != nil {
		business.ShowWelcomeMessage = *req.ShowWelcomeMessage
	}
	if req.ShowAboutStory != nil {
		business.ShowAboutStory = *req.ShowAboutStory
	}
	if req.ShowGallery != nil {
		business.ShowGallery = *req.ShowGallery
	}
	if req.ShowOperatingHours != nil {
		business.ShowOperatingHours = *req.ShowOperatingHours
	}
	if req.ShowSpecialFeatures != nil {
		business.ShowSpecialFeatures = *req.ShowSpecialFeatures
	}

	// Update AI Waiter settings
	if ((req.AiEnabled != nil && *req.AiEnabled) || (req.BusinessPageAiEnabled != nil && *req.BusinessPageAiEnabled)) && RespondIfBusinessLocked(c, business) {
		return
	}
	if req.AiEnabled != nil {
		business.AiSettings.AiEnabled = *req.AiEnabled
	}
	if req.AiName != nil {
		normalized, err := NormalizeAndValidateAiName(*req.AiName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		business.AiSettings.AiName = normalized
	}
	if req.AiPriority != nil {
		business.AiSettings.AiPriority = *req.AiPriority
	}
	if req.AiSpecialInstructions != nil {
		business.AiSettings.SpecialInstructions = *req.AiSpecialInstructions
	}
	if req.BusinessPageAiEnabled != nil {
		business.AiSettings.BusinessPageAiEnabled = *req.BusinessPageAiEnabled
	}
	if req.DefaultQRLogoURL != nil {
		business.DefaultQRLogoURL = *req.DefaultQRLogoURL
	}
	if req.DefaultQRForegroundColor != nil {
		business.DefaultQRForegroundColor = *req.DefaultQRForegroundColor
	}
	if req.DefaultQRBackgroundColor != nil {
		business.DefaultQRBackgroundColor = *req.DefaultQRBackgroundColor
	}
	if req.DefaultQRLogoSize != nil {
		business.DefaultQRLogoSize = *req.DefaultQRLogoSize
	}
	if req.DefaultQRShowBusinessName != nil {
		business.DefaultQRShowBusinessName = *req.DefaultQRShowBusinessName
	}
	if req.DefaultQRShowTableName != nil {
		business.DefaultQRShowTableName = *req.DefaultQRShowTableName
	}
	if req.DefaultQRTextFont != nil {
		business.DefaultQRTextFont = *req.DefaultQRTextFont
	}

	business.UpdatedAt = time.Now()

	if err := database.UpdateBusinessExceptDesignRotatingWallets(business, settlementChanged, tippingChanged); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update business"})
		return
	}

	// Evict cached lookups that may now be stale: the old slug (if changed),
	// the new slug, and the pricing snapshot (timezone/currency/business-active
	// fields all feed into pricing resolution).
	services.InvalidateBusinessCustomURL(oldCustomURL)
	if business.CustomURL != oldCustomURL {
		services.InvalidateBusinessCustomURL(business.CustomURL)
	}
	services.InvalidatePricingCache(business.ID)
	services.InvalidatePublicGuestBusiness(business.ID)
	invalidateReservationAvailability(business.ID)

	// Crypto rails stay opt-in — adding a payout wallet no longer auto-enables
	// USDC / any-token plugins (Mercado Pago and other checkout rails lead).

	// Send wallet change notification email if addresses were changed
	if walletChanged && business.Email != "" && emails.EmailServerInstance != nil {
		baseURL := config.FrontendBaseURL()
		dashboardURL := fmt.Sprintf("%s/business/%d/dashboard", baseURL, business.ID)

		// Determine language (User Preference > Business Default > English)
		language := determineBusinessOwnerLanguage(business)

		// Stamped with the business: business.Email is tenant-typed and
		// unverified, so a wallet toggle loop must count against the tenant
		// budget (and is collapsed by its dedupe window) rather than mailing
		// that address without limit. A spent budget can suppress this notice,
		// but the same request can already repoint business.Email, so the
		// budget adds no new way to hide a wallet change.
		if err := emails.EmailServerInstance.ForBusiness(business.ID).SendWalletChangeEmail(
			[]string{business.Email},
			business.Name, // owner name
			dashboardURL,
			language,
		); err != nil {
			// Log error but don't fail the request
			log.Printf("Failed to send wallet change email: %v", err)
		}
	}

	// Mirror the GetBusiness response discipline: owners get the full struct;
	// staff get the scoped projection. (RBAC-BIZRECORD-LEAK-01)
	if CheckBusinessOwnership(c, business) {
		c.JSON(http.StatusOK, business)
	} else {
		projection := staffBusinessProjection(business)
		if len(skippedFields) > 0 {
			projection["skipped_fields"] = skippedFields
		}
		c.JSON(http.StatusOK, projection)
	}
}

// DeleteBusiness soft deletes a business — owner-only.
func DeleteBusiness(c *gin.Context) {
	business, ok := requireBusinessOwnership(c, "id")
	if !ok {
		return
	}

	if err := database.DeleteBusiness(business.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete business"})
		return
	}
	services.InvalidateBusinessCustomURL(business.CustomURL)
	services.InvalidatePricingCache(business.ID)
	services.InvalidatePublicGuestBusiness(business.ID)
	invalidateReservationAvailability(business.ID)

	// Wave 5: permanent business closure purges tenant-scoped AI history.
	// An admin suspension must not call this path. Fail closed if
	// purge cannot complete so ops can retry without reporting a clean erasure.
	if _, err := database.PurgeBusinessAIData(business.ID); err != nil {
		log.Printf("DeleteBusiness: AI data purge failed for business %d: %v", business.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Business was deactivated but AI data cleanup did not complete. Contact support — do not assume transcripts are erased.",
			"code":  "ai_purge_incomplete",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Business deleted successfully"})
}

// Bill management handlers

// CreateBillRequest represents the request to create a new bill
type CreateBillRequest struct {
	TableID   *uint               `json:"table_id"`
	CounterID *uint               `json:"counter_id"`
	Notes     string              `json:"notes"`
	Items     []database.BillItem `json:"items"`
}

// UpdateBillRequest represents the request to update a bill
type UpdateBillRequest struct {
	Items []database.BillItem `json:"items"`
}

// AddBillItemRequest represents the request to add an item to a bill
type AddBillItemRequest struct {
	MenuItemID string                    `json:"menu_item_id" binding:"required"`
	Name       string                    `json:"name" binding:"required"`
	Price      *float64                  `json:"price" binding:"required"`
	Quantity   int                       `json:"quantity" binding:"required"`
	Options    []database.MenuItemOption `json:"options"`
}

type AdjustBillItemRequest struct {
	Quantity *int   `json:"quantity"`
	Void     bool   `json:"void"`
	Reason   string `json:"reason"`
}

// CreateBill creates a new bill for a business
func CreateBill(c *gin.Context) {
	business, ok := requireBusinessAccess(c, "id")
	if !ok {
		return
	}

	businessID := uint64(business.ID)

	var req CreateBillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	// Validate that either table_id or counter_id is provided, but not both
	if req.TableID == nil && req.CounterID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Either table_id or counter_id must be provided"})
		return
	}

	if req.TableID != nil && req.CounterID != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot specify both table_id and counter_id"})
		return
	}

	// Validate table if provided
	if req.TableID != nil {
		table, err := database.GetTableByID(*req.TableID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Table not found"})
			return
		}

		if table.BusinessID != uint(businessID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Table does not belong to this business"})
			return
		}

		// Check if table already has an active open or partially paid bill.
		existingBill, _, err := database.GetOpenBillByTableID(*req.TableID)
		if err == nil && existingBill != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "Table already has an open bill"})
			return
		}
		if err != nil && !errors.Is(err, database.ErrNoActiveBill) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check for active bill"})
			return
		}
	}

	// Validate counter if provided
	if req.CounterID != nil {
		counter, err := database.GetCounterByID(*req.CounterID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Counter not found"})
			return
		}

		if counter.BusinessID != uint(businessID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Counter does not belong to this business"})
			return
		}

		if !counter.IsActive {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Counter is not active"})
			return
		}
	}

	_, categories, err := database.GetMenuByBusinessID(uint(businessID))
	if err != nil {
		categories = []database.MenuCategory{}
	}
	offers, bundles := getActivePromotionsForBusiness(business)

	promotionInput := make([]services.PromotionInputLine, 0, len(req.Items))
	for _, item := range req.Items {
		promotionInput = append(promotionInput, services.PromotionInputLine{
			Name:            item.Name,
			MenuItemID:      item.MenuItemID,
			Quantity:        item.Quantity,
			UnitPrice:       item.Price,
			Options:         item.Options,
			ItemType:        item.ItemType,
			BundleID:        item.BundleID,
			ParentBundleID:  item.ParentBundleID,
			SourceOfferID:   item.SourceOfferID,
			SpecialRequests: "",
		})
	}

	promotionResult, err := services.ApplyPromotionsToOrder(categories, bundles, offers, promotionInput)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(promotionResult.Lines) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Bill must contain at least one valid item"})
		return
	}

	billItems, subtotal := promotionLinesToBillItems(promotionResult.Lines)
	if subtotal < 0 {
		subtotal = 0
	}
	subtotalCents := money.CentsFromMajor(subtotal)
	taxCents, serviceFeeCents, totalCents := services.BillTotalsCents(subtotalCents, business.TaxRate, business.ServiceFeeRate)

	// Create bill (BillNumber left unset — createBillTx generates
	// B{businessID}-{uuid[:12]} when empty; public_token is the guest capability).
	bill := &database.Bill{
		BusinessID:       uint(businessID),
		CounterID:        req.CounterID,
		Notes:            req.Notes,
		Subtotal:         subtotalCents,
		TaxAmount:        taxCents,
		ServiceFeeAmount: serviceFeeCents,
		TotalAmount:      totalCents,
		Status:           database.BillStatusOpen,
		SettlementAddr:   business.SettlementAddr,
		TippingAddr:      business.TippingAddr,
		CreatedByStaffID: staffIDFromContext(c),
	}

	// Set TableID if provided
	if req.TableID != nil {
		bill.TableID = *req.TableID
	}

	historyEvents := []database.BillHistoryEvent{
		{
			BusinessID: bill.BusinessID,
			EventType:  database.BillHistoryEventBillCreated,
			Actor:      getBusinessActionActor(c),
			Details: map[string]interface{}{
				"items_count":   len(billItems),
				"subtotal":      float64(bill.Subtotal) / 100.0,
				"total_amount":  float64(bill.TotalAmount) / 100.0,
				"table_id":      bill.TableID,
				"counter_id":    bill.CounterID,
				"created_notes": strings.TrimSpace(req.Notes),
			},
		},
	}

	if err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := services.ValidateOperatorOrderableLinesTx(tx, business, categories, promotionResult.Lines); err != nil {
			return err
		}
		return database.CreateBillWithHistoryTx(tx, bill, billItems, historyEvents)
	}); err != nil {
		var unavailable *services.ItemNotOrderableError
		if errors.As(err, &unavailable) {
			RespondWithOrderabilityConflict(c, unavailable)
			return
		}
		var projectionErr *services.OperatorOrderabilityProjectionError
		if errors.As(err, &projectionErr) {
			RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Failed to validate item availability")
			return
		}
		var occupancyConflict *database.ActiveBillConflictError
		if errors.As(err, &occupancyConflict) {
			c.JSON(http.StatusConflict, gin.H{
				"error":          "Counter already has an active bill",
				"code":           "counter_occupied",
				"active_bill_id": occupancyConflict.BillID,
			})
			return
		}
		if errors.Is(err, database.ErrCounterNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Counter not found"})
			return
		}
		if errors.Is(err, database.ErrCounterBusinessMismatch) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Counter does not belong to this business"})
			return
		}
		if errors.Is(err, database.ErrCounterInactive) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Counter is not active"})
			return
		}
		// Lost the race (or pre-check gap): the partial unique index rejected a
		// second active bill for this table. Surface a clean 409 instead of a
		// raw 500 so the client can refetch the existing open bill.
		if errors.Is(err, database.ErrActiveBillExists) {
			c.JSON(http.StatusConflict, gin.H{"error": "Table already has an open bill"})
			return
		}
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not create bill")
		return
	}

	// Push SSE event for new bill — hydrate leftover $0 / year-1 children so
	// globalBills does not ingest the raw snapshot.
	if err := database.HydrateBillSnapshotForWire(bill); err != nil {
		log.Printf("bill.created snapshot hydrate failed: bill_id=%d error=%v", bill.ID, err)
		bill.Items = ""
	}
	events.GetHub().PublishJSON(uint(businessID), "bill.created", bill)
	_ = events.NotifyTableBillChangedByBillID(database.GetDB(), bill.ID)
	if err := operational_alerts.NewService(database.GetDB()).CreateBillNewAlert(c.Request.Context(), *bill); err != nil {
		log.Printf("failed to create bill operational alert: business_id=%d bill_id=%d error=%v", bill.BusinessID, bill.ID, err)
	}

	c.JSON(http.StatusCreated, gin.H{
		"bill":  bill,
		"items": billItems,
	})
}

func promotionLinesToBillItems(lines []database.OrderItem) ([]database.BillItem, float64) {
	billItems := make([]database.BillItem, 0, len(lines))
	var subtotalCents int64
	stamp := time.Now().UTC()
	for _, line := range lines {
		priceCents := money.CentsFromMajor(line.Price)
		lineSubtotalCents := money.CentsFromMajor(line.Subtotal)
		billItems = append(billItems, database.BillItem{
			ID:                 line.ID,
			MenuItemID:         line.MenuItemID,
			Name:               line.MenuItemName,
			Price:              money.MajorFromCents(priceCents),
			Quantity:           line.Quantity,
			Options:            line.Options,
			ItemType:           line.ItemType,
			BundleID:           line.BundleID,
			ParentBundleID:     line.ParentBundleID,
			BundleOccurrenceID: line.BundleOccurrenceID,
			SourceOfferID:      line.SourceOfferID,
			Subtotal:           money.MajorFromCents(lineSubtotalCents),
			CreatedAt:          stamp,
		})
		subtotalCents += lineSubtotalCents
	}
	return billItems, money.MajorFromCents(subtotalCents)
}

// GetBusinessBills retrieves all bills for a business
func GetBusinessBills(c *gin.Context) {
	business, ok := businessFromBusinessIdentifierParam(c)
	if !ok {
		return
	}
	businessID := business.ID

	// Access control is handled by HybridAuthenticationMiddleware + RoleBasedAccessMiddleware
	// Business access validation and permission checking is done in middleware

	filters, err := parseBillListFilters(c, businessID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := database.GetBillListRowsByBusinessIDFilteredPaginated(businessID, filters, parseDatabasePagination(c))
	if err != nil {
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not list bills")
		return
	}

	respondWithPaginatedBills(c, business, result)
}

// GetOpenBusinessBills retrieves active open or partial bills for a business.
func GetOpenBusinessBills(c *gin.Context) {
	business, ok := businessFromBusinessIdentifierParam(c)
	if !ok {
		return
	}
	businessID := business.ID

	// Access control is handled by HybridAuthenticationMiddleware + RoleBasedAccessMiddleware
	// Business access validation and permission checking is done in middleware

	filters, err := parseBillListFilters(c, businessID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := database.GetOpenBillListRowsByBusinessIDFilteredPaginated(businessID, filters, parseDatabasePagination(c))
	if err != nil {
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not list open bills")
		return
	}

	respondWithPaginatedBills(c, business, result)
}

func businessForBillRBAC(bill *database.Bill) (*database.Business, error) {
	if bill == nil {
		return nil, fmt.Errorf("bill not found")
	}
	if bill.Business.ID != 0 {
		return &bill.Business, nil
	}
	return database.GetBillBusinessAccessByBillID(bill.ID)
}

// GetBill retrieves a specific bill by ID
func GetBill(c *gin.Context) {
	billID, err := strconv.ParseUint(c.Param("bill_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	bill, items, ok := billFromRequestContext(c)
	if !ok {
		bill, items, err = database.GetBillByID(uint(billID))
		if err != nil || bill == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
			return
		}

		// RequireBillBusinessAccess already authorized and stashed the
		// decision-complete business. GetBillByID's Business preload omits
		// is_demo/kind, so a second CheckBusinessAccess on that dest 403s
		// platform admins who just passed the middleware (production #431).
		if _, authorized := c.Get("_business"); !authorized {
			business, berr := businessForBillRBAC(bill)
			if berr != nil || business == nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
				return
			}
			if !CheckBusinessAccess(c, business) {
				c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to access this bill"})
				return
			}
		}
	}

	// Optional history_limit: absent → legacy unbounded read; a positive value
	// returns the newest N events plus the true total so the modal can render a
	// bounded list with a "show older" affordance instead of every event on
	// every SSE-triggered re-fetch. Clamp to a sane ceiling.
	historyLimit := 0
	if raw := strings.TrimSpace(c.Query("history_limit")); raw != "" {
		if parsed, perr := strconv.Atoi(raw); perr == nil && parsed > 0 {
			historyLimit = parsed
			if historyLimit > 500 {
				historyLimit = 500
			}
		}
	}

	history, historyTotal, err := database.GetBillHistoryByBillIDLimited(uint(billID), historyLimit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load bill history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bill":          bill,
		"items":         items,
		"history":       history,
		"history_total": historyTotal,
	})
}

// UpdateBill updates an existing bill
func UpdateBill(c *gin.Context) {
	billID, err := strconv.ParseUint(c.Param("bill_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	var req UpdateBillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	bill, existingItems, err := database.GetBillByID(uint(billID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Verify business access (owner or staff with the right perm)
	business, err := businessForBillRBAC(bill)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if !CheckBusinessAccess(c, business) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this bill"})
		return
	}

	if bill.Status != database.BillStatusOpen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot modify closed bill"})
		return
	}

	beforeSubtotal := bill.Subtotal
	beforeTotal := bill.TotalAmount
	req.Items = preserveBundleOccurrences(req.Items, existingItems)

	// Normalize every line through the shared line-subtotal rule (options
	// included; bundle children stay $0), then ApplyBillTotals for tax/service.
	// The old Price×Qty rewrite dropped option surcharges and billed child
	// catalog prices — the $269 / $403.61 / $408.55 dinner-check drift.
	for i := range req.Items {
		database.NormalizeBillItemLineMoney(&req.Items[i])
	}
	database.ApplyBillTotals(bill, req.Items, business)
	subtotal := money.MajorFromCents(bill.Subtotal)
	totalAmount := money.MajorFromCents(bill.TotalAmount)

	historyEvents := []database.BillHistoryEvent{
		{
			BillID:     bill.ID,
			BusinessID: bill.BusinessID,
			EventType:  database.BillHistoryEventBillUpdated,
			Actor:      getBusinessActionActor(c),
			Details: map[string]interface{}{
				"items_before": len(existingItems),
				"items_after":  len(req.Items),
				// Money keys are dollars (same contract as CreateBill/CloseBill).
				"subtotal_before": float64(beforeSubtotal) / 100.0,
				"subtotal_after":  subtotal,
				"total_before":    float64(beforeTotal) / 100.0,
				"total_after":     totalAmount,
			},
		},
	}

	if err := database.UpdateBillWithHistory(bill, req.Items, historyEvents); err != nil {
		if errors.Is(err, database.ErrBillNotPayable) {
			RespondWithError(c, http.StatusConflict, ErrCodeConflict, "Bill is no longer open; refresh to load the latest bill")
			return
		}
		if errors.Is(err, database.ErrBillStale) {
			RespondWithError(c, http.StatusConflict, ErrCodeConflict, "Bill changed while you were editing; refresh and try again")
			return
		}
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not update bill")
		return
	}

	publishBillUpdatedEvent(bill)

	c.JSON(http.StatusOK, gin.H{
		"bill":  bill,
		"items": req.Items,
	})
}

func preserveBundleOccurrences(items, existingItems []database.BillItem) []database.BillItem {
	occurrenceByID := make(map[string]string, len(existingItems))
	for _, item := range existingItems {
		if item.BundleOccurrenceID != "" {
			occurrenceByID[item.ID] = item.BundleOccurrenceID
		}
	}
	for i := range items {
		if items[i].BundleOccurrenceID == "" {
			items[i].BundleOccurrenceID = occurrenceByID[items[i].ID]
		}
	}
	return items
}

// AddBillItem adds an item to an existing bill
func AddBillItem(c *gin.Context) {
	billID, err := strconv.ParseUint(c.Param("bill_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	var req AddBillItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	if req.Price == nil || *req.Price < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Price must be zero or greater"})
		return
	}
	if req.Quantity <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Quantity must be greater than zero"})
		return
	}

	bill, items, err := database.GetBillByID(uint(billID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Verify business access (owner or staff with the right perm)
	business, err := businessForBillRBAC(bill)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if !CheckBusinessAccess(c, business) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this bill"})
		return
	}

	if bill.Status != database.BillStatusOpen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot modify closed bill"})
		return
	}

	priceCents := money.CentsFromMajor(*req.Price)

	// Create new bill item — line subtotal includes option surcharges.
	newItem := database.BillItem{
		ID:         fmt.Sprintf("item_%d", time.Now().UnixNano()),
		MenuItemID: req.MenuItemID,
		Name:       req.Name,
		Price:      money.MajorFromCents(priceCents),
		Quantity:   req.Quantity,
		Options:    req.Options,
	}
	database.NormalizeBillItemLineMoney(&newItem)
	actor := getBusinessActionActor(c)

	// Append under the row lock against the bill's current items so a
	// concurrent add/approval is never dropped by our stale snapshot.
	updatedBill, items, err := database.UpdateBillWithHistoryFn(bill.ID, func(locked *database.Bill, current []database.BillItem) ([]database.BillItem, []database.BillHistoryEvent, error) {
		beforeSubtotal := locked.Subtotal
		beforeTotal := locked.TotalAmount
		next := append(current, newItem)
		// Single-source tax/service/total (keeps loyalty discount net).
		database.ApplyBillTotals(locked, next, business)
		return next, []database.BillHistoryEvent{
			{
				BillID:     locked.ID,
				BusinessID: locked.BusinessID,
				EventType:  database.BillHistoryEventItemAdded,
				Actor:      actor,
				BillItemID: newItem.ID,
				ItemName:   newItem.Name,
				Details: map[string]interface{}{
					"quantity":   newItem.Quantity,
					"unit_price": newItem.Price,
					"subtotal":   newItem.Subtotal,
					// Money keys are dollars (same contract as CreateBill/CloseBill).
					"subtotal_before": float64(beforeSubtotal) / 100.0,
					"subtotal_after":  money.MajorFromCents(locked.Subtotal),
					"total_before":    float64(beforeTotal) / 100.0,
					"total_after":     money.MajorFromCents(locked.TotalAmount),
				},
			},
		}, nil
	})
	if err != nil {
		if errors.Is(err, database.ErrBillNotPayable) {
			RespondWithError(c, http.StatusConflict, ErrCodeConflict, "Bill is no longer open; refresh to load the latest bill")
			return
		}
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not add bill item")
		return
	}

	// Keep the preloaded relations of the response bill; take the columns the
	// locked write owns (and the fresh payment state) from the locked row.
	bill.Subtotal = updatedBill.Subtotal
	bill.TaxAmount = updatedBill.TaxAmount
	bill.ServiceFeeAmount = updatedBill.ServiceFeeAmount
	bill.TotalAmount = updatedBill.TotalAmount
	bill.Items = updatedBill.Items
	bill.UpdatedAt = updatedBill.UpdatedAt
	bill.PaidAmount = updatedBill.PaidAmount
	bill.Status = updatedBill.Status
	publishBillUpdatedEvent(bill)

	c.JSON(http.StatusOK, gin.H{
		"bill":  bill,
		"items": items,
	})
}

// AdjustBillItem updates a bill item quantity or voids the line with a reason.
func AdjustBillItem(c *gin.Context) {
	billID, err := strconv.ParseUint(c.Param("bill_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	itemID := strings.TrimSpace(c.Param("item_id"))
	if itemID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Item ID is required"})
		return
	}

	var req AdjustBillItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	bill, _, err := database.GetBillByID(uint(billID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	business, err := businessForBillRBAC(bill)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if !CheckBusinessAccess(c, business) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this bill"})
		return
	}

	req.Reason = strings.TrimSpace(req.Reason)
	switch {
	case req.Void && req.Quantity != nil:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot void an item and update quantity in the same request"})
		return
	case req.Void && req.Reason == "":
		c.JSON(http.StatusBadRequest, gin.H{"error": "Void reason is required"})
		return
	case !req.Void && req.Quantity == nil:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Quantity is required"})
		return
	case !req.Void && req.Quantity != nil && *req.Quantity <= 0:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Quantity must be greater than zero"})
		return
	}

	updatedBill, updatedItems, err := database.AdjustBillItem(
		uint(billID),
		itemID,
		getBusinessActionActor(c),
		req.Quantity,
		req.Void,
		req.Reason,
	)
	if err != nil {
		switch {
		case errors.Is(err, database.ErrBillNotPayable):
			c.JSON(http.StatusConflict, gin.H{"error": "Cannot modify closed bill"})
		case strings.Contains(err.Error(), "not found"):
			c.JSON(http.StatusNotFound, gin.H{"error": "Bill item not found"})
		case strings.Contains(err.Error(), "cannot modify closed bill"),
			strings.Contains(err.Error(), "required"),
			strings.Contains(err.Error(), "greater than zero"),
			strings.Contains(err.Error(), "cannot void"):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not adjust bill item")
		}
		return
	}

	publishBillUpdatedEvent(updatedBill)

	c.JSON(http.StatusOK, gin.H{
		"bill":  updatedBill,
		"items": updatedItems,
	})
}

// CloseBill closes a bill
func CloseBill(c *gin.Context) {
	billID, err := strconv.ParseUint(c.Param("bill_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	bill, items, err := database.GetBillByID(uint(billID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Verify business access (owner or staff with the right perm)
	business, err := businessForBillRBAC(bill)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	if !CheckBusinessAccess(c, business) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Not authorized to modify this bill"})
		return
	}

	if bill.Status != database.BillStatusOpen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Bill is already closed"})
		return
	}

	remainingCents := bill.TotalAmount - bill.PaidAmount
	if remainingCents > 0 {
		// Operator close of an unpaid check is not settlement (#704).
		c.JSON(http.StatusConflict, gin.H{"error": "Bill has unpaid remaining", "code": "bill_unpaid_remaining"})
		return
	}

	historyEvents := []database.BillHistoryEvent{
		{
			BillID:     bill.ID,
			BusinessID: bill.BusinessID,
			EventType:  database.BillHistoryEventBillClosed,
			Actor:      getBusinessActionActor(c),
			Details: map[string]interface{}{
				"open_items_count": len(items),
				"subtotal":         float64(bill.Subtotal) / 100.0,
				"total_amount":     float64(bill.TotalAmount) / 100.0,
				"paid_amount":      float64(bill.PaidAmount) / 100.0,
			},
		},
	}

	if err := database.CloseBillWithHistory(uint(billID), historyEvents); err != nil {
		if errors.Is(err, database.ErrBillNotOpen) {
			c.JSON(http.StatusConflict, gin.H{"error": "Bill is not open"})
			return
		}
		if errors.Is(err, database.ErrBillHasLiveKitchenTickets) {
			c.JSON(http.StatusConflict, gin.H{"error": "Bill has live kitchen tickets", "code": "bill_live_kitchen"})
			return
		}
		if errors.Is(err, database.ErrBillHasUnpaidRemaining) {
			c.JSON(http.StatusConflict, gin.H{"error": "Bill has unpaid remaining", "code": "bill_unpaid_remaining"})
			return
		}
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not close bill")
		return
	}

	// Attribute the closure to the staff member who initiated it. Owners
	// closing bills leave this nil — that's the intended semantic for the
	// analytics aggregate, which only counts staff-driven closures.
	StampBillClosedByStaff(c, uint(billID))

	// A freshly-closed bill may have been one of the "stale open bills" that
	// drove an insight — refresh the cache so the briefing clears on next poll.
	invalidateOwnerHomeCaches(bill.BusinessID)

	// Get updated bill
	updatedBill, _, err := database.GetBillByID(uint(billID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve updated bill"})
		return
	}

	// Push SSE event for bill closed
	events.GetHub().PublishJSON(updatedBill.BusinessID, "bill.closed", gin.H{"bill_id": updatedBill.ID})
	_ = events.NotifyTableBillChangedByBillID(database.GetDB(), updatedBill.ID)
	resolveBillOperationalAlert(c, updatedBill, string(updatedBill.Status))

	c.JSON(http.StatusOK, gin.H{
		"bill":  updatedBill,
		"items": items,
	})
}
