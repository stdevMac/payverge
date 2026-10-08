package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/structs"

	"github.com/gin-gonic/gin"
)

// Permission represents a granular permission in the system
type Permission string

const ContextAllowUserLevelPermissions = "allow_user_level_permissions"
const ContextAuthorizedAnyPermissions = "authorized_any_permissions"

// Permission categories for all dashboard modules and operations
const (
	// Business Management
	PermBusinessRead     Permission = "business:read"
	PermBusinessWrite    Permission = "business:write"
	PermBusinessDelete   Permission = "business:delete"
	PermBusinessSettings Permission = "business:settings"

	// Staff Management
	PermStaffRead        Permission = "staff:read"
	PermStaffWrite       Permission = "staff:write"
	PermStaffInvite      Permission = "staff:invite"
	PermStaffRemove      Permission = "staff:remove"
	PermStaffRoles       Permission = "staff:roles"
	PermStaffDelete      Permission = "staff:delete"
	PermStaffRole        Permission = "staff:role"
	PermStaffPermissions Permission = "staff:permissions"
	PermStaffDeactivate  Permission = "staff:deactivate"
	PermStaffReactivate  Permission = "staff:reactivate"
	PermStaffAudit       Permission = "staff:audit"
	PermStaffAdmin       Permission = "staff:admin" // Full staff management

	// Menu Management
	PermMenuRead       Permission = "menu:read"
	PermMenuWrite      Permission = "menu:write"
	PermMenuTranslate  Permission = "menu:translate"
	PermMenuCategories Permission = "menu:categories"
	PermMenuItems      Permission = "menu:items"

	// Table Management
	PermTablesRead   Permission = "tables:read"
	PermTablesWrite  Permission = "tables:write"
	PermTablesCreate Permission = "tables:create"
	PermTablesDelete Permission = "tables:delete"
	PermTablesQR     Permission = "tables:qr"

	// Bill Management
	PermBillsRead   Permission = "bills:read"
	PermBillsWrite  Permission = "bills:write"
	PermBillsCreate Permission = "bills:create"
	PermBillsClose  Permission = "bills:close"
	PermBillsItems  Permission = "bills:items"
	// PermBillsPayment gates recording in-person tender (cash, card, venmo, other)
	// via alternative_payments. Granted to manager + server by default so floor
	// staff can settle tabs without owner wallet access. Void/refund stays on
	// the owner-only bills:refund permission.
	PermBillsPayment Permission = "bills:payment"
	// PermBillsRefund gates voiding/refunding a bill (cancelling or reversing
	// money). It is owner-only and granted to NO staff role — the same trust bar
	// as payroll:write / fiscal:credit — matching the handler's existing
	// owner-only authorization (authorizeBillManagement). Owners/platform admins
	// bypass permission checks; an owner may grant it to a specific manager via
	// custom perms. Day-to-day bill ops (bills:close) remain with staff.
	PermBillsRefund Permission = "bills:refund"

	// Wave 4 REFUND track — noncustodial on-chain crypto refunds. Owner-only by
	// default (absent from every staff role). Request creates a durable
	// payment_refunds row; approve advances to awaiting_signature. Both are
	// money-moving and must never be silently granted to floor staff.
	PermRefundsCryptoRequest Permission = "refunds:crypto:request"
	PermRefundsCryptoApprove Permission = "refunds:crypto:approve"

	// Order Management
	PermOrdersRead    Permission = "orders:read"
	PermOrdersWrite   Permission = "orders:write"
	PermOrdersCreate  Permission = "orders:create"
	PermOrdersStatus  Permission = "orders:status"
	PermOrdersKitchen Permission = "orders:kitchen"

	// Analytics & Reporting
	PermAnalyticsRead  Permission = "analytics:read"
	PermAnalyticsSales Permission = "analytics:sales"
	PermAnalyticsTips  Permission = "analytics:tips"
	PermAnalyticsItems Permission = "analytics:items"
	PermReportsRead    Permission = "reports:read"
	PermReportsExport  Permission = "reports:export"

	// Financial Operations (Owner/Admin only)
	PermFinancialRead       Permission = "financial:read"
	PermFinancialWrite      Permission = "financial:write"
	PermFinancialWithdraw   Permission = "financial:withdraw"
	PermFinancialBlockchain Permission = "financial:blockchain"

	// Payroll writes (create / mark-paid / delete-draft / void) are owner-only by
	// default and granted to NO staff role — compensation data is sensitive and
	// self-dealing (a manager paying themselves) is a separation-of-duties risk.
	// An owner may still delegate it to a specific manager via custom permissions.
	// Payroll READS stay on financial:read so managers retain visibility.
	PermPayrollWrite Permission = "payroll:write"

	// Plugin Management
	PermPluginsRead     Permission = "plugins:read"
	PermPluginsWrite    Permission = "plugins:write"
	PermPluginsConfig   Permission = "plugins:config"
	PermPluginsPayments Permission = "plugins:payments"

	// Delivery Operations (3 families × {read, write})
	PermDeliverySettingsRead  Permission = "delivery:settings:read"
	PermDeliverySettingsWrite Permission = "delivery:settings:write"
	PermDeliveryDispatchRead  Permission = "delivery:dispatch:read"
	PermDeliveryDispatchWrite Permission = "delivery:dispatch:write"
	PermDeliveryDriversRead   Permission = "delivery:drivers:read"
	PermDeliveryDriversWrite  Permission = "delivery:drivers:write"

	// Settings & Configuration
	PermSettingsRead     Permission = "settings:read"
	PermSettingsWrite    Permission = "settings:write"
	PermSettingsDesign   Permission = "settings:design"
	PermSettingsGoogle   Permission = "settings:google"
	PermSettingsCurrency Permission = "settings:currency"
	PermSettingsLanguage Permission = "settings:language"
	PermCurrenciesRead   Permission = "currencies:read"
	PermCurrenciesWrite  Permission = "currencies:write"
	PermLanguagesRead    Permission = "languages:read"
	PermLanguagesWrite   Permission = "languages:write"

	// Tenant-scoped object storage mutations. Managers receive these by default;
	// other staff may receive an explicit custom grant. Owners bypass staff RBAC.
	PermFilesUpload Permission = "files:upload"
	PermFilesDelete Permission = "files:delete"

	// Counter Operations
	PermCounterRead     Permission = "counter:read"
	PermCounterWrite    Permission = "counter:write"
	PermCounterSettings Permission = "counter:settings"

	// Overview Dashboard
	PermOverviewRead Permission = "overview:read"
	PermOverviewKPI  Permission = "overview:kpi"

	// CRM (Customer Relationship Management)
	PermCRMRead   Permission = "crm:read"
	PermCRMWrite  Permission = "crm:write"
	PermCRMExport Permission = "crm:export"

	// Inventory Management
	PermInventoryRead    Permission = "inventory:read"
	PermInventoryWrite   Permission = "inventory:write"
	PermInventoryAdjust  Permission = "inventory:adjust"
	PermInventoryRecipes Permission = "inventory:recipes"

	// Reservations Management
	PermReservationsRead     Permission = "reservations:read"
	PermReservationsWrite    Permission = "reservations:write"
	PermReservationsCreate   Permission = "reservations:create"
	PermReservationsDelete   Permission = "reservations:delete"
	PermReservationsSettings Permission = "reservations:settings"

	// Director Console (owner-only by default; not granted to staff roles)
	PermDirectorRead  Permission = "director:read"
	PermDirectorWrite Permission = "director:write"

	// Ops Assistant (dashboard help)
	PermAssistantRead  Permission = "assistant:read"
	PermAssistantWrite Permission = "assistant:write"

	// AI Waiter
	PermAIWaiterRead     Permission = "ai_waiter:read"
	PermAIWaiterReply    Permission = "ai_waiter:reply"    // claim/pause/reply/release (front-line takeover)
	PermAIWaiterInsights Permission = "ai_waiter:insights" // aggregate metrics (manager+)
	PermAIWaiterWrite    Permission = "ai_waiter:write"    // close conversation (manager+)

	// Marketing
	PermMarketingRead  Permission = "marketing:read"
	PermMarketingWrite Permission = "marketing:write"

	// Thermal Printer Support
	PermPrintersRead  Permission = "printers:read"
	PermPrintersWrite Permission = "printers:write"
	PermPrintBill     Permission = "print:bill"
	PermPrintReceipt  Permission = "print:receipt"

	// Cash Register / Caja
	PermCashRegisterRead    Permission = "cash_register:read"
	PermCashRegisterOperate Permission = "cash_register:operate"

	// Operational Alerts
	PermAlertsRead     Permission = "alerts:read"
	PermAlertsClaim    Permission = "alerts:claim"
	PermAlertsResolve  Permission = "alerts:resolve"
	PermAlertsSettings Permission = "alerts:settings"

	// Fiscal Compliance
	PermFiscalRead   Permission = "fiscal:read"
	PermFiscalWrite  Permission = "fiscal:write"
	PermFiscalIssue  Permission = "fiscal:issue"
	PermFiscalRetry  Permission = "fiscal:retry"
	PermFiscalExport Permission = "fiscal:export"
	// PermFiscalCredit (emit a nota de crédito = tax/VAT reversal) and
	// PermFiscalCredentials (upload the AFIP private-key bundle) are owner-only by
	// default and granted to NO staff role — they sign/reverse legal tax documents,
	// the same trust bar as payroll:write. Owners/platform admins bypass permission
	// checks; an owner may still grant either to a specific manager via custom perms.
	PermFiscalCredit      Permission = "fiscal:credit"
	PermFiscalCredentials Permission = "fiscal:credentials"

	// Staff Scheduling (turnos). schedule:write/publish/approve are manager+
	// (build/publish/approve schedules, swaps, time-off). schedule:read +
	// schedule:self are granted to every staff role (view + self-service:
	// availability, time-off, swap/claim requests). Pay rates on a position
	// stay owner-only (payroll:write), never on schedule:write.
	PermScheduleRead    Permission = "schedule:read"
	PermScheduleWrite   Permission = "schedule:write"
	PermSchedulePublish Permission = "schedule:publish"
	PermScheduleApprove Permission = "schedule:approve"
	PermScheduleSelf    Permission = "schedule:self"

	// Time clock (Slice 4). timeclock:punch is self-service — every staff role
	// clocks in/out/break and reads their OWN timesheet (hours/minutes only,
	// never dollars). timeclock:manage is manager+ (owner bypasses): review the
	// pending-review queue, approve entries, and file manual entries. Any labor-$
	// derived from an approved entry stays owner/financial:read-gated in the
	// accounting labor endpoint, never on this surface.
	PermTimeclockPunch  Permission = "timeclock:punch"
	PermTimeclockManage Permission = "timeclock:manage"

	// Staff chat & announcements (Slice 7). read/send for every staff role;
	// announce/moderate are manager+owner. DM/channel privacy is enforced per-row
	// (CanReadChannel), independent of these coarse perms.
	PermChatRead     Permission = "chat:read"
	PermChatSend     Permission = "chat:send"
	PermChatAnnounce Permission = "chat:announce"
	PermChatModerate Permission = "chat:moderate"

	// Staff Engagement (Slice 9). checklist:complete / doc:read / recognition:send
	// / poll:vote are all-staff (self-service + read). checklist:manage /
	// doc:manage / poll:manage are manager+owner (author templates, publish docs,
	// create/close polls, see results). No money perms here — pay stays
	// payroll:write/owner.
	PermChecklistComplete Permission = "checklist:complete"
	PermChecklistManage   Permission = "checklist:manage"
	PermDocRead           Permission = "doc:read"
	PermDocManage         Permission = "doc:manage"
	PermRecognitionSend   Permission = "recognition:send"
	PermPollVote          Permission = "poll:vote"
	PermPollManage        Permission = "poll:manage"
)

