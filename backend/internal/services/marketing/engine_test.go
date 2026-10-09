package marketing

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services/menuengineering"
)

// newMarketingDB spins up an isolated in-memory SQLite DB (mirrors
// internal/services/foodcost/calculator_test.go:newCalcDB).
func newMarketingDB(t *testing.T) *database.DB {
	t.Helper()
	prev := database.GetDB()
	t.Cleanup(func() { database.SetTestDB(prev) })
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Bill{},
		&database.Bundle{},
		&database.CustomerBusiness{},
		&database.Menu{},
		&database.Offer{},
		&database.MarketingActivity{},
	))
	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	return database.GetDBWrapper()
}

// stubMenuEng returns a fixed menu-engineering report (no DB I/O), so engine
// tests don't need recipe/inventory fixtures.
func stubMenuEng(rep menuengineering.Report) MenuEngineeringFn {
	return func(_ uint, _ string, _ *time.Location) (menuengineering.Report, error) {
		return rep, nil
	}
}

// newTestEngine builds an Engine with inventory grounding stubbed empty so
// focused SQLite fixtures (no inventory_settings) exercise ranking/copy paths.
// Production fails closed on UnrecommendableMenuItemIDs errors — override via
// SetUnmakeableLoader when testing that seam.
func newTestEngine(db *database.DB, an *analytics.AnalyticsService, menuEng MenuEngineeringFn) *Engine {
	eng := NewEngine(db, an, menuEng)
	eng.SetUnmakeableLoader(func(uint) (map[string]bool, error) {
		return map[string]bool{}, nil
	})
	return eng
}

func TestSuggest_EmptyBusiness_NoSuggestions(t *testing.T) {
	db := newMarketingDB(t)
	eng := newTestEngine(db, (*analytics.AnalyticsService)(nil), stubMenuEng(menuengineering.Report{Sparse: true}))

	got, err := suggest(context.Background(), eng, 1, nil, "en")
	require.NoError(t, err)
	require.Empty(t, got, "a business with no data yields no suggestions")
}

func TestSuggest_SeededBusiness_RanksAndCaps(t *testing.T) {
	db := newMarketingDB(t)
	g := db.GetGorm()
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)

	// One active bundle.
	require.NoError(t, g.Create(&database.Bundle{BusinessID: 1, Name: "Date Night", Price: 39, IsActive: true}).Error)
	// Happy hour requires a live offer so it does not invent a discount (#242).
	require.NoError(t, g.Create(&database.Offer{
		BusinessID: 1, Name: "After-Work Special", DiscountType: "percentage", DiscountValue: 15, IsActive: true,
	}).Error)

	// Lapsed regulars: 12 customers, last visit 40 days ago, >=2 visits.
	// CustomerID must be distinct per (customer_id, business_id) unique index.
	old := now.AddDate(0, 0, -40)
	for i := 0; i < 12; i++ {
		require.NoError(t, g.Create(&database.CustomerBusiness{
			CustomerID: uint(i + 1), BusinessID: 1, VisitCount: 3, LastVisitAt: &old, TotalSpent: 100, OptInMarketing: true,
		}).Error)
	}

	// Bills concentrated at lunch so the evening window reads as weak.
	// BillNumber must be distinct (uniqueIndex;not null on bills table).
	for h := 12; h < 14; h++ {
		ts := time.Date(2026, 6, 23, h, 0, 0, 0, time.UTC)
		require.NoError(t, g.Create(&database.Bill{BusinessID: 1, BillNumber: fmt.Sprintf("TEST-%d", h), Status: database.BillStatusPaid, TotalAmount: 5000, CreatedAt: ts}).Error)
	}
	eveTs := time.Date(2026, 6, 23, 18, 0, 0, 0, time.UTC)
	require.NoError(t, g.Create(&database.Bill{BusinessID: 1, BillNumber: "TEST-18", Status: database.BillStatusPaid, TotalAmount: 500, CreatedAt: eveTs}).Error)

	rep := menuengineering.Report{Dishes: []menuengineering.DishClass{
		{MenuItemID: "m1", MenuItemName: "Carbonara", Quadrant: "star", QtySold: 120, MarginPerUnit: 9},
		{MenuItemID: "m2", MenuItemName: "Osso Buco", Quadrant: "puzzle", Action: "promote", MarginPerUnit: 14, QtySold: 5},
	}}
	eng := newTestEngine(db, nil, stubMenuEng(rep))
	eng.SetClock(func() time.Time { return now })

	got, err := suggest(context.Background(), eng, 1, time.UTC, "en")
	require.NoError(t, err)
	// Fully-seeded business: featured, offer, happy-hour (needs live offer),
	// win-back, move-item, combo — capped at maxSuggestions but all fire.
	require.GreaterOrEqual(t, len(got), 5)
	require.LessOrEqual(t, len(got), maxSuggestions)
	gotOrder := make([]Play, 0, len(got))
	for _, s := range got {
		gotOrder = append(gotOrder, s.Play)
	}
	require.Contains(t, gotOrder, PlayFeaturedDish)
	require.Contains(t, gotOrder, PlayHappyHour)
	require.Contains(t, gotOrder, PlayWinBack)
	require.Contains(t, gotOrder, PlayMoveItem)
	require.Contains(t, gotOrder, PlayComboDeal)
	require.Equal(t, PlayFeaturedDish, got[0].Play, "star dish still ranks first")
	for _, s := range got {
		if s.Play == PlayHappyHour {
			require.Equal(t, "After-Work Special", s.Metrics["suggested_offer"])
			require.Equal(t, "percentage", s.DiscountType)
		}
	}

	// Every suggestion has a stable, unique ID.
	seen := map[string]bool{}
	for _, s := range got {
		require.NotEmpty(t, s.ID)
		require.False(t, seen[s.ID], "duplicate suggestion id %s", s.ID)
		seen[s.ID] = true
	}
}

