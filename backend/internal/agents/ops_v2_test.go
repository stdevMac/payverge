package agents

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestFinalizeOpsGuidanceClassifiesSchemaFailureForTerminalTelemetry(t *testing.T) {
	_, _, _, err := finalizeOpsGuidanceWithValidation(OpsFinalizeInput{
		ResponseID: "", Locale: "en", Model: StructuredResponse{Answer: "Readable answer"},
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, errOpsAssistantSchemaValidation))
}

func TestFinalizeUnknownOpsGuidanceReportsDiscardedModelURLCandidates(t *testing.T) {
	for _, model := range []StructuredResponse{
		{Answer: "Use https://evil.example/track instead."},
		{Answer: "Use the safe prose.", Steps: []string{"Open [tracker](javascript:alert(1))"}},
		{Answer: "Use the safe prose.", FollowUps: []string{"Visit https://evil.example/follow-up"}},
	} {
		response, _, summary, err := finalizeOpsGuidanceWithValidation(OpsFinalizeInput{
			ResponseID: "ops-unknown-url", Locale: "en", BusinessID: 7, Model: model,
		})
		require.NoError(t, err)
		assert.NotContains(t, response.Answer.Content, "evil.example")
		assert.True(t, summary.ActionsDropped)
		assert.False(t, summary.SourcesDropped)
		assert.False(t, summary.EntitiesDropped)
	}
}

func TestOpsAskV2PersistsOneValidatedMultiGuideEnvelope(t *testing.T) {
	setupOpsServiceTestDB(t)
	var telemetry []llm.AITelemetryEvent
	llm.SetTelemetrySink(func(event llm.AITelemetryEvent) { telemetry = append(telemetry, event) })
	t.Cleanup(func() { llm.SetTelemetrySink(nil) })
	business := createOpsV2Business(t, "multi")
	service := NewOpsAssistantService(nil, NewRegistry(), database.GetDBWrapper(), nil)

	result, err := service.Ask(context.Background(), OpsAskRequest{
		BusinessID: business.ID,
		Message:    productionSpanishFiveTopicPrompt,
		Locale:     "es",
		ActiveTab:  "overview",
		Access: OpsAccessSnapshot{
			EffectivePermissions: map[string]bool{
				"menu:write": true, "tables:write": true, "plugins:write": true,
				"inventory:read": true, "ai_waiter:read": true,
			},
		},
	})
	require.NoError(t, err)
	require.NoError(t, assistantcontract.Validate(result.ResponseV2))
	assert.NotEmpty(t, result.ResponseV2.ResponseID)
	require.Len(t, result.ResponseV2.Sections, 5)
	assert.Equal(t, []string{
		"menu-add-item", "tables-create-qr", "plugins-connect", "inventory-stock", "ai-waiter-configure",
	}, opsV2SectionIDs(result.ResponseV2))
	assert.NotEmpty(t, result.Response.Answer, "V1 compatibility projection must remain present")

	var stored []database.OpsAssistantMessage
	require.NoError(t, database.GetDB().
		Where("business_id = ? AND role = ?", business.ID, database.OpsAssistantRoleAssistant).
		Find(&stored).Error)
	require.Len(t, stored, 1, "one ask must persist exactly one assistant envelope")
	var persisted assistantcontract.Response
	require.NoError(t, json.Unmarshal([]byte(stored[0].StructuredResponse), &persisted))
	require.NoError(t, assistantcontract.Validate(persisted))
	assert.Equal(t, result.ResponseV2, persisted)
	assert.Equal(t, result.Response.Answer, stored[0].Content)
	require.Len(t, telemetry, 1)
	assert.Equal(t, "ops", telemetry[0].Surface)
	assert.Equal(t, "v2", telemetry[0].ContractVersion)
	assert.Equal(t, "ok", telemetry[0].Outcome)
	assert.Equal(t, "es", telemetry[0].Language)
	assert.Equal(t, "offered", telemetry[0].ActionOutcome, "validated retained actions are trusted finalizer decisions")
	assert.Equal(t, "verified", telemetry[0].SourceOutcome, "validated retained sources are trusted finalizer decisions")
	assert.Equal(t, "none", telemetry[0].EntityOutcome)
	assert.Equal(t, "verified", telemetry[0].SchemaOutcome)
	assert.True(t, telemetry[0].V2ShadowValid)
	assert.Equal(t, "verified", telemetry[0].LanguageOutcome)
	assert.Positive(t, telemetry[0].TimeToFirstMs, "validated response availability must populate TTFC")
	assert.LessOrEqual(t, telemetry[0].TimeToFirstMs, telemetry[0].LatencyMs)
}

