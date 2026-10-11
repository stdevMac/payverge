package plugins

import (
	"github.com/stdevmac/payverge/backend/internal/services"
	"log"
	"sort"
)

// PluginInitializer is a function type for plugin registration
type PluginInitializer func(*services.PluginService)

type namedPluginInitializer struct {
	name        string
	initializer PluginInitializer
}

// pluginInitializers holds all plugin initialization functions
var pluginInitializers []namedPluginInitializer

// RegisterPluginInitializer registers a plugin initialization function
// This is called by individual plugin packages in their init() functions
func RegisterPluginInitializer(name string, initializer PluginInitializer) {
	pluginInitializers = append(pluginInitializers, namedPluginInitializer{
		name:        name,
		initializer: initializer,
	})
}

func orderedPluginInitializers() []namedPluginInitializer {
	ordered := append([]namedPluginInitializer(nil), pluginInitializers...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].name < ordered[j].name
	})
	return ordered
}

// runPluginInitializer runs a single plugin initializer with panic recovery so
// one misbehaving plugin cannot crash boot or skip the initializers ordered
// after it. A failed initializer simply leaves its plugin unregistered.
func runPluginInitializer(name string, initializer PluginInitializer, pluginService *services.PluginService) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Plugin initializer %q panicked, skipping it: %v", name, r)
		}
	}()
	initializer(pluginService)
}

// InitializeAllPlugins calls all registered plugin initializers
func InitializeAllPlugins(pluginService *services.PluginService) {
	log.Println("Initializing all plugins...")

	ordered := orderedPluginInitializers()
	for i, initializer := range ordered {
		log.Printf("Initializing plugin %d/%d: %s", i+1, len(ordered), initializer.name)
		runPluginInitializer(initializer.name, initializer.initializer, pluginService)
	}

	log.Printf("Successfully registered %d plugins in registry", len(GlobalRegistry.GetAllPlugins()))

	// Sync registry plugins to database
	if err := SyncPluginsToDatabase(); err != nil {
		log.Printf("Failed to sync plugins to database: %v", err)
	}
}
