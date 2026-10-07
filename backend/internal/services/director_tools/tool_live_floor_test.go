package director_tools

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLiveFloorTool_ReportsOccupancyAndOpenChecks(t *testing.T) {
	db := newTestDB(t)
	bizID := seedLiveFloorBusiness(t, db)

	tool := &LiveFloorTool{}
	result, err := tool.Run(context.Background(), map[string]any{}, ToolEnv{
		BusinessID: bizID, Locale: "es", DB: db,
	})
	require.NoError(t, err)

	assert.Equal(t, 10, result.Data["tables_total"])
	assert.Equal(t, 7, result.Data["available"])
	assert.Equal(t, 3, result.Data["occupied"])
	free, ok := result.Data["free_tables"].([]string)
	require.True(t, ok)
	assert.Contains(t, free, "4")
	assert.NotContains(t, free, "1")

	assert.InDelta(t, 33.17, result.Data["open_checks_total"].(float64), 0.01)
	assert.Equal(t, 3, result.Data["open_checks"])
	assert.Contains(t, result.Summary, "7")
	assert.NotContains(t, result.Summary, "qty_sold")
	assert.NotContains(t, result.Summary, "weekly_revenue")
	assert.NotContains(t, strings.ToLower(result.Summary), "get_live_floor")
}

func TestLiveFloorTool_IncludesServiceCallWait(t *testing.T) {
	db := newTestDB(t)
	bizID := seedLiveFloorBusiness(t, db)

	tool := &LiveFloorTool{}
	result, err := tool.Run(context.Background(), map[string]any{}, ToolEnv{
		BusinessID: bizID, Locale: "en", DB: db,
	})
	require.NoError(t, err)

	calls, ok := result.Data["service_calls"].([]map[string]any)
	require.True(t, ok)
	require.NotEmpty(t, calls)
	assert.Equal(t, "1", calls[0]["table"])
	assert.Equal(t, "Sam Server", calls[0]["claimed_by"])
	assert.GreaterOrEqual(t, calls[0]["wait_minutes"].(int), 20)
}

func TestLiveFloorTool_OmitsServiceCallsPastSeatingSLA(t *testing.T) {
	db := newTestDB(t)
	bizID := seedLiveFloorBusiness(t, db)

	var table2 database.Table
	require.NoError(t, db.GetGorm().Where("business_id = ? AND name = ?", bizID, "2").First(&table2).Error)
	staleStarted := time.Now().Add(-26 * time.Hour)
	require.NoError(t, db.GetGorm().Create(&database.OperationalAlert{
		BusinessID:   bizID,
		AlertType:    database.OperationalAlertTypeServiceCall,
		ResourceType: database.OperationalAlertResourceTypeTable,
		ResourceID:   int64(table2.ID),
		Status:       database.OperationalAlertStatusOpen,
		Priority:     database.OperationalAlertPriorityUrgent,
		Title:        "Water",
		Body:         "Table 2 asked for water",
		LastEventAt:  staleStarted,
		CreatedAt:    staleStarted,
		Metadata:     database.JSONRawMessage(`{}`),
	}).Error)

	tool := &LiveFloorTool{}
	result, err := tool.Run(context.Background(), map[string]any{}, ToolEnv{
		BusinessID: bizID, Locale: "en", DB: db,
	})
	require.NoError(t, err)

	calls, ok := result.Data["service_calls"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, calls, 1)
	assert.Equal(t, "1", calls[0]["table"])
}

func TestLiveFloorTool_KeepsSameSeatingCheckPleaseOmitsEmptyTable(t *testing.T) {
	db := newTestDB(t)
	bizID := seedLiveFloorBusiness(t, db)

	var table3, table4 database.Table
	require.NoError(t, db.GetGorm().Where("business_id = ? AND name = ?", bizID, "3").First(&table3).Error)
	require.NoError(t, db.GetGorm().Where("business_id = ? AND name = ?", bizID, "4").First(&table4).Error)
	staleStarted := time.Now().Add(-26 * time.Hour)
	require.NoError(t, db.GetGorm().Model(&database.Bill{}).
		Where("business_id = ? AND table_id = ?", bizID, table3.ID).
		Update("created_at", staleStarted.Add(-time.Hour)).Error)
	require.NoError(t, db.GetGorm().Create(&database.OperationalAlert{
		BusinessID:   bizID,
		AlertType:    database.OperationalAlertTypeServiceCall,
		ResourceType: database.OperationalAlertResourceTypeTable,
		ResourceID:   int64(table3.ID),
		Status:       database.OperationalAlertStatusOpen,
		Priority:     database.OperationalAlertPriorityUrgent,
		Title:        "Check",
		Body:         "Table 3 asked for the check",
		LastEventAt:  staleStarted,
		CreatedAt:    staleStarted,
		Metadata:     database.JSONRawMessage(`{"reason":"check"}`),
	}).Error)
	require.NoError(t, db.GetGorm().Create(&database.OperationalAlert{
		BusinessID:   bizID,
		AlertType:    database.OperationalAlertTypeServiceCall,
		ResourceType: database.OperationalAlertResourceTypeTable,
		ResourceID:   int64(table4.ID),
		Status:       database.OperationalAlertStatusOpen,
		Priority:     database.OperationalAlertPriorityUrgent,
		Title:        "Check",
		Body:         "Table 4 asked for the check",
		LastEventAt:  staleStarted,
		CreatedAt:    staleStarted,
		Metadata:     database.JSONRawMessage(`{"reason":"check"}`),
	}).Error)

	tool := &LiveFloorTool{}
	result, err := tool.Run(context.Background(), map[string]any{}, ToolEnv{
		BusinessID: bizID, Locale: "en", DB: db,
	})
	require.NoError(t, err)

	calls, ok := result.Data["service_calls"].([]map[string]any)
	require.True(t, ok)
	tables := make([]string, 0, len(calls))
	for _, call := range calls {
		tables = append(tables, call["table"].(string))
	}
	assert.Contains(t, tables, "1")
	assert.Contains(t, tables, "3")
	assert.NotContains(t, tables, "4")
}

