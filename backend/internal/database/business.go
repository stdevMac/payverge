package database

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/money"
	"github.com/stdevmac/payverge/backend/internal/txhash"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PaginationParams holds pagination parameters for list queries.
type PaginationParams struct {
	Page     int
	PageSize int
}

// OrderListOptions carries optional knobs for the paginated order list. The
// zero value lists all bills, newest first.
type OrderListOptions struct {
	// ActiveBillsOnly restricts to orders whose bill is still open/partial,
	// except kitchen-pipeline tickets (approved / in_kitchen / ready) which
	// stay visible even after the check is closed ($0 comps, already-paid).
	ActiveBillsOnly bool
	// SortAsc orders by created_at ASC (oldest first). The kitchen board sets
	// this so a row cap drops the newest orders, not the oldest FIFO ones.
	SortAsc bool
}

// KitchenLiveOrderStatuses are tickets expo still has to cook/plate even if
// the linked bill is already closed. Pending/delivered/cancelled are not in
// this set: pending-on-closed stays hidden; terminal tickets are history.
func KitchenLiveOrderStatuses() []OrderStatus {
	return []OrderStatus{OrderStatusApproved, OrderStatusInKitchen, OrderStatusOrderReady}
}

// PaginatedResult wraps a query result with pagination metadata.
type PaginatedResult[T any] struct {
	Data       []T   `json:"data"`
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalPages int   `json:"total_pages"`
}

// BillListFilters contains optional server-side filters for crowded bill lists.
// BillListFilters contains optional server-side filters for crowded bill lists.
type BillListFilters struct {
	Status      string
	Search      string
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	// CustomerID filters to bills linked to a CRM customer (crm_customer_id,
	// indexed). Used by the CRM → bills drill-in.
	CustomerID *uint
}

// BillListRow is the narrow read model for operator bill lists. It avoids
// hydrating the full Bill aggregate and relation graph for list endpoints.
type BillListRow struct {
	ID         uint   `gorm:"column:id" json:"id"`
	BusinessID uint   `gorm:"column:business_id" json:"business_id"`
	TableID    uint   `gorm:"column:table_id" json:"table_id"`
	CounterID  *uint  `gorm:"column:counter_id" json:"counter_id"`
	TableName  string `gorm:"column:table_name" json:"table_name,omitempty"`
	BillNumber string `gorm:"column:bill_number" json:"bill_number"`
	Notes      string `gorm:"column:notes" json:"notes"`
	Items      string `gorm:"column:items" json:"items"`
	// ItemCount counts bill_items rows (the live item store). The legacy
	// Items JSON snapshot is empty for those bills, so list consumers must
	// read this instead of parsing Items (audit G-03); fall back to the
	// snapshot only when this is 0 (pre-bill_items bills).
	ItemCount            int64      `gorm:"column:item_count" json:"item_count"`
	PhysicalItemQuantity int64      `gorm:"column:physical_item_quantity" json:"physical_item_quantity"`
	Subtotal             int64      `gorm:"column:subtotal" json:"subtotal"`
	TaxAmount            int64      `gorm:"column:tax_amount" json:"tax_amount"`
	ServiceFeeAmount     int64      `gorm:"column:service_fee_amount" json:"service_fee_amount"`
	TotalAmount          int64      `gorm:"column:total_amount" json:"total_amount"`
	PaidAmount           int64      `gorm:"column:paid_amount" json:"paid_amount"`
	TipAmount            int64      `gorm:"column:tip_amount" json:"tip_amount"`
	Status               BillStatus `gorm:"column:status" json:"status"`
	// Settlement/tipping wallets are not part of the list projection (FIND-045);
	// bill detail / guest pay routes load them from the full Bill row.
	CreatedByStaffID    *uint      `gorm:"column:created_by_staff_id" json:"created_by_staff_id,omitempty"`
	ClosedByStaffID     *uint      `gorm:"column:closed_by_staff_id" json:"closed_by_staff_id,omitempty"`
	CRMCustomerID       *uint      `gorm:"column:crm_customer_id" json:"crm_customer_id,omitempty"`
	CreatedAt           time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at" json:"updated_at"`
	ClosedAt            *time.Time `gorm:"column:closed_at" json:"closed_at"`
	FeedbackEmailSentAt *time.Time `gorm:"column:feedback_email_sent_at" json:"feedback_email_sent_at,omitempty"`
}

// DefaultPagination returns default pagination params (page 1, size 20).
func DefaultPagination() PaginationParams {
	return PaginationParams{Page: 1, PageSize: 20}
}

// Offset returns the SQL offset for the current page.
func (p PaginationParams) Offset() int {
	return (p.Page - 1) * p.PageSize
}

// Normalize clamps page/pageSize to valid ranges.
func (p PaginationParams) Normalize() PaginationParams {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 {
		p.PageSize = 20
	}
	if p.PageSize > 100 {
		p.PageSize = 100
	}
	return p
}

// Business operations

func applyCreateBusinessDefaults(business *Business) {
	if business.DefaultCurrency == "" {
		business.DefaultCurrency = "USD"
	}
	if business.DefaultLanguage == "" {
		business.DefaultLanguage = "en"
	}
}

// assignCreateCustomURLTx gives a new venue without a slug one derived from
// its name, so the storefront is routable at /b/<slug> (and at "/") as soon
// as it is published. A slug the caller chose is kept as is.
func assignCreateCustomURLTx(tx *gorm.DB, business *Business) error {
	if strings.TrimSpace(business.CustomURL) != "" {
		return nil
	}
	slug, err := AllocatePublicCustomURL(context.Background(), tx, business.Name, 0)
	if err != nil {
		return fmt.Errorf("failed to allocate custom URL: %w", err)
	}
	business.CustomURL = slug
	return nil
}

// CreateBusiness creates a new business in the database
func CreateBusiness(business *Business) error {
	applyCreateBusinessDefaults(business)

	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := assignCreateCustomURLTx(tx, business); err != nil {
			return err
		}
		if err := tx.Create(business).Error; err != nil {
			return fmt.Errorf("failed to create business: %w", err)
		}
		if err := ensureBusinessRevenueAggregateRowTx(tx, business.ID); err != nil {
			return fmt.Errorf("failed to create business revenue aggregate: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

// GetBusinessByID retrieves a business by its ID
func GetBusinessByID(id uint) (*Business, error) {
	var business Business
	if err := db.First(&business, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("business not found")
		}
		return nil, fmt.Errorf("failed to get business: %w", err)
	}

	return &business, nil
}

// GetBusinessByBusinessId retrieves a business by its BusinessId string
func GetBusinessByBusinessId(businessId string) (*Business, error) {
	var business Business
	if err := db.Where("business_id = ? AND is_active = ?", businessId, true).First(&business).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("business not found")
		}
		return nil, fmt.Errorf("failed to get business: %w", err)
	}
	return &business, nil
}

// IsRecordNotFound reports whether err signals a missing row (gorm.ErrRecordNotFound),
// letting service-layer callers distinguish "not found" from a transient DB failure
// without importing gorm directly.
func IsRecordNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

// GetBusinessByIdOrBusinessId resolves a business route parameter, which
// clients send either as the numeric ID or as the string businessId.
func GetBusinessByIdOrBusinessId(identifier string) (*Business, error) {
	if id, err := strconv.ParseUint(identifier, 10, 32); err == nil {
		return GetBusinessByID(uint(id))
	}
	// If not numeric, treat as businessId string
	return GetBusinessByBusinessId(identifier)
}

// businessAuthScopeColumns is the minimal Business projection that the
// authentication middleware needs when it stashes the resolved business under
// the gin "_business" context key (HybridAuthenticationMiddleware). It loads
// only the identity/ownership columns the middleware + printer handler read off
// the context value, plus every column the operational gate
// (businessFromContext → IsBusinessOperational) reads off it.
//
// DRIFT GUARD: IsBusinessOperational reads is_demo, is_active and closed_at —
// all MUST stay in this list. The middleware paths read id/owner_address/user_id;
// the printer handler reads id. If a new
// consumer reads another field off the *context* business value (not a
// re-fetched full row), add its column here. Polling/handler code that
// re-fetches a full row by ID (resolveBusinessFromIDParam, requireBusinessAccess,
// GetBusiness, etc.) is unaffected by this projection. Over-projecting a scalar
// is safe; the carried-but-unread columns (business_id slug, name) are kept for
// identity.
var businessAuthScopeColumns = []string{
	"id", "business_id", "name", "owner_address", "user_id", "demo_owner_user_id",
	"is_demo", "kind",
	"is_active", "closed_at",
}

// GetBusinessAuthScopeByIdOrBusinessId is a column-narrowed sibling of
// GetBusinessByIdOrBusinessId for the auth middleware hot path. It resolves the
// business by the SAME rules — numeric identifier → by primary key (no is_active
// filter, matching GetBusinessByID); non-numeric → by business_id slug with
// is_active = true (matching GetBusinessByBusinessId) — but projects only
// businessAuthScopeColumns instead of hydrating the full ~94-column row
// (jsonb onboarding_state, text blobs, all Stripe IDs) on every authenticated
// business-scoped request.
func GetBusinessAuthScopeByIdOrBusinessId(identifier string) (*Business, error) {
	var business Business
	if id, err := strconv.ParseUint(identifier, 10, 32); err == nil {
		// Numeric path mirrors GetBusinessByID: lookup by primary key only.
		if err := db.Select(businessAuthScopeColumns).First(&business, uint(id)).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("business not found")
			}
			return nil, fmt.Errorf("failed to get business: %w", err)
		}
		return &business, nil
	}
	// Slug path mirrors GetBusinessByBusinessId: business_id + is_active = true.
	if err := db.Select(businessAuthScopeColumns).
		Where("business_id = ? AND is_active = ?", identifier, true).
		First(&business).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("business not found")
		}
		return nil, fmt.Errorf("failed to get business: %w", err)
	}
	return &business, nil
}

// GetBusinessByOwnerAddress retrieves businesses owned by a specific address
func GetBusinessByOwnerAddress(ownerAddress string) ([]Business, error) {
	var businesses []Business
	if err := db.Where("owner_address = ? AND is_active = ?", ownerAddress, true).Find(&businesses).Error; err != nil {
		return nil, fmt.Errorf("failed to get businesses: %w", err)
	}
	return businesses, nil
}

// ListBusinessesForInsideUser is the SQL shape of the list/open rule used by
// GET /inside/businesses. Owners and demo owners see their own venues.
// Platform admins also see every active demo (admin read / impersonate) so
// Open business and GET /inside/businesses/:id share one set.
func ListBusinessesForInsideUser(userID uint, platformAdmin bool) ([]Business, error) {
	var businesses []Business
	q := db.Where("is_active = ? AND (user_id = ? OR demo_owner_user_id = ?)", true, userID, userID)
	if platformAdmin {
		q = db.Where("is_active = ? AND (user_id = ? OR demo_owner_user_id = ? OR is_demo = ?)", true, userID, userID, true)
	}
	if err := q.Find(&businesses).Error; err != nil {
		return nil, fmt.Errorf("failed to get businesses: %w", err)
	}
	return businesses, nil
}

// AdminLocked reports whether a business is locked by an explicit admin
// enforcement action — suspension (is_active=false) or account closure
// (closed_at set) — as opposed to a billing lapse. Admin locks must not be
// reversible through self-service payment flows.
func AdminLocked(business *Business) bool {
	if business == nil {
		return false
	}
	return !business.IsActive || business.ClosedAt != nil
}

// UpdateBusiness updates an existing business
func UpdateBusiness(business *Business) error {
	if err := db.Save(business).Error; err != nil {
		return fmt.Errorf("failed to update business: %w", err)
	}
	return nil
}

// businessDesignColumns are the embedded design_* columns (BusinessDesignSettings
// with gorm embeddedPrefix "design_"). They are listed explicitly so the general
// business update can Omit them and the design-settings update can scope to them —
// eliminating the lost-update race when both run concurrently (DI-1).
var businessDesignColumns = []string{
	"design_primary_color",
	"design_secondary_color",
	"design_font_family",
	"design_theme",
	"design_menu_layout",
	"design_show_images",
	"design_show_descriptions",
	"design_header_style",
	"design_corner_radius",
	"design_shadow_intensity",
	"design_background_pattern",
	"design_pattern_opacity",
	"design_hero_layout",
	"design_section_density",
}

// UpdateBusinessExceptDesign persists all business columns (including zero values)
// EXCEPT the design_* block, so it cannot clobber a concurrent design-settings
// write. GetBusinessByID loads without association preloads, so the nil
// associations are untouched — matching the prior db.Save behavior.
func UpdateBusinessExceptDesign(business *Business) error {
	return UpdateBusinessExceptDesignRotatingWallets(business, false, false)
}

// UpdateBusinessExceptDesignRotatingWallets is UpdateBusinessExceptDesign plus
// the payout-wallet rotation that must land in the same transaction. Open and
// partial bills copy settlement_addr and tipping_addr at open time; when an
// owner rotates a wallet those rows have to follow, and any still-active
// crypto quote was signed for the old recipient so it is expired.
func UpdateBusinessExceptDesignRotatingWallets(business *Business, settlementChanged, tippingChanged bool) error {
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(business).Select("*").Omit(businessDesignColumns...).Updates(business).Error; err != nil {
			return err
		}
		openStatuses := []BillStatus{BillStatusOpen, BillStatusPartial}
		if settlementChanged {
			if err := tx.Model(&Bill{}).
				Where("business_id = ? AND status IN ?", business.ID, openStatuses).
				Update("settlement_addr", business.SettlementAddr).Error; err != nil {
				return err
			}
		}
		if tippingChanged {
			if err := tx.Model(&Bill{}).
				Where("business_id = ? AND status IN ?", business.ID, openStatuses).
				Update("tipping_addr", business.TippingAddr).Error; err != nil {
				return err
			}
		}
		if settlementChanged || tippingChanged {
			if err := tx.Model(&CryptoPaymentQuote{}).
				Where("business_id = ? AND status = ?", business.ID, CryptoPaymentQuoteStatusActive).
				Updates(map[string]any{"status": CryptoPaymentQuoteStatusExpired, "client_key": nil, "updated_at": time.Now()}).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to update business: %w", err)
	}
	return nil
}

// UpdateBusinessDesignOnly persists only the design_* columns.
func UpdateBusinessDesignOnly(business *Business) error {
	if err := db.Model(business).Select(businessDesignColumns).Updates(business).Error; err != nil {
		return fmt.Errorf("failed to update design settings: %w", err)
	}
	return nil
}

// DeleteBusiness soft deletes a business and releases its workspace-creation
// replay ledger. Keeping that ledger after an owner explicitly abandons a
// workspace would make the original idempotency key replay the now-inactive
// tenant forever instead of allowing a deliberate fresh attempt.
func DeleteBusiness(id uint) error {
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Business{}).Where("id = ?", id).Update("is_active", false).Error; err != nil {
			return err
		}
		return tx.Where("business_id = ?", id).Delete(&BusinessCreationRequest{}).Error
	}); err != nil {
		return fmt.Errorf("failed to delete business: %w", err)
	}
	return nil
}

// GetBusinessDefaults returns the default currency and language for a business
func GetBusinessDefaults(businessID uint) (string, string, error) {
	var business Business
	if err := db.Select("default_currency, default_language").Where("id = ?", businessID).First(&business).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", fmt.Errorf("business not found")
		}
		return "", "", fmt.Errorf("failed to get business defaults: %w", err)
	}
	return business.DefaultCurrency, business.DefaultLanguage, nil
}

// Menu operations

// CreateMenu creates a new menu for a business
func CreateMenu(menu *Menu, categories []MenuCategory) error {
	ensureMenuEntityIDs(categories)
	// Convert categories to JSON string for database storage
	categoriesJSON, err := json.Marshal(categories)
	if err != nil {
		return fmt.Errorf("failed to marshal categories: %w", err)
	}
	menu.Categories = string(categoriesJSON)

	if err := db.Create(menu).Error; err != nil {
		return fmt.Errorf("failed to create menu: %w", err)
	}
	return nil
}

// GetMenuByBusinessID retrieves the active menu for a business
func GetMenuByBusinessID(businessID uint) (*Menu, []MenuCategory, error) {
	var menu Menu
	if err := db.Where("business_id = ? AND is_active = ?", businessID, true).First(&menu).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, fmt.Errorf("menu not found: %w", gorm.ErrRecordNotFound)
		}
		return nil, nil, fmt.Errorf("failed to get menu: %w", err)
	}

	// Parse categories from JSON
	var categories []MenuCategory
	if menu.Categories != "" {
		if err := json.Unmarshal([]byte(menu.Categories), &categories); err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal categories: %w", err)
		}
	}

	return &menu, categories, nil
}

// UpdateMenu updates an existing menu and increments the version
func UpdateMenu(menu *Menu, categories []MenuCategory) error {
	ensureMenuEntityIDs(categories)
	// Convert categories to JSON string for database storage
	categoriesJSON, err := json.Marshal(categories)
	if err != nil {
		return fmt.Errorf("failed to marshal categories: %w", err)
	}
	menu.Categories = string(categoriesJSON)
	menu.Version++

	if err := db.Omit(clause.Associations).Save(menu).Error; err != nil {
		return fmt.Errorf("failed to update menu: %w", err)
	}
	return nil
}

// ErrMenuVersionConflict is returned when optimistic locking detects a concurrent modification
var ErrMenuVersionConflict = fmt.Errorf("menu version conflict")

// ErrBusinessNotFound is returned when a business lookup misses.
var ErrBusinessNotFound = fmt.Errorf("business not found")

// ErrCategoryNotFound is returned when a category ID is not found in the menu
var ErrCategoryNotFound = fmt.Errorf("category not found")

// ErrItemNotFound is returned when an item ID is not found in the category
var ErrItemNotFound = fmt.Errorf("item not found")

// updateMenuWithVersionCheck performs an optimistic-lock update: it only writes if the
// current DB version matches expectedVersion, then bumps the version atomically.
// ensureMenuEntityIDs runs before the write so any blank category/item ID is
// server-assigned; callers that need the assigned IDs (mutation echo, §3.7 fix 3)
// read them off the categories slice after this returns.
func updateMenuWithVersionCheck(menu *Menu, categories []MenuCategory, expectedVersion uint) error {
	ensureMenuEntityIDs(categories)
	categoriesJSON, err := json.Marshal(categories)
	if err != nil {
		return fmt.Errorf("failed to marshal categories: %w", err)
	}

	result := db.Model(&Menu{}).
		Where("id = ? AND version = ?", menu.ID, expectedVersion).
		Updates(map[string]interface{}{
			"categories": string(categoriesJSON),
			"version":    expectedVersion + 1,
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update menu: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrMenuVersionConflict
	}

	// Update the in-memory struct to reflect the new state
	menu.Categories = string(categoriesJSON)
	menu.Version = expectedVersion + 1
	return nil
}

// mutateMenuUnderCAS reads the active menu, lets mutate() transform its
// categories in memory, and applies the result under optimistic CAS with bounded
// retry. Gives the version-less legacy item routes the same atomicity as the
// ID-based ones: a concurrent writer's change can no longer be clobbered by a
// blind save. mutate() must return a NEW slice (or the same one mutated) plus any
// validation error (e.g. an out-of-range sentinel), which is surfaced unchanged.
func mutateMenuUnderCAS(businessID uint, mutate func([]MenuCategory) ([]MenuCategory, error)) (uint, error) {
	const maxAttempts = 5
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		menu, cats, err := GetMenuByBusinessID(businessID)
		if err != nil {
			return 0, err
		}
		next, mErr := mutate(cats)
		if mErr != nil {
			return 0, mErr
		}
		v, aErr := ApplyMenuCategories(businessID, next, menu.Version)
		if aErr == nil {
			return v, nil
		}
		if errors.Is(aErr, ErrMenuVersionConflict) {
			lastErr = aErr
			continue
		}
		return 0, aErr
	}
	return 0, lastErr
}

// findCategoryByID returns the index of a category with the given ID, or -1 if not found.
func findCategoryByID(categories []MenuCategory, categoryID string) int {
	for i, cat := range categories {
		if cat.ID == categoryID {
			return i
		}
	}
	return -1
}

// findItemByID returns the index of an item with the given ID within a category, or -1 if not found.
func findItemByID(items []MenuItem, itemID string) int {
	for i, item := range items {
		if item.ID == itemID {
			return i
		}
	}
	return -1
}

// ErrReorderOutOfRange is returned when a reorder from/to index is invalid.
var ErrReorderOutOfRange = fmt.Errorf("reorder index out of range")

// moveInPlace moves the element at position from to position to within [0,len).
func moveIndex(length, from, to int) bool {
	return from >= 0 && from < length && to >= 0 && to < length
}

// ReorderMenuCategory moves a category from index `from` to index `to` under
// optimistic CAS. This is the granular alternative to re-uploading the whole
// menu on a drag (§3.7 fix 4): the server applies the single move to the stored
// tree, so the request carries only {from, to, version} — not the entire menu.
// Returns the bumped version.
func ReorderMenuCategory(businessID uint, from, to int, expectedVersion uint) (uint, error) {
	menu, categories, err := GetMenuByBusinessID(businessID)
	if err != nil {
		return 0, fmt.Errorf("failed to get menu: %w", err)
	}
	if !moveIndex(len(categories), from, to) {
		return 0, ErrReorderOutOfRange
	}
	moving := categories[from]
	categories = append(categories[:from], categories[from+1:]...)
	// Re-insert at the target index.
	rest := make([]MenuCategory, 0, len(categories)+1)
	rest = append(rest, categories[:to]...)
	rest = append(rest, moving)
	rest = append(rest, categories[to:]...)
	if err := updateMenuWithVersionCheck(menu, rest, expectedVersion); err != nil {
		return 0, err
	}
	return menu.Version, nil
}

// ReorderMenuItem moves an item within the category identified by categoryID
// from index `from` to index `to` under optimistic CAS. Returns the bumped
// version. Cross-category moves are not supported here (the drag UI only
// reorders within a category).
func ReorderMenuItem(businessID uint, categoryID string, from, to int, expectedVersion uint) (uint, error) {
	menu, categories, err := GetMenuByBusinessID(businessID)
	if err != nil {
		return 0, fmt.Errorf("failed to get menu: %w", err)
	}
	catIdx := findCategoryByID(categories, categoryID)
	if catIdx == -1 {
		return 0, ErrCategoryNotFound
	}
	items := categories[catIdx].Items
	if !moveIndex(len(items), from, to) {
		return 0, ErrReorderOutOfRange
	}
	moving := items[from]
	items = append(items[:from], items[from+1:]...)
	rest := make([]MenuItem, 0, len(items)+1)
	rest = append(rest, items[:to]...)
	rest = append(rest, moving)
	rest = append(rest, items[to:]...)
	categories[catIdx].Items = rest
	if err := updateMenuWithVersionCheck(menu, categories, expectedVersion); err != nil {
		return 0, err
	}
	return menu.Version, nil
}

// UpdateMenuCategoryByID updates a category identified by UUID with optimistic
// version checking. Returns the bumped menu version, the mutated category, and
// its index so the handler can echo it + key the async translate goroutine
// without a post-write re-read (§3.7 fixes 3+8).
func UpdateMenuCategoryByID(businessID uint, categoryID string, updatedCategory MenuCategory, expectedVersion uint) (uint, *MenuCategory, int, error) {
	menu, categories, err := GetMenuByBusinessID(businessID)
	if err != nil {
		return 0, nil, 0, fmt.Errorf("failed to get menu: %w", err)
	}

	idx := findCategoryByID(categories, categoryID)
	if idx == -1 {
		return 0, nil, 0, ErrCategoryNotFound
	}

	// Preserve the ID from the existing category
	updatedCategory.ID = categoryID
	categories[idx] = updatedCategory

	if err := updateMenuWithVersionCheck(menu, categories, expectedVersion); err != nil {
		return 0, nil, 0, err
	}
	mutated := categories[idx]
	return menu.Version, &mutated, idx, nil
}

