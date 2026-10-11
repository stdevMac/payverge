package services

import (
	"encoding/json"
	"fmt"
	"testing"
	"unicode/utf8"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// #870 regression suite. Three distinct defects, one grounding surface:
//
//  1. the Spanish ask 500s, because the sidebar thread title was cut at a BYTE
//     offset and the cut landed inside a multi-byte accented rune;
//  2. the briefing has no 86 board in its grounding at all, so the model
//     confidently claims nothing is sold out while the kitchen is 86'ing beef;
//  3. every money figure is rendered with a bare "$", so an ARS 40.600 floor
//     is read back to an Argentine owner as forty thousand dollars.

// setupDirectorGroundingTestDB migrates exactly the tables buildContext walks
// on this path: business + bills/payments for the metrics, the menu blob for
// the 86 board, and the inventory tables GetInventorySummary reads through
// UnrecommendableMenuItemIDs.
func setupDirectorGroundingTestDB(t *testing.T) *database.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Bill{},
		&database.Payment{},
		&database.AlternativePayment{},
		&database.Order{},
		&database.Menu{},
		&database.InventorySettings{},
		&database.InventoryItem{},
		&database.InventoryRecipe{},
		&database.DeliverySettings{},
		&database.DirectorConsoleThread{},
		&database.DirectorConsoleMessage{},
	))
	return database.GetDBWrapper()
}

func seedDirectorMenu(t *testing.T, db *database.DB, businessID uint, categories []database.MenuCategory) {
	t.Helper()
	blob, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, db.GetGorm().Create(&database.Menu{
		BusinessID: businessID,
		Categories: string(blob),
		IsActive:   true,
	}).Error)
}

// directorSpanishAskWithAccentAtByteLimit is a real Rioplatense morning-briefing
// ask whose 70th BYTE falls inside the "ó" of "recaudación". Byte-slicing it at
// directorThreadTitleRuneLimit yields invalid UTF-8; Postgres then rejects the
// TEXT insert on director_console_threads.title and the whole ask 500s. English
// asks are pure ASCII and never reproduce it — which is why only the ES path
// broke in production.
const directorSpanishAskWithAccentAtByteLimit = "Che, necesito el resumen de la mañana con las ventas, y la recaudación de hoy y qué platos están agotados en la parrilla, por favor."

// TestDirectorThreadTitleKeepsSpanishAsksValidUTF8 is the unit-level proof for
// the ES-only 500. The byte cut at 70 lands mid-accent on this question; the
// rune cut cannot.
func TestDirectorThreadTitleKeepsSpanishAsksValidUTF8(t *testing.T) {
	// Byte 70 of this ask is the SECOND byte of the "ó" in "recaudación", so a
	// byte cut at 70 splits the rune and produces invalid UTF-8. Verified: the
	// fixture is chosen for that property, not by accident.
	question := directorSpanishAskWithAccentAtByteLimit
	require.Greater(t, len(question), directorThreadTitleRuneLimit, "fixture must be long enough to truncate")
	require.False(t, utf8.Valid([]byte(question)[:directorThreadTitleRuneLimit]),
		"fixture must actually split a rune at the byte limit, otherwise it proves nothing")

	title := directorThreadTitle(question, "Parrilla Quebracho Azul")

	assert.True(t, utf8.ValidString(title),
		"a byte-sliced Spanish title is invalid UTF-8 and Postgres rejects it on the TEXT column, 500ing the whole ask")
	assert.LessOrEqual(t, utf8.RuneCountInString(title), directorThreadTitleRuneLimit)
	assert.NotEmpty(t, title)
}

func TestDirectorThreadTitleFallsBackToBusinessName(t *testing.T) {
	assert.Equal(t, "Parrilla Quebracho Azul strategy", directorThreadTitle("   ", "Parrilla Quebracho Azul"))
}

func TestDirectorThreadTitleLeavesShortAsksIntact(t *testing.T) {
	assert.Equal(t, "¿Cuánto vendimos hoy?", directorThreadTitle("  ¿Cuánto vendimos hoy?  ", "Parrilla"))
}

