package aicontract

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
	"github.com/stretchr/testify/require"
)

var assistantV2RequiredScenarioIDs = []string{
	"ops-two-howto-ordered-en",
	"ops-staff-without-permission-en",
	"ops-permission-hidden-destination",
	"ops-mutation-request-director-handoff-en",
	"ops-es-five-topic-five-sections",
	"ops-oversized-guides-fallback-en",
	"ops-history-restores-actions-sources-workflow",
	"waiter-in-stock-action-en",
	"waiter-translated-name-ambiguous-es",
	"waiter-unavailable-id-rejected",
	"waiter-ordering-disabled-en",
	"waiter-allergen-cart-safety-es",
	"waiter-long-menu-v2-sections-and-sources",
	"waiter-v2-history-roundtrip",
}

func actionTargetInvariantKey(action assistantcontract.Action) string {
	quantity := ""
	if action.Target.Quantity != nil {
		quantity = strconv.Itoa(*action.Target.Quantity)
	}
	notes := ""
	if action.Target.Notes != nil {
		notes = *action.Target.Notes
	}
	return strings.Join([]string{action.Type, action.Target.Kind, action.Target.ID, action.Target.Href, quantity, notes}, "|")
}

func sourceInvariantKey(source assistantcontract.Source) string {
	href := ""
	if source.Href != nil {
		href = *source.Href
	}
	return strings.Join([]string{source.ID, source.Type, source.Origin, href}, "|")
}

func entityInvariantKey(entity assistantcontract.Entity) string {
	return strings.Join([]string{entity.ID, entity.Type, entity.DisplayName, entity.Availability, entity.SourceID}, "|")
}

func invariantSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

// AssertAssistantV2Invariants validates the actual production-finalizer output
// carried by a scenario runner. It never creates response fields or effects.
func AssertAssistantV2Invariants(sc Scenario, result ScenarioRunResult) error {
	expect := sc.Expect.AssistantV2
	if expect == nil {
		return fmt.Errorf("scenario %s has no assistant_v2 expectations", sc.ID)
	}
	var response assistantcontract.Response
	decoder := json.NewDecoder(strings.NewReader(result.Text))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return fmt.Errorf("scenario %s decode V2: %w", sc.ID, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("scenario %s decode V2 trailing value: %v", sc.ID, err)
	}
	if expect.RolloutEnabled && response.Version != 2 {
		return fmt.Errorf("scenario %s rollout enabled but contract version is %d", sc.ID, response.Version)
	}
	if err := assistantcontract.Validate(response); err != nil {
		return fmt.Errorf("scenario %s invalid V2: %w", sc.ID, err)
	}

	allowedActions := invariantSet(expect.AllowedActionTargets)
	for _, action := range response.Actions {
		key := actionTargetInvariantKey(action)
		if _, ok := allowedActions[key]; !ok {
			return fmt.Errorf("scenario %s action target is not server-allowed: %s", sc.ID, key)
		}
	}
	trustedSources := invariantSet(expect.TrustedSources)
	for _, source := range response.Sources {
		key := sourceInvariantKey(source)
		if _, ok := trustedSources[key]; !ok {
			return fmt.Errorf("scenario %s source is not trusted evidence: %s", sc.ID, key)
		}
	}
	canonicalEntities := invariantSet(expect.CanonicalEntities)
	availabilityByTarget := make(map[string]string, len(response.Entities))
	for _, entity := range response.Entities {
		key := entityInvariantKey(entity)
		if _, ok := canonicalEntities[key]; !ok {
			return fmt.Errorf("scenario %s entity is not canonical: %s", sc.ID, key)
		}
		availabilityByTarget[entity.ID] = entity.Availability
	}
	if expect.NoUnavailableWaiterAction {
		for _, action := range response.Actions {
			if action.Type != "add_cart_item" {
				continue
			}
			availability, ok := availabilityByTarget["menu_item:"+action.Target.ID]
			if !ok || availability != "available" {
				return fmt.Errorf("scenario %s waiter action targets unknown/unavailable item %s", sc.ID, action.Target.ID)
			}
		}
	}
	if expect.ExactAnswer != "" && response.Answer.Content != expect.ExactAnswer {
		return fmt.Errorf("scenario %s exact answer mismatch: %q", sc.ID, response.Answer.Content)
	}
	for _, fragment := range expect.LocaleContains {
		if !containsFold(result.Text, fragment) {
			return fmt.Errorf("scenario %s locale %s missing %q", sc.ID, sc.Locale, fragment)
		}
	}
	for _, fragment := range expect.LocaleExcludes {
		if containsFold(result.Text, fragment) {
			return fmt.Errorf("scenario %s locale %s contains forbidden %q", sc.ID, sc.Locale, fragment)
		}
	}
	if expect.RequireReadableV1 {
		legacy, err := assistantcontract.ToLegacy(response)
		if err != nil {
			return fmt.Errorf("scenario %s V1 projection: %w", sc.ID, err)
		}
		if strings.TrimSpace(legacy.Answer) == "" {
			return fmt.Errorf("scenario %s V1 projection is empty", sc.ID)
		}
	}
	for _, key := range expect.RequiredZeroEffects {
		value, observed := result.Effects[key]
		if !observed {
			return fmt.Errorf("scenario %s required observed effect %s is missing", sc.ID, key)
		}
		if value != 0 {
			return fmt.Errorf("scenario %s observed effect %s=%d", sc.ID, key, value)
		}
	}
	if expect.ExactProviderCalls != nil && result.ProviderCalls != *expect.ExactProviderCalls {
		return fmt.Errorf("scenario %s provider calls want %d got %d", sc.ID, *expect.ExactProviderCalls, result.ProviderCalls)
	}
	return nil
}

