package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

type sourceRouteGroup struct {
	parent     string
	prefix     string
	middleware []ast.Expr
}

type sourceProtectedRoute struct {
	method     string
	path       string
	receiver   string
	middleware []ast.Expr
}

type expectedRouteAuthorization struct {
	domain              string
	method              string
	path                string
	permission          string
	resolver            string
	operationalRequired bool
}

func TestRepresentativeProtectedRouteAuthorizationMatrix(t *testing.T) {
	routes := parseProtectedRouteInventory(t)
	expected := []expectedRouteAuthorization{
		{domain: "uploads", method: "POST", path: "/api/v1/inside/businesses/:id/uploads", permission: "files:upload"},
		{domain: "settings-write", method: "PUT", path: "/api/v1/inside/businesses/:id/operating-hours", permission: "settings:write", operationalRequired: true},
		{domain: "payments", method: "POST", path: "/api/v1/inside/bills/:bill_id/alternative-payment", permission: "bills:payment", resolver: "RequireBillBusinessAccess"},
		{domain: "staff", method: "POST", path: "/api/v1/inside/businesses/:id/staff/invite", permission: "staff:invite", operationalRequired: true},
		{domain: "settings", method: "GET", path: "/api/v1/inside/businesses/:id/operating-hours", permission: "settings:read"},
		{domain: "plugins", method: "POST", path: "/api/v1/inside/businesses/:id/plugins/:plugin_id/enable", permission: "plugins:write", operationalRequired: true},
		{domain: "fiscal", method: "POST", path: "/api/v1/inside/businesses/:id/fiscal/receipts/issue", permission: "fiscal:issue", operationalRequired: true},
		{domain: "printing", method: "POST", path: "/api/v1/inside/businesses/:id/printers", permission: "printers:write", operationalRequired: true},
		{domain: "crm", method: "POST", path: "/api/v1/inside/businesses/:id/crm/customers", permission: "crm:write", operationalRequired: true},
		{domain: "inventory", method: "POST", path: "/api/v1/inside/businesses/:id/inventory/adjustments", permission: "inventory:adjust", operationalRequired: true},
		{domain: "analytics", method: "GET", path: "/api/v1/inside/businesses/:id/analytics/sales", permission: "analytics:sales", operationalRequired: true},
		{domain: "ai", method: "POST", path: "/api/v1/inside/businesses/:id/ai/director/ask", permission: "director:write", operationalRequired: true},
		{domain: "administration", method: "GET", path: "/api/v1/admin/users", resolver: "AuthenticationAdminMiddleware"},
	}

	for _, want := range expected {
		want := want
		t.Run(want.domain+"/"+want.method+" "+want.path, func(t *testing.T) {
			route, ok := routes[want.method+" "+want.path]
			if !ok {
				t.Fatalf("declared representative route is not wired")
			}
			if want.permission != "" && !routeHasPermission(route, want.permission) {
				t.Fatalf("route must declare permission %q", want.permission)
			}
			if want.resolver != "" && !routeHasMiddleware(route, want.resolver) {
				t.Fatalf("route must declare resolver %s", want.resolver)
			}
			if want.operationalRequired && !routeHasMiddleware(route, "RequireOperationalBusiness") {
				t.Fatalf("route must declare the operational-business (suspension) gate")
			}
		})
	}
}

func TestProtectedRouteInventoryHasExplicitAuthorizationPolicy(t *testing.T) {
	routes := parseProtectedRouteInventory(t)
	for key, route := range routes {
		if strings.HasPrefix(route.path, "/api/v1/admin") {
			if !routeHasMiddleware(route, "AuthenticationAdminMiddleware") {
				t.Errorf("%s is missing administrator authentication", key)
			}
			continue
		}
		if !routeHasMiddleware(route, "HybridAuthenticationMiddleware") {
			t.Errorf("%s is missing protected-route authentication", key)
		}
		if routeHasAnyMiddleware(route, "RoleBasedAccessMiddleware", "RoleBasedAnyAccessMiddleware") {
			continue
		}
		policy, ok := handlerAuthorizedProtectedRoutes[key]
		if !ok {
			t.Errorf("%s has no route RBAC and no centralized handler-authorization declaration", key)
			continue
		}
		if policy.domain == "" || policy.scope == "" || policy.resolver == "" || policy.reason == "" {
			t.Errorf("%s has an incomplete handler-authorization declaration: %+v", key, policy)
		}
		switch policy.scope {
		case routeTenantBusinessPath:
			if !strings.Contains(route.path, "/businesses/:id") {
				t.Errorf("%s declares business-path scope without a canonical /businesses/:id path", key)
			}
		case routeTenantResolved, routePrincipalSelf:
			// The named resolver documents the canonical handler-owned binding.
		case routePlatformGlobal:
			if strings.Contains(route.path, "/businesses/:id") {
				t.Errorf("%s cannot declare a tenant path as platform-global", key)
			}
		default:
			t.Errorf("%s declares unknown authorization scope %q", key, policy.scope)
		}
	}

	for key := range handlerAuthorizedProtectedRoutes {
		if _, ok := routes[key]; !ok {
			t.Errorf("stale protected-route policy declaration for %s", key)
		}
	}
}

