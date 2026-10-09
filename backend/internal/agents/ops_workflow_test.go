package agents

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveWorkflowContinuation_AdvancesAndCaps(t *testing.T) {
	for _, tok := range []string{"next", "continue", "done", "siguiente", "continuar", "listo"} {
		prev := &WorkflowState{ID: "first_menu_item", StepIndex: 0, StepTotal: 3}
		next, ok := ResolveWorkflowContinuation(tok, "en", prev)
		require.True(t, ok, tok)
		require.Equal(t, 1, next.StepIndex, tok)
	}
	// Cap at last step.
	last, ok := ResolveWorkflowContinuation("next", "en", &WorkflowState{ID: "x", StepIndex: 2, StepTotal: 3})
	require.True(t, ok)
	require.Equal(t, 2, last.StepIndex)
}

func TestResolveWorkflowContinuation_UnrelatedQuestionExits(t *testing.T) {
	prev := &WorkflowState{ID: "first_menu_item", StepIndex: 0, StepTotal: 3}
	_, ok := ResolveWorkflowContinuation("How do I configure AI Waiter from scratch?", "en", prev)
	require.False(t, ok)
	_, ok = ResolveWorkflowContinuation("¿cómo agrego un item?", "es", prev)
	require.False(t, ok)
}
