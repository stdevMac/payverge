package services

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestGetDeliverySettingsDTO_DefaultsAreDollars verifies the seeded default
// delivery settings (returned when a business has no delivery_settings row)
// surface as $5.00 fee / $30.00 free-delivery threshold — not $0.05 / $0.30.
// The underlying fields are int64 CENTS, so the defaults must be 500/3000;
// untyped float literals (5.0/30.0) truncate to 5/30 cents.
func TestGetDeliverySettingsDTO_DefaultsAreDollars(t *testing.T) {
	db := setupDeliveryPerfTestDB(t, nil)
	require.NoError(t, db.AutoMigrate(&database.DeliverySettings{}))

	svc := NewDeliveryService(db, nil)
	dto, err := svc.GetDeliverySettingsDTO(4242, false) // business with no settings row
	require.NoError(t, err)
	require.InDelta(t, 5.0, dto.FlatDeliveryFee, 0.001, "default flat delivery fee must be $5.00")
	require.InDelta(t, 30.0, dto.FreeDeliveryMinimum, 0.001, "default free-delivery minimum must be $30.00")
}
