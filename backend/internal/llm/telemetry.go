package llm

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"github.com/stdevmac/payverge/backend/internal/llmeval/langid"
	"github.com/stdevmac/payverge/backend/internal/locales"
)

// AITelemetryEvent is the privacy-safe AI event schema (Wave 6).
// Labels must be bounded enums — never free-text user content.
type AITelemetryEvent struct {
	Feature     string `json:"feature"`
	Surface     string `json:"surface"`
	Code        string `json:"code,omitempty"`
	ErrorClass  string `json:"error_class,omitempty"`
	ServedModel string `json:"served_model,omitempty"`
	LatencyMs   int64  `json:"latency_ms,omitempty"`
	TokensIn    int    `json:"tokens_in,omitempty"`
	TokensOut   int    `json:"tokens_out,omitempty"`
	// BusinessID is allowed as a low-cardinality tenant key when present.
	BusinessID uint `json:"business_id,omitempty"`
	// GuideID is a catalog enum, never free text.
	GuideID string `json:"guide_id,omitempty"`
	// Verdict is guardrail category enum: ok|abuse|injection|off_topic.
	Verdict string `json:"verdict,omitempty"`

	ContractVersion string `json:"contract_version,omitempty"`
	Outcome         string `json:"outcome,omitempty"`
	ActionOutcome   string `json:"action_outcome,omitempty"`
	SourceOutcome   string `json:"source_outcome,omitempty"`
	EntityOutcome   string `json:"entity_outcome,omitempty"`
	SchemaOutcome   string `json:"schema_outcome,omitempty"`
	LanguageOutcome string `json:"language_outcome,omitempty"`
	Language        string `json:"language,omitempty"`
	RepairCount     int    `json:"repair_count,omitempty"`
	ToolCalls       int    `json:"tool_calls,omitempty"`
	TimeToFirstMs   int64  `json:"time_to_first_ms,omitempty"`
	V2ShadowValid   bool   `json:"v2_shadow_valid"`
}

const assistantTelemetryLanguageConfidence = 0.34

var assistantTelemetryDetectorLanguages = map[string]bool{
	"en": true, "es": true, "fr": true, "de": true, "it": true, "pt": true, "nl": true, "tr": true,
	"ar": true, "ru": true, "ja": true, "ko": true, "zh": true, "hi": true, "th": true,
}

var assistantTelemetryScriptLanguages = map[string]bool{
	"ar": true, "ru": true, "ja": true, "ko": true, "zh": true, "hi": true, "th": true,
}

var assistantTelemetryLatinSignals = map[string][]string{
	"en": {"the", "and", "you", "with", "your", "for", "how", "can", "today", "our", "help"},
	"es": {"el", "la", "los", "las", "con", "para", "qué", "puedo", "tu", "nuestro", "ayudarte", "pedido"},
	"fr": {"le", "la", "les", "vous", "avec", "votre", "comment", "puis", "dans", "notre", "aider", "commande"},
	"de": {"der", "die", "das", "und", "mit", "ihrer", "wie", "kann", "ich", "unserem", "ihnen", "bestellung"},
	"it": {"il", "la", "con", "come", "posso", "tuo", "nostro", "oggi", "aiutarti", "ordine", "nel", "del"},
	"pt": {"o", "a", "com", "como", "posso", "você", "seu", "hoje", "nosso", "pedido", "ajudar", "ao"},
	"nl": {"de", "het", "een", "met", "uw", "hoe", "kan", "ik", "ons", "helpen", "bestelling", "vandaag"},
	"tr": {"ve", "ile", "nasıl", "size", "bir", "için", "bugün", "geldiniz", "yardımcı", "siparişinizle", "olabilirim"},
}

// ValidatedResponseLanguageOutcome classifies only finalizer-validated,
// renderable response text. The caller-provided fallback is "dropped" when an
// unsupported request locale was replaced before finalization; that decision
// remains authoritative even if the fallback content matches its new locale.
func ValidatedResponseLanguageOutcome(canonicalLocale, renderableText, fallbackOutcome string) string {
	if fallbackOutcome == "dropped" {
		return "dropped"
	}
	locale, ok := locales.Lookup(canonicalLocale)
	if !ok || locale.Canonical != canonicalLocale {
		return "none"
	}
	wanted := strings.SplitN(canonicalLocale, "-", 2)[0]
	if !assistantTelemetryDetectorLanguages[wanted] {
		return "none"
	}
	detected, confidence := langid.Detect(renderableText)
	if !assistantTelemetryDetectorLanguages[detected] ||
		!assistantTelemetryHasLanguageEvidence(detected, renderableText) ||
		confidence < assistantTelemetryLanguageConfidence {
		return "none"
	}
	if detected == wanted {
		return "verified"
	}
	return "dropped"
}

func assistantTelemetryHasLanguageEvidence(language, text string) bool {
	letterCount := 0
	for _, r := range text {
		if unicode.IsLetter(r) {
			letterCount++
		}
	}
	if assistantTelemetryScriptLanguages[language] {
		return letterCount >= 8
	}
	tokens := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) })
	if letterCount < 24 || len(tokens) < 8 {
		return false
	}
	tokenSet := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		tokenSet[token] = struct{}{}
	}
	signalCount := 0
	for _, signal := range assistantTelemetryLatinSignals[language] {
		if _, ok := tokenSet[signal]; ok {
			signalCount++
		}
	}
	return signalCount >= 2
}

