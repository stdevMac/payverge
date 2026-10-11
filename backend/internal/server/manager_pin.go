package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/auththrottle"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Error codes returned by the manager-PIN middleware. The frontend looks at
// the `code` field on the 4xx body to decide whether to prompt for a PIN
// (`pin_required`) or surface an "incorrect PIN" toast (`pin_invalid`).
const (
	ErrCodePinRequired = "pin_required"
	ErrCodePinInvalid  = "pin_invalid"
)

// managerPinHeader is the HTTP header the frontend sets after collecting a
// PIN. Kept lowercase here to match the Go `http.Header` canonical-key form
// used by Gin's `GetHeader`.
const managerPinHeader = "X-Manager-Pin"

// compVoidAuditContextKey carries the `*database.CompVoidAudit` stub that the
// middleware seeded into the request context. Downstream handlers can fill
// in fields like AmountCents or TargetID before flushing, but the row is
// already persisted by the middleware itself (tamper-evident = write first).
const compVoidAuditContextKey = "comp_void_audit_entry"

// managerPinThrottle is the process-wide DB-backed PIN lockout. Installed
// once from main via SetManagerPinThrottle; nil disables lockout (tests/dev
// that never call the setter behave exactly as before this change).
var managerPinThrottle *auththrottle.Throttle

// managerPinThrottleKind namespaces PIN attempts in the auth_attempts table.
const managerPinThrottleKind = "manager_pin"

// SetManagerPinThrottle installs the lockout backend. Call once at startup.
func SetManagerPinThrottle(t *auththrottle.Throttle) { managerPinThrottle = t }

// managerPinPrincipal is the per-staff lockout key. Brute-force ceiling is
// per manager identity, so a single attacker switching IPs still hits one
// shared counter.
func managerPinPrincipal(staffID uint) string {
	return "staff:" + strconv.FormatUint(uint64(staffID), 10)
}