func TestAssistantTelemetryMapsNeedsClarificationToFallback(t *testing.T) {
	event := llm.AITelemetryEvent{
		ActionOutcome: "none", SourceOutcome: "none", EntityOutcome: "none",
		SchemaOutcome: "none", LanguageOutcome: "none",
	}
	response := assistantcontract.NewResponse("clarify-telemetry", "Which item did you mean?")
	response.Status = assistantcontract.StatusNeedsClarification

	applyAssistantResponseTelemetryWithValidation(&event, response, "none", assistantFinalizerValidation{})

	assert.Equal(t, "fallback", event.Outcome)
}

func TestAssistantTelemetryVerifiesOnlyValidatedRetainedObjects(t *testing.T) {
	event := llm.AITelemetryEvent{
		ActionOutcome: "none", SourceOutcome: "none", EntityOutcome: "none",
		SchemaOutcome: "none", LanguageOutcome: "none",
	}
	response := assistantcontract.NewResponse("retained-telemetry", "Here is the grounded option.")
	response.Actions = []assistantcontract.Action{{
		ID: "open-menu", Type: "navigate", Label: "Open menu",
		Target: assistantcontract.ActionTarget{Kind: "payverge_page", ID: "menu", Href: "/menu"},
		State:  "ready", Confirmation: "none",
	}}
	response.Sources = []assistantcontract.Source{{
		ID: "menu-source", Type: "menu_item", Title: "Soup",
		Origin: "business_menu", RetrievedAt: "2026-08-08T12:00:00Z",
	}}
	response.Entities = []assistantcontract.Entity{{
		ID: "menu_item:soup", Type: "menu_item", DisplayName: "Soup",
		Availability: "available", SourceID: "menu-source",
	}}

	applyAssistantResponseTelemetryWithValidation(&event, response, "none", assistantFinalizerValidation{})

	assert.Equal(t, "offered", event.ActionOutcome)
	assert.Equal(t, "verified", event.SourceOutcome)
	assert.Equal(t, "verified", event.EntityOutcome)
	assert.True(t, event.V2ShadowValid)
}

func TestAssistantTelemetryUsesTrustedFinalizerDropProvenance(t *testing.T) {
	response := assistantcontract.NewResponse("dropped-provenance", "Here is the verified guide for the requested dashboard task.")
	response.Actions = []assistantcontract.Action{{
		ID: "open-menu", Type: "navigate", Label: "Open menu",
		Target: assistantcontract.ActionTarget{Kind: "payverge_page", ID: "menu", Href: "/menu"},
		State:  "ready", Confirmation: "none",
	}}
	response.Sources = []assistantcontract.Source{{
		ID: "menu-source", Type: "dashboard_guide", Title: "Menu guide",
		Origin: "ops_guide_catalog", RetrievedAt: "2026-08-09T12:00:00Z",
	}}

	tests := []struct {
		name       string
		validation assistantFinalizerValidation
		wantAction string
		wantSource string
	}{
		{name: "retained only", wantAction: "offered", wantSource: "verified"},
		{
			name:       "retained and rejected candidates",
			validation: assistantFinalizerValidation{ActionsDropped: true, SourcesDropped: true},
			wantAction: "rejected", wantSource: "dropped",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := llm.AITelemetryEvent{
				Language: "en", ActionOutcome: "none", SourceOutcome: "none", EntityOutcome: "none",
				SchemaOutcome: "none", LanguageOutcome: "none",
			}
			applyAssistantResponseTelemetryWithValidation(&event, response, "none", tt.validation)
			assert.Equal(t, tt.wantAction, event.ActionOutcome)
			assert.Equal(t, tt.wantSource, event.SourceOutcome)
			assert.Equal(t, "none", event.EntityOutcome)
		})
	}
}