// DeleteMenuCategoryByID removes a category identified by UUID with optimistic
// version checking. Returns the bumped version.
func DeleteMenuCategoryByID(businessID uint, categoryID string, expectedVersion uint) (uint, error) {
	menu, categories, err := GetMenuByBusinessID(businessID)
	if err != nil {
		return 0, fmt.Errorf("failed to get menu: %w", err)
	}

	idx := findCategoryByID(categories, categoryID)
	if idx == -1 {
		return 0, ErrCategoryNotFound
	}

	categories = append(categories[:idx], categories[idx+1:]...)

	if err := updateMenuWithVersionCheck(menu, categories, expectedVersion); err != nil {
		return 0, err
	}
	return menu.Version, nil
}

// AddMenuItemByID adds an item to a category identified by UUID with optimistic
// version checking. Returns the bumped version, the created item (with its
// server-assigned ID), and the (categoryIndex, itemIndex) position so the async
// translate goroutine can key on it without a post-write re-read (§3.7 fixes 3+8).
func AddMenuItemByID(businessID uint, categoryID string, item MenuItem, expectedVersion uint) (uint, *MenuItem, int, int, error) {
	menu, categories, err := GetMenuByBusinessID(businessID)
	if err != nil {
		return 0, nil, 0, 0, fmt.Errorf("failed to get menu: %w", err)
	}

	idx := findCategoryByID(categories, categoryID)
	if idx == -1 {
		return 0, nil, 0, 0, ErrCategoryNotFound
	}

	categories[idx].Items = append(categories[idx].Items, item)
	newItemIdx := len(categories[idx].Items) - 1

	if err := updateMenuWithVersionCheck(menu, categories, expectedVersion); err != nil {
		return 0, nil, 0, 0, err
	}
	// ensureMenuEntityIDs (inside updateMenuWithVersionCheck) assigned the UUID.
	created := categories[idx].Items[newItemIdx]
	return menu.Version, &created, idx, newItemIdx, nil
}

// UpdateMenuItemByID updates an item identified by UUID within a category
// identified by UUID. Returns the bumped version, the mutated item, and its
// (categoryIndex, itemIndex) position so the async translate goroutine can key
// on it without a post-write re-read (§3.7 fixes 3+8).
func UpdateMenuItemByID(businessID uint, categoryID string, itemID string, updatedItem MenuItem, expectedVersion uint) (uint, *MenuItem, int, int, error) {
	menu, categories, err := GetMenuByBusinessID(businessID)
	if err != nil {
		return 0, nil, 0, 0, fmt.Errorf("failed to get menu: %w", err)
	}

	catIdx := findCategoryByID(categories, categoryID)
	if catIdx == -1 {
		return 0, nil, 0, 0, ErrCategoryNotFound
	}

	itemIdx := findItemByID(categories[catIdx].Items, itemID)
	if itemIdx == -1 {
		return 0, nil, 0, 0, ErrItemNotFound
	}

	// Preserve the ID
	updatedItem.ID = itemID
	categories[catIdx].Items[itemIdx] = updatedItem

	if err := updateMenuWithVersionCheck(menu, categories, expectedVersion); err != nil {
		return 0, nil, 0, 0, err
	}
	mutated := categories[catIdx].Items[itemIdx]
	return menu.Version, &mutated, catIdx, itemIdx, nil
}

// DeleteMenuItemByID removes an item identified by UUID from a category
// identified by UUID. Returns the bumped version.
func DeleteMenuItemByID(businessID uint, categoryID string, itemID string, expectedVersion uint) (uint, error) {
	menu, categories, err := GetMenuByBusinessID(businessID)
	if err != nil {
		return 0, fmt.Errorf("failed to get menu: %w", err)
	}

	catIdx := findCategoryByID(categories, categoryID)
	if catIdx == -1 {
		return 0, ErrCategoryNotFound
	}

	itemIdx := findItemByID(categories[catIdx].Items, itemID)
	if itemIdx == -1 {
		return 0, ErrItemNotFound
	}

	items := categories[catIdx].Items
	categories[catIdx].Items = append(items[:itemIdx], items[itemIdx+1:]...)

	if err := updateMenuWithVersionCheck(menu, categories, expectedVersion); err != nil {
		return 0, err
	}
	return menu.Version, nil
}

// AddMenuCategoryVersioned adds a new category with optimistic version checking.
// Returns the bumped version, the created category (with its server-assigned
// ID), and its index so the async translate goroutine can key on it without a
// post-write re-read. When no menu exists yet it creates one (version 1).
func AddMenuCategoryVersioned(businessID uint, category MenuCategory, expectedVersion uint) (uint, *MenuCategory, int, error) {
	menu, categories, err := GetMenuByBusinessID(businessID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			newMenu := &Menu{
				BusinessID: businessID,
				Categories: "",
				IsActive:   true,
				Version:    1,
			}
			created := []MenuCategory{category}
			if cErr := CreateMenu(newMenu, created); cErr != nil {
				return 0, nil, 0, cErr
			}
			mutated := created[0]
			return newMenu.Version, &mutated, 0, nil
		}
		return 0, nil, 0, fmt.Errorf("failed to get menu: %w", err)
	}

	categories = append(categories, category)
	newCatIdx := len(categories) - 1

	if err := updateMenuWithVersionCheck(menu, categories, expectedVersion); err != nil {
		return 0, nil, 0, err
	}
	created := categories[newCatIdx]
	return menu.Version, &created, newCatIdx, nil
}

// AddMenuCategory adds a new category to an existing menu, creating the menu if
// it doesn't exist. Returns the bumped version, the created category (with its
// server-assigned ID), and its index so the handler can echo + key the async
// translate goroutine without a post-write re-read.
func AddMenuCategory(businessID uint, category MenuCategory) (uint, *MenuCategory, int, error) {
	menu, categories, err := GetMenuByBusinessID(businessID)
	if err != nil {
		// If menu doesn't exist, create a new one
		if errors.Is(err, gorm.ErrRecordNotFound) {
			newMenu := &Menu{
				BusinessID: businessID,
				Categories: "",
				IsActive:   true,
			}
			emptyCategories := []MenuCategory{}
			if err := CreateMenu(newMenu, emptyCategories); err != nil {
				return 0, nil, 0, fmt.Errorf("failed to create new menu: %w", err)
			}
			menu = newMenu
			categories = emptyCategories
		} else {
			return 0, nil, 0, fmt.Errorf("failed to get menu: %w", err)
		}
	}

	// Add new category
	categories = append(categories, category)
	newCatIdx := len(categories) - 1

	// Update menu with new categories
	if err := UpdateMenu(menu, categories); err != nil {
		return 0, nil, 0, err
	}
	created := categories[newCatIdx]
	return menu.Version, &created, newCatIdx, nil
}

// UpdateMenuCategory updates a specific category in a menu. Returns the bumped
// version and the mutated category.
func UpdateMenuCategory(businessID uint, categoryIndex int, updatedCategory MenuCategory) (uint, *MenuCategory, error) {
	menu, categories, err := GetMenuByBusinessID(businessID)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to get menu: %w", err)
	}

	if categoryIndex < 0 || categoryIndex >= len(categories) {
		return 0, nil, fmt.Errorf("category index out of range")
	}

	// Update the category
	categories[categoryIndex] = updatedCategory

	// Update menu with modified categories
	if err := UpdateMenu(menu, categories); err != nil {
		return 0, nil, err
	}
	mutated := categories[categoryIndex]
	return menu.Version, &mutated, nil
}

// UpdateMenuCategoryVersioned updates the category at categoryIndex under
// client-supplied optimistic CAS. A stale expectedVersion returns
// ErrMenuVersionConflict instead of silently clobbering a concurrent edit
// (L3-9). The stored category ID is preserved: index-addressed clients don't
// carry it, and identity must not churn on an in-place update.
func UpdateMenuCategoryVersioned(businessID uint, categoryIndex int, updatedCategory MenuCategory, expectedVersion uint) (uint, *MenuCategory, error) {
	menu, categories, err := GetMenuByBusinessID(businessID)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to get menu: %w", err)
	}
	if categoryIndex < 0 || categoryIndex >= len(categories) {
		return 0, nil, fmt.Errorf("category index out of range")
	}
	updatedCategory.ID = categories[categoryIndex].ID
	categories[categoryIndex] = updatedCategory
	if err := updateMenuWithVersionCheck(menu, categories, expectedVersion); err != nil {
		return 0, nil, err
	}
	mutated := categories[categoryIndex]
	return menu.Version, &mutated, nil
}

// DeleteMenuCategoryVersioned removes the category at categoryIndex under
// client-supplied optimistic CAS (L3-9): a stale expectedVersion returns
// ErrMenuVersionConflict — critical for deletes, where a stale index after a
// concurrent reorder would remove the wrong category.
func DeleteMenuCategoryVersioned(businessID uint, categoryIndex int, expectedVersion uint) (uint, error) {
	menu, categories, err := GetMenuByBusinessID(businessID)
	if err != nil {
		return 0, fmt.Errorf("failed to get menu: %w", err)
	}
	if categoryIndex < 0 || categoryIndex >= len(categories) {
		return 0, fmt.Errorf("category index out of range")
	}
	categories = append(categories[:categoryIndex], categories[categoryIndex+1:]...)
	if err := updateMenuWithVersionCheck(menu, categories, expectedVersion); err != nil {
		return 0, err
	}
	return menu.Version, nil
}

// DeleteMenuCategory removes a category from a menu. Returns the bumped version.
func DeleteMenuCategory(businessID uint, categoryIndex int) (uint, error) {
	menu, categories, err := GetMenuByBusinessID(businessID)
	if err != nil {
		return 0, fmt.Errorf("failed to get menu: %w", err)
	}

	if categoryIndex < 0 || categoryIndex >= len(categories) {
		return 0, fmt.Errorf("category index out of range")
	}

	// Remove the category
	categories = append(categories[:categoryIndex], categories[categoryIndex+1:]...)

	// Update menu with modified categories
	if err := UpdateMenu(menu, categories); err != nil {
		return 0, err
	}
	return menu.Version, nil
}

// AddMenuItem adds a new item to a specific category. Atomic under optimistic
// CAS with bounded retry (mutateMenuUnderCAS) so a concurrent menu write is not
// clobbered. The bounds check + mutation live inside the retried closure. Returns
// the bumped version and the created item (with its server-assigned ID).
func AddMenuItem(businessID uint, categoryIndex int, item MenuItem) (uint, *MenuItem, int, error) {
	var created MenuItem
	var itemIdx int
	v, err := mutateMenuUnderCAS(businessID, func(categories []MenuCategory) ([]MenuCategory, error) {
		if categoryIndex < 0 || categoryIndex >= len(categories) {
			return nil, fmt.Errorf("category index out of range")
		}
		toAdd := item
		if strings.TrimSpace(toAdd.ID) == "" {
			toAdd.ID = uuid.NewString()
		}
		categories[categoryIndex].Items = append(categories[categoryIndex].Items, toAdd)
		itemIdx = len(categories[categoryIndex].Items) - 1
		created = toAdd
		return categories, nil
	})
	if err != nil {
		return 0, nil, 0, err
	}
	return v, &created, itemIdx, nil
}

// AddMenuItemVersioned appends an item to the category at categoryIndex under
// client-supplied optimistic CAS (L3-9): a stale expectedVersion returns
// ErrMenuVersionConflict — a stale index after a concurrent reorder would
// otherwise land the item in the wrong category.
func AddMenuItemVersioned(businessID uint, categoryIndex int, item MenuItem, expectedVersion uint) (uint, *MenuItem, int, error) {
	menu, categories, err := GetMenuByBusinessID(businessID)
	if err != nil {
		return 0, nil, 0, fmt.Errorf("failed to get menu: %w", err)
	}
	if categoryIndex < 0 || categoryIndex >= len(categories) {
		return 0, nil, 0, fmt.Errorf("category index out of range")
	}
	categories[categoryIndex].Items = append(categories[categoryIndex].Items, item)
	newItemIdx := len(categories[categoryIndex].Items) - 1
	if err := updateMenuWithVersionCheck(menu, categories, expectedVersion); err != nil {
		return 0, nil, 0, err
	}
	// ensureMenuEntityIDs (inside updateMenuWithVersionCheck) assigned the UUID.
	created := categories[categoryIndex].Items[newItemIdx]
	return menu.Version, &created, newItemIdx, nil
}

// UpdateMenuItemVersioned updates the item at (categoryIndex, itemIndex) under
// client-supplied optimistic CAS. A stale expectedVersion returns
// ErrMenuVersionConflict instead of silently clobbering a concurrent edit
// (L3-9). The stored item ID is preserved, mirroring UpdateMenuItemByID.
func UpdateMenuItemVersioned(businessID uint, categoryIndex, itemIndex int, updatedItem MenuItem, expectedVersion uint) (uint, *MenuItem, error) {
	menu, categories, err := GetMenuByBusinessID(businessID)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to get menu: %w", err)
	}
	if categoryIndex < 0 || categoryIndex >= len(categories) {
		return 0, nil, fmt.Errorf("category index out of range")
	}
	if itemIndex < 0 || itemIndex >= len(categories[categoryIndex].Items) {
		return 0, nil, fmt.Errorf("item index out of range")
	}
	updatedItem.ID = categories[categoryIndex].Items[itemIndex].ID
	categories[categoryIndex].Items[itemIndex] = updatedItem
	if err := updateMenuWithVersionCheck(menu, categories, expectedVersion); err != nil {
		return 0, nil, err
	}
	mutated := categories[categoryIndex].Items[itemIndex]
	return menu.Version, &mutated, nil
}

// DeleteMenuItemVersioned removes the item at (categoryIndex, itemIndex) under
// client-supplied optimistic CAS (L3-9): a stale expectedVersion returns
// ErrMenuVersionConflict — a stale index after a concurrent reorder would
// otherwise delete the wrong item.
func DeleteMenuItemVersioned(businessID uint, categoryIndex, itemIndex int, expectedVersion uint) (uint, error) {
	menu, categories, err := GetMenuByBusinessID(businessID)
	if err != nil {
		return 0, fmt.Errorf("failed to get menu: %w", err)
	}
	if categoryIndex < 0 || categoryIndex >= len(categories) {
		return 0, fmt.Errorf("category index out of range")
	}
	if itemIndex < 0 || itemIndex >= len(categories[categoryIndex].Items) {
		return 0, fmt.Errorf("item index out of range")
	}
	items := categories[categoryIndex].Items
	categories[categoryIndex].Items = append(items[:itemIndex], items[itemIndex+1:]...)
	if err := updateMenuWithVersionCheck(menu, categories, expectedVersion); err != nil {
		return 0, err
	}
	return menu.Version, nil
}

// UpdateMenuItem updates a specific menu item. Atomic under optimistic CAS with
// bounded retry so a concurrent menu write is not clobbered. Returns the bumped
// version and the mutated item.
func UpdateMenuItem(businessID uint, categoryIndex, itemIndex int, updatedItem MenuItem) (uint, *MenuItem, error) {
	var mutated MenuItem
	v, err := mutateMenuUnderCAS(businessID, func(categories []MenuCategory) ([]MenuCategory, error) {
		if categoryIndex < 0 || categoryIndex >= len(categories) {
			return nil, fmt.Errorf("category index out of range")
		}
		if itemIndex < 0 || itemIndex >= len(categories[categoryIndex].Items) {
			return nil, fmt.Errorf("item index out of range")
		}
		categories[categoryIndex].Items[itemIndex] = updatedItem
		mutated = categories[categoryIndex].Items[itemIndex]
		return categories, nil
	})
	if err != nil {
		return 0, nil, err
	}
	return v, &mutated, nil
}

// DeleteMenuItem removes an item from a category. Atomic under optimistic CAS
// with bounded retry so a concurrent menu write is not clobbered. Returns the
// bumped version.
func DeleteMenuItem(businessID uint, categoryIndex, itemIndex int) (uint, error) {
	v, err := mutateMenuUnderCAS(businessID, func(categories []MenuCategory) ([]MenuCategory, error) {
		if categoryIndex < 0 || categoryIndex >= len(categories) {
			return nil, fmt.Errorf("category index out of range")
		}
		if itemIndex < 0 || itemIndex >= len(categories[categoryIndex].Items) {
			return nil, fmt.Errorf("item index out of range")
		}
		items := categories[categoryIndex].Items
		categories[categoryIndex].Items = append(items[:itemIndex], items[itemIndex+1:]...)
		return categories, nil
	})
	return v, err
}

// Table operations

// CreateTable creates a new table for a business
func CreateTable(table *Table) error {
	if err := db.Create(table).Error; err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}
	return nil
}

// GetTablesByBusinessID retrieves all active tables for a business
func GetTablesByBusinessID(businessID uint) ([]Table, error) {
	var tables []Table
	if err := db.Where("business_id = ? AND is_active = ?", businessID, true).Find(&tables).Error; err != nil {
		return nil, fmt.Errorf("failed to get tables: %w", err)
	}
	return tables, nil
}

// GetTableByCode retrieves a table by its unique code
func GetTableByCode(tableCode string) (*Table, error) {
	table, _, err := GetActiveTableWithBusinessByCode(tableCode)
	if err != nil {
		if errors.Is(err, ErrTableNotFound) {
			return nil, fmt.Errorf("table not found")
		}
		return nil, err
	}
	return table, nil
}

// GetActiveTableWithBusinessByCode retrieves the active table and its business
// in one indexed table-code lookup. Public QR endpoints need both records on
// every cache miss, so joining avoids a second roundtrip to businesses.
// ErrTableNotFound is returned when no active table matches the code (or its
// business is inactive). Callers can errors.Is on it to distinguish a genuine
// 404 from an infrastructure error (which must surface as 500, not a misleading
// "table not found").
var ErrTableNotFound = errors.New("table not found")

func GetActiveTableWithBusinessByCode(tableCode string) (*Table, *Business, error) {
	var table Table
	// Project only the public Business columns on the join (OF-04) instead of
	// hydrating the full ~94-column row on this unauthenticated QR path.
	// publicBusinessColumns is the exact set the guest response + gating read,
	// INCLUDING the is_demo/is_active/closed_at columns, so IsBusinessOperational computes the IDENTICAL gate as a full-row load.
	// Table codes are generated uppercase (randomTableCode / hex ToUpper), so
	// normalize lookup input. Keep raw/lowercase candidates as indexed fallbacks
	// for legacy rows and test fixtures that predate canonical table codes.
	tableCodeCandidates := tableCodeLookupCandidates(tableCode)
	if err := db.Joins("Business", db.Select(publicBusinessColumns)).
		Where("tables.table_code IN ? AND tables.is_active = ?", tableCodeCandidates, true).
		First(&table).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrTableNotFound
		}
		return nil, nil, fmt.Errorf("failed to get table: %w", err)
	}
	if !table.Business.IsActive {
		return nil, nil, ErrTableNotFound
	}
	return &table, &table.Business, nil
}

// CanonicalGuestTableCode maps a guest QR code to the stored active table_code.
func CanonicalGuestTableCode(code string) (string, error) {
	table, _, err := GetActiveTableWithBusinessByCode(code)
	if err != nil {
		return "", err
	}
	return table.TableCode, nil
}

func tableCodeLookupCandidates(tableCode string) []string {
	trimmed := strings.TrimSpace(tableCode)
	upper := strings.ToUpper(trimmed)
	lower := strings.ToLower(trimmed)

	candidates := make([]string, 0, 3)
	for _, candidate := range []string{upper, trimmed, lower} {
		if candidate == "" {
			continue
		}

		alreadyAdded := false
		for _, existing := range candidates {
			if existing == candidate {
				alreadyAdded = true
				break
			}
		}
		if !alreadyAdded {
			candidates = append(candidates, candidate)
		}
	}

	if len(candidates) == 0 {
		return []string{""}
	}
	return candidates
}

// GetTableByID retrieves a table by its ID
func GetTableByID(id uint) (*Table, error) {
	var table Table
	if err := db.First(&table, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("table not found")
		}
		return nil, fmt.Errorf("failed to get table: %w", err)
	}
	return &table, nil
}

// UpdateTable updates an existing table
func UpdateTable(table *Table) error {
	if err := db.Omit(clause.Associations).Save(table).Error; err != nil {
		return fmt.Errorf("failed to update table: %w", err)
	}
	return nil
}

// DeleteTable soft deletes a table (sets is_active to false).
// Returns ErrTableNotFound when no row is flipped (missing id or already
// inactive) so handlers can surface 404 instead of a silent 200 no-op (L3-30).
func DeleteTable(id uint) error {
	res := db.Model(&Table{}).Where("id = ? AND is_active = ?", id, true).Update("is_active", false)
	if res.Error != nil {
		return fmt.Errorf("failed to delete table: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrTableNotFound
	}
	return nil
}

// GenerateUniqueTableCode generates a unique 10-character random table code for a business
func GenerateUniqueTableCode(businessID uint, baseName string) (string, error) {
	const codeLength = 10

	// Try up to 10 times to generate a unique code
	for attempts := 0; attempts < 10; attempts++ {
		codeStr, err := randomTableCode(codeLength)
		if err != nil {
			return "", err
		}

		// Check if code already exists
		var existing Table
		if err := db.Where("table_code = ?", codeStr).First(&existing).Error; err != nil {
			// Code doesn't exist (GORM returns error when not found), so it's unique
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return codeStr, nil
			}
			// Some other database error occurred
			return "", err
		}
		// Code exists, try again
	}

	return "", fmt.Errorf("failed to generate unique table code after 10 attempts")
}

func randomTableCode(length int) (string, error) {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	tableCode := make([]byte, length)
	max := big.NewInt(int64(len(charset)))
	for i := range tableCode {
		n, err := cryptorand.Int(cryptorand.Reader, max)
		if err != nil {
			return "", err
		}
		tableCode[i] = charset[n.Int64()]
	}
	return string(tableCode), nil
}

// Bill operations

// generateUniqueBillNumber generates a unique bill number for a business using UUID.
func generateUniqueBillNumber(businessID uint) (string, error) {
	return fmt.Sprintf("B%d-%s", businessID, uuid.New().String()[:12]), nil
}

// GenerateBillPublicToken returns a 32-hex-char (16-byte crypto/rand) guest
// capability token for a bill.
func GenerateBillPublicToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := cryptorand.Read(buf); err != nil {
		return "", fmt.Errorf("generate bill public token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func ensureBillPublicToken(bill *Bill) error {
	if strings.TrimSpace(bill.PublicToken) != "" {
		return nil
	}
	token, err := GenerateBillPublicToken()
	if err != nil {
		return err
	}
	bill.PublicToken = token
	return nil
}

// BeforeCreate fills PublicToken for every insert path (handlers, services,
// tests) so the NOT NULL + unique index never sees an empty string.
func (b *Bill) BeforeCreate(tx *gorm.DB) error {
	return ensureBillPublicToken(b)
}

// NormalizeBillItemUUID is the exported form for callers outside this
// package that build BillItem slices and need them to satisfy the
// `bill_items.id` UUID column AND match the JSON snapshot stored on
// `bills.items`. Leaving the ID empty causes the relational row to get a
// gen_random_uuid() default while the JSON snapshot keeps "" — those drift
// silently breaks itemized splitting (which keys items by ID).
func NormalizeBillItemUUID(id string) string {
	return normalizeBillItemUUID(id)
}

func normalizeBillItemUUID(id string) string {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return uuid.New().String()
	}
	if _, err := uuid.Parse(trimmed); err != nil {
		return uuid.New().String()
	}
	return trimmed
}

// normalizeMoneyCents rounds a float64 dollar amount to the nearest cent (int64).
func normalizeMoneyCents(value float64) int64 {
	return money.CentsFromMajor(value)
}

