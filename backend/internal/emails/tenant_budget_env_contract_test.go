package emails

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// tenantMailEnvKeys is every variable TenantMailBudgetConfigFromEnv reads.
var tenantMailEnvKeys = []string{
	envTenantMailBusinessDailyCap,
	envTenantMailDemoDailyCap,
	envTenantMailRecipientDailyCap,
	envTenantMailRecipientGlobalDailyCap,
	envTenantMailRecipientGlobalAllDailyCap,
	envTenantMailReservationDailyCap,
	envTenantMailReservationRecipientDailyCap,
	envTenantMailDedupeMinutes,
}

// Compose only injects variables its environment: block names, so a cap that
// is not forwarded can never be tuned on a compose deploy even when it is set
// in the root .env. Every EMAIL_TENANT_* key must be forwarded to the backend
// by docker-compose.yml (the only compose file the open-source tree ships)
// and documented in .env.example, with the documented defaults matching the
// code.
func TestTenantMailBudgetEnv_ForwardedByComposeAndDocumented(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))

	for _, rel := range []string{"docker-compose.yml"} {
		raw, err := os.ReadFile(filepath.Join(repoRoot, rel))
		require.NoError(t, err, rel)
		backend := composeServiceBlock(t, string(raw), "backend")
		for _, key := range tenantMailEnvKeys {
			want := fmt.Sprintf("- %s=${%s:-}", key, key)
			require.Containsf(t, backend, want,
				"%s backend environment must forward %s (blank = code default)", rel, key)
		}
	}

	raw, err := os.ReadFile(filepath.Join(repoRoot, ".env.example"))
	require.NoError(t, err)
	envExample := string(raw)
	for _, key := range tenantMailEnvKeys {
		require.Regexpf(t, regexp.MustCompile(`(?m)^`+regexp.QuoteMeta(key)+`=$`), envExample,
			".env.example must document %s as a blank (code default) entry", key)
	}
	for _, phrase := range []string{
		fmt.Sprintf("Defaults: standard %d, demo %d.",
			defaultTenantMailBusinessDailyCap, defaultTenantMailDemoDailyCap),
		fmt.Sprintf("Default %d.", defaultTenantMailRecipientDailyCap),
		fmt.Sprintf("Default %d (clamped to tier cap / %d).",
			defaultTenantMailReservationDailyCap, tenantMailGuestLaneShareDivisor),
		fmt.Sprintf("guest-booking lane. Default %d.", defaultTenantMailReservationRecipientCap),
		fmt.Sprintf("receipts. Default %d.", defaultTenantMailRecipientGlobalDailyCap),
		fmt.Sprintf("any purpose. Default %d.", defaultTenantMailRecipientGlobalAllCap),
		fmt.Sprintf("Default %d (minutes).", int(defaultTenantMailDedupeWindow.Minutes())),
		// The window semantics bumpTenantMailCounter implements (review L2).
		"a 24-hour window that opens with the first counted send",
		"it is not a UTC calendar day",
	} {
		require.Containsf(t, envExample, phrase, ".env.example tenant budget defaults drifted from the code")
	}
}

// composeServiceBlock returns the text of one top-level service in a compose
// file: from its "  <name>:" line to the next two-space-indented key.
func composeServiceBlock(t *testing.T, body, service string) string {
	t.Helper()
	lines := strings.Split(body, "\n")
	start := -1
	for i, line := range lines {
		if line == "  "+service+":" {
			start = i
			break
		}
	}
	require.NotEqualf(t, -1, start, "service %q not found", service)
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		if len(line) > 2 && strings.HasPrefix(line, "  ") && line[2] != ' ' && line[2] != '#' {
			end = i
			break
		}
		if len(line) > 0 && line[0] != ' ' && line[0] != '#' {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}
