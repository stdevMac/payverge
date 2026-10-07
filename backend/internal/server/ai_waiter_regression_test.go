package server

import (
	"bytes"
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupAIWaiterTestDB(t testing.TB) *gorm.DB {
	return setupAIWaiterTestDBWithLogger(t, nil)
}

func setupAIWaiterTestDBWithLogger(t testing.TB, gormLogger logger.Interface) *gorm.DB {
	t.Helper()
	waitForPromotionTranslationBackfills(2 * time.Second)

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
		&database.Staff{},
		&database.StaffPermissionDeny{},
		&database.Table{},
		&database.Bill{},
		&database.Order{},
		&database.AiWaiterConversation{},
		&database.AiWaiterMessage{},
		&database.RBACAuditLog{},
		&database.StaffNotification{},
	))
	// bill_items: uuid default is Postgres-only; create with explicit schema for SQLite tests.
	require.NoError(t, gormDB.Exec(`CREATE TABLE IF NOT EXISTS bill_items (
		id TEXT PRIMARY KEY,
		bill_id INTEGER NOT NULL,
		menu_item_id TEXT DEFAULT '',
		name TEXT NOT NULL,
		price REAL NOT NULL,
		quantity INTEGER NOT NULL,
		options TEXT,
		item_type TEXT DEFAULT 'menu_item',
		bundle_id INTEGER,
		parent_bundle_id INTEGER,
		source_offer_id INTEGER,
		order_id INTEGER,
		subtotal REAL NOT NULL,
		created_at DATETIME
	)`).Error)
	InitializeRBAC(database.GetDBWrapper())

	// Test binaries share process memory: the insights cache is keyed by
	// business id, and fresh in-memory DBs reuse id=1, so wipe it per setup to
	// keep sibling tests isolated.
	aiInsightsCacheMu.Lock()
	aiInsightsCache = map[uint]cachedAiInsights{}
	aiInsightsCacheMu.Unlock()
	// Same for the in-memory guest AI per-network/per-device daily counters:
	// every httptest request shares one RemoteAddr.
	guestAIQuota.Reset()

	return gormDB
}

