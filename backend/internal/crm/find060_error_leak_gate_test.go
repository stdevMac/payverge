package crm

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// FIND-060: CRM INTERNAL responses must not forward err.Error() (GORM/driver leaks).
func TestFIND060_CRMHandlers_NoRawInternalErrError(t *testing.T) {
	re := regexp.MustCompile(`ErrCodeInternal,\s*err\.Error\(\)`)
	src, err := os.ReadFile(filepath.Join(".", "handlers.go"))
	require.NoError(t, err)
	require.False(t, re.Match(src), "crm handlers still pass err.Error() as INTERNAL message (FIND-060)")
}
