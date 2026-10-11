package server

import "testing"

// The direct-create path must apply the same country→seed logic as the webhook.
func TestApplyCreateSeed_DerivesFromCountry(t *testing.T) {
	cur, tz, bt, counter := applyCreateSeed("AR", "", "", "cafe", false)
	if cur != "ARS" {
		t.Errorf("currency got %q want ARS", cur)
	}
	if tz != "America/Argentina/Buenos_Aires" {
		t.Errorf("timezone got %q want AR zone", tz)
	}
	if bt != "cafe" {
		t.Errorf("type got %q want cafe", bt)
	}
	if !counter {
		t.Errorf("cafe should enable counter")
	}
}

func TestApplyCreateSeed_ExplicitWins(t *testing.T) {
	cur, tz, bt, counter := applyCreateSeed("AR", "USD", "America/New_York", "bar", false)
	if cur != "USD" || tz != "America/New_York" || bt != "bar" || counter {
		t.Errorf("explicit values should win, got %q %q %q %v", cur, tz, bt, counter)
	}
}

func TestApplyCreateSeed_UnknownCountryAndType(t *testing.T) {
	cur, tz, bt, counter := applyCreateSeed("ZZ", "", "", "not-a-type", false)
	if cur != "USD" || tz != "UTC" || bt != "other" || counter {
		t.Errorf("unknown should fall back to USD/UTC/other, got %q %q %q %v", cur, tz, bt, counter)
	}
}