func TestAssistantTelemetryPreservesFirstValidatedContentTiming(t *testing.T) {
	event := llm.AITelemetryEvent{
		Language: "en", ActionOutcome: "none", SourceOutcome: "none", EntityOutcome: "none",
		SchemaOutcome: "none", LanguageOutcome: "none",
	}
	first := assistantcontract.NewResponse("first-content", "Here is the first complete validated response for your restaurant request today.")
	second := assistantcontract.NewResponse("second-content", "Here is a later validated fallback response for your restaurant request today.")

	applyAssistantResponseTelemetryWithValidation(&event, first, "none", assistantFinalizerValidation{}, 5*time.Millisecond)
	applyAssistantResponseTelemetryWithValidation(&event, second, "none", assistantFinalizerValidation{}, 50*time.Millisecond)

	assert.Equal(t, int64(5), event.TimeToFirstMs)
}

func TestAssistantTelemetryInvalidShadowIsNotMeasuredAsValidatedContent(t *testing.T) {
	event := llm.AITelemetryEvent{
		ActionOutcome: "none", SourceOutcome: "none", EntityOutcome: "none",
		SchemaOutcome: "none", LanguageOutcome: "none", Language: "en",
	}
	invalid := assistantcontract.NewResponse("", "Invalid response")

	applyAssistantResponseTelemetryWithValidation(&event, invalid, "none", assistantFinalizerValidation{})

	assert.False(t, event.V2ShadowValid)
	assert.Equal(t, "dropped", event.SchemaOutcome)
	assert.Zero(t, event.TimeToFirstMs)
}

func TestOpsTelemetryEmitsOneInvalidOrErrorTerminalEvent(t *testing.T) {
	setupOpsServiceTestDB(t)
	service := NewOpsAssistantService(nil, NewRegistry(), database.GetDBWrapper(), nil)
	var telemetry []llm.AITelemetryEvent
	llm.SetTelemetrySink(func(event llm.AITelemetryEvent) { telemetry = append(telemetry, event) })
	t.Cleanup(func() { llm.SetTelemetrySink(nil) })

	_, err := service.Ask(context.Background(), OpsAskRequest{Message: "", Locale: "en"})
	require.Error(t, err)
	require.Len(t, telemetry, 1)
	assert.Equal(t, "invalid", telemetry[0].Outcome)
	assert.Equal(t, "none", telemetry[0].SchemaOutcome)
	assert.Equal(t, "none", telemetry[0].LanguageOutcome)

	_, err = service.Ask(context.Background(), OpsAskRequest{
		BusinessID: 999999, Message: "Explain an ambiguous concern", Locale: "en",
	})
	require.Error(t, err)
	require.Len(t, telemetry, 2)
	assert.Equal(t, "error", telemetry[1].Outcome)
	assert.Equal(t, "none", telemetry[1].SchemaOutcome)
	assert.Equal(t, "none", telemetry[1].LanguageOutcome)
}

func TestOpsAskPersistsStructuredResponse(t *testing.T) {
	setupOpsServiceTestDB(t)
	var telemetry []llm.AITelemetryEvent
	llm.SetTelemetrySink(func(event llm.AITelemetryEvent) { telemetry = append(telemetry, event) })
	t.Cleanup(func() { llm.SetTelemetrySink(nil) })
	business := createOpsV2Business(t, "rollout-shadow")
	service := NewOpsAssistantService(nil, NewRegistry(), database.GetDBWrapper(), nil)

	result, err := service.Ask(context.Background(), OpsAskRequest{
		BusinessID: business.ID,
		Message:    "How do I add a menu item?",
		Locale:     "en",
		ActiveTab:  "overview",
		Access: OpsAccessSnapshot{
			EffectivePermissions: map[string]bool{"menu:write": true},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Response.Answer)
	require.NoError(t, assistantcontract.Validate(result.ResponseV2))

	var stored database.OpsAssistantMessage
	require.NoError(t, database.GetDB().
		Where("business_id = ? AND role = ?", business.ID, database.OpsAssistantRoleAssistant).
		First(&stored).Error)
	require.NotEmpty(t, stored.StructuredResponse)
	var persisted assistantcontract.Response
	require.NoError(t, json.Unmarshal([]byte(stored.StructuredResponse), &persisted))
	assert.Equal(t, result.ResponseV2, persisted)
	require.Len(t, telemetry, 1)
	assert.Equal(t, "v2", telemetry[0].ContractVersion)
	assert.True(t, telemetry[0].V2ShadowValid)
	assert.Positive(t, telemetry[0].TimeToFirstMs)
}

func TestOpsDeterministicTelemetryCapturesTTFCBeforeAssistantPersistence(t *testing.T) {
	setupOpsServiceTestDB(t)
	db := database.GetDB()
	var telemetry []llm.AITelemetryEvent
	const callbackName = "task2_delay_assistant_persistence"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		message, ok := tx.Statement.Dest.(*database.OpsAssistantMessage)
		if ok && message.Role == database.OpsAssistantRoleAssistant {
			time.Sleep(40 * time.Millisecond)
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callbackName) })
	llm.SetTelemetrySink(func(event llm.AITelemetryEvent) { telemetry = append(telemetry, event) })
	t.Cleanup(func() { llm.SetTelemetrySink(nil) })
	business := createOpsV2Business(t, "ttfc-before-persistence")
	service := NewOpsAssistantService(nil, NewRegistry(), database.GetDBWrapper(), nil)

	_, err := service.Ask(context.Background(), OpsAskRequest{
		BusinessID: business.ID, Message: "How do I add a menu item?", Locale: "en", ActiveTab: "overview",
		Access: OpsAccessSnapshot{
			EffectivePermissions: map[string]bool{"menu:write": true},
		},
	})
	require.NoError(t, err)
	require.Len(t, telemetry, 1)
	require.Positive(t, telemetry[0].TimeToFirstMs)
	require.GreaterOrEqual(t, telemetry[0].LatencyMs-telemetry[0].TimeToFirstMs, int64(30),
		"assistant persistence latency must not inflate time to first validated content")
}

