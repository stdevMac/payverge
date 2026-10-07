package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setLoyaltyRateVenueCurrency rewrites the seeded venue's currency columns and
// drops the public-guest context cache so the next handler call reads them.
func setLoyaltyRateVenueCurrency(t testing.TB, businessID uint, display, fallback string) {
	t.Helper()
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", businessID).
		Updates(map[string]any{
			"display_currency": display,
			"default_currency": fallback,
		}).Error)
	services.ResetPricingCache()
}

// TestGetLoyaltyRateReportsVenueCurrency is the #895 gate. The redemption rate
// is expressed in whole units of the VENUE's currency (loyalty earn and burn
// both run on business-currency amounts; nothing converts to USD), but the
// public payload only carried the legacy points_per_dollar key, so an ARS carta
// answered {"points_per_dollar":1} and every reader had to assume dollars.
// The response must name the currency the rate is denominated in.
func TestGetLoyaltyRateReportsVenueCurrency(t *testing.T) {
	setupLoyaltyRateAccessDB(t, nil)
	const tableCode = "RATEARS"
	business := seedLoyaltyRateVenue(t, tableCode, 1)
	setLoyaltyRateVenueCurrency(t, business.ID, "ARS", "ARS")

	w := invokeGetLoyaltyRate(tableCode)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var payload map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))

	assert.Equal(t, "ARS", payload["currency"],
		"loyalty rate must name the venue currency, not leave the guest to assume USD")
	assert.Equal(t, 1.0, payload["points_per_currency_unit"],
		"currency-neutral rate key must carry the redemption rate")
	assert.NotContains(t, payload, "points_per_dollar",
		"the dollar-named alias is retired; the rate is per venue currency unit")
}

// TestGetLoyaltyRateFallsBackToDefaultCurrency covers the venue that never set
// a display currency: the reported currency comes from default_currency, and a
// venue with neither still reports USD rather than an empty string.
func TestGetLoyaltyRateFallsBackToDefaultCurrency(t *testing.T) {
	setupLoyaltyRateAccessDB(t, nil)
	const tableCode = "RATEEUR"
	business := seedLoyaltyRateVenue(t, tableCode, 20)
	setLoyaltyRateVenueCurrency(t, business.ID, "", "EUR")

	w := invokeGetLoyaltyRate(tableCode)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var payload map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.Equal(t, "EUR", payload["currency"])
	assert.Equal(t, 20.0, payload["points_per_currency_unit"])
}

func TestGetLoyaltyRateDefaultsToUSDWhenVenueCurrencyUnset(t *testing.T) {
	setupLoyaltyRateAccessDB(t, nil)
	const tableCode = "RATENONE"
	business := seedLoyaltyRateVenue(t, tableCode, 100)
	setLoyaltyRateVenueCurrency(t, business.ID, "", "")

	w := invokeGetLoyaltyRate(tableCode)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var payload map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.Equal(t, "USD", payload["currency"])
	assert.Equal(t, 100.0, payload["points_per_currency_unit"])
}
