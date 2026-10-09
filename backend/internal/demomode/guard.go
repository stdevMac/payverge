// Package demomode enforces the public-demo boundary (DEMO_MODE=true).
//
// A public demo hands everyone a session on the same seeded showroom, so the
// guard refuses every action that reaches outside the box (email, messaging,
// payment providers, fiscal authorities, printers, user-supplied URLs), that
// changes who controls an account (passwords, invites, tokens, venue deletion)
// or that stores visitor files. Everything else — menu, tables, orders,
// bills, kitchen, reservations, CRM, accounting — stays writable, because
// the nightly reset (deploy/demo/reset-demo.sh) restores the pristine seed.
//
// The guard is one global gin middleware. It runs after route matching, so it
// keys on the registered route pattern (c.FullPath()), never on the raw URL;
// cmd/app/demo_guard_routes_test.go proves every rule names a route that is
// really wired. It is a no-op unless config.DemoModeEnabled().
package demomode

import (
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/config"
)

// ErrorCode is the stable error code of a refused demo action.
const ErrorCode = "DEMO_MODE_FORBIDDEN"

// RateLimitedCode is the stable error code when a visitor exceeds the demo
// write budget.
const RateLimitedCode = "DEMO_MODE_RATE_LIMITED"

// Kind groups rules so the refusal message can say why.
type Kind string

const (
	KindAccount     Kind = "account"
	KindStaffAccess Kind = "staff_access"
	KindIntegration Kind = "integration"
	KindPayment     Kind = "payment"
	KindOutbound    Kind = "outbound"
	KindUpload      Kind = "upload"
	KindAdmin       Kind = "admin"
	// KindStorefront: the venue's public identity (name, logo, links, payout
	// addresses). Refused field by field in UpdateBusiness, not by route.
	KindStorefront Kind = "storefront"
	// KindAI: AI features the demo turns off even when DEMO_AI_API_KEY is set
	// (image generation). Text AI stays on under its own budget.
	KindAI Kind = "ai"
	// KindSize: a write body over MaxWriteBodyBytes.
	KindSize Kind = "size"
)

// MaxStorefrontTextRunes caps every storefront free text a visitor can write
// in the public demo (description, welcome message, about story, special
// features, AI house instructions, manual translations of them), so the shared
// showroom cannot become a long-form billboard until the nightly reset.
const MaxStorefrontTextRunes = 500

// StorefrontTextAction names the refusal for text over MaxStorefrontTextRunes.
var StorefrontTextAction = "storefront texts longer than " + strconv.Itoa(MaxStorefrontTextRunes) + " characters"

// TextTooLong reports whether s is over MaxStorefrontTextRunes.
func TextTooLong(s string) bool {
	return utf8.RuneCountInString(s) > MaxStorefrontTextRunes
}

// MaxWriteBodyBytes caps the body of every write in the public demo. It bounds
// the JSON blobs no field-level cap covers (menus, settings, layouts) by total
// size. Uploads are refused outright, so no showroom write comes close.
const MaxWriteBodyBytes int64 = 256 << 10

// Rule refuses one route (Method + gin pattern) or, with Prefix, every route
// whose pattern starts with Path. Method "*" matches every state-changing
// method (POST, PUT, PATCH, DELETE); reads are never refused.
type Rule struct {
	Method string
	Path   string
	Prefix bool
	Kind   Kind
	Action string
}

const (
	biz      = "/api/v1/inside/businesses/:id"
	anyWrite = "*"
)

