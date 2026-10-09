package llm

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateTelemetryEvent_RejectsContentBearingLabels(t *testing.T) {
	require.Error(t, ValidateTelemetryEvent(AITelemetryEvent{
		Feature: "waiter",
		Code:    "user said hello@example.com",
	}))
	require.Error(t, ValidateTelemetryEvent(AITelemetryEvent{
		Feature:    "ops_assistant",
		ErrorClass: "SELECT * FROM users",
	}))
	require.Error(t, ValidateTelemetryEvent(AITelemetryEvent{
		Feature: "director",
		GuideID: "Bearer sk_live_abc1234567890",
	}))
}

func TestValidateTelemetryEvent_AcceptsBoundedEnums(t *testing.T) {
	require.NoError(t, ValidateTelemetryEvent(AITelemetryEvent{
		Feature:         "ops_assistant",
		Surface:         "ops",
		Code:            "ai_timeout",
		ErrorClass:      "timeout",
		ServedModel:     "google/gemini-2.5-flash",
		GuideID:         "menu-add-item",
		Verdict:         "ok",
		BusinessID:      7,
		LatencyMs:       120,
		TokensIn:        10,
		TokensOut:       20,
		ContractVersion: "v2",
		Outcome:         "ok",
		ActionOutcome:   "offered",
		SourceOutcome:   "verified",
		EntityOutcome:   "verified",
		SchemaOutcome:   "verified",
		LanguageOutcome: "verified",
		Language:        "es-AR",
		RepairCount:     1,
		ToolCalls:       2,
		TimeToFirstMs:   45,
		V2ShadowValid:   true,
	}))
}

func TestTelemetryEventAlwaysSerializesBoundedShadowValidity(t *testing.T) {
	raw, err := json.Marshal(AITelemetryEvent{Feature: "waiter", V2ShadowValid: false})
	require.NoError(t, err)
	require.JSONEq(t, `{"feature":"waiter","surface":"","v2_shadow_valid":false}`, string(raw))
}

func TestValidatedResponseLanguageOutcomeUsesConfidentBaseLanguageDetection(t *testing.T) {
	tests := []struct {
		name      string
		canonical string
		text      string
		fallback  string
		want      string
	}{
		{
			name: "English match", canonical: "en",
			text: "Welcome to our restaurant, how can I help you with your order today?",
			want: "verified",
		},
		{
			name: "Spanish regional match", canonical: "es-AR",
			text: "Bienvenido a nuestro restaurante, ¿en qué puedo ayudarte con tu pedido?",
			want: "verified",
		},
		{
			name: "confident mismatch", canonical: "es-AR",
			text: "Welcome to our restaurant, how can I help you with your order today?",
			want: "dropped",
		},
		{
			name: "long English mismatch", canonical: "es-AR",
			text: "The restaurant menu includes several fresh dishes and the server can help with your complete order today.",
			want: "dropped",
		},
		{
			name: "long Spanish mismatch", canonical: "en",
			text: "El menú del restaurante incluye varios platos frescos y el mozo puede ayudarte con todo tu pedido hoy.",
			want: "dropped",
		},
		{
			name: "long Chinese script mismatch", canonical: "en",
			text: "餐厅今天提供多种新鲜菜肴，服务员可以帮助您完成整个订单。",
			want: "dropped",
		},
		{
			name: "long Arabic script mismatch", canonical: "en",
			text: "يقدم المطعم اليوم العديد من الأطباق الطازجة ويمكن للنادل مساعدتك في إكمال طلبك بالكامل.",
			want: "dropped",
		},
		{name: "short script is inconclusive", canonical: "en", text: "مرحبا", want: "none"},
		{name: "inconclusive", canonical: "en", text: "Okay", want: "none"},
		{name: "ambiguous detector token", canonical: "nl", text: "kan", want: "none"},
		{
			name: "Latin text below documented eight word floor", canonical: "en",
			text: "The menu with fresh dishes arrives today",
			want: "none",
		},
		{
			name: "one stopword cannot make arbitrary Latin text conclusive", canonical: "en",
			text: "the xylophone quartz nebula cobalt vertex zephyr marigold tundra",
			want: "none",
		},
		{
			name: "Danish is outside detector allowlist", canonical: "da",
			text: "Restauranten har mange friske retter og personalet kan hjælpe med hele din bestilling i dag.",
			want: "none",
		},
		{
			name: "Norwegian is outside detector allowlist", canonical: "no",
			text: "Restauranten har mange ferske retter og personalet kan hjelpe med hele bestillingen din i dag.",
			want: "none",
		},
		{
			name: "Polish is outside detector allowlist", canonical: "pl",
			text: "Restauracja ma wiele świeżych dań, a obsługa może dziś pomóc w całym zamówieniu.",
			want: "none",
		},
		{
			name: "Swedish is outside detector allowlist", canonical: "sv",
			text: "Restaurangen har många färska rätter och personalen kan hjälpa till med hela din beställning idag.",
			want: "none",
		},
		{
			name: "Vietnamese is outside detector allowlist", canonical: "vi",
			text: "Nhà hàng có nhiều món ăn tươi và nhân viên có thể giúp bạn hoàn tất đơn hàng hôm nay.",
			want: "none",
		},
		{
			name: "unsupported request fallback stays dropped", canonical: "en",
			text:     "Welcome to our restaurant, how can I help you with your order today?",
			fallback: "dropped", want: "dropped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ValidatedResponseLanguageOutcome(tt.canonical, tt.text, tt.fallback))
		})
	}
}

