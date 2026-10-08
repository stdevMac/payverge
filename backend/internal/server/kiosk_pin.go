package server

import (
	"errors"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

// kioskPinThrottleKind namespaces shared-terminal clock-in PIN attempts in the
// auth_attempts table. It is DISTINCT from managerPinThrottleKind so a kiosk
// lockout never bleeds into the manager step-up counter (and vice versa): a
// staffer fat-fingering their clock-in PIN must not lock them out of comps, and
// a comp lockout must not stop them clocking in.
const kioskPinThrottleKind = "kiosk_pin"

// ErrKioskPinLocked is returned by VerifyKioskPin when the per-staff kiosk PIN
// throttle is engaged (too many recent failures) or the throttle backend is
// unavailable. Both fail closed — a shared terminal must not become an
// unthrottled PIN oracle.
var ErrKioskPinLocked = errors.New("kiosk pin locked")

// VerifyKioskPin verifies a shared-terminal clock-in PIN for one staffer,
// reusing the manager-PIN throttle instance under the distinct kiosk kind.
//
// Returns:
//   - nil on a correct PIN (throttle cleared)
//   - ErrKioskPinLocked (with retryAt when known) while locked, or when the
//     throttle backend errors (fail-closed, mirroring RequireManagerPIN)
//   - database.ErrPinNotSet when the staffer never enrolled a PIN
//   - database.ErrPinInvalid on a mismatch (a failed attempt is recorded)
//
// The caller MUST tenant-scope the staffer (business + active) BEFORE calling
// this — database.VerifyStaffPin does not check business_id.
func VerifyKioskPin(staffID uint, pin string) (retryAt time.Time, err error) {
	principal := managerPinPrincipal(staffID)

	if managerPinThrottle != nil {
		locked, until, lerr := managerPinThrottle.IsLocked(principal, kioskPinThrottleKind)
		if lerr != nil {
			// Fail CLOSED: the lockout backend is unavailable, so deny rather
			// than let the terminal brute-force PINs unthrottled.
			logger.Logger.Errorf("[KioskPin] throttle backend error for %s; failing closed: %v", principal, lerr)
			return time.Time{}, ErrKioskPinLocked
		}
		if locked {
			return until, ErrKioskPinLocked
		}
	}

	if verr := database.VerifyStaffPin(staffID, pin); verr != nil {
		// Only a genuine wrong-PIN counts toward the brute-force lockout.
		// ErrPinNotSet / DB faults are surfaced without inflating the counter.
		if errors.Is(verr, database.ErrPinInvalid) && managerPinThrottle != nil {
			if rerr := managerPinThrottle.Record(principal, kioskPinThrottleKind); rerr != nil {
				logger.Logger.Errorf("[KioskPin] failed to record attempt for %s: %v", principal, rerr)
			}
		}
		return time.Time{}, verr
	}

	if managerPinThrottle != nil {
		if cerr := managerPinThrottle.Clear(principal, kioskPinThrottleKind); cerr != nil {
			logger.Logger.Errorf("[KioskPin] failed to clear throttle for %s: %v", principal, cerr)
		}
	}
	return time.Time{}, nil
}
