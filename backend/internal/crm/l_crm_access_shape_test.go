package crm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/structs"
)

// lcrmSQLRecorder captures every SQL statement GORM emits so tests can assert
// query count and projection shape (L-CRM access-shape gate: UB-01/UB-02/UB-03/
// N1-02/PRELOAD-05).
type lcrmSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *lcrmSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func (r *lcrmSQLRecorder) reset() { r.statements = nil }

func (r *lcrmSQLRecorder) countMatching(pred func(normalized string) bool) int {
	n := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		if pred(normalized) {
			n++
		}
	}
	return n
}

// updateCustomerBusinessesCount counts UPDATE statements against the
// customer_businesses table (the N+1 write under test for UB-03/N1-02).
func (r *lcrmSQLRecorder) updateCustomerBusinessesCount() int {
	return r.countMatching(func(s string) bool {
		return strings.HasPrefix(s, "update") &&
			(strings.Contains(s, "`customer_businesses`") || strings.Contains(s, "\"customer_businesses\"") || strings.Contains(s, " customer_businesses "))
	})
}

// selectFromCustomerBusinessesCount counts SELECT statements over the
// customer_businesses table (the aggregate/segment read under test for UB-01).
func (r *lcrmSQLRecorder) selectFromCustomerBusinessesCount() int {
	return r.countMatching(func(s string) bool {
		return strings.HasPrefix(s, "select") &&
			(strings.Contains(s, "from `customer_businesses`") || strings.Contains(s, "from \"customer_businesses\"") || strings.Contains(s, "from customer_businesses"))
	})
}

func setupLCRMHandlerDB(t *testing.T, rec logger.Interface) (*gorm.DB, *Handler) {
	t.Helper()

	previousDB := database.GetDB()
	previousSecretKey := structs.SecretKey
	t.Cleanup(func() {
		database.SetTestDB(previousDB)
		structs.SecretKey = previousSecretKey
	})

	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared",
		strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()), time.Now().UnixNano())
	cfg := &gorm.Config{}
	if rec != nil {
		cfg.Logger = rec
	} else {
		cfg.Logger = logger.Default.LogMode(logger.Silent)
	}
	db, err := gorm.Open(sqlite.Open(dsn), cfg)
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	database.SetTestDB(db)
	structs.SecretKey = []byte("test-secret-key-for-lcrm-tests")

	require.NoError(t, db.AutoMigrate(
		&database.Business{},
		&database.Customer{},
		&database.CustomerPreferences{},
		&database.CustomerBusiness{},
		&database.CustomerVisit{},
		&database.LoyaltyProgram{},
		&database.LoyaltyTier{},
		&session.UserSession{},
	))

	return db, NewHandler(NewService(db))
}

func lcrmSeedBusiness(t *testing.T, db *gorm.DB, id uint) {
	t.Helper()
	business := &database.Business{
		BusinessId:     fmt.Sprintf("lcrm-%d", id),
		OwnerAddress:   fmt.Sprintf("0xowner%d", id),
		Name:           "L-CRM Biz",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
	}
	business.ID = id
	require.NoError(t, db.Create(business).Error)
}