func TestLiveFloorTool_ReportsOpenCashDrawer(t *testing.T) {
	db := newTestDB(t)
	bizID := seedLiveFloorBusiness(t, db)

	tool := &LiveFloorTool{}
	result, err := tool.Run(context.Background(), map[string]any{}, ToolEnv{
		BusinessID: bizID, Locale: "es", DB: db,
	})
	require.NoError(t, err)

	drawer, ok := result.Data["cash_drawer"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, drawer["open"])
	assert.InDelta(t, 125.00, drawer["expected_cash"].(float64), 0.01)
	assert.Contains(t, strings.ToLower(result.Summary), "caja")
}

func seedLiveFloorBusiness(t *testing.T, db *database.DB) uint {
	t.Helper()
	require.NoError(t, db.GetGorm().AutoMigrate(
		&database.Table{},
		&database.Staff{},
		&database.OperationalAlert{},
		&database.CashRegisterSession{},
	))
	bizID := createTestBusiness(t, db, "Floor Lounge")

	for i := 1; i <= 10; i++ {
		tbl := database.Table{
			BusinessID: bizID,
			Name:       strconv.Itoa(i),
			TableCode:  "floor-" + strconv.Itoa(i) + "-" + t.Name(),
			Capacity:   4,
			IsActive:   true,
		}
		require.NoError(t, db.GetGorm().Create(&tbl).Error)
	}

	var table1, table2 database.Table
	require.NoError(t, db.GetGorm().Where("business_id = ? AND name = ?", bizID, "1").First(&table1).Error)
	require.NoError(t, db.GetGorm().Where("business_id = ? AND name = ?", bizID, "2").First(&table2).Error)

	bill1 := database.Bill{
		BusinessID:     bizID,
		TableID:        table1.ID,
		BillNumber:     "FLOOR-1-" + t.Name(),
		Status:         database.BillStatusOpen,
		TotalAmount:    582,
		SettlementAddr: "settle",
		TippingAddr:    "tip",
		Items:          "[]",
	}
	bill2 := database.Bill{
		BusinessID:     bizID,
		TableID:        table2.ID,
		BillNumber:     "FLOOR-2-" + t.Name(),
		Status:         database.BillStatusOpen,
		TotalAmount:    2735,
		SettlementAddr: "settle",
		TippingAddr:    "tip",
		Items:          "[]",
	}
	require.NoError(t, db.GetGorm().Create(&bill1).Error)
	require.NoError(t, db.GetGorm().Create(&bill2).Error)

	var table3 database.Table
	require.NoError(t, db.GetGorm().Where("business_id = ? AND name = ?", bizID, "3").First(&table3).Error)
	bill3 := database.Bill{
		BusinessID:     bizID,
		TableID:        table3.ID,
		BillNumber:     "FLOOR-3-" + t.Name(),
		Status:         database.BillStatusOpen,
		TotalAmount:    0,
		SettlementAddr: "settle",
		TippingAddr:    "tip",
		Items:          "[]",
	}
	require.NoError(t, db.GetGorm().Create(&bill3).Error)

	claimed := "Sam Server"
	waitStarted := time.Now().Add(-22 * time.Minute)
	alert := database.OperationalAlert{
		BusinessID:    bizID,
		AlertType:     database.OperationalAlertTypeServiceCall,
		ResourceType:  database.OperationalAlertResourceTypeTable,
		ResourceID:    int64(table1.ID),
		Status:        database.OperationalAlertStatusOpen,
		Priority:      database.OperationalAlertPriorityUrgent,
		Title:         "Agua",
		Body:          "Table 1 asked for water",
		ClaimedByName: claimed,
		LastEventAt:   waitStarted,
		CreatedAt:     waitStarted,
		Metadata:      database.JSONRawMessage(`{}`),
	}
	require.NoError(t, db.GetGorm().Create(&alert).Error)

	session := database.CashRegisterSession{
		BusinessID:        bizID,
		Status:            database.CashRegisterSessionStatusOpen,
		OpeningFloatCents: 10000,
		ExpectedCashCents: 12500,
		OpenedAt:          time.Now().Add(-4 * time.Hour),
		OpenedByLabel:     "owner",
	}
	require.NoError(t, db.GetGorm().Create(&session).Error)
	return bizID
}