// TestResolveThreadPersistsValidUTF8TitleForSpanishAsk drives the real
// resolveThread → CreateDirectorConsoleThread path. SQLite tolerates invalid
// UTF-8 where Postgres does not, so the assertion (not a driver error) is the
// proof: on the byte-slicing code the persisted title fails utf8.ValidString.
func TestResolveThreadPersistsValidUTF8TitleForSpanishAsk(t *testing.T) {
	db := setupDirectorGroundingTestDB(t)
	service := NewDirectorConsoleService(db, analytics.NewAnalyticsService(db), nil, nil)
	business := database.Business{Name: "Parrilla Quebracho Azul", SettlementAddr: "s-870", TippingAddr: "t-870"}
	require.NoError(t, db.GetGorm().Create(&business).Error)

	question := directorSpanishAskWithAccentAtByteLimit
	thread, reused, err := service.resolveThread(DirectorAskRequest{
		BusinessID: business.ID,
		Message:    question,
		Locale:     "es",
	}, business.Name)
	require.NoError(t, err)
	require.False(t, reused)
	require.NotNil(t, thread)

	assert.True(t, utf8.ValidString(thread.Title), "persisted title must be valid UTF-8: %q", thread.Title)

	var stored database.DirectorConsoleThread
	require.NoError(t, db.GetGorm().First(&stored, thread.ID).Error)
	assert.True(t, utf8.ValidString(stored.Title))
}

func TestBuildDirectorAvailabilityProjectsTheEightySixBoard(t *testing.T) {
	categories := []database.MenuCategory{
		{
			ID:   "cat-parrilla",
			Name: "Parrilla",
			Items: []database.MenuItem{
				{ID: "bife", Name: "Bife de Chorizo", IsAvailable: true, InventoryStatus: "out_of_stock"},
				{ID: "asado", Name: "Asado de Tira", IsAvailable: true},
				{ID: "vacio", Name: "Vacío", IsAvailable: false},
			},
		},
		{
			ID:   "cat-tragos",
			Name: "Tragos",
			Items: []database.MenuItem{
				{ID: "fernet", Name: "Fernet con Coca", IsAvailable: true},
			},
		},
	}
	blocked := map[string]bool{"asado": true}

	board := buildDirectorAvailability(categories, blocked, true)

	assert.True(t, board.Known)
	assert.Equal(t, 4, board.MenuItemCount)
	assert.Equal(t, 3, board.SoldOutCount, "inventory stamp, operator 86 and orderability block all count")
	assert.ElementsMatch(t, []string{"Bife de Chorizo", "Asado de Tira", "Vacío"}, board.SoldOutItems)
	assert.False(t, board.Truncated)

	prose := directorAvailabilityProse(board)
	assert.Contains(t, prose, "Bife de Chorizo")
	assert.Contains(t, prose, "3 of 4")
	assert.NotContains(t, prose, "Fernet con Coca", "an available item must never appear on the 86 board")
}

func TestBuildDirectorAvailabilityCapsNamesAndFlagsTruncation(t *testing.T) {
	items := make([]database.MenuItem, 0, directorSoldOutNameCap+5)
	for i := 0; i < directorSoldOutNameCap+5; i++ {
		items = append(items, database.MenuItem{ID: fmt.Sprintf("i-%d", i), Name: fmt.Sprintf("Plato %d", i)})
	}
	board := buildDirectorAvailability([]database.MenuCategory{{ID: "c", Name: "C", Items: items}}, nil, true)

	assert.Equal(t, directorSoldOutNameCap+5, board.SoldOutCount)
	assert.Len(t, board.SoldOutItems, directorSoldOutNameCap)
	assert.True(t, board.Truncated)
	assert.Contains(t, directorAvailabilityProse(board), "including")
}

func TestDirectorAvailabilityProseDistinguishesEmptyFromUnreadable(t *testing.T) {
	clean := buildDirectorAvailability([]database.MenuCategory{
		{ID: "c", Name: "C", Items: []database.MenuItem{{ID: "a", Name: "Empanadas", IsAvailable: true}}},
	}, map[string]bool{}, true)
	assert.Equal(t, 0, clean.SoldOutCount)
	assert.Contains(t, directorAvailabilityProse(clean), "nothing on the menu is sold out")

	unknown := buildDirectorAvailability(nil, nil, false)
	assert.False(t, unknown.Known)
	assert.Contains(t, directorAvailabilityProse(unknown), "could not be read",
		"an unreadable stock read must not be reported as a clean 86 board")
}