// lcrmSeedConnection inserts a CustomerBusiness with a real Customer + preferences.
func lcrmSeedConnection(t *testing.T, db *gorm.DB, cb *database.CustomerBusiness) {
	t.Helper()
	cust := &database.Customer{
		Email:        fmt.Sprintf("lcrm-cust-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Name:         "L-CRM Customer",
		IsActive:     true,
	}
	require.NoError(t, db.Create(cust).Error)
	require.NoError(t, db.Create(&database.CustomerPreferences{CustomerID: cust.ID, PreferredLanguage: "en", PreferredCurrency: "USD"}).Error)
	cb.CustomerID = cust.ID
	require.NoError(t, db.Create(cb).Error)
}

// --- UB-01: GetSegments single SQL aggregate -------------------------------

func TestGetSegments_SingleAggregateQueryAndCounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := &lcrmSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	db, handler := setupLCRMHandlerDB(t, rec)
	lcrmSeedBusiness(t, db, 100)

	now := time.Now().UTC()
	day := 24 * time.Hour
	mk := func(totalSpent float64, visits int, lastVisitDaysAgo int, joinDaysAgo int) *database.CustomerBusiness {
		var lv *time.Time
		if lastVisitDaysAgo >= 0 {
			v := now.Add(-time.Duration(lastVisitDaysAgo) * day)
			lv = &v
		}
		return &database.CustomerBusiness{
			BusinessID:   100,
			IsActive:     true,
			TotalSpent:   totalSpent,
			VisitCount:   visits,
			LastVisitAt:  lv,
			FirstVisitAt: now.Add(-time.Duration(joinDaysAgo) * day),
		}
	}

	// Distribution designed to hit each band with priority lapsed>atRisk>vip>new.
	// lapsed: last visit 120 days ago.
	lcrmSeedConnection(t, db, mk(100, 1, 120, 200))
	// lapsed via no-visit-but-joined-long-ago: no last visit, joined 100 days ago.
	lcrmSeedConnection(t, db, mk(50, 0, -1, 100))
	// atRisk: last visit 45 days ago.
	lcrmSeedConnection(t, db, mk(80, 2, 45, 60))
	// vip: 6 visits, spent above average, recent visit (5 days ago).
	lcrmSeedConnection(t, db, mk(5000, 6, 5, 50))
	// new: joined 10 days ago, recent visit, not vip (low spend/visits).
	lcrmSeedConnection(t, db, mk(10, 1, 3, 10))
	// regular (not emitted): recent visit 5 days, joined 200 days, low spend.
	lcrmSeedConnection(t, db, mk(20, 1, 5, 200))

	// Noise business — must not be counted.
	lcrmSeedBusiness(t, db, 999)
	lcrmSeedConnection(t, db, &database.CustomerBusiness{BusinessID: 999, IsActive: true, TotalSpent: 9999, VisitCount: 10, FirstVisitAt: now.Add(-time.Hour)})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "100"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/businesses/100/crm/segments", nil)

	rec.reset()
	handler.GetSegments(c)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var counts map[string]int
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &counts))

	// Expected from the seeded distribution under the original priority logic.
	assert.Equal(t, 2, counts["lapsed"], "lapsed count")
	assert.Equal(t, 1, counts["atRisk"], "atRisk count")
	assert.Equal(t, 1, counts["vip"], "vip count")
	assert.Equal(t, 1, counts["new"], "new count")

	// Access-shape: exactly ONE SELECT over customer_businesses.
	got := rec.selectFromCustomerBusinessesCount()
	assert.Equal(t, 1, got, "GetSegments must run a single aggregate over customer_businesses, got %d (statements: %v)", got, rec.statements)

	// And that single read must be a SQL aggregate (no per-row hydration): it
	// computes counts with SUM(CASE WHEN ...) / COUNT(...) rather than scanning
	// rows into Go. The portable SUM(CASE...) form is used so it runs on both
	// SQLite (tests) and Postgres (prod), which lacks nothing here but FILTER.
	aggregateReads := rec.countMatching(func(s string) bool {
		return strings.HasPrefix(s, "select") &&
			(strings.Contains(s, "from `customer_businesses`") || strings.Contains(s, "from customer_businesses")) &&
			(strings.Contains(s, "sum(case") || strings.Contains(s, "count(case") || strings.Contains(s, "count(*)"))
	})
	assert.Equal(t, 1, aggregateReads, "GetSegments must compute segments via a SQL aggregate, not Go loops over hydrated rows; statements: %v", rec.statements)
}

