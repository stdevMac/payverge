package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// InputValidation middleware for sanitizing and validating requests
func InputValidation() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Validate common attack patterns
		if containsMaliciousPatterns(c.Request.URL.Path) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid request format",
			})
			c.Abort()
			return
		}

		// Validate query parameters
		for key, values := range c.Request.URL.Query() {
			for _, value := range values {
				if containsMaliciousPatterns(value) {
					c.JSON(http.StatusBadRequest, gin.H{
						"error": "Invalid query parameter: " + key,
					})
					c.Abort()
					return
				}
			}
		}

		// Validate headers for common injection attacks
		userAgent := c.GetHeader("User-Agent")
		if userAgent != "" && containsMaliciousPatterns(userAgent) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid request headers",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// JSONSizeLimit limits the size of JSON payloads
func JSONSizeLimit(maxSize int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > maxSize {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error":    "Request payload too large",
				"max_size": maxSize,
			})
			c.Abort()
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSize)
		c.Next()
	}
}

// containsMaliciousPatterns checks for common attack patterns
func containsMaliciousPatterns(input string) bool {
	// Convert to lowercase for case-insensitive matching
	lower := strings.ToLower(input)

	// SQL-injection patterns removed: false-positive-prone on legitimate
	// inputs (names with apostrophes, search strings) and trivially bypassed
	// via whitespace/encoding. The real SQL-injection defense is GORM's
	// parameterized queries — see internal/database/*. Leaving the list in
	// provided a false sense of security.

	// XSS patterns
	xssPatterns := []string{
		"<script", "</script>", "javascript:", "vbscript:",
		"onload=", "onerror=", "onclick=", "onmouseover=",
		"eval(", "expression(", "url(javascript:",
	}

	// Path traversal patterns
	pathPatterns := []string{
		"../", "..\\", "..\\/", "..%2f", "..%5c",
		"%2e%2e%2f", "%2e%2e%5c", "....//", "....\\\\",
	}

	// Command injection patterns
	cmdPatterns := []string{
		"; cat ", "; ls ", "; rm ", "; wget ", "; curl ",
		"| cat ", "| ls ", "| rm ", "| wget ", "| curl ",
		"&& cat ", "&& ls ", "&& rm ", "&& wget ", "&& curl ",
	}

	allPatterns := append([]string{}, xssPatterns...)
	allPatterns = append(allPatterns, pathPatterns...)
	allPatterns = append(allPatterns, cmdPatterns...)

	for _, pattern := range allPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	return false
}
