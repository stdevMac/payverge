package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"
)

func extractContextUint(value any) (uint, bool) {
	switch v := value.(type) {
	case uint:
		return v, true
	case uint64:
		return uint(v), true
	case uint32:
		return uint(v), true
	case int:
		if v < 0 {
			return 0, false
		}
		return uint(v), true
	case int64:
		if v < 0 {
			return 0, false
		}
		return uint(v), true
	case int32:
		if v < 0 {
			return 0, false
		}
		return uint(v), true
	case float64:
		if v < 0 {
			return 0, false
		}
		return uint(v), true
	case float32:
		if v < 0 {
			return 0, false
		}
		return uint(v), true
	case string:
		parsed, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return 0, false
		}
		return uint(parsed), true
	default:
		return 0, false
	}
}

func contextUserID(c *gin.Context) (uint, bool) {
	uid, ok := c.Get("user_id")
	if !ok {
		return 0, false
	}
	parsed, ok := extractContextUint(uid)
	if !ok || parsed == 0 {
		return 0, false
	}
	return parsed, true
}

func demoOwnerMatches(c *gin.Context, business *database.Business) bool {
	if business == nil || business.DemoOwnerUserID == nil {
		return false
	}
	uid, ok := contextUserID(c)
	return ok && *business.DemoOwnerUserID == uid
}

// CheckBusinessOwnership verifies the authenticated user owns the given business.
// Supports Web3 owners, OAuth owners identified by linked user ID, and the
// demo-owner account that GetMyBusinesses already lists.
func CheckBusinessOwnership(c *gin.Context, business *database.Business) bool {
	if business == nil {
		return false
	}

	if addr, ok := c.Get("address"); ok {
		if a, ok := addr.(string); ok && business.OwnerAddress != "" && strings.EqualFold(strings.TrimSpace(business.OwnerAddress), strings.TrimSpace(a)) {
			return true
		}
	}

	if uid, ok := contextUserID(c); ok {
		if business.UserID != nil && *business.UserID == uid {
			return true
		}
		if business.DemoOwnerUserID != nil && *business.DemoOwnerUserID == uid {
			return true
		}
	}

	return false
}

// actorCanOpenListedBusiness is the single list/open rule: an actor may GET a
// business if they can already list it. True non-owners still fail.
func actorCanOpenListedBusiness(c *gin.Context, business *database.Business) bool {
	return CheckBusinessAccess(c, business)
}

func listedDemoForAdmin(business *database.Business) bool {
	if business == nil {
		return false
	}
	return business.IsDemo || business.Kind == database.BusinessKindDemo
}

// CheckBusinessAccess verifies the authenticated user has access to the given business.
// Same set as ListBusinessesForInsideUser: owners, demo owners, staff of this
// business, and platform admins on active demos. Live non-demo tenants stay
// owner/staff-only — admin is not a global impersonate key.
func CheckBusinessAccess(c *gin.Context, business *database.Business) bool {
	if business == nil {
		return false
	}
	if hasPlatformAdminRole(c) && listedDemoForAdmin(business) {
		return true
	}
	if CheckBusinessOwnership(c, business) {
		return true
	}

	// Staff member
	if staffBizID, ok := c.Get("staff_business_id"); ok {
		if bizID, ok := extractContextUint(staffBizID); ok && bizID == business.ID {
			return true
		}
	}

	return false
}

// denyUnlessListedBusinessAccess is the hybrid-middleware counterpart of
// GetMyBusinesses. Owners, demo owners, staff of this business, and platform
// admins pass; anyone else gets BIZ_NOT_OWNER.
func denyUnlessListedBusinessAccess(c *gin.Context, business *database.Business) bool {
	if actorCanOpenListedBusiness(c, business) {
		recordVerifiedBusinessOwner(c, business)
		return true
	}
	RespondWithError(c, http.StatusForbidden, ErrCodeNotBusinessOwner, "Access denied: not business owner")
	c.Abort()
	return false
}

func recordVerifiedBusinessOwner(c *gin.Context, business *database.Business) {
	if business == nil {
		return
	}
	if CheckBusinessOwnership(c, business) {
		if business.OwnerAddress != "" {
			c.Set("business_owner_address", business.OwnerAddress)
		}
		if business.UserID != nil {
			c.Set("business_owner_user_id", *business.UserID)
		} else if uid, ok := contextUserID(c); ok && demoOwnerMatches(c, business) {
			c.Set("business_owner_user_id", uid)
		}
	}
}

// requireBusinessAccess fetches the business referenced by URL param `idParam`
// and authorizes it for staff-with-the-right-perm or Web3/OAuth owners.
//
// Status codes written on failure (handler-level, after middleware has already
// validated the token):
//   - 404 if the business id doesn't resolve
//   - 403 if the actor isn't a member/owner of this specific business
//   - 500 on database errors
//
// On success returns (business, true) without writing to the response.
func requireBusinessAccess(c *gin.Context, idParam string) (*database.Business, bool) {
	business, ok := fetchBusinessByParam(c, idParam)
	if !ok {
		return nil, false
	}
	if !CheckBusinessAccess(c, business) {
		metrics.TenantAuthorizationMismatches.WithLabelValues("business_access").Inc()
		c.JSON(http.StatusForbidden, gin.H{"error": "You don't have access to this business"})
		c.Abort()
		return nil, false
	}
	return business, true
}

// requireBusinessOwnership is the owner-only counterpart. Use this for
// destructive or owner-exclusive operations (DeleteBusiness, settlement
// address mutation).
func requireBusinessOwnership(c *gin.Context, idParam string) (*database.Business, bool) {
	business, ok := fetchBusinessByParam(c, idParam)
	if !ok {
		return nil, false
	}
	if !CheckBusinessOwnership(c, business) {
		metrics.TenantAuthorizationMismatches.WithLabelValues("business_ownership").Inc()
		c.JSON(http.StatusForbidden, gin.H{"error": "You don't own this business"})
		c.Abort()
		return nil, false
	}
	return business, true
}

// fetchBusinessByParam handles the 404/500 path that both helpers share.
func fetchBusinessByParam(c *gin.Context, idParam string) (*database.Business, bool) {
	id := c.Param(idParam)
	business, err := database.GetBusinessByIdOrBusinessId(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve business"})
		}
		c.Abort()
		return nil, false
	}
	return business, true
}
