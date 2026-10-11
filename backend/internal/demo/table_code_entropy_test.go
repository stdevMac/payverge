package demo

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func seedDemoBusiness(t *testing.T, db *gorm.DB, id uint) {
	t.Helper()
	biz := database.Business{
		ID:             id,
		BusinessId:     fmt.Sprintf("demo-biz-%d", id),
		OwnerAddress:   "0xtest",
		Name:           "Demo Restaurant",
		SettlementAddr: "0xsettle",
		TippingAddr:    "0xtip",
		CounterEnabled: true,
		CounterCount:   2,
		CounterPrefix:  "D",
	}
	require.NoError(t, db.Create(&biz).Error)
}

func TestEnsureTablesAndCounters_UsesHighEntropyCodes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:demo-table-entropy?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	// ensureTablesAndCounters reads Business.counter_prefix/count for #192 naming.
	require.NoError(t, db.AutoMigrate(&database.Business{}, &database.Table{}, &database.Counter{}))
	seedDemoBusiness(t, db, 75)

	svc := &Service{}
	p := profile{Key: "secondary", BusinessIDSuffix: "secondary"}
	require.NoError(t, svc.ensureTablesAndCounters(context.Background(), db, 75, p))

	var tables []database.Table
	require.NoError(t, db.Where("business_id = ?", 75).Find(&tables).Error)
	require.Len(t, tables, 10)

	seen := map[string]struct{}{}
	for _, table := range tables {
		assert.False(t, predictableDemoTableCode.MatchString(table.TableCode),
			"table code %q must not use the legacy enumerable demo pattern", table.TableCode)
		assert.Regexp(t, `^[A-Z0-9]{10}$`, table.TableCode)
		_, dup := seen[table.TableCode]
		assert.False(t, dup, "duplicate table code %q", table.TableCode)
		seen[table.TableCode] = struct{}{}
	}

	var counters []database.Counter
	require.NoError(t, db.Where("business_id = ?", 75).Order("counter_number").Find(&counters).Error)
	require.Len(t, counters, 2)
	require.Equal(t, "D1", counters[0].Name)
	require.Equal(t, "D2", counters[1].Name)
}

func TestPredictableDemoTableCodeMatchesLiveEnumerablePatterns(t *testing.T) {
	mustMatch := []string{
		"demo-75-ai-pro-table-01",
		"demo-74-core-table-09",
		"DEMO-75-AI-PRO-TABLE-01",
		"demo-1-guest-table-01",
		"CORE-T01",
		"AI-T09",
	}
	mustNotMatch := []string{
		"ABCDEF1234",
		"K7M2Q9X4WP",
		"perf-seed-001-t01",
		"showcase-bellavista-01",
		"demo-75-table-01",
		"",
	}
	for _, code := range mustMatch {
		assert.Truef(t, predictableDemoTableCode.MatchString(code), "expected %q to match predictable demo pattern", code)
	}
	for _, code := range mustNotMatch {
		assert.Falsef(t, predictableDemoTableCode.MatchString(code), "expected %q not to match predictable demo pattern", code)
	}
}

func TestEnsureTablesAndCounters_RotatesLegacyPredictableCodes(t *testing.T) {
	cases := []struct {
		name string
		code string
	}{
		{name: "demo pattern", code: "demo-75-ai-pro-table-01"},
		{name: "demo core profile", code: "demo-74-core-table-09"},
		{name: "CORE-T pattern", code: "CORE-T01"},
		{name: "AI-T pattern", code: "AI-T02"},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dsn := "file:demo-table-rotate-" + tc.name + "?mode=memory&cache=shared"
			db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
				Logger: logger.Default.LogMode(logger.Silent),
			})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&database.Business{}, &database.Table{}, &database.Counter{}))

			bizID := uint(80 + i)
			seedDemoBusiness(t, db, bizID)
			legacy := database.Table{
				BusinessID: bizID,
				TableCode:  tc.code,
				Name:       "Mesa 1",
				Capacity:   2,
				IsActive:   true,
			}
			require.NoError(t, db.Create(&legacy).Error)

			svc := &Service{}
			p := profile{Key: "secondary", BusinessIDSuffix: "secondary"}
			require.NoError(t, svc.ensureTablesAndCounters(context.Background(), db, bizID, p))

			var updated database.Table
			require.NoError(t, db.Where("business_id = ? AND name = ?", bizID, "Mesa 1").First(&updated).Error)
			assert.NotEqual(t, tc.code, updated.TableCode)
			assert.False(t, predictableDemoTableCode.MatchString(updated.TableCode))
			assert.Regexp(t, `^[A-Z0-9]{10}$`, updated.TableCode)
		})
	}
}
