package server

import (
	"strings"

	"github.com/gin-gonic/gin"
)

func operatorLocaleFromAcceptLanguage(header string) string {
	first := strings.Split(header, ",")[0]
	first = strings.TrimSpace(strings.Split(first, ";")[0])
	first = strings.ToLower(strings.ReplaceAll(first, "_", "-"))
	if first == "es-ar" || strings.HasPrefix(first, "es-ar-") {
		return "es-AR"
	}
	if first == "es" || strings.HasPrefix(first, "es-") {
		return "es"
	}
	return "en"
}

// AuthTokenMissingMessage localizes the unauthenticated 401 human message.
// The wire code stays AUTH_TOKEN_MISSING.
func AuthTokenMissingMessage(c *gin.Context) string {
	switch operatorLocaleFromAcceptLanguage(c.GetHeader("Accept-Language")) {
	case "es-AR":
		return "Iniciá sesión para continuar."
	case "es":
		return "Inicia sesión para continuar."
	default:
		return "Missing authentication token"
	}
}
