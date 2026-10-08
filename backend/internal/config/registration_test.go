package config

import "testing"

func TestParseRegistrationMode(t *testing.T) {
	cases := []struct {
		raw     string
		want    RegistrationModeValue
		wantErr bool
	}{
		{"", RegistrationModeInvite, false},
		{"  ", RegistrationModeInvite, false},
		{"invite", RegistrationModeInvite, false},
		{" INVITE ", RegistrationModeInvite, false},
		{"open", RegistrationModeOpen, false},
		{"Open", RegistrationModeOpen, false},
		{"closed", RegistrationModeClosed, false},
		{"opne", RegistrationModeClosed, true}, // typo fails closed
		{"public", RegistrationModeClosed, true},
	}
	for _, tc := range cases {
		got, err := ParseRegistrationMode(tc.raw)
		if (err != nil) != tc.wantErr {
			t.Fatalf("ParseRegistrationMode(%q) err=%v wantErr=%v", tc.raw, err, tc.wantErr)
		}
		if got != tc.want {
			t.Fatalf("ParseRegistrationMode(%q)=%q want %q", tc.raw, got, tc.want)
		}
	}
}

func TestDefaultRegistrationModeIsInvite(t *testing.T) {
	if DefaultRegistrationMode != RegistrationModeInvite {
		t.Fatalf("default registration mode must be invite, got %q", DefaultRegistrationMode)
	}
}

func TestRegistrationModeReadsEnvOnceAndOverrideRestores(t *testing.T) {
	prev := registrationModeCache.Load()
	t.Cleanup(func() { registrationModeCache.Store(prev) })

	registrationModeCache.Store(nil)
	t.Setenv(RegistrationModeEnv, "open")
	if got := RegistrationMode(); got != RegistrationModeOpen {
		t.Fatalf("RegistrationMode()=%q want open", got)
	}
	// Cached: a later env change does not flip the live mode.
	t.Setenv(RegistrationModeEnv, "closed")
	if got := RegistrationMode(); got != RegistrationModeOpen {
		t.Fatalf("RegistrationMode() should be cached, got %q", got)
	}

	t.Run("override", func(t *testing.T) {
		SetRegistrationModeForTesting(t, RegistrationModeClosed)
		if got := RegistrationMode(); got != RegistrationModeClosed {
			t.Fatalf("override not applied: %q", got)
		}
	})
	if got := RegistrationMode(); got != RegistrationModeOpen {
		t.Fatalf("override not restored: %q", got)
	}
}

func TestDemoDataEnabled(t *testing.T) {
	for raw, want := range map[string]bool{"": false, "false": false, "0": false, "true": true, "TRUE": true, "1": true, "yes": true} {
		t.Setenv(DemoDataEnv, raw)
		if got := DemoDataEnabled(); got != want {
			t.Fatalf("DEMO_DATA=%q -> %v want %v", raw, got, want)
		}
	}
}
