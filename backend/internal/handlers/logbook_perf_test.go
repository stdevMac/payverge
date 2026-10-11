package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupLogbookPerfDB(t testing.TB, gormLogger logger.Interface) *LogbookHandler {
	t.Helper()
	cfg := &gorm.Config{}
	if gormLogger != nil {
		cfg.Logger = gormLogger
	}
	g, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), cfg)
	require.NoError(t, err)
	sqlDB, err := g.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, g.AutoMigrate(&database.Business{}, &database.Staff{}, &database.ShiftNote{}))
	database.SetTestDB(g)

	day := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 250; i++ {
		require.NoError(t, g.Create(&database.ShiftNote{
			BusinessID: 1, ForDate: day.Add(time.Duration(i) * time.Minute),
			AuthorStaffID: 7, Category: database.ShiftNoteCategoryOther, Content: "bench note",
		}).Error)
	}
	return NewLogbookHandler(database.GetDBWrapper())
}

func doLogbookList(t testing.TB, h *LogbookHandler) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/b/1/shift-notes?date=2026-06-30", nil)
	h.List(c)
	return w
}

func TestListShiftNotesAccessShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := &analyticsSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	h := setupLogbookPerfDB(t, rec)

	w := doLogbookList(t, h)
	require.Equal(t, http.StatusOK, w.Code)

	assert.Zero(t, rec.selectStarCount("shift_notes"), "logbook list must project columns, not SELECT *")
	assert.Equal(t, 1, rec.selectCount("shift_notes"), "logbook list must read in exactly one query (no N+1)")
}

func BenchmarkListShiftNotesSQLite(b *testing.B) {
	gin.SetMode(gin.TestMode)
	h := setupLogbookPerfDB(b, logger.Default.LogMode(logger.Silent))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if w := doLogbookList(b, h); w.Code != http.StatusOK {
			b.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
	}
}