// computeBillTotals calculates tax, service fee, and the GROSS total in cents
// (subtotal + tax + service fee). It does NOT apply the loyalty discount — callers
// that persist a bill total must run the result through NetBillTotalCents so an
// applied redemption is not silently erased. See NetBillTotalCents.
func computeBillTotals(subtotalCents int64, taxRate, serviceFeeRate float64) (int64, int64, int64) {
	return money.BillTotalsCents(subtotalCents, taxRate, serviceFeeRate)
}

// NetBillTotalCents returns what a bill owes after its loyalty discount, floored
// at zero. grossTotal is subtotal + tax + service fee. This is the single place
// the discount-application rule lives so every recompute path (item void/qty
// update, order approval, and the redeem/undo paths in package loyalty) stays
// consistent: TotalAmount is always net of LoyaltyDiscountCents, and a discount
// larger than the bill can never make the guest owe a negative amount.
func NetBillTotalCents(grossTotal, loyaltyDiscountCents int64) int64 {
	if loyaltyDiscountCents <= 0 {
		return grossTotal
	}
	if net := grossTotal - loyaltyDiscountCents; net > 0 {
		return net
	}
	return 0
}

func applyBillTotals(bill *Bill, items []BillItem, business *Business) {
	subtotalCents := int64(0)
	for _, item := range items {
		// Bundle children carry a catalog Price for kitchen display but must
		// never enter money math — their Subtotal is 0 by contract. Skip by
		// type so a stale non-zero Subtotal cannot inflate the check.
		if money.IsInformationalBillLine(item.ItemType) {
			continue
		}
		subtotalCents += normalizeMoneyCents(item.Subtotal)
	}

	bill.Subtotal = subtotalCents
	var grossTotal int64
	bill.TaxAmount, bill.ServiceFeeAmount, grossTotal = computeBillTotals(
		bill.Subtotal,
		business.TaxRate,
		business.ServiceFeeRate,
	)
	// Keep TotalAmount net of any loyalty redemption already applied to this bill.
	// Recompute paths reload the bill, so bill.LoyaltyDiscountCents is populated;
	// rebuilding only the gross total here would erase the redemption.
	bill.TotalAmount = NetBillTotalCents(grossTotal, bill.LoyaltyDiscountCents)
}

// billItemOptionSurcharges adapts MenuItemOption into the money helper shape.
func billItemOptionSurcharges(options []MenuItemOption) []money.OptionSurcharge {
	if len(options) == 0 {
		return nil
	}
	out := make([]money.OptionSurcharge, len(options))
	for i, option := range options {
		out[i] = money.OptionSurcharge{PriceChange: option.PriceChange}
	}
	return out
}

// NormalizeBillItemLineMoney rewrites a line's Subtotal from the canonical
// (price + options) × qty rule. Bundle children are forced to Subtotal 0 so
// Price×Qty recomputes cannot inflate open checks.
func NormalizeBillItemLineMoney(item *BillItem) {
	if item == nil {
		return
	}
	unitPriceCents := money.CentsFromMajor(item.Price)
	item.Price = money.MajorFromCents(unitPriceCents)
	if money.IsInformationalBillLine(item.ItemType) {
		// Keep a non-zero catalog Price for kitchen display if present, but the
		// billable amount is always zero on the child line.
		item.Subtotal = 0
		return
	}
	lineCents := money.LineSubtotalCents(
		item.ItemType,
		item.Price,
		item.Quantity,
		billItemOptionSurcharges(item.Options),
		item.Subtotal,
	)
	item.Subtotal = money.MajorFromCents(lineCents)
}

// ApplyBillTotals is the exported single-source recompute used by HTTP
// mutators (UpdateBill / AddBillItem) so list/detail/tables/analytics all
// persist the same tax → service → total math.
func ApplyBillTotals(bill *Bill, items []BillItem, business *Business) {
	applyBillTotals(bill, items, business)
}

// removeCancelledOrderBillItemsTx deletes the bill items contributed by a now-
// cancelled order and recomputes the bill's totals from what remains. It is the
// reverse of the approval step (which stamps each new bill item with order_id).
//
// It is idempotent and a no-op when no relational bill_items carry this
// order_id (e.g. legacy staff path that only billed on approve). Guest
// atomic checkout stamps order_id at create, so guest cancel of a still-
// pending order does remove those rows and recompute totals.
//
// B-8: items that exist only in the bills.items JSON snapshot (legacy bills
// predating relational bill_items) are matched by UUID and preserved across
// the rebuild — they have no relational counterpart to rebuild from.
func removeCancelledOrderBillItemsTx(tx *gorm.DB, billID, orderID uint) error {
	// Snapshot the legacy JSON-only items BEFORE deleting anything: an item in
	// bills.items with no relational row at all (by UUID) predates the
	// relational store and must survive the rebuild.
	// FOR UPDATE: this function does a read-modify-write of the bill's items,
	// totals and status, so it must serialize with concurrent approve/adjust/
	// payment writers on the same bill row. The cancel-branch caller already
	// holds this lock (business.go), so re-taking it here is a no-op within the
	// transaction; the clause also protects any future caller.
	var bill Bill
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&bill, billID).Error; err != nil {
		return fmt.Errorf("failed to load bill before removing cancelled items: %w", err)
	}

	var allRelationalIDs []string
	if err := tx.Model(&BillItem{}).Where("bill_id = ?", billID).Pluck("id", &allRelationalIDs).Error; err != nil {
		return fmt.Errorf("failed to list relational bill item ids: %w", err)
	}
	relationalIDSet := make(map[string]struct{}, len(allRelationalIDs))
	for _, id := range allRelationalIDs {
		relationalIDSet[id] = struct{}{}
	}

	var snapshot []BillItem
	if strings.TrimSpace(bill.Items) != "" {
		if err := json.Unmarshal([]byte(bill.Items), &snapshot); err != nil {
			return fmt.Errorf("failed to parse bill items snapshot: %w", err)
		}
	}
	legacyOnly := make([]BillItem, 0)
	for _, item := range snapshot {
		if _, exists := relationalIDSet[item.ID]; !exists {
			legacyOnly = append(legacyOnly, item)
		}
	}

	res := tx.Where("bill_id = ? AND order_id = ?", billID, orderID).Delete(&BillItem{})
	if res.Error != nil {
		return fmt.Errorf("failed to remove cancelled order items: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return nil
	}

	var business Business
	if err := tx.First(&business, bill.BusinessID).Error; err != nil {
		return fmt.Errorf("failed to load business for cancel recompute: %w", err)
	}

	remaining := []BillItem{}
	if err := tx.Where("bill_id = ?", billID).Order("created_at ASC, id ASC").Find(&remaining).Error; err != nil {
		return fmt.Errorf("failed to reload remaining bill items: %w", err)
	}

	// Final item set = surviving relational rows + JSON-only legacy items.
	combined := append(remaining, legacyOnly...)

	applyBillTotals(&bill, combined, &business)

	itemsJSON, err := json.Marshal(combined)
	if err != nil {
		return fmt.Errorf("failed to marshal remaining bill items: %w", err)
	}

	updates := map[string]interface{}{
		"subtotal":           bill.Subtotal,
		"tax_amount":         bill.TaxAmount,
		"service_fee_amount": bill.ServiceFeeAmount,
		"total_amount":       bill.TotalAmount,
		"items":              string(itemsJSON),
		"updated_at":         time.Now(),
	}
	if bill.Status == BillStatusOpen || bill.Status == BillStatusPartial || bill.Status == BillStatusPaid {
		newStatus := BillStatusOpen
		if bill.PaidAmount > billPaymentAmountTolerance {
			if bill.PaidAmount >= bill.TotalAmount-billPaymentAmountTolerance {
				newStatus = BillStatusPaid
			} else {
				newStatus = BillStatusPartial
			}
		}
		updates["status"] = newStatus

		// A cancel that leaves collected money above the new total is a refund
		// owed to the guest. The database package can't import the alerts
		// service (import cycle), so create the alert row directly in-tx.
		if newStatus == BillStatusPaid && bill.PaidAmount > bill.TotalAmount+billPaymentAmountTolerance {
			overpaid := bill.PaidAmount - bill.TotalAmount
			alert := OperationalAlert{
				BusinessID:   bill.BusinessID,
				AlertType:    OperationalAlertTypePaymentRefundReview,
				ResourceType: OperationalAlertResourceTypeBill,
				ResourceID:   int64(bill.ID),
				Priority:     OperationalAlertPriorityHigh,
				Status:       OperationalAlertStatusOpen,
				Title:        "Cancelled order left the bill overpaid",
				Body: fmt.Sprintf(
					"Cancelling order items dropped bill %s to %d¢, but %d¢ was already collected. Refund the guest %d¢.",
					bill.BillNumber, bill.TotalAmount, bill.PaidAmount, overpaid,
				),
				Metadata:    JSONRawMessage(fmt.Sprintf(`{"order_id":%d,"overpaid_cents":%d}`, orderID, overpaid)),
				LastEventAt: time.Now(),
			}
			if err := tx.Create(&alert).Error; err != nil {
				return fmt.Errorf("failed to record overpayment refund alert: %w", err)
			}
		}
	}

	if err := updateBillLifecycleTx(tx, billID, updates); err != nil {
		return fmt.Errorf("failed to update bill after cancelling order items: %w", err)
	}
	return nil
}

func createBillTx(tx *gorm.DB, bill *Bill, items []BillItem) error {
	if err := lockCounterForBillCreationTx(tx, bill); err != nil {
		return err
	}

	// PublicToken is a guest authorization capability, not optional metadata.
	// Generate it in the canonical creation path as well as BeforeCreate so
	// callers using a SkipHooks session cannot persist a blank capability.
	if err := ensureBillPublicToken(bill); err != nil {
		return fmt.Errorf("failed to generate bill public token: %w", err)
	}

	// Generate unique bill number if not provided
	if bill.BillNumber == "" {
		billNumber, err := generateUniqueBillNumber(bill.BusinessID)
		if err != nil {
			return fmt.Errorf("failed to generate bill number: %w", err)
		}
		bill.BillNumber = billNumber
	}

	stamp := time.Now().UTC()
	if !bill.CreatedAt.IsZero() {
		stamp = bill.CreatedAt
	}
	// Leftover Date Night children arrived with proto/JSON zeros (price 0,
	// created_at year-1). Stamp catalog prices + a real instant before the
	// snapshot is the wire copy (#708).
	if err := stampBillItemsForPersist(tx, items, bill.BusinessID, stamp); err != nil {
		return err
	}

	itemsJSON, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("failed to marshal items: %w", err)
	}
	bill.Items = string(itemsJSON)

	if err := tx.Create(bill).Error; err != nil {
		if (bill.TableID != 0 || bill.CounterID != nil) && isActiveBillIndexViolation(err) {
			return ErrActiveBillExists
		}
		return fmt.Errorf("failed to create bill: %w", err)
	}
	if err := syncCounterOccupancyForBillTx(tx, bill.ID); err != nil {
		return err
	}

	for i := range items {
		items[i].BillID = bill.ID
		if items[i].CreatedAt.IsZero() {
			items[i].CreatedAt = bill.CreatedAt
		}
		items[i].ID = normalizeBillItemUUID(items[i].ID)
	}
	if len(items) > 0 {
		if err := tx.Create(&items).Error; err != nil {
			return fmt.Errorf("failed to create bill items relation: %w", err)
		}
	}

	return nil
}

// CreateBillTx creates a bill on the caller-owned transaction without
// beginning or committing a nested transaction.
func CreateBillTx(tx *gorm.DB, bill *Bill, items []BillItem) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	return createBillTx(tx, bill, items)
}

// UpdateBillTx replaces a bill's authoritative item snapshot and relational
// rows on the caller-owned transaction.
func UpdateBillTx(tx *gorm.DB, bill *Bill, items []BillItem) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	return updateBillTx(tx, bill, items)
}

func updateBillTx(tx *gorm.DB, bill *Bill, items []BillItem) error {
	if err := stampBillItemsForPersist(tx, items, bill.BusinessID, time.Now().UTC()); err != nil {
		return err
	}
	// Normalize IDs before the JSON snapshot is written. Adjust/void look up
	// the snapshot; the relational rows must share the same id or PATCH
	// /items/:id 404s after AddBillItem (the handler returns the mutated slice).
	for i := range items {
		items[i].BillID = bill.ID
		items[i].ID = normalizeBillItemUUID(items[i].ID)
	}

	itemsJSON, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("failed to marshal items: %w", err)
	}
	bill.Items = string(itemsJSON)

	bill.UpdatedAt = time.Now()
	// Column whitelist: item edits own totals + the items snapshot, nothing
	// else. A full-row Save from a stale struct clobbers paid_amount/status/
	// loyalty columns written by concurrent payment and redemption flows.
	if err := tx.Model(&Bill{}).Where("id = ?", bill.ID).Updates(map[string]interface{}{
		"subtotal":           bill.Subtotal,
		"tax_amount":         bill.TaxAmount,
		"service_fee_amount": bill.ServiceFeeAmount,
		"total_amount":       bill.TotalAmount,
		"items":              bill.Items,
		"updated_at":         bill.UpdatedAt,
	}).Error; err != nil {
		return fmt.Errorf("failed to update bill: %w", err)
	}

	relationAvailable := true
	if err := tx.Where("bill_id = ?", bill.ID).Delete(&BillItem{}).Error; err != nil {
		if isMissingBillItemsRelationError(err) {
			relationAvailable = false
		} else {
			return fmt.Errorf("failed to clear existing bill items relation: %w", err)
		}
	}

	if relationAvailable && len(items) > 0 {
		if err := tx.Create(&items).Error; err != nil {
			return fmt.Errorf("failed to create bill items relation: %w", err)
		}
	}

	return nil
}

func createBillHistoryEventsTx(tx *gorm.DB, bill *Bill, events []BillHistoryEvent) error {
	if len(events) == 0 {
		return nil
	}

	for i := range events {
		if events[i].BillID == 0 && bill != nil {
			events[i].BillID = bill.ID
		}
		if events[i].BusinessID == 0 && bill != nil {
			events[i].BusinessID = bill.BusinessID
		}
	}

	if err := tx.Create(&events).Error; err != nil {
		return fmt.Errorf("failed to create bill history events: %w", err)
	}

	return nil
}

func billItemsFromJSONSnapshot(billID uint, raw string) ([]BillItem, error) {
	return billItemsFromJSONSnapshotConn(db, billID, raw)
}

func billItemsFromJSONSnapshotConn(conn *gorm.DB, billID uint, raw string) ([]BillItem, error) {
	var items []BillItem
	if strings.TrimSpace(raw) == "" {
		return []BillItem{}, nil
	}
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("failed to unmarshal items: %w", err)
	}
	for i := range items {
		if items[i].BillID == 0 {
			items[i].BillID = billID
		}
	}
	return hydrateBillItemsIfNeeded(conn, billID, items)
}

func billItemsForBillSnapshot(billID uint, raw string) ([]BillItem, error) {
	return billItemsForBillSnapshotConn(db, billID, raw)
}

func billItemsForBillSnapshotConn(conn *gorm.DB, billID uint, raw string) ([]BillItem, error) {
	var items []BillItem
	if err := conn.Where("bill_id = ?", billID).
		Order("created_at ASC, id ASC").
		Find(&items).Error; err != nil {
		if isMissingBillItemsRelationError(err) {
			return billItemsFromJSONSnapshotConn(conn, billID, raw)
		}
		return nil, fmt.Errorf("failed to query bill items: %w", err)
	}
	if len(items) > 0 {
		mergeBillItemSnapshotMetadata(items, raw)
		return hydrateBillItemsIfNeeded(conn, billID, items)
	}
	return billItemsFromJSONSnapshotConn(conn, billID, raw)
}

func mergeBillItemSnapshotMetadata(items []BillItem, raw string) {
	if strings.TrimSpace(raw) == "" {
		return
	}
	var snapshot []BillItem
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return
	}
	occurrenceByID := make(map[string]string, len(snapshot))
	for _, item := range snapshot {
		if item.BundleOccurrenceID != "" {
			occurrenceByID[item.ID] = item.BundleOccurrenceID
		}
	}
	for i := range items {
		if items[i].BundleOccurrenceID == "" {
			items[i].BundleOccurrenceID = occurrenceByID[items[i].ID]
		}
	}
}

func billItemsForBillSnapshotLazyByBillID(billID uint) ([]BillItem, error) {
	var items []BillItem
	if err := db.Where("bill_id = ?", billID).
		Order("created_at ASC, id ASC").
		Find(&items).Error; err != nil {
		if isMissingBillItemsRelationError(err) {
			raw, rawErr := billItemsJSONSnapshotByBillID(billID)
			if rawErr != nil {
				return nil, rawErr
			}
			return billItemsFromJSONSnapshot(billID, raw)
		}
		return nil, fmt.Errorf("failed to query bill items: %w", err)
	}
	if len(items) > 0 {
		return hydrateBillItemsIfNeeded(db, billID, items)
	}
	raw, err := billItemsJSONSnapshotByBillID(billID)
	if err != nil {
		return nil, err
	}
	return billItemsFromJSONSnapshot(billID, raw)
}

func billItemsJSONSnapshotByBillID(billID uint) (string, error) {
	var row struct {
		Items string
	}
	if err := db.Model(&Bill{}).Select("items").Where("id = ?", billID).First(&row).Error; err != nil {
		return "", fmt.Errorf("failed to load bill items snapshot: %w", err)
	}
	return row.Items, nil
}

func isMissingBillItemsRelationError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such table") ||
		strings.Contains(message, "does not exist")
}

// CreateBill creates a new bill
func CreateBill(bill *Bill, items []BillItem) error {
	return CreateBillWithHistory(bill, items, nil)
}

// CreateBillWithHistoryTx creates a bill and appends history events on the
// caller-owned transaction. This lets callers perform fail-closed validation
// and the bill mutation against one transaction snapshot.
func CreateBillWithHistoryTx(tx *gorm.DB, bill *Bill, items []BillItem, historyEvents []BillHistoryEvent) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	if err := createBillTx(tx, bill, items); err != nil {
		return err
	}

	if err := createBillHistoryEventsTx(tx, bill, historyEvents); err != nil {
		return err
	}
	return nil
}

// CreateBillWithHistory creates a new bill and appends history events atomically.
func CreateBillWithHistory(bill *Bill, items []BillItem, historyEvents []BillHistoryEvent) error {
	return db.Transaction(func(tx *gorm.DB) error {
		return CreateBillWithHistoryTx(tx, bill, items, historyEvents)
	})
}

// projectedBillBusinessPreload restricts the Business Preload in bill-detail
// and bill-number lookups to only the columns that callers actually read.
// This avoids hydrating the wide Business row (onboarding jsonb blob, all
// Stripe IDs, banner_images, social_media, onboarding_state, etc.) on the
// payment/webhook hot path.
//
// Column justification (keep in sync with grep of bill.Business.<Field>):
//
//	id                   — GORM PK/FK association
//	business_id          — guest_feedback_scheduler.go (BusinessId string)
//	user_id              — RBAC / tenant checks
//	owner_address        — RBAC / tenant checks
//	name                 — print/build_inputs.go, guest_feedback_scheduler.go
//	default_currency     — public_guest_response_helpers.go, business_handlers.go,
//	                       print/build_inputs.go
//	display_currency     — same as default_currency
//	default_language     — guest_feedback_scheduler.go
//	crm_enabled          — crm/settlement.go
//	street/city/state/postal_code/country — embedded BusinessAddress,
//	                       print/build_inputs.go (bill.Business.Address)
//	is_active/closed_at  — general availability checks
func projectedBillBusinessPreload(tx *gorm.DB) *gorm.DB {
	return tx.Select(
		"id", "business_id", "user_id", "owner_address",
		// name/logo/timezone are emitted by newBillPublicBusiness() in the bill
		// JSON serializer, so they must be projected or every bill payload loses them.
		"name", "logo", "timezone",
		"default_currency", "display_currency", "default_language",
		"tax_rate", "service_fee_rate",
		"crm_enabled",
		// embedded BusinessAddress columns (gorm:"embedded")
		"street", "city", "state", "postal_code", "country",
		// is_active + closed_at are the admin-lock columns.
		"is_active", "closed_at",
	)
}

// GetBillByID retrieves a bill by its ID with items parsed
func GetBillByID(id uint) (*Bill, []BillItem, error) {
	var bill Bill
	if err := db.Preload("Business", projectedBillBusinessPreload).
		Preload("Table").
		Preload("Payments").
		First(&bill, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, fmt.Errorf("bill not found")
		}
		return nil, nil, fmt.Errorf("failed to get bill: %w", err)
	}
	if err := loadBillManagedAlternativePayments(&bill); err != nil {
		return nil, nil, err
	}

	raw := bill.Items
	bill.loadedItemsRaw = &raw
	items, err := billItemsForBillSnapshot(bill.ID, bill.Items)
	if err != nil {
		return nil, nil, err
	}
	items, err = finalizeBillItemsRead(&bill, items)
	if err != nil {
		return nil, nil, err
	}

	return &bill, items, nil
}

// BusinessAccessDecisionColumns is the exact set CheckBusinessAccess +
// IsBusinessOperational read. Shared with dashboard-summaries so both
// paths scan is_demo/kind the same way on production Postgres.
//
// Do not add businesses.business_id (the slug). Selecting that varchar
// unique column into Business.BusinessId dropped later decision columns
// (is_demo/kind) on production Postgres while SQLite tests still passed.
var BusinessAccessDecisionColumns = []string{
	"id",
	"owner_address",
	"user_id",
	"is_demo",
	"kind",
	"demo_owner_user_id",
	"is_active",
	"closed_at",
	"timezone",
	"service_day_start_minute",
}

// GetBillBusinessAccessByBillID retrieves only the business fields required to
// authorize bill-scoped mutations. It intentionally avoids hydrating the bill
// aggregate, table, payments, or item payload.
func GetBillBusinessAccessByBillID(id uint) (*Business, error) {
	// Bind the FK with database/sql. A GORM Take into {BusinessID uint} from
	// Table("bills") can scan bills.id into that field, so bill 757 authorized
	// against businesses.id=757 (a live tenant) and 403'd.
	var businessID uint
	if err := db.Raw("SELECT business_id FROM bills WHERE id = ?", id).Scan(&businessID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("bill not found")
		}
		return nil, fmt.Errorf("failed to get bill business access: %w", err)
	}
	if businessID == 0 {
		return nil, fmt.Errorf("bill not found")
	}
	business, err := GetBusinessAuthScopeByIdOrBusinessId(strconv.FormatUint(uint64(businessID), 10))
	if err != nil {
		return nil, fmt.Errorf("failed to get bill business access: %w", err)
	}
	return business, nil
}

// GetBillHistoryByBillID retrieves append-only history for a bill.
func GetBillHistoryByBillID(billID uint) ([]BillHistoryEvent, error) {
	var history []BillHistoryEvent
	if err := db.Where("bill_id = ?", billID).Order("created_at DESC, id DESC").Find(&history).Error; err != nil {
		return nil, fmt.Errorf("failed to get bill history: %w", err)
	}
	return history, nil
}

// GetBillHistoryByBillIDLimited returns the newest `limit` history events plus
// the total event count. The bill-detail modal previously loaded EVERY event
// on every open (and re-fetched on each SSE change while open); a long-running
// bill can accumulate hundreds. limit <= 0 falls back to the full unbounded
// read (legacy shape) so callers that pass no limit are unaffected. The events
// come back newest-first (matching the unbounded reader) so the modal renders
// the most recent activity first with an honest "showing N of total".
func GetBillHistoryByBillIDLimited(billID uint, limit int) ([]BillHistoryEvent, int64, error) {
	var total int64
	if err := db.Model(&BillHistoryEvent{}).Where("bill_id = ?", billID).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count bill history: %w", err)
	}
	if limit <= 0 {
		history, err := GetBillHistoryByBillID(billID)
		if err != nil {
			return nil, 0, err
		}
		return history, total, nil
	}
	var history []BillHistoryEvent
	if err := db.Where("bill_id = ?", billID).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&history).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get bill history: %w", err)
	}
	return history, total, nil
}

