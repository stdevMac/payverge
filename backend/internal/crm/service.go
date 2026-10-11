package crm

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// createDefaultCustomerPreferences inserts a preferences row with sharing
// DISABLED (opt-in). Uses a map Create so the false bool is written explicitly
// and GORM does not substitute the model tag's default:true for a zero value.
func createDefaultCustomerPreferences(tx *gorm.DB, customerID uint, preferredCurrency string) error {
	if preferredCurrency == "" {
		preferredCurrency = "USD"
	}
	return tx.Model(&database.CustomerPreferences{}).Create(map[string]interface{}{
		"customer_id":                customerID,
		"preferred_language":         "en",
		"preferred_currency":         preferredCurrency,
		"receive_promotions":         true,
		"receive_newsletters":        true,
		"receive_birthday_offers":    true,
		"share_data_with_businesses": false,
	}).Error
}

func normalizeCustomerEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func normalizeCustomerName(name string) string {
	return strings.TrimSpace(name)
}

func preloadCustomerConnectionBusinessSummary(tx *gorm.DB) *gorm.DB {
	return tx.Select(
		"id",
		"business_id",
		"name",
		"logo",
		"custom_url",
		"default_currency",
		"display_currency",
		"is_active",
	)
}

// Service handles CRM business logic
type Service struct {
	db *gorm.DB
}

const (
	defaultBusinessCustomersPageSize = 20
	maxBusinessCustomersPageSize     = 100
)

// NewService creates a new CRM service
func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func normalizeBusinessCustomersPagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultBusinessCustomersPageSize
	}
	if pageSize > maxBusinessCustomersPageSize {
		pageSize = maxBusinessCustomersPageSize
	}
	return page, pageSize
}

// Sentinels for client-facing CRM auth failures. Handlers map these to 4xx
// responses; every other error stays server-side.
var (
	ErrCustomerEmailTaken         = errors.New("customer with this email already exists")
	ErrCustomerNameRequired       = errors.New("name is required")
	ErrInvalidCustomerCredentials = errors.New("invalid email or password")
	ErrWalletLinkedElsewhere      = errors.New("wallet address is already linked to another account")
)

// Customer Authentication

// RegisterCustomer creates a new customer account with default preferences
// (sharing opt-in = false) in a single transaction so a prefs failure rolls
// back the customer row.
func (s *Service) RegisterCustomer(email, password, name string) (*database.Customer, error) {
	email = normalizeCustomerEmail(email)
	name = normalizeCustomerName(name)
	if name == "" {
		return nil, ErrCustomerNameRequired
	}

	// Hash password outside the transaction (CPU-bound, no DB dependency).
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	var customer database.Customer
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Friendly pre-check; unique index remains the concurrency backstop.
		var existing database.Customer
		if err := tx.Where("email = ?", email).First(&existing).Error; err == nil {
			return ErrCustomerEmailTaken
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("database error: %w", err)
		}

		customer = database.Customer{
			Email:        email,
			PasswordHash: string(hashedPassword),
			Name:         name,
			IsActive:     true,
		}
		if err := tx.Create(&customer).Error; err != nil {
			return fmt.Errorf("failed to create customer: %w", err)
		}

		if err := createDefaultCustomerPreferences(tx, customer.ID, "USD"); err != nil {
			return fmt.Errorf("failed to create customer preferences: %w", err)
		}

		// Reload preferences inside the transaction so the response reflects
		// committed-in-tx state (share_data_with_businesses=false).
		var prefs database.CustomerPreferences
		if err := tx.Where("customer_id = ?", customer.ID).First(&prefs).Error; err != nil {
			return fmt.Errorf("failed to load customer preferences: %w", err)
		}
		customer.Preferences = &prefs
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &customer, nil
}

// AuthenticateCustomer verifies customer credentials
func (s *Service) AuthenticateCustomer(email, password string) (*database.Customer, error) {
	email = normalizeCustomerEmail(email)

	var customer database.Customer
	if err := s.db.Where("email = ? AND is_active = ?", email, true).First(&customer).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidCustomerCredentials
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(customer.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCustomerCredentials
	}

	// Update last login
	now := time.Now()
	customer.LastLoginAt = &now
	s.db.Omit(clause.Associations).Save(&customer)

	return &customer, nil
}

