//go:build integration_postgres

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/testperf/genesisdb"
)

// TestDeliveryCapConcurrentGuestCheckouts is the Postgres proof that
// concurrent guest checkouts cannot exceed MaxConcurrentDeliveries.
// Six checkouts race a cap of 2; exactly two commit.
func TestDeliveryCapConcurrentGuestCheckouts(t *testing.T) {
	ctx := context.Background()
	pg, err := genesisdb.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	previous := database.GetDB()
	database.SetTestDB(pg.DB)
	t.Cleanup(func() { database.SetTestDB(previous) })

	sqlDB, err := pg.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(20)

	business := &database.Business{
		BusinessId:          "delivery-cap-pg",
		OwnerAddress:        "0x1111111111111111111111111111111111111111",
		Name:                "Cap Race",
		SettlementAddr:      "0x2222222222222222222222222222222222222222",
		TippingAddr:         "0x3333333333333333333333333333333333333333",
		Timezone:            "UTC",
		IsActive:            true,
		BusinessPageEnabled: true,
	}
	require.NoError(t, pg.DB.Create(business).Error)

	categories := []database.MenuCategory{{
		ID:   "mains",
		Name: "Mains",
		Items: []database.MenuItem{{
			ID: "burger", Name: "Burger", Price: 12, IsAvailable: true,
		}},
	}}
	raw, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, pg.DB.Create(&database.Menu{
		BusinessID: business.ID,
		Categories: string(raw),
		IsActive:   true,
	}).Error)

	settings := &database.DeliverySettings{
		BusinessID:                  business.ID,
		DeliveryEnabled:             true,
		InHouseDeliveryEnabled:      true,
		FlatDeliveryFee:             500,
		FreeDeliveryMinimum:         0,
		MinimumOrderAmount:          0,
		EstimatedPrepTime:           20,
		MaxConcurrentDeliveries:     2,
		DeliveryHoursSameAsBusiness: false,
		DeliveryStartTime:           "00:00",
		DeliveryEndTime:             "00:00",
		ExternalPartnerLinks:        database.JSONRawMessage("[]"),
		DeliveryZones:               database.JSONRawMessage("[]"),
	}
	require.NoError(t, pg.DB.Create(settings).Error)
	// GORM omits a false bool that carries default:true, so Postgres would
	// turn delivery_hours_same_as_business back on and then fail closed
	// without operating-hours rows. Force the stored window.
	require.NoError(t, pg.DB.Model(settings).UpdateColumns(map[string]any{
		"delivery_enabled":                true,
		"in_house_delivery_enabled":       true,
		"max_concurrent_deliveries":       2,
		"delivery_hours_same_as_business": false,
		"delivery_start_time":             "00:00",
		"delivery_end_time":               "00:00",
		"minimum_order_amount":            0,
	}).Error)

	require.NoError(t, pg.DB.Create(&database.DeliveryZone{
		BusinessID:         business.ID,
		Name:               "Everywhere",
		Boundaries:         `{"postal_codes":["99999"]}`,
		DeliveryFee:        500,
		MinimumOrderAmount: 0,
		EstimatedTime:      20,
		Priority:           1,
		IsActive:           true,
	}).Error)

	service := NewDeliveryService(pg.DB, nil)
	const racers = 6
	start := make(chan struct{})
	results := make([]*GuestDeliveryCheckoutDTO, racers)
	errs := make([]error, racers)
	var wg sync.WaitGroup
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			req := tipCheckoutRequest(0)
			req.CustomerEmail = fmt.Sprintf("cap-racer-%d@example.test", i)
			req.CustomerName = fmt.Sprintf("Racer %d", i)
			results[i], errs[i] = service.GuestDeliveryCheckout(business.ID, req)
		}(i)
	}
	close(start)
	wg.Wait()

	success := 0
	for i, err := range errs {
		if err == nil {
			success++
			require.NotNil(t, results[i])
			require.NotNil(t, results[i].DeliveryOrder)
			continue
		}
		require.ErrorIsf(t, err, ErrDeliveryValidation, "racer %d: %v", i, err)
	}
	require.Equal(t, 2, success)

	var rows int64
	require.NoError(t, pg.DB.Model(&database.DeliveryOrder{}).
		Where("business_id = ?", business.ID).
		Count(&rows).Error)
	require.Equal(t, int64(2), rows)
}
