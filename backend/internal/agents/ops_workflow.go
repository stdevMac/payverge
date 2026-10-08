package agents

import (
	"strings"
)

// WorkflowState is the durable multi-step guidance projection (Wave 3).
type WorkflowState struct {
	ID        string
	StepIndex int
	StepTotal int
}

// continuation tokens advance a stored workflow without starting a new guide.
var workflowAdvanceTokens = map[string]struct{}{
	"next": {}, "continue": {}, "done": {}, "siguiente": {}, "continuar": {}, "listo": {},
	"ok": {}, "listo.": {}, "next step": {}, "siguiente paso": {},
}

// ResolveWorkflowContinuation advances or completes a stored workflow when the
// operator sends a short continuation token. Unrelated full questions return
// false so the normal guide/model path runs.
func ResolveWorkflowContinuation(message, locale string, previous *WorkflowState) (*WorkflowState, bool) {
	if previous == nil || previous.ID == "" || previous.StepTotal <= 0 {
		return nil, false
	}
	msg := strings.TrimSpace(strings.ToLower(message))
	if msg == "" {
		return nil, false
	}
	// Long questions are new intents, not continuations.
	if len(msg) > 40 || strings.ContainsAny(msg, "?¿") {
		return nil, false
	}
	if _, ok := workflowAdvanceTokens[msg]; !ok {
		// Allow "next please" style short phrases.
		if !strings.HasPrefix(msg, "next") && !strings.HasPrefix(msg, "continue") &&
			!strings.HasPrefix(msg, "siguiente") && !strings.HasPrefix(msg, "continuar") &&
			!strings.HasPrefix(msg, "listo") && msg != "done" {
			return nil, false
		}
	}
	next := *previous
	if next.StepIndex < next.StepTotal-1 {
		next.StepIndex++
	}
	// Cap at last step.
	if next.StepIndex >= next.StepTotal {
		next.StepIndex = next.StepTotal - 1
	}
	if next.StepIndex < 0 {
		next.StepIndex = 0
	}
	return &next, true
}

// WorkflowCompletionAnswer returns localized copy when the operator finishes
// the last step. Read-only — does not mutate restaurant state.
func WorkflowCompletionAnswer(locale string) string {
	if locale == "es" || locale == "es-AR" || locale == "es-ar" {
		return "Listo — completaste esta guía. Preguntame si necesitás el siguiente paso o otra área del panel."
	}
	return "Done — you finished this guide. Ask if you need the next step or another dashboard area."
}
