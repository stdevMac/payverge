// Package guardrails screens inbound user/guest text before it reaches an
// answering LLM. It implements contract C2 of the AI Excellence Campaign.
package guardrails

import "context"

// Verdict is the classifier's decision about a single inbound message.
type Verdict struct {
	Allowed  bool   // false => block/redirect
	Category string // "off_topic" | "abuse" | "injection" | "ok"
	Reason   string // short, log-safe, never shown raw to guests
}

// ClassifyRequest carries the single inbound message to screen plus the
// surface and locale that scope the decision. Text is the inbound message
// ONLY — never the conversation history.
type ClassifyRequest struct {
	Surface    string // "ai_waiter" | "director" | "ops_assistant" | "image_prompt"
	BusinessID uint
	Locale     string
	Text       string
}

// Surface constants used by callers in Phase 1.
const (
	SurfaceAIWaiter     = "ai_waiter"
	SurfaceDirector     = "director"
	SurfaceOpsAssistant = "ops_assistant"
	// SurfaceImagePrompt scopes operator-supplied image-style descriptions
	// (dish/plating/lighting/composition text) sent to the image generator.
	// It is NOT a dining chat rubric — image style fragments like "rustic
	// wooden table, warm light" are in scope, not off_topic.
	SurfaceImagePrompt = "image_prompt"
)

// Category constants for Verdict.Category.
const (
	CategoryOK        = "ok"
	CategoryOffTopic  = "off_topic"
	CategoryAbuse     = "abuse"
	CategoryInjection = "injection"
)

// InputClassifier screens an inbound message. SEMANTICS (C2): callers treat
// (err != nil) as fail-open Allowed=true and log it. Implementations that
// call an LLM add a hard 2s ctx timeout internally.
type InputClassifier interface {
	Classify(ctx context.Context, req ClassifyRequest) (Verdict, error)
}

// AllowAll is the fail-open default classifier used when no provider is
// configured. It allows everything and never errors.
type AllowAll struct{}

// Classify always allows.
func (AllowAll) Classify(_ context.Context, _ ClassifyRequest) (Verdict, error) {
	return Verdict{Allowed: true, Category: CategoryOK}, nil
}