func parseProtectedRouteInventory(t *testing.T) map[string]sourceProtectedRoute {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}

	groups := map[string]sourceRouteGroup{
		"protectedRoutes": {prefix: "/api/v1/inside"},
		"adminRoutes":     {prefix: "/api/v1/admin"},
	}
	ast.Inspect(file, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			return true
		}
		name, ok := assignment.Lhs[0].(*ast.Ident)
		if !ok {
			return true
		}
		call, ok := assignment.Rhs[0].(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Group" {
			return true
		}
		parent, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		prefix, ok := routeStringLiteral(call.Args[0])
		if !ok {
			return true
		}
		groups[name.Name] = sourceRouteGroup{parent: parent.Name, prefix: prefix, middleware: call.Args[1:]}
		return true
	})

	groupUse := map[string][]ast.Expr{}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Use" {
			return true
		}
		receiver, ok := selector.X.(*ast.Ident)
		if ok {
			groupUse[receiver.Name] = append(groupUse[receiver.Name], call.Args...)
		}
		return true
	})

	routes := map[string]sourceProtectedRoute{}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) < 2 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !isRouteMethod(selector.Sel.Name) {
			return true
		}
		receiver, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		path, inherited, protected := resolveProtectedGroup(receiver.Name, groups, groupUse)
		if !protected {
			return true
		}
		routePath, ok := routeStringLiteral(call.Args[0])
		if !ok {
			return true
		}
		middleware := append([]ast.Expr{}, inherited...)
		middleware = append(middleware, call.Args[1:len(call.Args)-1]...)
		fullPath := strings.TrimSuffix(path, "/") + routePath
		if routePath == "" {
			fullPath = strings.TrimSuffix(path, "/")
		}
		key := selector.Sel.Name + " " + fullPath
		routes[key] = sourceProtectedRoute{method: selector.Sel.Name, path: fullPath, receiver: receiver.Name, middleware: middleware}
		return true
	})
	return routes
}

func resolveProtectedGroup(name string, groups map[string]sourceRouteGroup, uses map[string][]ast.Expr) (string, []ast.Expr, bool) {
	seen := map[string]bool{}
	parts := []string{}
	middleware := []ast.Expr{}
	for name != "" && !seen[name] {
		seen[name] = true
		group, ok := groups[name]
		if !ok {
			return "", nil, false
		}
		parts = append([]string{group.prefix}, parts...)
		middleware = append(group.middleware, middleware...)
		middleware = append(uses[name], middleware...)
		if name == "protectedRoutes" || name == "adminRoutes" {
			return strings.Join(parts, ""), middleware, true
		}
		name = group.parent
	}
	return "", nil, false
}

func isRouteMethod(name string) bool {
	switch name {
	case "GET", "POST", "PUT", "PATCH", "DELETE":
		return true
	default:
		return false
	}
}

func routeStringLiteral(expr ast.Expr) (string, bool) {
	literal, ok := expr.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	return value, err == nil
}

func routeHasPermission(route sourceProtectedRoute, want string) bool {
	for _, expression := range route.middleware {
		call, ok := expression.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			continue
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "RoleBasedAccessMiddleware" {
			continue
		}
		if literal, ok := routeStringLiteral(call.Args[0]); ok && literal == want {
			return true
		}
		// Permission constants are used for money-sensitive routes. Resolve the
		// representative constants explicitly so a rename fails the matrix.
		if nested, ok := call.Args[0].(*ast.CallExpr); ok && len(nested.Args) == 1 {
			if selector, ok := nested.Args[0].(*ast.SelectorExpr); ok {
				constantPermissions := map[string]string{
					"PermBillsPayment": "bills:payment",
					"PermBillsRead":    "bills:read",
				}
				if constantPermissions[selector.Sel.Name] == want {
					return true
				}
			}
		}
	}
	return false
}

func routeHasAnyMiddleware(route sourceProtectedRoute, names ...string) bool {
	for _, name := range names {
		if routeHasMiddleware(route, name) {
			return true
		}
	}
	return false
}

func routeHasMiddleware(route sourceProtectedRoute, name string) bool {
	for _, expression := range route.middleware {
		found := false
		ast.Inspect(expression, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if ok && selector.Sel.Name == name {
				found = true
				return false
			}
			return !found
		})
		if found {
			return true
		}
	}
	return false
}
