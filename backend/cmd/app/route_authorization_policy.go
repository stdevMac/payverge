package main

// routeTenantScope documents how a protected route binds the authenticated
// actor to the resource it reads or mutates. Most tenant routes carry
// /businesses/:id and use RBAC middleware. Routes that resolve a tenant from a
// bill, staff record, request body, or the authenticated principal must be
// explicitly declared in handlerAuthorizedProtectedRoutes so a new route
// cannot silently rely on an undocumented handler shortcut.
type routeTenantScope string

const (
	routeTenantBusinessPath routeTenantScope = "business_path"
	routeTenantResolved     routeTenantScope = "resolved_resource"
	routePrincipalSelf      routeTenantScope = "principal_self"
	routePlatformGlobal     routeTenantScope = "platform_global"
)

type handlerAuthorizedRoutePolicy struct {
	domain   string
	scope    routeTenantScope
	resolver string
	reason   string
}

// Keeping this declaration in production package source makes handler-owned
// authorization reviewable in one place even though the CI gate is the only
// consumer today. Entries are exceptions to route-level RBAC, not exceptions
// to authorization: every resolved-resource mutation names the handler guard
// that binds its target to the authenticated principal or tenant.
var handlerAuthorizedProtectedRoutes = map[string]handlerAuthorizedRoutePolicy{
	// Authenticated-principal profile and privacy operations.
	"GET /api/v1/inside/get_user/:address": {
		domain: "users", scope: routePrincipalSelf, resolver: "GetUser caller-address self check", reason: "A user may read only their own record; platform administrators are checked in the handler.",
	},
	"PUT /api/v1/inside/update_user": {
		domain: "users", scope: routePrincipalSelf, resolver: "authenticated address from context", reason: "Profile updates resolve the user exclusively from the authenticated address.",
	},
	"PUT /api/v1/inside/set_language": {
		domain: "users", scope: routePrincipalSelf, resolver: "authenticated address or email from context", reason: "The compatibility body address is ignored and language is written for the authenticated principal.",
	},
	"POST /api/v1/inside/account/export": {
		domain: "privacy", scope: routePrincipalSelf, resolver: "resolveAccountUser", reason: "The export contains only the authenticated account's data.",
	},
	"POST /api/v1/inside/account/delete": {
		domain: "privacy", scope: routePrincipalSelf, resolver: "resolveAccountUser", reason: "Soft deletion targets only the authenticated account.",
	},
	"GET /api/v1/inside/settings/notifications": {
		domain: "users", scope: routePrincipalSelf, resolver: "resolveNotificationUser", reason: "Preferences are loaded from the authenticated user ID or wallet address.",
	},
	"PUT /api/v1/inside/settings/notifications": {
		domain: "users", scope: routePrincipalSelf, resolver: "resolveNotificationUser", reason: "Preferences are written for the authenticated user ID or wallet address.",
	},

	// Business creation and discovery operations that do not act on an existing
	// tenant, plus canonical principal-owned business reads.
	"POST /api/v1/inside/businesses/generate-id": {
		domain: "businesses", scope: routePlatformGlobal, resolver: "authenticated principal context", reason: "The generated slug candidate does not read or mutate an existing tenant.",
	},
	"POST /api/v1/inside/businesses": {
		domain: "businesses", scope: routePrincipalSelf, resolver: "authenticated user ID or wallet owner", reason: "Creation assigns canonical ownership from authentication context, never request ownership fields.",
	},
	"GET /api/v1/inside/businesses": {
		domain: "businesses", scope: routePrincipalSelf, resolver: "ListBusinessesForInsideUser or GetBusinessByOwnerAddress", reason: "The list is selected by the authenticated canonical user or wallet owner.",
	},
	"GET /api/v1/inside/businesses/:id": {
		domain: "businesses", scope: routeTenantResolved, resolver: "CheckBusinessOwnership and CheckBusinessAccess", reason: "Owners and staff receive role-appropriate records; unaffiliated callers receive only the public projection.",
	},
	"GET /api/v1/inside/businesses/check-url": {
		domain: "businesses", scope: routePlatformGlobal, resolver: "availability-only projection", reason: "The endpoint returns slug availability and does not expose or mutate a tenant record.",
	},
	"POST /api/v1/inside/google/businesses/search": {
		domain: "settings", scope: routePlatformGlobal, resolver: "Google Places search projection", reason: "Search is authenticated but is not associated with an existing tenant.",
	},

	// Tenant paths whose handlers resolve canonical membership because they
	// intentionally remain available before paid-plan activation.
	"POST /api/v1/inside/businesses/:id/onboarding-state/complete": {
		domain: "onboarding", scope: routeTenantBusinessPath, resolver: "requireBusinessAccess", reason: "Completion is a pre-activation setup mutation bound to canonical business membership.",
	},
	"GET /api/v1/inside/businesses/:id/setup-status": {
		domain: "onboarding", scope: routeTenantBusinessPath, resolver: "requireBusinessAccess", reason: "Setup state is available before activation but only to canonical business members.",
	},

	// Body/query/resource-resolved tenant operations. Route RBAC cannot consume
	// these business IDs, so the handler performs membership and permission
	// checks after resolving the canonical business.
	"GET /api/v1/inside/translations": {
		domain: "translations", scope: routeTenantResolved, resolver: "CurrencyHandler.authorizeBusinessAccess", reason: "The query business ID is checked for membership and menu:read.",
	},
	"PUT /api/v1/inside/translations": {
		domain: "translations", scope: routeTenantResolved, resolver: "CurrencyHandler.authorizeBusinessMutation", reason: "The body business ID is checked for the admin lifecycle lock, membership, and menu:translate.",
	},
	"GET /api/v1/inside/menu-translations": {
		domain: "translations", scope: routeTenantResolved, resolver: "CurrencyHandler.authorizeBusinessAccess", reason: "The query business ID is checked for membership and menu:read.",
	},
	"GET /api/v1/inside/translation-jobs/:jobId/status": {
		domain: "translations", scope: routeTenantResolved, resolver: "translation job business plus CheckBusinessAccess", reason: "The opaque job ID is resolved to its business before status is returned.",
	},
	"POST /api/v1/inside/staff/:staff_id/pin": {
		domain: "staff", scope: routeTenantResolved, resolver: "target staff business plus owner-or-self check", reason: "Owners may set tenant staff PINs and staff may rotate only their own PIN.",
	},
	"POST /api/v1/inside/push-subscriptions": {
		domain: "notifications", scope: routeTenantResolved, resolver: "resolvePushPrincipal plus CheckBusinessAccess", reason: "The body business is checked against the authenticated owner or staff principal before upsert.",
	},

	// Notification routes are self-scoped rather than
	// permission-scoped. Their database predicates include the authenticated
	// principal so another principal's rows cannot be read or mutated.
	"GET /api/v1/inside/businesses/:id/me/notifications": {
		domain: "notifications", scope: routePrincipalSelf, resolver: "business ID plus staffIDFromContext", reason: "The query predicates bind inbox rows to both the route business and authenticated staff ID.",
	},
	"GET /api/v1/inside/businesses/:id/me/notifications/unread-count": {
		domain: "notifications", scope: routePrincipalSelf, resolver: "business ID plus staffIDFromContext", reason: "The count predicate binds rows to both the route business and authenticated staff ID.",
	},
	"POST /api/v1/inside/businesses/:id/me/notifications/read": {
		domain: "notifications", scope: routePrincipalSelf, resolver: "business ID plus staffIDFromContext", reason: "The update predicate binds rows to both the route business and authenticated staff ID.",
	},
	"GET /api/v1/inside/businesses/:id/me/engagement-badges": {
		domain: "engagement", scope: routePrincipalSelf, resolver: "business ID plus staffIDFromContext", reason: "Badge counts are selected for the route business and authenticated staff ID.",
	},
	"GET /api/v1/inside/push-subscriptions": {
		domain: "notifications", scope: routePrincipalSelf, resolver: "resolvePushPrincipal", reason: "Only subscriptions matching the authenticated principal type and ID are returned.",
	},
	"DELETE /api/v1/inside/push-subscriptions/:subscriptionId": {
		domain: "notifications", scope: routePrincipalSelf, resolver: "resolvePushPrincipal", reason: "Deletion requires both the subscription ID and authenticated principal tuple.",
	},

	// Authenticated global catalogs contain no tenant-confidential state.
	"GET /api/v1/inside/plugins": {
		domain: "plugins", scope: routePlatformGlobal, resolver: "active plugin catalog projection", reason: "The route lists platform plugin metadata and does not mutate tenant configuration.",
	},
}

// handlerOperationalGatedMutations documents tenant mutations whose business
// is resolved from a body or opaque resource rather than a canonical :id route.
// Their handlers must apply the same 403 business_suspended/business_closed
// contract as RequireOperationalBusiness after authorization resolves the tenant.
var handlerOperationalGatedMutations = map[string]string{
	"PUT /api/v1/inside/translations":        "CurrencyHandler.authorizeBusinessMutation",
	"POST /api/v1/inside/push-subscriptions": "CreatePushSubscription active business check",
}