func TestOpsAskV2ReplayRestoresExactClaimedAssistantMessage(t *testing.T) {
	setupOpsServiceTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.OpsAssistantRequest{}))
	business := createOpsV2Business(t, "replay-exact")
	thread, err := database.CreateOpsAssistantThread(business.ID, "Replay", "en")
	require.NoError(t, err)

	firstUser := &database.OpsAssistantMessage{
		ThreadID: thread.ID, BusinessID: business.ID, Role: database.OpsAssistantRoleUser,
		Locale: "en", Content: "first request",
	}
	require.NoError(t, database.SaveOpsAssistantMessage(firstUser))
	firstV2 := assistantcontract.NewResponse("ops-first-response", "First answer")
	firstAssistant, err := persistOpsAssistantResponse(
		thread.ID, business.ID, "en", "first-model", 11, firstV2,
	)
	require.NoError(t, err)

	secondUser := &database.OpsAssistantMessage{
		ThreadID: thread.ID, BusinessID: business.ID, Role: database.OpsAssistantRoleUser,
		Locale: "en", Content: "second request",
	}
	require.NoError(t, database.SaveOpsAssistantMessage(secondUser))
	secondV2 := assistantcontract.NewResponse("ops-second-response", "Second answer")
	secondAssistant, err := persistOpsAssistantResponse(
		thread.ID, business.ID, "en", "second-model", 22, secondV2,
	)
	require.NoError(t, err)
	require.Greater(t, secondAssistant.ID, firstAssistant.ID)

	claim, replay, err := database.ClaimOpsAssistantRequest(business.ID, "replay-first-request")
	require.NoError(t, err)
	require.False(t, replay)
	require.NoError(t, database.CompleteOpsAssistantRequest(
		claim.ID, thread.ID, firstUser.ID, firstAssistant.ID, "",
	))

	service := NewOpsAssistantService(nil, NewRegistry(), database.GetDBWrapper(), nil)
	var telemetry []llm.AITelemetryEvent
	llm.SetTelemetrySink(func(event llm.AITelemetryEvent) { telemetry = append(telemetry, event) })
	t.Cleanup(func() { llm.SetTelemetrySink(nil) })
	result, err := service.Ask(context.Background(), OpsAskRequest{
		BusinessID: business.ID, Message: "retry first request", Locale: "en",
		ClientRequestID: "replay-first-request",
	})
	require.NoError(t, err)
	assert.Equal(t, firstAssistant.ID, result.AssistantMessage.ID)
	assert.Equal(t, firstV2.ResponseID, result.ResponseV2.ResponseID)
	assert.Equal(t, firstV2.Answer.Content, result.Response.Answer)
	assert.NotEqual(t, secondAssistant.ID, result.AssistantMessage.ID)
	assert.Equal(t, Usage{}, result.Usage, "replay must not report a fresh model call")
	assert.Empty(t, telemetry, "durable replay must not emit another terminal assistant event")
}

