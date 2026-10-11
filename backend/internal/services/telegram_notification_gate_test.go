package services

import "testing"

func TestTelegramConnectionConfiguredRequiresChatID(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]interface{}
		want   bool
	}{
		{
			name: "connected with chat id",
			config: map[string]interface{}{
				"is_connected": true,
				"chat_id":      " 12345 ",
			},
			want: true,
		},
		{
			name: "connected without chat id",
			config: map[string]interface{}{
				"is_connected": true,
			},
			want: false,
		},
		{
			name: "connected with nil chat id",
			config: map[string]interface{}{
				"is_connected": true,
				"chat_id":      nil,
			},
			want: false,
		},
		{
			name: "not connected",
			config: map[string]interface{}{
				"is_connected": false,
				"chat_id":      "12345",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := telegramConnectionConfigured(tt.config); got != tt.want {
				t.Fatalf("telegramConnectionConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTelegramEventNotificationEnabledUsesNewAndLegacyPreferences(t *testing.T) {
	tests := []struct {
		name      string
		config    map[string]interface{}
		eventType string
		want      bool
	}{
		{
			name: "new preference disables order created",
			config: map[string]interface{}{
				"notifications": map[string]interface{}{"order_created": false},
			},
			eventType: PluginEventOrderCreated,
			want:      false,
		},
		{
			name: "legacy preference disables order created",
			config: map[string]interface{}{
				"notification_settings": map[string]interface{}{"order_notifications": false},
			},
			eventType: PluginEventOrderCreated,
			want:      false,
		},
		{
			name:      "missing preference defaults enabled for known event",
			config:    map[string]interface{}{},
			eventType: PluginEventOrderCreated,
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TelegramEventNotificationEnabled(tt.config, tt.eventType); got != tt.want {
				t.Fatalf("TelegramEventNotificationEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestTelegramEventNotificationEnabledCanonicalDefaults pins the single source
// of truth shared by the enqueue gate and the send path. Opt-in events default
// off; unknown/unsupported events (e.g. order.status_changed, which has no
// renderer) are fail-closed so we never enqueue something we can't deliver.
func TestTelegramEventNotificationEnabledCanonicalDefaults(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		want      bool
	}{
		{name: "order created defaults on", eventType: PluginEventOrderCreated, want: true},
		{name: "payment received defaults on", eventType: PluginEventPaymentReceived, want: true},
		{name: "reservation created defaults on", eventType: PluginEventReservationCreated, want: true},
		{name: "reservation status changed defaults on", eventType: PluginEventReservationStatusChanged, want: true},
		{name: "low inventory defaults off (opt-in)", eventType: PluginEventInventoryLowStock, want: false},
		{name: "daily summary defaults off (opt-in)", eventType: PluginEventDailySummary, want: false},
		{name: "order status changed has no renderer, fail-closed", eventType: PluginEventOrderStatusChanged, want: false},
		{name: "unknown event fail-closed", eventType: "totally.unknown", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TelegramEventNotificationEnabled(map[string]interface{}{}, tt.eventType); got != tt.want {
				t.Fatalf("TelegramEventNotificationEnabled(%q) = %v, want %v", tt.eventType, got, tt.want)
			}
		})
	}
}