func TestValidateTelemetryEvent_AcceptsOmittedAssistantOutcomeFields(t *testing.T) {
	require.NoError(t, ValidateTelemetryEvent(AITelemetryEvent{
		Feature: "director",
		Surface: "director",
		Code:    "ok",
		Verdict: "ok",
	}))
}

func TestValidateTelemetryEvent_RejectsUnboundedAssistantOutcomeFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*AITelemetryEvent)
	}{
		{name: "contract version", mutate: func(ev *AITelemetryEvent) { ev.ContractVersion = "v3" }},
		{name: "outcome", mutate: func(ev *AITelemetryEvent) { ev.Outcome = "mostly ok" }},
		{name: "action outcome", mutate: func(ev *AITelemetryEvent) { ev.ActionOutcome = "clicked https://example.com" }},
		{name: "source outcome", mutate: func(ev *AITelemetryEvent) { ev.SourceOutcome = "source-title" }},
		{name: "entity outcome", mutate: func(ev *AITelemetryEvent) { ev.EntityOutcome = "burger@example.com" }},
		{name: "schema outcome", mutate: func(ev *AITelemetryEvent) { ev.SchemaOutcome = "schema https://example.com" }},
		{name: "language outcome", mutate: func(ev *AITelemetryEvent) { ev.LanguageOutcome = "mostly valid" }},
		{name: "unsupported locale", mutate: func(ev *AITelemetryEvent) { ev.Language = "xx-not-real" }},
		{name: "locale whitespace", mutate: func(ev *AITelemetryEvent) { ev.Language = " es" }},
		{name: "locale case alias", mutate: func(ev *AITelemetryEvent) { ev.Language = "ES" }},
		{name: "locale separator alias", mutate: func(ev *AITelemetryEvent) { ev.Language = "es_AR" }},
		{name: "negative repair count", mutate: func(ev *AITelemetryEvent) { ev.RepairCount = -1 }},
		{name: "negative tool calls", mutate: func(ev *AITelemetryEvent) { ev.ToolCalls = -1 }},
		{name: "negative first content", mutate: func(ev *AITelemetryEvent) { ev.TimeToFirstMs = -1 }},
		{name: "negative latency", mutate: func(ev *AITelemetryEvent) { ev.LatencyMs = -1 }},
		{name: "negative input tokens", mutate: func(ev *AITelemetryEvent) { ev.TokensIn = -1 }},
		{name: "negative output tokens", mutate: func(ev *AITelemetryEvent) { ev.TokensOut = -1 }},
		{name: "excessive repair count", mutate: func(ev *AITelemetryEvent) { ev.RepairCount = 17 }},
		{name: "excessive tool calls", mutate: func(ev *AITelemetryEvent) { ev.ToolCalls = 65 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := AITelemetryEvent{
				Feature: "concierge", Surface: "concierge", ContractVersion: "v2", Outcome: "ok",
				ActionOutcome: "none", SourceOutcome: "none", EntityOutcome: "none",
				SchemaOutcome: "none", LanguageOutcome: "none", Language: "en",
			}
			tt.mutate(&ev)
			require.Error(t, ValidateTelemetryEvent(ev))
		})
	}
}

