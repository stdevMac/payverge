package plugins

import (
	"log"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// InitializePluginRegistry registers all available plugins
func InitializePluginRegistry(pluginService *services.PluginService) {
	// Use the new initialization system from init.go
	InitializeAllPlugins(pluginService)
}

// SyncPluginsToDatabase ensures all registered plugins exist in the database
func SyncPluginsToDatabase() error {
	log.Println("Syncing plugins to database...")

	for name, plugin := range GlobalRegistry.GetAllPlugins() {
		// Check if plugin exists in database
		existingPlugin, err := database.GetPluginByName(name)
		if err != nil {
			// Plugin doesn't exist, create it
			dbPlugin := GlobalRegistry.ConvertToDBPlugin(plugin)
			_, err := database.CreatePlugin(dbPlugin)
			if err != nil {
				log.Printf("Failed to create plugin %s: %v", name, err)
				continue
			}
			log.Printf("Created plugin: %s", plugin.GetDisplayName())
		} else {
			// Plugin exists, update it if needed
			dbPlugin := GlobalRegistry.ConvertToDBPlugin(plugin)
			dbPlugin.ID = existingPlugin.ID // Preserve the ID

			// UpdatePluginForSync (not UpdatePlugin) so is_active=false actually
			// propagates — an env-disabled plugin must be able to deactivate its
			// catalog row instead of staying purchasable.
			_, err := database.UpdatePluginForSync(existingPlugin.ID, dbPlugin)
			if err != nil {
				log.Printf("Failed to update plugin %s: %v", name, err)
				continue
			}
			log.Printf("Updated plugin: %s", plugin.GetDisplayName())
		}
	}

	log.Println("Plugin sync completed")
	return nil
}

// GetPluginByName returns a plugin instance by name
func GetPluginByName(name string) (Plugin, bool) {
	return GlobalRegistry.GetPlugin(name)
}
