package database

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

// staff_notifications.title is VARCHAR(255) and body is VARCHAR(500), but
// announcement titles are unbounded and the batch INSERT is all-or-nothing: a
// single oversized title used to nuke the entire inbox batch (while SSE + push
// still fired). CreateStaffNotifications must clamp rune-safely before insert.
func TestCreateStaffNotifications_ClampsOversizedTitleAndBody(t *testing.T) {
	db, cleanup := newStaffNotificationTestDB(t)
	defer cleanup()

	longTitle := strings.Repeat("é", 600) // 600 runes, multi-byte
	longBody := strings.Repeat("ü", 900)
	rows := []StaffNotification{{
		BusinessID: 1, StaffID: 1, Kind: "announcement",
		Title: longTitle, Body: longBody,
	}}
	require.NoError(t, db.CreateStaffNotifications(rows), "oversized title must not fail the batch insert")

	var got StaffNotification
	require.NoError(t, db.GetGorm().First(&got).Error)
	require.LessOrEqual(t, utf8.RuneCountInString(got.Title), 255, "title clamped to column limit (runes)")
	require.LessOrEqual(t, utf8.RuneCountInString(got.Body), 500, "body clamped to column limit (runes)")
	require.True(t, utf8.ValidString(got.Title), "clamp must not split a rune")
	require.True(t, utf8.ValidString(got.Body), "clamp must not split a rune")
	require.True(t, strings.HasSuffix(got.Title, "…"), "clamped title signals truncation")
	require.True(t, strings.HasSuffix(got.Body, "…"), "clamped body signals truncation")
	require.True(t, strings.HasPrefix(got.Title, "é"), "clamped title keeps original prefix")
}

// Short titles/bodies pass through untouched — no ellipsis, no mutation.
func TestCreateStaffNotifications_LeavesShortRowsUntouched(t *testing.T) {
	db, cleanup := newStaffNotificationTestDB(t)
	defer cleanup()

	rows := []StaffNotification{{
		BusinessID: 1, StaffID: 1, Kind: "announcement",
		Title: "Short title", Body: "Short body",
	}}
	require.NoError(t, db.CreateStaffNotifications(rows))

	var got StaffNotification
	require.NoError(t, db.GetGorm().First(&got).Error)
	require.Equal(t, "Short title", got.Title)
	require.Equal(t, "Short body", got.Body)
}
