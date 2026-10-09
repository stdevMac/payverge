package main

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

func TestSettingsImageLimitsShowsDefaultsOnAFreshInstall(t *testing.T) {
	db := newCLITestDB(t, &database.PlatformSettings{})
	h := newCLIHarness(t, db, "", nil)

	require.Equal(t, cliExitOK, h.run("settings", "image-limits"), h.stderr.String())
	require.Contains(t, h.stdout.String(), "daily_limit=500 monthly_alert=2000")
	require.NotContains(t, h.stdout.String(), "updated")

	var rows int64
	require.NoError(t, db.Model(&database.PlatformSettings{}).Count(&rows).Error)
	require.Zero(t, rows, "showing the limits must not write settings")
}

func TestSettingsImageLimitsUpdatesOneValueAndKeepsTheOther(t *testing.T) {
	db := newCLITestDB(t, &database.PlatformSettings{})
	h := newCLIHarness(t, db, "", nil)

	require.Equal(t, cliExitOK, h.run("settings", "image-limits", "--daily", "40"), h.stderr.String())
	require.Contains(t, h.stdout.String(), "daily_limit=40 monthly_alert=2000")

	h = newCLIHarness(t, db, "", nil)
	require.Equal(t, cliExitOK, h.run("settings", "image-limits", "--monthly-alert", "900"), h.stderr.String())
	require.Contains(t, h.stdout.String(), "daily_limit=40 monthly_alert=900")

	got, err := database.NewDBWithConn(db).GetImageLimitSettings()
	require.NoError(t, err)
	require.Equal(t, database.ImageLimitSettings{DailyLimit: 40, MonthlyAlert: 900}, got)
}

func TestSettingsImageLimitsRejectsNonPositiveBeforeTouchingTheDatabase(t *testing.T) {
	for _, args := range [][]string{
		{"--daily", "0"},
		{"--daily", "-3"},
		{"--monthly-alert", "0"},
		{"--daily", "nope"},
	} {
		h := newCLIHarness(t, nil, "", nil)
		code := h.run(append([]string{"settings", "image-limits"}, args...)...)
		require.Equal(t, cliExitUsage, code, "%v: %s", args, h.stderr.String())
		require.Zero(t, h.opened, "%v must fail before opening the database", args)
	}
}
