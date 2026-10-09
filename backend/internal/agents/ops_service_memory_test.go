package agents

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"

	"github.com/stretchr/testify/require"
)

func TestPriorTurnsLLMExcludesCurrentUserMessage(t *testing.T) {
	messages := []database.OpsAssistantMessage{
		{ID: 10, Role: database.OpsAssistantRoleUser, Content: "How do I add an item?"},
		{ID: 11, Role: database.OpsAssistantRoleAssistant, Content: "Open the menu builder."},
		{ID: 12, Role: database.OpsAssistantRoleUser, Content: "how"},
	}

	got := priorTurnsLLM(messages, 12)
	require.Equal(t, []llm.Message{
		{Role: llm.RoleUser, Text: "How do I add an item?"},
		{Role: llm.RoleAssistant, Text: "Open the menu builder."},
	}, got)
}
