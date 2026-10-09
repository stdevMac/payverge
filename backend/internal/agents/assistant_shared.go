package agents

// Helpers shared by the ops assistant: finalizer validation, telemetry and
// structured/legacy response conversion.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
	"github.com/stdevmac/payverge/backend/internal/llm"
)

// assistantFinalizerValidation is bounded validation provenance produced only
// at trusted finalizer boundaries. It deliberately carries no content, IDs,
// URLs, tool arguments, or other high-cardinality values.
type assistantFinalizerValidation struct {
	ActionsDropped  bool
	SourcesDropped  bool
	EntitiesDropped bool
}

func modelHasActionCandidates(model StructuredResponse) bool {
	if len(model.Actions) > 0 {
		return true
	}
	values := make([]string, 0, 1+len(model.Steps)+len(model.FollowUps))
	values = append(values, model.Answer)
	values = append(values, model.Steps...)
	values = append(values, model.FollowUps...)
	for _, value := range values {
		if _, changed := sanitizeAssistantModelText(value); changed {
			return true
		}
	}
	return false
}

var assistantMarkdownLink = regexp.MustCompile(`\[([^\]\r\n]+)\]\(([^)\r\n]+)\)`)

var assistantRawDestination = regexp.MustCompile(`(?i)(?:https?://|javascript:)[^\s]+|/business/[0-9]+(?:/[^\s]+|\?[^\s]+)?`)

var assistantRepeatedHorizontalWhitespace = regexp.MustCompile(`[\t ]{2,}`)

func sanitizeAssistantModelText(value string) (string, bool) {
	changed := false
	result := assistantMarkdownLink.ReplaceAllStringFunc(value, func(link string) string {
		parts := assistantMarkdownLink.FindStringSubmatch(link)
		if len(parts) != 3 {
			return link
		}
		changed = true
		return parts[1]
	})
	result = assistantRawDestination.ReplaceAllStringFunc(result, func(candidate string) string {
		changed = true
		trimmed := strings.TrimRight(candidate, ".,;!?")
		return strings.TrimPrefix(candidate, trimmed)
	})
	if changed {
		result = assistantRepeatedHorizontalWhitespace.ReplaceAllString(result, " ")
		result = strings.TrimSpace(result)
	}
	return result, changed
}

