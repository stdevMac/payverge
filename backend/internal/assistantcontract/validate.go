package assistantcontract

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	maxAnswerRunes         = 12000
	maxSections            = 5
	maxActions             = 8
	maxSources             = 12
	maxEntities            = 20
	maxFollowUps           = 5
	maxSteps               = 8
	maxNotices             = 8
	maxIDRunes             = 200
	maxTypeRunes           = 100
	maxTitleRunes          = 300
	maxSectionAnswerRunes  = 4000
	maxStepRunes           = 1000
	maxLabelRunes          = 300
	maxHrefRunes           = 2000
	maxNoticeRunes         = 2000
	maxTimestampRunes      = 100
	maxSourceTitleRunes    = 500
	maxEntityNameRunes     = 300
	maxFollowUpLabelRunes  = 300
	maxFollowUpPromptRunes = 1000
	maxCartNotesRunes      = 200
)

var supportedActionTypes = map[string]struct{}{
	"navigate":               {},
	"external_link":          {},
	"director_handoff":       {},
	"add_cart_item":          {},
	"capture_lead":           {},
	"create_support_request": {},
}

var allowedActionTargetKinds = map[string]map[string]struct{}{
	"navigate": {
		"dashboard_area": {},
		"payverge_page":  {},
	},
	"external_link": {
		"external_url": {},
	},
	"director_handoff": {
		"director": {},
	},
	"add_cart_item": {
		"menu_item": {},
		"bundle":    {},
	},
	"capture_lead": {
		"lead_form": {},
	},
	"create_support_request": {
		"support_request": {},
	},
}

var supportedSourceTypes = map[string]struct{}{
	"payverge_page":    {},
	"product_registry": {},
	"pricing_registry": {},
	"knowledge_entry":  {},
	"dashboard_guide":  {},
	"business_state":   {},
	"menu_item":        {},
	"bundle":           {},
	"offer":            {},
	"order_state":      {},
	"support_record":   {},
}

func Validate(response Response) error {
	if response.Version != 2 {
		return fmt.Errorf("version must be 2")
	}
	if blank(response.ResponseID) {
		return fmt.Errorf("response_id is required")
	}
	if err := boundedText("response_id", response.ResponseID, maxIDRunes); err != nil {
		return err
	}
	if !validStatus(response.Status) {
		return fmt.Errorf("unsupported status %q", response.Status)
	}
	if response.Answer.Format != FormatMarkdown && response.Answer.Format != FormatPlainText {
		return fmt.Errorf("unsupported answer format %q", response.Answer.Format)
	}
	if utf8.RuneCountInString(response.Answer.Content) > maxAnswerRunes {
		return fmt.Errorf("answer content exceeds %d runes", maxAnswerRunes)
	}
	if err := validateCollectionsPresent(response); err != nil {
		return err
	}
	if err := validateCollectionLimits(response); err != nil {
		return err
	}

	sectionIDs, err := identitySet("section", sectionIdentityValues(response.Sections))
	if err != nil {
		return err
	}
	_ = sectionIDs
	actionIDs, err := identitySet("action", actionIdentityValues(response.Actions))
	if err != nil {
		return err
	}
	sourceIDs, err := identitySet("source", sourceIdentityValues(response.Sources))
	if err != nil {
		return err
	}
	entityIDs, err := identitySet("entity", entityIdentityValues(response.Entities))
	if err != nil {
		return err
	}
	if _, err := identitySet("follow-up", followUpIdentityValues(response.FollowUps)); err != nil {
		return err
	}
	if _, err := identitySet("notice", noticeIdentityValues(response.Notices)); err != nil {
		return err
	}

	if err := validateSections(response.Sections, actionIDs, sourceIDs, entityIDs); err != nil {
		return err
	}
	if err := validateSteps("steps", response.Steps); err != nil {
		return err
	}
	for _, action := range response.Actions {
		if err := validateAction(action); err != nil {
			return err
		}
	}
	for _, source := range response.Sources {
		if err := validateSource(source); err != nil {
			return err
		}
	}
	for _, entity := range response.Entities {
		if !isSupportedEntityType(entity.Type) {
			return fmt.Errorf("unsupported entity type %s", entity.Type)
		}
		if blank(entity.SourceID) {
			return fmt.Errorf("entity %s source_id is required", entity.ID)
		}
		if _, exists := sourceIDs[entity.SourceID]; !exists {
			return fmt.Errorf("entity %s references unknown source %s", entity.ID, entity.SourceID)
		}
		if err := boundedText("entity display_name", entity.DisplayName, maxEntityNameRunes); err != nil {
			return err
		}
		if err := boundedText("entity availability", entity.Availability, maxTypeRunes); err != nil {
			return err
		}
	}
	for _, followUp := range response.FollowUps {
		if err := boundedText("follow-up label", followUp.Label, maxFollowUpLabelRunes); err != nil {
			return err
		}
		if err := boundedText("follow-up prompt", followUp.Prompt, maxFollowUpPromptRunes); err != nil {
			return err
		}
	}
	for _, notice := range response.Notices {
		if err := boundedText("notice kind", notice.Kind, maxTypeRunes); err != nil {
			return err
		}
		if err := boundedText("notice message", notice.Message, maxNoticeRunes); err != nil {
			return err
		}
	}
	if response.Workflow != nil {
		if blank(response.Workflow.ID) {
			return fmt.Errorf("workflow id is required")
		}
		if err := boundedText("workflow id", response.Workflow.ID, maxIDRunes); err != nil {
			return err
		}
		if response.Workflow.StepTotal <= 0 || response.Workflow.StepIndex < 0 || response.Workflow.StepIndex >= response.Workflow.StepTotal {
			return fmt.Errorf("workflow requires 0 <= step_index < step_total with positive step_total")
		}
	}
	return nil
}