func TestAssistantV2InvariantScenarioFieldsAreRequired(t *testing.T) {
	scenario := Scenario{
		ID:      "assistant-v2-invariant-red",
		Surface: "ops_assistant",
		Locale:  "es",
		Expect: ExpectedOutcome{
			Code: "ok",
			AssistantV2: &AssistantV2Expected{
				RolloutEnabled:       true,
				AllowedActionTargets: []string{"navigate|payverge_page|pricing|/pricing||"},
				TrustedSources:       []string{"pricing:plans|pricing_registry|pricing_registry|/pricing"},
				CanonicalEntities:    []string{},
				LocaleContains:       []string{"precios"},
				RequireReadableV1:    true,
			},
		},
	}

	result := ScenarioRunResult{Text: `{"version":2}`}
	require.Error(t, AssertAssistantV2Invariants(scenario, result))
}

func TestAssistantV2ProductionScenarioMatrix(t *testing.T) {
	scenarios, err := LoadScenarios(filepath.Join("testdata", "scenarios.yaml"))
	require.NoError(t, err)
	byID := make(map[string]Scenario, len(scenarios))
	for _, scenario := range scenarios {
		byID[scenario.ID] = scenario
	}

	for _, scenarioID := range assistantV2RequiredScenarioIDs {
		scenario, ok := byID[scenarioID]
		require.True(t, ok, "missing required Assistant V2 scenario %s", scenarioID)
		require.NotNil(t, scenario.Expect.AssistantV2, "scenario %s must declare assistant_v2 invariants", scenarioID)
		runner, ok := GetScenarioRunner(scenarioID)
		require.True(t, ok, "scenario %s must use a registered production-path runner", scenarioID)

		t.Run(scenarioID, func(t *testing.T) {
			result, runErr := runner(scenario)
			require.NoError(t, runErr)
			require.NoError(t, MatchExpect(scenario, result))
			require.NoError(t, AssertAssistantV2Invariants(scenario, result))
		})
	}
}

func TestAssistantV2InvariantRejectsUntrustedReferences(t *testing.T) {
	quantity := 1
	response := assistantcontract.NewResponse("invariant_probe", "English verified answer")
	response.Sources = append(response.Sources, assistantcontract.Source{
		ID: "menu:menu_item:item-1", Type: "menu_item", Title: "Item 1",
		Origin: "business_menu", RetrievedAt: "2026-08-08T12:00:00Z",
	})
	response.Entities = append(response.Entities, assistantcontract.Entity{
		ID: "menu_item:item-1", Type: "menu_item", DisplayName: "Item 1",
		Availability: "available", SourceID: "menu:menu_item:item-1",
	})
	response.Actions = append(response.Actions, assistantcontract.Action{
		ID: "cart:item-1", Type: "add_cart_item", Label: "Add Item 1",
		Target: assistantcontract.ActionTarget{Kind: "menu_item", ID: "item-1", Quantity: &quantity},
		State:  "ready", Confirmation: "explicit",
	})
	scenario := Scenario{
		ID: "invariant-probe", Surface: "waiter", Locale: "en",
		Expect: ExpectedOutcome{Code: "ok", AssistantV2: &AssistantV2Expected{
			RolloutEnabled:       true,
			AllowedActionTargets: []string{"add_cart_item|menu_item|item-1||1|"},
			TrustedSources:       []string{"menu:menu_item:item-1|menu_item|business_menu|"},
			CanonicalEntities:    []string{"menu_item:item-1|menu_item|Item 1|available|menu:menu_item:item-1"},
			LocaleContains:       []string{"English verified"}, LocaleExcludes: []string{"Respuesta"},
			RequireReadableV1: true, NoUnavailableWaiterAction: true,
		}},
	}
	encode := func(value assistantcontract.Response) ScenarioRunResult {
		raw, err := json.Marshal(value)
		require.NoError(t, err)
		return ScenarioRunResult{Text: string(raw)}
	}
	require.NoError(t, AssertAssistantV2Invariants(scenario, encode(response)))

	tests := []struct {
		name   string
		mutate func(*assistantcontract.Response)
		want   string
	}{
		{name: "action target", mutate: func(r *assistantcontract.Response) { r.Actions[0].Target.ID = "unknown" }, want: "not server-allowed"},
		{name: "action quantity", mutate: func(r *assistantcontract.Response) { quantity := 2; r.Actions[0].Target.Quantity = &quantity }, want: "not server-allowed"},
		{name: "action notes", mutate: func(r *assistantcontract.Response) { notes := "model note"; r.Actions[0].Target.Notes = &notes }, want: "not server-allowed"},
		{name: "source origin", mutate: func(r *assistantcontract.Response) { r.Sources[0].Origin = "model" }, want: "not trusted evidence"},
		{name: "entity identity", mutate: func(r *assistantcontract.Response) {
			r.Entities[0].DisplayName = "Model alias"
			r.Entities[0].Availability = "unknown"
		}, want: "not canonical"},
		{name: "locale", mutate: func(r *assistantcontract.Response) { r.Answer.Content = "Respuesta" }, want: "locale en missing"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := response
			candidate.Actions = append([]assistantcontract.Action{}, response.Actions...)
			candidate.Sources = append([]assistantcontract.Source{}, response.Sources...)
			candidate.Entities = append([]assistantcontract.Entity{}, response.Entities...)
			test.mutate(&candidate)
			require.ErrorContains(t, AssertAssistantV2Invariants(scenario, encode(candidate)), test.want)
		})
	}
}
