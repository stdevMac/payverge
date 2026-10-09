package server

import (
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Task 16 (b): briefing copy params interpolate the actual age of the oldest
// stale bill rather than the constant "2 hours".
func TestBuildProactiveInsights_StaleBillsUseActualOldestAge(t *testing.T) {
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Bill{}))

	business := createBusinessHandlerTestBusiness(t, "0xStaleOwner", "stale-insight")
	// One bill open for ~3 days — well past the 2h surfacing threshold.
	oldest := time.Now().Add(-72 * time.Hour)
	// A second slightly younger stale bill (~1 day) so count=2 and oldest is 72h.
	younger := time.Now().Add(-25 * time.Hour)

	for i, created := range []time.Time{oldest, younger} {
		bill := &database.Bill{
			BusinessID:  business.ID,
			BillNumber:  fmt.Sprintf("STALE-%d-%d", business.ID, i),
			Status:      database.BillStatusOpen,
			TotalAmount: 1500,
			TableID:     uint(i + 1),
		}
		require.NoError(t, database.GetDB().Create(bill).Error)
		require.NoError(t, database.GetDB().Model(&database.Bill{}).Where("id = ?", bill.ID).
			Updates(map[string]interface{}{
				"created_at": created,
				"updated_at": created,
			}).Error)
	}

	// Fresh open bill must not affect the stale insight age.
	fresh := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("FRESH-%d", business.ID),
		Status:      database.BillStatusOpen,
		TotalAmount: 500,
		TableID:     9,
	}
	require.NoError(t, database.GetDB().Create(fresh).Error)

	// Direct unit path: info must reflect 2 stale bills and ~3-day age before
	// the full insight builder (which also runs inventory/food/labor queries).
	info := getStaleOpenBillsInfo(business.ID)
	require.Equal(t, int64(2), info.StaleTotal, "fresh bill must not count; both multi-day bills are past the threshold")
	// #817: the headline count must match the advertised duration — only the
	// 72h bill is as old as the "3 days" the copy renders; the 25h bill is not.
	require.Equal(t, int64(1), info.Count)
	require.Contains(t, info.Duration, "day")
	require.GreaterOrEqual(t, info.OldestMinutes, 72*60-5)

	insights := buildProactiveInsights(business.ID)
	var stale *proactiveInsight
	for i := range insights {
		if insights[i].Type == "stale_open_bills" {
			stale = &insights[i]
			break
		}
	}
	require.NotNil(t, stale, "expected stale_open_bills insight for multi-day open bills")

	count, ok := stale.Params["count"].(int64)
	if !ok {
		// JSON-friendly numeric types may arrive as int.
		if c, okInt := stale.Params["count"].(int); okInt {
			count = int64(c)
		}
	}
	// #817: count is the number of bills as old as the rendered duration
	// ("3 days" here), not everything past the 2h threshold.
	assert.Equal(t, int64(1), count, "only the bill matching the advertised age counts in the headline")

	// threshold_minutes remains the *surfacing* threshold (2h), not the age.
	assert.Equal(t, int(staleBillThreshold/time.Minute), stale.Params["threshold_minutes"])

	// duration must describe the oldest bill's real age (~3 days), not "2 hours".
	duration, _ := stale.Params["duration"].(string)
	require.NotEmpty(t, duration, "duration must be present for honest copy")
	assert.NotContains(t, duration, "2 hour", "must not hardcode the surfacing threshold as the age")
	assert.Contains(t, duration, "day", "72h-old bill should render as days")

	oldestMinutes, ok := stale.Params["oldest_minutes"].(int)
	if !ok {
		if m, ok64 := stale.Params["oldest_minutes"].(int64); ok64 {
			oldestMinutes = int(m)
		}
	}
	// ~72h = 4320 minutes; allow clock skew of a few minutes.
	assert.GreaterOrEqual(t, oldestMinutes, 72*60-5)
	assert.LessOrEqual(t, oldestMinutes, 72*60+5)
}

