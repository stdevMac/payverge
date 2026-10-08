package assistantcontract

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type v1RetirementFixture struct {
	name     string
	response Response
}

func TestV1RetirementEveryProductionTerminalResponseProjectsReadableV1(t *testing.T) {
	for _, fixture := range v1RetirementResponses() {
		t.Run(fixture.name, func(t *testing.T) {
			legacy, err := ToLegacy(fixture.response)
			require.NoError(t, err)
			require.NotEmpty(t, strings.TrimSpace(legacy.Answer))
		})
	}
}

func v1RetirementResponses() []v1RetirementFixture {
	concierge := NewResponse(
		"concierge-retirement",
		"I found pricing information and the next step to book a demo.",
	)
	concierge.Actions = []Action{
		{
			ID: "navigate:pricing", Type: "navigate", Label: "View pricing",
			Target: ActionTarget{Kind: "payverge_page", ID: "pricing", Href: "/pricing"},
			State:  "ready", Confirmation: "none",
		},
		{
			ID: "navigate:demo", Type: "navigate", Label: "Book a demo",
			Target: ActionTarget{Kind: "payverge_page", ID: "demo", Href: "/book-demo"},
			State:  "ready", Confirmation: "none",
		},
	}
	concierge.Sections = []Section{
		{
			ID: "pricing", Title: "Pricing", Answer: "Review the available plans.",
			Steps: []string{}, ActionIDs: []string{"navigate:pricing"}, SourceIDs: []string{}, EntityIDs: []string{},
		},
		{
			ID: "demo", Title: "Demo", Answer: "Choose a time that works for you.",
			Steps: []string{}, ActionIDs: []string{"navigate:demo"}, SourceIDs: []string{}, EntityIDs: []string{},
		},
	}

	ops := NewResponse("ops-retirement", "I found two setup guides for this request.")
	ops.Status = StatusBlocked
	ops.Sections = []Section{
		{
			ID: "menu-guide", Title: "Menu setup", Answer: "Open Menu Builder to manage dishes.",
			Steps: []string{"Review the current menu"}, ActionIDs: []string{}, SourceIDs: []string{}, EntityIDs: []string{},
		},
		{
			ID: "orders-guide", Title: "Order setup", Answer: "Review ordering settings before enabling guest orders.",
			Steps: []string{"Confirm the ordering policy"}, ActionIDs: []string{}, SourceIDs: []string{}, EntityIDs: []string{},
		},
	}

	quantity := 1
	waiter := NewResponse("waiter-retirement", "I found the available soup you requested.")
	waiter.Actions = []Action{{
		ID: "cart:item-42", Type: "add_cart_item", Label: "Add tomato soup",
		Target: ActionTarget{Kind: "menu_item", ID: "item-42", Quantity: &quantity},
		State:  "ready", Confirmation: "explicit",
	}}
	waiter.Sources = []Source{{
		ID: "menu:item-42", Type: "menu_item", Title: "Tomato soup",
		Origin: "business_menu", RetrievedAt: "2026-08-08T12:00:00Z",
	}}
	waiter.Entities = []Entity{{
		ID: "menu_item:item-42", Type: "menu_item", DisplayName: "Tomato soup",
		Availability: "available", SourceID: "menu:item-42",
	}}

	degraded := NewResponse("concierge-degraded-retirement", "Please ask again in English or Spanish.")
	degraded.Status = StatusDegraded
	clarification := NewResponse("waiter-clarification-retirement", "Which soup would you like to add?")
	clarification.Status = StatusNeedsClarification

	return []v1RetirementFixture{
		{name: "concierge multi-intent", response: concierge},
		{name: "ops blocked multi-guide", response: ops},
		{name: "waiter V2-only cart action", response: waiter},
		{name: "waiter needs clarification", response: clarification},
		{name: "localized degraded fallback", response: degraded},
	}
}
