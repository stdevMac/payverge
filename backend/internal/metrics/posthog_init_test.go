package metrics

import "testing"

// TestInitPostHogClientDoesNotFatalOnBadConfig asserts a misconfigured PostHog
// init degrades to a nil client (analytics off) instead of killing the process
// (EXT-7). A negative Interval makes posthog.NewWithConfig fail validation.
func TestInitPostHogClientDoesNotFatalOnBadConfig(t *testing.T) {
	postHogClient = nil
	// An interval/batch validation error is the only failure path; trigger it
	// via the dedicated helper so a bad analytics config cannot abort startup.
	InitPostHogClientWithConfig("key", "https://example.invalid", -1, -1)
	if postHogClient != nil {
		t.Fatalf("expected nil client on invalid config, got non-nil")
	}
	// TrackEvent must safely no-op with a nil client.
	if err := TrackEvent("u", "e", nil); err != nil {
		t.Fatalf("TrackEvent should no-op on nil client, got %v", err)
	}
}