// ConnectCustomerToBusiness creates or updates the relationship between customer and business
func (s *Service) ConnectCustomerToBusiness(customerID, businessID uint, optInMarketing bool) (*database.CustomerBusiness, error) {
	var customer database.Customer
	if err := s.db.Select("id", "is_active").First(&customer, customerID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("customer not found")
		}
		return nil, fmt.Errorf("database error: %w", err)
	}
	if !customer.IsActive {
		return nil, errors.New("customer account is inactive")
	}

	var business database.Business
	if err := s.db.Select("id", "is_active").First(&business, businessID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("business not found")
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	if !business.IsActive {
		return nil, errors.New("business is inactive")
	}

	connection, err := database.EnsureActiveCustomerBusinessConnection(s.db, customerID, businessID, optInMarketing)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure customer-business connection: %w", err)
	}

	return connection, nil
}

// GetCustomerByID retrieves a customer by ID
func (s *Service) GetCustomerByID(customerID uint) (*database.Customer, error) {
	var customer database.Customer
	if err := s.db.Preload("Preferences").Where("id = ? AND is_active = ?", customerID, true).First(&customer).Error; err != nil {
		return nil, err
	}
	return &customer, nil
}

// GetCustomerBusinessConnections retrieves all business connections for a customer
func (s *Service) GetCustomerBusinessConnections(customerID uint) ([]database.CustomerBusiness, error) {
	var connections []database.CustomerBusiness
	if err := s.db.Preload("Business", preloadCustomerConnectionBusinessSummary).Where("customer_id = ? AND is_active = ?", customerID, true).Find(&connections).Error; err != nil {
		return nil, err
	}
	return connections, nil
}

// Business CRM Management

// BusinessCustomerSummary holds whole-base aggregates for the CRM dashboard
// cards. Computed via SQL aggregates so a 500-customer business reports true
// totals instead of the current 20-row page (audit L6 finding #3). Monetary
// figures are USD dollars to match the CustomerBusiness.TotalSpent JSON wire.
type BusinessCustomerSummary struct {
	TotalCustomers   int64   `json:"total_customers"`
	ActiveThisMonth  int64   `json:"active_this_month"`
	AvgLifetimeSpend float64 `json:"avg_lifetime_spend"`
	TopTierCount     int64   `json:"top_tier_count"`
}

// escapeCustomerSearchLike escapes the LIKE wildcards (% and _) plus the escape
// character itself so an operator's literal "%" or "_" is matched literally, not
// as a wildcard. Used with `ESCAPE '\'`.
func escapeCustomerSearchLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}

// applyBusinessCustomerFilters narrows a CustomerBusiness query to the active
// connections of one business, optionally filtered by free-text search and
// loyalty tier. Shared by the list and the summary so both agree on scope.
//
// Search is case-insensitive (LOWER(col) LIKE LOWER(?) ESCAPE '\') so "acme"
// matches "ACME Corp" — the old case-sensitive LIKE missed mixed-case names on
// Postgres (audit §3.5 MED, fix 5). LOWER()/LIKE is used instead of ILIKE so the
// query stays portable across Postgres and the SQLite test backend; escaping the
// wildcards keeps the leading/trailing wildcards while treating operator-typed
// %/_ literally. pg_trgm is a deferred option (NOT added here).
func applyBusinessCustomerFilters(query *gorm.DB, businessID uint, search, tier string) *gorm.DB {
	query = query.Where("customer_businesses.business_id = ? AND customer_businesses.is_active = ?", businessID, true)

	if search != "" {
		pattern := "%" + escapeCustomerSearchLike(search) + "%"
		query = query.Joins("JOIN customers ON customers.id = customer_businesses.customer_id").
			Where(
				"LOWER(customers.name) LIKE LOWER(?) ESCAPE '\\' "+
					"OR LOWER(customers.email) LIKE LOWER(?) ESCAPE '\\' "+
					"OR LOWER(customers.phone) LIKE LOWER(?) ESCAPE '\\'",
				pattern, pattern, pattern,
			)
	}

	if tier != "" {
		query = query.Where("customer_businesses.loyalty_tier = ?", tier)
	}

	return query
}