// PublicBillTokenWhere is the sole guest capability predicate for public/unauth
// bill resolution. public_token is uniquely indexed (idx_bills_public_token);
// a single bind keeps the guest poll path as one indexed probe. bill_number is
// operator-facing and must never appear in public WHERE clauses.
// Bind the token ONCE at every call site.
const PublicBillTokenWhere = "bills.public_token = ?"

func isMissingAlternativePaymentsRelationError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "alternative_payments") &&
		(strings.Contains(msg, "no such table") || strings.Contains(msg, "does not exist"))
}

func loadBillManagedAlternativePayments(bill *Bill) error {
	var payments []AlternativePayment
	if err := db.Where("bill_id = ? AND payment_method IN ?", bill.ID, billManagedAlternativePaymentMethodStrings).
		Order("created_at ASC, id ASC").
		Find(&payments).Error; err != nil {
		if isMissingAlternativePaymentsRelationError(err) {
			return nil
		}
		return fmt.Errorf("failed to load alternative payments: %w", err)
	}
	bill.AlternativePayments = payments
	return nil
}

// ErrPublicGuestBillNotFound is returned when no bill matches the public token
// (or its business is inactive), so callers can distinguish a 404 from an infra error.
var ErrPublicGuestBillNotFound = errors.New("bill not found")

// projectedBillWithCurrency scans a narrow bills projection together with the
// owning venue's resolved display currency. Bill.Currency is gorm:"-", so GORM
// never selects or scans it; without this wrapper every projected guest bill
// read hands the serializers an empty currency and they fall back to "USD" —
// the exact bug behind #852/#856 on ARS venues. Embedding Bill keeps the
// projected column list (and the money contract) unchanged.
type projectedBillWithCurrency struct {
	Bill
	ResolvedCurrency string `gorm:"column:resolved_currency"`
}

// bill returns the scanned Bill with Currency populated from the joined venue.
// It hands back an interior pointer rather than a copy: Bill embeds the full
// Business and Table structs, so copying it out would double the per-read
// allocation on routes guests poll.
func (p *projectedBillWithCurrency) bill() *Bill {
	if p.Bill.Currency == "" {
		p.Bill.Currency = p.ResolvedCurrency
	}
	return &p.Bill
}

// GetPublicBillByToken retrieves only the bill fields exposed by public guest
// bill detail routes, keyed by the unguessable public_token capability. It
// verifies the owning business is active without hydrating business/table/payment
// relations. Empty/blank tokens return ErrPublicGuestBillNotFound.
func GetPublicBillByToken(token string) (*Bill, []BillItem, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, nil, ErrPublicGuestBillNotFound
	}
	var row projectedBillWithCurrency
	// Table (not Model): see GetPublicGuestOpenBillByTableID.
	if err := db.Table("bills").
		Select(
			"bills.id",
			"bills.business_id",
			"bills.table_id",
			"bills.bill_number",
			"bills.public_token",
			"bills.subtotal",
			"bills.tax_amount",
			"bills.service_fee_amount",
			"bills.total_amount",
			"bills.paid_amount",
			"bills.tip_amount",
			// Loyalty redemption fields: without these the projection returns
			// zero on reload and the guest's "Loyalty discount" line disappears,
			// leaving an unexplained subtotal-vs-total gap at payment time.
			"bills.loyalty_discount_cents",
			"bills.loyalty_points_redeemed",
			"bills.loyalty_redeemed_by_customer_id",
			"bills.status",
			"bills.settlement_addr",
			"bills.tipping_addr",
			"bills.created_at",
			"bills.updated_at",
			"bills.closed_at",
			// The businesses row is already joined for the is_active gate, so
			// the venue's currency rides along for free rather than costing a
			// second read or a full Business preload (#852/#856).
			BusinessDisplayCurrencySelectSQL+" AS resolved_currency",
		).
		Joins("JOIN businesses ON businesses.id = bills.business_id").
		Where(PublicBillTokenWhere+" AND businesses.is_active = ?", token, true).
		Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrPublicGuestBillNotFound
		}
		return nil, nil, fmt.Errorf("failed to get public guest bill: %w", err)
	}
	bill := row.bill()

	items, err := billItemsForBillSnapshotLazyByBillID(bill.ID)
	if err != nil {
		return nil, nil, err
	}
	items, err = finalizeBillItemsRead(bill, items)
	if err != nil {
		return nil, nil, err
	}

	return bill, items, nil
}

// GetGuestBillIDByToken resolves only bills.id for an active business, with no
// relation hydration, items snapshot, or alternative-payment load. Used by the
// public guest order-poll (polled every 3s per seated guest) and plugin status
// callers that need only the bill id. The JOIN gates on businesses.is_active so
// a bill on a deactivated business is not resolvable. (JSON-01/PRELOAD-01/OF-03)
func GetGuestBillIDByToken(token string) (uint, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, fmt.Errorf("bill not found")
	}
	var bill Bill
	err := db.Model(&Bill{}).
		Select("bills.id").
		Joins("JOIN businesses ON businesses.id = bills.business_id AND businesses.is_active = ?", true).
		Where(PublicBillTokenWhere, token).
		Take(&bill).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, fmt.Errorf("bill not found")
		}
		return 0, fmt.Errorf("failed to resolve bill: %w", err)
	}
	return bill.ID, nil
}

// GetGuestBillScopeByToken resolves bills.id and bills.business_id for an active
// business with the same lean single-query, no-hydration shape as
// GetGuestBillIDByToken. Used by plugin status callers that scope by business
// id but read no other bill fields. (JSON-01/PRELOAD-01/OF-03)
func GetGuestBillScopeByToken(token string) (id uint, businessID uint, err error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return 0, 0, fmt.Errorf("bill not found")
	}
	var bill Bill
	err = db.Model(&Bill{}).
		Select("bills.id", "bills.business_id").
		Joins("JOIN businesses ON businesses.id = bills.business_id AND businesses.is_active = ?", true).
		Where(PublicBillTokenWhere, token).
		Take(&bill).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, 0, fmt.Errorf("bill not found")
		}
		return 0, 0, fmt.Errorf("failed to resolve bill: %w", err)
	}
	return bill.ID, bill.BusinessID, nil
}

// GetBillBusinessIDByBillID resolves only bills.business_id for a bill id with no
// relation hydration. Used by webhook ingest paths that need ownership checks or
// business_id backfill without loading the full bill aggregate.
func GetBillBusinessIDByBillID(billID uint) (uint, error) {
	var bill Bill
	err := db.Model(&Bill{}).
		Select("business_id").
		Where("id = ?", billID).
		Take(&bill).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, fmt.Errorf("bill not found")
		}
		return 0, fmt.Errorf("failed to resolve bill business_id: %w", err)
	}
	return bill.BusinessID, nil
}

// GetGuestPluginCheckoutBillByToken loads the lean public guest bill fields
// plus table_code for plugin checkout return URLs, keyed by public_token.
// Avoids hydrating the full bill aggregate (Business/Table/Payments preloads).
func GetGuestPluginCheckoutBillByToken(token string) (*Bill, *Table, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, nil, fmt.Errorf("bill not found")
	}
	var bill Bill
	if err := db.Model(&Bill{}).
		Select(
			"bills.id",
			"bills.business_id",
			"bills.table_id",
			"bills.bill_number",
			"bills.total_amount",
			"bills.paid_amount",
			"bills.status",
		).
		Joins("JOIN businesses ON businesses.id = bills.business_id AND businesses.is_active = ?", true).
		Where(PublicBillTokenWhere, token).
		Take(&bill).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, fmt.Errorf("bill not found")
		}
		return nil, nil, fmt.Errorf("failed to get guest plugin checkout bill: %w", err)
	}

	if bill.TableID == 0 {
		return &bill, nil, nil
	}

	var table Table
	if err := db.Select("id", "table_code").
		Where("id = ?", bill.TableID).
		Take(&table).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, fmt.Errorf("table not found")
		}
		return nil, nil, fmt.Errorf("failed to get guest plugin checkout table: %w", err)
	}

	return &bill, &table, nil
}

// GetGuestPluginCheckoutBusiness loads only the business fields required for
// public plugin checkout (name metadata + currency resolution).
func GetGuestPluginCheckoutBusiness(businessID uint) (*Business, error) {
	var business Business
	// Operational-gate fields are selected alongside the display fields so the
	// guest checkout handler can enforce IsBusinessOperational (mirroring the
	// native crypto path) without a second business query: the demo
	// short-circuit and the admin close/suspend lock.
	if err := db.Select(
		"id",
		"name",
		"default_currency",
		"is_demo",
		"is_active",
		"closed_at",
	).
		Where("id = ?", businessID).
		Take(&business).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("business not found")
		}
		return nil, fmt.Errorf("failed to get guest plugin checkout business: %w", err)
	}
	return &business, nil
}

// ActiveBillStatuses returns statuses that still represent an in-progress bill.
func ActiveBillStatuses() []BillStatus {
	return []BillStatus{BillStatusOpen, BillStatusPartial}
}

// ErrNoActiveBill indicates a table has no open or partially paid bill.
var ErrNoActiveBill = errors.New("no active bill found for table")

// ErrActiveBillExists indicates a table or counter already has an open/partial
// bill, so a new one cannot be created. Partial unique indexes enforce both
// location types atomically.
var ErrActiveBillExists = errors.New("location already has an active bill")

var (
	ErrCounterNotFound         = errors.New("counter not found")
	ErrCounterBusinessMismatch = errors.New("counter does not belong to the bill business")
	ErrCounterInactive         = errors.New("counter is inactive")
)

// ActiveBillConflictError carries the bill an operator should open after a
// losing create race. Unwrap keeps existing errors.Is(..., ErrActiveBillExists)
// callers compatible.
type ActiveBillConflictError struct {
	BillID uint
}

func (e *ActiveBillConflictError) Error() string {
	return fmt.Sprintf("counter already has active bill %d", e.BillID)
}

func (e *ActiveBillConflictError) Unwrap() error { return ErrActiveBillExists }

func activeBillStatusStrings() []string {
	statuses := ActiveBillStatuses()
	values := make([]string, 0, len(statuses))
	for _, status := range statuses {
		values = append(values, string(status))
	}
	return values
}

func lockCounterForBillCreationTx(tx *gorm.DB, bill *Bill) error {
	if bill == nil || bill.CounterID == nil {
		return nil
	}

	var counter Counter
	err := tx.Select("id", "business_id", "is_active").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", *bill.CounterID).
		Take(&counter).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCounterNotFound
		}
		return fmt.Errorf("failed to lock counter: %w", err)
	}
	if counter.BusinessID != bill.BusinessID {
		return ErrCounterBusinessMismatch
	}
	if !counter.IsActive {
		return ErrCounterInactive
	}

	var active struct{ ID uint }
	err = tx.Model(&Bill{}).
		Select("id").
		Where("counter_id = ? AND status IN ?", counter.ID, activeBillStatusStrings()).
		Order("created_at ASC, id ASC").
		Take(&active).Error
	if err == nil {
		return &ActiveBillConflictError{BillID: active.ID}
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to check counter occupancy: %w", err)
	}
	return nil
}

// updateBillLifecycleTx is the canonical bill-status persistence path. It
// updates the bill and synchronizes the counter cache in the same transaction;
// callers that roll back leave both pieces unchanged.
func updateBillLifecycleTx(tx *gorm.DB, billID uint, updates map[string]interface{}) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	// settled_at is append-once accounting evidence. A terminal transition may
	// supply its exact effective time through closed_at; otherwise the database
	// transaction's current time is used. Reversals/refunds can reopen a bill,
	// but must never rewrite when its original settlement occurred.
	if status, ok := billStatusUpdate(updates["status"]); ok && billStatusIsSettled(status) {
		settledAt := time.Now().UTC()
		if closedAt, ok := billUpdateTime(updates["closed_at"]); ok {
			settledAt = closedAt.UTC()
		}
		updates["settled_at"] = gorm.Expr("COALESCE(settled_at, ?)", settledAt)
	}
	if err := tx.Model(&Bill{}).Where("id = ?", billID).Updates(updates).Error; err != nil {
		return err
	}
	return syncCounterOccupancyForBillTx(tx, billID)
}

func billStatusUpdate(value interface{}) (BillStatus, bool) {
	switch status := value.(type) {
	case BillStatus:
		return status, true
	case string:
		return BillStatus(status), true
	default:
		return "", false
	}
}

func billStatusIsSettled(status BillStatus) bool {
	return status == BillStatusPaid || status == BillStatusClosed
}

func billUpdateTime(value interface{}) (time.Time, bool) {
	switch at := value.(type) {
	case time.Time:
		return at, !at.IsZero()
	case *time.Time:
		return dereferenceTime(at)
	default:
		return time.Time{}, false
	}
}

func dereferenceTime(at *time.Time) (time.Time, bool) {
	if at == nil || at.IsZero() {
		return time.Time{}, false
	}
	return *at, true
}

func syncCounterOccupancyForBillTx(tx *gorm.DB, billID uint) error {
	var bill struct {
		ID         uint
		BusinessID uint
		CounterID  *uint
		Status     BillStatus
	}
	if err := tx.Model(&Bill{}).
		Select("id", "business_id", "counter_id", "status").
		Where("id = ?", billID).
		Take(&bill).Error; err != nil {
		return fmt.Errorf("failed to load bill counter occupancy: %w", err)
	}
	return syncCounterOccupancyForBillStateTx(
		tx,
		bill.ID,
		bill.BusinessID,
		bill.CounterID,
		bill.Status,
	)
}

func syncCounterOccupancyForBillStateTx(
	tx *gorm.DB,
	billID uint,
	businessID uint,
	counterID *uint,
	status BillStatus,
) error {
	if counterID == nil {
		return nil
	}

	if billStatusIsActive(status) {
		result := tx.Model(&Counter{}).
			Where("id = ? AND business_id = ?", *counterID, businessID).
			Update("current_bill_id", billID)
		if result.Error != nil {
			return fmt.Errorf("failed to claim counter for bill: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrCounterBusinessMismatch
		}
		return nil
	}

	if err := tx.Model(&Counter{}).
		Where("id = ? AND business_id = ? AND current_bill_id = ?", *counterID, businessID, billID).
		Update("current_bill_id", nil).Error; err != nil {
		return fmt.Errorf("failed to release counter for bill: %w", err)
	}
	return nil
}

// GetOpenBillListRowsByBusinessIDFilteredPaginated retrieves active bills for
// list views using the narrow bill-list projection instead of full Bill models.
func GetOpenBillListRowsByBusinessIDFilteredPaginated(businessID uint, filters BillListFilters, p PaginationParams) (*PaginatedResult[BillListRow], error) {
	filters.Status = ""
	return getBillListRowsByBusinessIDFilteredPaginated(
		filters,
		p,
		db.Model(&Bill{}).Where("bills.business_id = ? AND bills.status IN ?", businessID, activeBillStatusStrings()),
		"active bills",
	)
}

func applyBillStatusFilter(query *gorm.DB, status string) *gorm.DB {
	status = strings.TrimSpace(status)
	if status == "" || status == "all" {
		return query
	}

	rawStatuses := strings.Split(status, ",")
	statuses := make([]string, 0, len(rawStatuses))
	for _, rawStatus := range rawStatuses {
		normalized := strings.TrimSpace(rawStatus)
		if normalized != "" {
			statuses = append(statuses, normalized)
		}
	}
	if len(statuses) == 0 {
		return query
	}
	return query.Where("bills.status IN ?", statuses)
}

func applyBillListFilters(query *gorm.DB, filters BillListFilters) *gorm.DB {
	query = applyBillStatusFilter(query, filters.Status)
	// Hide leftover empty $0 terminal walk-ins (QA seat leftovers) from
	// default history. Real $0 comps with item rows stay visible (#712).
	query = query.Where(
		`NOT (
			bills.status IN ? AND bills.total_amount = 0
			AND NOT EXISTS (SELECT 1 FROM bill_items WHERE bill_items.bill_id = bills.id)
			AND (bills.items IS NULL OR TRIM(bills.items) = '' OR TRIM(bills.items) = '[]')
		)`,
		[]BillStatus{BillStatusClosed, BillStatusVoided, BillStatusAbandoned},
	)

	if filters.CreatedFrom != nil {
		query = query.Where("bills.created_at >= ?", *filters.CreatedFrom)
	}
	if filters.CreatedTo != nil {
		query = query.Where("bills.created_at < ?", *filters.CreatedTo)
	}
	if filters.CustomerID != nil {
		query = query.Where("bills.crm_customer_id = ?", *filters.CustomerID)
	}

	search := strings.ToLower(strings.TrimSpace(filters.Search))
	if search == "" {
		return query
	}

	search = strings.TrimPrefix(search, "#")
	search = strings.TrimSpace(search)

	conditions := []string{
		"LOWER(COALESCE(bills.bill_number, '')) LIKE ? ESCAPE '\\'",
	}
	args := []interface{}{"%" + escapeSQLLike(search) + "%"}

	if tableID, ok := parseBillSearchNamedID(search, "table"); ok {
		conditions = append(conditions, "bills.table_id = ?")
		args = append(args, tableID)
	} else if counterID, ok := parseBillSearchNamedID(search, "counter"); ok {
		conditions = append(conditions, "bills.counter_id = ?")
		args = append(args, counterID)
	} else if identifierID, ok := parseBillSearchID(search); ok {
		conditions = append(conditions, "bills.table_id = ?", "bills.counter_id = ?")
		args = append(args, identifierID, identifierID)
	}

	amountSearch := strings.TrimPrefix(strings.TrimSpace(search), "$")
	if amount, err := strconv.ParseFloat(amountSearch, 64); err == nil {
		conditions = append(conditions, "bills.total_amount = ?")
		args = append(args, int64(math.Round(amount*100)))
	}

	return query.Where("("+strings.Join(conditions, " OR ")+")", args...)
}

func parseBillSearchNamedID(search string, prefix string) (uint, bool) {
	remainder := strings.TrimSpace(strings.TrimPrefix(search, prefix))
	if remainder == search {
		return 0, false
	}
	return parseBillSearchID(remainder)
}

func parseBillSearchID(search string) (uint, bool) {
	search = strings.TrimSpace(strings.TrimPrefix(search, "#"))
	if search == "" || strings.ContainsAny(search, " .,$") {
		return 0, false
	}
	value, err := strconv.ParseUint(search, 10, 32)
	if err != nil || value == 0 {
		return 0, false
	}
	return uint(value), true
}

func escapeSQLLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return value
}

// GetBillListRowsByBusinessIDFilteredPaginated retrieves bills for list views
// using a narrow projection. The list endpoint should not hydrate Bill relation
// graphs only to immediately reshape them into DTOs.
func GetBillListRowsByBusinessIDFilteredPaginated(businessID uint, filters BillListFilters, p PaginationParams) (*PaginatedResult[BillListRow], error) {
	return getBillListRowsByBusinessIDFilteredPaginated(
		filters,
		p,
		db.Model(&Bill{}).Where("bills.business_id = ?", businessID),
		"bills",
	)
}

func getBillListRowsByBusinessIDFilteredPaginated(filters BillListFilters, p PaginationParams, base *gorm.DB, label string) (*PaginatedResult[BillListRow], error) {
	p = p.Normalize()
	base = applyBillListFilters(base, filters)

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count %s: %w", label, err)
	}

	var rows []BillListRow
	if err := base.
		Select(`bills.id,
			bills.business_id,
			bills.table_id,
			bills.counter_id,
			COALESCE(tables.name, '') AS table_name,
			bills.bill_number,
			bills.notes,
			bills.items,
			(SELECT COUNT(*) FROM bill_items WHERE bill_items.bill_id = bills.id) AS item_count,
			COALESCE((
				SELECT SUM(
					CASE
						-- Operator "items" = sellable lines (menu + bundle parents).
						-- Bundle children are kitchen components, not separate sold units.
						WHEN COALESCE(NULLIF(bill_items.item_type, ''), 'menu_item') IN ('menu_item', 'bundle')
							THEN CASE WHEN bill_items.quantity > 0 THEN bill_items.quantity ELSE 0 END
						ELSE 0
					END
				)
				FROM bill_items
				WHERE bill_items.bill_id = bills.id
			), 0) AS physical_item_quantity,
			bills.subtotal,
			bills.tax_amount,
			bills.service_fee_amount,
			bills.total_amount,
			bills.paid_amount,
			bills.tip_amount,
			bills.status,
			bills.created_by_staff_id,
			bills.closed_by_staff_id,
			bills.crm_customer_id,
			bills.created_at,
			bills.updated_at,
			bills.closed_at,
			bills.feedback_email_sent_at`).
		Joins("LEFT JOIN tables ON tables.id = bills.table_id").
		Order("bills.created_at DESC, bills.id DESC").
		Offset(p.Offset()).
		Limit(p.PageSize).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to get %s: %w", label, err)
	}

	totalPages := int(total) / p.PageSize
	if int(total)%p.PageSize != 0 {
		totalPages++
	}
	if err := hydrateBillListRowsForRead(db, rows); err != nil {
		return nil, err
	}
	return &PaginatedResult[BillListRow]{Data: rows, Total: total, Page: p.Page, PageSize: p.PageSize, TotalPages: totalPages}, nil
}

// GetOpenBillByTableID retrieves the active bill for a specific table.
func GetOpenBillByTableID(tableID uint) (*Bill, []BillItem, error) {
	var bill Bill
	if err := db.Where("table_id = ? AND status IN ?", tableID, activeBillStatusStrings()).Order("created_at DESC").First(&bill).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrNoActiveBill
		}
		return nil, nil, fmt.Errorf("failed to get active bill: %w", err)
	}

	items, err := billItemsForBillSnapshot(bill.ID, bill.Items)
	if err != nil {
		return nil, nil, err
	}
	items, err = finalizeBillItemsRead(&bill, items)
	if err != nil {
		return nil, nil, err
	}

	return &bill, items, nil
}

// GetPublicGuestOpenBillByTableID retrieves the active bill fields exposed by
// public guest bill/status routes plus the item snapshot. Internal flows that
// mutate bills should keep using GetOpenBillByTableID for the full aggregate.
// public_token is required so guest capability routes (pay/split/receipt) work
// from the open-by-table response (PV-LIVE-20260720-001).
func GetPublicGuestOpenBillByTableID(tableID uint) (*Bill, []BillItem, error) {
	var row projectedBillWithCurrency
	// Table (not Model): Model(&Bill{}) would heap-allocate a throwaway Bill —
	// and Bill embeds Business and Table, so that is kilobytes per guest poll.
	if err := db.Table("bills").Select(
		"bills.id",
		"bills.business_id",
		"bills.table_id",
		"bills.bill_number",
		"bills.public_token",
		"bills.subtotal",
		"bills.tax_amount",
		"bills.service_fee_amount",
		"bills.total_amount",
		"bills.paid_amount",
		"bills.tip_amount",
		// Guest bill/status helpers emit loyalty_discount as dollars. Without
		// this column the projection zeroes the discount and the guest
		// breakdown no longer adds up after a reload.
		"bills.loyalty_discount_cents",
		"bills.status",
		"bills.settlement_addr",
		"bills.tipping_addr",
		"bills.created_at",
		"bills.updated_at",
		"bills.closed_at",
		// Venue currency via a PK join, not a Business preload: the guest bill
		// payload must label ARS money as ARS (#852/#856).
		BusinessDisplayCurrencySelectSQL+" AS resolved_currency",
	).
		Joins("LEFT JOIN businesses ON businesses.id = bills.business_id").
		Where("bills.table_id = ? AND bills.status IN ?", tableID, activeBillStatusStrings()).
		Order("bills.created_at DESC").
		First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrNoActiveBill
		}
		return nil, nil, fmt.Errorf("failed to get public active bill: %w", err)
	}
	bill := row.bill()

	items, err := billItemsForBillSnapshotLazyByBillID(bill.ID)
	if err != nil {
		return nil, nil, err
	}
	items, err = finalizeBillItemsRead(bill, items)
	if err != nil {
		return nil, nil, err
	}

	return bill, items, nil
}