// TestGetSegments_MatchesLegacyGoLoop cross-checks the SQL aggregate against the
// original Go-loop logic over a randomized-ish distribution so behavior is
// proven identical, not just plausible.
func TestGetSegments_MatchesLegacyGoLoop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupLCRMHandlerDB(t, nil)
	lcrmSeedBusiness(t, db, 101)

	now := time.Now().UTC()
	day := 24 * time.Hour

	type seed struct {
		totalSpent       float64
		visits           int
		lastVisitDaysAgo int // -1 = none
		joinDaysAgo      int
	}
	seeds := []seed{
		{100, 1, 120, 200},
		{50, 0, -1, 100},
		{80, 2, 45, 60},
		{5000, 6, 5, 50},
		{10, 1, 3, 10},
		{20, 1, 5, 200},
		{3000, 7, 31, 120}, // atRisk (boundary just above 30)
		{4000, 8, 30, 120}, // NOT atRisk (==30), high spend+visits -> vip
		{200, 3, 91, 120},  // lapsed (>90)
		{0, 0, -1, 5},      // new (no visit, joined 5d ago)
		{6000, 9, 2, 3},    // new takes priority? join<=30 and vip? -> vip wins (checked before new)
	}
	for _, s := range seeds {
		var lv *time.Time
		if s.lastVisitDaysAgo >= 0 {
			v := now.Add(-time.Duration(s.lastVisitDaysAgo) * day)
			lv = &v
		}
		lcrmSeedConnection(t, db, &database.CustomerBusiness{
			BusinessID:   101,
			IsActive:     true,
			TotalSpent:   s.totalSpent,
			VisitCount:   s.visits,
			LastVisitAt:  lv,
			FirstVisitAt: now.Add(-time.Duration(s.joinDaysAgo) * day),
		})
	}

	// Compute the legacy expectation with the exact original Go loop.
	var rows []struct {
		TotalSpent   float64
		VisitCount   int
		LastVisitAt  *time.Time
		FirstVisitAt time.Time
	}
	require.NoError(t, db.Model(&database.CustomerBusiness{}).
		Select("total_spent, visit_count, last_visit_at, first_visit_at").
		Where("business_id = ? AND is_active = ?", 101, true).
		Find(&rows).Error)

	legacy := map[string]int{"lapsed": 0, "atRisk": 0, "vip": 0, "new": 0}
	var totalSpend float64
	for _, cb := range rows {
		totalSpend += cb.TotalSpent
	}
	avgSpend := float64(0)
	if len(rows) > 0 {
		avgSpend = totalSpend / float64(len(rows))
	}
	for _, cb := range rows {
		daysSinceVisit := -1
		if cb.LastVisitAt != nil {
			daysSinceVisit = int(now.Sub(*cb.LastVisitAt) / day)
		}
		daysSinceJoin := int(now.Sub(cb.FirstVisitAt) / day)
		switch {
		case daysSinceVisit > 90 || (daysSinceVisit < 0 && daysSinceJoin > 90):
			legacy["lapsed"]++
		case daysSinceVisit > 30 && daysSinceVisit <= 90:
			legacy["atRisk"]++
		case cb.VisitCount >= 5 && cb.TotalSpent > avgSpend:
			legacy["vip"]++
		case daysSinceJoin <= 30:
			legacy["new"]++
		}
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "101"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/businesses/101/crm/segments", nil)
	handler.GetSegments(c)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var counts map[string]int
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &counts))

	assert.Equal(t, legacy["lapsed"], counts["lapsed"], "lapsed")
	assert.Equal(t, legacy["atRisk"], counts["atRisk"], "atRisk")
	assert.Equal(t, legacy["vip"], counts["vip"], "vip")
	assert.Equal(t, legacy["new"], counts["new"], "new")
}

// --- UB-02: PreviewLoyalty projects total_spent only -----------------------