// customerSegmentPredicate is the single source of truth for the CRM segment
// membership SQL (fix 7). Given a segment key it returns a WHERE fragment + args
// that select exactly that band, replicating the mutually-exclusive priority
// lapsed > atRisk > vip > new used by GetSegments — so a segment card's count
// and the list you drill into always agree.
//
// The day-boundary thresholds are converted to absolute UTC timestamp cutoffs so
// the SQL stays portable (no DB-specific date math) and matches GetSegments'
// integer-truncated `daysSince > N` semantics exactly: `daysSince > N` ⟺ the
// event is at least (N+1) days in the past. Returns ok=false for an empty or
// unrecognized segment (no predicate applied).
func customerSegmentPredicate(businessID uint, segment string, now time.Time) (string, []interface{}, bool) {
	seg := strings.ToLower(strings.TrimSpace(segment))
	if seg == "" {
		return "", nil, false
	}
	now = now.UTC()
	day := 24 * time.Hour
	visitLapsed := now.Add(-91 * day) // daysSinceVisit > 90
	visitAtRisk := now.Add(-31 * day) // daysSinceVisit > 30
	joinLapsed := now.Add(-91 * day)  // daysSinceJoin  > 90
	joinNew := now.Add(-31 * day)     // daysSinceJoin <= 30

	// avgSpend over the same (business, active) scope, as a scalar subquery,
	// matching GetSegments' VIP band.
	const avgSubquery = "(SELECT COALESCE(AVG(total_spent), 0) FROM customer_businesses " +
		"WHERE business_id = ? AND is_active = ?)"

	// Reusable band fragments (no leading AND). Parameter order below must match
	// the placeholders in each returned clause.
	lapsedFrag := "((last_visit_at IS NOT NULL AND last_visit_at <= ?) " +
		"OR (last_visit_at IS NULL AND first_visit_at <= ?))"
	atRiskFrag := "(last_visit_at IS NOT NULL AND last_visit_at <= ?)"
	vipFrag := "(visit_count >= 5 AND total_spent > " + avgSubquery + ")"
	newFrag := "(first_visit_at > ?)"

	switch seg {
	case "lapsed":
		return lapsedFrag, []interface{}{visitLapsed, joinLapsed}, true
	case "at-risk", "at_risk", "atrisk":
		// NOT lapsed AND at-risk window.
		return "NOT " + lapsedFrag + " AND " + atRiskFrag,
			[]interface{}{visitLapsed, joinLapsed, visitAtRisk}, true
	case "vip":
		// NOT lapsed AND NOT at-risk AND vip.
		return "NOT " + lapsedFrag + " AND NOT " + atRiskFrag + " AND " + vipFrag,
			[]interface{}{visitLapsed, joinLapsed, visitAtRisk, businessID, true}, true
	case "new":
		// NOT lapsed AND NOT at-risk AND NOT vip AND joined within 30d.
		return "NOT " + lapsedFrag + " AND NOT " + atRiskFrag + " AND NOT " + vipFrag + " AND " + newFrag,
			[]interface{}{visitLapsed, joinLapsed, visitAtRisk, businessID, true, joinNew}, true
	default:
		return "", nil, false
	}
}

// businessCustomersQuery holds the optional narrowing/ordering for a customer
// list read. Zero-value fields mean "not applied", so the default call reads the
// legacy shape (BE-first).
type businessCustomersQuery struct {
	search  string
	tier    string
	segment string // "" = no segment filter (lapsed/vip/new/at-risk/at_risk)
	sortBy  string // whitelisted column key; "" = default ordering
	sortDir string // "asc"|"desc"; anything else => desc
}

// customerSortColumns whitelists the columns the customer list may be sorted by,
// mapping the API sort key to a safe SQL ORDER BY expression. Anything not in
// this map falls back to the default ordering — a hostile `sort_by` can never
// reach the SQL string (fix 6 injection guard). The `name` sort orders by the
// joined customers table; applyBusinessCustomerFilters already JOINs customers
// when a search is present, and the list read force-joins for the name sort.
var customerSortColumns = map[string]string{
	"total_spent":   "customer_businesses.total_spent",
	"visit_count":   "customer_businesses.visit_count",
	"loyalty_tier":  "customer_businesses.loyalty_tier",
	"last_visit_at": "customer_businesses.last_visit_at",
	"name":          "customers.name",
}

