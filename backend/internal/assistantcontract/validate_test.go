package assistantcontract

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

func TestValidateAcceptsNewResponse(t *testing.T) {
	require.NoError(t, Validate(NewResponse("resp_1", "Answer")))
}

func TestValidateRejectsInvalidEnvelope(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Response)
		want string
	}{
		{"wrong version", func(r *Response) { r.Version = 1 }, "version must be 2"},
		{"blank response ID", func(r *Response) { r.ResponseID = " \t" }, "response_id is required"},
		{"wrong status", func(r *Response) { r.Status = "done" }, "unsupported status"},
		{"wrong answer format", func(r *Response) { r.Answer.Format = "html" }, "unsupported answer format"},
		{"oversized answer", func(r *Response) { r.Answer.Content = strings.Repeat("界", 12001) }, "answer content exceeds 12000 runes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewResponse("resp_1", "Answer")
			tt.edit(&r)
			require.ErrorContains(t, Validate(r), tt.want)
		})
	}
}

func TestValidateRejectsBlankAndDuplicateIdentityIDs(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Response)
		want string
	}{
		{"blank section", func(r *Response) { r.Sections = []Section{{}} }, "section id is required"},
		{"duplicate section", func(r *Response) { r.Sections = []Section{{ID: "same"}, {ID: "same"}} }, "duplicate section id same"},
		{"blank action", func(r *Response) { r.Actions = []Action{{}} }, "action id is required"},
		{"duplicate action", func(r *Response) { r.Actions = []Action{{ID: "same"}, {ID: "same"}} }, "duplicate action id same"},
		{"blank source", func(r *Response) { r.Sources = []Source{{}} }, "source id is required"},
		{"duplicate source", func(r *Response) { r.Sources = []Source{{ID: "same"}, {ID: "same"}} }, "duplicate source id same"},
		{"blank entity", func(r *Response) { r.Entities = []Entity{{}} }, "entity id is required"},
		{"duplicate entity", func(r *Response) { r.Entities = []Entity{{ID: "same"}, {ID: "same"}} }, "duplicate entity id same"},
		{"blank follow-up", func(r *Response) { r.FollowUps = []FollowUp{{}} }, "follow-up id is required"},
		{"duplicate follow-up", func(r *Response) { r.FollowUps = []FollowUp{{ID: "same"}, {ID: "same"}} }, "duplicate follow-up id same"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewResponse("resp_1", "Answer")
			tt.edit(&r)
			require.ErrorContains(t, Validate(r), tt.want)
		})
	}
}

func TestValidateRejectsBlankAndDuplicateNoticeIDs(t *testing.T) {
	tests := []struct {
		name    string
		notices []Notice
		want    string
	}{
		{"blank", []Notice{{ID: " \t"}}, "notice id is required"},
		{"duplicate", []Notice{{ID: "notice_1"}, {ID: "notice_1"}}, "duplicate notice id notice_1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewResponse("resp_1", "Answer")
			r.Notices = tt.notices
			require.ErrorContains(t, Validate(r), tt.want)
		})
	}
}

func TestValidateRejectsNilSectionCollections(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Section)
		want string
	}{
		{"steps", func(s *Section) { s.Steps = nil }, "section menu steps must be an array"},
		{"action IDs", func(s *Section) { s.ActionIDs = nil }, "section menu action_ids must be an array"},
		{"source IDs", func(s *Section) { s.SourceIDs = nil }, "section menu source_ids must be an array"},
		{"entity IDs", func(s *Section) { s.EntityIDs = nil }, "section menu entity_ids must be an array"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewResponse("resp_1", "Answer")
			section := Section{
				ID:        "menu",
				Steps:     []string{},
				ActionIDs: []string{},
				SourceIDs: []string{},
				EntityIDs: []string{},
			}
			tt.edit(&section)
			r.Sections = []Section{section}
			require.ErrorContains(t, Validate(r), tt.want)
		})
	}
}

