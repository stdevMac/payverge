package services

import (
	"context"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildWizardMessages_MapsRolesAndKeepsLegacyJSON(t *testing.T) {
	sysPrompt := GetWizardPrompt("en")
	msgs := []wizardTurn{
		{Role: "system", Content: sysPrompt},
		{Role: "user", Content: "I run a taco truck."},
		{Role: "assistant", Content: `{"message":"Great! How many tacos?","is_complete":false}`},
		{Role: "user", Content: "5 tacos, 2 drinks."},
	}

	system, out := buildWizardMessages(sysPrompt, msgs)

	require.Contains(t, system, "digital menu")
	require.Len(t, out, 3)

	assert.Equal(t, llm.RoleUser, out[0].Role)
	assert.Equal(t, "I run a taco truck.", out[0].Text)

	assert.Equal(t, llm.RoleAssistant, out[1].Role)
	assert.Equal(t, "Great! How many tacos?", out[1].Text)

	assert.Equal(t, llm.RoleUser, out[2].Role)
	assert.Equal(t, "5 tacos, 2 drinks.", out[2].Text)
}

func TestContinueConversation_SendsSystemRoleAndRoleMessages(t *testing.T) {
	cap := &capturingMenuProvider{resp: &llm.Response{Text: `{"message":"Hi!","is_complete":false}`}}
	svc := NewMenuAIService(cap, llm.ModelConfig{Menu: "test-menu", Image: "test-image"})

	system, msgs := buildWizardMessages(GetWizardPrompt("en"), []wizardTurn{
		{Role: "system", Content: GetWizardPrompt("en")},
		{Role: "user", Content: "hello"},
	})
	_, err := svc.provider.Generate(context.Background(), llm.GenerateRequest{
		Model:    svc.menuModel,
		System:   system,
		Messages: msgs,
	})
	require.NoError(t, err)

	assert.NotEmpty(t, cap.lastReq.System)
	require.Len(t, cap.lastReq.Messages, 1)
	assert.Equal(t, llm.RoleUser, cap.lastReq.Messages[0].Role)
	assert.NotContains(t, cap.lastReq.System, "[System Instructions]")
	assert.NotContains(t, cap.lastReq.Messages[0].Text, "[User]")
}

func TestWizardTurnInstruction_LocalizedPerFamily(t *testing.T) {
	en := wizardTurnInstruction("en")
	es := wizardTurnInstruction("es")
	esar := wizardTurnInstruction("es-AR")

	assert.Contains(t, en, "is_complete")
	assert.NotEqual(t, en, es, "es instruction must be localized, not English")
	assert.NotEqual(t, es, esar, "es-AR instruction must differ from es (voseo)")
	assert.Contains(t, esar, "Argentina")
}