func IsSafeExternalHref(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil
}

func IsSafeInternalHref(raw string) bool {
	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.ContainsRune(raw, '\\') {
		return false
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] <= 0x1f || raw[i] == 0x7f {
			return false
		}
	}
	return true
}

func validateCollectionsPresent(response Response) error {
	collections := []struct {
		name string
		nil  bool
	}{
		{"sections", response.Sections == nil},
		{"steps", response.Steps == nil},
		{"actions", response.Actions == nil},
		{"sources", response.Sources == nil},
		{"entities", response.Entities == nil},
		{"follow_ups", response.FollowUps == nil},
		{"notices", response.Notices == nil},
	}
	for _, collection := range collections {
		if collection.nil {
			return fmt.Errorf("%s must be an array", collection.name)
		}
	}
	return nil
}

func validateCollectionLimits(response Response) error {
	limits := []struct {
		name   string
		length int
		limit  int
	}{
		{"sections", len(response.Sections), maxSections},
		{"actions", len(response.Actions), maxActions},
		{"sources", len(response.Sources), maxSources},
		{"entities", len(response.Entities), maxEntities},
		{"follow_ups", len(response.FollowUps), maxFollowUps},
		{"steps", len(response.Steps), maxSteps},
		{"notices", len(response.Notices), maxNotices},
	}
	for _, item := range limits {
		if item.length > item.limit {
			return fmt.Errorf("%s exceeds limit %d", item.name, item.limit)
		}
	}
	return nil
}