// TestSuggest_AccessShape asserts Suggest issues a small, bounded number of
// SELECTs (no N+1): automation settings, dismissed ids, menu image index,
// dayparts, lapsed counts, bundles, offers. The settings + dismissed reads
// (Slice 4) are each a single bounded, indexed query.
func TestSuggest_AccessShape(t *testing.T) {
	db := newMarketingDB(t)
	g := db.GetGorm()
	require.NoError(t, g.Create(&database.Bundle{BusinessID: 1, Name: "Combo", Price: 20, IsActive: true}).Error)
	require.NoError(t, g.Create(&database.Offer{BusinessID: 1, Name: "Sale", DiscountType: "percentage", DiscountValue: 10, IsActive: true}).Error)
	require.NoError(t, g.Create(&database.Menu{BusinessID: 1, Categories: "[]", IsActive: true, Version: 1}).Error)

	var queries int
	require.NoError(t, g.Callback().Query().After("*").Register("count_marketing_q", func(_ *gorm.DB) { queries++ }))

	eng := newTestEngine(db, nil, stubMenuEng(menuengineering.Report{Sparse: true}))
	_, err := suggest(context.Background(), eng, 1, time.UTC, "en")
	require.NoError(t, err)

	require.LessOrEqual(t, queries, 9, "Suggest must issue a bounded number of queries (no N+1); got %d", queries)
}

func TestSuggest_IncludesActiveOffer(t *testing.T) {
	db := newMarketingDB(t)
	g := db.GetGorm()
	require.NoError(t, g.Create(&database.Offer{
		BusinessID: 1, Name: "Summer Sale", Image: "https://cdn/summer.jpg",
		DiscountType: "percentage", DiscountValue: 20, IsActive: true,
	}).Error)
	// An inactive offer must be ignored. IsActive no longer carries a
	// `gorm:"default:true"` tag, so an explicit `IsActive: false` persists
	// faithfully on Create.
	oldPromo := database.Offer{BusinessID: 1, Name: "Old Promo", DiscountType: "fixed", DiscountValue: 5, IsActive: false}
	require.NoError(t, g.Create(&oldPromo).Error)

	eng := newTestEngine(db, nil, stubMenuEng(menuengineering.Report{Sparse: true}))
	got, err := suggest(context.Background(), eng, 1, time.UTC, "en")
	require.NoError(t, err)

	var offer *CampaignSuggestion
	for i := range got {
		if got[i].Play == PlayOffer {
			offer = &got[i]
		}
	}
	require.NotNil(t, offer, "an active offer produces an offer card")
	require.Equal(t, "Summer Sale", offer.TargetName)
	require.Equal(t, "https://cdn/summer.jpg", offer.ImageURL)
	require.Equal(t, ImageSourceOffer, offer.ImageSource)
	require.Equal(t, "percentage", offer.DiscountType)
}

func TestSuggest_FeaturedDish_HasMenuImage(t *testing.T) {
	db := newMarketingDB(t)
	g := db.GetGorm()

	cats := []database.MenuCategory{{
		ID: "c1", Name: "Mains", Items: []database.MenuItem{
			// IsAvailable must be true: json.Marshal writes the bool, and S2
			// suppresses is_available:false (86'd) items.
			{ID: "m1", Name: "Carbonara", Image: "https://cdn/carbonara.jpg", Price: 12.99, Description: "Wood-fired pasta with pecorino", IsAvailable: true},
		},
	}}
	blob, err := json.Marshal(cats)
	require.NoError(t, err)
	require.NoError(t, g.Create(&database.Menu{BusinessID: 1, Categories: string(blob), IsActive: true, Version: 1}).Error)

	rep := menuengineering.Report{Dishes: []menuengineering.DishClass{
		{MenuItemID: "m1", MenuItemName: "Carbonara", Quadrant: "star", QtySold: 50, MarginPerUnit: 9, AvgPrice: 12.99},
	}}
	eng := newTestEngine(db, nil, stubMenuEng(rep))

	got, err := suggest(context.Background(), eng, 1, time.UTC, "en")
	require.NoError(t, err)
	require.NotEmpty(t, got)
	require.Equal(t, PlayFeaturedDish, got[0].Play)
	require.Equal(t, "https://cdn/carbonara.jpg", got[0].ImageURL, "featured dish uses its menu photo")
	require.Equal(t, ImageSourceMenu, got[0].ImageSource)
	require.Equal(t, "Wood-fired pasta with pecorino", got[0].TargetDescription)
}

// Live-review regression: a single-dish happy-hour post showed "add a photo to
// finish this post" even when the hero dish had a real menu photo. Name-only
// plays (happy_hour, win_back) carry the hero item's name but no TargetItemID,
// so the enrichment loop resolves their image via lookupByName. This asserts
// that path returns the dish's menu photo so Share/Download aren't blocked.
func TestMenuMetaIndex_ResolvesImageByName(t *testing.T) {
	db := newMarketingDB(t)
	g := db.GetGorm()

	cats := []database.MenuCategory{{
		ID: "c1", Name: "Mains", Items: []database.MenuItem{
			{ID: "m1", Name: "Steak Plate", Image: "https://cdn/steak.jpg", Price: 24.0, Description: "Grilled ribeye", IsAvailable: true},
		},
	}}
	blob, err := json.Marshal(cats)
	require.NoError(t, err)
	require.NoError(t, g.Create(&database.Menu{BusinessID: 1, Categories: string(blob), IsActive: true, Version: 1}).Error)

	eng := newTestEngine(db, nil, stubMenuEng(menuengineering.Report{Sparse: true}))
	idx, err := eng.menuItemMetaIndex(context.Background(), 1)
	require.NoError(t, err)

	// Resolve by exact name and by differently-cased name (lookup normalizes).
	meta, ok := idx.lookupByName("Steak Plate")
	require.True(t, ok, "hero item resolves by name so the post is photo-ready")
	require.Equal(t, "https://cdn/steak.jpg", meta.ImageURL)

	meta, ok = idx.lookupByName("  steak plate  ")
	require.True(t, ok, "name lookup is case- and whitespace-insensitive")
	require.NotEmpty(t, meta.ImageURL)

	// A dish that isn't on the menu genuinely has no image.
	_, ok = idx.lookupByName("Nonexistent Dish")
	require.False(t, ok)
}