func createAIWaiterBusiness(t testing.TB, suffix string, aiEnabled bool) *database.Business {
	t.Helper()

	business := &database.Business{
		BusinessId:      fmt.Sprintf("ai-biz-%s", suffix),
		Name:            fmt.Sprintf("AI Biz %s", suffix),
		OwnerAddress:    fmt.Sprintf("0x%s", suffix),
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
		DefaultLanguage: "en",
		SourceLanguage:  "en",
		// Ordering-mode AI Waiter tests expect cart tools to remain valid when
		// the venue is "open". Zero-value Kitchen/Orders flags are false and
		// would strip every add_to_cart as Closed Mode browse-only.
		KitchenEnabled: true,
		OrdersEnabled:  true,
		AiSettings: database.BusinessAiSettings{
			AiEnabled: aiEnabled,
			AiName:    "Alfred",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, database.GetDB().Create(business).Error)
	if !aiEnabled {
		business.AiSettings.AiEnabled = false
		require.NoError(t, database.GetDB().Save(business).Error)
	}
	return business
}

func createAIWaiterStaff(t testing.TB, businessID uint, role database.StaffRole, email string) *database.Staff {
	t.Helper()

	staff := &database.Staff{
		BusinessID: businessID,
		Email:      email,
		Name:       string(role),
		Role:       role,
		IsActive:   true,
		InvitedBy:  "owner@example.com",
	}
	require.NoError(t, database.GetDB().Create(staff).Error)
	return staff
}

func createAIWaiterConversation(t testing.TB, businessID uint, sessionID string) *database.AiWaiterConversation {
	t.Helper()

	conv := &database.AiWaiterConversation{
		SessionID:  sessionID,
		BusinessID: businessID,
		TableCode:  "A1",
		Language:   "en",
		Mode:       "ordering",
		Status:     "active",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	require.NoError(t, database.GetDB().Create(conv).Error)
	return conv
}

func createAIWaiterTable(t testing.TB, businessID uint, tableCode string) *database.Table {
	t.Helper()

	table := &database.Table{
		BusinessID: businessID,
		Name:       tableCode,
		TableCode:  tableCode,
		Capacity:   4,
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)
	return table
}

func performAIWaiterRequest(t testing.TB, router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reqBody *bytes.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		require.NoError(t, err)
		reqBody = bytes.NewReader(payload)
	} else {
		reqBody = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reqBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestAIConversationRoutes_RequireConversationToBelongToRouteBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)

	ownedBusiness := createAIWaiterBusiness(t, "owned", true)
	otherBusiness := createAIWaiterBusiness(t, "other", true)
	manager := createAIWaiterStaff(t, ownedBusiness.ID, database.StaffRoleManager, "manager@example.com")

	ownedConv := createAIWaiterConversation(t, ownedBusiness.ID, "owned-session")
	otherConv := createAIWaiterConversation(t, otherBusiness.ID, "other-session")
	require.NoError(t, database.SaveAiWaiterMessage(ownedConv.ID, "assistant", "hello owned", ""))
	require.NoError(t, database.SaveAiWaiterMessage(otherConv.ID, "assistant", "hello other", ""))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleManager))
		c.Set("staff_id", manager.ID)
		c.Set("staff_business_id", ownedBusiness.ID)
		c.Next()
	})
	router.GET("/businesses/:id/ai/conversations/:convId/messages", RoleBasedAccessMiddleware("ai_waiter:read"), GetAiConversationMessages)
	router.POST("/businesses/:id/ai/conversations/:convId/pause", RoleBasedAccessMiddleware("ai_waiter:write"), ToggleAiPause)
	router.POST("/businesses/:id/ai/conversations/:convId/reply", RoleBasedAccessMiddleware("ai_waiter:write"), PostAiReply)
	router.POST("/businesses/:id/ai/conversations/:convId/close", RoleBasedAccessMiddleware("ai_waiter:write"), CloseAiConversation)

	t.Run("same business message read still works", func(t *testing.T) {
		w := performAIWaiterRequest(t, router, http.MethodGet, fmt.Sprintf("/businesses/%d/ai/conversations/%d/messages", ownedBusiness.ID, ownedConv.ID), nil)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("cross business message read is hidden", func(t *testing.T) {
		w := performAIWaiterRequest(t, router, http.MethodGet, fmt.Sprintf("/businesses/%d/ai/conversations/%d/messages", ownedBusiness.ID, otherConv.ID), nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("cross business pause is rejected", func(t *testing.T) {
		w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/businesses/%d/ai/conversations/%d/pause", ownedBusiness.ID, otherConv.ID), map[string]any{
			"is_paused": true,
		})
		assert.Equal(t, http.StatusNotFound, w.Code)

		var reloaded database.AiWaiterConversation
		require.NoError(t, database.GetDB().First(&reloaded, otherConv.ID).Error)
		assert.False(t, reloaded.IsPaused)
	})

	t.Run("cross business reply is rejected", func(t *testing.T) {
		var before int64
		require.NoError(t, database.GetDB().Model(&database.AiWaiterMessage{}).Where("conversation_id = ?", otherConv.ID).Count(&before).Error)

		w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/businesses/%d/ai/conversations/%d/reply", ownedBusiness.ID, otherConv.ID), map[string]any{
			"content": "manager reply",
		})
		assert.Equal(t, http.StatusNotFound, w.Code)

		var after int64
		require.NoError(t, database.GetDB().Model(&database.AiWaiterMessage{}).Where("conversation_id = ?", otherConv.ID).Count(&after).Error)
		assert.Equal(t, before, after)
	})

	t.Run("cross business close is rejected", func(t *testing.T) {
		w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/businesses/%d/ai/conversations/%d/close", ownedBusiness.ID, otherConv.ID), nil)
		assert.Equal(t, http.StatusNotFound, w.Code)

		var reloaded database.AiWaiterConversation
		require.NoError(t, database.GetDB().First(&reloaded, otherConv.ID).Error)
		assert.Equal(t, "active", reloaded.Status)
	})
}

