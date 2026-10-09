package fiscal

import (
	"os"
	"strconv"
	"strings"
)

// DefaultCFIDThresholdCents is the Factura B unidentified-consumidor-final
// identification threshold in int64 cents. RG 5700/2025 fixed it at a flat
// ARS 10,000,000 (previously an indexed table). Operators can still override
// with FISCAL_AR_CF_ID_THRESHOLD_CENTS if ARCA publishes a new figure before
// a release ships.
const DefaultCFIDThresholdCents int64 = 1_000_000_000

// CFIDThresholdCents returns the env-tunable Factura B CF identification
// threshold (cents). Invalid/empty env falls back to DefaultCFIDThresholdCents.
// Canonical here so both the synchronous issue validation (service) and the
// WSFE mapper (providers/ar) read the same knob.
func CFIDThresholdCents() int64 {
	raw := strings.TrimSpace(os.Getenv("FISCAL_AR_CF_ID_THRESHOLD_CENTS"))
	if raw == "" {
		return DefaultCFIDThresholdCents
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n < 0 {
		return DefaultCFIDThresholdCents
	}
	return n
}
