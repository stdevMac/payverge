package assistantcontract

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewResponseInitializesV2Collections(t *testing.T) {
	r := NewResponse("resp_1", "Hello")

	require.Equal(t, 2, r.Version)
	require.Equal(t, StatusComplete, r.Status)
	require.Equal(t, Answer{Format: FormatMarkdown, Content: "Hello"}, r.Answer)
	require.NotNil(t, r.Sections)
	require.NotNil(t, r.Steps)
	require.NotNil(t, r.Actions)
	require.NotNil(t, r.Sources)
	require.NotNil(t, r.Entities)
	require.NotNil(t, r.FollowUps)
	require.NotNil(t, r.Notices)
}

func TestNewResponseJSONEnvelopeShape(t *testing.T) {
	r := NewResponse("resp_1", "Hello")
	defaultPayload, err := json.Marshal(r)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"version": 2,
		"response_id": "resp_1",
		"answer": {"format": "markdown", "content": "Hello"},
		"sections": [],
		"steps": [],
		"actions": [],
		"sources": [],
		"entities": [],
		"follow_ups": [],
		"workflow": null,
		"notices": [],
		"status": "complete"
	}`, string(defaultPayload))

	disabledReason := "Requires permission"
	expiresAt := "2026-08-07T13:00:00Z"
	href := "/menu/42"
	r.Sections = append(r.Sections, Section{
		ID:        "menu",
		Title:     "Menu",
		Answer:    "Create and manage dishes from Menu Builder.",
		Steps:     []string{"Open Menu Builder"},
		ActionIDs: []string{"act_open_menu"},
		SourceIDs: []string{"src_menu_item_42"},
		EntityIDs: []string{"entity_menu_item_42"},
	})
	r.Steps = append(r.Steps, "Choose a dish")
	r.Actions = append(r.Actions, Action{
		ID:    "act_open_menu",
		Type:  "navigate",
		Label: "Open Menu",
		Target: ActionTarget{
			Kind: "dashboard_area",
			ID:   "menu",
			Href: "/business/7/dashboard?tab=menu",
		},
		State:          "disabled",
		Confirmation:   "none",
		DisabledReason: &disabledReason,
		ExpiresAt:      &expiresAt,
	})
	r.Sources = append(r.Sources, Source{
		ID:          "src_menu_item_42",
		Type:        "menu_item",
		Title:       "Harvest Bowl",
		Href:        &href,
		Origin:      "business_menu",
		RetrievedAt: "2026-08-07T12:00:00Z",
	})
	r.Entities = append(r.Entities, Entity{
		ID:           "entity_menu_item_42",
		Type:         "menu_item",
		DisplayName:  "Harvest Bowl",
		Availability: "available",
		SourceID:     "src_menu_item_42",
	})
	r.FollowUps = append(r.FollowUps, FollowUp{
		ID:     "follow_up_1",
		Label:  "Show unavailable dishes",
		Prompt: "Which dishes are unavailable?",
	})
	r.Workflow = &Workflow{ID: "workflow_1", StepIndex: 1, StepTotal: 3}
	r.Notices = append(r.Notices, Notice{
		ID:      "notice_1",
		Kind:    "warning",
		Message: "Some actions are unavailable.",
	})
	r.Status = StatusDegraded

	payload, err := json.Marshal(r)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"version": 2,
		"response_id": "resp_1",
		"answer": {"format": "markdown", "content": "Hello"},
		"sections": [{
			"id": "menu",
			"title": "Menu",
			"answer": "Create and manage dishes from Menu Builder.",
			"steps": ["Open Menu Builder"],
			"action_ids": ["act_open_menu"],
			"source_ids": ["src_menu_item_42"],
			"entity_ids": ["entity_menu_item_42"]
		}],
		"steps": ["Choose a dish"],
		"actions": [{
			"id": "act_open_menu",
			"type": "navigate",
			"label": "Open Menu",
			"target": {
				"kind": "dashboard_area",
				"id": "menu",
				"href": "/business/7/dashboard?tab=menu"
			},
			"state": "disabled",
			"confirmation": "none",
			"disabled_reason": "Requires permission",
			"expires_at": "2026-08-07T13:00:00Z"
		}],
		"sources": [{
			"id": "src_menu_item_42",
			"type": "menu_item",
			"title": "Harvest Bowl",
			"href": "/menu/42",
			"origin": "business_menu",
			"retrieved_at": "2026-08-07T12:00:00Z"
		}],
		"entities": [{
			"id": "entity_menu_item_42",
			"type": "menu_item",
			"display_name": "Harvest Bowl",
			"availability": "available",
			"source_id": "src_menu_item_42"
		}],
		"follow_ups": [{
			"id": "follow_up_1",
			"label": "Show unavailable dishes",
			"prompt": "Which dishes are unavailable?"
		}],
		"workflow": {"id": "workflow_1", "step_index": 1, "step_total": 3},
		"notices": [{"id": "notice_1", "kind": "warning", "message": "Some actions are unavailable."}],
		"status": "degraded"
	}`, string(payload))
}