func decodeAssistantStoredJSON(raw string, target any, strict bool) error {
	decoder := json.NewDecoder(strings.NewReader(raw))
	if strict {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return nil
}

func normalizeStructuredResponseArrays(response *StructuredResponse) {
	if response.Steps == nil {
		response.Steps = []string{}
	}
	if response.Actions == nil {
		response.Actions = []ActionLink{}
	}
	if response.FollowUps == nil {
		response.FollowUps = []string{}
	}
}

func legacyResponseFromStructured(response StructuredResponse) assistantcontract.LegacyResponse {
	actions := make([]assistantcontract.LegacyAction, 0, len(response.Actions))
	for _, action := range response.Actions {
		actions = append(actions, assistantcontract.LegacyAction{
			Label: action.Label, Href: action.Href, Target: action.Target, Kind: action.Kind,
			Disabled: action.Disabled, DisabledReason: action.DisabledReason,
		})
	}
	var workflow *assistantcontract.Workflow
	if response.Workflow != nil {
		workflow = &assistantcontract.Workflow{
			ID: response.Workflow.ID, StepIndex: response.Workflow.StepIndex, StepTotal: response.Workflow.StepTotal,
		}
	}
	return assistantcontract.LegacyResponse{
		Answer: response.Answer, Steps: append([]string{}, response.Steps...), Actions: actions,
		FollowUps: append([]string{}, response.FollowUps...), Workflow: workflow,
	}
}

func structuredResponseFromLegacy(response assistantcontract.LegacyResponse) StructuredResponse {
	actions := make([]ActionLink, 0, len(response.Actions))
	for _, action := range response.Actions {
		actions = append(actions, ActionLink{
			Label: action.Label, Href: action.Href, Target: action.Target, Kind: action.Kind,
			Disabled: action.Disabled, DisabledReason: action.DisabledReason,
		})
	}
	var workflow *WorkflowProgress
	if response.Workflow != nil {
		workflow = &WorkflowProgress{
			ID: response.Workflow.ID, StepIndex: response.Workflow.StepIndex, StepTotal: response.Workflow.StepTotal,
		}
	}
	return StructuredResponse{
		Answer: response.Answer, Steps: append([]string{}, response.Steps...), Actions: actions,
		FollowUps: append([]string{}, response.FollowUps...), Workflow: workflow,
	}
}

type Usage struct {
	Model       string `json:"model"`
	LatencyMs   int64  `json:"latency_ms"`
	TotalTokens int    `json:"total_tokens,omitempty"`
}

func boundedAssistantTelemetryMilliseconds(duration time.Duration) int64 {
	const maximum = int64((30 * time.Minute) / time.Millisecond)
	milliseconds := duration.Milliseconds()
	if milliseconds < 0 {
		return 0
	}
	if milliseconds > maximum {
		return maximum
	}
	return milliseconds
}

func applyAssistantResponseTelemetryWithValidation(event *llm.AITelemetryEvent, response assistantcontract.Response, languageOutcome string, validation assistantFinalizerValidation, firstContentElapsed ...time.Duration) {
	if event == nil {
		return
	}
	event.V2ShadowValid = false
	if err := assistantcontract.Validate(response); err != nil {
		event.SchemaOutcome = "dropped"
		event.Outcome = "invalid"
		return
	}
	event.V2ShadowValid = true
	event.SchemaOutcome = "verified"
	renderableText := assistantTelemetryRenderableText(response)
	event.LanguageOutcome = llm.ValidatedResponseLanguageOutcome(event.Language, renderableText, languageOutcome)
	if event.TimeToFirstMs == 0 && strings.TrimSpace(renderableText) != "" && len(firstContentElapsed) > 0 {
		event.TimeToFirstMs = boundedAssistantTelemetryFirstContentMilliseconds(firstContentElapsed[0])
	}
	if event.Outcome == "" {
		switch response.Status {
		case assistantcontract.StatusComplete:
			event.Outcome = "ok"
		case assistantcontract.StatusDegraded:
			event.Outcome = "fallback"
		case assistantcontract.StatusBlocked:
			event.Outcome = "blocked"
		case assistantcontract.StatusNeedsClarification:
			event.Outcome = "fallback"
		default:
			event.Outcome = "error"
		}
	}
	if validation.ActionsDropped {
		event.ActionOutcome = "rejected"
	} else if event.ActionOutcome == "none" && len(response.Actions) > 0 {
		event.ActionOutcome = "offered"
	}
	if validation.SourcesDropped {
		event.SourceOutcome = "dropped"
	} else if event.SourceOutcome == "none" && len(response.Sources) > 0 {
		event.SourceOutcome = "verified"
	}
	if validation.EntitiesDropped {
		event.EntityOutcome = "dropped"
	} else if event.EntityOutcome == "none" && len(response.Entities) > 0 {
		event.EntityOutcome = "verified"
	}
}

func assistantTelemetryRenderableText(response assistantcontract.Response) string {
	parts := []string{response.Answer.Content}
	parts = append(parts, response.Steps...)
	for _, section := range response.Sections {
		parts = append(parts, section.Title, section.Answer)
		parts = append(parts, section.Steps...)
	}
	return strings.Join(parts, "\n")
}

func boundedAssistantTelemetryFirstContentMilliseconds(duration time.Duration) int64 {
	milliseconds := boundedAssistantTelemetryMilliseconds(duration)
	if duration > 0 && milliseconds == 0 {
		return 1
	}
	return milliseconds
}

func assistantTelemetryLanguageOutcome(locale string) string {
	normalized := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(locale)), "_", "-")
	if normalized == "" || normalized == "en" || strings.HasPrefix(normalized, "en-") ||
		normalized == "es" || strings.HasPrefix(normalized, "es-") {
		// The response contract does not carry a finalizer-owned language
		// decision. A supported request locale is a bounded dimension, not proof
		// that the generated answer used that language.
		return "none"
	}
	return "dropped"
}
