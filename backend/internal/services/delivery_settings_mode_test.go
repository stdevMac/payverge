package services

import "testing"

func TestResolveDeliveryPaymentMode(t *testing.T) {
	cases := []struct {
		name   string
		stored string
		online bool
		want   string
	}{
		{"unset derives online", "", true, "online"},
		{"unset derives cod", "", false, "cash_on_delivery"},
		{"stored online honored", "online", true, "online"},
		{"stored online without method falls back to cod", "online", false, "cash_on_delivery"},
		{"stored cod honored even with online", "cash_on_delivery", true, "cash_on_delivery"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveDeliveryPaymentMode(tc.stored, tc.online); got != tc.want {
				t.Fatalf("resolveDeliveryPaymentMode(%q,%v)=%q want %q", tc.stored, tc.online, got, tc.want)
			}
		})
	}
}
