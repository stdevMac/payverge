package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/demomode"
)

// TestDemoGuardRulesNameWiredRoutes keeps the public-demo deny list honest:
// every exact rule must name a route main.go really registers (method and gin
// pattern), and every prefix rule must cover at least one registered write
// route. A renamed route would otherwise silently fall out of the guard.
func TestDemoGuardRulesNameWiredRoutes(t *testing.T) {
	routes := parseAllRouteInventory(t)
	if len(routes) < 200 {
		t.Fatalf("route inventory looks broken: only %d routes parsed", len(routes))
	}
	for _, rule := range demomode.Rules {
		matched := false
		for key := range routes {
			method, path, _ := strings.Cut(key, " ")
			if rule.Method != "*" && rule.Method != method {
				continue
			}
			if method == "GET" {
				continue
			}
			if rule.Prefix && strings.HasPrefix(path, rule.Path) || !rule.Prefix && path == rule.Path {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("demo guard rule %s %s (prefix=%v) matches no route registered in main.go", rule.Method, rule.Path, rule.Prefix)
		}
	}
}

// TestDemoGuardCoversSensitiveRouteFamilies fails when a new route lands in a
// family the demo must never expose (credentials, invites, uploads, outbound
// webhooks) without a matching guard rule.
func TestDemoGuardCoversSensitiveRouteFamilies(t *testing.T) {
	routes := parseAllRouteInventory(t)
	keys := make([]string, 0, len(routes))
	for k := range routes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	sensitive := []string{"/plugins/", "/staff/invite", "/uploads", "/webhooks/", "/fiscal/", "/password/", "/wallet/"}
	for _, key := range keys {
		method, path, _ := strings.Cut(key, " ")
		if method == "GET" || strings.HasPrefix(path, "/api/v1/admin/") {
			continue
		}
		for _, family := range sensitive {
			if !strings.Contains(path, family) {
				continue
			}
			if _, ok := demomode.Match(method, path); !ok {
				t.Errorf("%s is in sensitive family %q but the demo guard does not refuse it", key, family)
			}
		}
	}
}

// TestDemoGuardRefusesAIImageRoutesAndCustomerSignup fails when a new AI
// image route (anything under the venue's AI or marketing surface whose path
// names an image) or a customer self-registration route lands without a guard
// rule. Image generation stays off in the public demo even when
// DEMO_AI_API_KEY is set, and creating a CRM customer is account creation.
func TestDemoGuardRefusesAIImageRoutesAndCustomerSignup(t *testing.T) {
	routes := parseAllRouteInventory(t)
	const venue = "/api/v1/inside/businesses/:id/"
	checked := 0
	for key := range routes {
		method, path, _ := strings.Cut(key, " ")
		if method == "GET" {
			continue
		}
		isAIImage := strings.HasPrefix(path, venue) && strings.Contains(path, "image") &&
			(strings.Contains(path, "/ai/") || strings.Contains(path, "/marketing/") || strings.Contains(path, "generate-"))
		isCustomerSignup := strings.HasSuffix(path, "/crm/register")
		if !isAIImage && !isCustomerSignup {
			continue
		}
		checked++
		if _, ok := demomode.Match(method, path); !ok {
			t.Errorf("%s must be refused in DEMO_MODE", key)
		}
	}
	if checked < 6 {
		t.Fatalf("only %d AI image / customer signup routes found; the inventory or the filter is broken", checked)
	}
}

// parseAllRouteInventory returns "METHOD /full/pattern" for every literal
// route registration in main.go, resolving nested Group prefixes.
func parseAllRouteInventory(t *testing.T) map[string]bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}
	type group struct{ parent, prefix string }
	groups := map[string]group{}
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
		groups[name.Name] = group{parent: parent.Name, prefix: prefix}
		return true
	})
	resolve := func(name string) string {
		parts := []string{}
		seen := map[string]bool{}
		for !seen[name] {
			seen[name] = true
			g, ok := groups[name]
			if !ok {
				break
			}
			parts = append([]string{g.prefix}, parts...)
			name = g.parent
		}
		return strings.Join(parts, "")
	}
	routes := map[string]bool{}
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
		routePath, ok := routeStringLiteral(call.Args[0])
		if !ok {
			return true
		}
		base := strings.TrimSuffix(resolve(receiver.Name), "/")
		full := base + routePath
		if routePath == "" {
			full = base
		}
		// gin joins group prefixes with path.Join, so "/api/v1/" + "/crm"
		// serves "/api/v1/crm"; collapse the doubled slash the same way.
		for strings.Contains(full, "//") {
			full = strings.ReplaceAll(full, "//", "/")
		}
		routes[selector.Sel.Name+" "+full] = true
		return true
	})
	return routes
}