// StaffRoleHierarchy defines the hierarchy levels for staff roles
var StaffRoleHierarchy = map[database.StaffRole]int{
	database.StaffRoleKitchen: 1, // Lowest level
	database.StaffRoleHost:    2,
	database.StaffRoleServer:  3,
	database.StaffRoleManager: 4, // Highest staff level
	// Note: Web3 "owner" would be level 5, Platform "admin" would be level 6
}

// OwnerOnlyPermissions lists permissions that are NON-DELEGABLE to staff — never
// effective even when explicitly granted. It is intentionally EMPTY today.
//
// Payverge's sensitive money permissions (bills:refund, payroll:write,
// fiscal:credit, fiscal:credentials, director:*) are "owner-only BY DEFAULT":
// absent from every staff role, so a plain staff member never holds them. But
// they remain DELEGABLE — an owner may grant one to a specific trusted manager
// via custom permissions (a deliberate separation-of-duties escape hatch; see
// the permission-constant comments and TestAccountingRoutes_ManagerWithGranted*).
// Making them non-delegable here would silently break that documented capability.
//
// Add a permission to this set ONLY if it must be impossible to confer even with
// an explicit owner grant. The (role ∪ grants) − denies effective computation and
// the deny-override mechanism below are unaffected by this set being empty.
var OwnerOnlyPermissions = map[Permission]struct{}{}

