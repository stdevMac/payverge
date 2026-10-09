package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/agents"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
)

type stubOps struct{}

func (stubOps) Ask(context.Context, agents.OpsAskRequest) (*agents.OpsAskResult, error) {
	return &agents.OpsAskResult{}, nil
}
func (stubOps) ListMessages(uint, uint) ([]database.OpsAssistantMessage, error) { return nil, nil }

func setupOpsStubDB(t *testing.T) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: glogger.Default.LogMode(glogger.Silent)})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{}, &database.Staff{},
		&database.OpsAssistantThread{}, &database.OpsAssistantMessage{}, &database.Escalation{},
	))
	InitializeRBAC(database.GetDBWrapper())
}

func TestSubmitOpsAssistantFeedbackPersists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupOpsStubDB(t)
	SetOpsAssistantService(stubOps{})

	business := createOwnedBusiness(t, "0xFbOwner", "Fb Biz")
	thread, err := database.CreateOpsAssistantThread(business.ID, "t", "en")
	require.NoError(t, err)
	msg := &database.OpsAssistantMessage{
		ThreadID: thread.ID, BusinessID: business.ID,
		Role: database.OpsAssistantRoleAssistant, Content: "answer",
	}
	require.NoError(t, database.SaveOpsAssistantMessage(msg))

	body, _ := json.Marshal(map[string]any{"feedback": "up"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("token_type", "web3")
	c.Set("address", "0xFbOwner")
	c.Set("business_owner_address", "0xFbOwner")
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "messageId", Value: fmt.Sprintf("%d", msg.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	SubmitOpsAssistantFeedback(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var reloaded database.OpsAssistantMessage
	require.NoError(t, database.GetDB().First(&reloaded, msg.ID).Error)
	require.Equal(t, "up", reloaded.Feedback)
}
