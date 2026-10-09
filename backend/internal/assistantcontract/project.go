package assistantcontract

import (
	"fmt"
	"strings"
)

// LegacyAction matches agents.ActionLink without importing the agents package.
type LegacyAction struct {
	Label          string `json:"label"`
	Href           string `json:"href"`
	Target         string `json:"target,omitempty"`
	Kind           string `json:"kind"`
	Disabled       bool   `json:"disabled"`
	DisabledReason string `json:"disabled_reason,omitempty"`
}

// LegacyResponse matches agents.StructuredResponse without importing agents.
type LegacyResponse struct {
	Answer    string         `json:"answer"`
	Steps     []string       `json:"steps"`
	Actions   []LegacyAction `json:"actions"`
	FollowUps []string       `json:"follow_ups"`
	Workflow  *Workflow      `json:"workflow,omitempty"`
}

// FromLegacy projects the V1 assistant wire into a validated V2 response.
func FromLegacy(responseID string, legacy LegacyResponse) (Response, error) {
	response := NewResponse(responseID, legacy.Answer)
	response.Answer.Format = FormatPlainText
	response.Steps = append(response.Steps, legacy.Steps...)
	response.Workflow = copyWorkflow(legacy.Workflow)

	for i, legacyAction := range legacy.Actions {
		action, err := actionFromLegacy(i+1, legacyAction)
		if err != nil {
			return Response{}, err
		}
		response.Actions = append(response.Actions, action)
	}
	for i, followUp := range legacy.FollowUps {
		response.FollowUps = append(response.FollowUps, FollowUp{
			ID:     fmt.Sprintf("follow-up-%d", i+1),
			Label:  followUp,
			Prompt: followUp,
		})
	}

	if err := Validate(response); err != nil {
		return Response{}, fmt.Errorf("project legacy response: %w", err)
	}
	return response, nil
}

// ToLegacy projects a validated V2 response into the readable V1 wire.
func ToLegacy(response Response) (LegacyResponse, error) {
	if err := Validate(response); err != nil {
		return LegacyResponse{}, fmt.Errorf("project v2 response: %w", err)
	}

	legacy := LegacyResponse{
		Answer:    flattenLegacyAnswer(response.Answer.Content, response.Sections),
		Steps:     append([]string{}, response.Steps...),
		Actions:   []LegacyAction{},
		FollowUps: []string{},
		Workflow:  copyWorkflow(response.Workflow),
	}
	for _, section := range response.Sections {
		legacy.Steps = append(legacy.Steps, section.Steps...)
	}
	for _, action := range response.Actions {
		kind, representable := legacyActionKind(action.Type)
		if !representable {
			// V1 can only render links and handoffs. Omitting V2-only actions keeps
			// the response readable without inventing an executable V1 target.
			continue
		}

		disabled, disabledReason := legacyActionAvailability(action)
		legacyAction := LegacyAction{
			Label:          action.Label,
			Href:           action.Target.Href,
			Kind:           kind,
			Disabled:       disabled,
			DisabledReason: disabledReason,
		}
		legacy.Actions = append(legacy.Actions, legacyAction)
	}
	for _, followUp := range response.FollowUps {
		prompt := followUp.Prompt
		if blank(prompt) {
			prompt = followUp.Label
		}
		legacy.FollowUps = append(legacy.FollowUps, prompt)
	}

	return legacy, nil
}

func flattenLegacyAnswer(answer string, sections []Section) string {
	parts := make([]string, 0, len(sections)+1)
	if answer != "" {
		parts = append(parts, answer)
	}
	for _, section := range sections {
		sectionParts := make([]string, 0, 2)
		if section.Title != "" {
			sectionParts = append(sectionParts, section.Title)
		}
		if section.Answer != "" {
			sectionParts = append(sectionParts, section.Answer)
		}
		if len(sectionParts) != 0 {
			parts = append(parts, strings.Join(sectionParts, "\n"))
		}
	}
	return strings.Join(parts, "\n\n")
}

func legacyActionAvailability(action Action) (bool, string) {
	disabled := action.State != "ready" || action.Confirmation != "none" || action.ExpiresAt != nil
	if !disabled {
		return false, ""
	}
	if action.DisabledReason != nil && !blank(*action.DisabledReason) {
		return true, *action.DisabledReason
	}
	return true, ""
}

func actionFromLegacy(index int, legacy LegacyAction) (Action, error) {
	href := legacy.Href
	if href == "" {
		href = legacy.Target
	}

	actionType, targetKind, err := v2ActionKind(legacy.Kind)
	if err != nil {
		return Action{}, fmt.Errorf("legacy action %d: %w", index, err)
	}
	if err := validateLegacyHref(legacy.Kind, href); err != nil {
		return Action{}, fmt.Errorf("legacy action %d: %w", index, err)
	}

	state := "ready"
	if legacy.Disabled {
		state = "disabled"
	}
	action := Action{
		ID:    fmt.Sprintf("action-%d", index),
		Type:  actionType,
		Label: legacy.Label,
		Target: ActionTarget{
			Kind: targetKind,
			ID:   fmt.Sprintf("legacy-target-%d", index),
			Href: href,
		},
		State:        state,
		Confirmation: "none",
	}
	if legacy.DisabledReason != "" {
		disabledReason := legacy.DisabledReason
		action.DisabledReason = &disabledReason
	}
	return action, nil
}

func v2ActionKind(kind string) (actionType, targetKind string, err error) {
	switch kind {
	case "navigate":
		return "navigate", "payverge_page", nil
	case "external":
		return "external_link", "external_url", nil
	case "handoff":
		return "director_handoff", "director", nil
	default:
		return "", "", fmt.Errorf("unsupported legacy action kind %q", kind)
	}
}

func legacyActionKind(actionType string) (string, bool) {
	switch actionType {
	case "navigate":
		return "navigate", true
	case "external_link":
		return "external", true
	case "director_handoff":
		return "handoff", true
	default:
		return "", false
	}
}

func validateLegacyHref(kind, href string) error {
	switch kind {
	case "navigate":
		if !IsSafeInternalHref(href) {
			return fmt.Errorf("navigate action has unsafe href %q", href)
		}
	case "external":
		if !IsSafeExternalHref(href) {
			return fmt.Errorf("external action has unsafe href %q", href)
		}
	case "handoff":
		if href != "" && !IsSafeInternalHref(href) {
			return fmt.Errorf("handoff action has unsafe href %q", href)
		}
	}
	return nil
}

func copyWorkflow(workflow *Workflow) *Workflow {
	if workflow == nil {
		return nil
	}
	copy := *workflow
	return &copy
}