func TestPreviewLoyalty_ProjectsTotalSpentOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := &lcrmSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	db, handler := setupLCRMHandlerDB(t, rec)
	lcrmSeedBusiness(t, db, 110)

	lcrmSeedConnection(t, db, &database.CustomerBusiness{BusinessID: 110, IsActive: true, TotalSpent: 50, Notes: strings.Repeat("x", 4096)})
	lcrmSeedConnection(t, db, &database.CustomerBusiness{BusinessID: 110, IsActive: true, TotalSpent: 300, Notes: strings.Repeat("y", 4096)})
	lcrmSeedConnection(t, db, &database.CustomerBusiness{BusinessID: 110, IsActive: true, TotalSpent: 600, Notes: strings.Repeat("z", 4096)})

	body, _ := json.Marshal(map[string]any{
		"points_per_dollar":            1.0,
		"redemption_points_per_dollar": 100,
		"tiers": []map[string]any{
			{"name": "Bronze", "min_lifetime_spent": 0, "sort_order": 0},
			{"name": "Silver", "min_lifetime_spent": 200, "sort_order": 1},
			{"name": "Gold", "min_lifetime_spent": 500, "sort_order": 2},
		},
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "110"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/businesses/110/crm/loyalty/preview", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	rec.reset()
	handler.PreviewLoyalty(c)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var resp struct {
		TotalCustomers   int            `json:"total_customers"`
		TierDistribution map[string]int `json:"tier_distribution"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 3, resp.TotalCustomers)
	require.Equal(t, 1, resp.TierDistribution["Bronze"])
	require.Equal(t, 1, resp.TierDistribution["Silver"])
	require.Equal(t, 1, resp.TierDistribution["Gold"])

	// Access-shape: the read over customer_businesses must NOT select the full
	// row (no `select *` and no heavy `notes` column projected).
	selectStar := rec.countMatching(func(s string) bool {
		return strings.HasPrefix(s, "select *") &&
			(strings.Contains(s, "from `customer_businesses`") || strings.Contains(s, "from customer_businesses"))
	})
	assert.Zero(t, selectStar, "PreviewLoyalty must project total_spent, not select *; statements: %v", rec.statements)

	selectsNotes := rec.countMatching(func(s string) bool {
		return strings.HasPrefix(s, "select") &&
			strings.Contains(s, "from `customer_businesses`") &&
			strings.Contains(s, "notes")
	})
	assert.Zero(t, selectsNotes, "PreviewLoyalty must not read the notes column")
}

// --- UB-03 / N1-02: PutLoyalty batched tier updates ------------------------

func TestPutLoyalty_BatchedTierUpdatesAndCorrectness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := &lcrmSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	db, handler := setupLCRMHandlerDB(t, rec)
	lcrmSeedBusiness(t, db, 120)

	// Seed a known distribution spanning all 3 tiers + a reset case.
	// Tiers (cents): Bronze 0, Silver 20000 ($200), Gold 50000 ($500).
	// Start every customer at a STALE tier so each one changes -> max churn.
	type seed struct {
		totalSpent float64
		startTier  string
		expectTier string
	}
	seeds := []seed{
		{50, "Gold", "Bronze"},    // $50 -> Bronze
		{100, "Gold", "Bronze"},   // $100 -> Bronze
		{250, "Bronze", "Silver"}, // $250 -> Silver
		{300, "Bronze", "Silver"}, // $300 -> Silver
		{600, "Bronze", "Gold"},   // $600 -> Gold
		{900, "Silver", "Gold"},   // $900 -> Gold
		{0, "Silver", "Bronze"},   // $0 -> Bronze (min 0 qualifies)
	}
	ids := make([]uint, 0, len(seeds))
	for _, s := range seeds {
		cb := &database.CustomerBusiness{BusinessID: 120, IsActive: true, TotalSpent: s.totalSpent, LoyaltyTier: s.startTier}
		lcrmSeedConnection(t, db, cb)
		ids = append(ids, cb.ID)
	}

	body, _ := json.Marshal(map[string]any{
		"enabled":                      true,
		"points_per_dollar":            1.0,
		"redemption_points_per_dollar": 100,
		"tiers": []map[string]any{
			{"name": "Bronze", "min_lifetime_spent": 0, "sort_order": 0},
			{"name": "Silver", "min_lifetime_spent": 200, "sort_order": 1},
			{"name": "Gold", "min_lifetime_spent": 500, "sort_order": 2},
		},
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "120"}}
	c.Request = httptest.NewRequest(http.MethodPut, "/businesses/120/crm/loyalty", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	rec.reset()
	handler.PutLoyalty(c)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	// Access-shape: at most 3 UPDATE statements against customer_businesses
	// (one per distinct target tier), NOT one per row.
	updates := rec.updateCustomerBusinessesCount()
	assert.LessOrEqual(t, updates, 3, "PutLoyalty must batch tier updates to <=3 statements, got %d (statements: %v)", updates, rec.statements)

	// Correctness: final loyalty_tier matches the legacy per-row computeTier.
	for i, s := range seeds {
		var got database.CustomerBusiness
		require.NoError(t, db.Select("loyalty_tier").First(&got, ids[i]).Error)
		assert.Equal(t, s.expectTier, got.LoyaltyTier, "customer %d ($%.0f)", i, s.totalSpent)
	}
}

// TestPutLoyalty_ResetsToNoTierWhenNoLowestQualifies proves the sub-lowest-tier
// reset path: when no tier has min 0, customers below the lowest threshold get
// loyalty_tier reset to "" (matching the original computeTier "" return).
func TestPutLoyalty_ResetsToNoTierWhenNoLowestQualifies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupLCRMHandlerDB(t, nil)
	lcrmSeedBusiness(t, db, 121)

	// Tiers start at $200 — a $50 customer qualifies for NO tier -> "".
	below := &database.CustomerBusiness{BusinessID: 121, IsActive: true, TotalSpent: 50, LoyaltyTier: "Gold"}
	above := &database.CustomerBusiness{BusinessID: 121, IsActive: true, TotalSpent: 250, LoyaltyTier: "Bronze"}
	lcrmSeedConnection(t, db, below)
	lcrmSeedConnection(t, db, above)

	body, _ := json.Marshal(map[string]any{
		"enabled":                      true,
		"points_per_dollar":            1.0,
		"redemption_points_per_dollar": 100,
		"tiers": []map[string]any{
			{"name": "Silver", "min_lifetime_spent": 200, "sort_order": 0},
			{"name": "Gold", "min_lifetime_spent": 500, "sort_order": 1},
		},
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "121"}}
	c.Request = httptest.NewRequest(http.MethodPut, "/businesses/121/crm/loyalty", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	handler.PutLoyalty(c)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var gotBelow, gotAbove database.CustomerBusiness
	require.NoError(t, db.Select("loyalty_tier").First(&gotBelow, below.ID).Error)
	require.NoError(t, db.Select("loyalty_tier").First(&gotAbove, above.ID).Error)
	assert.Equal(t, "", gotBelow.LoyaltyTier, "$50 below lowest tier must reset to no-tier")
	assert.Equal(t, "Silver", gotAbove.LoyaltyTier, "$250 must land on Silver")
}

// TestPutLoyalty_SkipsUnchangedRows proves unchanged rows are not re-written
// (preserving the original skip-when-unchanged semantics, so updated_at and
// observable state stay stable).
func TestPutLoyalty_SkipsUnchangedRows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := &lcrmSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	db, handler := setupLCRMHandlerDB(t, rec)
	lcrmSeedBusiness(t, db, 122)

	// All three are ALREADY on their correct tier -> zero UPDATEs expected.
	lcrmSeedConnection(t, db, &database.CustomerBusiness{BusinessID: 122, IsActive: true, TotalSpent: 50, LoyaltyTier: "Bronze"})
	lcrmSeedConnection(t, db, &database.CustomerBusiness{BusinessID: 122, IsActive: true, TotalSpent: 250, LoyaltyTier: "Silver"})
	lcrmSeedConnection(t, db, &database.CustomerBusiness{BusinessID: 122, IsActive: true, TotalSpent: 600, LoyaltyTier: "Gold"})

	body, _ := json.Marshal(map[string]any{
		"enabled":                      true,
		"points_per_dollar":            1.0,
		"redemption_points_per_dollar": 100,
		"tiers": []map[string]any{
			{"name": "Bronze", "min_lifetime_spent": 0, "sort_order": 0},
			{"name": "Silver", "min_lifetime_spent": 200, "sort_order": 1},
			{"name": "Gold", "min_lifetime_spent": 500, "sort_order": 2},
		},
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "122"}}
	c.Request = httptest.NewRequest(http.MethodPut, "/businesses/122/crm/loyalty", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	rec.reset()
	handler.PutLoyalty(c)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	updates := rec.updateCustomerBusinessesCount()
	assert.Zero(t, updates, "no tier changed; PutLoyalty must issue zero customer_businesses UPDATEs, got %d (statements: %v)", updates, rec.statements)
}

// --- PRELOAD-05 (superseded by Wave 2 consent chokepoint): the list now loads
// Customer.Preferences with a single batched, column-projected preload solely to
// evaluate the guest's ShareDataWithBusinesses consent, then strips it before
// returning. The invariant is no longer "never read preferences" but "read them
// exactly once (batched, not per-row) and never leak them to the operator".

func TestGetBusinessCustomers_ListPreloadsConsentBatchedAndStripsIt(t *testing.T) {
	rec := &lcrmSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	db, handler := setupLCRMHandlerDB(t, rec)
	_ = handler
	lcrmSeedBusiness(t, db, 130)
	for i := 0; i < 5; i++ {
		lcrmSeedConnection(t, db, &database.CustomerBusiness{BusinessID: 130, IsActive: true, TotalSpent: float64(i * 10)})
	}

	svc := NewService(db)

	rec.reset()
	customers, total, err := svc.GetBusinessCustomers(130, 1, 20, "", "")
	require.NoError(t, err)
	require.Equal(t, int64(5), total)
	require.Len(t, customers, 5)
	assert.NotZero(t, customers[0].Customer.ID, "list must still preload the Customer")

	// LIST must query customer_preferences AT MOST ONCE (single batched preload),
	// never per-row (N+1).
	listPrefReads := rec.countMatching(func(s string) bool {
		return strings.HasPrefix(s, "select") &&
			(strings.Contains(s, "from `customer_preferences`") || strings.Contains(s, "from customer_preferences"))
	})
	assert.LessOrEqual(t, listPrefReads, 1, "GetBusinessCustomers list must load consent via a single batched preload, not per-row; statements: %v", rec.statements)

	// Preferences must never leave the service on operator reads.
	for _, cb := range customers {
		assert.Nil(t, cb.Customer.Preferences, "operators must never receive Customer.Preferences")
	}

	// DETAIL reader loads consent via a single batched preload (to evaluate
	// ShareDataWithBusinesses) and MUST strip Preferences before returning —
	// the Wave 2 chokepoint invariant, same as the list.
	rec.reset()
	detail, err := svc.GetCustomerBusinessDetails(customers[0].ID)
	require.NoError(t, err)
	require.NotNil(t, detail)
	detailPrefReads := rec.countMatching(func(s string) bool {
		return strings.HasPrefix(s, "select") &&
			(strings.Contains(s, "from `customer_preferences`") || strings.Contains(s, "from customer_preferences"))
	})
	assert.LessOrEqual(t, detailPrefReads, 1, "GetCustomerBusinessDetails must load consent via a single batched preload, not per-row; statements: %v", rec.statements)
	assert.Nil(t, detail.Customer.Preferences, "operators must never receive Customer.Preferences from the detail read")
}

// --- Benchmarks (L-CRM heaviest fixes) -------------------------------------

func benchSetupLCRMDB(b *testing.B) *gorm.DB {
	b.Helper()
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared",
		strings.NewReplacer("/", "_", " ", "_").Replace(b.Name()), time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		b.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		b.Fatalf("get sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	b.Cleanup(func() { _ = sqlDB.Close() })
	database.SetTestDB(db)
	structs.SecretKey = []byte("bench-secret")
	if err := db.AutoMigrate(
		&database.Business{}, &database.Customer{}, &database.CustomerPreferences{},
		&database.CustomerBusiness{}, &database.LoyaltyProgram{}, &database.LoyaltyTier{},
	); err != nil {
		b.Fatalf("automigrate: %v", err)
	}
	return db
}

func benchSeedConnections(b *testing.B, db *gorm.DB, businessID uint, n int) {
	b.Helper()
	biz := &database.Business{
		BusinessId:     fmt.Sprintf("bench-%d", businessID),
		OwnerAddress:   fmt.Sprintf("0xbench%d", businessID),
		Name:           "Bench Biz",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
	}
	biz.ID = businessID
	if err := db.Create(biz).Error; err != nil {
		b.Fatalf("create business: %v", err)
	}
	now := time.Now().UTC()
	for i := 0; i < n; i++ {
		cust := &database.Customer{
			Email:        fmt.Sprintf("bench-%d-%d@example.com", businessID, i),
			PasswordHash: "hash",
			Name:         fmt.Sprintf("Bench %d", i),
			IsActive:     true,
		}
		if err := db.Create(cust).Error; err != nil {
			b.Fatalf("create customer: %v", err)
		}
		var lv *time.Time
		if i%3 == 0 {
			v := now.Add(-time.Duration(i%120) * 24 * time.Hour)
			lv = &v
		}
		if err := db.Create(&database.CustomerBusiness{
			CustomerID:   cust.ID,
			BusinessID:   businessID,
			TotalSpent:   float64(i * 7),
			VisitCount:   i % 12,
			LastVisitAt:  lv,
			FirstVisitAt: now.Add(-time.Duration(i+1) * 24 * time.Hour),
			IsActive:     true,
		}).Error; err != nil {
			b.Fatalf("create customer_business: %v", err)
		}
	}
}

// BenchmarkGetSegments measures the segment computation over 200 customers
// (UB-01: full-row scan + Go loops -> single SQL aggregate).
func BenchmarkGetSegments(b *testing.B) {
	gin.SetMode(gin.TestMode)
	db := benchSetupLCRMDB(b)
	benchSeedConnections(b, db, 1, 200)
	handler := NewHandler(NewService(db))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		c.Request = httptest.NewRequest(http.MethodGet, "/businesses/1/crm/segments", nil)
		handler.GetSegments(c)
		if w.Code != http.StatusOK {
			b.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
	}
}

// BenchmarkPutLoyaltyRecompute measures the loyalty-tier recompute write path
// over 200 customers (UB-03/N1-02: per-row UPDATE loop -> batched UPDATEs).
// Each iteration alternates the tier ladder so every customer's tier flips,
// exercising the maximum write churn.
func BenchmarkPutLoyaltyRecompute(b *testing.B) {
	gin.SetMode(gin.TestMode)
	db := benchSetupLCRMDB(b)
	benchSeedConnections(b, db, 1, 200)
	handler := NewHandler(NewService(db))

	// Two ladders that classify the seeded spend differently so each PUT
	// reshuffles tiers and forces real UPDATEs.
	ladderA := []map[string]any{
		{"name": "Bronze", "min_lifetime_spent": 0, "sort_order": 0},
		{"name": "Silver", "min_lifetime_spent": 300, "sort_order": 1},
		{"name": "Gold", "min_lifetime_spent": 800, "sort_order": 2},
	}
	ladderB := []map[string]any{
		{"name": "Bronze", "min_lifetime_spent": 0, "sort_order": 0},
		{"name": "Silver", "min_lifetime_spent": 200, "sort_order": 1},
		{"name": "Gold", "min_lifetime_spent": 500, "sort_order": 2},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ladder := ladderA
		if i%2 == 1 {
			ladder = ladderB
		}
		body, _ := json.Marshal(map[string]any{
			"enabled":                      true,
			"points_per_dollar":            1.0,
			"redemption_points_per_dollar": 100,
			"tiers":                        ladder,
		})
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		c.Request = httptest.NewRequest(http.MethodPut, "/businesses/1/crm/loyalty", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		handler.PutLoyalty(c)
		if w.Code != http.StatusOK {
			b.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
	}
}