func validateSections(sections []Section, actions, sources, entities map[string]struct{}) error {
	for _, section := range sections {
		if err := boundedText("section title", section.Title, maxTitleRunes); err != nil {
			return err
		}
		if err := boundedText("section answer", section.Answer, maxSectionAnswerRunes); err != nil {
			return err
		}
		if len(section.Steps) > maxSteps {
			return fmt.Errorf("section %s steps exceeds limit %d", section.ID, maxSteps)
		}
		if len(section.ActionIDs) > maxActions {
			return fmt.Errorf("section %s action_ids exceeds limit %d", section.ID, maxActions)
		}
		if len(section.SourceIDs) > maxSources {
			return fmt.Errorf("section %s source_ids exceeds limit %d", section.ID, maxSources)
		}
		if len(section.EntityIDs) > maxEntities {
			return fmt.Errorf("section %s entity_ids exceeds limit %d", section.ID, maxEntities)
		}
		if err := validateSteps("section steps", section.Steps); err != nil {
			return err
		}
		for _, id := range section.ActionIDs {
			if _, exists := actions[id]; !exists {
				return fmt.Errorf("section %s references unknown action %s", section.ID, id)
			}
		}
		for _, id := range section.SourceIDs {
			if _, exists := sources[id]; !exists {
				return fmt.Errorf("section %s references unknown source %s", section.ID, id)
			}
		}
		for _, id := range section.EntityIDs {
			if _, exists := entities[id]; !exists {
				return fmt.Errorf("section %s references unknown entity %s", section.ID, id)
			}
		}
		requiredArrays := []struct {
			name string
			nil  bool
		}{
			{"steps", section.Steps == nil},
			{"action_ids", section.ActionIDs == nil},
			{"source_ids", section.SourceIDs == nil},
			{"entity_ids", section.EntityIDs == nil},
		}
		for _, array := range requiredArrays {
			if array.nil {
				return fmt.Errorf("section %s %s must be an array", section.ID, array.name)
			}
		}
	}
	return nil
}

func validateAction(action Action) error {
	if _, supported := supportedActionTypes[action.Type]; !supported {
		return fmt.Errorf("unsupported action type %s", action.Type)
	}
	if blank(action.Target.Kind) {
		return fmt.Errorf("action %s target kind is required", action.ID)
	}
	if err := boundedText("action target kind", action.Target.Kind, maxTypeRunes); err != nil {
		return err
	}
	if _, allowed := allowedActionTargetKinds[action.Type][action.Target.Kind]; !allowed {
		return fmt.Errorf("action %s type %s cannot target kind %s", action.ID, action.Type, action.Target.Kind)
	}
	if blank(action.Target.ID) {
		return fmt.Errorf("action %s target id is required", action.ID)
	}
	if err := boundedText("action target id", action.Target.ID, maxIDRunes); err != nil {
		return err
	}
	if err := boundedText("action label", action.Label, maxLabelRunes); err != nil {
		return err
	}
	if err := boundedText("action state", action.State, maxTypeRunes); err != nil {
		return err
	}
	if err := boundedText("action confirmation", action.Confirmation, maxTypeRunes); err != nil {
		return err
	}
	if action.DisabledReason != nil {
		if err := boundedText("action disabled_reason", *action.DisabledReason, maxNoticeRunes); err != nil {
			return err
		}
	}
	if action.ExpiresAt != nil {
		if err := boundedText("action expires_at", *action.ExpiresAt, maxTimestampRunes); err != nil {
			return err
		}
	}
	if utf8.RuneCountInString(action.Target.Href) > maxHrefRunes {
		return fmt.Errorf("action %s href exceeds %d runes", action.ID, maxHrefRunes)
	}
	if action.Type == "add_cart_item" {
		if action.Target.Quantity == nil {
			return fmt.Errorf("add_cart_item action %s quantity is required", action.ID)
		}
		if *action.Target.Quantity < 1 || *action.Target.Quantity > 20 {
			return fmt.Errorf("add_cart_item action %s quantity must be between 1 and 20", action.ID)
		}
		if action.Target.Notes != nil {
			if utf8.RuneCountInString(*action.Target.Notes) > maxCartNotesRunes {
				return fmt.Errorf("add_cart_item action %s notes exceeds %d runes", action.ID, maxCartNotesRunes)
			}
			if containsASCIIControl(*action.Target.Notes) {
				return fmt.Errorf("add_cart_item action %s notes contains control characters", action.ID)
			}
		}
	} else if action.Target.Quantity != nil || action.Target.Notes != nil {
		return fmt.Errorf("action %s type %s cannot include quantity or notes", action.ID, action.Type)
	}
	switch action.Type {
	case "external_link":
		if !IsSafeExternalHref(action.Target.Href) {
			return fmt.Errorf("external_link action %s requires a safe external href", action.ID)
		}
	case "navigate":
		if !IsSafeInternalHref(action.Target.Href) {
			return fmt.Errorf("navigate action %s requires a safe internal href", action.ID)
		}
	default:
		if action.Target.Href != "" && !IsSafeInternalHref(action.Target.Href) {
			return fmt.Errorf("action %s cannot use an external href", action.ID)
		}
	}
	return nil
}