// resolveCustomerOrderClause returns the ORDER BY clause for the requested sort,
// or the default (most-recent-visit-first) ordering when the sort key is not
// whitelisted. Returns whether a JOIN on customers is required (name sort).
func resolveCustomerOrderClause(sortBy, sortDir string) (clause string, needsCustomerJoin bool) {
	col, ok := customerSortColumns[sortBy]
	if !ok {
		// Fully qualify the columns: a search or name-sort JOINs customers, which
		// also has created_at/last_visit_at — a bare column would be ambiguous.
		return "customer_businesses.last_visit_at DESC NULLS LAST, customer_businesses.created_at DESC", false
	}
	dir := "DESC"
	if strings.EqualFold(sortDir, "asc") {
		dir = "ASC"
	}
	// NULLS LAST keeps unset last_visit_at at the bottom on both sort directions
	// for the timestamp column; harmless on the others.
	return col + " " + dir + " NULLS LAST", sortBy == "name"
}

// GetBusinessCustomers retrieves all customers for a business with pagination.
// `tier` filters server-side so a tier chip narrows the whole base, not just
// the visible page. Preserves the legacy default ordering (most recent visit).
func (s *Service) GetBusinessCustomers(businessID uint, page, pageSize int, search, tier string) ([]database.CustomerBusiness, int64, error) {
	return s.queryBusinessCustomers(businessID, page, pageSize, businessCustomersQuery{search: search, tier: tier})
}

// GetBusinessCustomersSorted retrieves a page ordered by a whitelisted column so
// header sort reorders the WHOLE base, not just the visible page (fix 6).
func (s *Service) GetBusinessCustomersSorted(businessID uint, page, pageSize int, search, tier, sortBy, sortDir string) ([]database.CustomerBusiness, int64, error) {
	return s.queryBusinessCustomers(businessID, page, pageSize, businessCustomersQuery{
		search:  search,
		tier:    tier,
		sortBy:  sortBy,
		sortDir: sortDir,
	})
}

// GetBusinessCustomersSegment retrieves a page narrowed to one behavioral
// segment (lapsed/vip/new/at-risk), using the SAME predicate as GetSegments so
// the count on the Segments card and the drilled-in list agree (fix 7).
func (s *Service) GetBusinessCustomersSegment(businessID uint, page, pageSize int, segment string) ([]database.CustomerBusiness, int64, error) {
	return s.queryBusinessCustomers(businessID, page, pageSize, businessCustomersQuery{segment: segment})
}

// queryBusinessCustomers is the single read path behind the list variants: it
// applies the shared business/search/tier scope, an optional segment predicate,
// counts, then reads one ordered page with the consent-only preload.
func (s *Service) queryBusinessCustomers(businessID uint, page, pageSize int, opts businessCustomersQuery) ([]database.CustomerBusiness, int64, error) {
	var customers []database.CustomerBusiness
	var total int64
	page, pageSize = normalizeBusinessCustomersPagination(page, pageSize)

	orderClause, needsCustomerJoin := resolveCustomerOrderClause(opts.sortBy, opts.sortDir)

	build := func() *gorm.DB {
		q := applyBusinessCustomerFilters(s.db.Model(&database.CustomerBusiness{}), businessID, opts.search, opts.tier)
		if where, args, ok := customerSegmentPredicate(businessID, opts.segment, time.Now()); ok {
			q = q.Where(where, args...)
		}
		// The name sort orders by customers.name; ensure the join exists even
		// when no search JOINed it. A no-op when search already joined customers.
		if needsCustomerJoin && opts.search == "" {
			q = q.Joins("JOIN customers ON customers.id = customer_businesses.customer_id")
		}
		return q
	}

	if err := build().Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Customer.Preferences is preloaded with selected columns ONLY to evaluate
	// the guest's ShareDataWithBusinesses consent; sanitizeCustomerForOperator
	// strips it before the rows leave the service.
	offset := (page - 1) * pageSize
	if err := build().Preload("Customer").
		Preload("Customer.Preferences", preloadCustomerConsent).
		Order(orderClause).
		Limit(pageSize).Offset(offset).Find(&customers).Error; err != nil {
		return nil, 0, err
	}

	for i := range customers {
		sanitizeCustomerForOperator(&customers[i])
	}
	return customers, total, nil
}