// IsOwnerOnlyPermission reports whether a permission is non-delegable to staff
// (i.e. never effective even if granted). Empty set today — see OwnerOnlyPermissions.
func IsOwnerOnlyPermission(permission string) bool {
	_, ok := OwnerOnlyPermissions[Permission(permission)]
	return ok
}

// StaffRolePermissions defines the default permissions for each staff role
var StaffRolePermissions = map[database.StaffRole][]Permission{
	database.StaffRoleManager: {
		// Business Operations (no financial access)
		PermBusinessRead, PermBusinessWrite, PermBusinessSettings,

		// Staff Management (full control)
		PermStaffRead, PermStaffWrite, PermStaffInvite, PermStaffRemove, PermStaffRoles,
		PermStaffDelete, PermStaffRole, PermStaffPermissions, PermStaffDeactivate,
		PermStaffReactivate, PermStaffAudit, PermStaffAdmin,

		// Menu Management (full control)
		PermMenuRead, PermMenuWrite, PermMenuTranslate, PermMenuCategories, PermMenuItems,

		// Table Management (full control)
		PermTablesRead, PermTablesWrite, PermTablesCreate, PermTablesDelete, PermTablesQR,

		// Bill Management (full control)
		PermBillsRead, PermBillsWrite, PermBillsCreate, PermBillsClose, PermBillsItems, PermBillsPayment,

		// Order Management (full control)
		PermOrdersRead, PermOrdersWrite, PermOrdersCreate, PermOrdersStatus, PermOrdersKitchen,

		// Analytics & Reporting (full access)
		PermAnalyticsRead, PermAnalyticsSales, PermAnalyticsTips, PermAnalyticsItems,
		PermReportsRead, PermReportsExport,

		// Settings & Configuration (full control)
		PermSettingsRead, PermSettingsWrite, PermSettingsDesign, PermSettingsGoogle,
		PermSettingsCurrency, PermSettingsLanguage,
		PermCurrenciesRead, PermCurrenciesWrite, PermLanguagesRead, PermLanguagesWrite,
		PermFilesUpload, PermFilesDelete,

		// Plugin Management (full control)
		PermPluginsRead, PermPluginsWrite, PermPluginsConfig, PermPluginsPayments,

		// Delivery Operations (full control)
		PermDeliverySettingsRead, PermDeliverySettingsWrite,
		PermDeliveryDispatchRead, PermDeliveryDispatchWrite,
		PermDeliveryDriversRead, PermDeliveryDriversWrite,

		// Counter Operations (full control)
		PermCounterRead, PermCounterWrite, PermCounterSettings,

		// Overview Dashboard (full access)
		PermOverviewRead, PermOverviewKPI,

		// CRM (full access)
		PermCRMRead, PermCRMWrite, PermCRMExport,

		// Inventory (manager access)
		PermInventoryRead, PermInventoryWrite, PermInventoryAdjust, PermInventoryRecipes,

		// Reservations (full access)
		PermReservationsRead, PermReservationsWrite, PermReservationsCreate,
		PermReservationsDelete, PermReservationsSettings,

		// AI Waiter (full: handle conversations + close + insights; config stays owner-only)
		PermAIWaiterRead, PermAIWaiterReply, PermAIWaiterInsights, PermAIWaiterWrite,

		// Marketing
		PermMarketingRead, PermMarketingWrite,

		// Thermal Printer Support (full control)
		PermPrintersRead, PermPrintersWrite, PermPrintBill, PermPrintReceipt,

		// Cash Register / Caja
		PermCashRegisterRead, PermCashRegisterOperate,

		// Operational Alerts (full control)
		PermAlertsRead, PermAlertsClaim, PermAlertsResolve, PermAlertsSettings,

		// Fiscal Compliance (day-to-day issuance + settings/resend). Credit notes
		// (fiscal:credit) and AFIP key upload (fiscal:credentials) are owner-only and
		// deliberately NOT granted here.
		PermFiscalRead, PermFiscalWrite, PermFiscalIssue, PermFiscalRetry, PermFiscalExport,

		// Accounting access (owner and manager)
		PermFinancialRead, PermFinancialWrite,

		// Staff Scheduling (full control)
		PermScheduleRead, PermScheduleWrite, PermSchedulePublish, PermScheduleApprove, PermScheduleSelf,

		// Time clock (punch own + review/approve/manual-entry the whole queue)
		PermTimeclockPunch, PermTimeclockManage,

		// Staff chat (read/send + announce + moderate)
		PermChatRead, PermChatSend, PermChatAnnounce, PermChatModerate,

		// Staff Engagement (full control)
		PermChecklistComplete, PermChecklistManage,
		PermDocRead, PermDocManage,
		PermRecognitionSend,
		PermPollVote, PermPollManage,

		// Ops Assistant
		PermAssistantRead, PermAssistantWrite,

		// NO withdrawal/blockchain permissions - those are owner/admin only
	},

	database.StaffRoleServer: {
		// Business Operations (read only)
		PermBusinessRead,

		// Menu Management (read only)
		PermMenuRead,

		// Table Management (read/write for service)
		PermTablesRead, PermTablesWrite, PermTablesQR,

		// Bill Management (full control for service)
		PermBillsRead, PermBillsWrite, PermBillsCreate, PermBillsClose, PermBillsItems, PermBillsPayment,

		// Order Management (create and manage orders)
		PermOrdersRead, PermOrdersWrite, PermOrdersCreate, PermOrdersStatus,

		// Staff directory (read-only)
		PermStaffRead,

		// Counter Operations (for takeaway service)
		PermCounterRead, PermCounterWrite,

		// Delivery (read settings, full dispatch control, no driver mgmt)
		PermDeliverySettingsRead,
		PermDeliveryDispatchRead, PermDeliveryDispatchWrite,

		// Reservations (read and create)
		PermReservationsRead, PermReservationsCreate,

		// Overview Dashboard (basic access)
		PermOverviewRead,

		// AI Waiter (handle live conversations: read + claim/reply, no close/insights)
		PermAIWaiterRead, PermAIWaiterReply,

		// Thermal Printer Support (print only)
		PermPrintBill, PermPrintReceipt,

		// Cash Register / Caja
		PermCashRegisterRead, PermCashRegisterOperate,

		// Operational Alerts
		PermAlertsRead, PermAlertsClaim, PermAlertsResolve,

		// Staff Scheduling (view own schedule + self-service: availability, time-off, swap/claim)
		PermScheduleRead, PermScheduleSelf,

		// Time clock (punch own + read own timesheet, hours only)
		PermTimeclockPunch,

		// Staff chat (read + send; channel privacy enforced per-row)
		PermChatRead, PermChatSend,

		// Staff Engagement (complete own checklists, read docs, send kudos, vote)
		PermChecklistComplete, PermDocRead, PermRecognitionSend, PermPollVote,

		PermAssistantRead,
	},

	database.StaffRoleHost: {
		// Business Operations (read only)
		PermBusinessRead,

		// Menu Management (read only)
		PermMenuRead,

		// Table Management (seating and basic management)
		PermTablesRead, PermTablesWrite,

		// Bill Management (view only)
		PermBillsRead,

		// Reservations (full access for host role)
		PermReservationsRead, PermReservationsWrite, PermReservationsCreate, PermReservationsDelete,

		// Order Management (view only)
		PermOrdersRead,

		// Staff directory (read-only)
		PermStaffRead,

		// Overview Dashboard (basic access)
		PermOverviewRead,

		// AI Waiter (handle live conversations: read + claim/reply, no close/insights)
		PermAIWaiterRead, PermAIWaiterReply,

		// Thermal Printer Support (print bill only)
		PermPrintBill,

		// Operational Alerts
		PermAlertsRead, PermAlertsClaim, PermAlertsResolve,

		// Staff Scheduling (view own schedule + self-service: availability, time-off, swap/claim)
		PermScheduleRead, PermScheduleSelf,

		// Time clock (punch own + read own timesheet, hours only)
		PermTimeclockPunch,

		// Staff chat (read + send; channel privacy enforced per-row)
		PermChatRead, PermChatSend,

		// Staff Engagement (complete own checklists, read docs, send kudos, vote)
		PermChecklistComplete, PermDocRead, PermRecognitionSend, PermPollVote,

		PermAssistantRead,
	},

	database.StaffRoleKitchen: {
		// Business Operations (read only)
		PermBusinessRead,

		// Menu Management (read only)
		PermMenuRead,

		// Order Management (kitchen operations)
		PermOrdersRead, PermOrdersWrite, PermOrdersStatus, PermOrdersKitchen,

		// Bill Management (view for order context)
		PermBillsRead,

		// Staff directory (read-only)
		PermStaffRead,

		// Delivery (visibility into dispatch queue for prep planning)
		PermDeliveryDispatchRead,

		// Overview Dashboard (basic access)
		PermOverviewRead,

		// Operational Alerts
		PermAlertsRead, PermAlertsClaim, PermAlertsResolve,

		// Staff Scheduling (view own schedule + self-service: availability, time-off, swap/claim)
		PermScheduleRead, PermScheduleSelf,

		// Time clock (punch own + read own timesheet, hours only)
		PermTimeclockPunch,

		// Staff chat (read + send; channel privacy enforced per-row)
		PermChatRead, PermChatSend,

		// Staff Engagement (complete own checklists, read docs, send kudos, vote)
		PermChecklistComplete, PermDocRead, PermRecognitionSend, PermPollVote,

		PermAssistantRead,
	},
}

