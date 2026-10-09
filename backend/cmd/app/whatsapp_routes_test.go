package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// whatsAppGatedCalls maps each WhatsApp call to the enclosing `if` conditions
// of every site that makes it, rendered as source text and in source order, so
// a duplicate registration cannot hide behind a later gated one. src is passed
// to parser.ParseFile (nil reads filename from disk).
func whatsAppGatedCalls(t *testing.T, filename string, src any) map[string][][]string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}
	// Print the full condition (operators included) so `!gate()` or
	// `gate() || x` can never pass for `gate()`.
	render := func(n ast.Node) string {
		var b bytes.Buffer
		if err := printer.Fprint(&b, fset, n); err != nil {
			t.Fatalf("print condition: %v", err)
		}
		return b.String()
	}

	found := map[string][][]string{}
	var stack []ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		stack = append(stack, n)
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		var key string
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "NewWhatsAppManager" {
			key = "services.NewWhatsAppManager"
		} else if ok && (sel.Sel.Name == "GET" || sel.Sel.Name == "POST") && len(call.Args) > 0 {
			if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if path, err := strconv.Unquote(lit.Value); err == nil && strings.Contains(path, "/whatsapp/") {
					key = sel.Sel.Name + " " + path
				}
			}
		}
		if key == "" {
			return true
		}
		conds := []string{}
		for i := len(stack) - 2; i >= 0; i-- {
			ifStmt, ok := stack[i].(*ast.IfStmt)
			if !ok {
				continue
			}
			// Only count the branch we are in when it is the `then` body.
			if within(ifStmt.Body, n) {
				conds = append(conds, render(ifStmt.Cond))
			}
		}
		found[key] = append(found[key], conds)
		return true
	})
	return found
}

func within(outer, inner ast.Node) bool {
	return outer != nil && outer.Pos() <= inner.Pos() && inner.End() <= outer.End()
}

const whatsAppGate = "services.WhatsAppAvailable()"

// whatsAppGatingViolations lists every way calls breaks the gating contract:
// every site that starts the manager or mounts a mutating/pairing route sits
// directly inside `if services.WhatsAppAvailable()`, and status is never gated
// on WhatsApp.
func whatsAppGatingViolations(calls map[string][][]string) []string {
	var out []string
	for _, key := range []string{
		"services.NewWhatsAppManager",
		"POST /businesses/:id/whatsapp/connect",
		"POST /businesses/:id/whatsapp/disconnect",
		"GET /businesses/:id/whatsapp/qr",
	} {
		sites, ok := calls[key]
		if !ok {
			out = append(out, key+" not found")
			continue
		}
		for _, conds := range sites {
			if len(conds) == 0 || conds[0] != whatsAppGate {
				out = append(out, key+" must sit directly inside `if "+whatsAppGate+"`; enclosing conditions: ["+strings.Join(conds, " | ")+"]")
			}
		}
	}

	status := "GET /businesses/:id/whatsapp/status"
	sites, ok := calls[status]
	if !ok {
		return append(out, status+" must stay mounted in every build")
	}
	for _, conds := range sites {
		for _, cond := range conds {
			if strings.Contains(strings.ToLower(cond), "whatsapp") {
				out = append(out, status+" must not be gated on WhatsApp availability (found `if "+cond+"`)")
			}
		}
	}
	return out
}

// WhatsApp connect/disconnect/qr and manager start-up are only reachable when
// the binary has the whatsapp tag AND WHATSAPP_ENABLED=true. Status stays
// mounted in every build so the dashboard can read `built` and hide the channel.
func TestWhatsAppRoutesGatedByAvailability(t *testing.T) {
	for _, v := range whatsAppGatingViolations(whatsAppGatedCalls(t, "main.go", nil)) {
		t.Error("main.go: " + v)
	}
}

// The checker itself must reject gates that merely mention the right call.
func TestWhatsAppGatingCheckRejectsWrongConditions(t *testing.T) {
	body := func(cond string) string {
		return `package main
func main() {
	r.GET("/businesses/:id/whatsapp/status", h)
	if ` + cond + ` {
		services.NewWhatsAppManager(nil, nil)
		r.POST("/businesses/:id/whatsapp/connect", h)
		r.POST("/businesses/:id/whatsapp/disconnect", h)
		r.GET("/businesses/:id/whatsapp/qr", h)
	}
}
`
	}
	if v := whatsAppGatingViolations(whatsAppGatedCalls(t, "ok.go", body(whatsAppGate))); len(v) != 0 {
		t.Fatalf("correctly gated source reported violations: %v", v)
	}
	for _, cond := range []string{
		"!services.WhatsAppAvailable()",
		"services.WhatsAppAvailable() || true",
		"services.WhatsAppRequested()",
		"services.WhatsAppAvailable() == false",
	} {
		if v := whatsAppGatingViolations(whatsAppGatedCalls(t, "bad.go", body(cond))); len(v) != 4 {
			t.Errorf("`if %s` should flag all 4 gated call sites, got %d: %v", cond, len(v), v)
		}
	}

	// Routes in the else branch are reachable exactly when the channel is off.
	elseBranch := `package main
func main() {
	r.GET("/businesses/:id/whatsapp/status", h)
	if services.WhatsAppAvailable() {
		services.NewWhatsAppManager(nil, nil)
	} else {
		r.POST("/businesses/:id/whatsapp/connect", h)
		r.POST("/businesses/:id/whatsapp/disconnect", h)
		r.GET("/businesses/:id/whatsapp/qr", h)
	}
}
`
	if v := whatsAppGatingViolations(whatsAppGatedCalls(t, "else.go", elseBranch)); len(v) != 3 {
		t.Errorf("routes in the else branch should be flagged, got %v", v)
	}

	// An ungated duplicate is reachable even when a later copy is gated.
	duplicate := `package main
func main() {
	r.GET("/businesses/:id/whatsapp/status", h)
	r.GET("/businesses/:id/whatsapp/qr", h)
	if services.WhatsAppAvailable() {
		services.NewWhatsAppManager(nil, nil)
		r.POST("/businesses/:id/whatsapp/connect", h)
		r.POST("/businesses/:id/whatsapp/disconnect", h)
		r.GET("/businesses/:id/whatsapp/qr", h)
	}
}
`
	if v := whatsAppGatingViolations(whatsAppGatedCalls(t, "dup.go", duplicate)); len(v) != 1 || !strings.HasPrefix(v[0], "GET /businesses/:id/whatsapp/qr ") {
		t.Errorf("an ungated duplicate before a gated registration should be flagged once, got %v", v)
	}

	// A second status registration behind the WhatsApp gate is flagged too.
	gatedStatusCopy := strings.Replace(body(whatsAppGate), "{\n\t\tservices.NewWhatsAppManager", "{\n\t\tr.GET(\"/businesses/:id/whatsapp/status\", h)\n\t\tservices.NewWhatsAppManager", 1)
	if v := whatsAppGatingViolations(whatsAppGatedCalls(t, "status.go", gatedStatusCopy)); len(v) != 1 {
		t.Errorf("a gated duplicate status registration should be flagged, got %v", v)
	}
}
