package main

import "testing"

// TestUpdateNotificationPreferenceRouteRemoved asserts the dead user-level
// Telegram preference route is absent from the route table parsed out of
// main.go, and from the handler-authorization policy that must stay in
// lockstep with that table.
func TestUpdateNotificationPreferenceRouteRemoved(t *testing.T) {
	const gone = "PUT /api/v1/inside/update_notification_preference"
	routes := parseAllRouteInventory(t)
	if routes[gone] {
		t.Fatalf("route %s is still registered", gone)
	}
	if _, ok := handlerAuthorizedProtectedRoutes[gone]; ok {
		t.Fatalf("route policy still declares %s", gone)
	}
}
