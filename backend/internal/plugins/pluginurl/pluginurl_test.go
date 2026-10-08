package pluginurl

import "testing"

func TestValidateBaseURL_RejectsSSRFVectors(t *testing.T) {
	allow := []string{"paypal.com"}
	bad := []string{
		"",                               // empty
		"http://api.paypal.com",          // non-https
		"ftp://api.paypal.com",           // non-https scheme
		"https://169.254.169.254",        // link-local metadata IP literal
		"https://169.254.169.254/latest", // metadata path
		"https://127.0.0.1",              // loopback IP literal
		"https://10.0.0.5:8080",          // RFC1918 IP literal
		"https://[::1]",                  // IPv6 loopback literal
		"https://evil.com",               // off-allowlist host
		"https://paypal.com.evil.com",    // suffix-spoof
		"https://evilpaypal.com",         // substring not boundary
		"https://paypal.com@evil.com",    // userinfo trick (host is evil.com)
		"https://internal-service",       // bare internal hostname
		"not a url at all",               // unparseable
	}
	for _, in := range bad {
		if err := ValidateBaseURL(in, allow); err == nil {
			t.Errorf("ValidateBaseURL(%q) = nil, want error", in)
		}
	}
}

func TestValidateBaseURL_AcceptsProviderHosts(t *testing.T) {
	cases := []struct {
		url   string
		allow []string
	}{
		{"https://api.paypal.com", []string{"paypal.com"}},
		{"https://api.sandbox.paypal.com", []string{"paypal.com"}},
		{"https://paypal.com", []string{"paypal.com"}},
		{"https://api.mercadopago.com", []string{"mercadopago.com"}},
		{"https://api.paypal.com/", []string{"paypal.com"}}, // trailing slash
	}
	for _, c := range cases {
		if err := ValidateBaseURL(c.url, c.allow); err != nil {
			t.Errorf("ValidateBaseURL(%q, %v) = %v, want nil", c.url, c.allow, err)
		}
	}
}
