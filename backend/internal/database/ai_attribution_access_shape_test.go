package database

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type aiSeriesSQLCapture struct {
	logger.Interface
	sqls []string
}

func (c *aiSeriesSQLCapture) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	c.sqls = append(c.sqls, sql)
}

// TestGetAiConversationTimeSeries_NoSelectStar is the decision-14 access-shape
// gate for the L4-9 path: aggregates only (no SELECT *), two GROUP BY scans
// (hour 30d + day 7d). A merged day×hour single scan was measured and reverted
// because it regressed B/op and allocs/op on SQLite.
func TestGetAiConversationTimeSeries_NoSelectStar(t *testing.T) {
	cap := &aiSeriesSQLCapture{Interface: logger.Default.LogMode(logger.Silent)}
	dsn := fmt.Sprintf("file:ai_series_shape_%d?mode=memory&cache=shared", time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: cap})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&Business{}, &AiWaiterConversation{}))
	prev := GetDB()
	SetTestDB(gormDB)
	t.Cleanup(func() { SetTestDB(prev) })

	biz := &Business{
		BusinessId: "ai-series-shape", Name: "Shape", OwnerAddress: "0xShape",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
	}
	require.NoError(t, gormDB.Create(biz).Error)

	ar, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	require.NoError(t, err)
	now := time.Date(2026, 3, 20, 15, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		created := now.AddDate(0, 0, -i)
		c := AiWaiterConversation{
			BusinessID: biz.ID, TableCode: "T1",
			SessionID: fmt.Sprintf("shape-%d", i), Mode: "ordering", Status: "closed",
			CreatedAt: created, UpdatedAt: created,
		}
		require.NoError(t, gormDB.Create(&c).Error)
		require.NoError(t, gormDB.Model(&c).Update("created_at", created).Error)
	}

	windowStart := now.AddDate(0, 0, -30)
	sevenDayStart := time.Date(2026, 3, 14, 0, 0, 0, 0, ar)
	cap.sqls = nil
	series, err := GetAiConversationTimeSeries(biz.ID, windowStart, sevenDayStart, ar)
	require.NoError(t, err)
	require.Equal(t, int64(10), series.Total30d)

	var groupByScans int
	for _, raw := range cap.sqls {
		low := strings.ToLower(raw)
		if !strings.Contains(low, "ai_waiter_conversations") {
			continue
		}
		if strings.Contains(low, "group by") {
			groupByScans++
		}
		require.NotContains(t, low, "select *", "timeseries must not SELECT *")
	}
	require.Equal(t, 2, groupByScans,
		"expected two GROUP BY aggregates (hour + day), got %d in %v",
		groupByScans, cap.sqls)
}
