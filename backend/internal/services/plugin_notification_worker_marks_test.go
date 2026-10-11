package services

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/logger"
)

// A failed outbox mark write must be LOUD: if MarkDelivered/retry/failed/dropped
// silently fails, the whole batch is redelivered after the stale-processing
// reclaim — up to BatchSize duplicate Telegram sends. The helper logs at error
// level with the delivery ids and count.
func TestLogPluginNotificationMarkFailure(t *testing.T) {
	var buf bytes.Buffer
	prevOut := logger.Logger.Out
	logger.Logger.SetOutput(&buf)
	defer logger.Logger.SetOutput(prevOut)

	// nil error → nothing logged.
	logPluginNotificationMarkFailure("telegram", "delivered", []uint{1, 2}, nil)
	require.Empty(t, buf.String())

	logPluginNotificationMarkFailure("telegram", "delivered", []uint{7, 8, 9}, errors.New("db connection lost"))
	out := buf.String()
	require.NotEmpty(t, out, "mark failure must be logged")
	require.Contains(t, strings.ToLower(out), "error", "logged at error level")
	require.Contains(t, out, "telegram")
	require.Contains(t, out, "delivered")
	require.Contains(t, out, "3", "delivery count present")
	require.Contains(t, out, "7", "delivery ids present")
	require.Contains(t, out, "db connection lost")
}