func TestValidateRejectsCollectionLimits(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Response)
		want string
	}{
		{"sections", func(r *Response) { r.Sections = make([]Section, 6) }, "sections exceeds limit 5"},
		{"actions", func(r *Response) { r.Actions = make([]Action, 9) }, "actions exceeds limit 8"},
		{"sources", func(r *Response) { r.Sources = make([]Source, 13) }, "sources exceeds limit 12"},
		{"entities", func(r *Response) { r.Entities = make([]Entity, 21) }, "entities exceeds limit 20"},
		{"follow-ups", func(r *Response) { r.FollowUps = make([]FollowUp, 6) }, "follow_ups exceeds limit 5"},
		{"steps", func(r *Response) { r.Steps = make([]string, 9) }, "steps exceeds limit 8"},
		{"notices", func(r *Response) { r.Notices = make([]Notice, 9) }, "notices exceeds limit 8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewResponse("resp_1", "Answer")
			tt.edit(&r)
			require.ErrorContains(t, Validate(r), tt.want)
		})
	}
}

func TestValidateRejectsNineSectionSteps(t *testing.T) {
	r := NewResponse("resp_1", "Answer")
	r.Sections = []Section{{
		ID:        "menu",
		Steps:     make([]string, 9),
		ActionIDs: []string{},
		SourceIDs: []string{},
		EntityIDs: []string{},
	}}
	require.ErrorContains(t, Validate(r), "section menu steps exceeds limit 8")
}

func TestValidateRejectsUnresolvedSectionAction(t *testing.T) {
	r := NewResponse("resp_1", "Answer")
	r.Sections = []Section{{ID: "menu", Title: "Menu", ActionIDs: []string{"missing"}}}
	require.ErrorContains(t, Validate(r), "section menu references unknown action missing")
}

func TestValidateRejectsUnresolvedSectionReferences(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Section)
		want string
	}{
		{"source", func(s *Section) { s.SourceIDs = []string{"missing"} }, "section menu references unknown source missing"},
		{"entity", func(s *Section) { s.EntityIDs = []string{"missing"} }, "section menu references unknown entity missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewResponse("resp_1", "Answer")
			s := Section{ID: "menu", Title: "Menu"}
			tt.edit(&s)
			r.Sections = []Section{s}
			require.ErrorContains(t, Validate(r), tt.want)
		})
	}
}

func TestValidateRejectsUnsafeOrMismatchedActionTargets(t *testing.T) {
	tests := []struct {
		name  string
		type_ string
		kind  string
		href  string
		want  string
	}{
		{"unsupported type", "execute_script", "target", "/safe", "unsupported action type execute_script"},
		{"external link with internal target", "external_link", "external_url", "/pricing", "external_link action act_1 requires a safe external href"},
		{"external link with credentials", "external_link", "external_url", "https://user:pass@example.com", "external_link action act_1 requires a safe external href"},
		{"navigate with external target", "navigate", "dashboard_area", "https://example.com", "navigate action act_1 requires a safe internal href"},
		{"navigate with protocol-relative target", "navigate", "dashboard_area", "//evil.example", "navigate action act_1 requires a safe internal href"},
		{"internal action with external target", "add_cart_item", "menu_item", "https://example.com/cart", "action act_1 cannot use an external href"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewResponse("resp_1", "Answer")
			var quantity *int
			if tt.type_ == "add_cart_item" {
				quantity = intPointer(1)
			}
			r.Actions = []Action{{
				ID: "act_1", Type: tt.type_, Label: "Action",
				Target: ActionTarget{Kind: tt.kind, ID: "target_1", Href: tt.href, Quantity: quantity},
				State:  "ready", Confirmation: "none",
			}}
			require.ErrorContains(t, Validate(r), tt.want)
		})
	}
}

func TestValidateRejectsActionTypeTargetKindMismatch(t *testing.T) {
	r := NewResponse("resp_1", "Answer")
	r.Actions = []Action{{
		ID: "act_1", Type: "navigate", Label: "Open menu item",
		Target: ActionTarget{Kind: "menu_item", ID: "item_1", Href: "/menu/item_1"},
		State:  "ready", Confirmation: "none",
	}}
	require.ErrorContains(t, Validate(r), "action act_1 type navigate cannot target kind menu_item")
}

