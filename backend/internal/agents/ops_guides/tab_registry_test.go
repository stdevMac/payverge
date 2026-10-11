package ops_guides

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTabRegistry_HasExact23CanonicalKeys(t *testing.T) {
	require.NoError(t, RegistryLoadError())
	require.Len(t, CanonicalTabKeys, 23)
	tabs := AllTabs()
	require.Len(t, tabs, 23)

	want := []string{
		"overview", "bills", "cash-register", "printers", "kitchen", "reservations",
		"menu", "tables", "ai-waiter", "director-console", "marketing", "analytics",
		"crm", "delivery", "counter", "inventory", "staff", "schedule",
		"business-page", "accounting", "fiscal", "plugins", "settings",
	}
	got := make([]string, 0, len(tabs))
	for _, r := range tabs {
		got = append(got, r.Key)
		assert.NotEmpty(t, r.RequiredPermission)
		assert.Contains(t, []string{"dashboard_tab", "standalone"}, r.RouteKind)
	}
	assert.Equal(t, want, got)

	// Printers are standalone settings routes, not a dashboard tab route.
	printers, ok := LookupTab("printers")
	require.True(t, ok)
	assert.Equal(t, "standalone", printers.RouteKind)
}
