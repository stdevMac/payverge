package trustpilot

import "testing"

// A plain HasSuffix(host, "trustpilot.com") would accept lookalike domains. The
// host check must only accept trustpilot.com itself or a true subdomain of it.
func TestIsTrustpilotHostRejectsLookalikes(t *testing.T) {
	valid := []string{
		"trustpilot.com",
		"www.trustpilot.com",
		"WWW.TRUSTPILOT.COM",
		"uk.trustpilot.com",
		" trustpilot.com ",
	}
	for _, h := range valid {
		if !isTrustpilotHost(h) {
			t.Errorf("expected %q to be accepted as a Trustpilot host", h)
		}
	}

	invalid := []string{
		"evil-trustpilot.com",
		"nottrustpilot.com",
		"trustpilot.com.evil.com",
		"trustpilot.evil.com",
		"faketrustpilot.com",
		"example.com",
		"",
	}
	for _, h := range invalid {
		if isTrustpilotHost(h) {
			t.Errorf("expected %q to be REJECTED as a Trustpilot host", h)
		}
	}
}

func TestValidateConfigRejectsLookalikeTrustpilotURL(t *testing.T) {
	tp := &TrustpilotPlugin{}

	bad := map[string]interface{}{
		"business_name":  "Acme",
		"trustpilot_url": "https://evil-trustpilot.com/review/acme.com",
	}
	if err := tp.ValidateConfig(bad); err == nil {
		t.Fatal("expected lookalike trustpilot_url to be rejected")
	}

	good := map[string]interface{}{
		"business_name":  "Acme",
		"trustpilot_url": "https://www.trustpilot.com/review/acme.com",
	}
	if err := tp.ValidateConfig(good); err != nil {
		t.Fatalf("expected genuine trustpilot_url to be accepted, got: %v", err)
	}
}
