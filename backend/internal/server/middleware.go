package server

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"web3-boilerplate/internal/metrics"
	"net/http"
	"strings"
)

// AuthenticationMiddleware checks if the user has a valid JWT token
func AuthenticationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := c.GetHeader("Authorization")
		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing authentication token"})
			c.Abort()
			return
		}

		// The token should be prefixed with "Bearer "
		tokenParts := strings.Split(tokenString, " ")
		if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
			c.Abort()
			return
		}

		tokenString = tokenParts[1]
		// Remove the \"\" from the token string
		tokenString = tokenString[1 : len(tokenString)-1]
		claims, err := VerifyToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
			c.Abort()
			return
		}

		c.Set("user_id", claims["user_id"])
		c.Set("address", claims["address"])
		c.Next()
	}
}

// AuthenticationAdminMiddleware checks if the user has a valid JWT token and if is an admin
func AuthenticationAdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := c.GetHeader("Authorization")
		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing authentication token"})
			c.Abort()
			return
		}

		// The token should be prefixed with "Bearer "
		tokenParts := strings.Split(tokenString, " ")
		if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
			c.Abort()
			return
		}

		tokenString = tokenParts[1]
		// Remove the \"\" from the token string
		tokenString = tokenString[1 : len(tokenString)-1]
		claims, err := VerifyToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authentication token"})
			c.Abort()
			return
		}

		if claims["role"] != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			c.Abort()
			return
		}

		c.Set("user_id", claims["user_id"])
		c.Set("address", claims["address"])
		c.Next()
	}
}

func PrometheusMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		status := fmt.Sprintf("%d", c.Writer.Status())
		metrics.TotalRequests.WithLabelValues(c.FullPath(), c.Request.Method, status).Inc()
	}
}
