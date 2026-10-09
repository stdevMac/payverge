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

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupAIMenuWizardPerfTestDB(t testing.TB, gormLogger logger.Interface) *gorm.DB {
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
		&database.MenuWizardSession{},
		&database.MenuWizardMessage{},
	))

	return gormDB
}

func createAIMenuWizardPerfBusiness(t testing.TB, suffix string) *database.Business {
	t.Helper()

	business := &database.Business{
		BusinessId:      fmt.Sprintf("ai-menu-wizard-%s", suffix),
		Name:            fmt.Sprintf("AI Menu Wizard %s", suffix),
		OwnerAddress:    "0xowner",
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

func createAIMenuWizardPerfSession(t testing.TB, businessID uint) *database.MenuWizardSession {
	t.Helper()

	session := &database.MenuWizardSession{
		BusinessID: businessID,
		Status:     database.WizardStatusInProgress,
		Config:     `{"cuisine":"test"}`,
		CreatedAt:  time.Now().Add(-24 * time.Hour),
		UpdatedAt:  time.Now(),
	}
	require.NoError(t, database.GetDB().Create(session).Error)
	return session
}

func seedAIMenuWizardMessages(t testing.TB, sessionID uint, count int) {
	t.Helper()

	base := time.Now().Add(-12 * time.Hour)
	messages := make([]database.MenuWizardMessage, 0, count)
	for i := 0; i < count; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		messages = append(messages, database.MenuWizardMessage{
			SessionID: sessionID,
			Role:      role,
			Content:   fmt.Sprintf("Wizard Message %03d %s", i, strings.Repeat("menu-detail ", 32)),
			CreatedAt: base.Add(time.Duration(i) * time.Second),
		})
	}
	require.NoError(t, database.GetDB().Create(&messages).Error)
}

func aiMenuWizardSessionRouter() *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("address", "0xowner")
		c.Next()
	})
	router.GET("/businesses/:id/ai/wizard/:sessionId", GetWizardSession)
	return router
}

func performAIMenuWizardRequest(t testing.TB, router *gin.Engine, method, path string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestGetWizardSessionBoundsAndProjectsMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupAIMenuWizardPerfTestDB(t, recorder)

	business := createAIMenuWizardPerfBusiness(t, "history-bound")
	session := createAIMenuWizardPerfSession(t, business.ID)
	seedAIMenuWizardMessages(t, session.ID, 250)

	recorder.statements = nil
	router := aiMenuWizardSessionRouter()
	w := performAIMenuWizardRequest(t, router, http.MethodGet, fmt.Sprintf("/businesses/%s/ai/wizard/%d", business.BusinessId, session.ID))

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Messages []struct {
			Role      string    `json:"role"`
			Content   string    `json:"content"`
			CreatedAt time.Time `json:"created_at"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Messages, 100)
	require.Equal(t, "Wizard Message 150 "+strings.Repeat("menu-detail ", 32), resp.Messages[0].Content)
	require.Equal(t, "Wizard Message 249 "+strings.Repeat("menu-detail ", 32), resp.Messages[len(resp.Messages)-1].Content)
	require.Equal(t, 1, recorder.selectCount("menu_wizard_messages"), "wizard session read should not preload messages and then query them again")
	require.Zero(t, recorder.selectStarCount("menu_wizard_messages"), "wizard session read should project displayed message fields only")
}

func BenchmarkGetWizardSessionSQLite(b *testing.B) {
	gin.SetMode(gin.TestMode)
	setupAIMenuWizardPerfTestDB(b, logger.Default.LogMode(logger.Silent))

	business := createAIMenuWizardPerfBusiness(b, "messages-bench")
	session := createAIMenuWizardPerfSession(b, business.ID)
	seedAIMenuWizardMessages(b, session.ID, 1000)

	router := aiMenuWizardSessionRouter()
	path := fmt.Sprintf("/businesses/%s/ai/wizard/%d", business.BusinessId, session.ID)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := performAIMenuWizardRequest(b, router, http.MethodGet, path)
		if w.Code != http.StatusOK {
			b.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}
	}
}
