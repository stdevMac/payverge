package auth

import (
	"os"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/config"
)

// TestMain pins EMAIL_VERIFICATION=required for the package. With no email
// env at all, EMAIL_VERIFICATION=auto resolves to "off" (the log provider is
// selected), which would silently flip every existing verified-email
// boundary test. Mode-specific tests override it with t.Setenv.
func TestMain(m *testing.M) {
	_ = os.Setenv(config.EmailVerificationEnv, string(config.EmailVerificationModeRequired))
	os.Exit(m.Run())
}
