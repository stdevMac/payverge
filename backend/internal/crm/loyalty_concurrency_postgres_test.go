//go:build integration
// +build integration

package crm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestPutLoyaltyAndSettlement_ConcurrentPostgresTransactionsRemainConsistent
// exercises the two transactions that must lock customer_businesses before
// loyalty_programs. It uses a real handler and service against separate
// PostgreSQL connections so a lock-order regression can deadlock instead of
// being hidden by SQLite's single-connection test setup.
func TestPutLoyaltyAndSettlement_ConcurrentPostgresTransactionsRemainConsistent(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres integration test")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err, "open postgres")
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(4)
	require.NoError(t, sqlDB.Ping(), "ping postgres")
	require.NoError(t, db.AutoMigrate(
		&database.Business{},
		&database.Customer{},
		&database.CustomerBusiness{},
		&database.Bill{},
		&database.BillItem{},
		&database.CustomerVisit{},
		&database.LoyaltyProgram{},
		&database.LoyaltyTier{},
	))

	previousDB := database.GetDB()
	database.SetTestDB(db)
	t.Cleanup(func() { database.SetTestDB(previousDB) })

	seedSuffix := fmt.Sprintf("%d", time.Now().UnixNano())
	business := &database.Business{
		BusinessId:      "loyalty-concurrency-" + seedSuffix,
		OwnerAddress:    "0x1111111111111111111111111111111111111111",
		Name:            "Loyalty Concurrency Bistro " + seedSuffix,
		SettlementAddr:  "0x2222222222222222222222222222222222222222",
		TippingAddr:     "0x3333333333333333333333333333333333333333",
		IsActive:        true,
		CRMEnabled:      true,
		DefaultCurrency: "USD",
	}
	require.NoError(t, db.Create(business).Error)

	customer := &database.Customer{
		Email:        "loyalty-concurrency-" + seedSuffix + "@example.com",
		PasswordHash: "test-hash",
		Name:         "Concurrency Customer",
		IsActive:     true,
	}
	require.NoError(t, db.Create(customer).Error)

	connection := &database.CustomerBusiness{
		CustomerID:  customer.ID,
		BusinessID:  business.ID,
		TotalSpent:  150,
		IsActive:    true,
		VisitCount:  0,
		LoyaltyTier: "Bronze",
	}
	require.NoError(t, db.Create(connection).Error)

	program := &database.LoyaltyProgram{
		BusinessID:      business.ID,
		Enabled:         true,
		PointsPerDollar: 1,
	}
	require.NoError(t, db.Create(program).Error)
	require.NoError(t, db.Create(&database.LoyaltyTier{
		LoyaltyProgramID:      program.ID,
		Name:                  "Bronze",
		MinLifetimeSpentCents: 0,
		SortOrder:             0,
	}).Error)

	customerID := customer.ID
	bill := &database.Bill{
		BusinessID:     business.ID,
		BillNumber:     "LOYALTY-CONCURRENCY-" + seedSuffix,
		Status:         database.BillStatusPaid,
		Items:          "[]",
		TotalAmount:    10000,
		PaidAmount:     10000,
		SettlementAddr: business.SettlementAddr,
		TippingAddr:    business.TippingAddr,
		CRMCustomerID:  &customerID,
	}
	require.NoError(t, db.Create(bill).Error)

	t.Cleanup(func() {
		cleanupStatements := []struct {
			name string
			err  error
		}{
			{"customer visits", db.Where("customer_business_id = ?", connection.ID).Delete(&database.CustomerVisit{}).Error},
			{"bill", db.Delete(&database.Bill{}, bill.ID).Error},
			{"customer-business connection", db.Delete(&database.CustomerBusiness{}, connection.ID).Error},
			{"loyalty program", db.Delete(&database.LoyaltyProgram{}, program.ID).Error},
			{"customer", db.Delete(&database.Customer{}, customer.ID).Error},
			{"business", db.Delete(&database.Business{}, business.ID).Error},
		}
		for _, statement := range cleanupStatements {
			if statement.err != nil && !strings.Contains(statement.err.Error(), "record not found") {
				t.Errorf("cleanup %s: %v", statement.name, statement.err)
			}
		}
	})

	gin.SetMode(gin.TestMode)
	operationContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	operationDB := db.WithContext(operationContext)
	handler := NewHandler(NewService(operationDB))
	settlementService := NewService(operationDB)

	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	workers.Add(2)

	go func() {
		defer workers.Done()
		<-start

		payload, marshalErr := json.Marshal(map[string]any{
			"enabled":                      true,
			"points_per_dollar":            1.0,
			"redemption_points_per_dollar": 100,
			"tiers": []map[string]any{
				{"name": "Bronze", "min_lifetime_spent": 0, "sort_order": 0},
				{"name": "Silver", "min_lifetime_spent": 200, "sort_order": 1},
			},
		})
		if marshalErr != nil {
			results <- fmt.Errorf("marshal loyalty save payload: %w", marshalErr)
			return
		}

		response := httptest.NewRecorder()
		requestContext, _ := gin.CreateTestContext(response)
		requestContext.Params = gin.Params{{Key: "id", Value: strconv.FormatUint(uint64(business.ID), 10)}}
		requestContext.Request = httptest.NewRequestWithContext(
			operationContext,
			http.MethodPut,
			"/businesses/"+strconv.FormatUint(uint64(business.ID), 10)+"/crm/loyalty",
			bytes.NewReader(payload),
		)
		requestContext.Request.Header.Set("Content-Type", "application/json")
		handler.PutLoyalty(requestContext)
		if response.Code != http.StatusOK {
			results <- fmt.Errorf("loyalty save returned %d: %s", response.Code, response.Body.String())
			return
		}
		results <- nil
	}()

	go func() {
		defer workers.Done()
		<-start
		results <- settlementService.RecordBillSettlementVisit(bill.ID)
	}()

	close(start)
	workersDone := make(chan struct{})
	go func() {
		workers.Wait()
		close(workersDone)
	}()

	select {
	case <-workersDone:
	case <-time.After(12 * time.Second):
		cancel()
		t.Fatal("concurrent loyalty save and settlement did not complete within timeout; possible PostgreSQL deadlock")
	}

	for i := 0; i < 2; i++ {
		require.NoError(t, <-results, "concurrent loyalty operation %d", i+1)
	}

	var reloadedConnection database.CustomerBusiness
	require.NoError(t, db.First(&reloadedConnection, connection.ID).Error)
	require.InDelta(t, 250.0, reloadedConnection.TotalSpent, 0.001)
	require.Equal(t, 1, reloadedConnection.VisitCount)
	require.Equal(t, 100, reloadedConnection.LoyaltyPoints)
	require.Equal(t, "Silver", reloadedConnection.LoyaltyTier)

	var visitCount int64
	require.NoError(t, db.Model(&database.CustomerVisit{}).
		Where("customer_business_id = ? AND bill_id = ?", connection.ID, bill.ID).
		Count(&visitCount).Error)
	require.Equal(t, int64(1), visitCount)

	var reloadedProgram database.LoyaltyProgram
	require.NoError(t, db.Preload("Tiers", func(query *gorm.DB) *gorm.DB {
		return query.Order("sort_order ASC")
	}).First(&reloadedProgram, program.ID).Error)
	require.Len(t, reloadedProgram.Tiers, 2)
	require.Equal(t, "Bronze", reloadedProgram.Tiers[0].Name)
	require.Equal(t, "Silver", reloadedProgram.Tiers[1].Name)
	require.Equal(t, int64(20000), reloadedProgram.Tiers[1].MinLifetimeSpentCents)
}