// TelemetrySink receives validated AI telemetry events. Production may wire
// Prometheus/PostHog; tests install a capturing sink. Default is a no-op.
type TelemetrySink func(AITelemetryEvent)

var (
	telemetryMu   sync.RWMutex
	telemetrySink TelemetrySink
)

// SetTelemetrySink installs the global sink. Pass nil to disable emission.
func SetTelemetrySink(s TelemetrySink) {
	telemetryMu.Lock()
	defer telemetryMu.Unlock()
	telemetrySink = s
}

// EmitTelemetry validates then forwards a privacy-safe AI event. Invalid
// events are dropped (never emit content-bearing labels).
func EmitTelemetry(ev AITelemetryEvent) {
	if err := ValidateTelemetryEvent(ev); err != nil {
		log.Printf("ai_telemetry_drop feature=%s err=%v", ev.Feature, err)
		return
	}
	telemetryMu.RLock()
	sink := telemetrySink
	telemetryMu.RUnlock()
	if sink != nil {
		sink(ev)
	}
}

var (
	// Labels that must never appear as free-form values on the wire.
	forbiddenLabelContent = regexp.MustCompile(`(?i)(@|select |password|bearer |http://|https://)`)
)

// ValidateTelemetryEvent rejects high-cardinality or content-bearing labels.
func ValidateTelemetryEvent(ev AITelemetryEvent) error {
	if strings.TrimSpace(ev.Feature) == "" {
		return fmt.Errorf("feature required")
	}
	check := func(name, v string) error {
		if v == "" {
			return nil
		}
		if len(v) > 64 {
			return fmt.Errorf("%s too long", name)
		}
		if forbiddenLabelContent.MatchString(v) {
			return fmt.Errorf("%s contains forbidden content", name)
		}
		if strings.ContainsAny(v, " \t\n\r") {
			return fmt.Errorf("%s must be a single token", name)
		}
		// served_model may include provider/model path with slashes only —
		// spaces already rejected above; no extra rule needed.
		return nil
	}
	for _, pair := range [][2]string{
		{"feature", ev.Feature},
		{"surface", ev.Surface},
		{"code", ev.Code},
		{"error_class", ev.ErrorClass},
		{"served_model", ev.ServedModel},
		{"guide_id", ev.GuideID},
		{"verdict", ev.Verdict},
		{"contract_version", ev.ContractVersion},
		{"outcome", ev.Outcome},
		{"action_outcome", ev.ActionOutcome},
		{"source_outcome", ev.SourceOutcome},
		{"entity_outcome", ev.EntityOutcome},
		{"schema_outcome", ev.SchemaOutcome},
		{"language_outcome", ev.LanguageOutcome},
		{"language", ev.Language},
	} {
		if err := check(pair[0], pair[1]); err != nil {
			return err
		}
	}
	if err := boundedEnum("contract_version", ev.ContractVersion, "v1", "v2"); err != nil {
		return err
	}
	if err := boundedEnum("outcome", ev.Outcome, "ok", "fallback", "invalid", "blocked", "timeout", "error"); err != nil {
		return err
	}
	if err := boundedEnum("action_outcome", ev.ActionOutcome, "none", "offered", "accepted", "rejected", "failed"); err != nil {
		return err
	}
	if err := boundedEnum("source_outcome", ev.SourceOutcome, "none", "verified", "dropped"); err != nil {
		return err
	}
	if err := boundedEnum("entity_outcome", ev.EntityOutcome, "none", "verified", "dropped"); err != nil {
		return err
	}
	if err := boundedEnum("schema_outcome", ev.SchemaOutcome, "none", "verified", "dropped"); err != nil {
		return err
	}
	if err := boundedEnum("language_outcome", ev.LanguageOutcome, "none", "verified", "dropped"); err != nil {
		return err
	}
	if ev.Language != "" {
		locale, ok := locales.Lookup(ev.Language)
		if !ok || locale.Canonical != ev.Language {
			return fmt.Errorf("language must be an exact canonical locale")
		}
	}
	for _, count := range []struct {
		name  string
		value int64
		max   int64
	}{
		{name: "latency_ms", value: ev.LatencyMs, max: 30 * 60 * 1000},
		{name: "tokens_in", value: int64(ev.TokensIn), max: 1_000_000},
		{name: "tokens_out", value: int64(ev.TokensOut), max: 1_000_000},
		{name: "repair_count", value: int64(ev.RepairCount), max: 16},
		{name: "tool_calls", value: int64(ev.ToolCalls), max: 64},
		{name: "time_to_first_ms", value: ev.TimeToFirstMs, max: 30 * 60 * 1000},
	} {
		if count.value < 0 || count.value > count.max {
			return fmt.Errorf("%s outside bounded range", count.name)
		}
	}
	return nil
}

func boundedEnum(name, value string, allowed ...string) error {
	if value == "" {
		return nil
	}
	for _, candidate := range allowed {
		if value == candidate {
			return nil
		}
	}
	return fmt.Errorf("%s is not a supported enum", name)
}
