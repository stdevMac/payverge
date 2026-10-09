package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// overlayEnv reads one "- KEY=value" line from the backend environment of
// deploy/demo/docker-compose.demo.yml.
func overlayEnv(t *testing.T, key string) (string, bool) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "demo", "docker-compose.demo.yml"))
	require.NoError(t, err)
	m := regexp.MustCompile(`(?m)^\s+- ` + regexp.QuoteMeta(key) + `=(.*)$`).FindSubmatch(raw)
	if m == nil {
		return "", false
	}
	return string(m[1]), true
}

// The public demo is a public, seeded showroom: the overlay must turn both
// switches on (DEMO_MODE refuses to start without DEMO_DATA).
func TestDemoOverlayTurnsOnDemoModeAndData(t *testing.T) {
	for _, key := range []string{"DEMO_MODE", "DEMO_DATA"} {
		v, ok := overlayEnv(t, key)
		require.True(t, ok, "%s missing from the demo overlay", key)
		require.Equal(t, "true", v, key)
	}
}

// The overlay must keep the hourly demo-day append running (the showroom's
// "today" moves between nightly resets) without giving every admin a
// showroom. "false" froze the showroom at the snapshot's day.
func TestDemoOverlayKeepsShowroomAdvancing(t *testing.T) {
	v, ok := overlayEnv(t, "ADMIN_DEMO_AUTOMATION_ENABLED")
	if !ok {
		v = "" // unset behaves like empty
	}
	plan := demoStartupPlanFrom(v, true)
	require.True(t, plan.Append, "hourly AppendDueDays must run on the public demo")
	require.True(t, plan.SeedOwner)
	require.False(t, plan.EnsureAllAdmins, "the public demo seeds one showroom, not one per admin")
}
