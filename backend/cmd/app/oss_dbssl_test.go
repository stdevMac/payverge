package main

import "testing"

func TestWarnDBSSLDisabled(t *testing.T) {
	cases := []struct {
		production bool
		mode, host string
		want       bool
	}{
		{false, "disable", "db.example.com", false},
		{true, "require", "db.example.com", false},
		{true, "disable", "postgres", false},
		{true, "disable", "localhost", false},
		{true, "disable", "127.0.0.1", false},
		{true, "disable", "::1", false},
		{true, "disable", "db.example.com", true},
		{true, "disable", "10.0.0.5", true},
		{true, "DISABLE", "rds.amazonaws.com", true},
		{true, "disable", "", true},
	}
	for _, tc := range cases {
		if got := warnDBSSLDisabled(tc.production, tc.mode, tc.host); got != tc.want {
			t.Errorf("warnDBSSLDisabled(%v, %q, %q) = %v, want %v", tc.production, tc.mode, tc.host, got, tc.want)
		}
	}
}
