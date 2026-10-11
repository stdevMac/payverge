package crm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestGetBusinessCustomerSummary_AggregatesOverWholeBase(t *testing.T) {
	db := setupCRMHandlerTestDB(t)
	service := NewService(db)
	business := createCRMTestBusiness(t, db, "biz-summary", "0xSummaryOwner", "Summary Biz")

	now := time.Now()
	recent := now.Add(-2 * 24 * time.Hour)
	stale := now.Add(-90 * 24 * time.Hour)

	var spendTotal float64
	for i := 0; i < 25; i++ {
		customer := createCRMTestCustomer(t, db, fmt.Sprintf("summary-%d@example.com", i))
		tier := "Bronze"
		if i < 7 {
			tier = "Gold"
		}
		visit := stale
		if i < 10 {
			visit = recent
		}
		spend := float64(100 + i) // 100..124
		spendTotal += spend
		conn := &database.CustomerBusiness{
			CustomerID:   customer.ID,
			BusinessID:   business.ID,
			LoyaltyTier:  tier,
			TotalSpent:   spend,
			VisitCount:   i,
			LastVisitAt:  &visit,
			FirstVisitAt: now.Add(-120 * 24 * time.Hour),
			IsActive:     true,
		}
		require.NoError(t, db.Create(conn).Error)
	}

	summary, err := service.GetBusinessCustomerSummary(business.ID, "", "", "")
	require.NoError(t, err)

	assert.EqualValues(t, 25, summary.TotalCustomers)
	assert.EqualValues(t, 10, summary.ActiveThisMonth)
	assert.EqualValues(t, 7, summary.TopTierCount)
	assert.InDelta(t, spendTotal/25.0, summary.AvgLifetimeSpend, 0.001)
}

func TestGetBusinessCustomerSummary_RoundsAvgLifetimeSpendToCents(t *testing.T) {
	db := setupCRMHandlerTestDB(t)
	service := NewService(db)
	business := createCRMTestBusiness(t, db, "biz-avg-round", "0xAvgRound", "Avg Round Biz")
	now := time.Now()

	// Three spends that average to a non-terminating decimal: (140+327+834)/3 = 433.666...
	for i, spend := range []float64{140, 327, 834} {
		customer := createCRMTestCustomer(t, db, fmt.Sprintf("avg-round-%d@example.com", i))
		conn := &database.CustomerBusiness{
			CustomerID:   customer.ID,
			BusinessID:   business.ID,
			LoyaltyTier:  "Bronze",
			TotalSpent:   spend,
			VisitCount:   i + 1,
			FirstVisitAt: now,
			IsActive:     true,
		}
		require.NoError(t, db.Create(conn).Error)
	}

	summary, err := service.GetBusinessCustomerSummary(business.ID, "", "", "")
	require.NoError(t, err)
	assert.Equal(t, 433.67, summary.AvgLifetimeSpend)
}

func TestGetBusinessCustomers_FiltersByTierServerSide(t *testing.T) {
	db := setupCRMHandlerTestDB(t)
	service := NewService(db)
	business := createCRMTestBusiness(t, db, "biz-tier", "0xTierOwner", "Tier Biz")

	now := time.Now()
	for i := 0; i < 25; i++ {
		customer := createCRMTestCustomer(t, db, fmt.Sprintf("tier-%d@example.com", i))
		tier := "Bronze"
		if i < 7 {
			tier = "Gold"
		}
		visit := now.Add(time.Duration(-i) * time.Hour)
		conn := &database.CustomerBusiness{
			CustomerID:   customer.ID,
			BusinessID:   business.ID,
			LoyaltyTier:  tier,
			TotalSpent:   float64(10 + i),
			LastVisitAt:  &visit,
			FirstVisitAt: now,
			IsActive:     true,
		}
		require.NoError(t, db.Create(conn).Error)
	}

	// Filtering by Gold must return all 7 Gold customers (even across pages),
	// not just the Gold rows that happen to fall on the visible page.
	customers, total, err := service.GetBusinessCustomers(business.ID, 1, 20, "", "Gold")
	require.NoError(t, err)
	assert.EqualValues(t, 7, total)
	for _, c := range customers {
		assert.Equal(t, "Gold", c.LoyaltyTier)
	}
}

