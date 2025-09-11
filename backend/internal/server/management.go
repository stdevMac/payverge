package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"payverge/internal/database"
)

// GetAllUsers Serve all users information
func GetAllUsers(c *gin.Context) {
	users, err := database.GetAllUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal server error",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"users": users,
	})
}
