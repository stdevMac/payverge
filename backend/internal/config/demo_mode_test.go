package config

import (
	"errors"
	"testing"
)

func TestDemoModeForcesLogEmailAndClosedSignup(t *testing.T) {
	t.Setenv(EmailProviderEnv, "resend")
	SetRegistrationModeForTesting(t, RegistrationModeOpen)

	SetDemoModeForTesting(t, false)
	if got := EmailProvider(); got != "resend" {
		t.Fatalf("EmailProvider off = %q", got)
	}
	if got := RegistrationMode(); got != RegistrationModeOpen {
		t.Fatalf("RegistrationMode off = %q", got)
	}

	SetDemoModeForTesting(t, true)
	if got := EmailProvider(); got != EmailProviderLog {
		t.Fatalf("EmailProvider in DEMO_MODE = %q, want log", got)
	}
	if got := RegistrationMode(); got != RegistrationModeClosed {
		t.Fatalf("RegistrationMode in DEMO_MODE = %q, want closed", got)
	}
}

func TestValidateDemoModeEnvNeedsDemoData(t *testing.T) {
	SetDemoModeForTesting(t, true)
	t.Setenv(DemoDataEnv, "")
	if err := ValidateDemoModeEnv(); !errors.Is(err, ErrDemoModeWithoutData) {
		t.Fatalf("err = %v, want ErrDemoModeWithoutData", err)
	}
	t.Setenv(DemoDataEnv, "true")
	if err := ValidateDemoModeEnv(); err != nil {
		t.Fatalf("err = %v", err)
	}
	SetDemoModeForTesting(t, false)
	t.Setenv(DemoDataEnv, "")
	if err := ValidateDemoModeEnv(); err != nil {
		t.Fatalf("DEMO_MODE off must not need DEMO_DATA: %v", err)
	}
}

func TestDemoModeEnvParsing(t *testing.T) {
	for raw, want := range map[string]bool{"true": true, "1": true, "ON": true, "": false, "false": false, "nope": false} {
		demoModeCache.Store(nil)
		t.Setenv(DemoModeEnv, raw)
		if got := DemoModeEnabled(); got != want {
			t.Errorf("DEMO_MODE=%q -> %v, want %v", raw, got, want)
		}
	}
	demoModeCache.Store(nil)
}

func TestDemoResetUTCAndWriteRate(t *testing.T) {
	cases := map[string]string{"": "03:00", "04:30": "04:30", "24:00": "03:00", "3:00": "03:00", "ab:cd": "03:00", "23:59": "23:59"}
	for raw, want := range cases {
		t.Setenv(DemoResetUTCEnv, raw)
		if got := DemoResetUTC(); got != want {
			t.Errorf("DEMO_RESET_UTC=%q -> %q, want %q", raw, got, want)
		}
	}
	for raw, want := range map[string]int{"": 30, "0": 30, "-4": 30, "x": 30, "120": 120} {
		t.Setenv(DemoWriteRateEnv, raw)
		if got := DemoWriteRatePerMinute(); got != want {
			t.Errorf("DEMO_WRITE_RATE_PER_MIN=%q -> %d, want %d", raw, got, want)
		}
	}
}