// RequireManagerPIN gates sensitive bill operations (comps, voids, refunds)
// behind a per-staff PIN. The behavior matrix is:
//
//   - Web3 / OAuth business owners: no PIN check. They already authenticate
//     end-to-end and have unrestricted authority over their own business.
//   - Staff with a configured PIN hash: must supply `X-Manager-Pin`; bcrypt
//     mismatch returns 403 with code `pin_invalid`. Missing header → 403
//     with code `pin_required`.
//   - Staff without a PIN hash yet: action is allowed (we don't lock people
//     out before onboarding), but the audit row is marked `pin_present=false`
//     so reviewers can spot it.
//
// On every accepted call we synchronously append a row to `comp_void_audit`
// with the supplied `targetType` / `action`. The middleware does NOT know
// the bill-item ID or amount yet; the downstream handler can decorate the
// stub via UpdateCompVoidAudit(c, ...) before the response is written.
func RequireManagerPIN(targetType, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		entry := &database.CompVoidAudit{
			TargetType: targetType,
			Action:     action,
			IPAddress:  c.ClientIP(),
			UserAgent:  c.GetHeader("User-Agent"),
		}

		// Best-effort: derive business_id from the URL when present so the
		// audit row is queryable by business even if the handler bails out
		// later. Falls back to 0 (caller can patch in UpdateCompVoidAudit).
		if rawBiz := extractBusinessID(c); rawBiz != "" {
			if bizID, err := strconv.ParseUint(rawBiz, 10, 64); err == nil {
				entry.BusinessID = uint(bizID)
			}
		}

		// `target_id` comes from URL params when available — handlers can
		// also overwrite via UpdateCompVoidAudit (e.g. for refund flows that
		// thread the payment ID separately).
		for _, key := range []string{"item_id", "bill_id", "payment_id"} {
			if v := strings.TrimSpace(c.Param(key)); v != "" {
				entry.TargetID = v
				break
			}
		}

		tokenType, _ := c.Get("token_type")
		ts, _ := tokenType.(string)

		// Owners (web3 / OAuth) bypass the PIN — they're the highest authority
		// on the business and we don't want to invent a separate flow for them.
		// We still log the audit row so the trail covers them too.
		if ts != "staff" {
			entry.PinPresent = false
			persistAudit(c, entry)
			c.Next()
			return
		}

		staffID := staffIDFromContext(c)
		if staffID == nil {
			RespondWithError(c, http.StatusForbidden, ErrCodeForbidden, "Staff identity is required for this action")
			c.Abort()
			return
		}
		entry.StaffID = staffID

		principal := managerPinPrincipal(*staffID)
		if managerPinThrottle != nil {
			locked, _, lerr := managerPinThrottle.IsLocked(principal, managerPinThrottleKind)
			if lerr != nil {
				// Fail CLOSED: the PIN lockout backend is unavailable (auth_attempts
				// missing or DB erroring), so deny this privileged action rather than
				// wave it through unthrottled. Logged so operators see the degradation.
				logger.Logger.Errorf("[ManagerPin] throttle backend error for %s; failing closed: %v", principal, lerr)
				RespondWithError(c, http.StatusTooManyRequests, ErrCodeRateLimited,
					"Manager PIN verification is temporarily unavailable. Try again shortly.")
				c.Abort()
				return
			}
			if locked {
				RespondWithError(c, http.StatusTooManyRequests, ErrCodeRateLimited,
					"Too many incorrect manager PIN attempts. Try again later.")
				c.Abort()
				return
			}
		}

		hasPin, err := database.StaffHasPin(*staffID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			RespondWithError(c, http.StatusInternalServerError, ErrCodeForbidden, "Failed to verify manager PIN")
			c.Abort()
			return
		}

		if !hasPin {
			// Pre-enrollment grace: allow the action but mark the audit row
			// so operators can see who acted without a PIN configured.
			entry.PinPresent = false
			persistAudit(c, entry)
			c.Next()
			return
		}

		supplied := strings.TrimSpace(c.GetHeader(managerPinHeader))
		if supplied == "" {
			RespondWithError(c, http.StatusForbidden, ErrCodePinRequired, "Manager PIN required")
			c.Abort()
			return
		}

		if err := database.VerifyStaffPin(*staffID, supplied); err != nil {
			// Treat any verification error (ErrPinInvalid, ErrPinNotSet,
			// or unexpected DB faults) as an auth failure. ErrPinNotSet
			// here is defensive — StaffHasPin already returned true.
			if managerPinThrottle != nil {
				if rerr := managerPinThrottle.Record(principal, managerPinThrottleKind); rerr != nil {
					logger.Logger.Errorf("[ManagerPin] failed to record failed PIN attempt for %s: %v", principal, rerr)
				}
			}
			RespondWithError(c, http.StatusForbidden, ErrCodePinInvalid, "Invalid manager PIN")
			c.Abort()
			return
		}

		if managerPinThrottle != nil {
			if cerr := managerPinThrottle.Clear(principal, managerPinThrottleKind); cerr != nil {
				logger.Logger.Errorf("[ManagerPin] failed to clear PIN throttle for %s: %v", principal, cerr)
			}
		}
		entry.PinPresent = true
		persistAudit(c, entry)
		c.Next()
	}
}

// persistAudit writes the audit row and stashes a pointer in the request
// context so the downstream handler can patch in late-bound fields (amount,
// target_id) by calling UpdateCompVoidAudit.
func persistAudit(c *gin.Context, entry *database.CompVoidAudit) {
	if err := database.RecordCompVoidAudit(entry); err != nil {
		// Audit failures are logged but never block the action — we'd
		// rather have an unaudited void than a stuck terminal during a
		// service. The error log surfaces in observability tooling.
		c.Error(err) //nolint:errcheck // best-effort observability
		return
	}
	c.Set(compVoidAuditContextKey, entry)
}

// UpdateCompVoidAudit lets a downstream handler enrich the audit row that
// the middleware already persisted (e.g. once the bill-item amount or the
// canonical target_id is known). The function silently no-ops when the
// middleware did not stash a row (owners-with-failed-write, etc.).
func UpdateCompVoidAudit(c *gin.Context, mutate func(*database.CompVoidAudit)) {
	raw, exists := c.Get(compVoidAuditContextKey)
	if !exists {
		return
	}
	entry, ok := raw.(*database.CompVoidAudit)
	if !ok || entry == nil {
		return
	}
	mutate(entry)
	if err := database.GetDB().Save(entry).Error; err != nil {
		c.Error(err) //nolint:errcheck // best-effort observability
	}
}