func TestGetAiConversationMessagesBoundsAndProjectsTranscript(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupAIWaiterTestDBWithLogger(t, recorder)

	business := createAIWaiterBusiness(t, "staff-history-bound", true)
	manager := createAIWaiterStaff(t, business.ID, database.StaffRoleManager, "staff-history@example.com")
	conv := createAIWaiterConversation(t, business.ID, "session-staff-history")
	base := time.Now().Add(-4 * time.Hour)
	for i := 0; i < 250; i++ {
		require.NoError(t, database.GetDB().Create(&database.AiWaiterMessage{
			ConversationID: conv.ID,
			Role:           "assistant",
			Content:        fmt.Sprintf("Staff Message %03d", i),
			ToolCalls:      strings.Repeat("tool-payload", 32),
			CreatedAt:      base.Add(time.Duration(i) * time.Second),
		}).Error)
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleManager))
		c.Set("staff_id", manager.ID)
		c.Set("staff_business_id", business.ID)
		c.Next()
	})
	router.GET("/businesses/:id/ai/conversations/:convId/messages", RoleBasedAccessMiddleware("ai_waiter:read"), GetAiConversationMessages)

	recorder.statements = nil
	w := performAIWaiterRequest(t, router, http.MethodGet, fmt.Sprintf("/businesses/%d/ai/conversations/%d/messages", business.ID, conv.ID), nil)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var messages []database.AiWaiterMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &messages))
	require.Len(t, messages, 200)
	require.Equal(t, "Staff Message 050", messages[0].Content)
	require.Equal(t, "Staff Message 249", messages[len(messages)-1].Content)
	require.Zero(t, recorder.selectStarCount("ai_waiter_messages"), "staff transcript should project displayed fields only")
	require.False(t, recordedSelectMentionsColumn(recorder, "ai_waiter_messages", "tool_calls"), "staff transcript should not hydrate tool call payloads")
}

func TestGetAiWaiterMessages_RejectsDisabledOrMissingBusinessesWithoutCreatingConversation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)

	disabledBusiness := createAIWaiterBusiness(t, "disabled", false)
	inactiveBusiness := createAIWaiterBusiness(t, "inactive", true)
	inactiveBusiness.IsActive = false
	require.NoError(t, database.GetDB().Save(inactiveBusiness).Error)

	router := gin.New()
	router.GET("/ai-waiter/:businessId/messages", GetAiWaiterMessages)

	t.Run("disabled business returns forbidden and leaves no conversation", func(t *testing.T) {
		w := performAIWaiterRequest(t, router, http.MethodGet, fmt.Sprintf("/ai-waiter/%d/messages?session_token=session-disabled", disabledBusiness.ID), nil)
		assert.Equal(t, http.StatusForbidden, w.Code)

		var count int64
		require.NoError(t, database.GetDB().Model(&database.AiWaiterConversation{}).Where("business_id = ?", disabledBusiness.ID).Count(&count).Error)
		assert.Zero(t, count)
	})

	t.Run("missing business returns not found and creates nothing", func(t *testing.T) {
		w := performAIWaiterRequest(t, router, http.MethodGet, "/ai-waiter/999999/messages?session_token=session-missing", nil)
		assert.Equal(t, http.StatusNotFound, w.Code)

		var count int64
		require.NoError(t, database.GetDB().Model(&database.AiWaiterConversation{}).Count(&count).Error)
		assert.Zero(t, count)
	})

	t.Run("inactive business returns not found and creates nothing", func(t *testing.T) {
		w := performAIWaiterRequest(t, router, http.MethodGet, fmt.Sprintf("/ai-waiter/%d/messages?session_token=session-inactive", inactiveBusiness.ID), nil)
		assert.Equal(t, http.StatusNotFound, w.Code)

		var count int64
		require.NoError(t, database.GetDB().Model(&database.AiWaiterConversation{}).Where("business_id = ?", inactiveBusiness.ID).Count(&count).Error)
		assert.Zero(t, count)
	})
}

