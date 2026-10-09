// Package geodefaults derives sensible locale defaults (currency + IANA
// timezone) from an ISO-3166 alpha-2 country code. It is a pure lookup with no
// external dependencies so it can be reused by every business-creation path and
// mirrored on the frontend. Currencies are constrained to the set the platform
// supports (see services/exchange_rate.go default seed); anything unknown falls
// back to {USD, UTC}, which is the historical default and therefore introduces
// no behavior change for unrecognized input.
package geodefaults

import "strings"

// Defaults is the locale seed derived from a country.
type Defaults struct {
	Currency string
	Timezone string
}

// countryTable maps upper-case ISO-3166 alpha-2 codes to their primary currency
// and most-populous IANA timezone. Multi-zone countries use a single
// representative zone; the owner can refine it in Settings.
var countryTable = map[string]Defaults{
	"US": {"USD", "America/New_York"},
	"CA": {"CAD", "America/Toronto"},
	"GB": {"GBP", "Europe/London"},
	"IE": {"EUR", "Europe/Dublin"},
	"AR": {"ARS", "America/Argentina/Buenos_Aires"},
	"BR": {"BRL", "America/Sao_Paulo"},
	"MX": {"MXN", "America/Mexico_City"},
	"ES": {"EUR", "Europe/Madrid"},
	"FR": {"EUR", "Europe/Paris"},
	"DE": {"EUR", "Europe/Berlin"},
	"IT": {"EUR", "Europe/Rome"},
	"PT": {"EUR", "Europe/Lisbon"},
	"NL": {"EUR", "Europe/Amsterdam"},
	"AT": {"EUR", "Europe/Vienna"},
	"BE": {"EUR", "Europe/Brussels"},
	"GR": {"EUR", "Europe/Athens"},
	"AU": {"AUD", "Australia/Sydney"},
	"CH": {"CHF", "Europe/Zurich"},
	"CN": {"CNY", "Asia/Shanghai"},
	"AE": {"AED", "Asia/Dubai"},
	"IN": {"INR", "Asia/Kolkata"},
	"JP": {"JPY", "Asia/Tokyo"},
	"KR": {"KRW", "Asia/Seoul"},
	"SG": {"SGD", "Asia/Singapore"},
	"HK": {"HKD", "Asia/Hong_Kong"},
	"NO": {"NOK", "Europe/Oslo"},
	"SE": {"SEK", "Europe/Stockholm"},
	"DK": {"DKK", "Europe/Copenhagen"},
	"PL": {"PLN", "Europe/Warsaw"},
	"CL": {"CLP", "America/Santiago"},
	"CO": {"COP", "America/Bogota"},
	"PE": {"PEN", "America/Lima"},
	"UY": {"UYU", "America/Montevideo"},
	"PY": {"PYG", "America/Asuncion"},
	"BO": {"BOB", "America/La_Paz"},
	"EC": {"USD", "America/Guayaquil"},
	"CR": {"CRC", "America/Costa_Rica"},
	"PA": {"USD", "America/Panama"},
	"DO": {"DOP", "America/Santo_Domingo"},
}

// fallback is returned for unknown or empty country codes. It matches the
// historical defaults so unrecognized input changes nothing.
var fallback = Defaults{Currency: "USD", Timezone: "UTC"}

// CountryDefaults returns the currency + timezone seed for a country code. Input
// is trimmed and upper-cased; unknown codes return {USD, UTC}.
func CountryDefaults(countryCode string) Defaults {
	code := strings.ToUpper(strings.TrimSpace(countryCode))
	if d, ok := countryTable[code]; ok {
		return d
	}
	return fallback
}
