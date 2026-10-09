package demo

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stretchr/testify/require"
)

func TestDemoLocale_SpanishVariants(t *testing.T) {
	require.Equal(t, "es", demoLocale(&database.Business{DefaultLanguage: "es"}))
	require.Equal(t, "es", demoLocale(&database.Business{DefaultLanguage: "es-AR"}))
	require.Equal(t, "es", demoLocale(&database.Business{SourceLanguage: "es-MX"}))
	require.Equal(t, "en", demoLocale(&database.Business{DefaultLanguage: "en"}))
	require.Equal(t, "en", demoLocale(nil))
}

func TestDirectorDemoSeed_es(t *testing.T) {
	es := directorDemoSeed("es")
	require.Equal(t, "es", es.Locale)
	require.Contains(t, es.ThreadTitle, "Director")
	require.NotContains(t, es.Content, "Dinner revenue")
	require.Contains(t, es.Content, "ingresos")
}

func TestInventoryCategoryLabel_es(t *testing.T) {
	require.Equal(t, "Verduras", inventoryCategoryLabel("es", "Produce"))
	require.Equal(t, "Proteína", inventoryCategoryLabel("es", "Protein"))
	require.Equal(t, "Bebidas", inventoryCategoryLabel("es", "Beverage"))
	require.Equal(t, "Produce", inventoryCategoryLabel("en", "Produce"))
}