// Rules is the full deny list. Keep it in step with
// docs/self-hosting/public-demo.md ("What the demo refuses").
var Rules = []Rule{
	// --- Accounts and identity ------------------------------------------
	{Method: http.MethodPost, Path: "/api/v1/auth/register", Kind: KindAccount, Action: "creating accounts"},
	{Method: http.MethodPost, Path: "/api/v1/auth/password/reset-request", Kind: KindAccount, Action: "changing passwords"},
	{Method: http.MethodPost, Path: "/api/v1/auth/password/reset", Kind: KindAccount, Action: "changing passwords"},
	{Method: http.MethodPost, Path: "/api/v1/auth/email/resend-verification", Kind: KindAccount, Action: "changing email addresses"},
	{Method: http.MethodPost, Path: "/api/v1/auth/wallet/link", Kind: KindAccount, Action: "linking wallets"},
	{Method: http.MethodDelete, Path: "/api/v1/auth/wallet/unlink", Kind: KindAccount, Action: "unlinking wallets"},
	{Method: http.MethodPut, Path: "/api/v1/inside/update_user", Kind: KindAccount, Action: "editing the demo account profile"},
	{Method: http.MethodPost, Path: "/api/v1/inside/account/delete", Kind: KindAccount, Action: "deleting the account"},
	{Method: http.MethodPost, Path: "/api/v1/inside/account/export", Kind: KindAccount, Action: "exporting account data"},
	{Method: http.MethodPost, Path: "/api/v1/inside/businesses", Kind: KindAccount, Action: "creating venues"},
	{Method: http.MethodPost, Path: "/api/v1/inside/businesses/generate-id", Kind: KindAccount, Action: "creating venues"},
	{Method: http.MethodDelete, Path: biz, Kind: KindAccount, Action: "deleting the venue"},
	{Method: http.MethodPost, Path: "/api/v1/customer/link-wallet", Kind: KindAccount, Action: "linking wallets"},
	{Method: http.MethodDelete, Path: "/api/v1/customer/account", Kind: KindAccount, Action: "deleting the account"},
	{Method: http.MethodPost, Path: "/api/v1/crm/register", Kind: KindAccount, Action: "creating customer accounts"},

	// --- Staff access, invites and tokens -------------------------------
	{Method: http.MethodPost, Path: biz + "/staff/invite", Kind: KindStaffAccess, Action: "inviting users"},
	{Method: http.MethodPost, Path: biz + "/staff/invitations/:invitationId/resend", Kind: KindStaffAccess, Action: "inviting users"},
	{Method: http.MethodPost, Path: biz + "/staff/invitations/:invitationId/revoke", Kind: KindStaffAccess, Action: "managing invitations"},
	{Method: http.MethodPost, Path: biz + "/staff/:staffId/deactivate", Kind: KindStaffAccess, Action: "removing staff access"},
	{Method: http.MethodPost, Path: biz + "/staff/:staffId/reactivate", Kind: KindStaffAccess, Action: "changing staff access"},
	{Method: http.MethodDelete, Path: biz + "/staff/:staffId", Kind: KindStaffAccess, Action: "removing staff access"},
	{Method: http.MethodPut, Path: biz + "/staff/:staffId/role", Kind: KindStaffAccess, Action: "changing staff roles"},
	{Method: http.MethodPost, Path: biz + "/staff/:staffId/permissions", Kind: KindStaffAccess, Action: "changing staff permissions"},
	{Method: http.MethodDelete, Path: biz + "/staff/:staffId/permissions", Kind: KindStaffAccess, Action: "changing staff permissions"},
	{Method: http.MethodPost, Path: biz + "/staff/:staffId/permission-denies", Kind: KindStaffAccess, Action: "changing staff permissions"},
	{Method: http.MethodDelete, Path: biz + "/staff/:staffId/permission-denies", Kind: KindStaffAccess, Action: "changing staff permissions"},
	{Method: http.MethodPost, Path: "/api/v1/inside/staff/:staff_id/pin", Kind: KindStaffAccess, Action: "changing staff PINs"},
	{Method: http.MethodPost, Path: "/api/v1/staff/request-login-code", Kind: KindStaffAccess, Action: "staff email sign-in (use the demo buttons)"},
	{Method: http.MethodPost, Path: "/api/v1/staff/verify-login-code", Kind: KindStaffAccess, Action: "staff email sign-in (use the demo buttons)"},
	{Method: http.MethodPost, Path: "/api/v1/staff/accept-invitation", Kind: KindStaffAccess, Action: "accepting invitations"},

	// --- Integrations and their credentials -----------------------------
	{Method: anyWrite, Path: biz + "/plugins/", Prefix: true, Kind: KindIntegration, Action: "changing integrations and their credentials"},
	{Method: http.MethodPost, Path: biz + "/whatsapp/connect", Kind: KindIntegration, Action: "connecting WhatsApp"},
	{Method: http.MethodPost, Path: biz + "/whatsapp/disconnect", Kind: KindIntegration, Action: "connecting WhatsApp"},
	{Method: http.MethodPut, Path: biz + "/google", Kind: KindIntegration, Action: "connecting Google Business"},
	{Method: http.MethodDelete, Path: biz + "/google", Kind: KindIntegration, Action: "connecting Google Business"},
	{Method: http.MethodPost, Path: "/api/v1/inside/google/businesses/search", Kind: KindIntegration, Action: "searching Google Business"},
	{Method: http.MethodPost, Path: biz + "/printers", Kind: KindIntegration, Action: "adding printers"},
	{Method: http.MethodPatch, Path: biz + "/printers/:printerId", Kind: KindIntegration, Action: "changing printers"},
	{Method: http.MethodDelete, Path: biz + "/printers/:printerId", Kind: KindIntegration, Action: "removing printers"},
	{Method: http.MethodPost, Path: biz + "/printers/:printerId/test", Kind: KindIntegration, Action: "test prints"},
	{Method: anyWrite, Path: biz + "/fiscal/", Prefix: true, Kind: KindIntegration, Action: "fiscal (ARCA) e-invoicing"},
	{Method: http.MethodPut, Path: "/api/v1/inside/bills/:bill_id/fiscal-customer", Kind: KindIntegration, Action: "fiscal (ARCA) e-invoicing"},
	{Method: http.MethodPost, Path: "/api/v1/guest/bill/:bill_token/fiscal-customer", Kind: KindIntegration, Action: "fiscal (ARCA) e-invoicing"},

	// --- Payments: cash and card-at-table only --------------------------
	{Method: http.MethodPost, Path: biz + "/bills/:bill_id/mercadopago/point/charge", Kind: KindPayment, Action: "card terminal charges"},
	{Method: http.MethodPost, Path: biz + "/bills/:bill_id/mercadopago/qr/charge", Kind: KindPayment, Action: "QR charges"},
	{Method: http.MethodPost, Path: biz + "/mercadopago/orders/:order_id/cancel", Kind: KindPayment, Action: "card terminal charges"},
	{Method: http.MethodPatch, Path: biz + "/mercadopago/terminals/:terminal_id/mode", Kind: KindPayment, Action: "card terminal setup"},
	{Method: anyWrite, Path: biz + "/crypto-refunds", Prefix: true, Kind: KindPayment, Action: "on-chain refunds"},
	{Method: http.MethodPost, Path: "/api/v1/guest/bill/:bill_token/crypto-payment", Kind: KindPayment, Action: "crypto payments"},
	{Method: http.MethodPost, Path: "/api/v1/guest/bill/:bill_token/crypto-quote", Kind: KindPayment, Action: "crypto payments"},
	{Method: http.MethodPost, Path: "/api/v1/guest/bill/:bill_token/cross-chain-payment", Kind: KindPayment, Action: "crypto payments"},
	{Method: http.MethodPost, Path: "/api/v1/guest/bill/:bill_token/plugin-payment", Kind: KindPayment, Action: "online card payments (pay cash or card at the table instead)"},

	// --- Outbound messages and user-supplied endpoints ------------------
	{Method: http.MethodPost, Path: "/api/v1/guest/bill/:bill_token/email-receipt", Kind: KindOutbound, Action: "emailing receipts"},
	{Method: http.MethodPost, Path: "/api/v1/inside/push-subscriptions", Kind: KindOutbound, Action: "push notifications"},
	{Method: http.MethodPost, Path: "/api/v1/email/unsubscribe", Kind: KindOutbound, Action: "email preferences"},
	{Method: anyWrite, Path: "/api/v1/webhooks/", Prefix: true, Kind: KindOutbound, Action: "provider webhooks"},

	// --- Uploads ---------------------------------------------------------
	{Method: http.MethodPost, Path: biz + "/uploads", Kind: KindUpload, Action: "uploads"},
	{Method: http.MethodPost, Path: biz + "/uploads/logo", Kind: KindUpload, Action: "uploads"},
	{Method: http.MethodPost, Path: biz + "/uploads/protected", Kind: KindUpload, Action: "uploads"},
	{Method: http.MethodDelete, Path: biz + "/uploads", Kind: KindUpload, Action: "uploads"},
	{Method: http.MethodPost, Path: biz + "/accounting/entries/:entryId/attachments", Kind: KindUpload, Action: "uploads"},
	{Method: http.MethodDelete, Path: biz + "/accounting/entries/:entryId/attachments/:attachmentId", Kind: KindUpload, Action: "uploads"},
	{Method: http.MethodPost, Path: biz + "/documents", Kind: KindUpload, Action: "uploads"},
	{Method: http.MethodPut, Path: biz + "/documents/:docId", Kind: KindUpload, Action: "uploads"},
	{Method: http.MethodPost, Path: biz + "/ai/extract-menu/upload/:jobId", Kind: KindUpload, Action: "uploads"},
	{Method: http.MethodPost, Path: "/api/v1/space-scan/:token/uploads", Kind: KindUpload, Action: "uploads"},
	{Method: http.MethodPost, Path: "/api/v1/space-scan/:token/complete-upload", Kind: KindUpload, Action: "uploads"},

	// --- Storefront ------------------------------------------------------
	// Gallery rows are outbound image URLs on the public storefront. The
	// identity fields of PUT /businesses/:id are refused in the handler
	// (body-level allowlist), not here.
	{Method: http.MethodPut, Path: biz + "/gallery-images", Kind: KindStorefront, Action: "changing the venue's gallery images"},

	// --- AI image generation (text AI stays on under its budget) --------
	{Method: http.MethodPost, Path: biz + "/ai/regenerate-image", Kind: KindAI, Action: "AI image generation"},
	{Method: http.MethodPost, Path: biz + "/ai/enhance-image", Kind: KindAI, Action: "AI image generation"},
	{Method: http.MethodPost, Path: biz + "/generate-menu-image", Kind: KindAI, Action: "AI image generation"},
	{Method: http.MethodPost, Path: biz + "/marketing/image", Kind: KindAI, Action: "AI image generation"},
	{Method: http.MethodPost, Path: biz + "/marketing/image/cleanup", Kind: KindAI, Action: "AI image generation"},

	// --- Platform admin writes (the admin uses `server admin ...`) ------
	{Method: anyWrite, Path: "/api/v1/admin/plugins", Prefix: true, Kind: KindAdmin, Action: "platform plugin settings"},
	{Method: anyWrite, Path: "/api/v1/admin/emails/", Prefix: true, Kind: KindAdmin, Action: "platform emails"},
	{Method: anyWrite, Path: "/api/v1/admin/users/", Prefix: true, Kind: KindAdmin, Action: "managing platform users"},
	{Method: anyWrite, Path: "/api/v1/admin/runtime-controls/invite-batches", Kind: KindAdmin, Action: "minting signup invites"},

	// --- Default deny for whole sensitive groups -------------------------
	// These come last so the specific rules above name the action. Anything
	// under them is refused unless Exempt lists it, so a new auth or admin
	// route is closed in the demo until someone reviews it.
	{Method: anyWrite, Path: "/api/v1/auth/", Prefix: true, Kind: KindAccount, Action: "signing up or changing sign-in methods (use the demo buttons)"},
	{Method: anyWrite, Path: "/api/v1/admin/", Prefix: true, Kind: KindAdmin, Action: "platform administration"},
}