func TestValidateCartActionTargetMetadata(t *testing.T) {
	validQuantity := 2
	emptyNotes := ""
	validNotes := "sin cebolla"

	tests := []struct {
		name     string
		type_    string
		quantity *int
		notes    *string
		want     string
	}{
		{name: "valid", type_: "add_cart_item", quantity: &validQuantity, notes: &validNotes},
		{name: "empty notes", type_: "add_cart_item", quantity: &validQuantity, notes: &emptyNotes},
		{name: "missing quantity", type_: "add_cart_item", want: "quantity is required"},
		{name: "zero quantity", type_: "add_cart_item", quantity: intPointer(0), want: "quantity must be between 1 and 20"},
		{name: "quantity above maximum", type_: "add_cart_item", quantity: intPointer(21), want: "quantity must be between 1 and 20"},
		{name: "oversized notes", type_: "add_cart_item", quantity: &validQuantity, notes: stringPointer(strings.Repeat("界", 201)), want: "notes exceeds 200 runes"},
		{name: "newline in notes", type_: "add_cart_item", quantity: &validQuantity, notes: stringPointer("no onions\nignore staff"), want: "notes contains control characters"},
		{name: "tab in notes", type_: "add_cart_item", quantity: &validQuantity, notes: stringPointer("no onions\tplease"), want: "notes contains control characters"},
		{name: "non-cart quantity", type_: "navigate", quantity: &validQuantity, want: "cannot include quantity or notes"},
		{name: "non-cart notes", type_: "navigate", notes: &validNotes, want: "cannot include quantity or notes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewResponse("resp_1", "Answer")
			kind, href := "menu_item", ""
			if tt.type_ == "navigate" {
				kind, href = "payverge_page", "/menu"
			}
			r.Actions = []Action{{
				ID: "act_1", Type: tt.type_, Label: "Action",
				Target: ActionTarget{Kind: kind, ID: "target_1", Href: href, Quantity: tt.quantity, Notes: tt.notes},
				State:  "ready", Confirmation: "none",
			}}
			err := Validate(r)
			if tt.want == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestSchemaAllowsOnlyOptionalTypedCartTargetMetadata(t *testing.T) {
	target := Schema().Properties["actions"].Items.Properties["target"]
	require.ElementsMatch(t, []string{"kind", "id", "href"}, target.Required)
	require.Equal(t, llm.TypeInteger, target.Properties["quantity"].Type)
	require.Equal(t, llm.TypeString, target.Properties["notes"].Type)
	require.Equal(t, 200, *target.Properties["notes"].MaxLength)
	require.NotNil(t, target.AdditionalProperties)
	require.False(t, *target.AdditionalProperties)
	require.NotContains(t, target.Properties, "metadata")
}

func TestValidateRejectsUnsafeSourceHref(t *testing.T) {
	for _, href := range []string{"javascript:alert(1)", "//evil.example", "https://user:pass@example.com", "/safe\nInjected"} {
		t.Run(href, func(t *testing.T) {
			r := NewResponse("resp_1", "Answer")
			r.Sources = []Source{{ID: "src_1", Type: "knowledge_entry", Title: "Guide", Href: &href, Origin: "knowledge", RetrievedAt: "2026-08-07T12:00:00Z"}}
			require.ErrorContains(t, Validate(r), "source src_1 has unsafe href")
		})
	}
}

func TestValidateRejectsInvalidWorkflowBounds(t *testing.T) {
	for _, workflow := range []*Workflow{
		{ID: "workflow_1", StepIndex: 0, StepTotal: 0},
		{ID: "workflow_1", StepIndex: -1, StepTotal: 2},
		{ID: "workflow_1", StepIndex: 2, StepTotal: 2},
	} {
		r := NewResponse("resp_1", "Answer")
		r.Workflow = workflow
		require.ErrorContains(t, Validate(r), "workflow requires 0 <= step_index < step_total with positive step_total")
	}
}

func TestValidateRejectsEntityWithoutValidSource(t *testing.T) {
	r := NewResponse("resp_1", "Answer")
	r.Entities = []Entity{{ID: "entity_1", Type: "menu_item", DisplayName: "Soup", Availability: "available", SourceID: "missing"}}
	require.ErrorContains(t, Validate(r), "entity entity_1 references unknown source missing")
}

func TestSafeHrefHelpers(t *testing.T) {
	require.True(t, IsSafeExternalHref(" https://example.com/path "))
	require.False(t, IsSafeExternalHref("https://user:pass@example.com"))
	require.False(t, IsSafeExternalHref("javascript:alert(1)"))
	require.True(t, IsSafeInternalHref("/business/7/dashboard?tab=menu"))
	require.False(t, IsSafeInternalHref("//evil.example"))
	require.False(t, IsSafeInternalHref("/safe\x00bad"))
}

func TestSafeInternalHrefRejectsBackslashesAndASCIIControls(t *testing.T) {
	for _, href := range []string{
		"/\\evil.example/path",
		"/\\\\evil.example/path",
		"/safe\tbad",
		"/safe\x1fbad",
		"/safe\x7fbad",
	} {
		t.Run(fmt.Sprintf("%q", href), func(t *testing.T) {
			require.False(t, IsSafeInternalHref(href))
		})
	}
}

func TestSchemaIsStrictAndBounded(t *testing.T) {
	schema := Schema()
	require.Equal(t, "object", schema.Type)
	require.ElementsMatch(t, []string{
		"version", "response_id", "answer", "sections", "steps", "actions",
		"sources", "entities", "follow_ups", "workflow", "notices", "status",
	}, schema.Required)
	require.Equal(t, 12000, *schema.Properties["answer"].Properties["content"].MaxLength)
	require.Equal(t, 5, *schema.Properties["sections"].MaxItems)
	require.Equal(t, 8, *schema.Properties["actions"].MaxItems)
	require.Equal(t, 12, *schema.Properties["sources"].MaxItems)
	require.Equal(t, 20, *schema.Properties["entities"].MaxItems)
	require.Equal(t, 5, *schema.Properties["follow_ups"].MaxItems)
	require.NotContains(t, schema.Properties["answer"].Properties["format"].Enum, "html")
	assertStrictObjectTree(t, schema)
}

func TestSchemaSupportsRequiredNullableFields(t *testing.T) {
	schema := Schema()
	requireSchemaTypes(t, schema.Properties["workflow"], llm.TypeObject, llm.TypeNull)

	action := schema.Properties["actions"].Items
	requireSchemaTypes(t, action.Properties["disabled_reason"], llm.TypeString, llm.TypeNull)
	requireSchemaTypes(t, action.Properties["expires_at"], llm.TypeString, llm.TypeNull)

	source := schema.Properties["sources"].Items
	requireSchemaTypes(t, source.Properties["href"], llm.TypeString, llm.TypeNull)
}

func TestSchemaConstrainsVersionToV2(t *testing.T) {
	payload, err := json.Marshal(Schema().Properties["version"])
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"integer","const":2}`, string(payload))
}

func TestSchemaConstrainsActionTargetKinds(t *testing.T) {
	targetKind := Schema().Properties["actions"].Items.Properties["target"].Properties["kind"]
	require.ElementsMatch(t, []string{
		"dashboard_area", "payverge_page", "external_url", "director",
		"menu_item", "bundle", "lead_form", "support_request",
	}, targetKind.Enum)
}

func TestSchemaMarshalingIsDeterministic(t *testing.T) {
	baseline, err := json.Marshal(Schema())
	require.NoError(t, err)
	for range 1000 {
		payload, marshalErr := json.Marshal(Schema())
		require.NoError(t, marshalErr)
		require.Equal(t, baseline, payload)
	}
}

func TestSchemaUsesClientStepAndNoticeLimits(t *testing.T) {
	schema := Schema()
	require.Equal(t, 8, *schema.Properties["steps"].MaxItems)
	require.Equal(t, 8, *schema.Properties["sections"].Items.Properties["steps"].MaxItems)
	require.Equal(t, 8, *schema.Properties["notices"].MaxItems)
}

func TestValidateRejectsSchemaConstrainedValues(t *testing.T) {
	overID := strings.Repeat("i", maxIDRunes+1)
	overType := strings.Repeat("k", maxTypeRunes+1)
	overNotice := strings.Repeat("n", maxNoticeRunes+1)
	overTimestamp := strings.Repeat("t", maxTimestampRunes+1)
	tests := []struct {
		name string
		edit func(*Response)
		want string
	}{
		{"entity type", func(r *Response) { r.Entities[0].Type = "category" }, "unsupported entity type category"},
		{"action target ID", func(r *Response) { r.Actions[0].Target.ID = overID }, "action target id exceeds 200 runes"},
		{"action target kind", func(r *Response) { r.Actions[0].Target.Kind = overType }, "action target kind exceeds 100 runes"},
		{"action state", func(r *Response) { r.Actions[0].State = overType }, "action state exceeds 100 runes"},
		{"action confirmation", func(r *Response) { r.Actions[0].Confirmation = overType }, "action confirmation exceeds 100 runes"},
		{"action disabled reason", func(r *Response) { r.Actions[0].DisabledReason = &overNotice }, "action disabled_reason exceeds 2000 runes"},
		{"action expiry", func(r *Response) { r.Actions[0].ExpiresAt = &overTimestamp }, "action expires_at exceeds 100 runes"},
		{"source origin", func(r *Response) { r.Sources[0].Origin = overType }, "source origin exceeds 100 runes"},
		{"source retrieved at", func(r *Response) { r.Sources[0].RetrievedAt = overTimestamp }, "source retrieved_at exceeds 100 runes"},
		{"entity availability", func(r *Response) { r.Entities[0].Availability = overType }, "entity availability exceeds 100 runes"},
		{"notice kind", func(r *Response) { r.Notices = []Notice{{ID: "notice_1", Kind: overType}} }, "notice kind exceeds 100 runes"},
		{"workflow ID", func(r *Response) { r.Workflow = &Workflow{ID: overID, StepIndex: 0, StepTotal: 1} }, "workflow id exceeds 200 runes"},
		{"section action IDs", func(r *Response) {
			r.Sections = []Section{{ID: "menu", Steps: []string{}, ActionIDs: repeatedIDs("act_1", maxActions+1), SourceIDs: []string{}, EntityIDs: []string{}}}
		}, "section menu action_ids exceeds limit 8"},
		{"section source IDs", func(r *Response) {
			r.Sections = []Section{{ID: "menu", Steps: []string{}, ActionIDs: []string{}, SourceIDs: repeatedIDs("src_1", maxSources+1), EntityIDs: []string{}}}
		}, "section menu source_ids exceeds limit 12"},
		{"section entity IDs", func(r *Response) {
			r.Sections = []Section{{ID: "menu", Steps: []string{}, ActionIDs: []string{}, SourceIDs: []string{}, EntityIDs: repeatedIDs("entity_1", maxEntities+1)}}
		}, "section menu entity_ids exceeds limit 20"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := validPopulatedResponse()
			tt.edit(&r)
			require.ErrorContains(t, Validate(r), tt.want)
		})
	}
}

func validPopulatedResponse() Response {
	r := NewResponse("resp_1", "Answer")
	r.Actions = []Action{{
		ID: "act_1", Type: "navigate", Label: "Open menu",
		Target: ActionTarget{Kind: "dashboard_area", ID: "menu", Href: "/menu"},
		State:  "ready", Confirmation: "none",
	}}
	r.Sources = []Source{{
		ID: "src_1", Type: "menu_item", Title: "Soup",
		Origin: "business_menu", RetrievedAt: "2026-08-07T12:00:00Z",
	}}
	r.Entities = []Entity{{
		ID: "entity_1", Type: "menu_item", DisplayName: "Soup",
		Availability: "available", SourceID: "src_1",
	}}
	return r
}

func repeatedIDs(id string, count int) []string {
	values := make([]string, count)
	for i := range values {
		values[i] = id
	}
	return values
}

func assertStrictObjectTree(t *testing.T, schema *llm.JSONSchema) {
	t.Helper()
	if schema.Type == llm.TypeObject {
		require.NotNil(t, schema.AdditionalProperties)
		require.False(t, *schema.AdditionalProperties)
		expectedRequired := mapKeys(schema.Properties)
		if _, hasQuantity := schema.Properties["quantity"]; hasQuantity {
			expectedRequired = withoutStrings(expectedRequired, "quantity", "notes")
		}
		require.ElementsMatch(t, expectedRequired, schema.Required)
	}
	for _, property := range schema.Properties {
		assertStrictObjectTree(t, property)
	}
	if schema.Items != nil {
		assertStrictObjectTree(t, schema.Items)
	}
	for _, option := range schema.AnyOf {
		assertStrictObjectTree(t, option)
	}
}

func withoutStrings(values []string, excluded ...string) []string {
	excludedSet := make(map[string]struct{}, len(excluded))
	for _, value := range excluded {
		excludedSet[value] = struct{}{}
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, skip := excludedSet[value]; !skip {
			result = append(result, value)
		}
	}
	return result
}

func requireSchemaTypes(t *testing.T, schema *llm.JSONSchema, expected ...string) {
	t.Helper()
	actual := make([]string, 0, len(schema.AnyOf))
	for _, option := range schema.AnyOf {
		actual = append(actual, option.Type)
	}
	require.ElementsMatch(t, expected, actual)
}

func mapKeys(values map[string]*llm.JSONSchema) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
