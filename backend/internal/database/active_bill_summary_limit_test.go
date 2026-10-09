package database

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"
)

func TestGetActiveBillSummariesByBusinessIDBoundedClampsHugeLimit(t *testing.T) {
	recorder := &activeBillCountSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	wrapped, bizID := setupActiveBillCountDB(t, recorder)
	recorder.reset()

	_, _, err := wrapped.GetActiveBillSummariesByBusinessIDBounded(bizID, 1_000_000)
	require.NoError(t, err)

	joined := strings.ToUpper(strings.Join(recorder.statements, "\n"))
	assert.Contains(t, joined, "LIMIT 201", "oversized limit must be clamped before SQL; got %s", joined)
}
