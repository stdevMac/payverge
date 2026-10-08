package config

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

// RegistrationModeEnv selects who may create a new operator account.
//
//   - "invite": signup requires a valid launch invite minted by a platform
//     admin (POST /api/v1/admin/runtime-controls/invite-batches). Default.
//   - "open":   anyone may sign up and create a business; invite codes are
//     ignored.
//   - "closed": no new identities are created through any signup path
//     (email, Google, wallet). Existing users can still sign in, and
//     operators are provisioned with `server admin create` or ADMIN_EMAIL.
const RegistrationModeEnv = "REGISTRATION_MODE"

type RegistrationModeValue string

const (
	RegistrationModeInvite RegistrationModeValue = "invite"
	RegistrationModeOpen   RegistrationModeValue = "open"
	RegistrationModeClosed RegistrationModeValue = "closed"
)

// DefaultRegistrationMode applies when REGISTRATION_MODE is unset or empty.
const DefaultRegistrationMode = RegistrationModeInvite

var registrationModeCache atomic.Pointer[RegistrationModeValue]

// ParseRegistrationMode normalises a raw REGISTRATION_MODE value. Empty
// resolves to DefaultRegistrationMode. Any unknown value is an error and fails
// closed to "closed" so a typo can never open public signup.
func ParseRegistrationMode(raw string) (RegistrationModeValue, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return DefaultRegistrationMode, nil
	case string(RegistrationModeInvite):
		return RegistrationModeInvite, nil
	case string(RegistrationModeOpen):
		return RegistrationModeOpen, nil
	case string(RegistrationModeClosed):
		return RegistrationModeClosed, nil
	default:
		return RegistrationModeClosed, fmt.Errorf("invalid %s %q: want invite|open|closed", RegistrationModeEnv, raw)
	}
}

// RegistrationMode returns the process registration mode (cached after the
// first read; signup paths consult it on every request).
func RegistrationMode() RegistrationModeValue {
	// A public demo never creates identities (DEMO_MODE).
	if DemoModeEnabled() {
		return RegistrationModeClosed
	}
	if v := registrationModeCache.Load(); v != nil {
		return *v
	}
	mode, _ := ParseRegistrationMode(os.Getenv(RegistrationModeEnv))
	registrationModeCache.Store(&mode)
	return mode
}

// SetRegistrationModeForTesting pins the registration mode for the duration
// of a test and restores the previous cache state on cleanup.
func SetRegistrationModeForTesting(tb testing.TB, mode RegistrationModeValue) {
	tb.Helper()
	prev := registrationModeCache.Load()
	m := mode
	registrationModeCache.Store(&m)
	tb.Cleanup(func() { registrationModeCache.Store(prev) })
}

// Self-host bootstrap env names. ADMIN_EMAIL + ADMIN_PASSWORD provision the
// first platform admin on boot; DEMO_DATA seeds the demo showroom for that
// admin on first boot.
const (
	AdminEmailEnv    = "ADMIN_EMAIL"
	AdminPasswordEnv = "ADMIN_PASSWORD"
	DemoDataEnv      = "DEMO_DATA"
)

// DemoDataEnabled reports whether DEMO_DATA asks for the demo showroom to be
// seeded for the bootstrap admin. Default false.
func DemoDataEnabled() bool {
	return envTruthy(os.Getenv(DemoDataEnv))
}

func envTruthy(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
