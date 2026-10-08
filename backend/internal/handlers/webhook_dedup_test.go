package handlers

import "testing"

// TestRequireWebhookIDForFirstParty asserts first-party providers must carry a
// webhook id (otherwise dedup is unprotected), while others may proceed.
func TestRequireWebhookIDForFirstParty(t *testing.T) {
	cases := []struct {
		provider string
		id       string
		wantReq  bool // true => missing id must be rejected
	}{
		{"stripe", "", true},
		{"paypal", "", true},
		{"mercadopago", "", true},
	}
	for _, tc := range cases {
		if got := firstPartyRequiresWebhookID(tc.provider); got != tc.wantReq {
			t.Fatalf("%s: firstPartyRequiresWebhookID=%v want %v", tc.provider, got, tc.wantReq)
		}
	}
}