// RBACMiddleware provides role-based access control
type RBACMiddleware struct {
	db *database.DB
}

// NewRBACMiddleware creates a new RBAC middleware instance
func NewRBACMiddleware(db *database.DB) *RBACMiddleware {
	return &RBACMiddleware{db: db}
}

// RequirePermissions creates a middleware that checks for required permissions
func (r *RBACMiddleware) RequirePermissions(requiredPermissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if user has required permissions
		if !r.hasPermissions(c, requiredPermissions...) {
			RespondWithErrorParams(c, http.StatusForbidden, ErrCodeInsufficientRole, "Insufficient permissions for this action", map[string]interface{}{
				"required_permissions": requiredPermissions,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireAnyPermissions allows the request when at least one permission is
// effective and records exactly which candidates passed. Handlers that serve a
// mixed resource stream (such as bill/receipt/kitchen print jobs) use that list
// to filter individual rows instead of treating one permission as access to all
// resource kinds.
func (r *RBACMiddleware) RequireAnyPermissions(candidatePermissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		granted := make([]string, 0, len(candidatePermissions))
		for _, permission := range candidatePermissions {
			if r.hasPermissions(c, permission) {
				granted = append(granted, permission)
			}
		}
		if len(granted) == 0 {
			RespondWithErrorParams(c, http.StatusForbidden, ErrCodeInsufficientRole, "Insufficient permissions for this action", map[string]interface{}{
				"any_required_permission": candidatePermissions,
			})
			c.Abort()
			return
		}
		c.Set(ContextAuthorizedAnyPermissions, granted)
		c.Next()
	}
}

// hasPermissions checks if the current user has the required permissions
func (r *RBACMiddleware) hasPermissions(c *gin.Context, requiredPermissions ...string) bool {
	tokenType, exists := c.Get("token_type")
	if !exists {
		return false
	}

	// Platform admin has all permissions
	if hasPlatformAdminRole(c) {
		return true
	}

	// Web3 business owners have all permissions for their business.
	// Explicitly verify that the authenticated address matches the business owner
	// address recorded by HybridAuthMiddleware. This avoids a silent bypass if
	// middleware ordering ever changes.
	if tokenType == "web3" {
		address, addrExists := c.Get("address")
		businessOwner, ownerExists := c.Get("business_owner_address")
		if addrExists && ownerExists {
			addrStr, addrOk := address.(string)
			ownerStr, ownerOk := businessOwner.(string)
			if addrOk && ownerOk && addrStr != "" && strings.EqualFold(addrStr, ownerStr) {
				return true
			}
			// Addresses present but do not match — deny.
			return false
		}
		// business_owner_address not set means no business ID was in the URL path.
		// Permission-protected routes must opt in before a web3 token without
		// verified business scope can pass.
		if addrExists && isUserLevelPermissionRoute(c) {
			return true
		}
	}

	// OAuth business owners (user token type) have all permissions for their business.
	// Explicitly verify that the authenticated user_id matches the business owner
	// user_id recorded by HybridAuthMiddleware.
	if tokenType == "user" {
		userID, uidExists := c.Get("user_id")
		businessOwnerUID, ownerExists := c.Get("business_owner_user_id")
		// Normalise to uint for comparison — context values may be float64 (JWT)
		// or uint (direct set).
		toUint := func(v interface{}) (uint, bool) {
			switch val := v.(type) {
			case float64:
				return uint(val), true
			case uint:
				return val, true
			case int:
				return uint(val), true
			}
			return 0, false
		}
		if uidExists && ownerExists {
			uid, uidOk := toUint(userID)
			ownerUID, ownerOk := toUint(businessOwnerUID)
			if uidOk && ownerOk && uid != 0 && uid == ownerUID {
				return true
			}
			// IDs present but do not match — deny.
			return false
		}
		// business_owner_user_id not set — no verified business scope on this
		// request. Mirror the web3 fail-closed path: only routes that explicitly
		// opt in via AllowUserLevelPermissions() may pass without owner context.
		if uidExists && isUserLevelPermissionRoute(c) {
			uid, ok := toUint(userID)
			return ok && uid != 0
		}
		return false
	}

	// Staff users need role-based permission checking
	if tokenType == "staff" {
		return r.checkStaffPermissions(c, requiredPermissions)
	}

	return false
}

func hasPlatformAdminRole(c *gin.Context) bool {
	return contextRoleEquals(c, "role", string(structs.RoleAdmin)) ||
		contextRoleEquals(c, "user_role", string(structs.RoleAdmin))
}

func contextRoleEquals(c *gin.Context, key string, expected string) bool {
	value, exists := c.Get(key)
	if !exists {
		return false
	}

	role, ok := value.(string)
	return ok && role == expected
}

func isUserLevelPermissionRoute(c *gin.Context) bool {
	value, exists := c.Get(ContextAllowUserLevelPermissions)
	if !exists {
		return false
	}

	allowed, ok := value.(bool)
	return ok && allowed
}

func AllowUserLevelPermissions() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(ContextAllowUserLevelPermissions, true)
		c.Next()
	}
}

// checkStaffPermissions checks if staff user has required permissions.
// A staff request is ALLOWED for required iff:
//
//	hasPermission(role ∪ grants, required) AND NOT denyMatches(denies, required)
//	AND NOT IsOwnerOnlyPermission(required)
//
// Denies and grants are loaded from gin context on the hot path (zero extra
// queries when hydrateLiveStaffContext ran); see getPermissionDenies.
func (r *RBACMiddleware) checkStaffPermissions(c *gin.Context, requiredPermissions []string) bool {
	staffRole, exists := c.Get("staff_role")
	if !exists {
		return false
	}

	roleStr, ok := staffRole.(string)
	if !ok {
		return false
	}

	// Role ∪ custom grants (before deny / owner-only filter). Used with hasPermission
	// so wildcard grants (e.g. menu:*) still match concrete required keys.
	granted := r.getStaffGrantedPermissions(database.StaffRole(roleStr), c)

	var denies []string
	if staffID, exists := c.Get("staff_id"); exists {
		denies = r.getPermissionDenies(c, staffID)
	}

	for _, required := range requiredPermissions {
		if IsOwnerOnlyPermission(required) {
			return false
		}
		if !r.hasPermission(granted, required) {
			return false
		}
		if denyMatches(denies, required) {
			return false
		}
	}

	return true
}

// getStaffGrantedPermissions returns role defaults ∪ custom grants (no deny
// subtraction). Used by the check path so wildcard grants compose with denyMatches.
func (r *RBACMiddleware) getStaffGrantedPermissions(role database.StaffRole, c *gin.Context) []string {
	rolePermissions := StaffRolePermissions[role]
	permissions := make([]string, len(rolePermissions))
	for i, perm := range rolePermissions {
		permissions[i] = string(perm)
	}

	if staffID, exists := c.Get("staff_id"); exists {
		if customPerms := r.getCustomPermissions(c, staffID); customPerms != nil {
			permissions = append(permissions, customPerms...)
		}
	}

	return permissions
}

// getStaffPermissions returns the effective staff permission list for display /
// SSE topic filtering: (role ∪ custom grants) minus denies and owner-only keys.
// Enforcement of wildcards + denies uses checkStaffPermissions (denyMatches at
// check time), which is the security core for route gates.
func (r *RBACMiddleware) getStaffPermissions(role database.StaffRole, c *gin.Context) []string {
	granted := r.getStaffGrantedPermissions(role, c)

	var denies []string
	if staffID, exists := c.Get("staff_id"); exists {
		denies = r.getPermissionDenies(c, staffID)
	}

	out := make([]string, 0, len(granted))
	for _, perm := range granted {
		if IsOwnerOnlyPermission(perm) {
			continue
		}
		if denyMatches(denies, perm) {
			continue
		}
		out = append(out, perm)
	}
	return out
}

// getCustomPermissions retrieves custom permissions for a staff member.
//
// Hot path: hydrateLiveStaffContext already loads the full staff row (including
// custom_permissions) and stashes the raw JSON under "staff_custom_permissions".
// When that key is present we reuse it and issue ZERO additional queries —
// removing the redundant per-request staff SELECT on every permission-checked
// staff request (MW-01). A present-but-empty value is authoritative ("no custom
// perms"), so we still skip the query.
//
// Fail-safe fallback: if the key is absent (a caller that did not run
// hydrateLiveStaffContext), we fall back to a narrow query that selects ONLY
// custom_permissions (+ pk) — never the full staff row.
func (r *RBACMiddleware) getCustomPermissions(c *gin.Context, staffID interface{}) []string {
	// Hot path: reuse the value hydrated into context. `exists` distinguishes a
	// real context hit (skip the query, even when the value is "") from a context
	// miss (must fall back to a query).
	if raw, exists := c.Get("staff_custom_permissions"); exists {
		if rawStr, ok := raw.(string); ok {
			return parseCustomPermissions(rawStr)
		}
		// Unexpected type stashed; treat as no custom perms (no query — context
		// was populated, so a fallback SELECT would not be more authoritative).
		return nil
	}

	// Context miss: fall back to a narrow query for custom_permissions only.
	var id uint
	switch v := staffID.(type) {
	case float64:
		id = uint(v)
	case uint:
		id = v
	case int:
		id = uint(v)
	default:
		return nil
	}

	var staff database.Staff
	if err := r.db.GetGorm().Select("custom_permissions").First(&staff, id).Error; err != nil {
		return nil
	}

	return parseCustomPermissions(staff.CustomPermissions)
}

// parseCustomPermissions parses the raw custom_permissions JSON array string.
// Empty or malformed input yields nil (no custom perms).
func parseCustomPermissions(raw string) []string {
	if raw == "" {
		return nil
	}
	var customPerms []string
	if err := json.Unmarshal([]byte(raw), &customPerms); err != nil {
		return nil
	}
	return customPerms
}

// getPermissionDenies retrieves explicit deny permission strings for a staff member.
//
// Hot path: hydrateLiveStaffContext loads denies in the same hydration step and
// stashes []string under "staff_permission_denies". When that key is present we
// reuse it and issue ZERO additional queries. A present-but-empty slice is
// authoritative ("no denies").
//
// Fail-safe fallback: if the key is absent, a single narrow query keyed by
// staff_id plucks permission from staff_permission_denies (not N+1, not full rows).
func (r *RBACMiddleware) getPermissionDenies(c *gin.Context, staffID interface{}) []string {
	if raw, exists := c.Get("staff_permission_denies"); exists {
		if list, ok := raw.([]string); ok {
			return list
		}
		// Unexpected type stashed; treat as no denies (context was populated).
		return nil
	}

	id, ok := coerceStaffID(staffID)
	if !ok {
		return nil
	}

	var denies []string
	if err := r.db.GetGorm().Model(&database.StaffPermissionDeny{}).
		Where("staff_id = ?", id).
		Pluck("permission", &denies).Error; err != nil {
		// This fallback is used by callers that did not run the normal staff
		// hydration middleware. Fail closed: an empty set on DB failure would
		// silently restore every inherited/custom permission.
		requestID, _ := c.Get("request_id")
		logger.Logger.Warnf("RBAC deny lookup failed closed (staff_id=%d request_id=%v): %v", id, requestID, err)
		return []string{"*:*"}
	}
	if denies == nil {
		return []string{}
	}
	return denies
}

func coerceStaffID(staffID interface{}) (uint, bool) {
	switch v := staffID.(type) {
	case float64:
		return uint(v), true
	case uint:
		return v, true
	case int:
		return uint(v), true
	case int64:
		return uint(v), true
	default:
		return 0, false
	}
}

// denyMatches reports whether required is blocked by any explicit deny entry.
// Honors exact match, category wildcards (menu:* blocks menu:read), and *:*.
func denyMatches(denies []string, required string) bool {
	for _, d := range denies {
		if d == required {
			return true
		}
		if strings.HasSuffix(d, ":*") {
			category := strings.TrimSuffix(d, ":*")
			if strings.HasPrefix(required, category+":") {
				return true
			}
		}
		if d == "*:*" {
			return true
		}
	}
	return false
}

// hasPermission checks if a permission exists in the user's permission list
func (r *RBACMiddleware) hasPermission(userPermissions []string, required string) bool {
	for _, perm := range userPermissions {
		// Check for exact match
		if perm == required {
			return true
		}

		// Check for wildcard permissions (e.g., "menu:*" matches "menu:read")
		if strings.HasSuffix(perm, ":*") {
			category := strings.TrimSuffix(perm, ":*")
			if strings.HasPrefix(required, category+":") {
				return true
			}
		}

		// Check for super wildcard (admin only)
		if perm == "*:*" {
			return true
		}
	}

	return false
}

// CanManageRole checks if one role can manage another based on hierarchy.
// Fail-closed: if either role is not in StaffRoleHierarchy (unknown string,
// corrupt DB row, or a future role string not yet wired into the hierarchy),
// returns false. This prevents bugs like GetRoleLevel("owner") returning 0
// and a manager (level 4) silently outranking the unknown role.
func CanManageRole(managerRole, targetRole database.StaffRole) bool {
	managerLevel, mOK := StaffRoleHierarchy[managerRole]
	targetLevel, tOK := StaffRoleHierarchy[targetRole]
	if !mOK || !tOK {
		return false
	}
	return managerLevel > targetLevel
}

// IsValidStaffRole reports whether the given role is one of the known staff
// roles defined in StaffRoleHierarchy. Use this for input validation before
// trusting a role string from a request body.
func IsValidStaffRole(role database.StaffRole) bool {
	_, ok := StaffRoleHierarchy[role]
	return ok
}

// RolePermissions returns the static permission list for a staff role, or
// nil if the role has no entry. Exported so RBAC handlers can validate
// grant requests against the actor's own perm set.
func RolePermissions(role database.StaffRole) []Permission {
	return StaffRolePermissions[role]
}

// Global RBAC middleware instance
var rbacMiddleware *RBACMiddleware

// InitializeRBAC initializes the global RBAC middleware
func InitializeRBAC(db *database.DB) {
	rbacMiddleware = NewRBACMiddleware(db)
}

// RequirePermissions is the global function for route protection (replaces RoleBasedAccessMiddleware)
func RequirePermissions(requiredPermissions ...string) gin.HandlerFunc {
	if rbacMiddleware == nil {
		// Fallback if RBAC not initialized
		return func(c *gin.Context) {
			RespondWithError(c, http.StatusInternalServerError, "", "RBAC system not initialized")
			c.Abort()
		}
	}

	return rbacMiddleware.RequirePermissions(requiredPermissions...)
}

// RequireAnyPermissions is the global any-of counterpart to
// RequirePermissions. It also exposes the passing candidates to the handler.
func RequireAnyPermissions(candidatePermissions ...string) gin.HandlerFunc {
	if rbacMiddleware == nil {
		return func(c *gin.Context) {
			RespondWithError(c, http.StatusInternalServerError, "", "RBAC system not initialized")
			c.Abort()
		}
	}
	return rbacMiddleware.RequireAnyPermissions(candidatePermissions...)
}

// ResolveContextPermissions resolves the caller's effective RBAC permissions
// from the gin context for display and compatibility callers.
//
// It preserves the caller's grants for display, but MUST NOT be used to decide
// a concrete permission: wildcard grants plus narrower explicit denies cannot
// be represented safely as a flat string list. Use
// ResolveContextEventPermissions for authorization decisions.
//
// Identity semantics:
//   - platform admin, verified web3 owner, and verified OAuth (user) owner all
//     hold ALL permissions for the business -> allAccess=true (the caller should
//     receive everything).
//   - staff resolve to their role permissions + custom permissions.
//
// allAccess=true means "treat as full access, do not topic-scope". When false,
// permissions is the concrete effective list to filter against. A context that
// fails every branch returns (nil, false) — no permissions, no access — which
// the caller must treat as deny.
func ResolveContextPermissions(c *gin.Context) (permissions []string, allAccess bool) {
	if rbacMiddleware == nil {
		return nil, false
	}

	tokenType, exists := c.Get("token_type")
	if !exists {
		return nil, false
	}

	// Platform admin: all permissions.
	if hasPlatformAdminRole(c) {
		return nil, true
	}

	// Verified web3 business owner: all permissions for their business.
	if tokenType == "web3" {
		address, addrExists := c.Get("address")
		businessOwner, ownerExists := c.Get("business_owner_address")
		if addrExists && ownerExists {
			addrStr, addrOk := address.(string)
			ownerStr, ownerOk := businessOwner.(string)
			if addrOk && ownerOk && addrStr != "" && strings.EqualFold(addrStr, ownerStr) {
				return nil, true
			}
			return nil, false
		}
	}

	// Verified OAuth (user) business owner: all permissions for their business.
	if tokenType == "user" {
		userID, uidExists := c.Get("user_id")
		businessOwnerUID, ownerExists := c.Get("business_owner_user_id")
		if uidExists && ownerExists {
			toUint := func(v interface{}) (uint, bool) {
				switch val := v.(type) {
				case float64:
					return uint(val), true
				case uint:
					return val, true
				case int:
					return uint(val), true
				}
				return 0, false
			}
			uid, uidOk := toUint(userID)
			ownerUID, ownerOk := toUint(businessOwnerUID)
			if uidOk && ownerOk && uid != 0 && uid == ownerUID {
				return nil, true
			}
			return nil, false
		}
	}

	// Staff: resolve role-based + custom permissions.
	if tokenType == "staff" {
		staffRole, ok := c.Get("staff_role")
		if !ok {
			return nil, false
		}
		roleStr, ok := staffRole.(string)
		if !ok {
			return nil, false
		}
		return rbacMiddleware.getStaffPermissions(database.StaffRole(roleStr), c), false
	}

	return nil, false
}

// ResolveContextEventPermissions returns a request-scoped checker for concrete
// SSE event permissions. Unlike an effective permission list, the checker keeps
// wildcard grants and explicit denies separate, so a grant such as financial:*
// cannot bypass an exact financial:read deny. It also applies the same
// non-delegable owner-only rule as REST's checkStaffPermissions.
//
// Grants and denies are resolved exactly once when the stream connects. Normal
// authenticated requests use the context-hydrated values, so evaluating all SSE
// topics adds no database queries.
func ResolveContextEventPermissions(c *gin.Context) (checker func(string) bool, allAccess bool) {
	if rbacMiddleware == nil {
		return nil, false
	}

	tokenTypeValue, exists := c.Get("token_type")
	if !exists {
		return nil, false
	}
	tokenType, ok := tokenTypeValue.(string)
	if !ok {
		return nil, false
	}

	if hasPlatformAdminRole(c) {
		return nil, true
	}

	if tokenType == "web3" {
		address, addressExists := c.Get("address")
		businessOwner, ownerExists := c.Get("business_owner_address")
		addressString, addressOK := address.(string)
		ownerString, ownerOK := businessOwner.(string)
		if addressExists && ownerExists && addressOK && ownerOK && addressString != "" && strings.EqualFold(addressString, ownerString) {
			return nil, true
		}
		return nil, false
	}

	if tokenType == "user" {
		userID, userExists := c.Get("user_id")
		businessOwnerID, ownerExists := c.Get("business_owner_user_id")
		uid, userOK := coerceStaffID(userID)
		ownerUID, ownerOK := coerceStaffID(businessOwnerID)
		if userExists && ownerExists && userOK && ownerOK && uid != 0 && uid == ownerUID {
			return nil, true
		}
		return nil, false
	}

	if tokenType != "staff" {
		return nil, false
	}

	staffRole, exists := c.Get("staff_role")
	if !exists {
		return nil, false
	}
	roleStr, ok := staffRole.(string)
	if !ok {
		return nil, false
	}

	granted := rbacMiddleware.getStaffGrantedPermissions(database.StaffRole(roleStr), c)
	var denies []string
	if staffID, exists := c.Get("staff_id"); exists {
		denies = rbacMiddleware.getPermissionDenies(c, staffID)
	}

	return func(required string) bool {
		if IsOwnerOnlyPermission(required) {
			return false
		}
		return rbacMiddleware.hasPermission(granted, required) && !denyMatches(denies, required)
	}, false
}