func BenchmarkSuggest(b *testing.B) {
	t := &testing.T{}
	db := newMarketingDB(t)
	g := db.GetGorm()
	old := time.Now().AddDate(0, 0, -40)
	for i := 0; i < 50; i++ {
		_ = g.Create(&database.CustomerBusiness{CustomerID: uint(i + 1), BusinessID: 1, VisitCount: 3, LastVisitAt: &old}).Error
		_ = g.Create(&database.Bill{BusinessID: 1, BillNumber: fmt.Sprintf("BENCH-%d", i), Status: database.BillStatusPaid, TotalAmount: int64(1000 + i), CreatedAt: time.Date(2026, 6, 23, 12+(i%6), 0, 0, 0, time.UTC)}).Error
	}
	_ = g.Create(&database.Bundle{BusinessID: 1, Name: "Combo", Price: 20, IsActive: true}).Error
	_ = g.Create(&database.Offer{BusinessID: 1, Name: "Sale", Image: "https://cdn/s.jpg", DiscountType: "percentage", DiscountValue: 15, IsActive: true}).Error
	_ = g.Create(&database.Menu{BusinessID: 1, Categories: `[{"id":"c1","name":"Mains","items":[{"id":"m1","name":"Carbonara","image":"https://cdn/c.jpg"}]}]`, IsActive: true, Version: 1}).Error
	rep := menuengineering.Report{Dishes: []menuengineering.DishClass{{MenuItemID: "m1", MenuItemName: "Carbonara", Quadrant: "star", QtySold: 120, MarginPerUnit: 9}}}
	eng := newTestEngine(db, nil, stubMenuEng(rep))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = suggest(context.Background(), eng, 1, time.UTC, "en")
	}
}

func TestWeekdayShortLocalized(t *testing.T) {
	require.Equal(t, "Mié", weekdayShortLocalized(time.Wednesday, "es"))
	require.Equal(t, "Mié", weekdayShortLocalized(time.Wednesday, "es-AR"))
	require.Equal(t, "Wed", weekdayShortLocalized(time.Wednesday, "en"))
	require.Equal(t, "Wed", weekdayShortLocalized(time.Wednesday, "xx")) // fallback

	// Underscore variant resolves through the registry normalization.
	require.Equal(t, "Mié", weekdayShortLocalized(time.Wednesday, "es_AR"))

	// Out-of-range weekday (effectively unreachable — input is a validated 0-6
	// SQL bucket) returns an obvious sentinel, not a plausible-but-wrong day.
	require.Equal(t, "Day", weekdayShortLocalized(time.Weekday(99), "es"))
	require.Equal(t, "Day", weekdayShortLocalized(time.Weekday(99), "en"))
}

// TestSuggest_LocalizesWeakestDaypartWeekday proves the happy-hour play's
// weakest-window label renders the weekday in the operator's language.
func TestSuggest_LocalizesWeakestDaypartWeekday(t *testing.T) {
	db := newMarketingDB(t)
	g := db.GetGorm()
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)

	// Lunch-heavy Monday, one thin evening bill so the evening window is weakest.
	// 2026-06-22 is a Monday.
	for h := 12; h < 14; h++ {
		ts := time.Date(2026, 6, 22, h, 0, 0, 0, time.UTC)
		require.NoError(t, g.Create(&database.Bill{BusinessID: 1, BillNumber: fmt.Sprintf("ES-%d", h), Status: database.BillStatusPaid, TotalAmount: 5000, CreatedAt: ts}).Error)
	}
	eveTs := time.Date(2026, 6, 22, 18, 0, 0, 0, time.UTC)
	require.NoError(t, g.Create(&database.Bill{BusinessID: 1, BillNumber: "ES-18", Status: database.BillStatusPaid, TotalAmount: 500, CreatedAt: eveTs}).Error)
	// Happy hour requires a live offer so the play does not invent a discount.
	require.NoError(t, g.Create(&database.Offer{
		BusinessID: 1, Name: "After-Work 2x1", DiscountType: "percentage", DiscountValue: 20, IsActive: true,
	}).Error)

	eng := newTestEngine(db, nil, stubMenuEng(menuengineering.Report{Sparse: true}))
	eng.SetClock(func() time.Time { return now })

	got, err := suggest(context.Background(), eng, 1, time.UTC, "es-AR")
	require.NoError(t, err)

	var hh *CampaignSuggestion
	for i := range got {
		if got[i].Play == PlayHappyHour {
			hh = &got[i]
		}
	}
	require.NotNil(t, hh, "happy-hour play should fire for a lunch-skewed business")
	require.True(t, strings.HasPrefix(hh.DaypartKey, "Lun"),
		"weakest-window weekday should be Spanish (Lun), got %q", hh.DaypartKey)
	require.Equal(t, "After-Work 2x1", hh.Metrics["suggested_offer"])
}

