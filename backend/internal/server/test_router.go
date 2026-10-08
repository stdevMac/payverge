//go:build integration
// +build integration

package server

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// BuildRouterForTest returns a Gin engine wired with the production middleware
// chain and the routes that participate in the RBAC matrix. It is exported
// only behind the integration build tag so production code never imports it.
func BuildRouterForTest(db *gorm.DB) *gin.Engine {
	database.SetTestDB(db)
	// Initialize the RBAC middleware so RoleBasedAccessMiddleware /
	// RequirePermissions don't 500 with "RBAC system not initialized" inside
	// the test harness. Production wires this up in cmd/app/main.go.
	InitializeRBAC(database.GetDBWrapper())
	r := gin.New()
	r.Use(gin.Recovery())

	protected := r.Group("/api/v1/inside")
	protected.Use(HybridAuthenticationMiddleware())

	registerRBACMatrixRoutes(protected)
	return r
}

// registerRBACMatrixRoutes mirrors the subset of cmd/app/main.go routes that
// the matrix exercises. Keep this in sync with the spec's endpoint list.
func registerRBACMatrixRoutes(g *gin.RouterGroup) {
	g.GET("/businesses/:id", GetBusiness)
	g.PUT("/businesses/:id", RoleBasedAccessMiddleware("business:settings"), UpdateBusiness)
	g.DELETE("/businesses/:id", RoleBasedAccessMiddleware("business:settings"), DeleteBusiness)

	// Settings (staff:write — manager-allowed).
	g.PUT("/businesses/:id/operating-hours", RequireOperationalBusiness(), RoleBasedAccessMiddleware("settings:write"), UpdateBusinessOperatingHours)
	g.PUT("/businesses/:id/special-features", RequireOperationalBusiness(), RoleBasedAccessMiddleware("settings:write"), UpdateBusinessSpecialFeatures)
	g.PUT("/businesses/:id/gallery-images", RequireOperationalBusiness(), RoleBasedAccessMiddleware("settings:write"), UpdateBusinessGalleryImages)
	g.PUT("/businesses/:id/design-settings", RequireOperationalBusiness(), RoleBasedAccessMiddleware("settings:design"), UpdateBusinessDesignSettings)
	g.PUT("/businesses/:id/hospitality-settings", RequireOperationalBusiness(), RoleBasedAccessMiddleware("settings:write"), UpdateBusinessHospitalitySettings)
	g.PUT("/businesses/:id/google", RoleBasedAccessMiddleware("settings:google"), UpdateBusinessGoogleInfo)
	g.DELETE("/businesses/:id/google", RoleBasedAccessMiddleware("settings:google"), RemoveBusinessGoogleInfo)
	g.POST("/businesses/:id/toggle-kitchen-orders", RequireOperationalBusiness(), RoleBasedAccessMiddleware("settings:write"), ToggleKitchenAndOrders)

	// Menu (manager-only via menu:items / menu:categories).
	g.POST("/businesses/:id/menu/items", RoleBasedAccessMiddleware("menu:items"), AddMenuItem)
	g.PUT("/businesses/:id/menu/items", RoleBasedAccessMiddleware("menu:items"), UpdateMenuItem)
	g.POST("/businesses/:id/menu/categories", RoleBasedAccessMiddleware("menu:categories"), AddMenuCategory)

	// Bills (manager + server via bills:create).
	g.POST("/businesses/:id/bills", RequireOperationalBusiness(), RoleBasedAccessMiddleware("bills:create"), CreateBill)

	// Staff (read for everyone via staff:read; invite for manager only).
	g.GET("/businesses/:id/staff", RequireOperationalBusiness(), RoleBasedAccessMiddleware("staff:read"), GetBusinessStaff)
	g.POST("/businesses/:id/staff/invite", RequireOperationalBusiness(), RoleBasedAccessMiddleware("staff:invite"), InviteStaff)

	// Plugins — only the middleware gate (plugins:read) is under test on this
	// route. We stub the handler so the test router doesn't depend on the
	// plugin registry initialization. Subsequent code reviews should not
	// expect the real GetBusinessPlugins behavior to be exercised here; that
	// belongs in a plugin-handler-specific test if/when one is needed.
	g.GET("/businesses/:id/plugins", RequireOperationalBusiness(), RoleBasedAccessMiddleware("plugins:read"), func(c *gin.Context) {
		c.JSON(200, gin.H{"plugins": []string{}})
	})

	// Counter settings (manager-only via counter:settings).
	g.PUT("/businesses/:id/counters/settings", RequireOperationalBusiness(), RoleBasedAccessMiddleware("counter:settings"), UpdateCounterSettings)
}
