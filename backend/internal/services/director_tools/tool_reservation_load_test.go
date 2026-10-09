package director_tools

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReservationLoadTool_Name(t *testing.T) {
	tool := &ReservationLoadTool{}
	assert.Equal(t, "get_reservation_load", tool.Name())
}

func TestReservationLoadTool_HumanLabel(t *testing.T) {
	tool := &ReservationLoadTool{}
	assert.NotEmpty(t, tool.HumanLabel("en"))
	assert.NotEmpty(t, tool.HumanLabel("es"))
}

func TestReservationLoadTool_Schema(t *testing.T) {
	tool := &ReservationLoadTool{}
	schema := tool.Schema()
	require.NotNil(t, schema)
	require.NotNil(t, schema.Properties["lookback_days"])
	require.NotNil(t, schema.Properties["lookahead_days"])
}

func TestReservationLoadTool_Run(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Reservations Bistro")

	gormDB := db.GetGorm()
	now := time.Now()

	// Today: 2 reservations totaling 6 covers.
	// Tomorrow: 1 reservation, 2 covers.
	// 8 days ahead: OUT OF WINDOW (default lookahead is 7).
	plans := []struct {
		dayOffset int
		party     int
	}{
		{0, 4},
		{0, 2},
		{1, 2},
		{-2, 5}, // within 7-day lookback
		{8, 3},  // outside default window
	}
	for i, p := range plans {
		ts := now.AddDate(0, 0, p.dayOffset)
		r := database.TableReservation{
			BusinessID:      bizID,
			CustomerName:    fmt.Sprintf("Guest-%d", i),
			PartySize:       p.party,
			ReservationTime: ts,
			Status:          "confirmed",
			CreatedAt:       ts,
			UpdatedAt:       ts,
		}
		require.NoError(t, gormDB.Create(&r).Error)
	}

	tool := &ReservationLoadTool{}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	result, err := tool.Run(context.Background(), map[string]any{
		"lookback_days":  float64(7),
		"lookahead_days": float64(7),
	}, env)
	require.NoError(t, err)
	assert.Equal(t, 7, result.Data["lookback_days"])
	assert.Equal(t, 7, result.Data["lookahead_days"])

	byDay, ok := result.Data["by_day"].([]map[string]any)
	require.True(t, ok, "by_day should be []map[string]any")
	require.NotEmpty(t, byDay)

	// 4 reservations should be inside [-2, +7] inclusive; the +8 row is out.
	totalReservations := 0
	totalCovers := 0
	for _, row := range byDay {
		totalReservations += row["reservations"].(int)
		totalCovers += row["covers"].(int)
	}
	assert.Equal(t, 4, totalReservations)
	assert.Equal(t, 4+2+2+5, totalCovers)
}

func TestReservationLoadTool_DefaultsToSevenAndSeven(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Default Reservations Bistro")

	tool := &ReservationLoadTool{}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	result, err := tool.Run(context.Background(), map[string]any{}, env)
	require.NoError(t, err)
	assert.Equal(t, 7, result.Data["lookback_days"])
	assert.Equal(t, 7, result.Data["lookahead_days"])
}

func TestReservationLoadTool_ValidatesBounds(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Bad Reservations Bounds")
	tool := &ReservationLoadTool{}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	_, err := tool.Run(context.Background(), map[string]any{"lookback_days": float64(-1)}, env)
	require.Error(t, err)
}