func TestSuggest_PausedShortCircuits(t *testing.T) {
	db := newMarketingDB(t)
	signalQueries := 0
	eng := newTestEngine(db, (*analytics.AnalyticsService)(nil), stubMenuEng(menuengineering.Report{
		Dishes: []menuengineering.DishClass{{
			MenuItemID: "m1", MenuItemName: "Carbonara", Quadrant: menuengineering.QuadrantStar, QtySold: 40,
		}},
	}))
	// Settings loader reports paused -> no candidates, paused flag true.
	eng.SetSettingsLoader(func(_ uint) (Settings, error) {
		return Settings{Enabled: false}, nil
	})
	eng.menuEng = func(_ uint, _ string, _ *time.Location) (menuengineering.Report, error) {
		signalQueries++
		return menuengineering.Report{}, nil
	}
	eng.SetHandledLoader(func(_ uint, _ []string, _ time.Time) ([]database.HandledMarketingActivity, error) {
		signalQueries++
		return nil, nil
	})
	batch, err := eng.SuggestionBatch(context.Background(), 1, nil, "en")
	require.NoError(t, err)
	require.True(t, batch.Paused, "paused flag must be set when engine is disabled")
	require.Equal(t, EmptyReasonPaused, batch.EmptyReason)
	require.Empty(t, batch.Suggestions, "no suggestions when paused")
	require.Zero(t, signalQueries, "paused must short-circuit before signal queries")
}

func TestSuggest_FiltersDisabledPlays(t *testing.T) {
	db := newMarketingDB(t)
	eng := newTestEngine(db, (*analytics.AnalyticsService)(nil), stubMenuEng(menuengineering.Report{
		Dishes: []menuengineering.DishClass{{
			MenuItemID: "m1", MenuItemName: "Carbonara", Quadrant: menuengineering.QuadrantStar, QtySold: 40,
		}},
	}))
	eng.SetSettingsLoader(func(_ uint) (Settings, error) {
		return Settings{Enabled: true, DisabledPlays: []string{"featured_dish"}}, nil
	})
	batch, err := eng.SuggestionBatch(context.Background(), 1, nil, "en")
	require.NoError(t, err)
	require.False(t, batch.Paused)
	out := batch.Suggestions
	for _, s := range out {
		require.NotEqual(t, PlayFeaturedDish, s.Play, "disabled play must be filtered out")
	}
}

func TestSuggestionCooldownAndDismissalLifecycle(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	featured := menuengineering.Report{Dishes: []menuengineering.DishClass{{
		MenuItemID: "m1", MenuItemName: "Carbonara", Quadrant: menuengineering.QuadrantStar, QtySold: 40,
	}}}

	tests := []struct {
		name     string
		handled  database.HandledMarketingActivity
		wantCard bool
	}{
		{
			name:    "dismissed stays excluded",
			handled: database.HandledMarketingActivity{SuggestionID: "1:featured_dish:m1", Status: "dismissed"},
		},
		{
			name: "posted is excluded during the 30 day cooldown",
			handled: database.HandledMarketingActivity{
				SuggestionID: "1:featured_dish:m1", Status: "posted", PostedAt: timePtr(now.Add(-30*24*time.Hour + time.Second)),
			},
		},
		{
			name: "posted returns when the 30 day cooldown expires",
			handled: database.HandledMarketingActivity{
				SuggestionID: "1:featured_dish:m1", Status: "posted", PostedAt: timePtr(now.Add(-30 * 24 * time.Hour)),
			},
			wantCard: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newMarketingDB(t)
			eng := newTestEngine(db, nil, stubMenuEng(featured))
			eng.SetClock(func() time.Time { return now })
			eng.SetHandledLoader(func(_ uint, suggestionIDs []string, cutoff time.Time) ([]database.HandledMarketingActivity, error) {
				require.Equal(t, []string{"1:featured_dish:m1"}, suggestionIDs)
				require.Equal(t, now.Add(-30*24*time.Hour), cutoff)
				return []database.HandledMarketingActivity{tt.handled}, nil
			})

			batch, err := eng.SuggestionBatch(context.Background(), 1, time.UTC, "en")
			require.NoError(t, err)
			if tt.wantCard {
				require.Len(t, batch.Suggestions, 1)
				require.Equal(t, EmptyReasonNone, batch.EmptyReason)
				return
			}
			require.Empty(t, batch.Suggestions)
			require.Equal(t, EmptyReasonAllHandled, batch.EmptyReason)
		})
	}
}

func TestSuggestionBatch_EmptyReason(t *testing.T) {
	featured := menuengineering.Report{Dishes: []menuengineering.DishClass{{
		MenuItemID: "m1", MenuItemName: "Carbonara", Quadrant: menuengineering.QuadrantStar, QtySold: 40,
	}}}
	allPlays := []string{"happy_hour", "featured_dish", "move_item", "win_back", "combo_deal", "offer"}

	tests := []struct {
		name     string
		report   menuengineering.Report
		settings Settings
		handled  []database.HandledMarketingActivity
		want     EmptyReason
		wantLen  int
	}{
		{name: "paused", report: featured, settings: Settings{Enabled: false}, want: EmptyReasonPaused},
		{name: "no data", report: menuengineering.Report{Sparse: true}, settings: Settings{Enabled: true}, want: EmptyReasonNoData},
		{name: "no enabled plays", report: featured, settings: Settings{Enabled: true, DisabledPlays: allPlays}, want: EmptyReasonNoEnabledPlays},
		{
			name: "all handled", report: featured, settings: Settings{Enabled: true},
			handled: []database.HandledMarketingActivity{{SuggestionID: "1:featured_dish:m1", Status: "dismissed"}},
			want:    EmptyReasonAllHandled,
		},
		{name: "suggestions exist", report: featured, settings: Settings{Enabled: true}, want: EmptyReasonNone, wantLen: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newMarketingDB(t)
			eng := newTestEngine(db, nil, stubMenuEng(tt.report))
			eng.SetSettingsLoader(func(_ uint) (Settings, error) { return tt.settings, nil })
			eng.SetHandledLoader(func(_ uint, _ []string, _ time.Time) ([]database.HandledMarketingActivity, error) {
				return tt.handled, nil
			})

			batch, err := eng.SuggestionBatch(context.Background(), 1, time.UTC, "en")
			require.NoError(t, err)
			require.Equal(t, tt.want, batch.EmptyReason)
			require.Len(t, batch.Suggestions, tt.wantLen)
		})
	}
}

