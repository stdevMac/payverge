package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

// DemoModeEnv turns an install into a PUBLIC demo (demo.payverge.io): anyone
// can enter the seeded showroom with one click, so every action that reaches
// outside the box or changes who controls it is refused, outbound email is
// forced to the log provider and signup is closed. See
// docs/self-hosting/public-demo.md. It needs DEMO_DATA (the showroom it
// exposes); ValidateDemoModeEnv refuses DEMO_MODE alone at startup.
const DemoModeEnv = "DEMO_MODE"

// DemoResetUTCEnv is the wall-clock time (UTC, HH:MM) the nightly reset runs.
// It is only displayed (banner, /instance); the timer in deploy/demo owns the
// schedule. Default 03:00.
const DemoResetUTCEnv = "DEMO_RESET_UTC"

// DemoWriteRateEnv caps state-changing requests per client IP per minute while
// DEMO_MODE is on (internal/demomode). Default 30; 0 or invalid = default.
const DemoWriteRateEnv = "DEMO_WRITE_RATE_PER_MIN"

const (
	defaultDemoResetUTC  = "03:00"
	defaultDemoWriteRate = 30
)

var demoModeCache atomic.Pointer[bool]

// DemoModeEnabled reports whether DEMO_MODE is on (cached after first read;
// the demo guard consults it on every request).
func DemoModeEnabled() bool {
	if v := demoModeCache.Load(); v != nil {
		return *v
	}
	on := envTruthy(os.Getenv(DemoModeEnv))
	demoModeCache.Store(&on)
	return on
}

// SetDemoModeForTesting pins DEMO_MODE for one test and restores the previous
// cache state on cleanup.
func SetDemoModeForTesting(tb testing.TB, on bool) {
	tb.Helper()
	prev := demoModeCache.Load()
	v := on
	demoModeCache.Store(&v)
	tb.Cleanup(func() { demoModeCache.Store(prev) })
}

// ErrDemoModeWithoutData is returned when DEMO_MODE is on without DEMO_DATA.
var ErrDemoModeWithoutData = errors.New("DEMO_MODE=true requires DEMO_DATA=true: the public demo exposes the seeded showroom, and there is none to expose")

// ValidateDemoModeEnv returns an error when DEMO_MODE is on without DEMO_DATA.
func ValidateDemoModeEnv() error {
	if DemoModeEnabled() && !DemoDataEnabled() {
		return ErrDemoModeWithoutData
	}
	return nil
}

// DemoResetUTC returns the displayed nightly reset time ("HH:MM", UTC).
func DemoResetUTC() string {
	raw := strings.TrimSpace(os.Getenv(DemoResetUTCEnv))
	if len(raw) != 5 || raw[2] != ':' {
		return defaultDemoResetUTC
	}
	h, errH := strconv.Atoi(raw[:2])
	m, errM := strconv.Atoi(raw[3:])
	if errH != nil || errM != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return defaultDemoResetUTC
	}
	return raw
}

// DemoWriteRatePerMinute is the per-IP write budget while DEMO_MODE is on.
func DemoWriteRatePerMinute() int {
	n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(DemoWriteRateEnv)))
	if err != nil || n <= 0 {
		return defaultDemoWriteRate
	}
	return n
}