// GetOpenBillSummaryAndItemsByTableID returns the projected open bill + its item
// snapshot for AI bill-context assembly. Reuses the public-guest projection so
// the AI hot path never hydrates the full Bill aggregate.
func GetOpenBillSummaryAndItemsByTableID(tableID uint) (*Bill, []BillItem, error) {
	return GetPublicGuestOpenBillByTableID(tableID)
}

// GetOpenBillSummaryByTableID retrieves only the fields needed by public table
// summary responses. It intentionally avoids loading bill_items or parsing the
// legacy items JSON snapshot.
func GetOpenBillSummaryByTableID(tableID uint) (*Bill, error) {
	var row projectedBillWithCurrency
	// Table (not Model): see GetPublicGuestOpenBillByTableID.
	if err := db.Table("bills").Select(
		"bills.id",
		"bills.business_id",
		"bills.table_id",
		"bills.bill_number",
		"bills.public_token",
		"bills.total_amount",
		"bills.paid_amount",
		"bills.status",
		"bills.created_at",
		// Summary responses carry money, so they must carry its currency
		// (#852/#856). One PK join, no Business hydration.
		BusinessDisplayCurrencySelectSQL+" AS resolved_currency",
	).
		Joins("LEFT JOIN businesses ON businesses.id = bills.business_id").
		Where("bills.table_id = ? AND bills.status IN ?", tableID, activeBillStatusStrings()).
		Order("bills.created_at DESC").
		First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNoActiveBill
		}
		return nil, fmt.Errorf("failed to get active bill summary: %w", err)
	}
	return row.bill(), nil
}

// HasActiveBillForTableID checks table occupancy without hydrating a bill
// aggregate. This is used by the guest new-bill path where the common case is
// "no active bill"; a narrow projection avoids GORM's record-not-found log and
// lets Postgres satisfy the probe from a partial active-bill index.
func HasActiveBillForTableID(tableID uint) (bool, error) {
	var ids []uint
	if err := db.Model(&Bill{}).
		Where("table_id = ? AND status IN ?", tableID, activeBillStatusStrings()).
		Limit(1).
		Pluck("id", &ids).Error; err != nil {
		return false, fmt.Errorf("failed to check active bill: %w", err)
	}
	return len(ids) > 0, nil
}

// UpdateBill updates an existing bill
func UpdateBill(bill *Bill, items []BillItem) error {
	return UpdateBillWithHistory(bill, items, nil)
}

// ErrBillStale reports that the bill's item snapshot changed between the
// caller's read and the locked write; the caller must reload and retry.
var ErrBillStale = errors.New("bill changed since it was loaded")

// UpdateBillWithHistory replaces a bill's items and appends history events
// atomically. bill must carry the snapshot the caller computed the new items
// from (the raw column recorded by GetBillByID, else bill.Items): if another writer changed the snapshot meanwhile the write
// is refused with ErrBillStale instead of silently dropping that writer's
// lines. Payment/status columns are not compared, so a concurrent payment does
// not conflict with an item edit.
func UpdateBillWithHistory(bill *Bill, items []BillItem, historyEvents []BillHistoryEvent) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var current Bill
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "status", "items").First(&current, bill.ID).Error; err != nil {
			return fmt.Errorf("failed to load bill for update: %w", err)
		}
		// Re-check status under lock: the handler validated against a snapshot
		// that may be stale by the time we write. Partial stays editable: guests
		// keep ordering after paying part of a bill. Paid/closed are frozen.
		if current.Status != BillStatusOpen && current.Status != BillStatusPartial {
			return ErrBillNotPayable
		}
		expected := bill.Items
		if bill.loadedItemsRaw != nil {
			expected = *bill.loadedItemsRaw
		}
		if current.Items != expected {
			return ErrBillStale
		}
		if err := updateBillTx(tx, bill, items); err != nil {
			return err
		}
		written := bill.Items
		bill.loadedItemsRaw = &written
		return createBillHistoryEventsTx(tx, bill, historyEvents)
	})
}

// BillItemsMutation edits a bill loaded under FOR UPDATE. It receives the
// locked bill and its current items and returns the new items plus the history
// events to append. It may set totals on bill (e.g. via ApplyBillTotals).
type BillItemsMutation func(bill *Bill, items []BillItem) ([]BillItem, []BillHistoryEvent, error)

// UpdateBillWithHistoryFn applies mutate to the bill's current items while the
// row is locked, so concurrent item writers serialize instead of each writing
// a snapshot that drops the other's lines. Returns ErrBillNotPayable when the
// bill is no longer open or partial.
func UpdateBillWithHistoryFn(billID uint, mutate BillItemsMutation) (*Bill, []BillItem, error) {
	var (
		updated Bill
		items   []BillItem
	)
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&updated, billID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			return fmt.Errorf("failed to load bill for update: %w", err)
		}
		if updated.Status != BillStatusOpen && updated.Status != BillStatusPartial {
			return ErrBillNotPayable
		}
		current, err := billItemsForBillSnapshotConn(tx, updated.ID, updated.Items)
		if err != nil {
			return fmt.Errorf("failed to parse bill items: %w", err)
		}
		next, events, err := mutate(&updated, current)
		if err != nil {
			return err
		}
		if err := updateBillTx(tx, &updated, next); err != nil {
			return err
		}
		items = next
		return createBillHistoryEventsTx(tx, &updated, events)
	})
	if err != nil {
		return nil, nil, err
	}
	return &updated, items, nil
}

// detachVoidedOrderLineTx drops a voided bill line from its kitchen ticket and
// restores that line's inventory. Missing, foreign, or already-cancelled
// orders are left alone. Returns whether the order row changed and whether
// that change cancelled it. Does not remove the bill line.
func detachVoidedOrderLineTx(tx *gorm.DB, bill *Bill, targetItem BillItem, actor, reason string) (bool, bool, error) {
	if targetItem.OrderID == nil {
		return false, false, nil
	}
	var order Order
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, *targetItem.OrderID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, false, nil
		}
		return false, false, fmt.Errorf("failed to lock order for voided line: %w", err)
	}
	if order.BillID != bill.ID || order.Status == OrderStatusOrderCancelled {
		return false, false, nil
	}

	orderItems, err := parseOrderItemsSnapshot(order.Items)
	if err != nil {
		return false, false, err
	}
	kept := make([]OrderItem, 0, len(orderItems))
	for _, orderItem := range orderItems {
		if normalizeBillItemUUID(orderItem.ID) == targetItem.ID {
			continue
		}
		kept = append(kept, orderItem)
	}

	// The line is not on this ticket (merged in under another id, or already
	// trimmed): leave the order and its stock alone. Restoring here would
	// double-count, because a later cancel of the order restores every line
	// still on the ticket.
	if len(kept) == len(orderItems) {
		return false, false, nil
	}

	// No item left on the ticket (do not ignore leftovers). Cancel in this
	// transaction; the bill line is removed by the caller, so do not call
	// removeCancelledOrderBillItemsTx.
	cancelled := len(kept) == 0
	itemsJSON, err := json.Marshal(kept)
	if err != nil {
		return false, false, fmt.Errorf("failed to marshal items: %w", err)
	}
	now := time.Now()
	updates := map[string]interface{}{
		"items":      string(itemsJSON),
		"updated_at": now,
	}
	if cancelled {
		updates["status"] = OrderStatusOrderCancelled
		updates["cancelled_by"] = actor
		updates["cancelled_at"] = now
		updates["cancel_reason"] = reason
	}
	if err := tx.Model(&Order{}).Where("id = ?", order.ID).Updates(updates).Error; err != nil {
		return false, false, fmt.Errorf("failed to update order items after void: %w", err)
	}

	if err := RestoreOrderLineInventoryTx(tx, &order, targetItem.MenuItemID, targetItem.Quantity, actor); err != nil {
		return false, false, err
	}
	return true, cancelled, nil
}

// AdjustBillItem updates a live bill item quantity or voids the line with an audit event.
func AdjustBillItem(
	billID uint,
	itemID string,
	actor string,
	quantity *int,
	voidItem bool,
	reason string,
) (*Bill, []BillItem, error) {
	var updatedBill *Bill
	var updatedItems []BillItem

	err := db.Transaction(func(tx *gorm.DB) error {
		var bill Bill
		// Lock the bill row FOR UPDATE so concurrent
		// PUT /bills/:id/items/:id requests (e.g. staff edit + order-
		// approval flow) serialize. Without this, two concurrent writers
		// read the same pre-edit subtotal snapshot and the later commit
		// silently clobbers the other's recomputed totals. Matches the
		// pattern in ApplyConfirmedPayment / ConfirmPendingAlternativePayment.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&bill, billID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("bill not found")
			}
			return fmt.Errorf("failed to get bill: %w", err)
		}

		if bill.Status != BillStatusOpen {
			return ErrBillNotPayable
		}

		var business Business
		if err := tx.First(&business, bill.BusinessID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("business not found")
			}
			return fmt.Errorf("failed to get business: %w", err)
		}

		var items []BillItem
		if strings.TrimSpace(bill.Items) != "" {
			if err := json.Unmarshal([]byte(bill.Items), &items); err != nil {
				return fmt.Errorf("failed to unmarshal items: %w", err)
			}
		}

		itemIndex := -1
		for i := range items {
			if items[i].ID == itemID {
				itemIndex = i
				break
			}
		}

		if itemIndex == -1 {
			return fmt.Errorf("item not found in bill")
		}

		if voidItem && quantity != nil {
			return fmt.Errorf("cannot void item and update quantity in the same request")
		}

		if !voidItem && quantity == nil {
			return fmt.Errorf("quantity is required when not voiding an item")
		}

		beforeSubtotal := bill.Subtotal
		beforeTotal := bill.TotalAmount
		targetItem := items[itemIndex]
		var historyEvent *BillHistoryEvent

		if voidItem {
			trimmedReason := strings.TrimSpace(reason)
			if trimmedReason == "" {
				return fmt.Errorf("void reason is required")
			}

			items = append(items[:itemIndex], items[itemIndex+1:]...)
			recomputed, err := recomputeBillOfferDiscountsTx(tx, bill.BusinessID, items)
			if err != nil {
				return err
			}
			items = recomputed
			applyBillTotals(&bill, items, &business)

			orderTouched := false
			orderCancelled := false
			if targetItem.OrderID != nil {
				var detachErr error
				orderTouched, orderCancelled, detachErr = detachVoidedOrderLineTx(tx, &bill, targetItem, actor, trimmedReason)
				if detachErr != nil {
					return detachErr
				}
			}

			details := map[string]interface{}{
				"quantity":        targetItem.Quantity,
				"unit_price":      targetItem.Price,
				"subtotal":        targetItem.Subtotal,
				"subtotal_before": beforeSubtotal,
				"subtotal_after":  bill.Subtotal,
				"total_before":    beforeTotal,
				"total_after":     bill.TotalAmount,
			}
			if orderTouched {
				details["order_id"] = *targetItem.OrderID
				details["order_cancelled"] = orderCancelled
			}
			historyEvent = &BillHistoryEvent{
				BillID:     bill.ID,
				BusinessID: bill.BusinessID,
				EventType:  BillHistoryEventItemVoided,
				Actor:      actor,
				Reason:     trimmedReason,
				BillItemID: targetItem.ID,
				ItemName:   targetItem.Name,
				Details:    details,
			}
		} else {
			nextQuantity := *quantity
			if nextQuantity <= 0 {
				return fmt.Errorf("quantity must be greater than zero")
			}

			if nextQuantity == targetItem.Quantity {
				updatedBill = &bill
				updatedItems = items
				return nil
			}

			lineSubtotalBefore := targetItem.Subtotal
			items[itemIndex].Quantity = nextQuantity
			// Keep option surcharges and leave bundle children at $0 — bare
			// Price×Qty was stripping options and inflating bundle checks.
			NormalizeBillItemLineMoney(&items[itemIndex])
			recomputed, err := recomputeBillOfferDiscountsTx(tx, bill.BusinessID, items)
			if err != nil {
				return err
			}
			items = recomputed
			applyBillTotals(&bill, items, &business)

			historyEvent = &BillHistoryEvent{
				BillID:     bill.ID,
				BusinessID: bill.BusinessID,
				EventType:  BillHistoryEventItemQtyUpdated,
				Actor:      actor,
				BillItemID: targetItem.ID,
				ItemName:   targetItem.Name,
				Details: map[string]interface{}{
					"quantity_before":      targetItem.Quantity,
					"quantity_after":       nextQuantity,
					"unit_price":           targetItem.Price,
					"line_subtotal_before": lineSubtotalBefore,
					"line_subtotal_after":  items[itemIndex].Subtotal,
					"subtotal_before":      beforeSubtotal,
					"subtotal_after":       bill.Subtotal,
					"total_before":         beforeTotal,
					"total_after":          bill.TotalAmount,
				},
			}
		}

		if err := updateBillTx(tx, &bill, items); err != nil {
			return err
		}

		if err := createBillHistoryEventsTx(tx, &bill, []BillHistoryEvent{*historyEvent}); err != nil {
			return err
		}

		updatedBill = &bill
		updatedItems = items
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	return updatedBill, updatedItems, nil
}

// CloseBill closes a bill and sets the closed timestamp
func CloseBill(billID uint) error {
	return CloseBillWithHistory(billID, nil)
}

// CloseBillWithHistory closes a bill and appends history events atomically.
//
// The UPDATE is scoped to rows still in the `open` state so a concurrent
// payment-completion that has already transitioned the bill to `paid` (or
// any non-open status) will not be silently reverted to `closed`. When
// RowsAffected is 0 we return ErrBillNotOpen so callers can map the race
// to a 409 instead of reporting misleading success.
func CloseBillWithHistory(billID uint, historyEvents []BillHistoryEvent) error {
	now := time.Now()
	tx := db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var current Bill
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND status = ?", billID, BillStatusOpen).
		First(&current).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrBillNotOpen
		}
		return fmt.Errorf("failed to load bill for close: %w", err)
	}

	var liveKitchen int64
	if err := tx.Model(&Order{}).
		Where("bill_id = ? AND status IN ?", billID, []OrderStatus{
			OrderStatusApproved, OrderStatusInKitchen, OrderStatusOrderReady,
		}).
		Count(&liveKitchen).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to check kitchen tickets: %w", err)
	}
	if liveKitchen > 0 {
		tx.Rollback()
		return ErrBillHasLiveKitchenTickets
	}

	terminalStatus := BillStatusClosed
	remaining := current.TotalAmount - current.PaidAmount
	if remaining < 0 {
		remaining = 0
	}
	if remaining > 0 {
		// Leftover money is settle, not abandon. Closing after a kitchen bump
		// used to flip $37.82 unpaid to abandoned and free the table (#704 / 762).
		tx.Rollback()
		return ErrBillHasUnpaidRemaining
	} else if current.TotalAmount == 0 {
		// Empty $0 walk-ins are voids, not closed service (#712/#704).
		terminalStatus = BillStatusVoided
	}

	closeUpdates := map[string]interface{}{
		"status":    terminalStatus,
		"closed_at": &now,
	}
	// settled_at is accounting evidence that money settled. Only the real
	// close (paid in full, total > 0) earns it — a $0 walk-in voided here
	// moved no money, and stamping it produced "settled" checks nobody paid
	// (#798 leftover 762).
	if terminalStatus == BillStatusClosed {
		closeUpdates["settled_at"] = gorm.Expr("COALESCE(settled_at, ?)", now.UTC())
	}
	result := tx.Model(&Bill{}).
		Where("id = ? AND status = ? AND total_amount = ? AND paid_amount = ?", billID, BillStatusOpen, current.TotalAmount, current.PaidAmount).
		Updates(closeUpdates)
	if result.Error != nil {
		tx.Rollback()
		return fmt.Errorf("failed to close bill: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		tx.Rollback()
		return ErrBillNotOpen
	}
	if err := syncCounterOccupancyForBillTx(tx, billID); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to release counter after closing bill: %w", err)
	}

	var bill Bill
	if err := tx.First(&bill, billID).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to load bill after close: %w", err)
	}
	// A close that waits behind a guest payment request must not leave durable
	// capacity reserved on a terminal bill. Expire pending requests and release
	// split holds in the same transaction as the status transition.
	if err := expirePendingAlternativePaymentRequestsTx(tx, bill.ID, now); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to expire pending alternative payments: %w", err)
	}
	if tx.Dialector.Name() == "postgres" || tx.Migrator().HasTable(&BillSplitShare{}) {
		if _, err := releaseActiveBillSplitSharesTx(tx, now, bill.ID); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to release bill split holds: %w", err)
		}
	}
	// B-7: a closed bill can never have its pending orders approved (409
	// forever) — cancel them in the same transaction.
	if err := cancelPendingOrdersForClosedBillTx(tx, &bill, "system"); err != nil {
		tx.Rollback()
		return err
	}

	if err := createBillHistoryEventsTx(tx, &bill, historyEvents); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}

// AbandonUnpaidOpenBill terminals an unpaid open check as abandoned. Delivery
// walk-outs use this door. Operator CloseBill / CloseBillWithHistory / Liberar
// must not — leftover money after kitchen is settle_required, not abandon
// (#704 / 762). Kitchen and remaining stay gated so a delivered-but-unpaid
// dine-in check cannot be written off through this helper.
func AbandonUnpaidOpenBill(billID uint, actor, reason string) error {
	now := time.Now()
	trimmedReason := strings.TrimSpace(reason)
	if trimmedReason == "" {
		trimmedReason = "unpaid check abandoned"
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var bill Bill
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status = ?", billID, BillStatusOpen).
			First(&bill).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrBillNotOpen
			}
			return fmt.Errorf("failed to lock bill for abandon: %w", err)
		}
		liveKitchen, err := countLiveKitchenTicketsTx(tx, bill.ID)
		if err != nil {
			return err
		}
		if liveKitchen > 0 {
			return ErrBillHasLiveKitchenTickets
		}
		remaining := bill.TotalAmount - bill.PaidAmount
		if remaining < 0 {
			remaining = 0
		}
		if remaining <= 0 {
			return ErrBillHasUnpaidRemaining
		}
		if bill.PaidAmount > 0 {
			return ErrBillHasUnpaidRemaining
		}
		if err := tx.Model(&Bill{}).
			Where("id = ? AND status = ? AND paid_amount = 0", bill.ID, BillStatusOpen).
			Updates(map[string]interface{}{
				"status":       BillStatusAbandoned,
				"abandoned_at": gorm.Expr("COALESCE(abandoned_at, ?)", now),
				"closed_at":    gorm.Expr("COALESCE(closed_at, ?)", now),
				"updated_at":   now,
			}).Error; err != nil {
			return fmt.Errorf("failed to abandon unpaid bill: %w", err)
		}
		event := BillHistoryEvent{
			BillID:     bill.ID,
			BusinessID: bill.BusinessID,
			EventType:  BillHistoryEventBillAbandoned,
			Actor:      actor,
			Reason:     trimmedReason,
			Details: map[string]interface{}{
				"source": "abandon_unpaid_open",
			},
			CreatedAt: now,
		}
		if err := tx.Create(&event).Error; err != nil {
			return fmt.Errorf("failed to record abandon history: %w", err)
		}
		if err := cancelPendingOrdersForClosedBillTx(tx, &bill, actor); err != nil {
			return err
		}
		return nil
	})
}

// Payment operations

var ErrPaymentExceedsRemaining = errors.New("payment amount exceeds remaining balance")
var ErrBillNotPayable = errors.New("bill is not open for payment")

// ErrPaymentRequestAlreadyPending: guest payment requests still waiting for
// staff already reserve the bill's whole open balance (a guest tapping "pay at
// the counter" twice). Distinct from
// ErrPaymentExceedsRemaining so the guest is told staff are on the way rather
// than that the bill changed.
var ErrPaymentRequestAlreadyPending = errors.New("a payment request for this bill is already waiting for staff")
var ErrBillNotOpen = errors.New("bill is not open")
var ErrBillHasLiveKitchenTickets = errors.New("bill has live kitchen tickets")
var ErrBillHasUnpaidRemaining = errors.New("bill has unpaid remaining")
var ErrBillCRMCustomerConflict = errors.New("bill already belongs to a different CRM customer")
var ErrInvalidCRMCustomerID = errors.New("crm customer id must be greater than zero")
var ErrInvalidPaymentAmount = errors.New("payment amount must be greater than zero")
var ErrInvalidTipAmount = errors.New("tip amount cannot be negative")
var ErrPaymentNotFound = errors.New("payment not found")

// ErrPaymentTxHashConflict indicates a payment reference is already owned by another payment.
var ErrPaymentTxHashConflict = errors.New("payment transaction hash conflict")

var ErrAlternativePaymentAlreadyConfirmed = errors.New("alternative payment request already confirmed")
var ErrAlternativePaymentBillMismatch = errors.New("alternative payment request does not belong to bill")
var ErrAlternativePaymentRequestExpired = errors.New("alternative payment request has expired")
var ErrAlternativePaymentRequestNotPending = errors.New("alternative payment request is no longer pending")

type BillCRMCustomerAttachment struct {
	BillID        uint
	CRMCustomerID *uint
}

