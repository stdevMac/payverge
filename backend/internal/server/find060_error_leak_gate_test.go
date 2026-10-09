package server

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// FIND-060: high-traffic menu/bill handlers must not return raw err.Error()
// on InternalServerError responses (GORM/driver text leaks).
func TestFIND060_HighTrafficHandlers_NoRawInternalErrError(t *testing.T) {
	// Match the exact anti-pattern we scrubbed. Leave product-safe domain
	// BadRequest messages that intentionally surface validation copy alone.
	re := regexp.MustCompile(`c\.JSON\(http\.StatusInternalServerError,\s*gin\.H\{"error":\s*err\.Error\(\)\}\)`)
	files := []string{
		"business_menu_core_handlers.go",
		"business_handlers.go",
		"inventory_handlers.go",
		"table_handlers.go",
		// FIND-060 residual expansion (print/fiscal/reservation/director)
		"reservation_handlers.go",
		"director_console_handler.go",
		"whatsapp_handlers.go",
		"ai_menu_handlers.go",
		"admin_email_handlers.go",
		"director_console_stream_handler.go",
	}
	for _, name := range files {
		path := filepath.Join(".", name)
		// tests run with cwd = package dir
		src, err := os.ReadFile(path)
		require.NoError(t, err, name)
		require.Falsef(t, re.Match(src),
			"%s still returns gin.H{\"error\": err.Error()} on 500 (FIND-060)", name)
	}
}
