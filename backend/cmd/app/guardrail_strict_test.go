package main

import "testing"

func TestResolveGuardrailStrict(t *testing.T) {
	cases := []struct {
		name       string
		env        string
		production bool
		want       bool
	}{
		{"prod default => strict", "", true, true},
		{"dev default => open", "", false, false},
		{"explicit false in prod overrides to open", "false", true, false},
		{"explicit true in dev overrides to strict", "true", false, true},
		{"whitespace+case false", "  FALSE ", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveGuardrailStrict(tc.env, tc.production); got != tc.want {
				t.Fatalf("resolveGuardrailStrict(%q, %v) = %v, want %v", tc.env, tc.production, got, tc.want)
			}
		})
	}
}