func TestSuggestionBatch_BackendFailureIsNotEmptySuccess(t *testing.T) {
	db := newMarketingDB(t)
	eng := newTestEngine(db, nil, stubMenuEng(menuengineering.Report{Sparse: true}))
	eng.SetHandledLoader(func(_ uint, _ []string, _ time.Time) ([]database.HandledMarketingActivity, error) {
		return nil, fmt.Errorf("activity unavailable")
	})

	batch, err := eng.SuggestionBatch(context.Background(), 1, time.UTC, "en")
	require.ErrorContains(t, err, "activity unavailable")
	require.Equal(t, SuggestionBatch{}, batch)
}

func TestSuggestionBatch_SignalQueryFailureIsNotNoData(t *testing.T) {
	db := newMarketingDB(t)
	require.NoError(t, db.GetGorm().Migrator().DropTable(&database.Bill{}))
	eng := newTestEngine(db, nil, stubMenuEng(menuengineering.Report{Sparse: true}))

	batch, err := eng.SuggestionBatch(context.Background(), 1, time.UTC, "en")
	require.ErrorContains(t, err, "load weakest daypart signals")
	require.Equal(t, SuggestionBatch{}, batch)
}

func timePtr(v time.Time) *time.Time { return &v }

func seedWeakEveningDaypart(t *testing.T, g *gorm.DB) {
	t.Helper()
	// Wednesday 2026-08-12: lunch-heavy so evening is the weak window.
	for h := 12; h < 14; h++ {
		ts := time.Date(2026, 8, 12, h, 0, 0, 0, time.UTC)
		require.NoError(t, g.Create(&database.Bill{
			BusinessID: 1, BillNumber: fmt.Sprintf("HH-%d", h), Status: database.BillStatusPaid, TotalAmount: 5000, CreatedAt: ts,
		}).Error)
	}
	eveTs := time.Date(2026, 8, 12, 18, 0, 0, 0, time.UTC)
	require.NoError(t, g.Create(&database.Bill{
		BusinessID: 1, BillNumber: "HH-18", Status: database.BillStatusPaid, TotalAmount: 500, CreatedAt: eveTs,
	}).Error)
}

func TestSuggest_HappyHour_OmittedWithoutInWindowOffer(t *testing.T) {
	db := newMarketingDB(t)
	g := db.GetGorm()
	seedWeakEveningDaypart(t, g)
	start, end := 11*60, 15*60
	require.NoError(t, g.Create(&database.Offer{
		BusinessID: 1, Name: "Weekday Lunch 15% Off", DiscountType: "percentage", DiscountValue: 15,
		IsActive: true, WeekdayMask: 62, StartMinute: &start, EndMinute: &end,
	}).Error)

	eng := newTestEngine(db, nil, stubMenuEng(menuengineering.Report{Sparse: true}))
	// 15:14 America/New_York on Wed 2026-08-12 is 19:14 UTC (EDT).
	ny, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	eng.SetClock(func() time.Time {
		return time.Date(2026, 8, 12, 15, 14, 0, 0, ny)
	})

	got, err := suggest(context.Background(), eng, 1, ny, "en")
	require.NoError(t, err)
	for _, s := range got {
		require.NotEqual(t, PlayHappyHour, s.Play, "ended lunch offer must not become a happy-hour deal")
		require.NotEqual(t, "Steak Plate", s.TargetName)
	}
}

func TestSuggest_HappyHour_AttachesLiveOfferNotOOSSteak(t *testing.T) {
	db := newMarketingDB(t)
	g := db.GetGorm()
	seedWeakEveningDaypart(t, g)

	falseAvail := false
	cats := []database.MenuCategory{{
		ID: "c1", Name: "Mains", Items: []database.MenuItem{
			{ID: "steak", Name: "Steak Plate", Image: "https://cdn/steak.jpg", Price: 42, Description: "Grilled ribeye", IsAvailable: falseAvail},
		},
	}}
	blob, err := json.Marshal(cats)
	require.NoError(t, err)
	require.NoError(t, g.Create(&database.Menu{BusinessID: 1, Categories: string(blob), IsActive: true, Version: 1}).Error)
	require.NoError(t, g.Create(&database.Offer{
		BusinessID: 1, Name: "After-Work Special", DiscountType: "percentage", DiscountValue: 20,
		IsActive: true, Image: "https://cdn/hh.jpg",
	}).Error)

	eng := newTestEngine(db, nil, stubMenuEng(menuengineering.Report{Sparse: true}))
	ny, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	eng.SetClock(func() time.Time {
		return time.Date(2026, 8, 12, 18, 0, 0, 0, ny)
	})

	got, err := suggest(context.Background(), eng, 1, ny, "en")
	require.NoError(t, err)
	var hh *CampaignSuggestion
	for i := range got {
		require.NotEqual(t, "Steak Plate", got[i].TargetName, "OOS steak must not be the caption subject")
		require.NotContains(t, strings.ToLower(got[i].TargetName), "steak")
		if got[i].Play == PlayHappyHour {
			hh = &got[i]
		}
	}
	require.NotNil(t, hh, "happy hour with a live in-window offer should ship")
	require.Equal(t, "After-Work Special", hh.TargetName)
	require.Equal(t, "After-Work Special", hh.Metrics["suggested_offer"])
	require.Equal(t, "percentage", hh.DiscountType)
	require.InDelta(t, 20.0, hh.DiscountValue, 0.001)
	require.NotEqual(t, ImageSourceMenu, hh.ImageSource)
}