func AttachCRMCustomerIDToBillIfEmpty(billID uint, customerID uint) (*BillCRMCustomerAttachment, error) {
	if customerID == 0 {
		return nil, ErrInvalidCRMCustomerID
	}

	var attachment BillCRMCustomerAttachment
	err := db.Transaction(func(tx *gorm.DB) error {
		var bill struct {
			ID            uint
			Status        BillStatus
			CRMCustomerID *uint
		}
		if err := tx.Model(&Bill{}).
			Select("id", "status", "crm_customer_id").
			Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&bill, billID).Error; err != nil {
			return err
		}
		if bill.CRMCustomerID != nil {
			if *bill.CRMCustomerID == customerID {
				attachment = BillCRMCustomerAttachment{BillID: bill.ID, CRMCustomerID: bill.CRMCustomerID}
				return nil
			}
			return ErrBillCRMCustomerConflict
		}
		if !billStatusIsActive(bill.Status) {
			return ErrBillNotOpen
		}
		if err := tx.Model(&Bill{}).Where("id = ?", billID).Update("crm_customer_id", customerID).Error; err != nil {
			return err
		}
		bill.CRMCustomerID = &customerID
		attachment = BillCRMCustomerAttachment{BillID: bill.ID, CRMCustomerID: bill.CRMCustomerID}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &attachment, nil
}

func billStatusIsActive(status BillStatus) bool {
	for _, activeStatus := range ActiveBillStatuses() {
		if status == activeStatus {
			return true
		}
	}
	return false
}

var newManualPaymentTxHash = func(billID uint) string {
	return fmt.Sprintf("manual_%d_%s", billID, uuid.NewString())
}

// billPaidInTxHook, when set (main.go wires it to fiscal.EnqueueIssueJobInTx),
// runs INSIDE the settlement transaction whenever a confirmed payment
// transitions a bill to paid — the fiscal transactional outbox. database
// cannot import fiscal (fiscal imports database), so the enqueue is injected.
var billPaidInTxHook func(tx *gorm.DB, bill *Bill, paymentID, alternativePaymentID *uint) error

// SetBillPaidInTxHook installs the in-transaction bill-paid hook. Call once at
// startup before the server accepts traffic; pass nil to clear (tests).
func SetBillPaidInTxHook(hook func(tx *gorm.DB, bill *Bill, paymentID, alternativePaymentID *uint) error) {
	billPaidInTxHook = hook
}

func runBillPaidInTxHook(tx *gorm.DB, bill *Bill, paymentID, alternativePaymentID *uint) error {
	if billPaidInTxHook == nil || bill == nil || bill.Status != BillStatusPaid {
		return nil
	}
	return billPaidInTxHook(tx, bill, paymentID, alternativePaymentID)
}

// FinalizeDirectBillPaidTransitionTx completes the shared lifecycle work for a
// transition that made a bill paid without creating a new payment row (for
// example, a loyalty discount reducing the amount owed to the amount already
// collected). The caller must persist the paid status in the same transaction
// before invoking this function.
//
// It stamps canonical closure/settlement time, releases counter occupancy, and
// invokes the in-transaction paid hook with nil payment identities. Any error
// rolls back the caller's transaction, so a paid row cannot escape without its
// fiscal outbox/lifecycle side effects.
func FinalizeDirectBillPaidTransitionTx(tx *gorm.DB, bill *Bill, settlementTime time.Time) error {
	if tx == nil || bill == nil {
		return errors.New("paid bill transition requires transaction and bill")
	}
	if bill.Status != BillStatusPaid {
		return errors.New("paid bill transition requires paid status")
	}
	if settlementTime.IsZero() {
		settlementTime = time.Now()
	}
	if bill.ClosedAt == nil {
		closedAt := settlementTime
		bill.ClosedAt = &closedAt
	}
	if bill.SettledAt == nil {
		settledAt := bill.ClosedAt.UTC()
		bill.SettledAt = &settledAt
	}
	if err := tx.Model(&Bill{}).Where("id = ? AND status = ?", bill.ID, BillStatusPaid).
		Updates(map[string]interface{}{
			"closed_at":  gorm.Expr("COALESCE(closed_at, ?)", bill.ClosedAt),
			"settled_at": gorm.Expr("COALESCE(settled_at, ?)", bill.SettledAt),
		}).Error; err != nil {
		return fmt.Errorf("stamp paid bill settlement: %w", err)
	}
	if err := syncCounterOccupancyForBillStateTx(
		tx, bill.ID, bill.BusinessID, bill.CounterID, bill.Status,
	); err != nil {
		return fmt.Errorf("release paid bill occupancy: %w", err)
	}
	if err := runBillPaidInTxHook(tx, bill, nil, nil); err != nil {
		return fmt.Errorf("run paid bill hook: %w", err)
	}
	return nil
}

type ConfirmedPaymentInput struct {
	BillID          uint
	PayerAddr       string
	Amount          int64
	TipAmount       int64
	TxHash          string
	Status          PaymentStatus
	PaymentMethod   string
	SourceChain     string
	SourceToken     string
	SettlementChain string
	LifiRouteID     string
	// RefundDestination is verified on-chain/signature evidence captured before
	// settlement. When present it is written in the same transaction as the
	// Payment so a process crash cannot leave a confirmed crypto payment without
	// its refund destination.
	RefundDestination *PaymentRefundDestination
	// BlockNumber / BlockHash pin a confirmed crypto settlement to the exact
	// block it landed in, so the reorg reconciler can later re-read the canonical
	// chain and detect an orphaned credit. Nil for non-crypto /
	// evidence-less settlements. Written atomically with the Payment row.
	BlockNumber *int64
	BlockHash   *string
	// CryptoQuoteID / CryptoQuoteExactMicrounits bind a guest crypto
	// settlement to its persisted quote. When set, the quote is
	// consumed (active -> consumed) in this transaction right after the payment
	// insert, so a quote settles at most once; a quote that is no longer active
	// rolls the whole settlement back with ErrCryptoQuoteUnavailable. Zero for
	// every non-quote settlement path.
	CryptoQuoteID              uint
	CryptoQuoteExactMicrounits int64
	// PayerGuestSession is the guestsession.Fingerprint of the guest browser
	// session that paid. Nil for staff, webhook and
	// operator settlements. It is attribution only: it never takes part in
	// idempotent replay matching (PaymentMatchesConfirmedInput).
	PayerGuestSession *string
}

func effectivePaymentStatus(status PaymentStatus) PaymentStatus {
	if status == "" {
		return PaymentStatusConfirmed
	}
	return status
}

func normalizedPaymentMethod(method string) string {
	method = strings.ToLower(strings.TrimSpace(method))
	if method == "" {
		return "crypto"
	}
	return method
}

func effectiveSettlementChain(chain string) string {
	if chain == "" {
		return "base"
	}
	return chain
}

// PaymentMatchesConfirmedInput checks whether an existing payment is the same
// settlement attempt represented by input after applying database defaults.
func PaymentMatchesConfirmedInput(payment *Payment, input ConfirmedPaymentInput) bool {
	if payment == nil {
		return false
	}
	return payment.Amount == input.Amount &&
		payment.TipAmount == input.TipAmount &&
		effectivePaymentStatus(payment.Status) == effectivePaymentStatus(input.Status) &&
		normalizedPaymentMethod(payment.PaymentMethod) == normalizedPaymentMethod(input.PaymentMethod) &&
		payment.PayerAddr == input.PayerAddr &&
		payment.SourceChain == input.SourceChain &&
		payment.SourceToken == input.SourceToken &&
		effectiveSettlementChain(payment.SettlementChain) == effectiveSettlementChain(input.SettlementChain) &&
		payment.LifiRouteId == input.LifiRouteID
}

const billPaymentAmountTolerance = 0

// applyBillPaymentAmounts applies a payment to a bill in-memory and persists
// the amount/status changes atomically. When the payment transitions the bill
// to a terminal paid state, the optional closingStaffID stamps
// closed_by_staff_id and closed_at in the same UPDATE — eliminating the race
// where a handler-level post-hoc stamp could misattribute a previous
// auto-closed bill to an unrelated staff member who hit the endpoint later.
//
// closingStaffID may be nil (webhook / guest / owner paths). In that case the
// attribution columns stay untouched, which is the intended semantic for
// non-staff-driven closures.
func applyBillPaymentAmounts(tx *gorm.DB, bill *Bill, paymentAmount, tipAmount int64, closingStaffID *uint, settlementTime time.Time) error {
	if paymentAmount <= 0 {
		return ErrInvalidPaymentAmount
	}
	if tipAmount < 0 {
		return ErrInvalidTipAmount
	}
	// Reject every terminal state. Voided is terminal but (unlike Paid/Closed)
	// still has remaining = TotalAmount > 0, so without this guard a late
	// crypto/webhook/manual payment would silently resurrect a deliberately
	// cancelled bill. Mirrors the split-hold guard in HoldBillSplitShare.
	if bill.Status == BillStatusPaid || bill.Status == BillStatusClosed || bill.Status == BillStatusVoided {
		return ErrBillNotPayable
	}

	remaining := bill.TotalAmount - bill.PaidAmount
	if paymentAmount > remaining {
		return ErrPaymentExceedsRemaining
	}

	bill.PaidAmount += paymentAmount
	bill.TipAmount += tipAmount

	if bill.PaidAmount == bill.TotalAmount {
		bill.Status = BillStatusPaid
	} else if bill.PaidAmount > bill.TotalAmount {
		bill.Status = BillStatusPaid
	} else if bill.PaidAmount > 0 {
		bill.Status = BillStatusPartial
	}

	updates := map[string]interface{}{
		"paid_amount": bill.PaidAmount,
		"tip_amount":  bill.TipAmount,
		"status":      bill.Status,
	}

	// Attribute the closure atomically with the status transition so a later
	// handler call can't misattribute to an unrelated staff member.
	if bill.Status == BillStatusPaid {
		if bill.ClosedAt == nil {
			now := settlementTime
			if now.IsZero() {
				now = time.Now()
			}
			bill.ClosedAt = &now
			updates["closed_at"] = &now
		}
		if bill.SettledAt == nil && bill.ClosedAt != nil {
			settledAt := bill.ClosedAt.UTC()
			bill.SettledAt = &settledAt
		}
		if closingStaffID != nil && bill.ClosedByStaffID == nil {
			bill.ClosedByStaffID = closingStaffID
			updates["closed_by_staff_id"] = closingStaffID
		}
	}

	if status, ok := billStatusUpdate(updates["status"]); ok && billStatusIsSettled(status) {
		settledAt := time.Now().UTC()
		if closedAt, ok := billUpdateTime(updates["closed_at"]); ok {
			settledAt = closedAt.UTC()
		}
		updates["settled_at"] = gorm.Expr("COALESCE(settled_at, ?)", settledAt)
	}
	if err := tx.Model(&Bill{}).Where("id = ?", bill.ID).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to update bill payment: %w", err)
	}
	if err := syncCounterOccupancyForBillStateTx(
		tx,
		bill.ID,
		bill.BusinessID,
		bill.CounterID,
		bill.Status,
	); err != nil {
		return fmt.Errorf("failed to update bill payment: %w", err)
	}

	return nil
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") ||
		strings.Contains(message, "duplicate key") ||
		strings.Contains(message, "sqlstate 23505")
}

// IsUniqueConstraintError reports whether err is a unique/duplicate-key
// violation. Exported so callers in other packages (e.g. the Stripe checkout
// reconciliation path) can recover from a concurrent double-insert race guarded
// by a unique index rather than surfacing a raw create error.
func IsUniqueConstraintError(err error) bool {
	return isUniqueConstraintError(err)
}

// isActiveBillIndexViolation reports whether err is a unique-constraint failure
// originating from an active-bill occupancy index. It is deliberately narrow
// so a bill_number collision is not misclassified.
func isActiveBillIndexViolation(err error) bool {
	if !isUniqueConstraintError(err) {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "idx_bills_active_per_table") ||
		strings.Contains(message, "idx_bills_active_per_counter") ||
		strings.Contains(message, "table_id") ||
		strings.Contains(message, "counter_id")
}

func applyConfirmedPaymentTx(tx *gorm.DB, input ConfirmedPaymentInput, closingStaffID *uint, txHash string, generatedTxHash bool, now time.Time) (*Bill, *Payment, bool, error) {
	var updatedBill Bill
	// One on-chain transfer is one ledger key: EVM-shaped hashes are folded to
	// canonical 0x+lowercase before the dedupe lookup and the insert, so a
	// re-spelled hash (case, missing 0x) cannot settle a second bill. Provider
	// references (Stripe ids, synthetic keys) pass through untouched.
	txHash = txhash.NormalizeReference(txHash)
	if input.TipAmount < 0 {
		return nil, nil, false, ErrInvalidTipAmount
	}

	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&updatedBill, input.BillID).Error; err != nil {
		return nil, nil, false, fmt.Errorf("failed to lock bill: %w", err)
	}
	previousPaidAmount := updatedBill.PaidAmount
	effectiveStatus := effectivePaymentStatus(input.Status)

	if txHash != "" && !generatedTxHash {
		var existing Payment
		err := tx.Where("tx_hash = ?", txHash).First(&existing).Error
		if err == nil {
			if existing.BillID != input.BillID || !PaymentMatchesConfirmedInput(&existing, input) {
				return nil, nil, false, fmt.Errorf("%w: %s", ErrPaymentTxHashConflict, txHash)
			}
			if input.RefundDestination != nil {
				dest := *input.RefundDestination
				dest.PaymentID = existing.ID
				if err := persistPaymentRefundDestinationTx(tx, &dest); err != nil {
					return nil, nil, false, fmt.Errorf("failed to persist refund destination: %w", err)
				}
			}
			return &updatedBill, &existing, false, nil
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, false, fmt.Errorf("failed to check existing payment: %w", err)
		}
	}

	if err := applyBillPaymentAmounts(tx, &updatedBill, input.Amount, input.TipAmount, closingStaffID, now); err != nil {
		return nil, nil, false, err
	}

	payment := &Payment{
		BillID:          input.BillID,
		PayerAddr:       input.PayerAddr,
		Amount:          input.Amount,
		TipAmount:       input.TipAmount,
		TxHash:          txHash,
		Status:          input.Status,
		PaymentMethod:   input.PaymentMethod,
		SourceChain:     input.SourceChain,
		SourceToken:     input.SourceToken,
		SettlementChain: input.SettlementChain,
		LifiRouteId:     input.LifiRouteID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if input.PayerGuestSession != nil && strings.TrimSpace(*input.PayerGuestSession) != "" {
		payment.PayerGuestSession = input.PayerGuestSession
	}
	if isCryptoSettlementMethod(input.PaymentMethod) {
		if wallet := strings.TrimSpace(updatedBill.SettlementAddr); wallet != "" {
			payment.SettlementAddr = &wallet
		}
	}
	payment.Status = effectiveStatus
	if payment.Status == PaymentStatusConfirmed {
		payment.ConfirmedAt = &now
		// Persist reorg evidence only for a confirmed settlement carrying a block
		// hash — this is what the reconciler re-reads. Manual/non-crypto closes
		// leave it nil (never a reorg candidate).
		if input.BlockHash != nil && strings.TrimSpace(*input.BlockHash) != "" {
			payment.BlockNumber = input.BlockNumber
			payment.BlockHash = input.BlockHash
		}
	}
	if strings.TrimSpace(payment.PaymentMethod) == "" {
		payment.PaymentMethod = "crypto"
	}
	if payment.SettlementChain == "" {
		payment.SettlementChain = "base"
	}

	if err := tx.Create(payment).Error; err != nil {
		if txHash != "" && isUniqueConstraintError(err) {
			return nil, nil, false, fmt.Errorf("%w: %s", ErrPaymentTxHashConflict, txHash)
		}
		return nil, nil, false, fmt.Errorf("failed to create payment: %w", err)
	}
	if input.CryptoQuoteID != 0 {
		if err := consumeCryptoPaymentQuoteTx(tx, input.CryptoQuoteID, input.BillID, input.CryptoQuoteExactMicrounits, txHash, payment.ID, now); err != nil {
			return nil, nil, false, err
		}
	}
	if input.RefundDestination != nil {
		dest := *input.RefundDestination
		dest.PaymentID = payment.ID
		if err := persistPaymentRefundDestinationTx(tx, &dest); err != nil {
			return nil, nil, false, fmt.Errorf("failed to persist refund destination: %w", err)
		}
	}

	revenueDeltaCents := input.Amount
	tipDeltaCents := input.TipAmount
	if payment.Status == PaymentStatusReversed {
		revenueDeltaCents = 0
		tipDeltaCents = 0
	}
	recognizedBillDelta := int64(0)
	if payment.Status == PaymentStatusConfirmed && previousPaidAmount <= 0 && updatedBill.PaidAmount > 0 {
		recognizedBillDelta = 1
	}
	if err := RecordPaymentMilestonesTx(tx, updatedBill.BusinessID, updatedBill.ID, revenueDeltaCents, tipDeltaCents, recognizedBillDelta, updatedBill.Status); err != nil {
		return nil, nil, false, fmt.Errorf("failed to record payment milestones: %w", err)
	}
	if err := runBillPaidInTxHook(tx, &updatedBill, &payment.ID, nil); err != nil {
		return nil, nil, false, fmt.Errorf("failed to enqueue fiscal outbox job: %w", err)
	}

	return &updatedBill, payment, true, nil
}

func ApplyConfirmedPayment(input ConfirmedPaymentInput, closingStaffID *uint) (*Bill, bool, error) {
	txHash := strings.TrimSpace(input.TxHash)
	generatedTxHash := false
	if txHash == "" {
		txHash = newManualPaymentTxHash(input.BillID)
		generatedTxHash = true
	}
	now := time.Now()

	var updatedBill *Bill
	applied := false
	err := db.Transaction(func(tx *gorm.DB) error {
		bill, _, wasApplied, err := applyConfirmedPaymentTx(tx, input, closingStaffID, txHash, generatedTxHash, now)
		if err != nil {
			return err
		}
		updatedBill = bill
		applied = wasApplied
		return nil
	})
	if err != nil {
		return nil, false, err
	}

	return updatedBill, applied, nil
}

// ApplyConfirmedPaymentInTx applies a confirmed payment inside an existing
// transaction so callers can co-commit related ledger/accounting rows.
// Returns (bill, applied, err) — applied is false on idempotent tx_hash hits.
func ApplyConfirmedPaymentInTx(tx *gorm.DB, input ConfirmedPaymentInput, closingStaffID *uint) (*Bill, bool, error) {
	if tx == nil {
		return nil, false, errors.New("tx is nil")
	}
	txHash := strings.TrimSpace(input.TxHash)
	generatedTxHash := false
	if txHash == "" {
		txHash = newManualPaymentTxHash(input.BillID)
		generatedTxHash = true
	}
	now := time.Now()
	bill, _, applied, err := applyConfirmedPaymentTx(tx, input, closingStaffID, txHash, generatedTxHash, now)
	return bill, applied, err
}

func CreateConfirmedAlternativePayment(payment *AlternativePayment, closingStaffID *uint) (*Bill, bool, error) {
	var updatedBill Bill
	now := time.Now()
	applied := false

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&updatedBill, payment.BillID).Error; err != nil {
			return fmt.Errorf("failed to lock bill: %w", err)
		}
		previousPaidAmount := updatedBill.PaidAmount

		// Idempotency. When the request carries a per-attempt key, dedupe on it:
		// a retry reuses the key (skip), but two genuinely separate tenders of
		// the same amount carry different keys and are both credited. Without a
		// key, fall back to the legacy content-equality dedup so a keyless
		// double-submit still can't double-credit.
		var existing AlternativePayment
		if payment.IdempotencyKey != "" {
			if err := tx.Where("bill_id = ? AND idempotency_key = ?", payment.BillID, payment.IdempotencyKey).
				First(&existing).Error; err == nil {
				return nil // already processed (retry of the same request)
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("failed to check existing alternative payment: %w", err)
			}
		} else if err := tx.Where("bill_id = ? AND participant_addr = ? AND amount = ? AND payment_method = ? AND status = ?",
			payment.BillID, payment.ParticipantAddr, payment.Amount, payment.PaymentMethod, AltPaymentStatusConfirmed).
			First(&existing).Error; err == nil {
			return nil // already processed
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("failed to check existing alternative payment: %w", err)
		}

		if _, err := releaseExpiredBillSplitSharesTx(tx, now, &updatedBill.ID); err != nil {
			return fmt.Errorf("failed to release expired split holds: %w", err)
		}
		activeHeldCents, err := activeHeldSplitCentsTx(tx, updatedBill.ID, now)
		if err != nil {
			return fmt.Errorf("failed to total active split holds: %w", err)
		}
		if activeHeldCents > 0 {
			remaining := updatedBill.TotalAmount - updatedBill.PaidAmount
			availableWithoutHeld := remaining - activeHeldCents
			if availableWithoutHeld < 0 {
				availableWithoutHeld = 0
			}
			if payment.Amount > availableWithoutHeld {
				if payment.Amount != remaining {
					return ErrSplitAmountUnavailable
				}
				if _, err := releaseActiveBillSplitSharesTx(tx, now, updatedBill.ID); err != nil {
					return fmt.Errorf("failed to release split holds for cashier override: %w", err)
				}
			}
		}

		if err := applyBillPaymentAmounts(tx, &updatedBill, payment.Amount, payment.TipAmountCents, closingStaffID, now); err != nil {
			return err
		}

		payment.Status = AltPaymentStatusConfirmed
		payment.UpdatedAt = now
		if payment.CreatedAt.IsZero() {
			payment.CreatedAt = now
		}
		if payment.ConfirmedAt == nil {
			payment.ConfirmedAt = &now
		}
		if payment.BillAmountCents == 0 && payment.TipAmountCents == 0 {
			payment.BillAmountCents = payment.Amount
		}

		if err := tx.Create(payment).Error; err != nil {
			return fmt.Errorf("failed to create alternative payment: %w", err)
		}
		if err := AttachCashRegisterMovementForAlternativePaymentTx(
			tx,
			updatedBill.BusinessID,
			*payment,
			CashRegisterMovementTypeCashSale,
			alternativePaymentTotalCashCents(*payment),
			cashRegisterActorForAlternativePayment(*payment, closingStaffID, "system:cash_payment"),
			now,
		); err != nil {
			return fmt.Errorf("failed to attach cash register movement: %w", err)
		}
		recognizedBillDelta := int64(0)
		if previousPaidAmount <= 0 && updatedBill.PaidAmount > 0 {
			recognizedBillDelta = 1
		}
		if err := RecordPaymentMilestonesTx(tx, updatedBill.BusinessID, updatedBill.ID, payment.Amount, payment.TipAmountCents, recognizedBillDelta, updatedBill.Status); err != nil {
			return fmt.Errorf("failed to record alternative payment milestones: %w", err)
		}
		if err := runBillPaidInTxHook(tx, &updatedBill, nil, &payment.ID); err != nil {
			return fmt.Errorf("failed to enqueue fiscal outbox job: %w", err)
		}
		applied = true

		return nil
	})
	if err != nil {
		return nil, false, err
	}

	return &updatedBill, applied, nil
}

func ConfirmPendingAlternativePayment(billID, requestID uint, confirmedBy string, closingStaffID *uint) (*AlternativePayment, *Bill, error) {
	var payment AlternativePayment
	var updatedBill Bill
	now := time.Now()

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&payment, requestID).Error; err != nil {
			return fmt.Errorf("failed to get alternative payment request: %w", err)
		}
		if payment.BillID != billID {
			return ErrAlternativePaymentBillMismatch
		}
		if payment.Status == AltPaymentStatusConfirmed {
			return ErrAlternativePaymentAlreadyConfirmed
		}
		if payment.Status != AltPaymentStatusPending {
			return ErrAlternativePaymentRequestNotPending
		}
		if AlternativePaymentRequestExpired(payment, now) {
			return ErrAlternativePaymentRequestExpired
		}

		splitShare, err := lockBillSplitShareForAlternativePaymentTx(tx, payment, now)
		if err != nil {
			return err
		}
		// A guest tip on the pending request stands unless the held share
		// declares a larger one. A zero share tip must not wipe it.
		tipCents := payment.TipAmountCents
		if splitShare != nil && splitShare.TipCents > tipCents {
			tipCents = splitShare.TipCents
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&updatedBill, billID).Error; err != nil {
			return fmt.Errorf("failed to lock bill: %w", err)
		}
		previousPaidAmount := updatedBill.PaidAmount
		if err := applyBillPaymentAmounts(tx, &updatedBill, payment.Amount, tipCents, closingStaffID, now); err != nil {
			return err
		}

		payment.Status = AltPaymentStatusConfirmed
		payment.ConfirmedBy = confirmedBy
		payment.ConfirmedAt = &now
		payment.UpdatedAt = now
		payment.BillAmountCents = payment.Amount
		payment.TipAmountCents = tipCents

		if err := tx.Model(&AlternativePayment{}).Where("id = ?", payment.ID).Updates(map[string]interface{}{
			"status":            payment.Status,
			"confirmed_by":      payment.ConfirmedBy,
			"confirmed_at":      payment.ConfirmedAt,
			"updated_at":        payment.UpdatedAt,
			"bill_amount_cents": payment.BillAmountCents,
			"tip_amount_cents":  payment.TipAmountCents,
		}).Error; err != nil {
			return fmt.Errorf("failed to confirm alternative payment request: %w", err)
		}
		if err := markBillSplitShareSettledByAlternativePaymentTx(tx, splitShare, payment, now); err != nil {
			return err
		}
		if err := AttachCashRegisterMovementForAlternativePaymentTx(
			tx,
			updatedBill.BusinessID,
			payment,
			CashRegisterMovementTypeCashSale,
			alternativePaymentTotalCashCents(payment),
			cashRegisterActorForAlternativePayment(payment, closingStaffID, "system:cash_payment"),
			now,
		); err != nil {
			return fmt.Errorf("failed to attach cash register movement: %w", err)
		}
		recognizedBillDelta := int64(0)
		if previousPaidAmount <= 0 && updatedBill.PaidAmount > 0 {
			recognizedBillDelta = 1
		}
		if err := RecordPaymentMilestonesTx(tx, updatedBill.BusinessID, updatedBill.ID, payment.Amount, tipCents, recognizedBillDelta, updatedBill.Status); err != nil {
			return fmt.Errorf("failed to record alternative payment milestones: %w", err)
		}
		if err := runBillPaidInTxHook(tx, &updatedBill, nil, &payment.ID); err != nil {
			return fmt.Errorf("failed to enqueue fiscal outbox job: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	return &payment, &updatedBill, nil
}

// GetPaymentByTxHash retrieves a payment by its transaction hash
func GetPaymentByTxHash(txHash string) (*Payment, error) {
	var payment Payment
	if err := db.Where("tx_hash = ?", txhash.NormalizeReference(txHash)).First(&payment).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPaymentNotFound
		}
		return nil, fmt.Errorf("failed to get payment: %w", err)
	}
	return &payment, nil
}

