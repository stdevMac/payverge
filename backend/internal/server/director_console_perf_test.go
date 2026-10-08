package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupDirectorConsolePerfTestDB(t testing.TB, gormLogger logger.Interface) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	cfg := &gorm.Config{}
	if gormLogger != nil {
		cfg.Logger = gormLogger
	}
	gormDB, err := gorm.Open(sqlite.Open(dsn), cfg)
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
	})

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.DirectorConsoleThread{},
		&database.DirectorConsoleMessage{},
	))

	return gormDB
}

func createDirectorConsolePerfBusiness(t testing.TB, suffix string) *database.Business {
	t.Helper()

	business := &database.Business{
		BusinessId:      fmt.Sprintf("director-biz-%s", suffix),
		Name:            fmt.Sprintf("Director Biz %s", suffix),
		OwnerAddress:    "0xOwnerA",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
		DefaultLanguage: "en",
		SourceLanguage:  "en",
		IsActive:        true,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	require.NoError(t, database.GetDB().Create(business).Error)
	return business
}

func createDirectorConsolePerfThread(t testing.TB, businessID uint, title string) *database.DirectorConsoleThread {
	t.Helper()

	thread, err := database.CreateDirectorConsoleThread(businessID, title, "en")
	require.NoError(t, err)
	return thread
}

func seedDirectorConsoleMessages(t testing.TB, businessID, threadID uint, count int) {
	t.Helper()

	base := time.Now().Add(-24 * time.Hour)
	largeStructured := `{"summary":"` + strings.Repeat("structured-response ", 64) + `","diagnosis":"ok","evidence":["a"],"actions":[],"expected_impact":"medium","follow_ups":[]}`
	messages := make([]database.DirectorConsoleMessage, 0, count)
	for i := 0; i < count; i++ {
		role := database.DirectorMessageRoleUser
		if i%2 == 1 {
			role = database.DirectorMessageRoleAssistant
		}
		messages = append(messages, database.DirectorConsoleMessage{
			ThreadID:           threadID,
			BusinessID:         businessID,
			Role:               role,
			Locale:             "en",
			Content:            fmt.Sprintf("Director Message %03d %s", i, strings.Repeat("content ", 32)),
			StructuredResponse: largeStructured,
			ModelName:          "gemini-2.0-flash",
			LatencyMs:          int64(100 + i),
			CreatedAt:          base.Add(time.Duration(i) * time.Second),
			UpdatedAt:          base.Add(time.Duration(i) * time.Second),
		})
	}
	require.NoError(t, database.GetDB().Create(&messages).Error)
}

func directorConsoleMessagesRouter() *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xOwnerA")
		c.Next()
	})
	router.GET("/businesses/:id/ai/director/threads/:threadId/messages", GetDirectorThreadMessages)
	return router
}

func performDirectorConsoleRequest(t testing.TB, router *gin.Engine, method, path string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestGetDirectorThreadMessagesBoundsAndProjectsTranscript(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupDirectorConsolePerfTestDB(t, recorder)

	business := createDirectorConsolePerfBusiness(t, "history-bound")
	thread := createDirectorConsolePerfThread(t, business.ID, "History Bound")
	seedDirectorConsoleMessages(t, business.ID, thread.ID, 250)

	prevService := directorConsoleService
	directorConsoleService = services.NewDirectorConsoleService(database.GetDBWrapper(), nil, nil, nil)
	t.Cleanup(func() {
		directorConsoleService = prevService
	})

	recorder.statements = nil
	router := directorConsoleMessagesRouter()
	w := performDirectorConsoleRequest(t, router, http.MethodGet, fmt.Sprintf("/businesses/%d/ai/director/threads/%d/messages", business.ID, thread.ID))

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Messages []services.DirectorMessageDTO `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Messages, 200)
	require.Equal(t, "Director Message 050 "+strings.Repeat("content ", 32), resp.Messages[0].Content)
	require.Equal(t, "Director Message 249 "+strings.Repeat("content ", 32), resp.Messages[len(resp.Messages)-1].Content)
	require.Zero(t, recorder.selectStarCount("director_console_messages"), "director transcript should project displayed fields only")
}

func BenchmarkGetDirectorThreadMessagesSQLite(b *testing.B) {
	gin.SetMode(gin.TestMode)
	setupDirectorConsolePerfTestDB(b, logger.Default.LogMode(logger.Silent))

	business := createDirectorConsolePerfBusiness(b, "messages-bench")
	thread := createDirectorConsolePerfThread(b, business.ID, "Messages Bench")
	seedDirectorConsoleMessages(b, business.ID, thread.ID, 1000)

	prevService := directorConsoleService
	directorConsoleService = services.NewDirectorConsoleService(database.GetDBWrapper(), nil, nil, nil)
	b.Cleanup(func() {
		directorConsoleService = prevService
	})

	router := directorConsoleMessagesRouter()
	path := fmt.Sprintf("/businesses/%d/ai/director/threads/%d/messages", business.ID, thread.ID)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := performDirectorConsoleRequest(b, router, http.MethodGet, path)
		if w.Code != http.StatusOK {
			b.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}
	}
}
