package metrics

import (
	"time"

	"github.com/posthog/posthog-go"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

var postHogClient posthog.Client

// PostHogConfigured reports whether analytics delivery can accept events.
// Callers with durable outboxes use this to avoid acknowledging a no-op send.
func PostHogConfigured() bool {
	return postHogClient != nil
}

// InitPostHogClient initializes the PostHog client. A misconfiguration must NOT
// abort startup (EXT-7): analytics is non-essential and every Track* helper
// already no-ops on a nil client.
func InitPostHogClient(apiKey string, endpoint string) {
	InitPostHogClientWithConfig(apiKey, endpoint, 0, 0)
}

// InitPostHogClientWithConfig allows tests to exercise the validation path.
// Interval/BatchSize <= 0 fall through to library defaults; negative values
// trigger a validation error which is logged and treated as disabled analytics.
func InitPostHogClientWithConfig(apiKey, endpoint string, interval time.Duration, batchSize int) {
	cfg := posthog.Config{Endpoint: endpoint}
	if interval != 0 {
		cfg.Interval = interval
	}
	if batchSize != 0 {
		cfg.BatchSize = batchSize
	}
	client, err := posthog.NewWithConfig(apiKey, cfg)
	if err != nil {
		logger.Logger.Warnf("PostHog client init failed; analytics disabled: %v", err)
		return
	}
	postHogClient = client
}

// TrackEvent captures an event with PostHog
func TrackEvent(distinctId string, event string, properties map[string]interface{}) error {
	// Skip tracking if client is not initialized (e.g., during tests)
	if postHogClient == nil {
		return nil
	}

	propertiesPostHog := posthog.NewProperties()

	for key, value := range properties {
		propertiesPostHog.Set(key, value)
	}

	err := postHogClient.Enqueue(posthog.Capture{
		DistinctId: distinctId,        // Unique ID to identify the user
		Event:      event,             // Event name
		Properties: propertiesPostHog, // Add any custom properties
	})

	if err != nil {
		logger.Logger.Errorf("Error tracking event '%s' for user '%s': %v", event, distinctId, err)
	}

	return err
}

// ClosePostHogClient closes the PostHog client properly to avoid leaks
func ClosePostHogClient() {
	if postHogClient == nil {
		return
	}
	_ = postHogClient.Close()
}
