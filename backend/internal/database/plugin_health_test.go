package database

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPluginHealthTestDB(t *testing.T) {
	t.Helper()
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&Business{}, &Plugin{}, &BusinessPlugin{}))
	SetTestDB(gormDB)
}

func TestBusinessPluginWebhookHealthRoundTrip(t *testing.T) {
	setupPluginHealthTestDB(t)

	plugin := paymentPlugin(t, "stripe")
	const bizID = uint(7)
	require.NoError(t, EnableBusinessPlugin(bizID, plugin.ID, map[string]interface{}{}))

	// A failure stamps status + error + error_at, and surfaces in the projection.
	require.NoError(t, RecordBusinessPluginWebhookFailure(bizID, "stripe", "signature rejected"))
	rows, err := GetBusinessPlugins(bizID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "error", rows[0]["last_status"])
	require.Equal(t, "signature rejected", rows[0]["last_error"])
	require.NotNil(t, rows[0]["last_error_at"])
	require.Nil(t, rows[0]["last_success_at"])

	// A subsequent success clears the error and stamps success_at.
	// FIND-059: idle empty last_error is omitted from the list projection.
	require.NoError(t, RecordBusinessPluginWebhookSuccess(bizID, "stripe"))
	rows, err = GetBusinessPlugins(bizID)
	require.NoError(t, err)
	require.Equal(t, "ok", rows[0]["last_status"])
	_, hasLastError := rows[0]["last_error"]
	require.False(t, hasLastError, "empty last_error must be omitted after success")
	_, hasLastErrorAt := rows[0]["last_error_at"]
	require.False(t, hasLastErrorAt, "null last_error_at must be omitted after success")
	require.NotNil(t, rows[0]["last_success_at"])
}

// Health writes are scoped to the (business, plugin) row: another business's
// row for the same plugin name is never touched, and an unknown plugin name is
// a harmless no-op rather than an error.
func TestBusinessPluginWebhookHealthIsScoped(t *testing.T) {
	setupPluginHealthTestDB(t)

	plugin := paymentPlugin(t, "stripe")
	require.NoError(t, EnableBusinessPlugin(1, plugin.ID, map[string]interface{}{}))
	require.NoError(t, EnableBusinessPlugin(2, plugin.ID, map[string]interface{}{}))

	require.NoError(t, RecordBusinessPluginWebhookFailure(1, "stripe", "boom"))

	rows2, err := GetBusinessPlugins(2)
	require.NoError(t, err)
	require.Len(t, rows2, 1)
	// Untouched row has empty health — FIND-059 omits idle status/error keys.
	_, hasStatus := rows2[0]["last_status"]
	require.False(t, hasStatus, "business 2 health must be untouched (no last_status)")
	_, hasErr := rows2[0]["last_error"]
	require.False(t, hasErr, "business 2 health must be untouched (no last_error)")

	// Unknown plugin name: no matching row, no error.
	require.NoError(t, RecordBusinessPluginWebhookFailure(1, "does-not-exist", "boom"))
}

// FIND-059: healthy / idle plugin list rows must not invent blank last_error.
func TestGetBusinessPlugins_OmitsEmptyHealthFields(t *testing.T) {
	setupPluginHealthTestDB(t)

	plugin := paymentPlugin(t, "stripe")
	require.NoError(t, EnableBusinessPlugin(9, plugin.ID, map[string]interface{}{}))

	rows, err := GetBusinessPlugins(9)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	for _, banned := range []string{"last_error", "last_error_at", "last_success_at"} {
		if _, ok := rows[0][banned]; ok {
			t.Fatalf("idle health field %q must be omitted, got %v", banned, rows[0][banned])
		}
	}
	// last_status is also empty on a never-webhooks row.
	if _, ok := rows[0]["last_status"]; ok {
		t.Fatalf("empty last_status must be omitted, got %v", rows[0]["last_status"])
	}
}

// Issue #795: a disabled rail must not read as ok/ready. All demo payment
// plugins shipped is_enabled=false with last_status="ok", so the operator
// wire advertised healthy rails on a venue where every rail was off. Stale
// success health from when the rail was enabled is dropped for disabled
// rows; a recorded error stays visible — it is a factual failure record,
// not a readiness claim.
func TestGetBusinessPlugins_DisabledRailDoesNotReadOk(t *testing.T) {
	setupPluginHealthTestDB(t)

	plugin := paymentPlugin(t, "mercadopago")
	const bizID = uint(86)
	require.NoError(t, EnableBusinessPlugin(bizID, plugin.ID, map[string]interface{}{}))
	require.NoError(t, RecordBusinessPluginWebhookSuccess(bizID, "mercadopago"))
	require.NoError(t, DisableBusinessPlugin(bizID, plugin.ID))

	rows, err := GetBusinessPlugins(bizID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	if s, ok := rows[0]["last_status"].(string); ok {
		require.NotEqual(t, "ok", s,
			"a disabled rail must not read last_status=ok on the operator wire (#795)")
	}
	_, hasSuccessAt := rows[0]["last_success_at"]
	require.False(t, hasSuccessAt,
		"stale success health on a disabled rail reads as ready — must be omitted (#795)")
}

// A disabled rail keeps its recorded error: suppressing failures would be a
// different lie. Only the ok/ready claim is scrubbed.
func TestGetBusinessPlugins_DisabledRailKeepsRecordedError(t *testing.T) {
	setupPluginHealthTestDB(t)

	plugin := paymentPlugin(t, "stripe")
	const bizID = uint(87)
	require.NoError(t, EnableBusinessPlugin(bizID, plugin.ID, map[string]interface{}{}))
	require.NoError(t, RecordBusinessPluginWebhookFailure(bizID, "stripe", "signature rejected"))
	require.NoError(t, DisableBusinessPlugin(bizID, plugin.ID))

	rows, err := GetBusinessPlugins(bizID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "error", rows[0]["last_status"])
	require.Equal(t, "signature rejected", rows[0]["last_error"])
}

func TestHasEnabledPaymentPlugin(t *testing.T) {
	setupPluginHealthTestDB(t)

	plugin := paymentPlugin(t, "stripe")
	require.NoError(t, EnableBusinessPlugin(3, plugin.ID, map[string]interface{}{
		"secret_key": "sk_live_should_not_be_read",
	}))

	has, err := HasEnabledPaymentPlugin(nil, 3)
	require.NoError(t, err)
	require.True(t, has)

	has, err = HasEnabledPaymentPlugin(nil, 99)
	require.NoError(t, err)
	require.False(t, has)
}
