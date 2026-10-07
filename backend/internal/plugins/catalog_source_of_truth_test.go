package plugins_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/plugins"
	"github.com/stdevmac/payverge/backend/internal/services"

	// Blank-import every code-backed plugin so their init() registers with the
	// GlobalRegistry. This is exactly the set main.go blank-imports.
	_ "github.com/stdevmac/payverge/backend/internal/plugins/mercadopago"
	_ "github.com/stdevmac/payverge/backend/internal/plugins/paypal"
	_ "github.com/stdevmac/payverge/backend/internal/plugins/stripe"
	_ "github.com/stdevmac/payverge/backend/internal/plugins/telegram"
	_ "github.com/stdevmac/payverge/backend/internal/plugins/trustpilot"
)

// TestSeedAndRegistryPluginNamesDisjoint enforces the single-source-of-truth
// rule: a plugin's catalog row is owned by EITHER the Go registry (code-backed
// plugins, written by SyncPluginsToDatabase) OR the seed in
// services.InitializeDefaultPlugins — never both. Seeding a code-backed plugin
// creates a dead row that silently drifts from the Go definition.
func TestSeedAndRegistryPluginNamesDisjoint(t *testing.T) {
	previousDB := database.GetDB()
	t.Cleanup(func() { database.SetTestDB(previousDB) })

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}, &database.PluginTranslation{}))
	database.SetTestDB(db)

	// Populate the registry from the blank-imported plugin init()s.
	plugins.InitializeAllPlugins(services.NewPluginService(database.GetDBWrapper()))

	registered := plugins.GlobalRegistry.GetAllPlugins()
	require.NotEmpty(t, registered, "expected code-backed plugins to register")

	seedNames := make(map[string]struct{})
	for _, name := range services.DefaultSeedPluginNames() {
		seedNames[name] = struct{}{}
	}

	for name := range registered {
		if _, dup := seedNames[name]; dup {
			t.Errorf("plugin %q exists in BOTH the Go registry and the seed catalog; "+
				"code-backed plugins must not be seeded (single source of truth)", name)
		}
	}
}
