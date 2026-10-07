package loyalty

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// loyaltySQLCapture records every SQL statement so access-shape tests can
// prove the guest rate path no longer Preload("Tiers") or SELECT * the
// loyalty_programs row for a single float.
type loyaltySQLCapture struct {
	logger.Interface
	mu   sync.Mutex
	sqls []string
}

func (c *loyaltySQLCapture) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	c.mu.Lock()
	c.sqls = append(c.sqls, sql)
	c.mu.Unlock()
}

func (c *loyaltySQLCapture) reset() {
	c.mu.Lock()
	c.sqls = nil
	c.mu.Unlock()
}

func (c *loyaltySQLCapture) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.sqls))
	copy(out, c.sqls)
	return out
}

func (c *loyaltySQLCapture) matching(substr string) []string {
	var out []string
	for _, s := range c.snapshot() {
		if strings.Contains(strings.ToLower(s), strings.ToLower(substr)) {
			out = append(out, s)
		}
	}
	return out
}

func (c *loyaltySQLCapture) selectStarCount(table string) int {
	count := 0
	for _, statement := range c.snapshot() {
		normalized := strings.ToLower(strings.TrimSpace(statement))
		if !strings.HasPrefix(normalized, "select *") {
			continue
		}
		if strings.Contains(normalized, "from `"+table+"`") ||
			strings.Contains(normalized, "from \""+table+"\"") ||
			strings.Contains(normalized, "from "+table) {
			count++
		}
	}
	return count
}

func setupLoyaltyAccessShapeDB(t testing.TB) (*gorm.DB, *loyaltySQLCapture) {
	t.Helper()
	cap := &loyaltySQLCapture{Interface: logger.Default.LogMode(logger.Silent)}
	dsn := fmt.Sprintf("file:loyalty-shape-%s-%d?mode=memory&cache=shared",
		strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()), time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: cap})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(
		&database.Business{},
		&database.LoyaltyProgram{},
		&database.LoyaltyTier{},
	))
	return db, cap
}

func seedLoyaltyProgramWithTiers(t testing.TB, db *gorm.DB, businessID uint, rate float64) {
	t.Helper()
	program := &database.LoyaltyProgram{
		BusinessID:                businessID,
		Enabled:                   true,
		PointsPerDollar:           1,
		RedemptionPointsPerDollar: rate,
	}
	require.NoError(t, db.Create(program).Error)
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&database.LoyaltyTier{
			LoyaltyProgramID:      program.ID,
			Name:                  fmt.Sprintf("Tier %d", i+1),
			MinLifetimeSpentCents: int64((i + 1) * 10000),
			SortOrder:             i,
			Color:                 "#1a6b6a",
		}).Error)
	}
}

// TestRedemptionRateForBusinessAccessShape is the #560 access-shape gate:
// the guest loyalty-rate float must not Preload discarded Tiers or SELECT *
// the loyalty_programs row.
func TestRedemptionRateForBusinessAccessShape(t *testing.T) {
	db, cap := setupLoyaltyAccessShapeDB(t)
	const businessID uint = 42
	seedLoyaltyProgramWithTiers(t, db, businessID, 75)

	cap.reset()
	rate := RedemptionRateForBusiness(db, businessID)
	require.Equal(t, 75.0, rate)

	assert.Empty(t, cap.matching("loyalty_tiers"),
		"RedemptionRateForBusiness must not query loyalty_tiers; Tiers are unused on the rate path")
	assert.Zero(t, cap.selectStarCount("loyalty_programs"),
		"RedemptionRateForBusiness must project the rate columns, not SELECT * loyalty_programs")

	programSelects := cap.matching("loyalty_programs")
	require.NotEmpty(t, programSelects, "expected a loyalty_programs SELECT")
	joined := strings.ToLower(strings.Join(programSelects, " "))
	assert.Contains(t, joined, "redemption_points_per_dollar")
	assert.Contains(t, joined, "enabled")
	assert.NotContains(t, joined, "`points_per_dollar`",
		"earn-rate column is unused when returning the redeem float")
	assert.NotContains(t, joined, `"points_per_dollar"`,
		"earn-rate column is unused when returning the redeem float")
}

func TestRedemptionRateForBusiness_DisabledOrMissing(t *testing.T) {
	db, _ := setupLoyaltyAccessShapeDB(t)
	require.Equal(t, 0.0, RedemptionRateForBusiness(db, 99), "missing program is rate 0")

	require.NoError(t, db.Create(&database.LoyaltyProgram{
		BusinessID:                7,
		Enabled:                   true,
		RedemptionPointsPerDollar: 50,
	}).Error)
	// GORM's default:true rewrites Enabled:false on insert; force the disable.
	require.NoError(t, db.Model(&database.LoyaltyProgram{}).
		Where("business_id = ?", 7).
		Update("enabled", false).Error)
	require.Equal(t, 0.0, RedemptionRateForBusiness(db, 7), "disabled program is rate 0")
}

func BenchmarkRedemptionRateForBusiness(b *testing.B) {
	db, _ := setupLoyaltyAccessShapeDB(b)
	seedLoyaltyProgramWithTiers(b, db, 1, 100)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := RedemptionRateForBusiness(db, 1); got != 100 {
			b.Fatalf("rate = %v", got)
		}
	}
}
