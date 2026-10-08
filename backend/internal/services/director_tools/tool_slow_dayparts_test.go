package director_tools

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlowDaypartsTool_Name(t *testing.T) {
	tool := &SlowDaypartsTool{}
	assert.Equal(t, "get_slow_dayparts", tool.Name())
}

func TestSlowDaypartsTool_HumanLabel(t *testing.T) {
	tool := &SlowDaypartsTool{}
	assert.NotEmpty(t, tool.HumanLabel("en"))
	assert.NotEmpty(t, tool.HumanLabel("es"))
}

func TestSlowDaypartsTool_Schema(t *testing.T) {
	tool := &SlowDaypartsTool{}
	schema := tool.Schema()
	require.NotNil(t, schema)
	require.NotNil(t, schema.Properties["period"])
	require.NotNil(t, schema.Properties["min_orders"])
}

func TestSlowDaypartsTool_Run(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Slow Dayparts Bistro")

	// Seed bills across hours of today. Three "busy" hours with $200
	// each (10am, 1pm, 7pm) and one "slow" hour with $30 (3pm). Mean
	// across 4 buckets = (200+200+200+30)/4 = 157.5 → 60% threshold = 94.5.
	// Only the 3pm bucket falls below 94.5 with order_count >= 1.
	gormDB := db.GetGorm()
	now := time.Now()
	hourly := map[int]int64{
		10: 20000, // 10:00 — busy
		13: 20000, // 13:00 — busy
		15: 3000,  // 15:00 — slow
		19: 20000, // 19:00 — busy
	}

	for hour, cents := range hourly {
		ts := time.Date(now.Year(), now.Month(), now.Day(), hour, 30, 0, 0, time.UTC)
		bill := database.Bill{
			BusinessID:  bizID,
			BillNumber:  fmt.Sprintf("DPT-%02d", hour),
			Status:      database.BillStatusOpen,
			Items:       "[]",
			TotalAmount: cents,
			CreatedAt:   ts,
			UpdatedAt:   ts,
		}
		require.NoError(t, gormDB.Create(&bill).Error)
	}

	tool := &SlowDaypartsTool{}
	env := ToolEnv{
		BusinessID: bizID,
		Locale:     "en",
		DB:         db,
	}

	result, err := tool.Run(context.Background(), map[string]any{"period": "day", "min_orders": float64(1)}, env)
	require.NoError(t, err)

	assert.Equal(t, "day", result.Data["period"])
	slow, ok := result.Data["slow"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, slow, 1)
	assert.Equal(t, 15, slow[0]["hour_start"])
	assert.Equal(t, 16, slow[0]["hour_end"])
	assert.Equal(t, 1, slow[0]["orders"])
	assert.Contains(t, result.Summary, "1 slow")
}

func TestSlowDaypartsTool_RespectsMinOrders(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Min Orders Slow Bistro")
	gormDB := db.GetGorm()

	now := time.Now()
	// Two busy hours + one slow hour with a single order: min_orders=2
	// should filter the slow hour out.
	hours := []struct {
		hour  int
		cents int64
	}{
		{10, 20000},
		{13, 20000},
		{15, 3000},
	}
	for _, h := range hours {
		ts := time.Date(now.Year(), now.Month(), now.Day(), h.hour, 30, 0, 0, time.UTC)
		require.NoError(t, gormDB.Create(&database.Bill{
			BusinessID:  bizID,
			BillNumber:  fmt.Sprintf("MIN-%02d", h.hour),
			Status:      database.BillStatusOpen,
			Items:       "[]",
			TotalAmount: h.cents,
			CreatedAt:   ts,
			UpdatedAt:   ts,
		}).Error)
	}

	tool := &SlowDaypartsTool{}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	result, err := tool.Run(context.Background(), map[string]any{"period": "day", "min_orders": float64(2)}, env)
	require.NoError(t, err)
	slow := result.Data["slow"].([]map[string]any)
	assert.Empty(t, slow)
}

func TestSlowDaypartsTool_DefaultsToWeek(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Default Period Slow Bistro")
	tool := &SlowDaypartsTool{}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	result, err := tool.Run(context.Background(), map[string]any{}, env)
	require.NoError(t, err)
	assert.Equal(t, "week", result.Data["period"])
}

func TestSlowDaypartsTool_ValidatesPeriod(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Bad Period Slow")
	tool := &SlowDaypartsTool{}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	_, err := tool.Run(context.Background(), map[string]any{"period": "decade"}, env)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "period"))
}