func TestActionTargetCartMetadataJSONRoundTrip(t *testing.T) {
	quantity := 2
	notes := "sin cebolla"
	target := ActionTarget{
		Kind: "menu_item", ID: "item-42", Quantity: &quantity, Notes: &notes,
	}

	payload, err := json.Marshal(target)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"kind":"menu_item",
		"id":"item-42",
		"href":"",
		"quantity":2,
		"notes":"sin cebolla"
	}`, string(payload))

	var restored ActionTarget
	require.NoError(t, json.Unmarshal(payload, &restored))
	require.Equal(t, target, restored)

	emptyNotes := ""
	target.Notes = &emptyNotes
	payload, err = json.Marshal(target)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"kind":"menu_item",
		"id":"item-42",
		"href":"",
		"quantity":2,
		"notes":""
	}`, string(payload))
}

func TestActionTargetNonCartJSONShapeIsUnchanged(t *testing.T) {
	target := ActionTarget{Kind: "payverge_page", ID: "pricing", Href: "/pricing"}
	payload, err := json.Marshal(target)
	require.NoError(t, err)
	require.JSONEq(t, `{"kind":"payverge_page","id":"pricing","href":"/pricing"}`, string(payload))
}

func TestActionTargetJSONRejectsNonIntegerQuantityShapes(t *testing.T) {
	for _, payload := range []string{
		`{"kind":"menu_item","id":"item-42","href":"","quantity":1.5}`,
		`{"kind":"menu_item","id":"item-42","href":"","quantity":"2"}`,
		`{"kind":"menu_item","id":"item-42","href":"","quantity":true}`,
	} {
		t.Run(payload, func(t *testing.T) {
			var target ActionTarget
			require.Error(t, json.Unmarshal([]byte(payload), &target))
		})
	}
}

func TestFromLegacyCreatesStableActionIDs(t *testing.T) {
	legacy := LegacyResponse{Answer: "Open pricing", Actions: []LegacyAction{{Label: "Pricing", Href: "/pricing", Kind: "navigate"}}}
	r, err := FromLegacy("resp_1", legacy)
	require.NoError(t, err)
	require.Equal(t, FormatPlainText, r.Answer.Format)
	require.Equal(t, "action-1", r.Actions[0].ID)
	require.Equal(t, "/pricing", r.Actions[0].Target.Href)
}

