package geodefaults

import "testing"

func TestCountryDefaults(t *testing.T) {
	cases := []struct {
		name     string
		code     string
		currency string
		timezone string
	}{
		{"argentina", "AR", "ARS", "America/Argentina/Buenos_Aires"},
		{"us", "US", "USD", "America/New_York"},
		{"brazil lowercase", "br", "BRL", "America/Sao_Paulo"},
		{"spain euro", "ES", "EUR", "Europe/Madrid"},
		{"uae", "AE", "AED", "Asia/Dubai"},
		{"chile", "CL", "CLP", "America/Santiago"},
		{"colombia", "CO", "COP", "America/Bogota"},
		{"peru", "PE", "PEN", "America/Lima"},
		{"uruguay", "UY", "UYU", "America/Montevideo"},
		{"paraguay zero-decimal", "PY", "PYG", "America/Asuncion"},
		{"bolivia", "BO", "BOB", "America/La_Paz"},
		{"ecuador dollarized", "EC", "USD", "America/Guayaquil"},
		{"costa rica", "CR", "CRC", "America/Costa_Rica"},
		{"panama dollarized", "PA", "USD", "America/Panama"},
		{"dominican republic", "DO", "DOP", "America/Santo_Domingo"},
		{"trim + case", "  mx ", "MXN", "America/Mexico_City"},
		{"unknown country", "ZZ", "USD", "UTC"},
		{"empty", "", "USD", "UTC"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CountryDefaults(tc.code)
			if got.Currency != tc.currency {
				t.Errorf("currency: got %q want %q", got.Currency, tc.currency)
			}
			if got.Timezone != tc.timezone {
				t.Errorf("timezone: got %q want %q", got.Timezone, tc.timezone)
			}
		})
	}
}

// Guards the spec invariant: every emitted currency is one the platform supports
// (matches the default seed in services/exchange_rate.go).
func TestCountryDefaultsCurrencyIsSupported(t *testing.T) {
	supported := map[string]bool{
		"USD": true, "EUR": true, "GBP": true, "JPY": true, "AUD": true,
		"CAD": true, "CHF": true, "CNY": true, "ARS": true, "AED": true,
		"BRL": true, "MXN": true, "INR": true, "KRW": true, "SGD": true,
		"HKD": true, "NOK": true, "SEK": true, "DKK": true, "PLN": true,
		"CLP": true, "COP": true, "PEN": true, "UYU": true,
		"PYG": true, "BOB": true, "CRC": true, "DOP": true,
	}
	for code := range countryTable {
		if got := CountryDefaults(code); !supported[got.Currency] {
			t.Errorf("country %q maps to unsupported currency %q", code, got.Currency)
		}
	}
}