func TestOpsAskV2InFlightDuplicateDoesNotEmitTerminalTelemetry(t *testing.T) {
	setupOpsServiceTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.OpsAssistantRequest{}))
	business := createOpsV2Business(t, "in-flight-duplicate")
	_, replay, err := database.ClaimOpsAssistantRequest(business.ID, "still-processing")
	require.NoError(t, err)
	require.False(t, replay)

	var telemetry []llm.AITelemetryEvent
	llm.SetTelemetrySink(func(event llm.AITelemetryEvent) { telemetry = append(telemetry, event) })
	t.Cleanup(func() { llm.SetTelemetrySink(nil) })
	service := NewOpsAssistantService(nil, NewRegistry(), database.GetDBWrapper(), nil)

	_, err = service.Ask(context.Background(), OpsAskRequest{
		BusinessID: business.ID, Message: "same request", Locale: "en",
		ClientRequestID: "still-processing",
	})

	require.ErrorIs(t, err, database.ErrOpsAssistantRequestInFlight)
	assert.Empty(t, telemetry, "an in-flight duplicate is not a new terminal assistant request")
}

func TestOpsAskV2ReplayRejectsClaimedMessageOutsideBusiness(t *testing.T) {
	setupOpsServiceTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.OpsAssistantRequest{}))
	business := createOpsV2Business(t, "replay-owner")
	foreign := createOpsV2Business(t, "replay-foreign")
	thread, err := database.CreateOpsAssistantThread(business.ID, "Owner thread", "en")
	require.NoError(t, err)
	foreignThread, err := database.CreateOpsAssistantThread(foreign.ID, "Foreign thread", "en")
	require.NoError(t, err)
	foreignResponse := assistantcontract.NewResponse("ops-foreign-response", "Foreign answer")
	foreignAssistant, err := persistOpsAssistantResponse(
		foreignThread.ID, foreign.ID, "en", "foreign-model", 0, foreignResponse,
	)
	require.NoError(t, err)

	claim, replay, err := database.ClaimOpsAssistantRequest(business.ID, "replay-foreign-message")
	require.NoError(t, err)
	require.False(t, replay)
	require.NoError(t, database.CompleteOpsAssistantRequest(
		claim.ID, thread.ID, 0, foreignAssistant.ID, "",
	))

	service := NewOpsAssistantService(nil, NewRegistry(), database.GetDBWrapper(), nil)
	result, err := service.Ask(context.Background(), OpsAskRequest{
		BusinessID: business.ID, Message: "retry", Locale: "en",
		ClientRequestID: "replay-foreign-message",
	})
	require.ErrorContains(t, err, "ops replay response unavailable")
	assert.Nil(t, result)
}

func TestOpsAskV2ReplayRejectsCompletedClaimWithoutAssistantMessage(t *testing.T) {
	setupOpsServiceTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.OpsAssistantRequest{}))
	business := createOpsV2Business(t, "replay-missing")
	thread, err := database.CreateOpsAssistantThread(business.ID, "Missing response", "en")
	require.NoError(t, err)
	claim, replay, err := database.ClaimOpsAssistantRequest(business.ID, "replay-missing-message")
	require.NoError(t, err)
	require.False(t, replay)
	require.NoError(t, database.GetDB().Model(&database.OpsAssistantRequest{}).
		Where("id = ?", claim.ID).
		Updates(map[string]any{"status": "completed", "thread_id": thread.ID, "assistant_message_id": nil}).Error)

	service := NewOpsAssistantService(nil, NewRegistry(), database.GetDBWrapper(), nil)
	result, err := service.Ask(context.Background(), OpsAskRequest{
		BusinessID: business.ID, Message: "retry", Locale: "en",
		ClientRequestID: "replay-missing-message",
	})
	require.ErrorContains(t, err, "ops replay response unavailable")
	assert.Nil(t, result)
}

