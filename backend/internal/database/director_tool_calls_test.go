package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveAndListDirectorToolCalls(t *testing.T) {
	setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&DirectorConsoleThread{}, &DirectorConsoleMessage{}, &DirectorToolCall{}))

	thread, err := CreateDirectorConsoleThread(99, "test", "en")
	require.NoError(t, err)

	rec := &DirectorToolCall{
		ThreadID:   thread.ID,
		BusinessID: 99,
		ToolName:   "get_revenue_summary",
		ArgsJSON:   `{"period":"week"}`,
		Summary:    "Found 142 orders this week",
		DurationMs: 412,
		Success:    true,
	}
	require.NoError(t, SaveDirectorToolCall(rec))
	assert.NotZero(t, rec.ID)

	var rows []DirectorToolCall
	require.NoError(t, GetDB().Where("thread_id = ?", thread.ID).Order("id asc").Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, "get_revenue_summary", rows[0].ToolName)
	assert.Equal(t, "Found 142 orders this week", rows[0].Summary)
}