// Order operations

var ErrOrderNotFound = errors.New("order not found")

// CreateOrderTx inserts an order (with its items serialized) on the provided
// gorm handle. Passing a caller's transaction lets the order and a plugin
// notification outbox row commit atomically (transactional outbox, N-1); passing
// the package db is the standalone create.
func CreateOrderTx(tx *gorm.DB, order *Order, items []OrderItem) error {
	if tx == nil {
		return gorm.ErrInvalidDB
	}
	stamp := order.CreatedAt
	if stamp.IsZero() {
		stamp = time.Now().UTC()
	}
	if err := stampOrderItemsForPersist(tx, items, order.BusinessID, stamp); err != nil {
		return err
	}
	// Convert items to JSON string for database storage
	itemsJSON, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("failed to marshal items: %w", err)
	}
	order.Items = string(itemsJSON)

	// Select only the columns Order owns so a hydrated Bill/Business relation on
	// the struct is never upserted as a side effect of the insert.
	if err := tx.Omit("Bill", "Business").Create(order).Error; err != nil {
		return fmt.Errorf("failed to create order: %w", err)
	}
	return nil
}

func GetOrderByRequestIdentity(billID uint, createdBy string, clientRequestID string) (*Order, []OrderItem, error) {
	var order Order
	if err := db.
		Preload("Bill").
		Preload("Business", preloadOrderListBusinessCurrency).
		Where("bill_id = ? AND created_by = ? AND client_request_id = ?", billID, createdBy, clientRequestID).
		First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrOrderNotFound
		}
		return nil, nil, fmt.Errorf("failed to get order: %w", err)
	}

	if err := hydrateOrderSnapshotForRead(&order); err != nil {
		return nil, nil, err
	}
	items, err := parseOrderItemsSnapshot(order.Items)
	if err != nil {
		return nil, nil, err
	}

	return &order, items, nil
}

// GetOrderByBusinessRequestIdentityTx resolves guest checkout replay identity
// without requiring the caller to already know the bill id.
func GetOrderByBusinessRequestIdentityTx(tx *gorm.DB, businessID uint, createdBy, clientRequestID string) (*Order, []OrderItem, error) {
	if tx == nil {
		return nil, nil, gorm.ErrInvalidDB
	}
	var order Order
	if err := tx.Where(
		"business_id = ? AND created_by = ? AND client_request_id = ?",
		businessID, createdBy, clientRequestID,
	).First(&order).Error; err != nil {
		return nil, nil, err
	}
	var items []OrderItem
	if order.Items != "" {
		if err := json.Unmarshal([]byte(order.Items), &items); err != nil {
			return nil, nil, fmt.Errorf("failed to unmarshal replay order items: %w", err)
		}
	}
	return &order, items, nil
}

// GetOrderByID retrieves an order by its ID with items parsed
func GetOrderByID(id uint) (*Order, []OrderItem, error) {
	var order Order
	if err := db.Preload("Bill").Preload("Business", preloadOrderListBusinessCurrency).First(&order, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, fmt.Errorf("order not found")
		}
		return nil, nil, fmt.Errorf("failed to get order: %w", err)
	}

	if err := hydrateOrderSnapshotForRead(&order); err != nil {
		return nil, nil, err
	}
	items, err := parseOrderItemsSnapshot(order.Items)
	if err != nil {
		return nil, nil, err
	}

	return &order, items, nil
}

// GetOrderBusinessIDByID retrieves only the business id needed for
// route-scoped authorization before the mutation transaction reloads the order.
func GetOrderBusinessIDByID(id uint) (uint, error) {
	var row struct {
		BusinessID uint
	}
	err := db.Model(&Order{}).
		Select("business_id").
		Where("id = ?", id).
		Take(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, fmt.Errorf("order not found")
		}
		return 0, fmt.Errorf("failed to get order business id: %w", err)
	}
	return row.BusinessID, nil
}

// GetBillItemsPreferRelational reads bill items from the relational table first,
// falling back to JSON for legacy bills without relational data.
func GetBillItemsPreferRelational(billID uint) ([]BillItem, error) {
	return billItemsForBillSnapshotLazyByBillID(billID)
}

// GetBillByIDLean retrieves a bill row by ID and nothing else: no Business/
// Table/Payments preloads, no managed alternative-payment load, and no
// bill_items snapshot second query. It issues exactly ONE query and returns the
// fresh SCALAR columns of the bill.
//
// This is the settle-path reload (OF-03): after a settlement write the in-memory
// bill is a lean projection and is stale, so the paid-bill side effects reload
// it. Their consumers — recordCRMSettlementVisitForPaidBill (bill.ID/Status),
// sendPaymentCompletionEmails (bill.BusinessID/ID/TotalAmount/BillNumber/
// CRMCustomerID; it re-fetches business/items/customer itself),
// enqueueFiscalJobForPaidBill (bill.ID/Status), plus the milestone flush, SSE
// publish, split-state republish, operational alert, and JSON response — read
// ONLY scalar columns. None reads a preloaded relation, so the heavy aggregate
// hydration GetBillByID performs is pure over-fetch here.
//
// The returned []BillItem is always empty (the signature is kept stable for
// existing callers, which all discard it). Use GetBillByID when a caller truly
// needs the items snapshot or the relations.
func GetBillByIDLean(id uint) (*Bill, []BillItem, error) {
	var bill Bill
	if err := db.First(&bill, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, fmt.Errorf("bill not found")
		}
		return nil, nil, fmt.Errorf("failed to get bill: %w", err)
	}
	return &bill, nil, nil
}

// GuestOrdersByBillLimit caps the public guest-orders poll. A real table
// rarely passes a dozen rounds; the cap only stops one pathological bill from
// making every guest poll unbounded.
const GuestOrdersByBillLimit = 100

// GetOrdersByBillID retrieves the newest GuestOrdersByBillLimit orders of a
// bill, projected to the guestOrderView columns. It preloads
// Business with the narrow currency projection only (id/default_currency/
// display_currency). Operator JSON uses Order.MarshalJSON; the public guest
// poll uses guestOrderView, which calls Order.ResolvedCurrency() so the
// same projection is consumed rather than discarded. This is a PUBLIC
// guest-poll path, so the full Business row must never be hydrated (X-1).
// Intentionally does NOT preload Bill: no order-list caller reads
// order.bill, so hydrating the parent bill (and its heavy items JSON)
// per order is pure over-fetch. Proven by orders_bill_lookup_perf_test.go.
func GetOrdersByBillID(billID uint) ([]Order, error) {
	var orders []Order
	// Guest poll projection: exactly the guestOrderView fields plus
	// business_id for the currency preload. quote_snapshot (jsonb) and the
	// staff actor columns never leave the database on this public read.
	if err := db.
		Select("id", "bill_id", "business_id", "order_number", "status", "notes",
			"items", "cancel_reason", "created_at", "updated_at", "approved_at", "cancelled_at").
		Preload("Business", preloadOrderListBusinessCurrency).
		Where("bill_id = ?", billID).
		Order("created_at DESC").
		Limit(GuestOrdersByBillLimit).
		Find(&orders).Error; err != nil {
		return nil, fmt.Errorf("failed to get orders: %w", err)
	}
	if err := hydrateOrdersForRead(db, orders); err != nil {
		return nil, err
	}
	return orders, nil
}

// preloadOrderListBillSummary projects the kitchen/pending-order card fields
// only. Settlement and tipping wallets intentionally stay off this preload —
// order list is polled by KDS/owner dashboards and never needs payment
// destinations (those live on dedicated bill/payment endpoints). (FIND-039)
func preloadOrderListBillSummary(tx *gorm.DB) *gorm.DB {
	return tx.Select(
		"id",
		"business_id",
		"table_id",
		"counter_id",
		"bill_number",
		"notes",
		"subtotal",
		"tax_amount",
		"service_fee_amount",
		"total_amount",
		"paid_amount",
		"tip_amount",
		"status",
		"created_by_staff_id",
		"closed_by_staff_id",
		"crm_customer_id",
		"created_at",
		"updated_at",
		"closed_at",
		"feedback_email_sent_at",
	)
}

// orderListColumns projects every Order column the operator order list emits.
// quote_snapshot (per-order jsonb checkout quote, json:"-") and
// client_request_id (json:"-") never leave the database on this KDS/dashboard
// poll. Qualified because ActiveBillsOnly joins bills.
var orderListColumns = []string{
	"orders.id", "orders.bill_id", "orders.business_id", "orders.order_number",
	"orders.status", "orders.created_by", "orders.approved_by", "orders.cancelled_by",
	"orders.notes", "orders.items", "orders.cancel_reason", "orders.created_at",
	"orders.updated_at", "orders.approved_at", "orders.cancelled_at", "orders.kitchen_acked_at",
}

func preloadOrderListBusinessCurrency(tx *gorm.DB) *gorm.DB {
	return tx.Select("id", "default_currency", "display_currency")
}

// preloadOrderListTableName projects just enough of the joined table to render a
// friendly location label on kitchen tickets (KDS). table_id is already selected
// in the bill summary; the 0-sentinel (counter/delivery bills) simply resolves to
// a zero-value Table the frontend treats as "no table".
func preloadOrderListTableName(tx *gorm.DB) *gorm.DB {
	return tx.Select("id", "name")
}

func parseOrderStatusFilters(status string) []string {
	parts := strings.Split(status, ",")
	statuses := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		statuses = append(statuses, trimmed)
	}
	return statuses
}

// GetOrdersByBusinessIDPaginated retrieves orders with pagination and optional status filter.
func GetOrdersByBusinessIDPaginated(businessID uint, status string, p PaginationParams, opts ...OrderListOptions) (*PaginatedResult[Order], error) {
	var opt OrderListOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	p = p.Normalize()
	base := db.Model(&Order{}).Where("orders.business_id = ?", businessID)
	if statuses := parseOrderStatusFilters(status); len(statuses) > 0 {
		base = base.Where("orders.status IN ?", statuses)
	}
	// Exclude orders whose linked delivery has reached a terminal dead state
	// (cancelled/failed — e.g. swept by ExpireUnpaidDeliveries after the unpaid
	// window). Such orders are no longer actionable and must not linger in the
	// approval/kitchen queue (audit M7). NOT EXISTS keeps dine-in orders — which
	// have no delivery_orders row — untouched.
	base = base.Where(`NOT EXISTS (
		SELECT 1 FROM delivery_orders d
		WHERE d.order_id = orders.id
		  AND d.status IN (?)
	)`, []string{string(DeliveryStatusCancelled), string(DeliveryStatusFailed)})
	if opt.ActiveBillsOnly {
		base = base.Joins("JOIN bills ON bills.id = orders.bill_id").
			Where("bills.status IN ? OR orders.status IN ?", ActiveBillStatuses(), KitchenLiveOrderStatuses())
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count orders: %w", err)
	}
	// Default DESC (newest first) preserves the legacy history-list ordering.
	// The kitchen board asks for ASC so that when the row cap is hit the query
	// keeps the OLDEST active orders — the FIFO-critical end a cook works first
	// — instead of dropping them (audit MED). created_at ties break on id in the
	// same direction so paging is deterministic across the cap boundary.
	orderClause := "orders.created_at DESC, orders.id DESC"
	if opt.SortAsc {
		orderClause = "orders.created_at ASC, orders.id ASC"
	}
	var orders []Order
	if err := base.
		Select(orderListColumns).
		Preload("Bill", preloadOrderListBillSummary).
		Preload("Bill.Table", preloadOrderListTableName).
		Preload("Business", preloadOrderListBusinessCurrency).
		Order(orderClause).
		Offset(p.Offset()).
		Limit(p.PageSize).
		Find(&orders).Error; err != nil {
		return nil, fmt.Errorf("failed to get orders: %w", err)
	}
	totalPages := int(total) / p.PageSize
	if int(total)%p.PageSize != 0 {
		totalPages++
	}

	// Attach delivery linkage for guest-delivery orders in one batched query
	// (pending-queue cards render address/expiry without N+1 lookups).
	orderIDs := make([]uint, 0, len(orders))
	for i := range orders {
		orderIDs = append(orderIDs, orders[i].ID)
	}
	if len(orderIDs) > 0 {
		type deliveryMetaRow struct {
			OrderID uint
			// OrderDeliveryMeta fields inlined for explicit GORM scan mapping
			DeliveryID       uint
			DeliveryNumber   string
			DeliveryStatus   string
			CustomerName     string
			CustomerPhone    string
			Street           string
			City             string
			PaymentExpiresAt *time.Time
		}
		var metas []deliveryMetaRow
		if err := db.Table("delivery_orders").
			Select("order_id, id as delivery_id, delivery_number, status as delivery_status, customer_name, customer_phone, delivery_street as street, delivery_city as city, payment_expires_at").
			Where("order_id IN ?", orderIDs).
			Scan(&metas).Error; err != nil {
			// Best-effort enrichment: cards degrade to dine-in styling, data is intact.
			log.Printf("orders list: delivery meta enrichment failed (business=%d): %v", businessID, err)
		}
		byOrder := make(map[uint]deliveryMetaRow, len(metas))
		for _, m := range metas {
			byOrder[m.OrderID] = m
		}
		for i := range orders {
			if m, ok := byOrder[orders[i].ID]; ok {
				orders[i].Delivery = &OrderDeliveryMeta{
					DeliveryID:       m.DeliveryID,
					DeliveryNumber:   m.DeliveryNumber,
					DeliveryStatus:   m.DeliveryStatus,
					CustomerName:     m.CustomerName,
					CustomerPhone:    m.CustomerPhone,
					Street:           m.Street,
					City:             m.City,
					PaymentExpiresAt: m.PaymentExpiresAt,
				}
			}
		}
	}

	if err := hydrateOrdersForRead(db, orders); err != nil {
		return nil, err
	}
	return &PaginatedResult[Order]{Data: orders, Total: total, Page: p.Page, PageSize: p.PageSize, TotalPages: totalPages}, nil
}

// UpdateOrderStatus updates the status of an order and records bill history
// when relevant. Delivery-linked orders cannot be cancelled through this
// public entry point — the delivery lifecycle owns those (it uses
// CancelDeliveryLinkedOrder, the cancel-side analogue of
// ApproveDeliveryLinkedOrder).
func UpdateOrderStatus(orderID uint, status OrderStatus, actor string, reason string) error {
	return updateOrderStatus(orderID, status, actor, reason, false)
}

// CancelDeliveryLinkedOrder is the delivery lifecycle's internal entry point
// for propagating a delivery cancellation onto the kitchen order. It bypasses
// the delivery-linkage cancel guard that UpdateOrderStatus enforces.
func CancelDeliveryLinkedOrder(orderID uint, actor string, reason string) error {
	return updateOrderStatus(orderID, OrderStatusOrderCancelled, actor, reason, true)
}

// lockOrderBillTx locks the bill an order belongs to and returns its id (0
// when the order has none). It reads bill_id without locking the order, so the
// caller must lock the order next and check the id did not change.
func lockOrderBillTx(tx *gorm.DB, orderID uint) (uint, error) {
	var ref struct{ BillID uint }
	if err := tx.Model(&Order{}).Select("bill_id").Where("id = ?", orderID).Take(&ref).Error; err != nil {
		return 0, fmt.Errorf("failed to get order: %w", err)
	}
	if ref.BillID == 0 {
		return 0, nil
	}
	var lockBill Bill
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&lockBill, ref.BillID).Error; err != nil {
		return 0, fmt.Errorf("failed to lock bill for order update: %w", err)
	}
	return ref.BillID, nil
}

