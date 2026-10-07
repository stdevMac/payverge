package assistantcontract

import (
	"sort"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

func Schema() *llm.JSONSchema {
	return strictObject(map[string]*llm.JSONSchema{
		"version":     {Type: llm.TypeInteger, Const: 2},
		"response_id": boundedString(maxIDRunes),
		"answer": strictObject(map[string]*llm.JSONSchema{
			"format":  {Type: llm.TypeString, Enum: []string{string(FormatMarkdown), string(FormatPlainText)}},
			"content": boundedString(maxAnswerRunes),
		}),
		"sections": boundedArray(strictObject(map[string]*llm.JSONSchema{
			"id":         boundedString(maxIDRunes),
			"title":      boundedString(maxTitleRunes),
			"answer":     boundedString(maxSectionAnswerRunes),
			"steps":      boundedArray(boundedString(maxStepRunes), maxSteps),
			"action_ids": boundedArray(boundedString(maxIDRunes), maxActions),
			"source_ids": boundedArray(boundedString(maxIDRunes), maxSources),
			"entity_ids": boundedArray(boundedString(maxIDRunes), maxEntities),
		}), maxSections),
		"steps": boundedArray(boundedString(maxStepRunes), maxSteps),
		"actions": boundedArray(strictObject(map[string]*llm.JSONSchema{
			"id":    boundedString(maxIDRunes),
			"type":  {Type: llm.TypeString, Enum: actionTypes()},
			"label": boundedString(maxLabelRunes),
			"target": strictObjectWithOptional(map[string]*llm.JSONSchema{
				"kind":     boundedEnumString(maxTypeRunes, actionTargetKinds()),
				"id":       boundedString(maxIDRunes),
				"href":     boundedString(maxHrefRunes),
				"quantity": {Type: llm.TypeInteger},
				"notes":    boundedString(maxCartNotesRunes),
			}, "quantity", "notes"),
			"state":           boundedString(maxTypeRunes),
			"confirmation":    boundedString(maxTypeRunes),
			"disabled_reason": nullable(boundedString(maxNoticeRunes)),
			"expires_at":      nullable(boundedString(maxTimestampRunes)),
		}), maxActions),
		"sources": boundedArray(strictObject(map[string]*llm.JSONSchema{
			"id":           boundedString(maxIDRunes),
			"type":         {Type: llm.TypeString, Enum: sourceTypes()},
			"title":        boundedString(maxSourceTitleRunes),
			"href":         nullable(boundedString(maxHrefRunes)),
			"origin":       boundedString(maxTypeRunes),
			"retrieved_at": boundedString(maxTimestampRunes),
		}), maxSources),
		"entities": boundedArray(strictObject(map[string]*llm.JSONSchema{
			"id":           boundedString(maxIDRunes),
			"type":         {Type: llm.TypeString, Enum: entityTypes()},
			"display_name": boundedString(maxEntityNameRunes),
			"availability": boundedString(maxTypeRunes),
			"source_id":    boundedString(maxIDRunes),
		}), maxEntities),
		"follow_ups": boundedArray(strictObject(map[string]*llm.JSONSchema{
			"id":     boundedString(maxIDRunes),
			"label":  boundedString(maxFollowUpLabelRunes),
			"prompt": boundedString(maxFollowUpPromptRunes),
		}), maxFollowUps),
		"workflow": nullable(strictObject(map[string]*llm.JSONSchema{
			"id":         boundedString(maxIDRunes),
			"step_index": {Type: llm.TypeInteger},
			"step_total": {Type: llm.TypeInteger},
		})),
		"notices": boundedArray(strictObject(map[string]*llm.JSONSchema{
			"id":      boundedString(maxIDRunes),
			"kind":    boundedString(maxTypeRunes),
			"message": boundedString(maxNoticeRunes),
		}), maxNotices),
		"status": {Type: llm.TypeString, Enum: []string{
			string(StatusComplete), string(StatusNeedsClarification), string(StatusBlocked), string(StatusDegraded),
		}},
	})
}

func strictObject(properties map[string]*llm.JSONSchema) *llm.JSONSchema {
	return strictObjectWithOptional(properties)
}

func strictObjectWithOptional(properties map[string]*llm.JSONSchema, optional ...string) *llm.JSONSchema {
	additionalProperties := false
	optionalSet := make(map[string]struct{}, len(optional))
	for _, name := range optional {
		optionalSet[name] = struct{}{}
	}
	required := make([]string, 0, len(properties)-len(optionalSet))
	for name := range properties {
		if _, isOptional := optionalSet[name]; isOptional {
			continue
		}
		required = append(required, name)
	}
	sort.Strings(required)
	return &llm.JSONSchema{
		Type:                 llm.TypeObject,
		Properties:           properties,
		Required:             required,
		AdditionalProperties: &additionalProperties,
	}
}

func boundedArray(items *llm.JSONSchema, limit int) *llm.JSONSchema {
	return &llm.JSONSchema{Type: llm.TypeArray, Items: items, MaxItems: intPointer(limit)}
}

func boundedString(limit int) *llm.JSONSchema {
	return &llm.JSONSchema{Type: llm.TypeString, MaxLength: intPointer(limit)}
}

func boundedEnumString(limit int, values []string) *llm.JSONSchema {
	return &llm.JSONSchema{Type: llm.TypeString, Enum: values, MaxLength: intPointer(limit)}
}

func nullable(schema *llm.JSONSchema) *llm.JSONSchema {
	return &llm.JSONSchema{AnyOf: []*llm.JSONSchema{schema, {Type: llm.TypeNull}}}
}

func intPointer(value int) *int { return &value }