func TestFromLegacyProjectsAllFieldsAndValidates(t *testing.T) {
	legacy := LegacyResponse{
		Answer: "Choose where to go",
		Steps:  []string{"Review pricing", "Ask the director"},
		Actions: []LegacyAction{
			{Label: "Pricing", Href: "/pricing", Kind: "navigate"},
			{Label: "Docs", Href: "https://docs.payverge.io/start", Kind: "external"},
			{Label: "Director", Kind: "handoff", Disabled: true, DisabledReason: "Owner access required"},
		},
		FollowUps: []string{"Which plan fits me?", "What can the director do?"},
		Workflow:  &Workflow{ID: "choose-plan", StepIndex: 1, StepTotal: 3},
	}

	r, err := FromLegacy("resp_all", legacy)
	require.NoError(t, err)
	require.NoError(t, Validate(r))
	require.Equal(t, "Choose where to go", r.Answer.Content)
	require.Equal(t, []string{"Review pricing", "Ask the director"}, r.Steps)
	require.Equal(t, &Workflow{ID: "choose-plan", StepIndex: 1, StepTotal: 3}, r.Workflow)
	require.Equal(t, []Action{
		{
			ID: "action-1", Type: "navigate", Label: "Pricing",
			Target: ActionTarget{Kind: "payverge_page", ID: "legacy-target-1", Href: "/pricing"},
			State:  "ready", Confirmation: "none",
		},
		{
			ID: "action-2", Type: "external_link", Label: "Docs",
			Target: ActionTarget{Kind: "external_url", ID: "legacy-target-2", Href: "https://docs.payverge.io/start"},
			State:  "ready", Confirmation: "none",
		},
		{
			ID: "action-3", Type: "director_handoff", Label: "Director",
			Target: ActionTarget{Kind: "director", ID: "legacy-target-3"},
			State:  "disabled", Confirmation: "none", DisabledReason: stringPointer("Owner access required"),
		},
	}, r.Actions)
	require.Equal(t, []FollowUp{
		{ID: "follow-up-1", Label: "Which plan fits me?", Prompt: "Which plan fits me?"},
		{ID: "follow-up-2", Label: "What can the director do?", Prompt: "What can the director do?"},
	}, r.FollowUps)
	require.Nil(t, r.Actions[0].DisabledReason)
}

func TestFromLegacyUsesTargetOnlyAsSafeCompatibilityFallback(t *testing.T) {
	r, err := FromLegacy("resp_fallback", LegacyResponse{
		Actions: []LegacyAction{{Label: "Pricing", Target: "/pricing", Kind: "navigate"}},
	})
	require.NoError(t, err)
	require.Equal(t, "/pricing", r.Actions[0].Target.Href)

	_, err = FromLegacy("resp_unsafe_href", LegacyResponse{
		Actions: []LegacyAction{{Label: "Pricing", Href: "javascript:alert(1)", Target: "/pricing", Kind: "navigate"}},
	})
	require.ErrorContains(t, err, "unsafe")
}

