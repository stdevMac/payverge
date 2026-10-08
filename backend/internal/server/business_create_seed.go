package server

import (
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/geodefaults"
)

// serverValidBusinessTypes is the canonical enum (kept package-local to avoid an
// internal/server → internal/services import cycle). Unknown input → "other".
var serverValidBusinessTypes = map[string]bool{
	"restaurant": true, "cafe": true, "bar": true, "quick_service": true,
	"food_truck": true, "bakery": true, "fine_dining": true, "other": true,
}

// serverCounterStyleTypes default CounterEnabled=true at creation.
var serverCounterStyleTypes = map[string]bool{
	"cafe": true, "quick_service": true, "food_truck": true, "bakery": true,
}

// normalizeBusinessType lower-cases/trims and coerces unrecognized input to "other".
func normalizeBusinessType(raw string) string {
	t := strings.ToLower(strings.TrimSpace(raw))
	if serverValidBusinessTypes[t] {
		return t
	}
	return "other"
}

// applyCreateSeed resolves the locale + type + counter seed for a directly
// created business. Resolution order: explicit → country default → {USD, UTC}.
// An explicit timezone that does not parse falls back to the country default.
func applyCreateSeed(country, explicitCurrency, explicitTimezone, rawType string, explicitCounter bool) (currency, timezone, businessType string, counterEnabled bool) {
	def := geodefaults.CountryDefaults(country)

	currency = strings.TrimSpace(explicitCurrency)
	if currency == "" {
		currency = def.Currency
	}
	timezone = strings.TrimSpace(explicitTimezone)
	if timezone == "" {
		timezone = def.Timezone
	} else if _, err := time.LoadLocation(timezone); err != nil {
		timezone = def.Timezone
	}
	businessType = normalizeBusinessType(rawType)
	counterEnabled = explicitCounter || serverCounterStyleTypes[businessType]
	return currency, timezone, businessType, counterEnabled
}
