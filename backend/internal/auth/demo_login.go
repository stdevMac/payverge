package auth

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/demomode"
	"github.com/stdevmac/payverge/backend/internal/server"
	"gorm.io/gorm"
)

type demoLoginRequest struct {
	Role string `json:"role"`
}

// DemoLogin is the public demo's one-click sign-in:
//
//	POST /api/v1/auth/demo/login {"role": "owner" | "kitchen" | "waiter"}
//
// It issues a normal session for one of the fixed showroom identities: the
// non-admin showroom owner, or a seeded staff member of the core demo venue.
// No password exists or is published. Unless DEMO_MODE is on it answers 404,
// exactly like an unknown route, so a normal install has no such door. It
// never signs anyone in as a platform admin.
func (h *AuthHandler) DemoLogin(c *gin.Context) {
	if !config.DemoModeEnabled() {
		c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
		return
	}
	var req demoLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Choose owner, kitchen or waiter.")
		return
	}
	role := strings.ToLower(strings.TrimSpace(req.Role))
	ctx := c.Request.Context()

	owner, err := demomode.FindShowroomOwner(ctx, h.db)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			server.RespondWithError(c, http.StatusServiceUnavailable, server.ErrCodeInternal, "The demo is still being prepared. Try again in a minute.")
			return
		}
		log.Printf("[DemoLogin] load showroom owner: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not start the demo")
		return
	}
	if strings.EqualFold(owner.Role, "admin") {
		// EnsureShowroomOwner refuses this at boot; never hand out an admin.
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "The demo owner account is misconfigured")
		return
	}

	if role == "owner" {
		token, err := h.mintOperatorSession(c, owner, demomode.SessionProvider)
		if err != nil {
			log.Printf("[DemoLogin] mint owner session: %v", err)
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not start the demo")
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success":  true,
			"token":    token,
			"role":     "owner",
			"redirect": "/dashboard",
			"user": gin.H{
				"id":    owner.ID,
				"email": owner.Email,
				"name":  owner.Name,
				"role":  owner.Role,
			},
		})
		return
	}

	staffRole, ok := demomode.StaffRoleFor(role)
	if !ok {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Choose owner, kitchen or waiter.")
		return
	}
	staff, err := demomode.ShowroomStaff(ctx, h.db, owner.ID, staffRole)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			server.RespondWithError(c, http.StatusServiceUnavailable, server.ErrCodeInternal, "The demo is still being prepared. Try again in a minute.")
			return
		}
		log.Printf("[DemoLogin] load showroom staff: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not start the demo")
		return
	}
	if err := server.IssueDemoStaffSession(c, staff); err != nil {
		log.Printf("[DemoLogin] staff session: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Could not start the demo")
	}
}
