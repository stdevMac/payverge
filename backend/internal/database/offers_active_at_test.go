package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newOffersTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	prev := db
	t.Cleanup(func() { SetTestDB(prev) })
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	g, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := g.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, g.AutoMigrate(&Business{}, &Offer{}))
	SetTestDB(g)
	return g
}

func TestGetActiveOffersByBusinessIDAt_WeekdayLunchDaypartNY(t *testing.T) {
	g := newOffersTestDB(t)
	biz := Business{BusinessId: "lunch-ny", Name: "NY Cafe", Timezone: "America/New_York", IsActive: true}
	require.NoError(t, g.Create(&biz).Error)

	start, end := 11*60, 15*60
	require.NoError(t, g.Create(&Offer{
		BusinessID: biz.ID, Name: "Weekday Lunch 15% Off", DiscountType: "percentage", DiscountValue: 15,
		IsActive: true, ApplicableTo: "all", WeekdayMask: 62, StartMinute: &start, EndMinute: &end,
	}).Error)

	ny, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	// Wednesday 2026-08-12.
	noon := time.Date(2026, 8, 12, 12, 0, 0, 0, ny)
	afterLunch := time.Date(2026, 8, 12, 15, 14, 0, 0, ny)

	atNoon, err := GetActiveOffersByBusinessIDAt(biz.ID, noon, biz.Timezone)
	require.NoError(t, err)
	require.Len(t, atNoon, 1, "lunch offer must be listed at 12:00 America/New_York")
	require.Equal(t, "Weekday Lunch 15% Off", atNoon[0].Name)
	require.NotNil(t, atNoon[0].StartMinute)
	require.NotNil(t, atNoon[0].EndMinute)
	require.Equal(t, 660, *atNoon[0].StartMinute)
	require.Equal(t, 900, *atNoon[0].EndMinute)

	atDinner, err := GetActiveOffersByBusinessIDAt(biz.ID, afterLunch, biz.Timezone)
	require.NoError(t, err)
	require.Empty(t, atDinner, "lunch offer must be hidden at 15:14 America/New_York")
}

func TestFilterOffersActiveAt_UsesBusinessTimezone(t *testing.T) {
	start, end := 11*60, 15*60
	offers := []Offer{{
		Name: "Weekday Lunch 15% Off", WeekdayMask: 62, StartMinute: &start, EndMinute: &end, IsActive: true,
	}}
	ny, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	utcDinner := time.Date(2026, 8, 12, 19, 14, 0, 0, time.UTC) // 15:14 EDT

	require.Empty(t, FilterOffersActiveAt(offers, utcDinner, "America/New_York"))
	require.Len(t, FilterOffersActiveAt(offers, time.Date(2026, 8, 12, 16, 0, 0, 0, time.UTC), "America/New_York"), 1)
	require.Empty(t, FilterOffersActiveAt(offers, utcDinner, ""), "empty timezone falls back to UTC (19:14 is past 15:00)")
	_ = ny
}
