package services

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func setupDefaultPluginSeedTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	previousDB := database.GetDB()
	t.Cleanup(func() {
		database.SetTestDB(previousDB)
	})

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.PluginTranslation{}))
	database.SetTestDB(db)

	return db
}

// The open-source catalog does not advertise unbuilt integrations: the
// QuickBooks and Tax Reports "coming soon" placeholders are not seeded.
func TestInitializeDefaultPluginsSeedsNoRoadmapPlaceholders(t *testing.T) {
	setupDefaultPluginSeedTestDB(t)

	require.NoError(t, InitializeDefaultPlugins())

	for _, name := range []string{"quickbooks", "tax_reports", "loyalty"} {
		_, err := database.GetPluginByName(name)
		require.Error(t, err, "plugin %q must not be seeded", name)
	}
	for _, name := range DefaultSeedPluginNames() {
		require.NotContains(t, []string{"quickbooks", "tax_reports"}, name)
	}
}

// Review M1 (oss/c1): while guests cannot settle through the cross-chain rail,
// the catalog must not offer it to operators as if it worked. The seed marks
// it coming-soon (EnableBusinessPlugin refuses coming-soon plugins and the
// operator UI renders them disabled), switches off subscriptions that were
// enabled while it was advertised, and says why in every catalog language.
func TestInitializeDefaultPluginsMarksUnavailableCrossChainRailComingSoon(t *testing.T) {
	if CrossChainGuestSettlementAvailable {
		t.Skip("cross-chain guest settlement is available; the rail is seeded live")
	}
	setupDefaultPluginSeedTestDB(t)

	// A row and subscriptions left over from when the rail was advertised.
	liveCrossChain, err := database.CreatePlugin(database.Plugin{
		Name:         PluginNameCrossChainPayment,
		DisplayName:  "Any Token Payment",
		Description:  "Optional: accept other crypto tokens and convert them to USDC for settlement",
		Message:      "Advanced / selective crypto rail. Not the primary way restaurants get paid.",
		Category:     database.PluginCategoryPayment,
		Version:      "1.0.0",
		Features:     `["Converts to USDC"]`,
		ConfigSchema: `{}`,
		IsActive:     true,
		ComingSoon:   false,
	})
	require.NoError(t, err)
	usdc, err := database.CreatePlugin(database.Plugin{
		Name:         PluginNameUSDCPayment,
		DisplayName:  "USDC Payment",
		Category:     database.PluginCategoryPayment,
		Version:      "1.0.0",
		Features:     `[]`,
		ConfigSchema: `{}`,
		IsActive:     true,
	})
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&database.BusinessPlugin{
		BusinessID: 41, PluginID: liveCrossChain.ID, IsEnabled: true, Config: `{"enabled":true}`,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessPlugin{
		BusinessID: 41, PluginID: usdc.ID, IsEnabled: true, Config: `{"enabled":true}`,
	}).Error)

	require.NoError(t, InitializeDefaultPlugins())

	crossChain, err := database.GetPluginByName(PluginNameCrossChainPayment)
	require.NoError(t, err)
	require.True(t, crossChain.ComingSoon, "cross-chain must be coming-soon while guests cannot settle through it")
	require.Contains(t, crossChain.Message, "Not available yet")

	var crossChainSub database.BusinessPlugin
	require.NoError(t, database.GetDB().Where("plugin_id = ?", crossChain.ID).First(&crossChainSub).Error)
	require.False(t, crossChainSub.IsEnabled, "a subscription enabled while the rail was advertised must be switched off")

	var usdcSub database.BusinessPlugin
	require.NoError(t, database.GetDB().Where("plugin_id = ?", usdc.ID).First(&usdcSub).Error)
	require.True(t, usdcSub.IsEnabled, "the payer-bound USDC rail stays as the operator left it")

	for _, lang := range []string{"es", "es-AR"} {
		var translation database.PluginTranslation
		require.NoError(t, database.GetDB().
			Where("plugin_id = ? AND language_code = ? AND field_name = ?", crossChain.ID, lang, "message").
			First(&translation).Error)
		require.Contains(t, translation.Content, "Todavía no disponible", "%s catalog message must say the rail is unavailable", lang)
	}
}