func TestGetBusinessCustomers_HandlerReturnsSummaryMeta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))

	business := createCRMTestBusiness(t, db, "biz-summary-handler", "0xOwner", "Biz")
	now := time.Now()
	recent := now.Add(-1 * 24 * time.Hour)
	for i := 0; i < 25; i++ {
		customer := createCRMTestCustomer(t, db, fmt.Sprintf("handler-summary-%d@example.com", i))
		tier := "Bronze"
		if i < 7 {
			tier = "Gold"
		}
		conn := &database.CustomerBusiness{
			CustomerID:   customer.ID,
			BusinessID:   business.ID,
			LoyaltyTier:  tier,
			TotalSpent:   float64(100),
			LastVisitAt:  &recent,
			FirstVisitAt: now,
			IsActive:     true,
		}
		require.NoError(t, db.Create(conn).Error)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	handler.GetBusinessCustomers(c)
	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Total   int `json:"total"`
		Summary struct {
			TotalCustomers   int     `json:"total_customers"`
			ActiveThisMonth  int     `json:"active_this_month"`
			AvgLifetimeSpend float64 `json:"avg_lifetime_spend"`
			TopTierCount     int     `json:"top_tier_count"`
		} `json:"summary"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, 25, response.Total)
	assert.Equal(t, 25, response.Summary.TotalCustomers)
	assert.Equal(t, 25, response.Summary.ActiveThisMonth)
	assert.Equal(t, 7, response.Summary.TopTierCount)
	assert.InDelta(t, 100.0, response.Summary.AvgLifetimeSpend, 0.001)
}

// L5-2: summary cards must honor the same tier filter as the list so
// TotalCustomers matches list total when tier=Gold (and TopTierCount still
// counts Gold within that scope — here equal to total).
func TestGetBusinessCustomerSummary_AppliesTierFilterLikeList(t *testing.T) {
	db := setupCRMHandlerTestDB(t)
	service := NewService(db)
	business := createCRMTestBusiness(t, db, "biz-tier-summary", "0xTierSummary", "Tier Summary Biz")

	now := time.Now()
	recent := now.Add(-2 * 24 * time.Hour)
	for i := 0; i < 25; i++ {
		customer := createCRMTestCustomer(t, db, fmt.Sprintf("tier-sum-%d@example.com", i))
		tier := "Bronze"
		if i < 7 {
			tier = "Gold"
		}
		conn := &database.CustomerBusiness{
			CustomerID:   customer.ID,
			BusinessID:   business.ID,
			LoyaltyTier:  tier,
			TotalSpent:   float64(100 + i),
			VisitCount:   i + 1,
			LastVisitAt:  &recent,
			FirstVisitAt: now.Add(-60 * 24 * time.Hour),
			IsActive:     true,
		}
		require.NoError(t, db.Create(conn).Error)
	}

	// Unfiltered baseline: full base.
	allSummary, err := service.GetBusinessCustomerSummary(business.ID, "", "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 25, allSummary.TotalCustomers)
	assert.EqualValues(t, 7, allSummary.TopTierCount)

	// Gold filter: summary totals must match list total.
	goldSummary, err := service.GetBusinessCustomerSummary(business.ID, "", "Gold", "")
	require.NoError(t, err)
	_, goldTotal, err := service.GetBusinessCustomers(business.ID, 1, 20, "", "Gold")
	require.NoError(t, err)
	assert.EqualValues(t, goldTotal, goldSummary.TotalCustomers, "summary total must match list total for tier=Gold")
	assert.EqualValues(t, 7, goldSummary.TotalCustomers)
	// Within Gold filter every row is Gold, so top-tier count equals total.
	assert.EqualValues(t, 7, goldSummary.TopTierCount)
	assert.EqualValues(t, 7, goldSummary.ActiveThisMonth)

	// Bronze filter: top-tier (Gold) count is 0 inside the filtered set.
	bronzeSummary, err := service.GetBusinessCustomerSummary(business.ID, "", "Bronze", "")
	require.NoError(t, err)
	_, bronzeTotal, err := service.GetBusinessCustomers(business.ID, 1, 50, "", "Bronze")
	require.NoError(t, err)
	assert.EqualValues(t, bronzeTotal, bronzeSummary.TotalCustomers)
	assert.EqualValues(t, 18, bronzeSummary.TotalCustomers)
	assert.EqualValues(t, 0, bronzeSummary.TopTierCount, "Gold count within Bronze filter must be 0")
}

func TestGetBusinessCustomers_HandlerSummaryHonorsTierQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	handler := NewHandler(NewService(db))

	business := createCRMTestBusiness(t, db, "biz-tier-handler", "0xTierH", "Tier Handler Biz")
	now := time.Now()
	recent := now.Add(-1 * 24 * time.Hour)
	for i := 0; i < 10; i++ {
		customer := createCRMTestCustomer(t, db, fmt.Sprintf("tier-h-%d@example.com", i))
		tier := "Bronze"
		if i < 3 {
			tier = "Gold"
		}
		conn := &database.CustomerBusiness{
			CustomerID:   customer.ID,
			BusinessID:   business.ID,
			LoyaltyTier:  tier,
			TotalSpent:   float64(50),
			LastVisitAt:  &recent,
			FirstVisitAt: now,
			IsActive:     true,
		}
		require.NoError(t, db.Create(conn).Error)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/?tier=Gold", nil)

	handler.GetBusinessCustomers(c)
	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Total   int `json:"total"`
		Summary struct {
			TotalCustomers   int     `json:"total_customers"`
			ActiveThisMonth  int     `json:"active_this_month"`
			AvgLifetimeSpend float64 `json:"avg_lifetime_spend"`
			TopTierCount     int     `json:"top_tier_count"`
		} `json:"summary"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, 3, response.Total)
	assert.Equal(t, 3, response.Summary.TotalCustomers, "handler summary must apply tier=Gold")
	assert.Equal(t, 3, response.Summary.TopTierCount)
}
