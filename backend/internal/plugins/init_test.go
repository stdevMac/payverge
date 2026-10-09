package plugins

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/stretchr/testify/require"
)

// A panic in one plugin initializer must not crash boot or skip the remaining
// initializers — each is isolated by a recover.
func TestRunPluginInitializerRecoversAndContinues(t *testing.T) {
	require.NotPanics(t, func() {
		runPluginInitializer("boom", func(*services.PluginService) { panic("initializer exploded") }, nil)
	})

	ran := false
	runPluginInitializer("ok", func(*services.PluginService) { ran = true }, nil)
	require.True(t, ran, "a later initializer must still run after an earlier one panics")
}

func TestOrderedPluginInitializersSortsByPluginName(t *testing.T) {
	original := pluginInitializers
	defer func() { pluginInitializers = original }()

	pluginInitializers = []namedPluginInitializer{
		{name: "telegram"},
		{name: "mercadopago"},
		{name: "paypal"},
		{name: "stripe"},
	}

	ordered := orderedPluginInitializers()

	require.Equal(t, []string{"mercadopago", "paypal", "stripe", "telegram"}, []string{
		ordered[0].name,
		ordered[1].name,
		ordered[2].name,
		ordered[3].name,
	})
	require.Equal(t, "telegram", pluginInitializers[0].name, "source order should not be mutated")
}