func TestGetAiWaiterMessagesBoundsAndProjectsHistory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupAIWaiterTestDBWithLogger(t, recorder)

	business := createAIWaiterBusiness(t, "history-bound", true)
	createAIWaiterTable(t, business.ID, "A1")
	conv := createAIWaiterConversation(t, business.ID, "session-history-bound")
	base := time.Now().Add(-3 * time.Hour)
	for i := 0; i < 150; i++ {
		require.NoError(t, database.GetDB().Create(&database.AiWaiterMessage{
			ConversationID: conv.ID,
			Role:           "assistant",
			Content:        fmt.Sprintf("Message %03d", i),
			ToolCalls:      strings.Repeat("tool-payload", 32),
			CreatedAt:      base.Add(time.Duration(i) * time.Second),
		}).Error)
	}

	router := gin.New()
	router.GET("/ai-waiter/:businessId/messages", GetAiWaiterMessages)

	recorder.statements = nil
	w := performAIWaiterRequest(t, router, http.MethodGet, fmt.Sprintf("/ai-waiter/%d/messages?session_token=session-history-bound&mode=ordering&table_code=A1", business.ID), nil)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var messages []struct {
		ID        uint   `json:"id"`
		Role      string `json:"role"`
		Content   string `json:"content"`
		CreatedAt int64  `json:"created_at"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &messages))
	require.Len(t, messages, 100)
	require.Equal(t, "Message 050", messages[0].Content)
	require.Equal(t, "Message 149", messages[len(messages)-1].Content)
	require.Equal(t, 1, recorder.selectCount("businesses"), "public history should not re-read the business after resolving the route identifier")
	require.Zero(t, recorder.selectStarCount("ai_waiter_messages"), "public history should project response fields only")
	require.False(t, recordedSelectMentionsColumn(recorder, "ai_waiter_messages", "tool_calls"), "public history should not hydrate tool call payloads")
}

func BenchmarkGetAiWaiterMessagesSQLite(b *testing.B) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(b)

	business := createAIWaiterBusiness(b, "messages-bench", true)
	createAIWaiterTable(b, business.ID, "A1")
	conv := createAIWaiterConversation(b, business.ID, "session-messages-bench")
	base := time.Now().Add(-24 * time.Hour)
	messages := make([]database.AiWaiterMessage, 0, 1000)
	largeToolPayload := strings.Repeat("tool-payload", 128)
	for i := 0; i < 1000; i++ {
		messages = append(messages, database.AiWaiterMessage{
			ConversationID: conv.ID,
			Role:           "assistant",
			Content:        fmt.Sprintf("Message %04d", i),
			ToolCalls:      largeToolPayload,
			CreatedAt:      base.Add(time.Duration(i) * time.Second),
		})
	}
	require.NoError(b, db.Create(&messages).Error)

	router := gin.New()
	router.GET("/ai-waiter/:businessId/messages", GetAiWaiterMessages)
	path := fmt.Sprintf("/ai-waiter/%d/messages?session_token=session-messages-bench&mode=ordering&table_code=A1", business.ID)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := performAIWaiterRequest(b, router, http.MethodGet, path, nil)
		if w.Code != http.StatusOK {
			b.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}
	}
}

func BenchmarkGetAiConversationMessagesSQLite(b *testing.B) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(b)

	business := createAIWaiterBusiness(b, "staff-messages-bench", true)
	manager := createAIWaiterStaff(b, business.ID, database.StaffRoleManager, "staff-messages-bench@example.com")
	conv := createAIWaiterConversation(b, business.ID, "session-staff-messages-bench")
	base := time.Now().Add(-24 * time.Hour)
	messages := make([]database.AiWaiterMessage, 0, 1000)
	largeToolPayload := strings.Repeat("tool-payload", 128)
	for i := 0; i < 1000; i++ {
		messages = append(messages, database.AiWaiterMessage{
			ConversationID: conv.ID,
			Role:           "assistant",
			Content:        fmt.Sprintf("Staff Message %04d", i),
			ToolCalls:      largeToolPayload,
			CreatedAt:      base.Add(time.Duration(i) * time.Second),
		})
	}
	require.NoError(b, db.Create(&messages).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleManager))
		c.Set("staff_id", manager.ID)
		c.Set("staff_business_id", business.ID)
		c.Next()
	})
	router.GET("/businesses/:id/ai/conversations/:convId/messages", RoleBasedAccessMiddleware("ai_waiter:read"), GetAiConversationMessages)
	path := fmt.Sprintf("/businesses/%d/ai/conversations/%d/messages", business.ID, conv.ID)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := performAIWaiterRequest(b, router, http.MethodGet, path, nil)
		if w.Code != http.StatusOK {
			b.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}
	}
}

func TestAIWaiterPublicEndpointsRejectClosedVenue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)

	business := createAIWaiterBusiness(t, "closed-venue", true)
	closedAt := time.Now().Add(-time.Hour)
	business.ClosedAt = &closedAt
	require.NoError(t, database.GetDB().Save(business).Error)

	router := gin.New()
	router.GET("/ai-waiter/:businessId/messages", GetAiWaiterMessages)
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)

	t.Run("messages endpoint rejects a closed venue", func(t *testing.T) {
		w := performAIWaiterRequest(t, router, http.MethodGet, fmt.Sprintf("/ai-waiter/%d/messages?session_token=session-closed", business.ID), nil)
		assert.Equal(t, http.StatusForbidden, w.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, services.OrderErrCodeBusinessUnavailable, resp["code"])
	})

	t.Run("chat endpoint rejects a closed venue", func(t *testing.T) {
		w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
			"session_token": "session-closed-post",
			"mode":          "concierge",
			"history": []map[string]any{
				{"role": "user", "content": "Are you open?"},
			},
		})
		assert.Equal(t, http.StatusForbidden, w.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, services.OrderErrCodeBusinessUnavailable, resp["code"])
	})

	var count int64
	require.NoError(t, database.GetDB().Model(&database.AiWaiterConversation{}).Where("business_id = ?", business.ID).Count(&count).Error)
	assert.Zero(t, count)
}

func TestGetAiWaiterMessages_RejectsInvalidOrderingScopeWithoutCreatingConversation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)

	business := createAIWaiterBusiness(t, "ordering-scope", true)
	otherBusiness := createAIWaiterBusiness(t, "ordering-scope-other", true)
	otherTable := createAIWaiterTable(t, otherBusiness.ID, "OTHER-A1")

	router := gin.New()
	router.GET("/ai-waiter/:businessId/messages", GetAiWaiterMessages)

	t.Run("ordering mode requires a table code", func(t *testing.T) {
		w := performAIWaiterRequest(t, router, http.MethodGet, fmt.Sprintf("/ai-waiter/%d/messages?session_token=session-no-table&mode=ordering", business.ID), nil)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "Table code is required")

		var count int64
		require.NoError(t, database.GetDB().Model(&database.AiWaiterConversation{}).Where("business_id = ?", business.ID).Count(&count).Error)
		assert.Zero(t, count)
	})

	t.Run("ordering mode rejects a table from another business", func(t *testing.T) {
		w := performAIWaiterRequest(t, router, http.MethodGet, fmt.Sprintf("/ai-waiter/%d/messages?session_token=session-wrong-table&mode=ordering&table_code=%s", business.ID, otherTable.TableCode), nil)
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "Table not found")

		var count int64
		require.NoError(t, database.GetDB().Model(&database.AiWaiterConversation{}).Where("business_id = ?", business.ID).Count(&count).Error)
		assert.Zero(t, count)
	})
}

func TestHandleAIWaiter_RejectsInvalidScopeBeforeAIProcessing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)

	business := createAIWaiterBusiness(t, "post-ordering-scope", true)
	otherBusiness := createAIWaiterBusiness(t, "post-ordering-scope-other", true)
	otherTable := createAIWaiterTable(t, otherBusiness.ID, "OTHER-B2")

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)

	baseBody := map[string]any{
		"session_token": "session-post-scope",
		"history": []map[string]any{
			{
				"role":    "user",
				"content": "Can I order a burger?",
			},
		},
	}

	t.Run("ordering mode requires a table code", func(t *testing.T) {
		w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
			"session_token": baseBody["session_token"],
			"history":       baseBody["history"],
			"mode":          "ordering",
		})
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "Table code is required")
	})

	t.Run("ordering mode rejects a table from another business", func(t *testing.T) {
		w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
			"session_token": "session-post-wrong-table",
			"history":       baseBody["history"],
			"mode":          "ordering",
			"table_code":    otherTable.TableCode,
		})
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "Table not found")
	})

	t.Run("invalid mode is rejected", func(t *testing.T) {
		w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
			"session_token": "session-post-invalid-mode",
			"history":       baseBody["history"],
			"mode":          "banquet",
		})
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "Invalid mode")
	})
}
