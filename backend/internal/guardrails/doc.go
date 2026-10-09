// Package guardrails screens inbound user/guest text before it reaches an
// answering LLM, implementing contract C2 of the AI Excellence Campaign.
//
// # Surfaces
//
// The scoped surfaces today (see prompts/classifier.md):
//   - "ai_waiter": guest chat — only this restaurant's menu/food/ordering/
//     dining topics are in scope; everything else is off_topic.
//   - "director":  owner copilot — only the owner's own business operations
//     and growth are in scope. This replaces the keyword-blocklist scope gate
//     (isBusinessQuestion) that Lane C removes from director_console_service.go.
//   - "ops_assistant": operator dashboard help.
//   - "image_prompt": operator-supplied image-style descriptions (dish,
//     plating, lighting, background, camera/composition) sent to the image
//     generator. Terse style fragments are in scope; unrelated non-food
//     imagery, real people, and third-party brands are off_topic.
//
// # Fail-open contract
//
// Classify never surfaces an error. Provider failures, the internal 2s
// timeout, empty/malformed JSON, an unknown category, or a nil provider all
// return Allowed=true and are logged via slog with feature="guardrail" so the
// Lane A observer pipeline can account for them. Callers therefore treat a
// configured classifier as best-effort: when in doubt, the guest/owner is
// served. AllowAll is the explicit fail-open default used when no provider is
// wired (e.g. local dev without OPENROUTER_API_KEY).
//
// # Model
//
// GeminiClassifier uses the cheap guardrail model id (contract C1:
// OPENROUTER_MODEL_GUARDRAIL, ModelConfig.Guardrail), injected by
// cmd/app/main.go. Every call sets GenerateRequest.Feature="guardrail" and a
// strict ResponseSchema forcing {allowed,category,reason}.
//
// # Swap path to an external moderation API
//
// The seam is the InputClassifier interface, not the concrete type. To move to
// an external moderation vendor (e.g. OpenAI omni-moderation, AWS, or a
// self-hosted classifier) later:
//
//  1. Add a new file (e.g. moderation_api.go) with a type implementing
//     InputClassifier.Classify — map the vendor's category labels onto the
//     Verdict.Category constants (ok/off_topic/abuse/injection) and preserve
//     the fail-open rule (return Allowed=true on any vendor error/timeout).
//  2. Change the single construction site in cmd/app/main.go from
//     guardrails.NewGeminiClassifier(...) to the new constructor.
//
// No caller in services/ or server/ changes, because they depend only on the
// InputClassifier interface and the C2 Verdict shape. TestInterfaceSwapPath
// guards that any such implementation drops into the same wiring.
package guardrails
