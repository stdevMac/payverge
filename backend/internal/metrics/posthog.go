package metrics

import (
	"github.com/posthog/posthog-go"
	"log"
)

var postHogClient posthog.Client

// InitPostHogClient Initialize the PostHog client
func InitPostHogClient(apiKey string, endpoint string) {
	client, err := posthog.NewWithConfig(apiKey, posthog.Config{
		Endpoint: endpoint, // You can set this to your self-hosted PostHog endpoint if applicable
	})

	if err != nil {
		log.Fatalf("Failed to initialize PostHog client: %v", err)
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
		log.Printf("Error tracking event '%s' for user '%s': %v", event, distinctId, err)
	}

	return err
}

// IdentifyUser can be used to update user profile data in PostHog
func IdentifyUser(distinctId string, userProperties map[string]interface{}) error {
	// Skip tracking if client is not initialized (e.g., during tests)
	if postHogClient == nil {
		return nil
	}

	propertiesPostHog := posthog.NewProperties()

	for key, value := range userProperties {
		propertiesPostHog.Set(key, value)
	}
	err := postHogClient.Enqueue(posthog.Identify{
		DistinctId: distinctId,
		Properties: propertiesPostHog,
	})

	if err != nil {
		log.Printf("Error identifying user '%s': %v", distinctId, err)
	}

	return err
}

// TrackGeneralEvent is used for non-user-specific events like API hits
func TrackGeneralEvent(event string, properties map[string]interface{}) error {
	// Skip tracking if client is not initialized (e.g., during tests)
	if postHogClient == nil {
		return nil
	}

	propertiesPostHog := posthog.NewProperties()

	for key, value := range properties {
		propertiesPostHog.Set(key, value)
	}

	err := postHogClient.Enqueue(posthog.Capture{
		DistinctId: "system", // Generic identifier for system-wide events
		Event:      event,
		Properties: propertiesPostHog,
	})

	if err != nil {
		log.Printf("Error tracking general event '%s': %v", event, err)
	}

	return err
}

// Close the PostHog client properly to avoid leaks
func ClosePostHogClient() {
	postHogClient.Close()
}
