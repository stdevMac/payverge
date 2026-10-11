package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"
)

// TestGetOrdersByBusinessIDPaginatedDefaultsNewestFirst locks the legacy
// ordering: without SortAsc the first page is the newest orders (created_at
// DESC) — this is the history-list contract.
func TestGetOrdersByBusinessIDPaginatedDefaultsNewestFirst(t *testing.T) {
	business, pagination := setupOrderListPerfDB(t, logger.Default.LogMode(logger.Silent))

	result, err := GetOrdersByBusinessIDPaginated(business.ID, "", pagination)
	require.NoError(t, err)
	require.Len(t, result.Data, 100)

	// created_at strictly non-increasing across the page (newest first).
	for i := 1; i < len(result.Data); i++ {
		assert.Falsef(
			t,
			result.Data[i].CreatedAt.After(result.Data[i-1].CreatedAt),
			"default order must be newest-first; row %d is newer than row %d",
			i, i-1,
		)
	}
}

// TestGetOrdersByBusinessIDPaginatedSortAscKeepsOldest proves the FIFO fix:
// with SortAsc the first page is the OLDEST orders, so a row cap drops the
// newest rather than the oldest orders the kitchen cooks first.
func TestGetOrdersByBusinessIDPaginatedSortAscKeepsOldest(t *testing.T) {
	business, pagination := setupOrderListPerfDB(t, logger.Default.LogMode(logger.Silent))

	descPage, err := GetOrdersByBusinessIDPaginated(business.ID, "", pagination)
	require.NoError(t, err)
	require.Len(t, descPage.Data, 100)

	ascPage, err := GetOrdersByBusinessIDPaginated(business.ID, "", pagination, OrderListOptions{SortAsc: true})
	require.NoError(t, err)
	require.Len(t, ascPage.Data, 100)

	// created_at strictly non-decreasing across the page (oldest first).
	for i := 1; i < len(ascPage.Data); i++ {
		assert.Falsef(
			t,
			ascPage.Data[i].CreatedAt.Before(ascPage.Data[i-1].CreatedAt),
			"SortAsc order must be oldest-first; row %d is older than row %d",
			i, i-1,
		)
	}

	// The oldest order (ascending page, row 0) must predate the newest order
	// (descending page, row 0) — the two caps keep opposite ends.
	assert.True(
		t,
		ascPage.Data[0].CreatedAt.Before(descPage.Data[0].CreatedAt),
		"ASC page must start with an older order than the DESC page",
	)
}
