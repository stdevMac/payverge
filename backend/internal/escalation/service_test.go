package escalation

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/agents"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupEscalationDB(t *testing.T) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file:escalation_service_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	_ = gormDB.Migrator().DropTable(&database.Escalation{})
	require.NoError(t, gormDB.AutoMigrate(&database.Escalation{}))
	database.SetTestDB(gormDB)
}

func TestEscalatePersistsAndNotifiesTelegram(t *testing.T) {
	setupEscalationDB(t)

	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	oldBase := baseURL
	baseURL = srv.URL
	defer func() { baseURL = oldBase }()

	t.Setenv("TELEGRAM_ESCALATION_BOT_TOKEN", "test-token")
	t.Setenv("TELEGRAM_ESCALATION_CHAT_ID", "12345")

	svc := NewService(nil) // nil email → skip email fanout
	require.True(t, svc.telegram.Enabled())

	svc.Escalate(agents.EscalationEvent{
		Source:     "ops",
		BusinessID: 9,
		Issue:      "kds frozen",
		Transcript: "orders not printing",
		Reason:     "tool",
		ToolName:   "create_support_escalation",
	})

	require.Equal(t, int32(1), atomic.LoadInt32(&hits), "telegram notifier must be invoked")

	rows, total, err := database.ListEscalations(10, 0)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	require.Equal(t, database.EscalationSourceOps, rows[0].Source)
	require.NotNil(t, rows[0].BusinessID)
	require.Equal(t, uint(9), *rows[0].BusinessID)
	require.Equal(t, "kds frozen", rows[0].Issue)
	require.Equal(t, "open", rows[0].Status)
}

func TestTelegramNotifierDarkWhenUnconfigured(t *testing.T) {
	setupEscalationDB(t)
	t.Setenv("TELEGRAM_ESCALATION_BOT_TOKEN", "")
	t.Setenv("TELEGRAM_ESCALATION_CHAT_ID", "")

	svc := NewService(nil)
	require.False(t, svc.telegram.Enabled())

	// Silent no-op: no panic, row still persists.
	svc.Escalate(agents.EscalationEvent{Source: "ops", Issue: "hi", Reason: "loop_failure"})
	_, total, err := database.ListEscalations(10, 0)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
}