// TestBuildContextGroundsVenueCurrencyAndSoldOutItems is the integration-level
// proof: the real buildContext, on a real ARS business with a real menu blob,
// must ship both the venue currency and the 86 board to the model.
func TestBuildContextGroundsVenueCurrencyAndSoldOutItems(t *testing.T) {
	db := setupDirectorGroundingTestDB(t)
	service := NewDirectorConsoleService(db, analytics.NewAnalyticsService(db), nil, nil)

	business := database.Business{
		Name:            "Parrilla Quebracho Azul",
		SettlementAddr:  "s-870-ctx",
		TippingAddr:     "t-870-ctx",
		DefaultCurrency: "ARS",
		DisplayCurrency: "ARS",
		Timezone:        "America/Argentina/Buenos_Aires",
	}
	require.NoError(t, db.GetGorm().Create(&business).Error)

	seedDirectorMenu(t, db, business.ID, []database.MenuCategory{{
		ID:   "cat-parrilla",
		Name: "Parrilla",
		Items: []database.MenuItem{
			{ID: "bife", Name: "Bife de Chorizo", Price: 18000, IsAvailable: false},
			{ID: "empanadas", Name: "Empanadas de Carne", Price: 4000, IsAvailable: true},
		},
	}})

	ctx, err := service.buildContext(DirectorAskRequest{
		BusinessID: business.ID,
		Message:    "¿Qué tenemos agotado?",
		Locale:     "es",
		ActiveTab:  "overview",
	}, &business)
	require.NoError(t, err)

	assert.Equal(t, "ARS", ctx.Business.Currency, "the venue currency must reach the model")
	assert.Contains(t, ctx.MetricsSummary, "ARS", "money prose must carry the venue currency, never a bare $")
	assert.NotContains(t, ctx.MetricsSummary, "$", "a bare $ on an ARS venue is a wrong number to the owner")
	assert.Contains(t, ctx.MetricsSummary, "Collected today")

	require.True(t, ctx.Availability.Known, "stock is readable for this venue")
	assert.Equal(t, 2, ctx.Availability.MenuItemCount)
	assert.Equal(t, 1, ctx.Availability.SoldOutCount)
	assert.Equal(t, []string{"Bife de Chorizo"}, ctx.Availability.SoldOutItems)
	assert.Contains(t, ctx.AvailabilitySummary, "Bife de Chorizo")

	// The snapshot the model actually sees is the marshalled payload.
	blob, err := json.Marshal(ctx)
	require.NoError(t, err)
	assert.Contains(t, string(blob), "Bife de Chorizo")
	assert.Contains(t, string(blob), `"currency":"ARS"`)
}

func TestBuildContextDefaultsCurrencyToUSDWhenUnset(t *testing.T) {
	db := setupDirectorGroundingTestDB(t)
	service := NewDirectorConsoleService(db, analytics.NewAnalyticsService(db), nil, nil)

	business := database.Business{Name: "Demo Diner", SettlementAddr: "s-870-usd", TippingAddr: "t-870-usd"}
	require.NoError(t, db.GetGorm().Create(&business).Error)

	ctx, err := service.buildContext(DirectorAskRequest{
		BusinessID: business.ID,
		Message:    "How did we do today?",
		Locale:     "en",
	}, &business)
	require.NoError(t, err)

	assert.Equal(t, "USD", ctx.Business.Currency)
	assert.Contains(t, ctx.MetricsSummary, "USD")
}

// TestBuildOverviewSalesResponseRendersVenueCurrency covers the deterministic
// answer the operator reads verbatim — the one that showed "$40600.00" for an
// ARS 40.600 floor.
func TestBuildOverviewSalesResponseRendersVenueCurrency(t *testing.T) {
	service := &DirectorConsoleService{}
	payload := &directorContext{}
	payload.Business.Currency = "ARS"
	payload.Metrics.TodayCollected = 40600
	payload.Metrics.TodayFloorRemaining = 12000
	payload.Metrics.ActiveBills = 3

	for _, locale := range []string{"es", "es-AR", "en"} {
		t.Run(locale, func(t *testing.T) {
			resp := service.buildOverviewSalesResponse(142, locale, payload)
			assert.Contains(t, resp.Summary, "ARS 40600.00")
			assert.NotContains(t, resp.Summary, "$")
			for _, ev := range resp.Evidence {
				assert.NotContains(t, ev, "$", "evidence line must not claim dollars on an ARS venue: %q", ev)
			}
			assert.NotContains(t, resp.ExpectedImpact, "$")
		})
	}
}

func TestBuildOverviewSalesResponseFallsBackToUSD(t *testing.T) {
	service := &DirectorConsoleService{}
	payload := &directorContext{}
	payload.Metrics.TodayCollected = 120.5

	resp := service.buildOverviewSalesResponse(2, "en", payload)
	assert.Contains(t, resp.Summary, "USD 120.50")
}
