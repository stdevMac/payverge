package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// BenchmarkGetAiConversationTimeSeriesSQLite is the decision-14 microbench for
// the L4-9 business-TZ timeseries path (single grouped scan + cached localized
// SQL fragments). Fixture: 500 conversations over 30 days, America/Argentina
// (UTC-3, no DST).
func BenchmarkGetAiConversationTimeSeriesSQLite(b *testing.B) {
	business := setupAiAttributionDB(b, nil)
	ar, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	require.NoError(b, err)

	now := time.Date(2026, 3, 20, 18, 0, 0, 0, time.UTC)
	for i := 0; i < 500; i++ {
		created := now.Add(-time.Duration(i%30) * 24 * time.Hour).Add(time.Duration(i%24) * time.Hour)
		c := AiWaiterConversation{
			BusinessID: business.ID, TableCode: "T1",
			SessionID: fmt.Sprintf("bench-ts-%d", i), Mode: "ordering", Status: "closed",
			CreatedAt: created, UpdatedAt: created,
		}
		require.NoError(b, GetDB().Create(&c).Error)
		require.NoError(b, GetDB().Model(&c).Update("created_at", created).Error)
	}

	windowStart := now.AddDate(0, 0, -30)
	localNow := now.In(ar)
	localMidnight := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, ar)
	sevenDayStart := localMidnight.AddDate(0, 0, -6)

	// Warm cache + plan.
	_, err = GetAiConversationTimeSeries(business.ID, windowStart, sevenDayStart, ar)
	require.NoError(b, err)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := GetAiConversationTimeSeries(business.ID, windowStart, sevenDayStart, ar); err != nil {
			b.Fatal(err)
		}
	}
}