// Exempt lists the reviewed writes inside a default-deny group that the demo
// needs: signing in with the demo buttons and keeping or ending a session.
// They sign in to accounts that already exist and create nothing. SIWE
// wallet sign-in stays refused because it creates a user on first use.
var Exempt = []Route{
	{Method: http.MethodPost, Path: "/api/v1/auth/demo/login"},
	{Method: http.MethodPost, Path: "/api/v1/auth/login"},
	{Method: http.MethodPost, Path: "/api/v1/auth/logout"},
	{Method: http.MethodPost, Path: "/api/v1/auth/signout"},
	{Method: http.MethodPost, Path: "/api/v1/auth/refresh"},
}

// Route is one method + gin route pattern.
type Route struct {
	Method string
	Path   string
}

func exempt(method, fullPath string) bool {
	for _, r := range Exempt {
		if r.Method == method && r.Path == fullPath {
			return true
		}
	}
	return false
}

// Match returns the rule refusing method + route pattern, if any.
func Match(method, fullPath string) (Rule, bool) {
	if fullPath == "" || !isWrite(method) || exempt(method, fullPath) {
		return Rule{}, false
	}
	for _, r := range Rules {
		if r.Method != anyWrite && r.Method != method {
			continue
		}
		if r.Prefix {
			if strings.HasPrefix(fullPath, r.Path) {
				return r, true
			}
			continue
		}
		if fullPath == r.Path {
			return r, true
		}
	}
	return Rule{}, false
}