// GetBusinessCustomerSummary computes the CRM dashboard card aggregates over the
// entire (filtered) customer base in a single SQL pass — no row hydration.
// `topTier` defaults to "Gold" (the headline tier counted by TopTierCount).
// `search` and `tier` match the list filters so card totals stay aligned with
// the customer table when the operator searches or toggles a tier chip (L5-2).
// `topTier` remains independent of the list `tier` filter so the Gold count
// label still means "Gold" even when filtering to Bronze (then TopTierCount=0).
func (s *Service) GetBusinessCustomerSummary(businessID uint, search, tier, topTier string) (BusinessCustomerSummary, error) {
	if topTier == "" {
		topTier = "Gold"
	}
	monthAgo := time.Now().Add(-30 * 24 * time.Hour)

	var summary BusinessCustomerSummary
	query := applyBusinessCustomerFilters(s.db.Model(&database.CustomerBusiness{}), businessID, search, tier)

	row := struct {
		Total    int64
		Active   int64
		AvgSpend float64
		TopTier  int64
	}{}

	if err := query.Select(
		"COUNT(*) AS total, "+
			"COUNT(CASE WHEN customer_businesses.last_visit_at IS NOT NULL AND customer_businesses.last_visit_at >= ? THEN 1 END) AS active, "+
			"COALESCE(AVG(customer_businesses.total_spent), 0) AS avg_spend, "+
			"COUNT(CASE WHEN customer_businesses.loyalty_tier = ? THEN 1 END) AS top_tier",
		monthAgo, topTier,
	).Scan(&row).Error; err != nil {
		return summary, err
	}

	summary.TotalCustomers = row.Total
	summary.ActiveThisMonth = row.Active
	// Lifetime spend is dollars on the wire — round to cents so operators never
	// see AVG() float residue (e.g. 444.1666666666667).
	summary.AvgLifetimeSpend = math.Round(row.AvgSpend*100) / 100
	summary.TopTierCount = row.TopTier
	return summary, nil
}

// GetCustomerBusinessDetails retrieves detailed information about a customer's relationship with a business
func (s *Service) GetCustomerBusinessDetails(customerBusinessID uint) (*database.CustomerBusiness, error) {
	var customerBusiness database.CustomerBusiness
	if err := s.db.Preload("Customer").
		Preload("Customer.Preferences", preloadCustomerConsent).
		First(&customerBusiness, customerBusinessID).Error; err != nil {
		return nil, err
	}
	sanitizeCustomerForOperator(&customerBusiness)
	return &customerBusiness, nil
}

func (s *Service) GetCustomerBusinessDetailsForBusiness(businessID, customerBusinessID uint) (*database.CustomerBusiness, error) {
	customerBusiness, err := s.GetCustomerBusinessDetails(customerBusinessID)
	if err != nil {
		return nil, err
	}
	if customerBusiness.BusinessID != businessID {
		return nil, errors.New("customer does not belong to this business")
	}
	return customerBusiness, nil
}

// UpdateCustomerBusinessNotes updates business-specific notes for a customer
func (s *Service) UpdateCustomerBusinessNotes(customerBusinessID uint, notes string) error {
	return s.db.Model(&database.CustomerBusiness{}).
		Where("id = ?", customerBusinessID).
		Update("notes", notes).Error
}

// UpdateCustomerBusinessTags updates tags for a customer
func (s *Service) UpdateCustomerBusinessTags(customerBusinessID uint, tags string) error {
	return s.db.Model(&database.CustomerBusiness{}).
		Where("id = ?", customerBusinessID).
		Update("tags", tags).Error
}

// UpdateCustomerBusinessAllergies updates allergy notes for a customer-business link.
func (s *Service) UpdateCustomerBusinessAllergies(customerBusinessID uint, allergies string) error {
	return s.db.Model(&database.CustomerBusiness{}).
		Where("id = ?", customerBusinessID).
		Update("allergies", allergies).Error
}

// AdjustCustomerBusinessLoyaltyPoints sets an absolute loyalty point balance for
// service-recovery comps and operator corrections. Negative values are rejected;
// the caller must already have verified business ownership.
func (s *Service) AdjustCustomerBusinessLoyaltyPoints(customerBusinessID uint, points int) error {
	if points < 0 {
		return errors.New("loyalty points cannot be negative")
	}
	return s.db.Model(&database.CustomerBusiness{}).
		Where("id = ?", customerBusinessID).
		Update("loyalty_points", points).Error
}

