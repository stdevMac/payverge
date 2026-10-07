package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stretchr/testify/require"
)

func TestRecordAssistantTerminal_RecordsTrustedSchemaAndLanguageValidation(t *testing.T) {
	responseBefore := testutil.ToFloat64(AssistantResponsesTotal.WithLabelValues("ops", "v2", "fallback"))
	schemaBefore := testutil.ToFloat64(AssistantValidationTotal.WithLabelValues("ops", "schema", "verified"))
	languageBefore := testutil.ToFloat64(AssistantValidationTotal.WithLabelValues("ops", "language", "dropped"))
	actionBefore := testutil.ToFloat64(AssistantValidationTotal.WithLabelValues("ops", "action", "verified"))

	err := RecordAssistantTerminal(llm.AITelemetryEvent{
		Feature: "ops_assistant", Surface: "ops", ContractVersion: "v2", Outcome: "fallback",
		ActionOutcome: "offered", SourceOutcome: "none", EntityOutcome: "none",
		SchemaOutcome: "verified", LanguageOutcome: "dropped", Language: "en",
		LatencyMs: 40, TimeToFirstMs: 25,
	})
	require.NoError(t, err)
	require.Equal(t, responseBefore+1, testutil.ToFloat64(AssistantResponsesTotal.WithLabelValues("ops", "v2", "fallback")))
	require.Equal(t, schemaBefore+1, testutil.ToFloat64(AssistantValidationTotal.WithLabelValues("ops", "schema", "verified")))
	require.Equal(t, languageBefore+1, testutil.ToFloat64(AssistantValidationTotal.WithLabelValues("ops", "language", "dropped")))
	require.Equal(t, actionBefore+1, testutil.ToFloat64(AssistantValidationTotal.WithLabelValues("ops", "action", "verified")))
}

func TestRecordAssistantTerminal_RejectsMaliciousValidationBeforeMutation(t *testing.T) {
	before := testutil.ToFloat64(AssistantResponsesTotal.WithLabelValues("waiter", "v2", "ok"))
	err := RecordAssistantTerminal(llm.AITelemetryEvent{
		Feature: "waiter", Surface: "waiter", ContractVersion: "v2", Outcome: "ok",
		ActionOutcome: "none", SourceOutcome: "none", EntityOutcome: "none",
		SchemaOutcome: "verified https://evil.example", LanguageOutcome: "verified", Language: "en",
	})
	require.Error(t, err)
	require.Equal(t, before, testutil.ToFloat64(AssistantResponsesTotal.WithLabelValues("waiter", "v2", "ok")))
}

func TestRecordAssistantTerminal_DoesNotCountCompatibleLegacyEvent(t *testing.T) {
	before := testutil.ToFloat64(AssistantResponsesTotal.WithLabelValues("waiter", "v2", "ok"))
	legacy := llm.AITelemetryEvent{Feature: "waiter", Surface: "waiter", Code: "ok"}
	require.NoError(t, llm.ValidateTelemetryEvent(legacy), "legacy events with omitted terminal fields stay schema-compatible")
	require.Error(t, RecordAssistantTerminal(legacy))
	require.Equal(t, before, testutil.ToFloat64(AssistantResponsesTotal.WithLabelValues("waiter", "v2", "ok")))
}

// The marketing concierge was removed with the SaaS funnel; its surface label
// must not be accepted (and so cannot mint a dead Prometheus series).
func TestRecordAssistantTerminal_RejectsRemovedConciergeSurface(t *testing.T) {
	err := RecordAssistantTerminal(llm.AITelemetryEvent{
		Feature: "concierge", Surface: "concierge", ContractVersion: "v2", Outcome: "ok",
		ActionOutcome: "none", SourceOutcome: "none", EntityOutcome: "none",
		SchemaOutcome: "verified", LanguageOutcome: "none", Language: "en",
		LatencyMs: 25,
	})
	require.Error(t, err)
}

func TestRecordAssistantTerminal_DoesNotObserveUnavailableTimeToFirstContent(t *testing.T) {
	observer := AssistantTimeToFirstContent.WithLabelValues("ops", "v1")
	metric, ok := observer.(prometheus.Metric)
	require.True(t, ok)
	before := &dto.Metric{}
	require.NoError(t, metric.Write(before))
	require.Zero(t, before.GetHistogram().GetSampleCount())

	require.NoError(t, RecordAssistantTerminal(llm.AITelemetryEvent{
		Feature: "ops_assistant", Surface: "ops", ContractVersion: "v1", Outcome: "fallback",
		ActionOutcome: "none", SourceOutcome: "none", EntityOutcome: "none",
		SchemaOutcome: "verified", LanguageOutcome: "none", Language: "en",
		LatencyMs: 25, TimeToFirstMs: 0,
	}))

	after := &dto.Metric{}
	require.NoError(t, metric.Write(after))
	require.Zero(t, after.GetHistogram().GetSampleCount(),
		"zero means TTFC was not measured and must not become a fabricated observation")
}