// demoSensitiveGroups classifies a write route into the groups the public demo
// must default-deny. It looks at literal path segments only, so a
// ":bill_token" parameter does not make a route a "token" route.
func demoSensitiveGroups(path string) []string {
	var groups []string
	literal := []string{}
	for _, seg := range strings.Split(strings.ToLower(path), "/") {
		if seg != "" && !strings.HasPrefix(seg, ":") {
			literal = append(literal, seg)
		}
	}
	has := func(words ...string) bool {
		for _, seg := range literal {
			for _, w := range words {
				if strings.Contains(seg, w) {
					return true
				}
			}
		}
		return false
	}
	if has("auth", "login", "signin", "signup", "register", "password", "session", "token", "api-key", "apikey", "api_key", "2fa", "totp", "mfa", "update_user") ||
		strings.Contains(path, "/account/") || strings.HasSuffix(path, "/account") {
		groups = append(groups, "account/auth/session/token")
	}
	if has("staff", "invit", "pin") {
		groups = append(groups, "staff access")
	}
	if has("plugin", "integration", "credential", "oauth", "connect", "google", "whatsapp", "telegram", "secret") {
		groups = append(groups, "integrations/credentials")
	}
	if has("wallet", "payout", "settlement", "withdraw", "tipping", "refund") {
		groups = append(groups, "wallets/payout")
	}
	if has("webhook", "callback", "url", "push-subscription", "email", "contact", "subscribe", "notify", "intake", "lead") {
		groups = append(groups, "webhooks/outbound")
	}
	if has("upload", "document", "attachment", "image") {
		groups = append(groups, "uploads")
	}
	if strings.HasPrefix(path, "/api/v1/admin/") {
		groups = append(groups, "admin")
	}
	if has("fiscal") {
		groups = append(groups, "fiscal")
	}
	if has("subscription", "billing", "checkout") {
		groups = append(groups, "subscription")
	}
	return groups
}

