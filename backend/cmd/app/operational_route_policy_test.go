package main

import (
	"strings"
	"testing"
)

// The admin lifecycle lock (suspend / close) is the only gate on a business.
// These tests keep every tenant mutation behind it: a new write route must
// carry RequireOperationalBusiness (or a bill/table access middleware that
// applies the same lock), declare a handler-level check, or be a reviewed
// exception that has to work on a locked business.

func TestOperatorMutationRoutesDeclareOperationalGate(t *testing.T) {
	routes := parseProtectedRouteInventory(t)
	gated := []string{
		"PUT /api/v1/inside/businesses/:id",
		"PUT /api/v1/inside/businesses/:id/design-settings",
		"PUT /api/v1/inside/businesses/:id/hospitality-settings",
		"PUT /api/v1/inside/businesses/:id/gallery-images",
		"PUT /api/v1/inside/businesses/:id/operating-hours",
		"PUT /api/v1/inside/businesses/:id/special-features",
		"PUT /api/v1/inside/businesses/:id/currencies",
		"PUT /api/v1/inside/businesses/:id/languages",
		"POST /api/v1/inside/businesses/:id/plugins/:plugin_id/enable",
		"POST /api/v1/inside/businesses/:id/plugins/:plugin_id/disable",
		"PUT /api/v1/inside/businesses/:id/plugins/:plugin_id/config",
		"PUT /api/v1/inside/businesses/:id/alerts/settings",
		"POST /api/v1/inside/businesses/:id/toggle-kitchen-orders",
		"POST /api/v1/inside/businesses/:id/tables/qr-branding",
	}

	for _, key := range gated {
		route, ok := routes[key]
		if !ok {
			t.Errorf("operator mutation %s is not wired", key)
			continue
		}
		if !routeHasAnyMiddleware(route, "RequireOperationalBusiness") {
			t.Errorf("operator mutation %s is missing RequireOperationalBusiness", key)
		}
	}
}

func TestEveryBusinessMutationHasOperationalPolicy(t *testing.T) {
	routes := parseProtectedRouteInventory(t)
	// Routes that intentionally carry no lifecycle gate: they must keep
	// working on a suspended or closed business (account exit, setup), or
	// they only read. Any new exception needs an explicit policy decision here.
	operationalGateExceptions := map[string]string{
		"DELETE /api/v1/inside/businesses/:id":                         "account exit",
		"POST /api/v1/inside/businesses/:id/onboarding-state/complete": "preactivation setup",
		"POST /api/v1/inside/businesses/:id/onboarding/qr-preview":     "preactivation setup milestone",
		"POST /api/v1/inside/businesses/:id/uploads":                   "preactivation assets",
		"POST /api/v1/inside/businesses/:id/uploads/protected":         "preactivation assets",
		"POST /api/v1/inside/businesses/:id/uploads/logo":              "preactivation assets",
		"DELETE /api/v1/inside/businesses/:id/uploads":                 "preactivation asset cleanup",
		"POST /api/v1/inside/analytics/dashboard-summaries":            "read-only batched analytics projection",
	}

	for key, route := range routes {
		if route.method == "GET" || !isTenantMutationRoute(key, route) {
			continue
		}
		if routeHasAnyMiddleware(route,
			"RequireOperationalBusiness",
			"RequireBillBusinessAccess",
			"RequireTableBusinessAccess",
		) {
			continue
		}
		if resolver, ok := handlerOperationalGatedMutations[key]; ok && strings.TrimSpace(resolver) != "" {
			continue
		}
		if _, ok := operationalGateExceptions[key]; !ok {
			t.Errorf("business mutation %s has no operational gate and no declared policy exception", key)
		}
	}

	for key := range operationalGateExceptions {
		if _, ok := routes[key]; !ok {
			t.Errorf("stale operational-policy exception for %s", key)
		}
	}
	for key, resolver := range handlerOperationalGatedMutations {
		if _, ok := routes[key]; !ok {
			t.Errorf("stale handler operational-policy declaration for %s", key)
		}
		if strings.TrimSpace(resolver) == "" {
			t.Errorf("handler operational-policy declaration for %s has no resolver", key)
		}
	}
}

func isTenantMutationRoute(key string, route sourceProtectedRoute) bool {
	if !strings.HasPrefix(route.path, "/api/v1/inside/") {
		return false
	}
	if strings.Contains(route.path, "/businesses/:id") {
		return true
	}
	if routeHasAnyMiddleware(route, "RoleBasedAccessMiddleware", "RoleBasedAnyAccessMiddleware") {
		return true
	}
	policy, ok := handlerAuthorizedProtectedRoutes[key]
	return ok && (policy.scope == routeTenantBusinessPath || policy.scope == routeTenantResolved)
}

func TestAccountExitAndReadRoutesRemainAvailableWhenLocked(t *testing.T) {
	routes := parseProtectedRouteInventory(t)
	lockSafe := []string{
		"GET /api/v1/inside/businesses/:id",
		"DELETE /api/v1/inside/businesses/:id",
	}

	for _, key := range lockSafe {
		route, ok := routes[key]
		if !ok {
			t.Errorf("lock-safe route %s is not wired", key)
			continue
		}
		if routeHasAnyMiddleware(route, "RequireOperationalBusiness") {
			t.Errorf("lock-safe route %s must remain available while suspended or closed", key)
		}
	}
}
