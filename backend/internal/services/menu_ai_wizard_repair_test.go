package services

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type scriptedWizardProvider struct {
	responses []*llm.Response
	requests  []llm.GenerateRequest
}

func (p *scriptedWizardProvider) Generate(_ context.Context, req llm.GenerateRequest) (*llm.Response, error) {
	p.requests = append(p.requests, req)
	if len(p.responses) == 0 {
		return nil, fmt.Errorf("script exhausted")
	}
	resp := p.responses[0]
	p.responses = p.responses[1:]
	return resp, nil
}

func seedWizardRepairSession(t *testing.T, config string, turns []wizardTurn) uint {
	t.Helper()
	dsn := fmt.Sprintf("file:wizard-repair-%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&database.Business{}, &database.MenuWizardSession{}, &database.MenuWizardMessage{}))
	database.SetTestDB(db)
	session := database.MenuWizardSession{BusinessID: 1, Status: database.WizardStatusInProgress, Config: config, Language: "en"}
	require.NoError(t, db.Create(&session).Error)
	require.NoError(t, db.Create(&database.MenuWizardMessage{SessionID: session.ID, Role: "system", Content: GetWizardPrompt("en")}).Error)
	for _, turn := range turns {
		require.NoError(t, db.Create(&database.MenuWizardMessage{SessionID: session.ID, Role: turn.Role, Content: turn.Content}).Error)
	}
	return session.ID
}

func wizardRepairConfig(t *testing.T, sessionID uint) map[string]string {
	t.Helper()
	var session database.MenuWizardSession
	require.NoError(t, database.GetDB().First(&session, sessionID).Error)
	config := map[string]string{}
	require.NoError(t, json.Unmarshal([]byte(session.Config), &config))
	return config
}

func wizardRepairMessages(t *testing.T, sessionID uint, role string) []database.MenuWizardMessage {
	t.Helper()
	var rows []database.MenuWizardMessage
	require.NoError(t, database.GetDB().Where("session_id = ? AND role = ?", sessionID, role).Order("id ASC").Find(&rows).Error)
	return rows
}

func TestContinueConversationRepairsTruncatedOutputOnce(t *testing.T) {
	sessionID := seedWizardRepairSession(t, `{"cuisine":"Argentine"}`, []wizardTurn{{Role: "user", Content: "Six dishes"}})
	provider := &scriptedWizardProvider{responses: []*llm.Response{
		{Text: `{"message":"Almost`, FinishReason: "length", ProviderRequestID: "gen-1"},
		{Text: `{"message":"Which price range?","is_complete":false,"extracted_config":{"items_per_category":"6"}}`, FinishReason: "stop", ProviderRequestID: "gen-2"},
	}}
	svc := NewMenuAIService(provider, llm.ModelConfig{Menu: "test-menu"})
	got, err := svc.RetryWizardConversation(context.Background(), sessionID)
	require.NoError(t, err)
	require.Equal(t, "Which price range?", got.Message)
	require.Len(t, provider.requests, 2)
	require.Equal(t, 1200, provider.requests[1].MaxTokens)
	require.Equal(t, "Argentine", wizardRepairConfig(t, sessionID)["cuisine"])
	require.Equal(t, "6", wizardRepairConfig(t, sessionID)["items_per_category"])
	require.Len(t, wizardRepairMessages(t, sessionID, "assistant"), 1)
}

func TestContinueConversationSecondDecodeFailurePreservesState(t *testing.T) {
	sessionID := seedWizardRepairSession(t, `{"cuisine":"Argentine"}`, []wizardTurn{{Role: "user", Content: "Six dishes"}})
	provider := &scriptedWizardProvider{responses: []*llm.Response{
		{Text: `{"message":"Almost`, FinishReason: "length"},
		{Text: `not-json`, FinishReason: "stop"},
	}}
	svc := NewMenuAIService(provider, llm.ModelConfig{Menu: "test-menu"})
	_, err := svc.RetryWizardConversation(context.Background(), sessionID)
	require.ErrorIs(t, err, llm.ErrStructuredOutput)
	require.Len(t, provider.requests, 2)
	require.Equal(t, map[string]string{"cuisine": "Argentine"}, wizardRepairConfig(t, sessionID))
	require.Empty(t, wizardRepairMessages(t, sessionID, "assistant"))
	var session database.MenuWizardSession
	require.NoError(t, database.GetDB().First(&session, sessionID).Error)
	require.Equal(t, database.WizardStatusInProgress, session.Status)
}

func TestRetryWizardConversationDoesNotDuplicateUserTurn(t *testing.T) {
	sessionID := seedWizardRepairSession(t, `{}`, nil)
	provider := &scriptedWizardProvider{responses: []*llm.Response{
		{Text: `bad`, FinishReason: "length"}, {Text: `still bad`, FinishReason: "stop"},
		{Text: `{"message":"Recovered","is_complete":false}`, FinishReason: "stop"},
	}}
	svc := NewMenuAIService(provider, llm.ModelConfig{Menu: "test-menu"})
	_, err := svc.ContinueWizardConversation(context.Background(), sessionID, "Six dishes")
	require.ErrorIs(t, err, llm.ErrStructuredOutput)
	_, err = svc.RetryWizardConversation(context.Background(), sessionID)
	require.NoError(t, err)
	require.Len(t, wizardRepairMessages(t, sessionID, "user"), 1)
	require.Len(t, wizardRepairMessages(t, sessionID, "assistant"), 1)
}
