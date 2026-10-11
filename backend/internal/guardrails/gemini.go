package guardrails

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/locales"
)

//go:embed prompts/classifier.md
var classifierPromptMD string

// classifierSystemPrompt returns the embedded classifier system prompt with
// its trailing whitespace trimmed. The {{SURFACE}}/{{MESSAGE}} placeholders
// are filled per request in buildClassifierMessages.
func classifierSystemPrompt() string {
	return strings.TrimSpace(classifierPromptMD)
}

// GeminiClassifier implements InputClassifier by issuing one strict-JSON call
// to the configured guardrail model via llm.Provider. It is the production
// classifier; AllowAll is the fail-open fallback when no provider is wired.
type GeminiClassifier struct {
	provider  llm.Provider
	model     string
	fallbacks []string
	// strict makes the classifier fail CLOSED: when it cannot reach a verdict
	// (nil provider / empty model / provider error / timeout / bad JSON), it
	// blocks (Allowed=false, Category=off_topic) instead of allowing. Default
	// (false) preserves the historical fail-open contract C2.
	strict bool
}

// NewGeminiClassifier builds a fail-open classifier over the given provider and
// model. It is the non-strict wrapper around NewGeminiClassifierStrict.
//
// model is the guardrail model id (contract C1 OPENROUTER_MODEL_GUARDRAIL,
// injected by cmd/app/main.go in Phase 1). fallbacks is the ordered list of
// OpenRouter fallback model ids (OPENROUTER_GUARDRAIL_FALLBACKS) forwarded on
// every classifier call so the guardrail survives a primary-model outage. A
// nil provider or empty model is tolerated: Classify then fails open (see
// Task 4).
func NewGeminiClassifier(provider llm.Provider, model string, fallbacks []string) *GeminiClassifier {
	return NewGeminiClassifierStrict(provider, model, fallbacks, false)
}

// NewGeminiClassifierStrict builds a classifier whose failure mode is chosen by
// the strict flag. strict=false keeps the fail-open contract C2 (errors allow);
// strict=true fails CLOSED (errors block as off_topic) for deployments that
// would rather drop a borderline message than let an unscreened one through.
func NewGeminiClassifierStrict(provider llm.Provider, model string, fallbacks []string, strict bool) *GeminiClassifier {
	return &GeminiClassifier{provider: provider, model: strings.TrimSpace(model), fallbacks: fallbacks, strict: strict}
}

// strictResult is the log label describing how a classifier failure was
// resolved given the strict flag.
func strictResult(strict bool) string {
	if strict {
		return "failed_closed"
	}
	return "failed_open"
}

// classifierVerdictSchema is the strict JSON schema the model must satisfy.
// allowed+category+reason are all required so a partial object is a parse
// error (which fails open at the caller).
var classifierVerdictSchema = &llm.JSONSchema{
	Type: llm.TypeObject,
	Properties: map[string]*llm.JSONSchema{
		"allowed": {Type: llm.TypeBoolean, Description: "true only when category is ok"},
		"category": {
			Type:        llm.TypeString,
			Description: "one of ok|off_topic|abuse|injection",
			Enum:        []string{CategoryOK, CategoryOffTopic, CategoryAbuse, CategoryInjection},
		},
		"reason": {Type: llm.TypeString, Description: "short log-safe English label"},
	},
	Required: []string{"allowed", "category", "reason"},
}

// modelVerdict mirrors the strict schema for unmarshalling.
type modelVerdict struct {
	Allowed  bool   `json:"allowed"`
	Category string `json:"category"`
	Reason   string `json:"reason"`
}

// buildClassifierMessages renders the single user turn carrying the surface
// and the inbound text. The NativeName of the locale is included only as a
// hint label; the prompt itself is language-agnostic.
func buildClassifierMessages(req ClassifyRequest) []llm.Message {
	nativeName := req.Locale
	if lc, ok := locales.Lookup(req.Locale); ok && lc.NativeName != "" {
		nativeName = lc.NativeName
	}
	body := fmt.Sprintf("surface: %s\nlanguage_hint: %s\nmessage:\n%s",
		req.Surface, nativeName, req.Text)
	return []llm.Message{{Role: llm.RoleUser, Text: body}}
}

// classifierTimeout is the hard internal deadline contract C2 mandates. If
// the caller's context is already tighter, the tighter one wins.
const classifierTimeout = 2 * time.Second