func TestOpsHistoryRestoresV2V1AndMalformedRowsSafely(t *testing.T) {
	v2 := assistantcontract.NewResponse("ops-stored-v2", "Trusted V2 answer")
	v2JSON, err := json.Marshal(v2)
	require.NoError(t, err)

	legacyV2, restoredV2, ok := RestoreOpsResponse(41, 7, "fallback", string(v2JSON))
	require.True(t, ok)
	require.NoError(t, assistantcontract.Validate(restoredV2))
	assert.Equal(t, v2, restoredV2)
	assert.Equal(t, "Trusted V2 answer", legacyV2.Answer)

	v1JSON := `{"answer":"Legacy answer","steps":["Read the guide"],"actions":[{"label":"Menu","href":"/business/7/dashboard?tab=menu","kind":"navigate"},{"label":"Foreign","href":"/business/99/dashboard?tab=menu","kind":"navigate"},{"label":"Noncanonical","href":"/business/7/admin","kind":"navigate"},{"label":"Unsafe","href":"https://evil.test","kind":"navigate"}],"follow_ups":["Next"],"workflow":{"id":"attacker","step_index":0,"step_total":2}}`
	legacyV1, restoredV1, ok := RestoreOpsResponse(42, 7, "fallback", v1JSON)
	require.True(t, ok)
	require.NoError(t, assistantcontract.Validate(restoredV1))
	assert.Equal(t, "Legacy answer", legacyV1.Answer)
	assert.Equal(t, "ops-history-42", restoredV1.ResponseID)
	assert.Empty(t, restoredV1.Actions, "historical model-owned labels and actions must fail closed even on canonical hrefs")
	assert.Nil(t, restoredV1.Workflow, "historical model-owned workflows must not become continuation state")

	_, wrongTotalV1, ok := RestoreOpsResponse(47, 7, "fallback", `{"answer":"Legacy","steps":[],"actions":[],"follow_ups":[],"workflow":{"id":"first_menu_item","step_index":0,"step_total":999}}`)
	require.True(t, ok)
	assert.Nil(t, wrongTotalV1.Workflow, "even a known workflow ID is untrusted when restored from V1")

	malformedLegacy, malformedV2, ok := RestoreOpsResponse(43, 7, "Stored readable answer", `{"answer":`)
	require.True(t, ok)
	require.NoError(t, assistantcontract.Validate(malformedV2))
	assert.Equal(t, "Stored readable answer", malformedLegacy.Answer)
	assert.Equal(t, "Stored readable answer", malformedV2.Answer.Content)
	assert.Empty(t, malformedV2.Actions)

	foreignV2 := assistantcontract.NewResponse("ops-foreign-v2", "Untrusted stored answer")
	foreignV2.Actions = append(foreignV2.Actions, assistantcontract.Action{
		ID: "navigate:menu-add-item", Type: "navigate", Label: "Menu",
		Target: assistantcontract.ActionTarget{Kind: "dashboard_area", ID: "menu", Href: "/business/99/dashboard?tab=menu"},
		State:  "ready", Confirmation: "none",
	})
	require.NoError(t, assistantcontract.Validate(foreignV2))
	foreignJSON, err := json.Marshal(foreignV2)
	require.NoError(t, err)
	foreignLegacy, degradedV2, ok := RestoreOpsResponse(44, 7, "Safe stored content", string(foreignJSON))
	require.True(t, ok)
	assert.Equal(t, "Safe stored content", foreignLegacy.Answer)
	assert.Equal(t, assistantcontract.StatusDegraded, degradedV2.Status)
	assert.Empty(t, degradedV2.Actions)

	sharedDestinationV2 := assistantcontract.NewResponse("ops-shared-destination", "Two overview guides")
	for _, guideID := range []string{"overview-get-started", "support-contact"} {
		sharedDestinationV2.Actions = append(sharedDestinationV2.Actions, assistantcontract.Action{
			ID: "navigate:" + guideID, Type: "navigate", Label: guideID,
			Target: assistantcontract.ActionTarget{Kind: "dashboard_area", ID: "overview", Href: "/business/7/dashboard?tab=overview"},
			State:  "ready", Confirmation: "none",
		})
	}
	require.NoError(t, assistantcontract.Validate(sharedDestinationV2))
	sharedJSON, err := json.Marshal(sharedDestinationV2)
	require.NoError(t, err)
	_, sharedRestored, ok := RestoreOpsResponse(45, 7, "fallback", string(sharedJSON))
	require.True(t, ok)
	assert.Equal(t, sharedDestinationV2, sharedRestored, "each action ID must resolve its own guide even when hrefs are shared")

	rawHref := "/business/7/admin"
	rawSourceV2 := assistantcontract.NewResponse("ops-raw-source", "Untrusted source")
	rawSourceV2.Sources = append(rawSourceV2.Sources, assistantcontract.Source{
		ID: "model-source", Type: "payverge_page", Title: "Admin", Href: &rawHref,
		Origin: "model", RetrievedAt: time.Now().UTC().Format(time.RFC3339),
	})
	require.NoError(t, assistantcontract.Validate(rawSourceV2))
	rawSourceJSON, err := json.Marshal(rawSourceV2)
	require.NoError(t, err)
	_, degradedSourceV2, ok := RestoreOpsResponse(46, 7, "Safe source fallback", string(rawSourceJSON))
	require.True(t, ok)
	assert.Empty(t, degradedSourceV2.Sources)
	assert.Equal(t, "Safe source fallback", degradedSourceV2.Answer.Content)
}