func TestSuggest_HappyHour_SkipsOOSSteakOfferAndAttachesNext(t *testing.T) {
	db := newMarketingDB(t)
	g := db.GetGorm()
	seedWeakEveningDaypart(t, g)
	cats := []database.MenuCategory{{
		ID: "c1", Name: "Mains", Items: []database.MenuItem{
			{ID: "steak", Name: "Steak Plate", Image: "https://cdn/steak.jpg", Price: 42, IsAvailable: true},
			{ID: "bowl", Name: "Harvest Bowl", Image: "https://cdn/bowl.jpg", Price: 18, IsAvailable: true},
		},
	}}
	blob, err := json.Marshal(cats)
	require.NoError(t, err)
	require.NoError(t, g.Create(&database.Menu{BusinessID: 1, Categories: string(blob), IsActive: true, Version: 1}).Error)
	target := "steak"
	require.NoError(t, g.Create(&database.Offer{
		BusinessID: 1, Name: "Weekday Lunch 15% Off", DiscountType: "percentage", DiscountValue: 15,
		IsActive: true, ApplicableTo: "all", Image: "https://cdn/bowl.jpg",
	}).Error)
	require.NoError(t, g.Create(&database.Offer{
		BusinessID: 1, Name: "$5 Off the Steak Plate", DiscountType: "fixed", DiscountValue: 5,
		IsActive: true, ApplicableTo: "item", TargetID: &target, Image: "https://cdn/steak.jpg",
	}).Error)

	eng := newTestEngine(db, nil, stubMenuEng(menuengineering.Report{Sparse: true}))
	eng.SetUnmakeableLoader(func(uint) (map[string]bool, error) {
		return map[string]bool{"steak": true}, nil
	})
	eng.SetClock(func() time.Time {
		return time.Date(2026, 8, 12, 18, 0, 0, 0, time.UTC)
	})

	batch, err := eng.SuggestionBatch(context.Background(), 1, time.UTC, "en")
	require.NoError(t, err)
	require.Contains(t, batch.InventoryBlocked, "Steak Plate")
	var hh *CampaignSuggestion
	for i := range batch.Suggestions {
		s := batch.Suggestions[i]
		require.NotContains(t, strings.ToLower(s.TargetName), "steak")
		if mid, ok := offerLinkedMenuItemID(&s); ok {
			require.NotEqual(t, "steak", mid)
		}
		if s.Play == PlayHappyHour {
			hh = &batch.Suggestions[i]
		}
		if s.Play == PlayOffer {
			require.NotEqual(t, "https://cdn/bowl.jpg", s.ImageURL,
				"generic lunch offer must not wear the Harvest Bowl photo")
		}
	}
	require.NotNil(t, hh, "happy hour should attach the next live offer, not vanish")
	require.Equal(t, "Weekday Lunch 15% Off", hh.TargetName)
	require.Empty(t, hh.ImageURL, "happy hour must not invent the bowl photo either")
}

func TestSuggest_HappyHour_OmittedWhenOfferTargetsOOSSteak(t *testing.T) {
	db := newMarketingDB(t)
	g := db.GetGorm()
	seedWeakEveningDaypart(t, g)
	cats := []database.MenuCategory{{
		ID: "c1", Name: "Mains", Items: []database.MenuItem{
			{ID: "steak", Name: "Steak Plate", Image: "https://cdn/steak.jpg", Price: 42, IsAvailable: true},
		},
	}}
	blob, err := json.Marshal(cats)
	require.NoError(t, err)
	require.NoError(t, g.Create(&database.Menu{BusinessID: 1, Categories: string(blob), IsActive: true, Version: 1}).Error)
	target := "steak"
	require.NoError(t, g.Create(&database.Offer{
		BusinessID: 1, Name: "$5 Off the Steak Plate", DiscountType: "fixed", DiscountValue: 5,
		IsActive: true, ApplicableTo: "item", TargetID: &target,
	}).Error)

	eng := newTestEngine(db, nil, stubMenuEng(menuengineering.Report{Sparse: true}))
	eng.SetUnmakeableLoader(func(uint) (map[string]bool, error) {
		return map[string]bool{"steak": true}, nil
	})

	got, err := suggest(context.Background(), eng, 1, time.UTC, "en")
	require.NoError(t, err)
	for _, s := range got {
		require.NotEqual(t, PlayHappyHour, s.Play, "happy hour tied to OOS steak must be omitted")
		require.NotContains(t, strings.ToLower(s.TargetName), "steak")
	}
}

func TestSuggest_IDsAreDeterministic(t *testing.T) {
	db := newMarketingDB(t)
	rep := menuengineering.Report{Dishes: []menuengineering.DishClass{{
		MenuItemID: "m1", MenuItemName: "Carbonara", Quadrant: menuengineering.QuadrantStar, QtySold: 40,
	}}}
	eng := newTestEngine(db, (*analytics.AnalyticsService)(nil), stubMenuEng(rep))
	a, err := suggest(context.Background(), eng, 1, nil, "en")
	require.NoError(t, err)
	b, err := suggest(context.Background(), eng, 1, nil, "en")
	require.NoError(t, err)
	require.NotEmpty(t, a)
	require.Equal(t, a[0].ID, b[0].ID, "suggestion ids must be stable across recomputes")
	require.Equal(t, "1:featured_dish:m1", a[0].ID)
}