func TestEmitTelemetry_DropsInvalidAndDeliversValid(t *testing.T) {
	var got []AITelemetryEvent
	SetTelemetrySink(func(ev AITelemetryEvent) { got = append(got, ev) })
	t.Cleanup(func() { SetTelemetrySink(nil) })

	EmitTelemetry(AITelemetryEvent{Feature: "waiter", Code: "user said hello@example.com"})
	require.Empty(t, got, "content-bearing labels must be dropped")

	EmitTelemetry(AITelemetryEvent{
		Feature: "director",
		Surface: "director",
		Code:    "ai_timeout",
		Verdict: "ok",
	})
	require.Len(t, got, 1)
	require.Equal(t, "director", got[0].Feature)
	require.Equal(t, "ai_timeout", got[0].Code)
}

// Production AI surfaces that must emit privacy-safe telemetry events.
var requiredTelemetrySurfaces = []string{
	"waiter", "waiter_whatsapp", "ops_assistant", "concierge",
	"director", "extraction", "wizard", "image", "marketing",
}

func TestTelemetry_SurfaceCompletenessContract(t *testing.T) {
	var got []AITelemetryEvent
	SetTelemetrySink(func(ev AITelemetryEvent) { got = append(got, ev) })
	t.Cleanup(func() { SetTelemetrySink(nil) })

	for _, surface := range requiredTelemetrySurfaces {
		// Success / block / timeout / privacy signals for each surface.
		for _, code := range []string{"ok", "ai_timeout", "ai_privacy_policy", "guardrail_blocked"} {
			EmitTelemetry(AITelemetryEvent{
				Feature: surface,
				Surface: surface,
				Code:    code,
				Verdict: "ok",
			})
		}
	}
	require.GreaterOrEqual(t, len(got), len(requiredTelemetrySurfaces)*4)
	// No content-bearing labels survived validation.
	for _, ev := range got {
		require.NoError(t, ValidateTelemetryEvent(ev))
		require.NotContains(t, ev.Code, "@")
		require.NotContains(t, ev.Feature, " ")
	}
}

func TestLoadModelConfig_ProductionAlignedDefaults(t *testing.T) {
	// Wave 6: defaults must match production-aligned model IDs.
	t.Setenv("OPENROUTER_MODEL_CHAT", "")
	t.Setenv("OPENROUTER_MODEL_MENU", "")
	t.Setenv("OPENROUTER_MODEL_DIRECTOR", "")
	t.Setenv("OPENROUTER_MODEL_IMAGE", "")
	t.Setenv("OPENROUTER_MODEL_GUARDRAIL", "")
	cfg := LoadModelConfig()
	require.Equal(t, "google/gemini-2.5-flash", cfg.Chat)
	require.Equal(t, "google/gemini-2.5-flash", cfg.Menu)
	require.Equal(t, "google/gemini-2.5-flash", cfg.Director)
	require.Equal(t, "google/gemini-2.5-flash-image", cfg.Image)
	require.Equal(t, "google/gemini-2.5-flash-lite", cfg.Guardrail)
}
