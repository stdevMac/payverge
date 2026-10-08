package services

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/analytics"

	"github.com/stretchr/testify/assert"
)

func TestFormatWeeklyTopItems_Spanish(t *testing.T) {
	items := []analytics.ItemStats{{ItemName: "Empanada", TotalSold: 12}}
	got := formatWeeklyTopItems(items, "es-AR")
	assert.Contains(t, got, "vendidos")
	assert.Contains(t, got, "Empanada")
}

func TestFormatWeeklyTopItems_EmptyEnglish(t *testing.T) {
	got := formatWeeklyTopItems(nil, "en")
	assert.Equal(t, "No items sold this week", got)
}

func TestFormatWeeklyTopItems_EmptySpanish(t *testing.T) {
	got := formatWeeklyTopItems(nil, "es")
	assert.Contains(t, got, "No se vendieron")
}
