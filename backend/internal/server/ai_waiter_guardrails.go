package server

import "github.com/stdevmac/payverge/backend/internal/guardrails"

var aiWaiterClassifier guardrails.InputClassifier = guardrails.AllowAll{}

// SetAIWaiterClassifier overrides the guest-message classifier (wired in main.go
// to the Gemini classifier; AllowAll by default and in tests).
func SetAIWaiterClassifier(c guardrails.InputClassifier) {
	if c == nil {
		aiWaiterClassifier = guardrails.AllowAll{}
		return
	}
	aiWaiterClassifier = c
}