// Issue #817: the briefing said "2 bills are 6+ days old" when only one was —
// the copy pairs a count of ALL bills past the 2h surfacing threshold with the
// age of the OLDEST one. The advertised (count, duration) pair must be honest:
// count only bills at least as old as the rendered duration bucket.
func TestGetStaleOpenBillsInfo_AgesEachBillNotJustOldest(t *testing.T) {
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Bill{}))

	business := createBusinessHandlerTestBusiness(t, "0xStaleAgeOwner", "stale-age-insight")

	// Mirrors the live repro: #761 partial, 6 days old; #1143 open, ~7h old
	// (tonight's service, past the 2h threshold but NOT a 6-day walkout).
	leftover := time.Now().Add(-6*24*time.Hour - 30*time.Minute)
	tonight := time.Now().Add(-7 * time.Hour)
	for i, row := range []struct {
		created time.Time
		status  database.BillStatus
	}{{leftover, database.BillStatusPartial}, {tonight, database.BillStatusOpen}} {
		bill := &database.Bill{
			BusinessID:  business.ID,
			BillNumber:  fmt.Sprintf("STALE-AGE-%d-%d", business.ID, i),
			Status:      row.status,
			TotalAmount: 3782,
			TableID:     uint(i + 1),
		}
		require.NoError(t, database.GetDB().Create(bill).Error)
		require.NoError(t, database.GetDB().Model(&database.Bill{}).Where("id = ?", bill.ID).
			Updates(map[string]interface{}{"created_at": row.created, "updated_at": row.created}).Error)
	}

	info := getStaleOpenBillsInfo(business.ID)
	require.Contains(t, info.Duration, "6 days", "oldest leftover must be aged for real")
	require.Equal(t, int64(1), info.Count,
		"only the 6-day leftover is open longer than the advertised duration; tonight's 7h bill is not")
	require.Equal(t, int64(2), info.StaleTotal, "both bills are past the 2h surfacing threshold")

	// When every stale bill really is in the oldest bucket, the count says so.
	require.NoError(t, database.GetDB().Model(&database.Bill{}).
		Where("business_id = ?", business.ID).
		Updates(map[string]interface{}{"created_at": leftover, "updated_at": leftover}).Error)
	info = getStaleOpenBillsInfo(business.ID)
	require.Equal(t, int64(2), info.Count, "two genuine 6-day bills may be counted together")
	require.Contains(t, info.Duration, "6 days")
}

func TestFormatInsightDuration_HumanReadable(t *testing.T) {
	assert.Equal(t, "45 minutes", FormatInsightDuration(45*time.Minute))
	assert.Equal(t, "1 hour", FormatInsightDuration(60*time.Minute))
	assert.Equal(t, "2 hours", FormatInsightDuration(120*time.Minute))
	assert.Equal(t, "1 day", FormatInsightDuration(24*time.Hour))
	assert.Equal(t, "3 days", FormatInsightDuration(72*time.Hour))
	assert.Equal(t, "14 days", FormatInsightDuration(14*24*time.Hour))
}

// BenchmarkGetStaleOpenBillsInfo measures the stale-bills insight read after
// #817 added the duration-bucket COUNT. The single-stale-bill case runs the
// legacy 2-query shape (COUNT + oldest row); the 50-bill case adds the one
// extra indexed COUNT that only fires when StaleTotal > 1.
func BenchmarkGetStaleOpenBillsInfo(b *testing.B) {
	gormDB := setupStaffHandlerTestDBForTB(b)
	if err := gormDB.AutoMigrate(&database.Bill{}); err != nil {
		b.Fatal(err)
	}
	business := &database.Business{
		BusinessId:     "stale-bench",
		Name:           "Stale Bench",
		OwnerAddress:   "0xStaleBench",
		SettlementAddr: "0xsettle",
		TippingAddr:    "0xtip",
		IsActive:       true,
	}
	if err := database.GetDB().Create(business).Error; err != nil {
		b.Fatal(err)
	}
	seed := func(n int) {
		if err := database.GetDB().Where("business_id = ?", business.ID).Delete(&database.Bill{}).Error; err != nil {
			b.Fatal(err)
		}
		for i := 0; i < n; i++ {
			created := time.Now().Add(-time.Duration(3+i) * time.Hour)
			bill := &database.Bill{
				BusinessID:  business.ID,
				BillNumber:  fmt.Sprintf("BENCH-%d-%d", business.ID, i),
				Status:      database.BillStatusOpen,
				TotalAmount: 1000,
				TableID:     uint(i + 1),
			}
			if err := database.GetDB().Create(bill).Error; err != nil {
				b.Fatal(err)
			}
			if err := database.GetDB().Model(&database.Bill{}).Where("id = ?", bill.ID).
				Updates(map[string]interface{}{"created_at": created, "updated_at": created}).Error; err != nil {
				b.Fatal(err)
			}
		}
	}
	for _, tc := range []struct {
		name string
		n    int
	}{{"one_stale_bill_legacy_shape", 1}, {"fifty_stale_bills_bucket_count", 50}} {
		b.Run(tc.name, func(b *testing.B) {
			seed(tc.n)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				info := getStaleOpenBillsInfo(business.ID)
				if info.StaleTotal != int64(tc.n) {
					b.Fatalf("expected %d stale bills, got %d", tc.n, info.StaleTotal)
				}
			}
		})
	}
}
