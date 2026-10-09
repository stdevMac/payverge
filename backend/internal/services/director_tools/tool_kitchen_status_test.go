package director_tools

import (
	"context"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKitchenStatusTool_ListsTicketsAndKitchenStaff(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.GetGorm().AutoMigrate(&database.Staff{}))
	bizID := createTestBusiness(t, db, "Kitchen Lounge")

	require.NoError(t, db.GetGorm().Create(&database.Staff{
		BusinessID: bizID,
		Email:      "paolo@example.test",
		Name:       "Paolo Ferrari",
		Role:       database.StaffRoleKitchen,
		IsActive:   true,
		InvitedBy:  "owner",
	}).Error)
	require.NoError(t, db.GetGorm().Create(&database.Staff{
		BusinessID: bizID,
		Email:      "sam@example.test",
		Name:       "Sam Server",
		Role:       database.StaffRoleServer,
		IsActive:   true,
		InvitedBy:  "owner",
	}).Error)

	bill := database.Bill{
		BusinessID:     bizID,
		BillNumber:     "KDS-1-" + t.Name(),
		Status:         database.BillStatusOpen,
		SettlementAddr: "settle",
		TippingAddr:    "tip",
		Items:          "[]",
	}
	require.NoError(t, db.GetGorm().Create(&bill).Error)
	require.NoError(t, db.GetGorm().Create(&database.Order{
		BillID:      bill.ID,
		BusinessID:  bizID,
		OrderNumber: "K-1",
		Status:      database.OrderStatusInKitchen,
		CreatedBy:   "guest",
		Items:       `[{"name":"Steak Plate","quantity":1}]`,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&database.Order{
		BillID:      bill.ID,
		BusinessID:  bizID,
		OrderNumber: "K-2",
		Status:      database.OrderStatusOrderReady,
		CreatedBy:   "guest",
		Items:       `[{"name":"Harvest Bowl","quantity":1}]`,
	}).Error)

	tool := &KitchenStatusTool{}
	result, err := tool.Run(context.Background(), map[string]any{}, ToolEnv{
		BusinessID: bizID, Locale: "es", DB: db,
	})
	require.NoError(t, err)

	assert.Equal(t, 1, result.Data["in_kitchen"])
	assert.Equal(t, 1, result.Data["ready"])
	staff, ok := result.Data["kitchen_staff"].([]string)
	require.True(t, ok)
	assert.Contains(t, staff, "Paolo Ferrari")
	assert.NotContains(t, staff, "Sam Server")
	assert.Contains(t, result.Summary, "Paolo")
	assert.NotContains(t, strings.ToLower(result.Summary), "get_kitchen")
	assert.NotContains(t, result.Summary, "qty_sold")
}
