package main

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
)

// The dashboard reads kitchen-orders-status for every staff session and hides
// the Bills and Kitchen tabs when that read fails, so it must be gated on a
// permission every staff role holds (business:read), not settings:read, which
// kitchen staff lack.
func TestKitchenOrdersStatusReadableByEveryStaffRole(t *testing.T) {
	routes := parseProtectedRouteInventory(t)
	route, ok := routes["GET /api/v1/inside/businesses/:id/kitchen-orders-status"]
	if !ok {
		t.Fatal("kitchen-orders-status route is not wired")
	}
	if !routeHasPermission(route, "business:read") {
		t.Fatal("kitchen-orders-status must be gated on business:read")
	}
	if routeHasPermission(route, "settings:read") {
		t.Fatal("kitchen-orders-status must not require settings:read: kitchen staff lack it")
	}
	for _, role := range []database.StaffRole{
		database.StaffRoleManager, database.StaffRoleServer,
		database.StaffRoleHost, database.StaffRoleKitchen,
	} {
		held := false
		for _, perm := range server.StaffRolePermissions[role] {
			if perm == server.PermBusinessRead {
				held = true
				break
			}
		}
		if !held {
			t.Errorf("staff role %s lacks business:read and could not read kitchen-orders-status", role)
		}
	}

	toggle, ok := routes["POST /api/v1/inside/businesses/:id/toggle-kitchen-orders"]
	if !ok {
		t.Fatal("toggle-kitchen-orders route is not wired")
	}
	if !routeHasPermission(toggle, "settings:write") {
		t.Fatal("toggle-kitchen-orders must stay on settings:write")
	}
}