// UpdateCustomerProfile updates customer profile information
func (s *Service) UpdateCustomerProfile(customerID uint, updates map[string]interface{}) error {
	return s.db.Model(&database.Customer{}).Where("id = ?", customerID).Updates(updates).Error
}

// UpdateCustomerPreferences upserts customer preferences for customerID.
// If no preferences row exists, one is created with sharing disabled (opt-in)
// and then the provided updates are applied. Scoped strictly by customer_id.
func (s *Service) UpdateCustomerPreferences(customerID uint, updates map[string]interface{}) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var prefs database.CustomerPreferences
		err := tx.Where("customer_id = ?", customerID).First(&prefs).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Insert a default (share=false) row. OnConflict handles a concurrent
			// create racing the same customer_id unique index.
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "customer_id"}},
				DoNothing: true,
			}).Model(&database.CustomerPreferences{}).Create(map[string]interface{}{
				"customer_id":                customerID,
				"preferred_language":         "en",
				"preferred_currency":         "USD",
				"receive_promotions":         true,
				"receive_newsletters":        true,
				"receive_birthday_offers":    true,
				"share_data_with_businesses": false,
			}).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		if len(updates) == 0 {
			return nil
		}
		return tx.Model(&database.CustomerPreferences{}).
			Where("customer_id = ?", customerID).
			Updates(updates).Error
	})
}

// LinkWalletToCustomer links a wallet address to a customer account
func (s *Service) LinkWalletToCustomer(customerID uint, walletAddress string) error {
	walletAddress = strings.TrimSpace(walletAddress)

	// Check if wallet is already linked to another account
	var existing database.Customer
	if err := s.db.Where("wallet_address = ? AND id != ?", walletAddress, customerID).First(&existing).Error; err == nil {
		return ErrWalletLinkedElsewhere
	}

	return s.db.Model(&database.Customer{}).
		Where("id = ?", customerID).
		Update("wallet_address", walletAddress).Error
}

