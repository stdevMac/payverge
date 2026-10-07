package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/structs"

	"github.com/gin-gonic/gin"
)

const (
	contextBillKey      = "_bill"
	contextBillItemsKey = "_bill_items"
)

// businessFromContext loads the business for the current request.
// It prefers a pre-loaded business set by earlier middleware (key "_business"),
// otherwise it reads :id from the route and fetches from the database. The
// :id segment may be either a numeric primary key or a string business_id
// slug — mirroring the resolution used by auth and business handlers — so
// slug-based dashboard URLs (e.g. /businesses/demo-core-business/plugins)
// resolve here too instead of failing with "Business not found".
// The loaded business is cached in the context so downstream
// handlers/middlewares don't re-fetch.
func businessFromContext(c *gin.Context) (*database.Business, error) {
	if v, ok := c.Get("_business"); ok {
		if b, ok := v.(*database.Business); ok && b != nil {
			return b, nil
		}
	}
	if businessID := extractBusinessID(c); businessID != "" {
		b, err := database.GetBusinessByIdOrBusinessId(businessID)
		if err != nil {
			return nil, err
		}
		c.Set("_business", b)
		return b, nil
	}

	if staffID := strings.TrimSpace(c.Param("staff_id")); staffID != "" {
		parsed, err := strconv.ParseUint(staffID, 10, 64)
		if err != nil || parsed == 0 {
			return nil, fmt.Errorf("business not found")
		}
		staff, err := database.GetDBWrapper().StaffService.GetByID(uint(parsed))
		if err != nil {
			return nil, err
		}
		b, err := database.GetBusinessByID(staff.BusinessID)
		if err != nil {
			return nil, err
		}
		c.Set("_business", b)
		return b, nil
	}

	return nil, fmt.Errorf("business not found")
}

// RequireOperationalBusiness rejects requests for a business the server
// administrator suspended (is_active=false) or closed (closed_at) with 403
// business_suspended / business_closed. Demo showrooms never lock. It is the
// only business-level lock on operator routes.
func RequireOperationalBusiness() gin.HandlerFunc {
	return func(c *gin.Context) {
		b, err := businessFromContext(c)
		if err != nil || b == nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "Business not found"})
			return
		}
		if RespondIfBusinessLocked(c, b) {
			return
		}
		c.Next()
	}
}

// hydrateLivePlatformAdminRole copies users.role into the gin context when the
// JWT/session claim is missing or not a string. Hybrid sets c.Set("role",
// claims["role"]) and listed-demo access keys off that value; a stale or
// non-string claim would 403 bill GET while GET /businesses/:id/bills still
// passes via a different ownership path.
func hydrateLivePlatformAdminRole(c *gin.Context) {
	if hasPlatformAdminRole(c) {
		return
	}
	uid, ok := contextUserID(c)
	if !ok {
		return
	}
	var role string
	if err := database.GetDB().Table("users").Select("role").Where("id = ?", uid).Limit(1).Scan(&role).Error; err != nil {
		return
	}
	if role == string(structs.RoleAdmin) {
		c.Set("role", role)
	}
}

// resolveBillBusinessAccess applies CheckBusinessAccess to the projected
// bill-access row. Production Postgres has returned that row with ID set and
// is_demo/kind still zero, so a platform admin who can already list/open the
// demo still 403s bill GET. Reload the auth-scope row and retry.
func resolveBillBusinessAccess(c *gin.Context, projected *database.Business) (*database.Business, bool) {
	hydrateLivePlatformAdminRole(c)
	if CheckBusinessAccess(c, projected) {
		return projected, true
	}
	if !hasPlatformAdminRole(c) || projected == nil || projected.ID == 0 {
		return projected, false
	}
	full, err := database.GetBusinessAuthScopeByIdOrBusinessId(strconv.FormatUint(uint64(projected.ID), 10))
	if err != nil || !CheckBusinessAccess(c, full) {
		return projected, false
	}
	return full, true
}

func billFromRequestContext(c *gin.Context) (*database.Bill, []database.BillItem, bool) {
	billValue, ok := c.Get(contextBillKey)
	if !ok {
		return nil, nil, false
	}
	bill, ok := billValue.(*database.Bill)
	if !ok || bill == nil {
		return nil, nil, false
	}

	itemsValue, ok := c.Get(contextBillItemsKey)
	if !ok {
		return nil, nil, false
	}
	items, ok := itemsValue.([]database.BillItem)
	if !ok {
		return nil, nil, false
	}

	return bill, items, true
}

func RequireBillBusinessAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		billID, err := strconv.ParseUint(c.Param("bill_id"), 10, 64)
		if err != nil || billID == 0 {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid bill id"})
			return
		}

		business, err := database.GetBillBusinessAccessByBillID(uint(billID))
		if err != nil || business == nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
			return
		}

		resolved, ok := resolveBillBusinessAccess(c, business)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Not authorized to access this bill"})
			return
		}
		business = resolved

		// Bill routes are keyed by :bill_id, not :id, so the auth middleware never
		// resolved the owning business and never set the owner context that the
		// downstream RBAC permission check consults. Populate it from the business
		// whose ownership we just verified, so a web3 (SIWE) owner is authorized on
		// bill mutations exactly like an OAuth owner — previously the web3 owner
		// fell through hasPermissions to a denial while OAuth owners passed. This
		// runs only AFTER CheckBusinessAccess, so it cannot widen access: the caller
		// is already a verified owner (address or user_id match) or scoped staff
		// (handled by the staff branch of the permission check).
		if business.OwnerAddress != "" {
			c.Set("business_owner_address", business.OwnerAddress)
		}
		if business.UserID != nil {
			c.Set("business_owner_user_id", *business.UserID)
		}

		if RespondIfBusinessLocked(c, business) {
			return
		}

		c.Set("_business", business)
		c.Next()
	}
}

// RequireTableBusinessAccess resolves a table-scoped route to its owning
// business, enforces CheckBusinessAccess, and sets business_owner_* context so
// OAuth/web3 owners satisfy RoleBasedAccessMiddleware without fail-open.
func RequireTableBusinessAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		tableID, err := strconv.ParseUint(c.Param("id"), 10, 64)
		if err != nil || tableID == 0 {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid table id"})
			return
		}
		table, err := database.GetTableByID(uint(tableID))
		if err != nil || table == nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "Table not found"})
			return
		}
		business, err := database.GetBusinessByID(table.BusinessID)
		if err != nil || business == nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "Business not found"})
			return
		}
		if !CheckBusinessAccess(c, business) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Not authorized to access this table"})
			return
		}
		// Populate owner context AFTER access verification so RBAC can authorize
		// the verified owner without the OAuth fail-open branch.
		if business.OwnerAddress != "" {
			c.Set("business_owner_address", business.OwnerAddress)
		}
		if business.UserID != nil {
			c.Set("business_owner_user_id", *business.UserID)
		}
		if RespondIfBusinessLocked(c, business) {
			return
		}
		c.Set("_business", business)
		c.Next()
	}
}
