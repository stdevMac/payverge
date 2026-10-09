package handlers

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// FIND-060 residual: print jobs + fiscal compliance must not return raw
// err.Error() on InternalServerError (GORM/driver/provider text leaks).
func TestFIND060_PrintAndFiscal_NoRawInternalErrError(t *testing.T) {
	re := regexp.MustCompile(`c\.JSON\(http\.StatusInternalServerError,\s*gin\.H\{"error":\s*err\.Error\(\)\}\)`)
	files := []string{
		"print_job_handlers.go",
		"fiscal_handlers.go",
		"printer_handlers.go",
		"operational_alert_handlers.go",
	}
	for _, name := range files {
		path := filepath.Join(".", name)
		src, err := os.ReadFile(path)
		require.NoError(t, err, name)
		require.Falsef(t, re.Match(src),
			"%s still returns gin.H{\"error\": err.Error()} on 500 (FIND-060)", name)
	}
}