// Classify implements InputClassifier. It NEVER surfaces an error: provider
// failures, the 2s timeout, empty/malformed JSON, an unknown category, or a nil
// provider are all resolved by the classifier's failure mode and logged with
// the guardrails feature tag (result=failed_open|failed_closed) for the Lane A
// observer pipeline. In the default (non-strict) mode these conditions fail
// OPEN (Allowed=true), per contract C2; in strict mode (GUARDRAIL_STRICT) they
// fail CLOSED (Allowed=false, Category=off_topic). The returned error is always
// nil — kept in the signature to satisfy the interface.
func (c *GeminiClassifier) Classify(ctx context.Context, req ClassifyRequest) (Verdict, error) {
	if c.provider == nil || c.model == "" {
		slog.Warn("guardrails classifier not configured",
			"feature", "guardrail", "surface", req.Surface, "business_id", req.BusinessID,
			"result", strictResult(c.strict))
		if c.strict {
			return Verdict{Allowed: false, Category: CategoryOffTopic, Reason: "classifier_unconfigured_strict"}, nil
		}
		return Verdict{Allowed: true, Category: CategoryOK, Reason: "classifier_unconfigured"}, nil
	}

	cctx, cancel := context.WithTimeout(ctx, classifierTimeout)
	defer cancel()

	verdict, err := c.classifyRaw(cctx, req)
	if err != nil {
		slog.Warn("guardrails classifier failed",
			"feature", "guardrail",
			"surface", req.Surface,
			"business_id", req.BusinessID,
			"err", err,
			"result", strictResult(c.strict))
		if c.strict {
			return Verdict{Allowed: false, Category: CategoryOffTopic, Reason: "classifier_error_strict"}, nil
		}
		return Verdict{Allowed: true, Category: CategoryOK, Reason: "classifier_error"}, nil
	}
	return verdict, nil
}

// classifyRaw performs the provider call and parses the verdict. Timeout and
// fail-open handling wrap this in Classify (Task 4).
func (c *GeminiClassifier) classifyRaw(ctx context.Context, req ClassifyRequest) (Verdict, error) {
	temp := float32(0)
	feature := "guardrail"
	// Screening a guest's message spends the guest scope, not the owner's.
	audience := llm.BudgetAudienceOwner
	if req.Surface == SurfaceAIWaiter {
		audience = llm.BudgetAudienceGuest
	}
	resp, err := c.provider.Generate(ctx, llm.GenerateRequest{
		Model:          c.model,
		Fallbacks:      c.fallbacks,
		Feature:        feature,
		System:         classifierSystemPrompt(),
		Messages:       buildClassifierMessages(req),
		ResponseSchema: classifierVerdictSchema,
		Temperature:    &temp,
		MaxTokens:      80,
		BusinessID:     req.BusinessID,
		BudgetAudience: audience,
	})
	if err != nil {
		return Verdict{}, err
	}
	if resp == nil || strings.TrimSpace(resp.Text) == "" {
		return Verdict{}, fmt.Errorf("guardrails: empty classifier response")
	}
	return parseVerdict(resp.Text)
}

// parseVerdict extracts a Verdict from the model's JSON text. It tolerates
// a fenced/padded object by slicing the first { to the last } if a direct
// unmarshal fails. An unknown category is treated as a parse error so the
// caller fails open rather than acting on a hallucinated label.
func parseVerdict(raw string) (Verdict, error) {
	text := strings.TrimSpace(raw)
	var mv modelVerdict
	if err := json.Unmarshal([]byte(text), &mv); err != nil {
		start := strings.Index(text, "{")
		end := strings.LastIndex(text, "}")
		if start == -1 || end <= start {
			return Verdict{}, fmt.Errorf("guardrails: unparseable verdict: %w", err)
		}
		if err2 := json.Unmarshal([]byte(text[start:end+1]), &mv); err2 != nil {
			return Verdict{}, fmt.Errorf("guardrails: unparseable verdict: %w", err2)
		}
	}
	switch mv.Category {
	case CategoryOK, CategoryOffTopic, CategoryAbuse, CategoryInjection:
	default:
		return Verdict{}, fmt.Errorf("guardrails: unknown category %q", mv.Category)
	}
	allowed := mv.Allowed && mv.Category == CategoryOK
	return Verdict{Allowed: allowed, Category: mv.Category, Reason: mv.Reason}, nil
}