func TestOpsAskV2ModelPathFiltersUnauthorizedReadTools(t *testing.T) {
	setupOpsServiceTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.OpsAssistantToolCall{}))
	business := createOpsV2Business(t, "model-denied")
	readTool := &opsReadStateStub{name: "get_business_context", data: map[string]any{
		"ai_enabled": true, "currency": "USD",
	}}
	registry := NewRegistry()
	registry.Register(readTool)
	provider := &finalizationProvider{responses: []*llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "unauthorized-read", Name: "get_business_context"}}},
		{Text: `{"answer":"I can explain the dashboard.","steps":[],"actions":[],"follow_ups":[]}`},
	}}
	ai, err := services.NewAIService(provider, llm.ModelConfig{Director: "test-model"})
	require.NoError(t, err)
	service := NewOpsAssistantService(ai, registry, database.GetDBWrapper(), nil)

	result, err := service.Ask(context.Background(), OpsAskRequest{
		BusinessID: business.ID,
		Message:    "Explain an ambiguous back-office concern.",
		Locale:     "en",
		ActiveTab:  "overview",
		Access: OpsAccessSnapshot{
			EffectivePermissions: map[string]bool{"assistant:read": true},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 0, readTool.calls, "the model must not receive or execute an unauthorized read tool")
	require.NoError(t, assistantcontract.Validate(result.ResponseV2))

	var persisted assistantcontract.Response
	require.NoError(t, json.Unmarshal([]byte(result.AssistantMessage.StructuredResponse), &persisted))
	require.NoError(t, assistantcontract.Validate(persisted))
	assert.Equal(t, result.ResponseV2, persisted)
}

func TestOpsAskV2ModelPathProjectsSharedReadToolByPermission(t *testing.T) {
	tests := []struct {
		name       string
		permission string
		wantCalls  int
		contains   []string
		excludes   []string
	}{
		{
			name: "ai waiter read sees only ai state", permission: "ai_waiter:read", wantCalls: 1,
			contains: []string{"ai_enabled"}, excludes: []string{"Secret Cafe", "currency"},
		},
		{
			// With SaaS billing gone the business context carries no financial
			// projection, so financial:read alone never reaches the tool.
			name: "financial read cannot read business context", permission: "financial:read", wantCalls: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupOpsServiceTestDB(t)
			require.NoError(t, database.GetDB().AutoMigrate(&database.OpsAssistantToolCall{}))
			business := createOpsV2Business(t, tt.name)
			readTool := &opsReadStateStub{name: "get_business_context", data: map[string]any{
				"id": business.ID, "name": "Secret Cafe", "currency": "USD",
				"ai_enabled": true,
			}}
			registry := NewRegistry()
			registry.Register(readTool)
			provider := &finalizationProvider{responses: []*llm.Response{
				{ToolCalls: []llm.ToolCall{{ID: "authorized-read", Name: "get_business_context"}}},
				{Text: `{"answer":"I can explain the dashboard.","steps":[],"actions":[],"follow_ups":[]}`},
			}}
			ai, err := services.NewAIService(provider, llm.ModelConfig{Director: "test-model"})
			require.NoError(t, err)
			service := NewOpsAssistantService(ai, registry, database.GetDBWrapper(), nil)

			_, err = service.Ask(context.Background(), OpsAskRequest{
				BusinessID: business.ID, Message: "Explain an ambiguous back-office concern.", Locale: "en",
				Access: OpsAccessSnapshot{EffectivePermissions: map[string]bool{tt.permission: true}},
			})
			require.NoError(t, err)
			require.Equal(t, tt.wantCalls, readTool.calls)
			if tt.wantCalls == 0 {
				return
			}
			require.Len(t, provider.requests, 2)
			toolWire := opsV2ToolMessages(provider.requests[1].Messages)
			for _, want := range tt.contains {
				assert.Contains(t, toolWire, want)
			}
			for _, unwanted := range tt.excludes {
				assert.NotContains(t, toolWire, unwanted)
			}
		})
	}
}