// UnlinkCustomerFromBusiness soft-unlinks a customer from one business by
// flipping customer_businesses.is_active=false. It never deletes the global
// customer row (cross-business data must stay intact) — L5-9.
func (s *Service) UnlinkCustomerFromBusiness(businessID, customerBusinessID uint) error {
	res := s.db.Model(&database.CustomerBusiness{}).
		Where("id = ? AND business_id = ? AND is_active = ?", customerBusinessID, businessID, true).
		Update("is_active", false)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// GetCustomerStats retrieves aggregated statistics for a customer at a business
func (s *Service) GetCustomerStats(customerBusinessID uint) (map[string]interface{}, error) {
	var customerBusiness database.CustomerBusiness
	if err := s.db.First(&customerBusiness, customerBusinessID).Error; err != nil {
		return nil, err
	}

	var visitStats struct {
		TotalVisits   int64
		AvgSpending   float64
		LastVisitDate *time.Time
	}

	s.db.Model(&database.CustomerVisit{}).
		Where("customer_business_id = ?", customerBusinessID).
		Select("COUNT(*) as total_visits, AVG(amount_spent) as avg_spending, MAX(visit_date) as last_visit_date").
		Scan(&visitStats)

	stats := map[string]interface{}{
		"loyalty_points":   customerBusiness.LoyaltyPoints,
		"loyalty_tier":     customerBusiness.LoyaltyTier,
		"total_spent":      customerBusiness.TotalSpent,
		"visit_count":      customerBusiness.VisitCount,
		"avg_spending":     visitStats.AvgSpending,
		"last_visit_date":  visitStats.LastVisitDate,
		"first_visit_date": customerBusiness.FirstVisitAt,
	}

	return stats, nil
}

// loadLoyaltyProgram returns the business's loyalty program with its tiers
// preloaded. A missing program (or a missing table — e.g. in a legacy test DB
// that hasn't run the loyalty migration) yields a disabled zero-point default
// so settlement keeps working unchanged. Pass the active transaction as db
// so the query runs inside it — calling s.db here from inside a
// Transaction() callback deadlocks under SQLite.
func (s *Service) loadLoyaltyProgram(db *gorm.DB, businessID uint) (*database.LoyaltyProgram, error) {
	return s.loadLoyaltyProgramWithLock(db, businessID, false)
}

func (s *Service) loadLoyaltyProgramWithLock(db *gorm.DB, businessID uint, lock bool) (*database.LoyaltyProgram, error) {
	if db == nil {
		db = s.db
	}
	disabled := &database.LoyaltyProgram{Enabled: false, PointsPerDollar: 0, RedemptionPointsPerDollar: 0}
	var p database.LoyaltyProgram
	query := db.Preload("Tiers")
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := query.Where("business_id = ?", businessID).First(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || strings.Contains(err.Error(), "no such table") {
			return disabled, nil
		}
		return nil, err
	}
	if validateTierLadder(p.Tiers) != nil {
		repaired, err := repairDuplicateTierRows(db, &p)
		if err != nil {
			return nil, fmt.Errorf("repair loyalty tier ladder: %w", err)
		}
		if repaired {
			query = db.Preload("Tiers")
			if lock {
				query = query.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			if err := query.Where("business_id = ?", businessID).First(&p).Error; err != nil {
				return nil, err
			}
		}
	}
	return &p, nil
}

// loadLoyaltyProgramForSettlement loads the program through the normal repair
// path, then rejects any ladder that still violates the authoritative
// invariants. Display callers keep the invalid program so they can return the
// localized invalid-state response; settlement callers must never consume it.
func (s *Service) loadLoyaltyProgramForSettlement(db *gorm.DB, businessID uint) (*database.LoyaltyProgram, error) {
	program, err := s.loadLoyaltyProgramWithLock(db, businessID, true)
	if err != nil {
		return nil, err
	}
	if err := validateTierLadder(program.Tiers); err != nil {
		return nil, ErrInvalidLoyaltyTierLadder
	}
	return program, nil
}

// DeleteCustomerAccount anonymizes a guest account in one transaction.
// The customers row is kept because email is UNIQUE NOT NULL: it is rewritten
// to deleted-<id>@deleted.invalid and every other personal field is cleared.
// Consent (customer_preferences) and saved addresses are deleted. Business
// links lose marketing opt-ins and free-text PII. Visit feedback and
// communication error text that can echo an address are blanked. Terminal
// delivery orders (delivered, cancelled, failed) are anonymized and detached;
// in-flight orders are left unchanged so an active delivery can finish.
// Returns gorm.ErrRecordNotFound when no customers row matches customerID.
func (s *Service) DeleteCustomerAccount(customerID uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&database.Customer{}).Where("id = ?", customerID).Updates(map[string]interface{}{
			"email":                 fmt.Sprintf("deleted-%d@deleted.invalid", customerID),
			"name":                  "",
			"phone":                 "",
			"wallet_address":        "",
			"birthday":              nil,
			"profile_image_url":     "",
			"password_hash":         "",
			"verification_token":    "",
			"password_reset_token":  "",
			"password_reset_expiry": nil,
			"email_verified":        false,
			"is_active":             false,
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		if err := tx.Where("customer_id = ?", customerID).Delete(&database.CustomerPreferences{}).Error; err != nil {
			return err
		}
		if err := tx.Where("customer_id = ?", customerID).Delete(&database.CustomerAddress{}).Error; err != nil {
			return err
		}

		if err := tx.Model(&database.CustomerBusiness{}).Where("customer_id = ?", customerID).Updates(map[string]interface{}{
			"is_active":           false,
			"opt_in_marketing":    false,
			"opt_in_sms":          false,
			"opt_in_email":        false,
			"notes":               "",
			"allergies":           "",
			"dietary_preferences": "",
			"favorite_items":      "",
			"tags":                "",
		}).Error; err != nil {
			return err
		}

		var linkIDs []uint
		if err := tx.Model(&database.CustomerBusiness{}).Where("customer_id = ?", customerID).Pluck("id", &linkIDs).Error; err != nil {
			return err
		}
		if len(linkIDs) > 0 {
			if err := tx.Model(&database.CustomerVisit{}).Where("customer_business_id IN ?", linkIDs).Update("feedback", "").Error; err != nil {
				return err
			}
			if err := tx.Model(&database.CustomerCommunication{}).Where("customer_business_id IN ?", linkIDs).Update("error_message", "").Error; err != nil {
				return err
			}
		}

		// Pluck terminal order IDs before detaching customer_id so the status
		// history rows (driver GPS fixes, notes) can be scrubbed too.
		var terminalOrderIDs []uint
		if err := tx.Model(&database.DeliveryOrder{}).
			Where("customer_id = ? AND status IN ?", customerID, []database.DeliveryStatus{
				database.DeliveryStatusDelivered,
				database.DeliveryStatusCancelled,
				database.DeliveryStatusFailed,
			}).
			Pluck("id", &terminalOrderIDs).Error; err != nil {
			return err
		}
		if len(terminalOrderIDs) == 0 {
			return nil
		}

		if err := tx.Model(&database.DeliveryOrder{}).
			Where("id IN ?", terminalOrderIDs).
			Updates(map[string]interface{}{
				// Precise drop-off and last driver fix locate the customer's
				// home as well as the street address does.
				"dropoff_latitude":           nil,
				"dropoff_longitude":          nil,
				"dropoff_timestamp":          nil,
				"current_latitude":           nil,
				"current_longitude":          nil,
				"current_timestamp":          nil,
				"customer_name":              "Deleted customer",
				"customer_phone":             "",
				"customer_email":             nil,
				"delivery_street":            nil,
				"delivery_apartment":         nil,
				"delivery_city":              nil,
				"delivery_state":             nil,
				"delivery_postal_code":       nil,
				"delivery_country":           nil,
				"delivery_formatted_address": nil,
				"delivery_instructions":      nil,
				"customer_id":                nil,
			}).Error; err != nil {
			return err
		}

		if err := tx.Model(&database.DeliveryStatusHistory{}).
			Where("delivery_order_id IN ?", terminalOrderIDs).
			Updates(map[string]interface{}{
				"latitude":  nil,
				"longitude": nil,
				"timestamp": nil,
				"notes":     "",
			}).Error; err != nil {
			return err
		}

		return nil
	})
}

// CustomerExportRow is one narrow CRM export record. It carries the operator
// CSV columns plus the consent flag used to redact phone. Password hashes and
// verification tokens are never selected.
type CustomerExportRow struct {
	CustomerBusinessID uint
	Email              string
	Name               string
	Phone              string
	ShareData          *bool
	LoyaltyPoints      int
	LoyaltyTier        string
	TotalSpent         float64
	VisitCount         int
	LastVisitAt        *time.Time
	FirstVisitAt       time.Time
	OptInMarketing     bool
	OptInEmail         bool
	OptInSMS           bool
	Notes              string
	Tags               string
}

const customerExportPageSize = 500

const customerExportSelect = "customer_businesses.id AS customer_business_id, " +
	"customers.email, customers.name, customers.phone, " +
	"customer_preferences.share_data_with_businesses AS share_data, " +
	"customer_businesses.loyalty_points, customer_businesses.loyalty_tier, " +
	"customer_businesses.total_spent, customer_businesses.visit_count, " +
	"customer_businesses.last_visit_at, customer_businesses.first_visit_at, " +
	"customer_businesses.opt_in_marketing, customer_businesses.opt_in_email, " +
	"customer_businesses.opt_in_sms, customer_businesses.notes, customer_businesses.tags"

// ExportBusinessCustomers streams active customers for a business in keyset
// pages of customerExportPageSize. Phone is cleared unless the customer has
// opted in to sharing, matching sanitizeCustomerForOperator.
func (s *Service) ExportBusinessCustomers(businessID uint, emit func(CustomerExportRow) error) error {
	var lastID uint
	for {
		var page []CustomerExportRow
		err := s.db.Table("customer_businesses").
			Select(customerExportSelect).
			Joins("JOIN customers ON customers.id = customer_businesses.customer_id").
			Joins("LEFT JOIN customer_preferences ON customer_preferences.customer_id = customers.id").
			Where("customer_businesses.business_id = ? AND customer_businesses.is_active = ? AND customer_businesses.id > ?", businessID, true, lastID).
			Order("customer_businesses.id").
			Limit(customerExportPageSize).
			Scan(&page).Error
		if err != nil {
			return err
		}
		for i := range page {
			row := page[i]
			if row.ShareData == nil || !*row.ShareData {
				row.Phone = ""
			}
			if err := emit(row); err != nil {
				return err
			}
			lastID = row.CustomerBusinessID
		}
		if len(page) < customerExportPageSize {
			return nil
		}
	}
}