// S2: unavailable (86'd) menu items must not appear as featured/move plays.
func TestSuggest_SuppressesUnavailableMenuItems(t *testing.T) {
	db := newMarketingDB(t)
	g := db.GetGorm()

	cats := []map[string]any{{
		"id": "c1", "name": "Mains", "items": []map[string]any{
			{"id": "m1", "name": "Carbonara", "image": "https://cdn/c.jpg", "price": 12.99, "is_available": false},
			{"id": "m2", "name": "Risotto", "image": "https://cdn/r.jpg", "price": 18.0, "is_available": true},
		},
	}}
	blob, err := json.Marshal(cats)
	require.NoError(t, err)
	require.NoError(t, g.Create(&database.Menu{BusinessID: 1, Categories: string(blob), IsActive: true, Version: 1}).Error)

	rep := menuengineering.Report{Dishes: []menuengineering.DishClass{
		{MenuItemID: "m1", MenuItemName: "Carbonara", Quadrant: menuengineering.QuadrantStar, QtySold: 50, MarginPerUnit: 9},
		{MenuItemID: "m2", MenuItemName: "Risotto", Quadrant: "puzzle", Action: "promote", MarginPerUnit: 14, QtySold: 4},
	}}
	eng := newTestEngine(db, nil, stubMenuEng(rep))
	got, err := suggest(context.Background(), eng, 1, time.UTC, "en")
	require.NoError(t, err)

	for _, s := range got {
		require.NotEqual(t, "m1", s.TargetItemID, "86'd Carbonara must not be suggested")
	}
	var move *CampaignSuggestion
	for i := range got {
		if got[i].Play == PlayMoveItem {
			move = &got[i]
		}
	}
	require.NotNil(t, move, "available promote item still surfaces")
	require.Equal(t, "m2", move.TargetItemID)
	require.Equal(t, RankingVersionS2, move.RankingVersion)
	require.NotEmpty(t, move.WhyFactors)
}

// Issue #200: zero-stock recipe dishes must not be marketed even when
// is_available remains true on the menu JSON (default warn sync).
func TestSuggest_SuppressesInventoryOutOfStockMenuItems(t *testing.T) {
	db := newMarketingDB(t)
	g := db.GetGorm()
	require.NoError(t, g.AutoMigrate(
		&database.InventorySettings{},
		&database.InventoryItem{},
		&database.InventoryRecipe{},
	))

	cats := []map[string]any{{
		"id": "c1", "name": "Mains", "items": []map[string]any{
			{"id": "steak", "name": "Steak Plate", "image": "https://cdn/s.jpg", "price": 42.0, "is_available": true},
			{"id": "salad", "name": "Garden Salad", "image": "https://cdn/g.jpg", "price": 12.0, "is_available": true},
		},
	}}
	blob, err := json.Marshal(cats)
	require.NoError(t, err)
	require.NoError(t, g.Create(&database.Menu{BusinessID: 1, Categories: string(blob), IsActive: true, Version: 1}).Error)
	require.NoError(t, g.Create(&database.InventorySettings{
		BusinessID: 1, InventoryEnabled: true, AvailabilitySyncMode: database.InventoryAvailabilityModeWarn,
	}).Error)
	beef := database.InventoryItem{
		BusinessID: 1, Name: "Premium Beef", Unit: "kg", CurrentQuantity: 0, ReorderThreshold: 1, IsActive: true,
	}
	require.NoError(t, g.Create(&beef).Error)
	require.NoError(t, g.Create(&database.InventoryRecipe{
		BusinessID: 1, MenuItemID: "steak", MenuItemName: "Steak Plate",
		InventoryItemID: beef.ID, QuantityRequired: 0.3,
	}).Error)

	rep := menuengineering.Report{Dishes: []menuengineering.DishClass{
		{MenuItemID: "steak", MenuItemName: "Steak Plate", Quadrant: menuengineering.QuadrantStar, QtySold: 50, MarginPerUnit: 18},
		{MenuItemID: "salad", MenuItemName: "Garden Salad", Quadrant: "puzzle", Action: "promote", MarginPerUnit: 8, QtySold: 4},
	}}
	eng := NewEngine(db, nil, stubMenuEng(rep))
	batch, err := eng.SuggestionBatch(context.Background(), 1, time.UTC, "en")
	require.NoError(t, err)
	got := batch.Suggestions
	require.Contains(t, batch.InventoryBlocked, "Steak Plate")

	for _, s := range got {
		require.NotEqual(t, "steak", s.TargetItemID, "zero-stock Steak Plate must not be suggested")
		require.NotEqual(t, "Steak Plate", s.TargetName, "zero-stock hero name must not remain")
	}
	var move *CampaignSuggestion
	for i := range got {
		if got[i].Play == PlayMoveItem {
			move = &got[i]
		}
	}
	require.NotNil(t, move, "in-stock promote item still surfaces")
	require.Equal(t, "salad", move.TargetItemID)
}

