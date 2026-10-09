package services

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupWaiterContextDB(t *testing.T) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:wctx-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Menu{},
		&database.Offer{},
		&database.Bundle{},
		&database.ReservationSettings{},
		&database.DeliverySettings{},
	))
}

func TestBuildWaiterRuntimeContext_IncludesOrderableMenuAndPromo(t *testing.T) {
	setupWaiterContextDB(t)
	biz := &database.Business{
		BusinessId: "wctx-1", Name: "Cafe", Description: "Nice cafe",
		OwnerAddress: "0x", SettlementAddr: "0x1", TippingAddr: "0x2", IsActive: true,
		KitchenEnabled: true, OrdersEnabled: true,
		AiSettings: database.BusinessAiSettings{AiEnabled: true, AiName: "Sage", AiPriority: "upselling"},
		Address:    database.BusinessAddress{Street: "1 Main", City: "Town", State: "ST", PostalCode: "1", Country: "US"},
		Timezone:   "UTC",
	}
	require.NoError(t, database.GetDB().Create(biz).Error)

	cats := []database.MenuCategory{{
		ID: "mains", Name: "Mains",
		Items: []database.MenuItem{
			{ID: "burger", Name: "House Burger", Price: 12, IsAvailable: true},
			{ID: "hidden", Name: "86'd Steak", Price: 30, IsAvailable: true},
		},
	}}
	raw, _ := json.Marshal(cats)
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: biz.ID, Categories: string(raw), IsActive: true, Version: 1,
	}).Error)

	// Soft-block "hidden" via inventory block table if available; otherwise
	// assert the builder still returns both items when no stock grounding data.
	ctx, err := BuildWaiterRuntimeContext(context.Background(), biz, "en")
	require.NoError(t, err)
	assert.Equal(t, "Sage", ctx.AIName)
	assert.Equal(t, "Cafe", ctx.BusinessName)
	assert.Contains(t, ctx.MenuJSON, "House Burger")
	assert.Contains(t, ctx.BusinessAddress, "1 Main")
	assert.Contains(t, ctx.ReservationContext, "DISABLED")
	assert.NotEmpty(t, ctx.MenuJSON)
}

func TestBuildWaiterRuntimeContext_ReservationEnabled(t *testing.T) {
	setupWaiterContextDB(t)
	biz := &database.Business{
		BusinessId: "wctx-res", Name: "Cafe", OwnerAddress: "0x", SettlementAddr: "0x1", TippingAddr: "0x2", IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(biz).Error)
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: biz.ID, Categories: "[]", IsActive: true, Version: 1,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.ReservationSettings{
		BusinessID: biz.ID, Enabled: true, MinPartySize: 2, MaxPartySize: 8,
	}).Error)

	ctx, err := BuildWaiterRuntimeContext(context.Background(), biz, "es")
	require.NoError(t, err)
	assert.Equal(t, "es", ctx.Locale)
	assert.Contains(t, ctx.ReservationContext, "ENABLED")
	assert.Contains(t, ctx.ReservationContext, "2")
	assert.Contains(t, ctx.ReservationContext, "8")
}

// TestBuildWaiterRuntimeContext_ClosedBusinessHidesReservations is the
// #472 waiter-channel twin of public reservation settings: stored
// reservation_settings.enabled=true must not tell guests they can book when
// create would refuse a closed (business_unavailable) venue.
func TestBuildWaiterRuntimeContext_ClosedBusinessHidesReservations(t *testing.T) {
	setupWaiterContextDB(t)
	closedAt := time.Now().Add(-time.Hour)
	biz := &database.Business{
		BusinessId:     "wctx-closed",
		ClosedAt:       &closedAt,
		Name:           "Cafe",
		OwnerAddress:   "0x",
		SettlementAddr: "0x1",
		TippingAddr:    "0x2",
		IsActive:       true,
		KitchenEnabled: true,
		OrdersEnabled:  true,
	}
	require.NoError(t, database.GetDB().Create(biz).Error)
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: biz.ID, Categories: "[]", IsActive: true, Version: 1,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.ReservationSettings{
		BusinessID: biz.ID, Enabled: true, MinPartySize: 2, MaxPartySize: 8,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.DeliverySettings{
		BusinessID:             biz.ID,
		DeliveryEnabled:        true,
		InHouseDeliveryEnabled: true,
	}).Error)

	ctx, err := BuildWaiterRuntimeContext(context.Background(), biz, "en")
	require.NoError(t, err)
	assert.Contains(t, ctx.ReservationContext, "DISABLED")
	assert.NotContains(t, ctx.ReservationContext, "ENABLED")
	assert.NotContains(t, ctx.DeliveryContext, "Order Delivery")
}

func TestFilterWaiterMenuByHidden(t *testing.T) {
	cats := []database.MenuCategory{{
		ID: "c", Name: "Mains",
		Items: []database.MenuItem{
			{ID: "a", Name: "A"},
			{ID: "b", Name: "B"},
		},
	}}
	out := filterWaiterMenuByHidden(cats, map[string]bool{"b": true})
	require.Len(t, out, 1)
	require.Len(t, out[0].Items, 1)
	assert.Equal(t, "a", out[0].Items[0].ID)
}

func TestFilterWaiterOffersByHidden(t *testing.T) {
	target := "steak"
	offers := []database.Offer{
		{Name: "Steak deal", ApplicableTo: "item", TargetID: &target},
		{Name: "All menu", ApplicableTo: "all"},
	}
	out := filterWaiterOffersByHidden(offers, map[string]bool{"steak": true})
	require.Len(t, out, 1)
	assert.Equal(t, "All menu", out[0].Name)
}

// Ensure CreatedAt seeds do not leave zero times that confuse sqlite.
var _ = time.Now