// demoReviewedSensitiveWrites are writes in a sensitive group that the public
// demo deliberately allows, each with the reason. Anything else in a group
// must be refused by a guard rule.
var demoReviewedSensitiveWrites = map[string]string{
	"POST /api/v1/auth/demo/login":                                                   "the demo sign-in buttons",
	"POST /api/v1/auth/login":                                                        "signs in to an existing account; creates nothing",
	"POST /api/v1/auth/logout":                                                       "ends a session",
	"POST /api/v1/auth/signout":                                                      "ends a session",
	"POST /api/v1/auth/refresh":                                                      "keeps the demo session alive",
	"POST /api/v1/staff/logout":                                                      "ends a staff session",
	"POST /api/v1/crm/login":                                                         "signs in an existing CRM customer; signup is refused",
	"POST /api/v1/customer/connect-business":                                         "links an existing CRM customer to the venue; no account or credential",
	"POST /api/v1/ai-waiter/:businessId/session":                                     "guest AI chat session (text AI is part of the showroom)",
	"POST /api/v1/inside/businesses/:id/ai/test-chat/session":                        "AI waiter preview chat",
	"POST /api/v1/inside/businesses/:id/cash-register/sessions":                      "cash drawer shift, not a login session",
	"POST /api/v1/inside/businesses/:id/cash-register/sessions/:sessionId/close":     "cash drawer shift",
	"POST /api/v1/inside/businesses/:id/cash-register/sessions/:sessionId/movements": "cash drawer shift",
	"POST /api/v1/inside/businesses/:id/spaces/:spaceId/scan-sessions":               "floor-plan scan session; its uploads are refused",
	"POST /api/v1/inside/businesses/:id/scan-sessions/:sessionId/cancel":             "floor-plan scan session",
	"POST /api/v1/inside/businesses/:id/scan-sessions/:sessionId/complete":           "floor-plan scan session",
	"POST /api/v1/inside/businesses/:id/scan-sessions/:sessionId/retry-process":      "floor-plan scan session",
	"POST /api/v1/space-scan/:token/connect":                                         "pairs the scanning phone; uploads are refused",
	"POST /api/v1/space-scan/:token/status":                                          "phone reports scan progress on its own token; no account or credential",
	"POST /api/v1/space-scan/:token/apply-review":                                    "applies a finished scan to the demo floor plan, same as editing tables; scans cannot finish because uploads are refused",
	"POST /api/v1/inside/businesses/:id/staff/:staffId/positions":                    "staff job positions, not access",
	"DELETE /api/v1/inside/businesses/:id/staff/:staffId/positions/:positionId":      "staff job positions, not access",
	"PUT /api/v1/inside/businesses/:id/staff/:staffId/positions/:positionId/rate":    "staff pay rate, not access",
	"PUT /api/v1/inside/businesses/:id/staff/:staffId/compensation":                  "staff pay, not access",
	"POST /api/v1/inside/businesses/:id/ai/director/threads/:threadId/pin":           "pins an AI chat thread",
	"POST /api/v1/inside/businesses/:id/ai/director/threads/:threadId/unpin":         "pins an AI chat thread",
	"DELETE /api/v1/inside/push-subscriptions/:subscriptionId":                       "removes a push subscription; adding one is refused",
	"POST /api/v1/inside/bills/:bill_id/refund":                                      "records a refund on demo bills; moves no money (on-chain refunds are refused)",
	"POST /api/v1/inside/businesses/:id/documents/:docId/ack":                        "acknowledges a document; no upload",
}

// TestDemoGuardDefaultDeniesSensitiveGroups walks every write route main.go
// registers and fails when a route in a sensitive group (accounts and
// sessions, staff access, integrations, wallets, outbound endpoints, uploads,
// admin, fiscal, subscriptions) is neither refused by the demo guard nor in
// the reviewed allowlist above. A new route in one of those groups therefore
// fails here until it gets a rule or a reviewed exemption.
func TestDemoGuardDefaultDeniesSensitiveGroups(t *testing.T) {
	routes := parseAllRouteInventory(t)
	keys := make([]string, 0, len(routes))
	for k := range routes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	classified := 0
	for _, key := range keys {
		method, path, _ := strings.Cut(key, " ")
		if method == "GET" {
			continue
		}
		groups := demoSensitiveGroups(path)
		if len(groups) == 0 {
			continue
		}
		classified++
		_, refused := demomode.Match(method, path)
		_, reviewed := demoReviewedSensitiveWrites[key]
		if refused && reviewed {
			t.Errorf("%s is both refused by the guard and listed as a reviewed demo write; drop one", key)
		}
		if !refused && !reviewed {
			t.Errorf("%s is in sensitive group(s) %v but the demo guard does not refuse it; add a demomode rule or a reviewed entry", key, groups)
		}
	}
	if classified < 100 {
		t.Fatalf("only %d sensitive write routes classified; the inventory or the classifier is broken", classified)
	}
	for key := range demoReviewedSensitiveWrites {
		if !routes[key] {
			t.Errorf("reviewed demo write %s is not a route main.go registers", key)
		}
	}
}