func TestOpsAskV2ModelPathAllowsPluginReadPermission(t *testing.T) {
	setupOpsServiceTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.OpsAssistantToolCall{}))
	business := createOpsV2Business(t, "plugin-read")
	readTool := &opsReadStateStub{name: "get_plugin_status", data: map[string]any{
		"plugins": []map[string]any{{"enabled": true, "connected": true, "status": "ok"}},
	}}
	registry := NewRegistry()
	registry.Register(readTool)
	provider := &finalizationProvider{responses: []*llm.Response{
		{ToolCalls: []llm.ToolCall{{ID: "plugin-read", Name: "get_plugin_status"}}},
		{Text: `{"answer":"I can explain the dashboard.","steps":[],"actions":[],"follow_ups":[]}`},
	}}
	ai, err := services.NewAIService(provider, llm.ModelConfig{Director: "test-model"})
	require.NoError(t, err)
	service := NewOpsAssistantService(ai, registry, database.GetDBWrapper(), nil)

	_, err = service.Ask(context.Background(), OpsAskRequest{
		BusinessID: business.ID, Message: "Explain an ambiguous back-office concern.", Locale: "en",
		Access: OpsAccessSnapshot{EffectivePermissions: map[string]bool{"plugins:read": true}},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, readTool.calls)
}

func TestOpsAskV2ModelPathCannotPersistMarkdownRoutesOrRawActions(t *testing.T) {
	response, legacy, _, err := finalizeOpsGuidanceWithValidation(OpsFinalizeInput{
		ResponseID: "ops-model-untrusted", Locale: "en", BusinessID: 7,
		Model: StructuredResponse{
			Answer:  "Use [Open settings](/business/999/dashboard?tab=settings) to continue.",
			Actions: []ActionLink{{Label: "Foreign settings", Href: "/business/999/dashboard?tab=settings", Kind: "navigate"}},
		},
	})
	require.NoError(t, err)
	require.NoError(t, assistantcontract.Validate(response))
	assert.Equal(t, assistantcontract.FormatPlainText, response.Answer.Format)
	assert.Contains(t, response.Answer.Content, "Open settings")
	assert.NotContains(t, response.Answer.Content, "/business/")
	assert.Empty(t, response.Actions)
	assert.Empty(t, response.Sources)
	assert.Empty(t, legacy.Actions)
}

func TestOpsReadOnlyPluginStateV2IsLocalizedPluralizedAndEnabledOnly(t *testing.T) {
	tests := []struct {
		name   string
		locale string
		rows   []map[string]any
		want   string
	}{
		{name: "english none", locale: "en", rows: []map[string]any{}, want: "0 plugins enabled; 0 connected. Status: none."},
		{name: "english singular", locale: "en", rows: []map[string]any{{"enabled": true, "connected": true, "status": "ok"}}, want: "1 plugin enabled; 1 connected. Status: ok."},
		{name: "spanish singular", locale: "es", rows: []map[string]any{{"enabled": true, "connected": false, "status": "unknown"}}, want: "1 plugin activado; 0 conectados. Estado: desconocido."},
		{name: "argentine plural enabled health ignores disabled error", locale: "es-AR", rows: []map[string]any{
			{"enabled": false, "connected": false, "status": "error"},
			{"enabled": true, "connected": true, "status": "ok"},
			{"enabled": true, "connected": false, "status": "error"},
		}, want: "2 plugins activados; 1 conectado. Estado: con errores."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := opsReadOnlyStateSummary(tt.locale, "plugins-connect", map[string]any{"plugins": tt.rows})
			assert.Equal(t, tt.want, got)
		})
	}
}

func createOpsV2Business(t *testing.T, suffix string) *database.Business {
	t.Helper()
	business := &database.Business{
		BusinessId: "ops-v2-" + suffix, Name: "Ops V2 Cafe", OwnerAddress: "0x",
		SettlementAddr: "0x1", TippingAddr: "0x2", IsActive: true,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, database.GetDB().Create(business).Error)
	return business
}

func opsV2SectionIDs(response assistantcontract.Response) []string {
	ids := make([]string, 0, len(response.Sections))
	for _, section := range response.Sections {
		ids = append(ids, section.ID)
	}
	return ids
}

func opsV2ToolMessages(messages []llm.Message) string {
	combined := ""
	for _, message := range messages {
		if message.Role == llm.RoleTool {
			combined += message.Text
		}
	}
	return combined
}