// Warn-mode recipe OOS keeps menu is_available:true but kitchen cannot make the
// dish — Marketing must match AI Waiter UnrecommendableMenuItemIDs (#200).
// Uses the SetUnmakeableLoader seam plus an item-scoped offer to prove offer
// suppression (complementary to the real-inventory test above).
func TestSuggest_SuppressesUnmakeableStockoutItems(t *testing.T) {
	db := newMarketingDB(t)
	g := db.GetGorm()

	cats := []map[string]any{{
		"id": "c1", "name": "Mains", "items": []map[string]any{
			{"id": "steak", "name": "Steak Plate", "image": "https://cdn/steak.jpg", "price": 42.0, "is_available": true},
			{"id": "bowl", "name": "Harvest Bowl", "image": "https://cdn/bowl.jpg", "price": 18.0, "is_available": true},
		},
	}}
	blob, err := json.Marshal(cats)
	require.NoError(t, err)
	require.NoError(t, g.Create(&database.Menu{BusinessID: 1, Categories: string(blob), IsActive: true, Version: 1}).Error)
	target := "steak"
	require.NoError(t, g.Create(&database.Offer{
		BusinessID: 1, Name: "$5 Off the Steak Plate", DiscountType: "fixed", DiscountValue: 5,
		IsActive: true, ApplicableTo: "item", TargetID: &target,
	}).Error)

	rep := menuengineering.Report{Dishes: []menuengineering.DishClass{
		{MenuItemID: "steak", MenuItemName: "Steak Plate", Quadrant: menuengineering.QuadrantStar, QtySold: 40, MarginPerUnit: 18},
		{MenuItemID: "bowl", MenuItemName: "Harvest Bowl", Quadrant: "puzzle", Action: "promote", MarginPerUnit: 9, QtySold: 3},
	}}
	eng := newTestEngine(db, nil, stubMenuEng(rep))
	eng.SetUnmakeableLoader(func(_ uint) (map[string]bool, error) {
		return map[string]bool{"steak": true}, nil
	})
	batch, err := eng.SuggestionBatch(context.Background(), 1, time.UTC, "en")
	require.NoError(t, err)
	got := batch.Suggestions
	require.Contains(t, batch.InventoryBlocked, "Steak Plate")

	for _, s := range got {
		require.NotEqual(t, "steak", s.TargetItemID, "OOS Steak Plate must not be featured")
		require.NotContains(t, strings.ToLower(s.TargetName), "steak",
			"OOS steak must not appear as a marketing target (play=%s name=%q)", s.Play, s.TargetName)
		if mid, ok := offerLinkedMenuItemID(&s); ok {
			require.NotEqual(t, "steak", mid)
		}
	}
	var move *CampaignSuggestion
	for i := range got {
		if got[i].Play == PlayMoveItem {
			move = &got[i]
		}
	}
	require.NotNil(t, move, "makeable promote item still surfaces")
	require.Equal(t, "bowl", move.TargetItemID)

	rows, total, err := db.ListMarketingActivities(database.MarketingActivityQuery{
		BusinessID: 1, Status: "dismissed", PerPage: 20,
	})
	require.NoError(t, err)
	require.Greater(t, total, int64(0), "inventory-hidden ideas must land in Historial Ocultos")
	var foundSteak bool
	for _, row := range rows {
		if strings.Contains(strings.ToLower(row.TargetName), "steak") ||
			strings.Contains(strings.ToLower(row.Title), "steak") {
			foundSteak = true
			require.Equal(t, "dismissed", row.Status)
			require.Equal(t, "inventory", row.CreatedBy)
		}
	}
	require.True(t, foundSteak, "Steak Plate idea must be recoverable as dismissed: %+v", rows)
}

// Inventory grounding errors must fail closed — never return an empty hide-set
// that would let OOS dishes reappear in suggestions (matches AI Waiter).
func TestSuggest_UnmakeableLoaderErrorFailsClosed(t *testing.T) {
	db := newMarketingDB(t)
	eng := NewEngine(db, nil, stubMenuEng(menuengineering.Report{Sparse: true}))
	eng.SetUnmakeableLoader(func(uint) (map[string]bool, error) {
		return nil, fmt.Errorf("inventory grounding unavailable")
	})
	_, err := suggest(context.Background(), eng, 1, time.UTC, "en")
	require.ErrorContains(t, err, "load unmakeable menu items")
	require.ErrorContains(t, err, "inventory grounding unavailable")
}

// S2: missing is_available on menu JSON must NOT suppress (FE treats absent as available).
func TestSuggest_MissingAvailabilityIsNotUnavailable(t *testing.T) {
	db := newMarketingDB(t)
	g := db.GetGorm()

	// No is_available field at all.
	cats := []map[string]any{{
		"id": "c1", "name": "Mains", "items": []map[string]any{
			{"id": "m1", "name": "Carbonara", "image": "https://cdn/c.jpg", "price": 12.99},
		},
	}}
	blob, err := json.Marshal(cats)
	require.NoError(t, err)
	require.NoError(t, g.Create(&database.Menu{BusinessID: 1, Categories: string(blob), IsActive: true, Version: 1}).Error)

	rep := menuengineering.Report{Dishes: []menuengineering.DishClass{
		{MenuItemID: "m1", MenuItemName: "Carbonara", Quadrant: menuengineering.QuadrantStar, QtySold: 50, MarginPerUnit: 9},
	}}
	eng := newTestEngine(db, nil, stubMenuEng(rep))
	got, err := suggest(context.Background(), eng, 1, time.UTC, "en")
	require.NoError(t, err)
	require.NotEmpty(t, got)
	require.Equal(t, PlayFeaturedDish, got[0].Play)
	require.Equal(t, "m1", got[0].TargetItemID)
	require.True(t, hasWhyFactor(&got[0], "photo_ready"), "menu photo should boost rank")
}

// S2: ranking improvements must not break empty_reason contract or cooldown length.
func TestSuggest_S2RankingPreservesEmptyAndCooldown(t *testing.T) {
	// Mutation guard: cooldown constant still 30 days.
	require.Equal(t, 30*24*time.Hour, postedSuggestionCooldown)

	db := newMarketingDB(t)
	eng := newTestEngine(db, nil, stubMenuEng(menuengineering.Report{Sparse: true}))
	batch, err := eng.SuggestionBatch(context.Background(), 1, time.UTC, "en")
	require.NoError(t, err)
	require.Equal(t, EmptyReasonNoData, batch.EmptyReason)
	require.Empty(t, batch.Suggestions)
}

// suggest returns only the ranked suggestions of the live SuggestionBatch.
func suggest(ctx context.Context, e *Engine, businessID uint, loc *time.Location, locale string) ([]CampaignSuggestion, error) {
	batch, err := e.SuggestionBatch(ctx, businessID, loc, locale)
	return batch.Suggestions, err
}