func TestFromLegacyRejectsUnknownOrUnsafeActions(t *testing.T) {
	tests := []struct {
		name   string
		action LegacyAction
		want   string
	}{
		{"unknown kind", LegacyAction{Label: "Run", Href: "/safe", Kind: "execute"}, "unsupported legacy action kind"},
		{"unsafe navigation", LegacyAction{Label: "Open", Href: "//evil.example", Kind: "navigate"}, "unsafe"},
		{"unsafe external link", LegacyAction{Label: "Open", Href: "javascript:alert(1)", Kind: "external"}, "unsafe"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := FromLegacy("resp_invalid", LegacyResponse{Actions: []LegacyAction{tt.action}})
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestFromLegacyInitializesEmptyCollections(t *testing.T) {
	r, err := FromLegacy("resp_empty", LegacyResponse{})
	require.NoError(t, err)
	require.NotNil(t, r.Sections)
	require.NotNil(t, r.Steps)
	require.NotNil(t, r.Actions)
	require.NotNil(t, r.Sources)
	require.NotNil(t, r.Entities)
	require.NotNil(t, r.FollowUps)
	require.NotNil(t, r.Notices)
}

func TestLegacyResponseMatchesV1JSONWire(t *testing.T) {
	payload, err := json.Marshal(LegacyResponse{
		Answer: "Open pricing",
		Steps:  []string{},
		Actions: []LegacyAction{{
			Label: "Pricing", Href: "/pricing", Target: "/fallback", Kind: "navigate",
			Disabled: true, DisabledReason: "Not available",
		}},
		FollowUps: []string{},
		Workflow:  &Workflow{ID: "pricing", StepIndex: 0, StepTotal: 1},
	})
	require.NoError(t, err)
	require.JSONEq(t, `{
		"answer":"Open pricing",
		"steps":[],
		"actions":[{
			"label":"Pricing",
			"href":"/pricing",
			"target":"/fallback",
			"kind":"navigate",
			"disabled":true,
			"disabled_reason":"Not available"
		}],
		"follow_ups":[],
		"workflow":{"id":"pricing","step_index":0,"step_total":1}
	}`, string(payload))
}

func TestToLegacyProjectsRepresentableFields(t *testing.T) {
	disabledReason := "Owner access required"
	r := NewResponse("resp_legacy", "Choose where to go")
	r.Steps = []string{"Review pricing", "Ask the director"}
	r.Actions = []Action{
		{
			ID: "action-1", Type: "navigate", Label: "Pricing",
			Target: ActionTarget{Kind: "payverge_page", ID: "pricing", Href: "/pricing"},
			State:  "ready", Confirmation: "none",
		},
		{
			ID: "action-2", Type: "external_link", Label: "Docs",
			Target: ActionTarget{Kind: "external_url", ID: "docs", Href: "https://docs.payverge.io/start"},
			State:  "ready", Confirmation: "none",
		},
		{
			ID: "action-3", Type: "director_handoff", Label: "Director",
			Target: ActionTarget{Kind: "director", ID: "director"},
			State:  "disabled", Confirmation: "none", DisabledReason: &disabledReason,
		},
	}
	r.FollowUps = []FollowUp{
		{ID: "follow-up-1", Label: "Plan details", Prompt: "Which plan fits me?"},
		{ID: "follow-up-2", Label: "What can the director do?"},
	}
	r.Workflow = &Workflow{ID: "choose-plan", StepIndex: 1, StepTotal: 3}

	legacy, err := ToLegacy(r)
	require.NoError(t, err)
	require.Equal(t, LegacyResponse{
		Answer: "Choose where to go",
		Steps:  []string{"Review pricing", "Ask the director"},
		Actions: []LegacyAction{
			{Label: "Pricing", Href: "/pricing", Kind: "navigate"},
			{Label: "Docs", Href: "https://docs.payverge.io/start", Kind: "external"},
			{Label: "Director", Kind: "handoff", Disabled: true, DisabledReason: "Owner access required"},
		},
		FollowUps: []string{"Which plan fits me?", "What can the director do?"},
		Workflow:  &Workflow{ID: "choose-plan", StepIndex: 1, StepTotal: 3},
	}, legacy)
}

func TestToLegacyFlattensSectionsInOrder(t *testing.T) {
	r := NewResponse("resp_sections", "Overview")
	r.Steps = []string{"Start here"}
	r.Sections = []Section{
		{
			ID: "menu", Title: "Menu", Answer: "Edit dishes.",
			Steps: []string{"Open menu"}, ActionIDs: []string{}, SourceIDs: []string{}, EntityIDs: []string{},
		},
		{
			ID: "orders", Answer: "Review active orders.",
			Steps: []string{}, ActionIDs: []string{}, SourceIDs: []string{}, EntityIDs: []string{},
		},
		{
			ID: "tables", Title: "Tables",
			Steps: []string{"Open tables"}, ActionIDs: []string{}, SourceIDs: []string{}, EntityIDs: []string{},
		},
	}

	legacy, err := ToLegacy(r)
	require.NoError(t, err)
	require.Equal(t, "Overview\n\nMenu\nEdit dishes.\n\nReview active orders.\n\nTables", legacy.Answer)
	require.Equal(t, []string{"Start here", "Open menu", "Open tables"}, legacy.Steps)
}

func TestToLegacyFlattensSectionsWithoutEmptySeparators(t *testing.T) {
	r := NewResponse("resp_sections", "")
	r.Sections = []Section{
		{ID: "empty", Steps: []string{}, ActionIDs: []string{}, SourceIDs: []string{}, EntityIDs: []string{}},
		{ID: "answer", Answer: "Only useful copy.", Steps: []string{}, ActionIDs: []string{}, SourceIDs: []string{}, EntityIDs: []string{}},
	}

	legacy, err := ToLegacy(r)
	require.NoError(t, err)
	require.Equal(t, "Only useful copy.", legacy.Answer)
}

func TestToLegacyFailsClosedForUnavailableActionStates(t *testing.T) {
	tests := []struct {
		name   string
		state  string
		reason *string
		want   string
	}{
		{name: "pending", state: "pending"},
		{name: "expired", state: "expired"},
		{name: "unavailable", state: "unavailable"},
		{name: "preserves trusted reason", state: "pending", reason: stringPointer("Owner approval required"), want: "Owner approval required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewResponse("resp_state", "Open pricing")
			r.Actions = []Action{{
				ID: "action-1", Type: "navigate", Label: "Pricing",
				Target: ActionTarget{Kind: "payverge_page", ID: "pricing", Href: "/pricing"},
				State:  tt.state, Confirmation: "none", DisabledReason: tt.reason,
			}}

			legacy, err := ToLegacy(r)
			require.NoError(t, err)
			require.True(t, legacy.Actions[0].Disabled)
			require.Equal(t, tt.want, legacy.Actions[0].DisabledReason)
		})
	}
}

func TestToLegacyDisablesConfirmationAndExpiryCapabilities(t *testing.T) {
	expiresAt := "2026-08-08T20:00:00Z"
	tests := []struct {
		name         string
		confirmation string
		expiresAt    *string
		want         string
	}{
		{
			name: "confirmation required", confirmation: "required",
		},
		{
			name: "expires at", confirmation: "none", expiresAt: &expiresAt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewResponse("resp_capability", "Open pricing")
			r.Actions = []Action{{
				ID: "action-1", Type: "navigate", Label: "Pricing",
				Target: ActionTarget{Kind: "payverge_page", ID: "pricing", Href: "/pricing"},
				State:  "ready", Confirmation: tt.confirmation, ExpiresAt: tt.expiresAt,
			}}

			legacy, err := ToLegacy(r)
			require.NoError(t, err)
			require.True(t, legacy.Actions[0].Disabled)
			require.Equal(t, tt.want, legacy.Actions[0].DisabledReason)
		})
	}
}

func TestToLegacyOmitsV2OnlyActions(t *testing.T) {
	r := NewResponse("resp_cart", "I found the dish.")
	quantity := 1
	r.Actions = []Action{
		{
			ID: "action-cart", Type: "add_cart_item", Label: "Add soup",
			Target: ActionTarget{Kind: "menu_item", ID: "item-1", Href: "/menu/item-1", Quantity: &quantity},
			State:  "ready", Confirmation: "none",
		},
		{
			ID: "action-menu", Type: "navigate", Label: "View menu",
			Target: ActionTarget{Kind: "payverge_page", ID: "menu", Href: "/menu"},
			State:  "ready", Confirmation: "none",
		},
	}

	legacy, err := ToLegacy(r)
	require.NoError(t, err)
	require.Equal(t, []LegacyAction{{Label: "View menu", Href: "/menu", Kind: "navigate"}}, legacy.Actions)
}

func TestToLegacyValidatesBeforeProjection(t *testing.T) {
	r := NewResponse("resp_unsafe", "Open this")
	r.Actions = []Action{{
		ID: "action-1", Type: "external_link", Label: "Unsafe",
		Target: ActionTarget{Kind: "external_url", ID: "unsafe", Href: "javascript:alert(1)"},
		State:  "ready", Confirmation: "none",
	}}

	_, err := ToLegacy(r)
	require.ErrorContains(t, err, "safe external href")
}

func TestToLegacyInitializesEmptyCollections(t *testing.T) {
	legacy, err := ToLegacy(NewResponse("resp_empty", ""))
	require.NoError(t, err)
	require.NotNil(t, legacy.Steps)
	require.NotNil(t, legacy.Actions)
	require.NotNil(t, legacy.FollowUps)
}

func stringPointer(value string) *string {
	return &value
}