func containsASCIIControl(value string) bool {
	for _, r := range value {
		if r <= 0x1f || r == 0x7f {
			return true
		}
	}
	return false
}

func validateSource(source Source) error {
	if _, supported := supportedSourceTypes[source.Type]; !supported {
		return fmt.Errorf("unsupported source type %s", source.Type)
	}
	if err := boundedText("source title", source.Title, maxSourceTitleRunes); err != nil {
		return err
	}
	if err := boundedText("source origin", source.Origin, maxTypeRunes); err != nil {
		return err
	}
	if err := boundedText("source retrieved_at", source.RetrievedAt, maxTimestampRunes); err != nil {
		return err
	}
	if source.Href != nil {
		if utf8.RuneCountInString(*source.Href) > maxHrefRunes || (!IsSafeInternalHref(*source.Href) && !IsSafeExternalHref(*source.Href)) {
			return fmt.Errorf("source %s has unsafe href", source.ID)
		}
	}
	return nil
}

func validateSteps(name string, steps []string) error {
	for _, step := range steps {
		if err := boundedText(name, step, maxStepRunes); err != nil {
			return err
		}
	}
	return nil
}

func identitySet(kind string, values []string) (map[string]struct{}, error) {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		if blank(value) {
			return nil, fmt.Errorf("%s id is required", kind)
		}
		if _, exists := result[value]; exists {
			return nil, fmt.Errorf("duplicate %s id %s", kind, value)
		}
		if err := boundedText(kind+" id", value, maxIDRunes); err != nil {
			return nil, err
		}
		result[value] = struct{}{}
	}
	return result, nil
}

func validStatus(status Status) bool {
	switch status {
	case StatusComplete, StatusNeedsClarification, StatusBlocked, StatusDegraded:
		return true
	default:
		return false
	}
}

func blank(value string) bool { return strings.TrimSpace(value) == "" }

func boundedText(name, value string, limit int) error {
	if utf8.RuneCountInString(value) > limit {
		return fmt.Errorf("%s exceeds %d runes", name, limit)
	}
	return nil
}

func sectionIdentityValues(values []Section) []string {
	ids := make([]string, len(values))
	for i := range values {
		ids[i] = values[i].ID
	}
	return ids
}

func actionIdentityValues(values []Action) []string {
	ids := make([]string, len(values))
	for i := range values {
		ids[i] = values[i].ID
	}
	return ids
}

func sourceIdentityValues(values []Source) []string {
	ids := make([]string, len(values))
	for i := range values {
		ids[i] = values[i].ID
	}
	return ids
}

func entityIdentityValues(values []Entity) []string {
	ids := make([]string, len(values))
	for i := range values {
		ids[i] = values[i].ID
	}
	return ids
}

func followUpIdentityValues(values []FollowUp) []string {
	ids := make([]string, len(values))
	for i := range values {
		ids[i] = values[i].ID
	}
	return ids
}

func noticeIdentityValues(values []Notice) []string {
	ids := make([]string, len(values))
	for i := range values {
		ids[i] = values[i].ID
	}
	return ids
}

func actionTypes() []string {
	return []string{"navigate", "external_link", "director_handoff", "add_cart_item", "capture_lead", "create_support_request"}
}

func actionTargetKinds() []string {
	unique := make(map[string]struct{})
	for _, kinds := range allowedActionTargetKinds {
		for kind := range kinds {
			unique[kind] = struct{}{}
		}
	}
	kinds := make([]string, 0, len(unique))
	for kind := range unique {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds
}

func sourceTypes() []string {
	return []string{"payverge_page", "product_registry", "pricing_registry", "knowledge_entry", "dashboard_guide", "business_state", "menu_item", "bundle", "offer", "order_state", "support_record"}
}

func entityTypes() []string {
	return []string{"menu_item", "bundle", "offer"}
}

func isSupportedEntityType(value string) bool {
	for _, entityType := range entityTypes() {
		if value == entityType {
			return true
		}
	}
	return false
}