func isWrite(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// Message is the refusal copy for one rule.
func Message(r Rule) string {
	if r.Kind == KindUpload {
		return "Uploads are disabled in the public demo. Install your own Payverge to upload files."
	}
	return "Disabled in the public demo: " + r.Action + ". Install your own Payverge to use it — everything else here works, and the demo resets nightly."
}

func abort(c *gin.Context, rule Rule) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
		"error": Message(rule),
		"code":  ErrorCode,
		"params": gin.H{
			"reason": "demo_mode",
			"kind":   string(rule.Kind),
		},
	})
}

// Refuse writes the standard demo refusal (403, DEMO_MODE_FORBIDDEN) for a
// check a handler makes itself, when what is refused depends on the request
// body rather than the route (for example the storefront identity fields of
// UpdateBusiness). Callers check config.DemoModeEnabled() first.
func Refuse(c *gin.Context, kind Kind, action string) {
	abort(c, Rule{Kind: kind, Action: action})
}

// Guard is the global DEMO_MODE middleware: noindex on every response, the
// deny list above, and a per-IP write budget.
func Guard() gin.HandlerFunc {
	limiter := newWriteLimiter()
	return func(c *gin.Context) {
		if !config.DemoModeEnabled() {
			c.Next()
			return
		}
		c.Header("X-Robots-Tag", "noindex, nofollow")
		if rule, ok := Match(c.Request.Method, c.FullPath()); ok {
			abort(c, rule)
			return
		}
		if isWrite(c.Request.Method) {
			if c.Request.ContentLength > MaxWriteBodyBytes {
				abort(c, Rule{Kind: KindSize, Action: "requests larger than " + strconv.FormatInt(MaxWriteBodyBytes>>10, 10) + " KB"})
				return
			}
			// Chunked bodies have no Content-Length; the reader stops them at
			// the same size and the handler's bind fails.
			if c.Request.Body != nil {
				c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxWriteBodyBytes)
			}
		}
		if isWrite(c.Request.Method) && !limiter.allow(c.ClientIP()) {
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "The public demo limits how fast one visitor can make changes. Wait a minute and try again.",
				"code":  RateLimitedCode,
			})
			return
		}
		c.Next()
	}
}