func updateOrderStatus(orderID uint, status OrderStatus, actor string, reason string, allowDeliveryLinkedCancel bool) error {
	// Start a transaction
	tx := db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Approve and cancel rewrite the bill, so they lock it before the order:
	// lock order is bill -> order, the same as a line void (AdjustBillItem)
	// and a floor merge, which lock the check and then its tickets. Taking
	// the order first deadlocked against a void on the same ticket.
	writesBill := status == OrderStatusApproved || status == OrderStatusOrderCancelled
	var lockedBillID uint
	if writesBill {
		billID, err := lockOrderBillTx(tx, orderID)
		if err != nil {
			tx.Rollback()
			return err
		}
		lockedBillID = billID
	}

	// Read current order state with a real FOR UPDATE row lock to prevent the
	// TOCTOU race. The old `gorm:query_option` Set was a GORM-v1 idiom that
	// GORM v2 silently ignores (B-1) — the SELECT ran unlocked.
	var currentOrder Order
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&currentOrder, orderID).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to get order: %w", err)
	}
	if writesBill && currentOrder.BillID != lockedBillID {
		// A merge moved the ticket between the bill lock and the order lock.
		tx.Rollback()
		return fmt.Errorf("%w: order %d moved to another check concurrently", ErrInvalidStatusTransition, orderID)
	}

	// Guard: no-op if order is already in the requested state
	if currentOrder.Status == status {
		tx.Rollback()
		return nil
	}

	// Validate state machine transition
	if err := ValidateTransition(currentOrder.Status, status); err != nil {
		tx.Rollback()
		return err
	}

	now := time.Now()
	cancelReason := strings.TrimSpace(reason)
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": now,
	}

	if status == OrderStatusApproved && actor != "" {
		updates["approved_by"] = actor
		updates["approved_at"] = now
	}

	if status == OrderStatusOrderCancelled {
		updates["cancelled_by"] = actor
		updates["cancelled_at"] = now
		updates["cancel_reason"] = cancelReason
	}

	// Update order status — conditional on the status we just validated under
	// the row lock. Belt-and-suspenders vs. the FOR UPDATE clause above: even
	// if a future call site mutates this row without locking, a committed
	// guest cancel can never be silently overwritten by an approve.
	statusUpdate := tx.Model(&Order{}).
		Where("id = ? AND status = ?", orderID, currentOrder.Status).
		Updates(updates)
	if statusUpdate.Error != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update order status: %w", statusUpdate.Error)
	}
	if statusUpdate.RowsAffected == 0 {
		tx.Rollback()
		return fmt.Errorf("%w: order %d changed concurrently (expected status %q)", ErrInvalidStatusTransition, orderID, currentOrder.Status)
	}

	historyBill := &Bill{ID: currentOrder.BillID, BusinessID: currentOrder.BusinessID}
	var historyEvents []BillHistoryEvent

	// If order is approved, add items to the bill
	if status == OrderStatusApproved {
		// Delivery-linked orders must go through ApproveDeliveryLinkedOrder:
		// the bill already carries the items + delivery fee + tip from
		// checkout; appending here would double-bill and erase the fee.
		var deliveryLinked int64
		if err := tx.Model(&DeliveryOrder{}).Where("order_id = ?", orderID).Count(&deliveryLinked).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to check delivery linkage: %w", err)
		}
		if deliveryLinked > 0 {
			tx.Rollback()
			return errors.New("delivery-linked orders must be approved through the delivery lifecycle")
		}

		// Parse order items from JSON
		var orderItems []OrderItem
		if currentOrder.Items != "" {
			if err := json.Unmarshal([]byte(currentOrder.Items), &orderItems); err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to unmarshal order items: %w", err)
			}
		}

		// Read bill with a real FOR UPDATE lock to prevent concurrent modification
		var bill Bill
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&bill, currentOrder.BillID).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to get bill for integration: %w", err)
		}
		// A guest can pay a share (status partial) while this order is still
		// pending. Approval must still land the items; every other status stays
		// rejected with the same error.
		if bill.Status != BillStatusOpen && bill.Status != BillStatusPartial {
			tx.Rollback()
			return fmt.Errorf("bill is not open for approved order items")
		}

		if err := DeductApprovedOrderInventoryTx(tx, &currentOrder, orderItems, actor); err != nil {
			tx.Rollback()
			return err
		}

		// Parse existing bill items from JSON
		var billItems []BillItem
		if bill.Items != "" {
			if err := json.Unmarshal([]byte(bill.Items), &billItems); err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to unmarshal bill items: %w", err)
			}
		}
		existingItemCount := len(billItems)
		alreadyBilled := false
		for i := range billItems {
			if billItems[i].OrderID != nil && *billItems[i].OrderID == currentOrder.ID {
				alreadyBilled = true
				break
			}
		}

		if !alreadyBilled {
			// Stamp each new bill item with the approving order's ID so a later cancel
			// of this order can remove exactly its items. Shared pointer is safe — all
			// items in this loop belong to the same order and currentOrder is not mutated.
			approvedOrderID := currentOrder.ID
			// One instant for both copies of the line. The JSON snapshot on
			// bills.items is what Bill.MarshalJSON puts on the wire, so leaving
			// CreatedAt zero here rendered every approved line — bundle parents
			// and their children included — at 0001-01-01T00:00:00Z next to
			// correctly stamped lines from the bill create/update paths (#708).
			itemStamp := now.UTC()

			// Convert order items to bill items
			for _, orderItem := range orderItems {
				itemType := strings.TrimSpace(orderItem.ItemType)
				if itemType == "" {
					itemType = "menu_item"
				}
				menuItemID := strings.TrimSpace(orderItem.MenuItemID)
				if menuItemID == "" {
					menuItemID = orderItem.MenuItemName
				}
				billItem := BillItem{
					ID:                 normalizeBillItemUUID(orderItem.ID),
					MenuItemID:         menuItemID,
					Name:               orderItem.MenuItemName,
					Price:              orderItem.Price,
					Quantity:           orderItem.Quantity,
					Options:            orderItem.Options,
					ItemType:           itemType,
					BundleID:           orderItem.BundleID,
					ParentBundleID:     orderItem.ParentBundleID,
					BundleOccurrenceID: orderItem.BundleOccurrenceID,
					SourceOfferID:      orderItem.SourceOfferID,
					OrderID:            &approvedOrderID,
					Subtotal:           orderItem.Subtotal,
					CreatedAt:          itemStamp,
				}
				billItems = append(billItems, billItem)
			}
			if err := stampBillItemsForPersist(tx, billItems, currentOrder.BusinessID, itemStamp); err != nil {
				tx.Rollback()
				return err
			}

			// Calculate new bill totals
			newSubtotal := bill.Subtotal
			orderSubtotalCents := int64(0)
			itemNames := make([]string, 0, len(orderItems))
			for _, orderItem := range orderItems {
				itemCents := normalizeMoneyCents(orderItem.Subtotal)
				newSubtotal += itemCents
				orderSubtotalCents += itemCents
				itemNames = append(itemNames, orderItem.MenuItemName)
			}
			if newSubtotal < 0 {
				newSubtotal = 0
			}

			// Update bill with new items and totals
			billItemsJSON, err := json.Marshal(billItems)
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to marshal bill items: %w", err)
			}

			// Get business to use configured tax and service fee rates
			var business Business
			if err := tx.First(&business, currentOrder.BusinessID).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to get business for tax/service fee rates: %w", err)
			}

			newTaxAmount, newServiceFeeAmount, newGrossTotal := computeBillTotals(
				newSubtotal,
				business.TaxRate,
				business.ServiceFeeRate,
			)
			// Approving an order onto a bill that already has a loyalty redemption
			// must keep the total net of that discount — otherwise the discount is
			// silently erased the moment a new order lands on the bill.
			newTotalAmount := NetBillTotalCents(newGrossTotal, bill.LoyaltyDiscountCents)

			// Same paid-vs-total rule as removeCancelledOrderBillItemsTx: the
			// share already collected may now cover the larger total, fall
			// short of it, or be only rounding dust.
			approvedBillStatus := BillStatusOpen
			if bill.PaidAmount > billPaymentAmountTolerance {
				if bill.PaidAmount >= newTotalAmount-billPaymentAmountTolerance {
					approvedBillStatus = BillStatusPaid
				} else {
					approvedBillStatus = BillStatusPartial
				}
			}

			billUpdates := map[string]interface{}{
				"items":              string(billItemsJSON),
				"subtotal":           newSubtotal,
				"tax_amount":         newTaxAmount,
				"service_fee_amount": newServiceFeeAmount,
				"total_amount":       newTotalAmount,
				"status":             approvedBillStatus,
				"updated_at":         time.Now(),
			}

			if err := tx.Model(&Bill{}).Where("id = ?", currentOrder.BillID).Updates(billUpdates).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to update bill with approved order items: %w", err)
			}

			// Append only the new items from this order to the relational table.
			// Reuse the exact BillItem values already written to the JSON snapshot
			// (billItems[existingItemCount:]) so the relational rows carry the SAME
			// normalized UUIDs. normalizeBillItemUUID is non-deterministic for the
			// synthetic, non-UUID order-line IDs (e.g. "bundle-5-..."), so normalizing
			// a second time here would assign different IDs and silently diverge the
			// JSON snapshot from the relational rows — breaking itemized splitting,
			// which keys items by ID.
			newBillItems := billItems[existingItemCount:]
			newRelItems := make([]BillItem, len(newBillItems))
			for i := range newBillItems {
				rel := newBillItems[i]
				rel.BillID = currentOrder.BillID
				if rel.CreatedAt.IsZero() {
					rel.CreatedAt = now
				}
				newRelItems[i] = rel
			}
			if len(newRelItems) > 0 {
				if err := tx.Create(&newRelItems).Error; err != nil {
					tx.Rollback()
					return fmt.Errorf("failed to create bill items relation: %w", err)
				}
			}

			historyEvents = append(historyEvents, BillHistoryEvent{
				BillID:      currentOrder.BillID,
				BusinessID:  currentOrder.BusinessID,
				EventType:   BillHistoryEventOrderApproved,
				Actor:       actor,
				OrderID:     &currentOrder.ID,
				OrderNumber: currentOrder.OrderNumber,
				Details: map[string]interface{}{
					"items_added":    len(orderItems),
					"subtotal_added": orderSubtotalCents,
					"item_names":     itemNames,
				},
			})
		} else {
			// Atomic guest checkout already persisted these order-linked bill rows.
			// Approval still performs inventory deduction and status/history work,
			// but must not append or charge the same physical items a second time.
			historyEvents = append(historyEvents, BillHistoryEvent{
				BillID: currentOrder.BillID, BusinessID: currentOrder.BusinessID,
				EventType: BillHistoryEventOrderApproved, Actor: actor,
				OrderID: &currentOrder.ID, OrderNumber: currentOrder.OrderNumber,
				Details: map[string]interface{}{"items_added": 0, "already_billed": true},
			})
		}
	}

	if status == OrderStatusOrderCancelled {
		// Symmetric to the approve-path delivery guard above (B-2/X-3):
		// cancelling a delivery-linked order kitchen-side only would leave the
		// delivery leg live — no driver release, no guest email, prepaid money
		// in limbo. The lifecycle bypasses via CancelDeliveryLinkedOrder.
		if !allowDeliveryLinkedCancel {
			var deliveryLinked int64
			if err := tx.Model(&DeliveryOrder{}).Where("order_id = ?", orderID).Count(&deliveryLinked).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to check delivery linkage: %w", err)
			}
			if deliveryLinked > 0 {
				tx.Rollback()
				return ErrDeliveryLinkedCancel
			}
		}

		// Lock the bill FOR UPDATE before touching inventory or bill items. The
		// cancel path does a read-modify-write of the bill's items JSON, totals and
		// status (removeCancelledOrderBillItemsTx); without this lock a concurrent
		// approve of a sibling order on the same bill (which locks the bill at
		// business.go:3335, appends its items and recomputes totals) would be
		// clobbered by this cancel's blind write, undercharging the guest and
		// diverging the JSON snapshot from the relational bill_items. Lock order is
		// bill -> inventory, matching the approve path, so the two never deadlock.
		if currentOrder.BillID != 0 {
			var lockBill Bill
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockBill, currentOrder.BillID).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to lock bill for order cancel: %w", err)
			}
		}

		if err := RestoreCancelledOrderInventoryTx(tx, &currentOrder, actor); err != nil {
			tx.Rollback()
			return err
		}

		// Remove this order's items from the bill and recompute the total. Without
		// this, cancelling an order that was already approved (its items live on the
		// bill) leaves the guest charged for a cancelled order — inventory is
		// restored but the money is not. No-op for orders cancelled while pending.
		if err := removeCancelledOrderBillItemsTx(tx, currentOrder.BillID, currentOrder.ID); err != nil {
			tx.Rollback()
			return err
		}

		historyEvents = append(historyEvents, BillHistoryEvent{
			BillID:      currentOrder.BillID,
			BusinessID:  currentOrder.BusinessID,
			EventType:   BillHistoryEventOrderCanceled,
			Actor:       actor,
			Reason:      cancelReason,
			OrderID:     &currentOrder.ID,
			OrderNumber: currentOrder.OrderNumber,
			Details: map[string]interface{}{
				"previous_status":   string(currentOrder.Status),
				"had_been_approved": currentOrder.Status == OrderStatusApproved || currentOrder.ApprovedAt != nil,
			},
		})
	}

	if err := createBillHistoryEventsTx(tx, historyBill, historyEvents); err != nil {
		tx.Rollback()
		return err
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// CancelPendingGuestOrder atomically cancels a still-pending guest order and
// writes the order.cancelled bill-history event in the same transaction
// (B-5/X-5: guest cancellations used to be invisible in the bill audit trail).
//
// Guest checkout bills items immediately (atomic already_billed path), so cancel
// must also remove this order's bill_items and recompute totals — otherwise the
// guest remains charged for a cancelled pending order. removeCancelledOrderBillItemsTx
// is a no-op when no rows exist for the order_id (legacy staff-pending-then-cancel).
// The conditional UPDATE means a lost race with approval returns (false, nil)
// and writes nothing.
func CancelPendingGuestOrder(order *Order, reason string, now time.Time) (bool, error) {
	if order == nil {
		return false, errors.New("order is required")
	}
	updated := false
	err := db.Transaction(func(tx *gorm.DB) error {
		// Bill before order, as in updateOrderStatus: the conditional UPDATE
		// below takes the order's row lock, so the bill lock must come first.
		// Pinning bill_id makes a ticket a merge just moved a lost race.
		if order.BillID != 0 {
			var lockBill Bill
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&lockBill, order.BillID).Error; err != nil {
				return fmt.Errorf("failed to lock bill for guest cancel: %w", err)
			}
		}
		res := tx.Model(&Order{}).
			Where("id = ? AND status = ? AND bill_id = ?", order.ID, OrderStatusPending, order.BillID).
			Updates(map[string]interface{}{
				"status":        OrderStatusOrderCancelled,
				"cancelled_by":  "guest",
				"cancel_reason": reason,
				"cancelled_at":  now,
				"updated_at":    now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		updated = true

		// The bill is already locked above; removeCancelledOrderBillItemsTx
		// re-takes FOR UPDATE (no-op in this tx).
		if order.BillID != 0 {
			// Pending guest orders do not deduct inventory (deduction is on approve).
			// Restore is still safe: no consumption rows → no-op.
			if err := RestoreCancelledOrderInventoryTx(tx, order, "guest"); err != nil {
				return err
			}
			if err := removeCancelledOrderBillItemsTx(tx, order.BillID, order.ID); err != nil {
				return err
			}
		}

		return createBillHistoryEventsTx(tx, &Bill{ID: order.BillID, BusinessID: order.BusinessID}, []BillHistoryEvent{{
			BillID:      order.BillID,
			BusinessID:  order.BusinessID,
			EventType:   BillHistoryEventOrderCanceled,
			Actor:       "guest",
			Reason:      strings.TrimSpace(reason),
			OrderID:     &order.ID,
			OrderNumber: order.OrderNumber,
			Details: map[string]interface{}{
				"previous_status":   string(OrderStatusPending),
				"had_been_approved": false,
			},
		}})
	})
	return updated, err
}

// Counter operations

// GetBusinessCounters retrieves all counters for a business
func GetBusinessCounters(businessID uint) ([]Counter, error) {
	var counters []Counter
	if err := db.Where("business_id = ?", businessID).Order("counter_number").Find(&counters).Error; err != nil {
		return nil, fmt.Errorf("failed to get counters: %w", err)
	}
	return counters, nil
}

// GetAvailableCounters retrieves counters that don't have active bills
func GetAvailableCounters(businessID uint) ([]Counter, error) {
	var counters []Counter
	if err := db.Where(
		`counters.business_id = ? AND counters.is_active = ? AND NOT EXISTS (`+
			`SELECT 1 FROM bills WHERE bills.counter_id = counters.id AND bills.status IN ?`+
			`)`,
		businessID, true, activeBillStatusStrings(),
	).Order("counter_number").Find(&counters).Error; err != nil {
		return nil, fmt.Errorf("failed to get available counters: %w", err)
	}
	return counters, nil
}

// counterPrefixMaxLen mirrors the server-side counter_prefix limit (L2-34).
const counterPrefixMaxLen = 5

// isGeneratedCounterName reports whether name looks like a default counter name
// for counterNumber — i.e. "<prefix><number>" for SOME prefix within the
// server's length limit, with no whitespace.
//
// Used by the R2-12 stale-prefix repair to tell a leftover auto-name ("C4"
// after the prefix moved to "M") from a name an operator typed ("Ventana de
// retiro"). Deliberately conservative: anything ambiguous is treated as custom
// and preserved, because losing an operator's label is the worse failure.
func isGeneratedCounterName(name string, counterNumber int) bool {
	suffix := strconv.Itoa(counterNumber)
	if !strings.HasSuffix(name, suffix) {
		return false
	}
	prefix := strings.TrimSuffix(name, suffix)
	if prefix == "" || len(prefix) > counterPrefixMaxLen {
		return false
	}
	if strings.ContainsAny(prefix, " \t\n\r") {
		return false
	}
	// "C11" for counter 1 could be counter 11's name under prefix "C" — a
	// digit-adjacent split is ambiguous, so leave it alone.
	if last := prefix[len(prefix)-1]; last >= '0' && last <= '9' {
		return false
	}
	return true
}

// isSeedDefaultCounterName reports legacy/demo seed labels that are not
// operator-typed custom names — e.g. "Counter 1", "Mostrador 2", "Pickup 3".
// These must be rewriteable when the Counter Name Prefix changes (or when the
// operator clicks Apply prefix), otherwise prefix "D" appears to do nothing
// while tiles still say "Counter 1" / "Counter 2" (#192).
func isSeedDefaultCounterName(name string, counterNumber int) bool {
	suffix := " " + strconv.Itoa(counterNumber)
	if !strings.HasSuffix(name, suffix) {
		return false
	}
	word := strings.TrimSuffix(name, suffix)
	switch strings.ToLower(strings.TrimSpace(word)) {
	case "counter", "mostrador", "pickup":
		return true
	default:
		return false
	}
}

// shouldRewriteCounterName decides whether an existing counter label may be
// overwritten with the current prefix+number form.
func shouldRewriteCounterName(existing string, counterNumber int, desired string, prefixChanged, wasInactive bool) bool {
	if existing == desired {
		return false
	}
	// Seed leftovers always sync to the current prefix — otherwise demo rows
	// named "Counter N" stay forever under prefix "D".
	if isSeedDefaultCounterName(existing, counterNumber) {
		return true
	}
	if !isGeneratedCounterName(existing, counterNumber) {
		return false
	}
	// Generated auto-names rewrite on prefix change, or on reactivation when
	// they still carry a stale prefix (R2-12).
	return prefixChanged || wasInactive
}

// UpdateBusinessCounters updates counter configuration for a business.
// L2-35: only rewrite counter names when the prefix actually changed (or the
// row is newly created). Saving count/enable alone must not destroy custom names.
func UpdateBusinessCounters(businessID uint, enabled bool, count int, prefix string) error {
	// Start transaction
	tx := db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to start transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var prior Business
	if err := tx.Select("counter_prefix").Where("id = ?", businessID).First(&prior).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to load business counter settings: %w", err)
	}
	prefixChanged := prior.CounterPrefix != prefix

	// Update business settings
	if err := tx.Model(&Business{}).Where("id = ?", businessID).Updates(map[string]interface{}{
		"counter_enabled": enabled,
		"counter_count":   count,
		"counter_prefix":  prefix,
	}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update business counter settings: %w", err)
	}

	if enabled {
		// Deactivate counters beyond the new count
		if err := tx.Model(&Counter{}).Where("business_id = ? AND counter_number > ?", businessID, count).Update("is_active", false).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to deactivate excess counters: %w", err)
		}

		// Create or update counters up to the new count
		for i := 1; i <= count; i++ {
			var existingCounter Counter
			err := tx.Where("business_id = ? AND counter_number = ?", businessID, i).First(&existingCounter).Error

			if errors.Is(err, gorm.ErrRecordNotFound) {
				// Create new counter — name always comes from the current prefix.
				newCounter := Counter{
					BusinessID:    businessID,
					CounterNumber: i,
					Name:          fmt.Sprintf("%s%d", prefix, i),
					IsActive:      true,
				}
				if err := tx.Create(&newCounter).Error; err != nil {
					tx.Rollback()
					return fmt.Errorf("failed to create counter %d: %w", i, err)
				}
			} else if err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to check existing counter %d: %w", i, err)
			} else {
				// Reactivate; only rewrite default/auto names — never custom.
				updates := map[string]interface{}{
					"is_active": true,
				}
				desiredName := fmt.Sprintf("%s%d", prefix, i)
				// L2-35: never rewrite an operator-typed name. Prefix changes
				// retarget auto-generated labels ("C1" → "M1"); seed leftovers
				// ("Counter 1") always sync; custom labels survive (#192).
				if shouldRewriteCounterName(
					existingCounter.Name,
					i,
					desiredName,
					prefixChanged,
					!existingCounter.IsActive,
				) {
					updates["name"] = desiredName
				}
				if err := tx.Model(&existingCounter).Updates(updates).Error; err != nil {
					tx.Rollback()
					return fmt.Errorf("failed to update counter %d: %w", i, err)
				}
			}
		}
	} else {
		// Disable all counters for this business
		if err := tx.Model(&Counter{}).Where("business_id = ?", businessID).Update("is_active", false).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to disable counters: %w", err)
		}
	}

	return tx.Commit().Error
}

// GetCounterByID retrieves a counter by its ID
func GetCounterByID(id uint) (*Counter, error) {
	var counter Counter
	if err := db.First(&counter, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("counter not found")
		}
		return nil, fmt.Errorf("failed to get counter: %w", err)
	}
	return &counter, nil
}

// publicBusinessColumns is the column set the unauthenticated storefront and
// QR/table guest loaders project (OF-04). Much of the Business row is public
// content, so the set is wide — but it deliberately drops the truly-private
// columns no public consumer reads: the Stripe IDs/payment-method, the
// onboarding blob, and the owner email. It DOES include the owner settlement/
// tipping wallets: the QR guest bill-creation path (CreateBillByTableCode)
// snapshots them onto the new bill so guest USDC settlement has a destination,
// and the guest bill response already returns settlement_address to the guest
// as the pay-to target — they are guest-facing payment destinations, not
// secrets (see the wallet note in the column list). It DOES keep the columns
// that drive the guest operational gate (is_active, closed_at, is_demo) — see
// IsBusinessOperational; dropping any of those silently fail-opens/closes guest
// ordering + AI.
//
// The set is the UNION of everything the public consumers touch:
//   - publicBusinessProjection (storefront response)
//   - buildPublicGuestBusinessResponse (QR/table guest response)
//   - IsAIWaiterAvailable / IsBusinessOperational
//     (guest AI gate + suspended/closed order rejection)
//   - guestOrderingEnabled (kitchen/orders toggles)
//   - applyBusinessContentTranslations (description/welcome/about prose)
//   - loadPublicBusinessByCustomURL / GetActiveTableWithBusinessByCode gating
//     (is_active, business_page_enabled)
//
// Embedded columns use their GORM prefixes: Address has explicit unprefixed
// column tags (street/city/...), DesignSettings uses design_, AiSettings ai_.
// When in doubt a scalar is INCLUDED — over-projecting a scalar is safe;
// under-projecting blanks the live storefront.
var publicBusinessColumns = []string{
	// gating + identity
	"id", "business_id", "is_active", "business_page_enabled",
	"name", "logo", "custom_url",
	// embedded address (BusinessAddress: explicit unprefixed column tags)
	"street", "city", "state", "postal_code", "country",
	// contact + social
	"description", "phone", "website", "social_media", "banner_images",
	// reviews / google
	"show_reviews", "google_reviews_enabled", "google_place_id",
	"google_business_name", "google_review_link", "google_business_url",
	// currency / language
	"default_currency", "display_currency", "default_language", "source_language",
	// money rates
	"tax_rate", "service_fee_rate", "tax_inclusive", "service_inclusive",
	// hospitality copy + toggles
	"welcome_message", "about_story", "show_welcome_message", "show_about_story",
	"show_gallery", "show_operating_hours", "show_special_features",
	// embedded design settings (design_ prefix)
	"design_primary_color", "design_secondary_color", "design_font_family",
	"design_theme", "design_menu_layout", "design_show_images",
	"design_show_descriptions", "design_header_style", "design_corner_radius",
	"design_shadow_intensity", "design_background_pattern", "design_pattern_opacity",
	"design_hero_layout", "design_section_density",
	// embedded AI settings — also feeds IsAIWaiterAvailable. NOTE the columns
	// are DOUBLE-prefixed: embeddedPrefix "ai_" + fields that themselves begin
	// with "Ai" → ai_ai_enabled / ai_ai_name / ai_ai_priority. Only
	// BusinessPageAiEnabled lacks the leading "Ai" → ai_business_page_ai_enabled.
	// ai_special_instructions is operator-only (never on a guest path) — dropped.
	"ai_ai_enabled", "ai_ai_name", "ai_ai_priority", "ai_business_page_ai_enabled",
	// QR-response operational toggles
	"timezone", "kitchen_enabled", "orders_enabled", "crm_enabled", "counter_enabled",
	// closed_at (with is_active, projected above) is the admin-lock input of
	// IsBusinessOperational. Never emitted: every public consumer builds a
	// hand-crafted gin.H.
	"closed_at",
	// is_demo drives IsBusinessOperational's demo short-circuit (demo showrooms
	// never lock mid-demo). Dropping it would fail-CLOSE a demo storefront.
	"is_demo",
	// kind is the Demo Center / test-fixture classifier.
	// The public storefront emits it so /b pages can noindex demo/test venues
	// even when is_demo was not set. Non-sensitive classification.
	"kind",
	// owner settlement wallets — the QR guest bill-creation path
	// (CreateBillByTableCode) snapshots these onto the new bill so guest USDC
	// settlement has a destination; the guest bill response already returns
	// settlement_address to the guest as the pay-to target. They are guest-facing
	// payment destinations, NOT secrets like the Stripe IDs/email that stay
	// dropped. They must still never be raw-serialized onto a business response —
	// every public consumer builds a hand-crafted gin.H (buildPublicGuestBusinessResponse
	// omits them). Dropping them silently fail-503s guest USDC settlement.
	"settlement_addr", "tipping_addr",
	// timestamps surfaced by publicBusinessProjection
	"created_at", "updated_at",
}

// GetBusinessByCustomURL retrieves a business by its custom URL.
// Case-insensitive by contract: a unique index enforces uniqueness on
// lower(custom_url), so any casing resolves to the single owner. ” is
// excluded — it is the GORM zero value shared by every slug-less business.
func GetBusinessByCustomURL(customURL string) (*Business, error) {
	var business Business
	if err := db.Select(publicBusinessColumns).
		Where("custom_url <> '' AND LOWER(custom_url) = LOWER(?)", customURL).
		First(&business).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBusinessNotFound
		}
		return nil, fmt.Errorf("failed to get business by custom URL: %w", err)
	}
	return &business, nil
}

// GetPublicBusinessByID retrieves a business by its numeric ID using the same
// narrow public column projection as GetBusinessByCustomURL. Callers that serve
// guests MUST apply the storefront publish gates (business_page_enabled &&
// is_active) themselves.
func GetPublicBusinessByID(id uint) (*Business, error) {
	var business Business
	if err := db.Select(publicBusinessColumns).
		Where("id = ?", id).First(&business).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBusinessNotFound
		}
		return nil, fmt.Errorf("failed to get business by id: %w", err)
	}
	return &business, nil
}

// StorefrontSlug is the minimal projection the public sitemap needs — no
// sensitive fields, just the routable slug and its last-modified time.
type StorefrontSlug struct {
	CustomURL string    `json:"custom_url"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListPublishedStorefrontSlugs returns published, active storefront slugs for
// the public sitemap. Narrow projection (two columns), bounded by limit, and
// filtered to publishable rows with a non-empty slug. Mirrors the publish/active
// gate in loadPublicBusinessByCustomURL so the sitemap and the page agree on
// what is public. CI/local fixtures (kind=test) stay out; published demo
// showrooms are included so crawlers can discover live converting /b/ URLs.
// No SELECT *, no preloads.
func ListPublishedStorefrontSlugs(limit int) ([]StorefrontSlug, error) {
	var out []StorefrontSlug
	err := db.Model(&Business{}).
		Select("custom_url", "updated_at").
		Where("business_page_enabled = ? AND is_active = ? AND custom_url <> ''", true, true).
		Where("(kind IS NULL OR kind = '' OR kind <> ?)", BusinessKindTest).
		Order("updated_at DESC").
		Limit(limit).
		Find(&out).Error
	return out, err
}

// ==================== Operational state ====================

// IsBusinessOperational reports whether a venue's product features stay
// available. Payverge has no billing, so only an instance-admin enforcement
// action (close: closed_at set, or suspend: is_active=false) takes a venue
// offline. Demo showrooms never lock.
//
// DRIFT GUARD: every column read here (is_demo, is_active, closed_at) MUST be
// present in publicBusinessColumns and BusinessAccessDecisionColumns. The
// unauthenticated storefront/QR loaders project the Business row and then
// call this (directly or via IsAIWaiterAvailable) to gate guest ordering and
// AI; a missing column silently zeroes the field and mis-computes the gate.
func IsBusinessOperational(business *Business) bool {
	if business == nil {
		return false
	}
	if business.IsDemo {
		return true
	}
	return !AdminLocked(business)
}

// IsAIWaiterAvailable reports whether the guest-facing AI Waiter should be
// offered for a business: the venue must be operational and have the AI toggle
// enabled. This is the single source of truth for guest AI gating; the public
// guest response surfaces it so unauthenticated diners get a correct signal.
func IsAIWaiterAvailable(business *Business) bool {
	return IsBusinessOperational(business) && business.AiSettings.AiEnabled
}

// ToggleKitchenAndOrders enables or disables kitchen and orders features for a business.
//
// X-11 (intentional, documented): both flags are always written to the same
// value — the two columns exist for historical reasons but behave as one
// switch. The flags gate GUEST ordering endpoints only; operator order routes
// deliberately ignore them (staff override). Do not change either behavior
// without a product decision.
func ToggleKitchenAndOrders(businessID uint, enabled bool) error {
	business, err := GetBusinessByID(businessID)
	if err != nil {
		return fmt.Errorf("failed to get business: %w", err)
	}

	if !IsBusinessOperational(business) {
		return fmt.Errorf("kitchen and orders are unavailable while the business is suspended or closed")
	}

	updates := map[string]interface{}{
		"kitchen_enabled": enabled,
		"orders_enabled":  enabled,
		"updated_at":      time.Now(),
	}

	if err := db.Model(&Business{}).Where("id = ?", businessID).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to toggle kitchen and orders: %w", err)
	}

	return nil
}
