package crm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func setupLoyaltyHandlerDB(t *testing.T) (*gorm.DB, *Handler) {
	t.Helper()
	db := setupCRMHandlerTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.LoyaltyProgram{}, &database.LoyaltyTier{}))
	handler := NewHandler(NewService(db))
	return db, handler
}

func seedLoyaltyTestBusiness(t *testing.T, db *gorm.DB, id uint) {
	t.Helper()
	business := &database.Business{
		BusinessId:     fmt.Sprintf("loyalty-%d", id),
		OwnerAddress:   fmt.Sprintf("0xowner%d", id),
		Name:           "Loyalty Biz",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
	}
	business.ID = id
	require.NoError(t, db.Create(business).Error)
}

func TestGetLoyaltyHandler_ReturnsProgramAndTiers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupLoyaltyHandlerDB(t)
	seedLoyaltyTestBusiness(t, db, 42)

	program := &database.LoyaltyProgram{BusinessID: 42, Enabled: true, PointsPerDollar: 1.5}
	require.NoError(t, db.Create(program).Error)
	require.NoError(t, db.Create(&database.LoyaltyTier{LoyaltyProgramID: program.ID, Name: "Bronze", MinLifetimeSpentCents: 0, SortOrder: 0}).Error)
	require.NoError(t, db.Create(&database.LoyaltyTier{LoyaltyProgramID: program.ID, Name: "Silver", MinLifetimeSpentCents: 20000, SortOrder: 1}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "42"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/businesses/42/crm/loyalty", nil)

	handler.GetLoyalty(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Program database.LoyaltyProgram `json:"program"`
		Tiers   []database.LoyaltyTier  `json:"tiers"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 1.5, resp.Program.PointsPerDollar)
	require.Len(t, resp.Tiers, 2)
}

func TestGetLoyaltyHandler_RepairsDuplicateLoadedTierNamesAndThresholds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupLoyaltyHandlerDB(t)
	seedLoyaltyTestBusiness(t, db, 43)

	program := &database.LoyaltyProgram{BusinessID: 43, Enabled: true, PointsPerDollar: 1.5}
	require.NoError(t, db.Create(program).Error)
	require.NoError(t, db.Create(&database.LoyaltyTier{
		LoyaltyProgramID:      program.ID,
		Name:                  "Bronze",
		MinLifetimeSpentCents: 0,
		SortOrder:             0,
	}).Error)
	require.NoError(t, db.Create(&database.LoyaltyTier{
		LoyaltyProgramID:      program.ID,
		Name:                  "Silver",
		MinLifetimeSpentCents: 25000,
		SortOrder:             1,
	}).Error)
	require.NoError(t, db.Create(&database.LoyaltyTier{
		LoyaltyProgramID:      program.ID,
		Name:                  "Gold",
		MinLifetimeSpentCents: 75000,
		SortOrder:             2,
	}).Error)
	// This is the exact live drift: a second Bronze at the same threshold.
	require.NoError(t, db.Create(&database.LoyaltyTier{
		LoyaltyProgramID:      program.ID,
		Name:                  " bronze ",
		MinLifetimeSpentCents: 0,
		SortOrder:             3,
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "43"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/businesses/43/crm/loyalty", nil)

	handler.GetLoyalty(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Valid           bool                   `json:"valid"`
		ValidationError string                 `json:"validation_error"`
		Tiers           []database.LoyaltyTier `json:"tiers"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Valid, "repaired ladder must be valid: %s", resp.ValidationError)
	require.Empty(t, resp.ValidationError)
	require.Len(t, resp.Tiers, 3)
	require.Equal(t, []string{"Bronze", "Silver", "Gold"}, []string{
		resp.Tiers[0].Name,
		resp.Tiers[1].Name,
		resp.Tiers[2].Name,
	})

	var persistedCount int64
	require.NoError(t, db.Model(&database.LoyaltyTier{}).
		Where("loyalty_program_id = ?", program.ID).
		Count(&persistedCount).Error)
	require.Equal(t, int64(3), persistedCount, "GET repair must remove the invalid persisted row")
}

func TestGetLoyaltyHandler_MarksPersistedNonIncreasingThresholdsInvalid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupLoyaltyHandlerDB(t)
	seedLoyaltyTestBusiness(t, db, 44)

	program := &database.LoyaltyProgram{BusinessID: 44, Enabled: true, PointsPerDollar: 1}
	require.NoError(t, db.Create(program).Error)
	require.NoError(t, db.Create(&database.LoyaltyTier{
		LoyaltyProgramID:      program.ID,
		Name:                  "Bronze",
		MinLifetimeSpentCents: 25000,
		SortOrder:             0,
	}).Error)
	require.NoError(t, db.Create(&database.LoyaltyTier{
		LoyaltyProgramID:      program.ID,
		Name:                  "Silver",
		MinLifetimeSpentCents: 10000,
		SortOrder:             1,
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "44"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/businesses/44/crm/loyalty", nil)

	handler.GetLoyalty(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Valid           bool                   `json:"valid"`
		ValidationError string                 `json:"validation_error"`
		Tiers           []database.LoyaltyTier `json:"tiers"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.False(t, resp.Valid)
	require.Equal(t, "loyalty tier spend thresholds must increase with tier order", resp.ValidationError)
	require.Len(t, resp.Tiers, 2)

	var persistedCount int64
	require.NoError(t, db.Model(&database.LoyaltyTier{}).
		Where("loyalty_program_id = ?", program.ID).
		Count(&persistedCount).Error)
	require.Equal(t, int64(2), persistedCount, "ambiguous threshold drift must remain for operator repair")
}

func TestGetLoyaltyHandler_RepairsDuplicateThresholdAndPrefersNamedTierWhenNameIsBlank(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupLoyaltyHandlerDB(t)
	seedLoyaltyTestBusiness(t, db, 45)

	program := &database.LoyaltyProgram{BusinessID: 45, Enabled: true, PointsPerDollar: 1}
	require.NoError(t, db.Create(program).Error)
	require.NoError(t, db.Create(&database.LoyaltyTier{
		LoyaltyProgramID:      program.ID,
		Name:                  "",
		MinLifetimeSpentCents: 10000,
		SortOrder:             0,
	}).Error)
	require.NoError(t, db.Create(&database.LoyaltyTier{
		LoyaltyProgramID:      program.ID,
		Name:                  "Silver",
		MinLifetimeSpentCents: 10000,
		SortOrder:             1,
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "45"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/businesses/45/crm/loyalty", nil)

	handler.GetLoyalty(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Valid bool                   `json:"valid"`
		Tiers []database.LoyaltyTier `json:"tiers"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Valid, "named survivor should produce a valid ladder")
	require.Len(t, resp.Tiers, 1)
	require.Equal(t, "Silver", resp.Tiers[0].Name)

	var persistedCount int64
	require.NoError(t, db.Model(&database.LoyaltyTier{}).
		Where("loyalty_program_id = ?", program.ID).
		Count(&persistedCount).Error)
	require.Equal(t, int64(1), persistedCount, "runtime repair must remove the duplicate threshold and blank survivor")
}

func TestPutLoyaltyHandlerLocksProgramAndCustomerBusinessesDuringRecalculation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupLoyaltyHandlerDB(t)
	seedLoyaltyTestBusiness(t, db, 46)
	program := &database.LoyaltyProgram{BusinessID: 46, Enabled: true, PointsPerDollar: 1}
	require.NoError(t, db.Create(program).Error)
	require.NoError(t, db.Create(&database.LoyaltyTier{
		LoyaltyProgramID: program.ID, Name: "Bronze", MinLifetimeSpentCents: 0, SortOrder: 0,
	}).Error)
	require.NoError(t, db.Create(&database.CustomerBusiness{
		CustomerID: 1, BusinessID: 46, IsActive: true, TotalSpent: 300,
		LoyaltyTier: "Bronze",
	}).Error)

	var programLocked, customerLocked atomic.Bool
	var lockOrder []string
	callbackName := fmt.Sprintf("payverge:test:capture_loyalty_save_locks:%s", t.Name())
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Schema == nil {
			return
		}
		locking, ok := tx.Statement.Clauses["FOR"]
		lock, locked := locking.Expression.(clause.Locking)
		if !ok || !locked || lock.Strength != "UPDATE" {
			return
		}
		switch tx.Statement.Schema.Table {
		case "loyalty_programs":
			programLocked.Store(true)
			lockOrder = append(lockOrder, "loyalty_programs")
		case "customer_businesses":
			customerLocked.Store(true)
			lockOrder = append(lockOrder, "customer_businesses")
		}
	}))
	defer func() { _ = db.Callback().Query().Remove(callbackName) }()

	body, _ := json.Marshal(map[string]any{
		"enabled":                      true,
		"points_per_dollar":            1.0,
		"redemption_points_per_dollar": 100,
		"tiers": []map[string]any{
			{"name": "Bronze", "min_lifetime_spent": 0, "sort_order": 0},
			{"name": "Silver", "min_lifetime_spent": 200, "sort_order": 1},
		},
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "46"}}
	c.Request = httptest.NewRequest(http.MethodPut, "/businesses/46/crm/loyalty", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.PutLoyalty(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.True(t, programLocked.Load(), "ladder save must lock the program row before replacing tiers")
	require.True(t, customerLocked.Load(), "ladder save must lock customer-business rows before recalculation")
	programLockIndex, customerLockIndex := -1, -1
	for i, table := range lockOrder {
		if table == "loyalty_programs" && programLockIndex == -1 {
			programLockIndex = i
		}
		if table == "customer_businesses" && customerLockIndex == -1 {
			customerLockIndex = i
		}
	}
	require.Less(t, customerLockIndex, programLockIndex, "loyalty save must lock customer-business rows before the loyalty program")
	var customerBusiness database.CustomerBusiness
	require.NoError(t, db.Where("business_id = ?", 46).First(&customerBusiness).Error)
	require.Equal(t, "Silver", customerBusiness.LoyaltyTier)
}

func TestPutLoyaltyHandler_UpsertsAndReplacesTiers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupLoyaltyHandlerDB(t)
	seedLoyaltyTestBusiness(t, db, 7)

	body, _ := json.Marshal(map[string]any{
		"enabled":                      true,
		"points_per_dollar":            2.0,
		"redemption_points_per_dollar": 100,
		"tiers": []map[string]any{
			{"name": "Bronze", "min_lifetime_spent": 0, "sort_order": 0},
			{"name": "Gold", "min_lifetime_spent": 500, "sort_order": 1},
		},
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	c.Request = httptest.NewRequest(http.MethodPut, "/businesses/7/crm/loyalty", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.PutLoyalty(c)

	require.Equal(t, http.StatusOK, w.Code)

	var saved database.LoyaltyProgram
	require.NoError(t, db.Preload("Tiers").Where("business_id = ?", 7).First(&saved).Error)
	require.Equal(t, 2.0, saved.PointsPerDollar)
	require.Equal(t, 100.0, saved.RedemptionPointsPerDollar)
	require.True(t, saved.Enabled)
	require.Len(t, saved.Tiers, 2)
	// 500 dollars on the wire → 50_000 cents in DB.
	var gold database.LoyaltyTier
	for _, tier := range saved.Tiers {
		if tier.Name == "Gold" {
			gold = tier
		}
	}
	require.Equal(t, int64(50000), gold.MinLifetimeSpentCents)
}

func TestPutLoyaltyHandlerNormalizesGappedSortOrdersBeforeAddingTier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupLoyaltyHandlerDB(t)
	seedLoyaltyTestBusiness(t, db, 47)

	program := &database.LoyaltyProgram{BusinessID: 47, Enabled: true, PointsPerDollar: 1}
	require.NoError(t, db.Create(program).Error)
	for _, tier := range []database.LoyaltyTier{
		{LoyaltyProgramID: program.ID, Name: "Bronze", MinLifetimeSpentCents: 0, SortOrder: 0},
		{LoyaltyProgramID: program.ID, Name: "Silver", MinLifetimeSpentCents: 20000, SortOrder: 4},
		{LoyaltyProgramID: program.ID, Name: "Gold", MinLifetimeSpentCents: 100000, SortOrder: 9},
	} {
		require.NoError(t, db.Create(&tier).Error)
	}

	body, _ := json.Marshal(map[string]any{
		"enabled":                      true,
		"points_per_dollar":            1.0,
		"redemption_points_per_dollar": 100,
		"tiers": []map[string]any{
			// This is a repaired ladder whose named survivor retained sort_order
			// 1, leaving a gap at 0.
			{"name": "Bronze", "min_lifetime_spent": 0, "sort_order": 1},
			{"name": "Silver", "min_lifetime_spent": 200, "sort_order": 2},
			{"name": "Gold", "min_lifetime_spent": 500, "sort_order": 3},
			// This is the frontend's old tiers.length value, colliding with Gold.
			{"name": "Platinum", "min_lifetime_spent": 750, "sort_order": 3},
		},
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "47"}}
	c.Request = httptest.NewRequest(http.MethodPut, "/businesses/47/crm/loyalty", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.PutLoyalty(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var saved database.LoyaltyProgram
	require.NoError(t, db.Preload("Tiers", func(tx *gorm.DB) *gorm.DB {
		return tx.Order("sort_order ASC, id ASC")
	}).Where("business_id = ?", 47).First(&saved).Error)
	require.Len(t, saved.Tiers, 4)
	for i, tier := range saved.Tiers {
		require.Equal(t, i, tier.SortOrder, "tier %q must have a unique canonical sort_order", tier.Name)
	}
	require.Equal(t, []string{"Bronze", "Silver", "Gold", "Platinum"}, []string{
		saved.Tiers[0].Name,
		saved.Tiers[1].Name,
		saved.Tiers[2].Name,
		saved.Tiers[3].Name,
	})
}

func TestPutLoyaltyHandler_RejectsOutOfRangeRate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupLoyaltyHandlerDB(t)
	seedLoyaltyTestBusiness(t, db, 9)

	body, _ := json.Marshal(map[string]any{
		"enabled":                      true,
		"points_per_dollar":            250.0,
		"redemption_points_per_dollar": 100,
		"tiers":                        []map[string]any{},
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "9"}}
	c.Request = httptest.NewRequest(http.MethodPut, "/businesses/9/crm/loyalty", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.PutLoyalty(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPutLoyaltyHandler_RejectsDuplicateTierNames(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupLoyaltyHandlerDB(t)
	seedLoyaltyTestBusiness(t, db, 21)

	body, _ := json.Marshal(map[string]any{
		"enabled":                      true,
		"points_per_dollar":            1.0,
		"redemption_points_per_dollar": 100,
		"tiers": []map[string]any{
			{"name": "Bronze", "min_lifetime_spent": 0, "sort_order": 0},
			// Case-insensitive duplicate of "Bronze".
			{"name": "bronze", "min_lifetime_spent": 250, "sort_order": 1},
		},
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "21"}}
	c.Request = httptest.NewRequest(http.MethodPut, "/businesses/21/crm/loyalty", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.PutLoyalty(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	// Nothing should have been persisted.
	var count int64
	require.NoError(t, db.Model(&database.LoyaltyProgram{}).Where("business_id = ?", 21).Count(&count).Error)
	require.Zero(t, count)
}

func TestPutLoyaltyHandler_RejectsOverlappingThresholds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupLoyaltyHandlerDB(t)
	seedLoyaltyTestBusiness(t, db, 22)

	body, _ := json.Marshal(map[string]any{
		"enabled":                      true,
		"points_per_dollar":            1.0,
		"redemption_points_per_dollar": 100,
		"tiers": []map[string]any{
			{"name": "Bronze", "min_lifetime_spent": 100, "sort_order": 0},
			// Same threshold (100 dollars → 10_000 cents) as Bronze.
			{"name": "Silver", "min_lifetime_spent": 100, "sort_order": 1},
		},
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "22"}}
	c.Request = httptest.NewRequest(http.MethodPut, "/businesses/22/crm/loyalty", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.PutLoyalty(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var count int64
	require.NoError(t, db.Model(&database.LoyaltyTier{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestPutLoyaltyHandler_RejectsOutOfRangeThresholdCents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupLoyaltyHandlerDB(t)
	seedLoyaltyTestBusiness(t, db, 25)

	// Wire shape is min_lifetime_spent in dollars. -0.05 unmarshals to
	// min_lifetime_spent_cents -5.
	body, _ := json.Marshal(map[string]any{
		"enabled":                      true,
		"points_per_dollar":            1.0,
		"redemption_points_per_dollar": 100,
		"tiers": []map[string]any{
			{"name": "Bronze", "min_lifetime_spent": -0.05, "sort_order": 0},
		},
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "25"}}
	c.Request = httptest.NewRequest(http.MethodPut, "/businesses/25/crm/loyalty", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.PutLoyalty(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "loyalty tier spend thresholds must be between 0 and 1,000,000,000.00")
	var count int64
	require.NoError(t, db.Model(&database.LoyaltyProgram{}).Where("business_id = ?", 25).Count(&count).Error)
	require.Zero(t, count)
}

func TestPutLoyaltyHandler_RejectsInvalidThresholdOrdering(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupLoyaltyHandlerDB(t)
	seedLoyaltyTestBusiness(t, db, 23)

	body, _ := json.Marshal(map[string]any{
		"enabled":                      true,
		"points_per_dollar":            1.0,
		"redemption_points_per_dollar": 100,
		"tiers": []map[string]any{
			{"name": "Bronze", "min_lifetime_spent": 250, "sort_order": 0},
			{"name": "Silver", "min_lifetime_spent": 100, "sort_order": 1},
		},
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "23"}}
	c.Request = httptest.NewRequest(http.MethodPut, "/businesses/23/crm/loyalty", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.PutLoyalty(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPreviewLoyaltyHandler_RejectsInvalidLoadedLadder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupLoyaltyHandlerDB(t)
	seedLoyaltyTestBusiness(t, db, 24)

	body, _ := json.Marshal(map[string]any{
		"points_per_dollar":            1.0,
		"redemption_points_per_dollar": 100,
		"tiers": []map[string]any{
			{"name": "Bronze", "min_lifetime_spent": 0, "sort_order": 0},
			{"name": "bronze", "min_lifetime_spent": 250, "sort_order": 1},
		},
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "24"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/businesses/24/crm/loyalty/preview", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.PreviewLoyalty(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPreviewLoyaltyHandler_RejectsOutOfRangeRate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupLoyaltyHandlerDB(t)
	seedLoyaltyTestBusiness(t, db, 24)

	body, _ := json.Marshal(map[string]any{
		"points_per_dollar":            250.0,
		"redemption_points_per_dollar": 100,
		"tiers": []map[string]any{
			{"name": "Bronze", "min_lifetime_spent": 0, "sort_order": 0},
		},
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "24"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/businesses/24/crm/loyalty/preview", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.PreviewLoyalty(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPreviewLoyaltyHandler_ReturnsDistribution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupLoyaltyHandlerDB(t)
	seedLoyaltyTestBusiness(t, db, 11)

	customers := []database.CustomerBusiness{
		{CustomerID: 1, BusinessID: 11, IsActive: true, TotalSpent: 50},
		{CustomerID: 2, BusinessID: 11, IsActive: true, TotalSpent: 300},
		{CustomerID: 3, BusinessID: 11, IsActive: true, TotalSpent: 600},
	}
	for i := range customers {
		require.NoError(t, db.Create(&customers[i]).Error)
	}

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
	c.Params = gin.Params{{Key: "id", Value: "11"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/businesses/11/crm/loyalty/preview", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.PreviewLoyalty(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		TotalCustomers   int            `json:"total_customers"`
		TierDistribution map[string]int `json:"tier_distribution"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 3, resp.TotalCustomers)
	require.Equal(t, 1, resp.TierDistribution["Bronze"])
	require.Equal(t, 1, resp.TierDistribution["Silver"])
	require.Equal(t, 1, resp.TierDistribution["Gold"])
}
