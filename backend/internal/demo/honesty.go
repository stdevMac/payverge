package demo

import (
	"strings"
	"time"
)

// Append cadence matches cmd/app/main.go admin_demo_append cron ("0 * * * *").
const DefaultAppendInterval = time.Hour

const (
	HeartbeatFresh   = "fresh"
	HeartbeatStale   = "stale"
	HeartbeatUnknown = "unknown"
)

// Heartbeat reports whether the hourly demo append worker is alive relative
// to 2× DefaultAppendInterval. Surfaced on the Demo Center summary so a dead
// admin_demo_append cannot claim success for weeks.
type Heartbeat struct {
	Status             string     `json:"status"`
	LastAppendAt       *time.Time `json:"last_append_at,omitempty"`
	AppendIntervalSecs int64      `json:"append_interval_seconds"`
	StaleAfterSecs     int64      `json:"stale_after_seconds"`
}

// isExplorerLinkableTxHash reports whether a tx hash looks like a real EVM
// transaction id that a block explorer would accept as a deep link.
// Demo seeds must never produce these — fake-but-checkable is worse than
// obviously synthetic.
func isExplorerLinkableTxHash(tx string) bool {
	tx = strings.TrimSpace(tx)
	if len(tx) < 3 {
		return false
	}
	// Canonical EVM tx: 0x + 64 hex chars (optionally allow 0x + 40–64).
	if !strings.HasPrefix(strings.ToLower(tx), "0x") {
		return false
	}
	body := tx[2:]
	if len(body) < 40 {
		return false
	}
	for _, r := range body {
		isHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
		if !isHex {
			return false
		}
	}
	return true
}
