package database

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// directorSQLRecorder captures SQL statements emitted by GORM for column-projection assertions.
type directorSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *directorSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func (r *directorSQLRecorder) reset() {
	r.statements = nil
}

// statementContains returns true if any recorded statement (normalised to lower-case)
// contains the given substring.
func (r *directorSQLRecorder) statementContains(sub string) bool {
	needle := strings.ToLower(sub)
	for _, s := range r.statements {
		if strings.Contains(strings.ToLower(s), needle) {
			return true
		}
	}
	return false
}

// setupDirectorSummariesTestDB opens an isolated in-memory SQLite DB, migrates
// the director tables, and returns (businessID, threadID). The supplied gorm
// logger is wired in so callers can capture SQL.
func setupDirectorSummariesTestDB(t testing.TB, gormLogger logger.Interface) (businessID uint, threadID uint) {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	cfg := &gorm.Config{}
	if gormLogger != nil {
		cfg.Logger = gormLogger
	}
	gormDB, err := gorm.Open(sqlite.Open(dsn), cfg)
	require.NoError(t, err, "open in-memory SQLite")

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	SetTestDB(gormDB)
	require.NoError(t, db.AutoMigrate(&Business{}, &DirectorConsoleThread{}, &DirectorConsoleMessage{}))

	biz := &Business{
		BusinessId:      fmt.Sprintf("director-summary-biz-%d", time.Now().UnixNano()),
		Name:            "Director Summary Test",
		OwnerAddress:    "0xDirectorSummaryOwner",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
	}
	require.NoError(t, db.Create(biz).Error)

	now := time.Now()
	thread := &DirectorConsoleThread{
		BusinessID:    biz.ID,
		Title:         "Test thread",
		Locale:        "en",
		LastMessageAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	require.NoError(t, db.Create(thread).Error)

	return biz.ID, thread.ID
}

// largeStructuredResponse is a ~4KB JSON blob mimicking a real director response.
var largeStructuredResponse = `{"actions":[` + strings.Repeat(`{"kind":"menu.adjust_prices","params":{"scope":"all","mode":"percent","value":10}},`, 40) + `{"kind":"noop"}],"summary":"` + strings.Repeat("x", 3000) + `"}`

// seedDirectorMessages inserts count messages (alternating user/assistant) into
// the given thread, each with a large StructuredResponse blob and a distinct Content.
func seedDirectorMessages(t testing.TB, bizID, threadID uint, count int) {
	t.Helper()
	base := time.Now().Add(-time.Duration(count) * time.Second)
	for i := 0; i < count; i++ {
		role := DirectorMessageRoleUser
		content := fmt.Sprintf("user message %d", i)
		structured := ""
		if i%2 == 1 {
			role = DirectorMessageRoleAssistant
			content = fmt.Sprintf("assistant summary %d", i)
			structured = largeStructuredResponse
		}
		msg := &DirectorConsoleMessage{
			BusinessID:         bizID,
			ThreadID:           threadID,
			Role:               role,
			Locale:             "en",
			Content:            content,
			StructuredResponse: structured,
			ModelName:          "gemini-2.5-flash",
			LatencyMs:          int64(100 + i),
			CreatedAt:          base.Add(time.Duration(i) * time.Second),
			UpdatedAt:          base.Add(time.Duration(i) * time.Second),
		}
		require.NoError(t, db.Create(msg).Error)
	}
}

// TestListDirectorConsoleMessageSummaries_OmitsStructuredResponse is the
// load-bearing access-shape guard for JSON-02: verifies that
// ListDirectorConsoleMessageSummaries does NOT hydrate the structured_response
// blob while Content, Role, and ID are present and returned in ascending order.
// A SQL-capture probe proves the column is absent from the generated SELECT.
func TestListDirectorConsoleMessageSummaries_OmitsStructuredResponse(t *testing.T) {
	recorder := &directorSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	bizID, threadID := setupDirectorSummariesTestDB(t, recorder)
	seedDirectorMessages(t, bizID, threadID, 6)

	recorder.reset() // only capture the query under test

	msgs, err := ListDirectorConsoleMessageSummaries(bizID, threadID, 12)
	require.NoError(t, err)
	require.Len(t, msgs, 6, "should return all 6 seeded messages")

	// StructuredResponse must NOT be hydrated.
	for i, m := range msgs {
		assert.Empty(t, m.StructuredResponse, "row %d: StructuredResponse must not be hydrated by the summaries loader", i)
	}

	// ID, Role, Content must be populated.
	for i, m := range msgs {
		assert.NotZero(t, m.ID, "row %d: ID must be set", i)
		assert.NotEmpty(t, m.Role, "row %d: Role must be set", i)
		assert.NotEmpty(t, m.Content, "row %d: Content must be set", i)
	}

	// Results must be in ascending chronological order.
	for i := 1; i < len(msgs); i++ {
		assert.True(t, !msgs[i].CreatedAt.Before(msgs[i-1].CreatedAt),
			"messages must be in ascending chronological order (index %d before %d)", i-1, i)
	}

	// SQL-capture: the generated SELECT must NOT contain structured_response.
	assert.False(t, recorder.statementContains("structured_response"),
		"ListDirectorConsoleMessageSummaries must not SELECT structured_response; got SQL: %v", recorder.statements)

	// Sanity: at least one SELECT was emitted for the query under test.
	assert.True(t, recorder.statementContains("director_console_messages"),
		"expected a SELECT against director_console_messages")
}

// TestListDirectorConsoleMessages_IncludesStructuredResponse proves the existing
// full loader DOES SELECT structured_response, confirming the column-split is real.
func TestListDirectorConsoleMessages_IncludesStructuredResponse(t *testing.T) {
	recorder := &directorSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	bizID, threadID := setupDirectorSummariesTestDB(t, recorder)
	seedDirectorMessages(t, bizID, threadID, 4)

	recorder.reset()

	msgs, err := ListDirectorConsoleMessages(bizID, threadID, 12)
	require.NoError(t, err)
	require.Len(t, msgs, 4)

	// At least one assistant message should have StructuredResponse hydrated.
	found := false
	for _, m := range msgs {
		if m.StructuredResponse != "" {
			found = true
			break
		}
	}
	assert.True(t, found, "ListDirectorConsoleMessages must hydrate StructuredResponse for assistant messages")

	// SQL must reference structured_response.
	assert.True(t, recorder.statementContains("structured_response"),
		"ListDirectorConsoleMessages must SELECT structured_response; got SQL: %v", recorder.statements)
}

// TestListDirectorConsoleMessageSummaries_AscendingOrder verifies the reversal
// logic: DESC-limit-then-reverse yields chronological order regardless of limit.
func TestListDirectorConsoleMessageSummaries_AscendingOrder(t *testing.T) {
	bizID, threadID := setupDirectorSummariesTestDB(t, logger.Default.LogMode(logger.Silent))
	seedDirectorMessages(t, bizID, threadID, 8)

	msgs, err := ListDirectorConsoleMessageSummaries(bizID, threadID, 5)
	require.NoError(t, err)
	require.Len(t, msgs, 5, "limit clamped to 5 should return exactly 5 messages")

	for i := 1; i < len(msgs); i++ {
		assert.True(t, !msgs[i].CreatedAt.Before(msgs[i-1].CreatedAt),
			"ascending order broken at index %d", i)
	}
}

// BenchmarkListDirectorConsoleMessages_Before measures the OLD full-column read
// (ListDirectorConsoleMessages with structured_response).
func BenchmarkListDirectorConsoleMessages_Before(b *testing.B) {
	bizID, threadID := setupDirectorSummariesTestDB(b, logger.Default.LogMode(logger.Silent))
	seedDirectorMessages(b, bizID, threadID, 12)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msgs, err := ListDirectorConsoleMessages(bizID, threadID, 12)
		if err != nil {
			b.Fatal(err)
		}
		if len(msgs) != 12 {
			b.Fatalf("expected 12 messages, got %d", len(msgs))
		}
	}
}

// BenchmarkListDirectorConsoleMessageSummaries_After measures the NEW
// column-minimal read (ListDirectorConsoleMessageSummaries without structured_response).
func BenchmarkListDirectorConsoleMessageSummaries_After(b *testing.B) {
	bizID, threadID := setupDirectorSummariesTestDB(b, logger.Default.LogMode(logger.Silent))
	seedDirectorMessages(b, bizID, threadID, 12)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msgs, err := ListDirectorConsoleMessageSummaries(bizID, threadID, 12)
		if err != nil {
			b.Fatal(err)
		}
		if len(msgs) != 12 {
			b.Fatalf("expected 12 messages, got %d", len(msgs))
		}
	}
}
