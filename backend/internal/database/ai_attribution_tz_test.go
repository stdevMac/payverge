package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// L4-9: conversation day/hour buckets must use business TZ, not UTC.
// America/Argentina/Buenos_Aires is UTC-3 year-round (no DST).
func TestGetAiConversationTimeSeries_BucketsInBusinessTZ(t *testing.T) {
	business := setupAiAttributionDB(t, nil)

	ar, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	require.NoError(t, err)

	// 2026-03-15 02:00 UTC = 2026-03-14 23:00 AR. UTC bucketing would put this
	// on hour 2 / day 15; business TZ must put it on hour 23 / day 14.
	createdUTC := time.Date(2026, 3, 15, 2, 0, 0, 0, time.UTC)

	conv := AiWaiterConversation{
		BusinessID: business.ID,
		TableCode:  "T1",
		SessionID:  fmt.Sprintf("tz-bucket-%d", time.Now().UnixNano()),
		Mode:       "ordering",
		Status:     "closed",
		CreatedAt:  createdUTC,
		UpdatedAt:  createdUTC,
	}
	require.NoError(t, GetDB().Create(&conv).Error)
	require.NoError(t, GetDB().Model(&conv).Update("created_at", createdUTC).Error)

	windowStart := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	sevenDayStart := time.Date(2026, 3, 10, 0, 0, 0, 0, ar)

	series, err := GetAiConversationTimeSeries(business.ID, windowStart, sevenDayStart, ar)
	require.NoError(t, err)
	require.Equal(t, int64(1), series.Total30d)
	require.Equal(t, 1, series.HourCounts[23], "hour must be business-local 23, not UTC 2")
	require.Equal(t, 0, series.HourCounts[2], "UTC hour must not receive the bucket")
	require.Equal(t, 1, series.DayCounts["2026-03-14"], "day key must be business-local calendar date")
	require.Equal(t, 0, series.DayCounts["2026-03-15"], "UTC date must not receive the bucket")
}

func TestGetAiConversationTimeSeries_UTCUnchanged(t *testing.T) {
	business := setupAiAttributionDB(t, nil)

	createdUTC := time.Date(2026, 3, 15, 14, 30, 0, 0, time.UTC)
	conv := AiWaiterConversation{
		BusinessID: business.ID,
		TableCode:  "T1",
		SessionID:  fmt.Sprintf("tz-utc-%d", time.Now().UnixNano()),
		Mode:       "ordering",
		Status:     "closed",
		CreatedAt:  createdUTC,
		UpdatedAt:  createdUTC,
	}
	require.NoError(t, GetDB().Create(&conv).Error)
	require.NoError(t, GetDB().Model(&conv).Update("created_at", createdUTC).Error)

	windowStart := createdUTC.AddDate(0, 0, -7)
	series, err := GetAiConversationTimeSeries(business.ID, windowStart, windowStart, time.UTC)
	require.NoError(t, err)
	require.Equal(t, 1, series.HourCounts[14])
	require.Equal(t, 1, series.DayCounts["2026-03-15"])
}
