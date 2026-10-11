package crm

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type customerConnectionsSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *customerConnectionsSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func (r *customerConnectionsSQLRecorder) selectStarCount(table string) int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		if !strings.HasPrefix(normalized, "select *") {
			continue
		}
		if strings.Contains(normalized, "from `"+table+"`") ||
			strings.Contains(normalized, "from \""+table+"\"") ||
			strings.Contains(normalized, "from "+table) {
			count++
		}
	}
	return count
}

func setupCustomerConnectionsPerfDB(t testing.TB, gormLogger logger.Interface) (*Service, uint) {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	cfg := &gorm.Config{}
	if gormLogger != nil {
		cfg.Logger = gormLogger
	}
	gormDB, err := gorm.Open(sqlite.Open(dsn), cfg)
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(&database.Customer{}, &database.Business{}, &database.CustomerBusiness{}))

	customer := &database.Customer{
		Email:        fmt.Sprintf("customer-connections-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Name:         "Customer Connections Perf",
		IsActive:     true,
	}
	require.NoError(t, gormDB.Create(customer).Error)

	now := time.Now().UTC()
	for i := 0; i < 300; i++ {
		business := &database.Business{
			BusinessId:      fmt.Sprintf("customer-connection-%03d-%d", i, time.Now().UnixNano()),
			Name:            fmt.Sprintf("Connected Business %03d", i),
			OwnerAddress:    fmt.Sprintf("0xCustomerConnection%03d", i),
			SettlementAddr:  "0x1111111111111111111111111111111111111111",
			TippingAddr:     "0x2222222222222222222222222222222222222222",
			DefaultCurrency: "USD",
			BannerImages:    strings.Repeat("banner-payload", 256),
			SocialMedia:     strings.Repeat("social-payload", 256),
			IsActive:        true,
		}
		require.NoError(t, gormDB.Create(business).Error)

		require.NoError(t, gormDB.Create(&database.CustomerBusiness{
			CustomerID:     customer.ID,
			BusinessID:     business.ID,
			LoyaltyPoints:  i,
			TotalSpent:     float64(i * 10),
			VisitCount:     i % 20,
			FirstVisitAt:   now.Add(-time.Duration(i) * time.Hour),
			OptInMarketing: true,
			OptInEmail:     true,
			IsActive:       true,
		}).Error)
	}

	return NewService(database.GetDB()), customer.ID
}

func TestGetCustomerBusinessConnectionsProjectsBusinessSummary(t *testing.T) {
	recorder := &customerConnectionsSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	service, customerID := setupCustomerConnectionsPerfDB(t, recorder)

	recorder.statements = nil
	connections, err := service.GetCustomerBusinessConnections(customerID)
	require.NoError(t, err)
	require.Len(t, connections, 300)
	assert.NotZero(t, connections[0].Business.ID)
	assert.NotEmpty(t, connections[0].Business.Name)
	assert.Equal(t, "USD", connections[0].Business.DefaultCurrency)
	assert.Empty(t, connections[0].Business.BannerImages, "customer connection cards do not need business banner payloads")
	assert.Zero(t, recorder.selectStarCount("businesses"), "customer business connections should preload a business display projection")
}

func BenchmarkGetCustomerBusinessConnectionsSQLite(b *testing.B) {
	service, customerID := setupCustomerConnectionsPerfDB(b, logger.Default.LogMode(logger.Silent))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		connections, err := service.GetCustomerBusinessConnections(customerID)
		if err != nil {
			b.Fatal(err)
		}
		if len(connections) != 300 {
			b.Fatalf("expected 300 connections, got %d", len(connections))
		}
	}
}
