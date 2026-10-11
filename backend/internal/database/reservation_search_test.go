package database

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"
)

func createSearchReservation(
	t *testing.T,
	businessID uint,
	reservationTime time.Time,
	name, phone, email string,
) {
	t.Helper()
	require.NoError(t, db.Create(&TableReservation{
		BusinessID:       businessID,
		CustomerName:     name,
		CustomerPhone:    phone,
		CustomerEmail:    email,
		PartySize:        2,
		ReservationTime:  reservationTime,
		Duration:         60,
		Status:           "confirmed",
		Source:           "staff",
		ConfirmationCode: fmt.Sprintf("CONF-SEARCH-%d-%s", reservationTime.UnixNano(), name),
	}).Error)
}

func TestGetReservationsByBusinessIDPaginatedSearch(t *testing.T) {
	setupReservationPerfTestDB(t, logger.Default.LogMode(logger.Silent))
	biz := helperReservationPerfBusiness(t)
	start := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	createSearchReservation(t, biz.ID, start.Add(1*time.Hour), "Alice Smith", "+15551110001", "alice@example.com")
	createSearchReservation(t, biz.ID, start.Add(2*time.Hour), "Bob Jones", "+15552220002", "bob@example.com")
	createSearchReservation(t, biz.ID, start.Add(3*time.Hour), "Carol_Wildcard", "+15553330003", "carol@example.com")
	createSearchReservation(t, biz.ID, start.Add(4*time.Hour), "Percent%Guest", "+15554440004", "percent@example.com")

	t.Run("matches by customer name substring", func(t *testing.T) {
		result, err := GetReservationsByBusinessIDPaginatedWithSearch(
			biz.ID, start, end, "", "alice", PaginationParams{PageSize: 50},
		)
		require.NoError(t, err)
		require.Len(t, result.Data, 1)
		assert.Equal(t, "Alice Smith", result.Data[0].CustomerName)
	})

	t.Run("matches by phone substring", func(t *testing.T) {
		result, err := GetReservationsByBusinessIDPaginatedWithSearch(
			biz.ID, start, end, "", "2220002", PaginationParams{PageSize: 50},
		)
		require.NoError(t, err)
		require.Len(t, result.Data, 1)
		assert.Equal(t, "Bob Jones", result.Data[0].CustomerName)
	})

	t.Run("matches by email substring", func(t *testing.T) {
		result, err := GetReservationsByBusinessIDPaginatedWithSearch(
			biz.ID, start, end, "", "carol@example", PaginationParams{PageSize: 50},
		)
		require.NoError(t, err)
		require.Len(t, result.Data, 1)
		assert.Equal(t, "Carol_Wildcard", result.Data[0].CustomerName)
	})

	t.Run("escapes percent and underscore as literals", func(t *testing.T) {
		// Searching for a literal underscore must not match every name via LIKE wildcards.
		resultUnderscore, err := GetReservationsByBusinessIDPaginatedWithSearch(
			biz.ID, start, end, "", "Carol_", PaginationParams{PageSize: 50},
		)
		require.NoError(t, err)
		require.Len(t, resultUnderscore.Data, 1)
		assert.Equal(t, "Carol_Wildcard", resultUnderscore.Data[0].CustomerName)

		// Searching for a literal percent must only hit Percent%Guest, not everything.
		resultPercent, err := GetReservationsByBusinessIDPaginatedWithSearch(
			biz.ID, start, end, "", "Percent%", PaginationParams{PageSize: 50},
		)
		require.NoError(t, err)
		require.Len(t, resultPercent.Data, 1)
		assert.Equal(t, "Percent%Guest", resultPercent.Data[0].CustomerName)
	})

	t.Run("empty and whitespace search returns unfiltered set", func(t *testing.T) {
		empty, err := GetReservationsByBusinessIDPaginatedWithSearch(
			biz.ID, start, end, "", "", PaginationParams{PageSize: 50},
		)
		require.NoError(t, err)
		assert.Equal(t, int64(4), empty.Total)

		ws, err := GetReservationsByBusinessIDPaginatedWithSearch(
			biz.ID, start, end, "", "   ", PaginationParams{PageSize: 50},
		)
		require.NoError(t, err)
		assert.Equal(t, int64(4), ws.Total)
	})

	t.Run("search SQL uses escaped LIKE not SELECT star on tables", func(t *testing.T) {
		recorder := &reservationSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
		setupReservationPerfTestDB(t, recorder)
		biz2 := helperReservationPerfBusiness(t)
		createSearchReservation(t, biz2.ID, start.Add(time.Hour), "Dana", "+1", "dana@example.com")

		_, err := GetReservationsByBusinessIDPaginatedWithSearch(
			biz2.ID, start, end, "", "dana", PaginationParams{PageSize: 20},
		)
		require.NoError(t, err)

		joined := strings.ToLower(strings.Join(recorder.statements, "\n"))
		assert.Contains(t, joined, "like", "search must use LIKE against customer fields")
		assert.Contains(t, joined, "customer_name")
		assert.Contains(t, joined, "customer_phone")
		assert.Contains(t, joined, "customer_email")
		assert.Zero(t, recorder.selectStarCount("tables"), "search path must not SELECT * tables")
	})
}
