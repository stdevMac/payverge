package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

type expectedFileMutationRoute struct {
	method     string
	permission string
}

func TestFileMutationRoutesAreTenantScopedAndPermissionGuarded(t *testing.T) {
	parsed, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}

	uploadGroups := tenantUploadRouteGroups(parsed)
	if len(uploadGroups) == 0 {
		t.Errorf("missing protected tenant route group /businesses/:id/uploads")
	}

	expected := map[string]expectedFileMutationRoute{
		"UploadFile":          {method: httpMethodPost, permission: "files:upload"},
		"UploadFileProtected": {method: httpMethodPost, permission: "files:upload"},
		"UploadBusinessLogo":  {method: httpMethodPost, permission: "files:upload"},
		"DeleteUploadedFile":  {method: httpMethodDelete, permission: "files:delete"},
	}
	found := make(map[string]bool, len(expected))

	ast.Inspect(parsed, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		handler := fileMutationHandler(call, expected)
		if handler == "" {
			return true
		}
		found[handler] = true

		routeSelector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			t.Errorf("%s must be registered as a route method call", handler)
			return true
		}
		receiver, ok := routeSelector.X.(*ast.Ident)
		if !ok || !uploadGroups[receiver.Name] {
			t.Errorf("%s must be registered under /businesses/:id/uploads, not a top-level protected route", handler)
			return true
		}

		want := expected[handler]
		if routeSelector.Sel.Name != want.method {
			t.Errorf("%s must use %s, got %s", handler, want.method, routeSelector.Sel.Name)
		}
		if permission := routePermission(call); permission != want.permission {
			t.Errorf("%s must require %q, got %q", handler, want.permission, permission)
		}
		return true
	})

	for handler := range expected {
		if !found[handler] {
			t.Errorf("missing file mutation route for server.%s", handler)
		}
	}
}

const (
	httpMethodPost   = "POST"
	httpMethodDelete = "DELETE"
)

func tenantUploadRouteGroups(file *ast.File) map[string]bool {
	groups := make(map[string]bool)
	ast.Inspect(file, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			return true
		}
		identifier, ok := assignment.Lhs[0].(*ast.Ident)
		if !ok {
			return true
		}
		call, ok := assignment.Rhs[0].(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Group" {
			return true
		}
		receiver, ok := selector.X.(*ast.Ident)
		if !ok || receiver.Name != "protectedRoutes" {
			return true
		}
		path, ok := stringLiteral(call.Args[0])
		if ok && path == "/businesses/:id/uploads" {
			groups[identifier.Name] = true
		}
		return true
	})
	return groups
}

func fileMutationHandler(call *ast.CallExpr, expected map[string]expectedFileMutationRoute) string {
	for _, arg := range call.Args {
		selector, ok := arg.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		receiver, ok := selector.X.(*ast.Ident)
		if !ok || receiver.Name != "server" {
			continue
		}
		if _, ok := expected[selector.Sel.Name]; ok {
			return selector.Sel.Name
		}
	}
	return ""
}

func routePermission(routeCall *ast.CallExpr) string {
	for _, arg := range routeCall.Args {
		call, ok := arg.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			continue
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "RoleBasedAccessMiddleware" {
			continue
		}
		receiver, ok := selector.X.(*ast.Ident)
		if !ok || receiver.Name != "server" {
			continue
		}
		permission, _ := stringLiteral(call.Args[0])
		return permission
	}
	return ""
}

func stringLiteral(expression ast.Expr) (string, bool) {
	literal, ok := expression.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	return value, err == nil
}
