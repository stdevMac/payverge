package server

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/guardrails"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type denyClassifier struct{ category string }

func (d denyClassifier) Classify(_ context.Context, _ guardrails.ClassifyRequest) (guardrails.Verdict, error) {
	return guardrails.Verdict{Allowed: false, Category: d.category}, nil
}

type spyProvider struct{ called bool }

func (s *spyProvider) Generate(_ context.Context, _ llm.GenerateRequest) (*llm.Response, error) {
	s.called = true
	return &llm.Response{Text: "should not happen"}, nil
}

func TestHandleAIWaiter_OffTopicRedirectNoLLM(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "guard-off", true)
	table := createAIWaiterTable(t, business.ID, "GD-1")
	require.NoError(t, db.AutoMigrate(&database.Menu{}))
	require.NoError(t, db.Create(&database.Menu{BusinessID: business.ID, Categories: "[]", IsActive: true, Version: 1}).Error)
	conv := createAIWaiterConversation(t, business.ID, "guard-off-token")
	conv.TableCode = table.TableCode
	require.NoError(t, db.Save(conv).Error)

	sp := &spyProvider{}
	svc, _ := services.NewAIService(sp, llm.ModelConfig{Chat: "m", Image: "i", Director: "d", Menu: "mn"})
	SetAIService(svc)
	SetAIWaiterClassifier(denyClassifier{category: "off_topic"})
	t.Cleanup(func() { SetAIService(nil); SetAIWaiterClassifier(nil) })

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": "guard-off-token", "mode": "ordering", "table_code": table.TableCode, "language": "en",
		"history": []map[string]any{{"role": "user", "content": "who won the 1998 world cup?"}},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.False(t, sp.called, "off_topic must not call the LLM")
	assert.Contains(t, w.Body.String(), "menu")
}

func TestHandleAIWaiter_RedactsPIIAtIngest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "guard-pii", true)
	table := createAIWaiterTable(t, business.ID, "GD-2")
	require.NoError(t, db.AutoMigrate(&database.Menu{}))
	require.NoError(t, db.Create(&database.Menu{BusinessID: business.ID, Categories: "[]", IsActive: true, Version: 1}).Error)
	conv := createAIWaiterConversation(t, business.ID, "guard-pii-token")
	conv.TableCode = table.TableCode
	require.NoError(t, db.Save(conv).Error)

	svc, _ := services.NewAIService(okProvider{}, llm.ModelConfig{Chat: "m", Image: "i", Director: "d", Menu: "mn"})
	SetAIService(svc)
	t.Cleanup(func() { SetAIService(nil) })

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	_ = performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": "guard-pii-token", "mode": "ordering", "table_code": table.TableCode, "language": "en",
		"history": []map[string]any{{"role": "user", "content": "my email is bob@example.com call me at +14155550123"}},
	})
	var stored database.AiWaiterMessage
	require.NoError(t, db.Where("conversation_id = ? AND role = ?", conv.ID, "user").Order("id desc").First(&stored).Error)
	assert.NotContains(t, stored.Content, "bob@example.com")
	assert.NotContains(t, stored.Content, "4155550123")
}
