package server

import (
	"net/http"

	"github.com/stdevmac/payverge/backend/internal/config"

	"github.com/gin-gonic/gin"
)

// GetPlatformRegistrationMode is the public GET
// /api/v1/platform/registration-mode probe. The signup UI reads it before a
// user exists to decide whether to show the signup form (open), require an
// invite code (invite), or show login only (closed).
func GetPlatformRegistrationMode(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"registration_mode": string(config.RegistrationMode()),
	})
}
