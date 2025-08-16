package middleware

import (
	"net/http"
	"regexp"
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

// EthereumAddressValidation validates Ethereum addresses
func EthereumAddressValidation(paramName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		address := c.Param(paramName)
		if address == "" {
			address = c.Query(paramName)
		}

		if address != "" && !isValidEthereumAddress(address) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid Ethereum address format",
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
				"error": "Request payload too large",
				"max_size": maxSize,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// containsMaliciousPatterns checks for common attack patterns
func containsMaliciousPatterns(input string) bool {
	// Convert to lowercase for case-insensitive matching
	lower := strings.ToLower(input)
	
	// SQL injection patterns
	sqlPatterns := []string{
		"union select", "drop table", "delete from", "insert into",
		"update set", "exec(", "execute(", "sp_", "xp_",
		"'or'1'='1", "' or 1=1", "admin'--", "' union",
	}
	
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

	allPatterns := append(sqlPatterns, xssPatterns...)
	allPatterns = append(allPatterns, pathPatterns...)
	allPatterns = append(allPatterns, cmdPatterns...)

	for _, pattern := range allPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	return false
}

// isValidEthereumAddress validates Ethereum address format
func isValidEthereumAddress(address string) bool {
	// Ethereum addresses are 42 characters long (including 0x prefix)
	if len(address) != 42 {
		return false
	}

	// Must start with 0x
	if !strings.HasPrefix(strings.ToLower(address), "0x") {
		return false
	}

	// Must contain only hexadecimal characters after 0x
	hexPattern := regexp.MustCompile("^0x[0-9a-fA-F]{40}$")
	return hexPattern.MatchString(address)
}

// SanitizeInput removes potentially dangerous characters
func SanitizeInput(input string) string {
	// Remove null bytes
	input = strings.ReplaceAll(input, "\x00", "")
	
	// Remove control characters except newline and tab
	var result strings.Builder
	for _, r := range input {
		if r >= 32 || r == '\n' || r == '\t' {
			result.WriteRune(r)
		}
	}
	
	return result.String()
}
