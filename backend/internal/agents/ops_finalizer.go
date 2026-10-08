package agents

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/stdevmac/payverge/backend/internal/agents/ops_guides"
	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
)

const (
	maxOpsFinalizerSections = 5
	maxOpsStateSummaryRunes = 1200
)

// OpsAccessSnapshot contains only server-assembled authorization state. The
// finalizer never infers access from model output or tool evidence.
type OpsAccessSnapshot struct {
	EffectivePermissions map[string]bool
	HiddenGuideIDs       map[string]bool
	Suspended            bool
}

// OpsFinalizeInput combines untrusted model presentation with trusted guide,
// evidence, access, and workflow state. Executable output is rebuilt solely
// from the trusted fields.
type OpsFinalizeInput struct {
	ResponseID string
	Locale     string
	BusinessID uint
	Model      StructuredResponse
	Matches    []ops_guides.GuideMatch
	Evidence   []ToolEvidence
	Access     OpsAccessSnapshot
	Workflow   *WorkflowState
}

var trustedOpsStateToolGuides = map[string]map[string]struct{}{
	"get_business_context": {
		"ai-waiter-configure": {},
	},
	"get_setup_status": {
		"overview-get-started": {},
	},
	"get_plugin_status": {
		"plugins-connect": {},
	},
}

const (
	opsStateEnglishEffectVerb = `(?:created|updated|deleted|published|charged|refunded|closed|invited|enabled|disabled|applied|approved)`
	opsStateSpanishEffectNoun = `(?:publicación|actualización|eliminación|cobro|reembolso|cierre|invitación|activación|desactivación)`
)

var unsafeOpsStateClaimPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:^|[\s,;:.!?])i\s+(?:created|updated|deleted|published|charged|refunded|closed|invited|enabled|disabled|applied|approved)(?:$|[\s,;:.!?])`),
	regexp.MustCompile(`(?i)(?:^|[\s,;:.!?¿¡])ya\s+(?:creé|actualicé|eliminé|publiqué|cobré|reembolsé|cerré|invité|activé|desactivé|apliqué|aprobé)(?:$|[\s,;:.!?¿¡])`),
	regexp.MustCompile(`(?i)(?:^|[\s,;:.!?])(?:successfully|correctly)\s+` + opsStateEnglishEffectVerb + `(?:$|[\s,;:.!?])`),
	regexp.MustCompile(`(?i)(?:^|[\s,;:.!?])we\s+` + opsStateEnglishEffectVerb + `[^.!?;\n]{0,80}(?:successfully|correctly)(?:$|[\s,;:.!?])`),
	regexp.MustCompile(`(?i)(?:^|[\s,;:.!?])(?:was|were)\s+(?:created|updated|deleted|published|charged|refunded|closed|invited|enabled|disabled|applied|approved)\s+(?:successfully|correctly)(?:$|[\s,;:.!?])`),
	regexp.MustCompile(`(?i)(?:^|[\s,;:.!?])(?:created|updated|deleted|published|charged|refunded|closed|invited|enabled|disabled|applied|approved)\s+(?:successfully|correctly)(?:$|[\s,;:.!?])`),
	regexp.MustCompile(`(?i)(?:^|[\s,;:.!?¿¡])se\s+(?:creó|actualizó|eliminó|publicó|cobró|reembolsó|cerró|invitó|activó|desactivó|aplicó|aprobó)[^.!?;¿¡\n]{0,80}(?:correctamente|con éxito)(?:$|[\s,;:.!?¿¡])`),
	regexp.MustCompile(`(?i)(?:^|[\s,;:.!?¿¡])(?:fue|fueron)\s+(?:cread[oa]s?|actualizad[oa]s?|eliminad[oa]s?|publicad[oa]s?|cobrad[oa]s?|reembolsad[oa]s?|cerrad[oa]s?|invitad[oa]s?|activad[oa]s?|desactivad[oa]s?|aplicad[oa]s?|aprobad[oa]s?)\s+(?:correctamente|con éxito)(?:$|[\s,;:.!?¿¡])`),
	regexp.MustCompile(`(?i)(?:^|[\s,;:.!?¿¡])(?:cread[oa]s?|actualizad[oa]s?|eliminad[oa]s?|publicad[oa]s?|cobrad[oa]s?|reembolsad[oa]s?|cerrad[oa]s?|invitad[oa]s?|activad[oa]s?|desactivad[oa]s?|aplicad[oa]s?|aprobad[oa]s?)\s+(?:correctamente|con éxito)(?:$|[\s,;:.!?¿¡])`),
	regexp.MustCompile(`(?i)(?:^|[\s,;:.!?¿¡])(?:la|el)\s+` + opsStateSpanishEffectNoun + `\s+se\s+(?:completó|realizó|finalizó)\s+(?:correctamente|con éxito)(?:$|[\s,;:.!?¿¡])`),
}

// FinalizeOpsV2 constructs permission-aware, localized V2 guidance. Model
// actions, URLs, steps, follow-ups, workflow, and execution claims are ignored.
func FinalizeOpsV2(in OpsFinalizeInput) (assistantcontract.Response, error) {
	response, _, err := finalizeOpsV2WithValidation(in)
	return response, err
}

func finalizeOpsV2WithValidation(in OpsFinalizeInput) (assistantcontract.Response, assistantFinalizerValidation, error) {
	validation := assistantFinalizerValidation{ActionsDropped: modelHasActionCandidates(in.Model)}
	if strings.TrimSpace(in.ResponseID) == "" {
		return assistantcontract.Response{}, validation, fmt.Errorf("finalize ops v2: response_id is required")
	}
	if in.BusinessID == 0 {
		return assistantcontract.Response{}, validation, fmt.Errorf("finalize ops v2: business_id is required")
	}
	if len(in.Matches) == 0 {
		return assistantcontract.Response{}, validation, fmt.Errorf("finalize ops v2: at least one guide match is required")
	}

	locale := canonicalOpsLocale(in.Locale)
	catalog := ops_guides.NewDefaultCatalog()
	guides, hiddenCount, truncated := trustedVisibleOpsGuides(catalog, locale, in.Matches, in.Access)
	if hiddenCount > 0 || truncated || len(guides)+hiddenCount < len(in.Matches) {
		validation.ActionsDropped = true
		validation.SourcesDropped = true
	}
	if len(guides) == 0 && hiddenCount == 0 {
		return assistantcontract.Response{}, validation, fmt.Errorf("finalize ops v2: no trusted guide matches")
	}

	if len(guides) == 0 {
		response := assistantcontract.NewResponse(in.ResponseID, hiddenOpsGuidanceAnswer(locale))
		response.Status = assistantcontract.StatusBlocked
		if err := assistantcontract.Validate(response); err != nil {
			return assistantcontract.Response{}, validation, fmt.Errorf("finalize ops v2: %w", err)
		}
		return response, validation, nil
	}

	stateByGuide, stateDropped := trustedOpsStateSummariesWithValidation(in.Evidence, guides, in.Access)
	validation.SourcesDropped = validation.SourcesDropped || stateDropped
	retrievedAt := time.Now().UTC().Format(time.RFC3339)
	response := assistantcontract.NewResponse(in.ResponseID, "")
	aggregatedFollowUpIDs := make([]string, 0, maxOpsFinalizerSections)

	// #874: a lone guide used to be emitted twice — once as answer.content and
	// again as the single section carrying the same text — so the widget printed
	// the paragraph twice and ToLegacy flattened it into a duplicated V1 answer.
	// One guide is the whole answer; sections exist to separate several of them.
	singleGuide := len(guides) == 1

	for _, guide := range guides {
		answer := guide.Answer
		if state := stateByGuide[guide.ID]; state != "" {
			answer += "\n\n" + opsCurrentStatePrefix(locale) + state
		}

		actionID := "navigate:" + guide.ID
		sourceID := "guide:" + guide.ID
		href := guideDestinationHref(in.BusinessID, guide.Destination)
		if href == "" {
			return assistantcontract.Response{}, validation, fmt.Errorf("finalize ops v2: guide %s has no trusted destination", guide.ID)
		}
		action := trustedOpsNavigationAction(guide, href, locale, in.Access)
		response.Actions = append(response.Actions, action)
		response.Sources = append(response.Sources, assistantcontract.Source{
			ID: sourceID, Type: "dashboard_guide", Title: guide.FollowUpLabel,
			Origin: "ops_guide_catalog", RetrievedAt: retrievedAt,
		})
		if singleGuide {
			response.Answer.Content = answer
			response.Steps = append([]string{}, guide.Steps...)
		} else {
			response.Sections = append(response.Sections, assistantcontract.Section{
				ID: guide.ID, Title: guide.FollowUpLabel, Answer: answer,
				Steps:     append([]string{}, guide.Steps...),
				ActionIDs: []string{actionID}, SourceIDs: []string{sourceID}, EntityIDs: []string{},
			})
		}
		aggregatedFollowUpIDs = append(aggregatedFollowUpIDs, guide.FollowUpIDs...)
	}

	filteredFollowUpIDs := filterHiddenOpsFollowUps(aggregatedFollowUpIDs, in.Access.HiddenGuideIDs)
	followUps, err := catalog.ResolveFollowUps(locale, filteredFollowUpIDs)
	if err != nil {
		return assistantcontract.Response{}, validation, fmt.Errorf("finalize ops v2 follow-ups: %w", err)
	}
	if len(followUps) > maxOpsFinalizerSections {
		followUps = followUps[:maxOpsFinalizerSections]
		truncated = true
	}
	response.FollowUps = followUps
	response.Workflow = trustedOpsWorkflow(in.Workflow, guides)
	if !singleGuide {
		response.Answer.Content = multiGuideOpsAnswer(locale, len(response.Sections))
	}
	if truncated {
		response.Status = assistantcontract.StatusDegraded
	}

	if err := assistantcontract.Validate(response); err != nil {
		return assistantcontract.Response{}, validation, fmt.Errorf("finalize ops v2: %w", err)
	}
	return response, validation, nil
}

func trustedVisibleOpsGuides(catalog *ops_guides.Catalog, locale string, matches []ops_guides.GuideMatch, access OpsAccessSnapshot) ([]ops_guides.Guide, int, bool) {
	guides := make([]ops_guides.Guide, 0, min(len(matches), maxOpsFinalizerSections))
	seen := make(map[string]struct{}, len(matches))
	hiddenCount := 0
	truncated := false
	for _, match := range matches {
		id := strings.TrimSpace(match.Guide.ID)
		guide, ok := catalog.Get(locale, id)
		if !ok {
			continue
		}
		if _, duplicate := seen[guide.ID]; duplicate {
			continue
		}
		seen[guide.ID] = struct{}{}
		if access.HiddenGuideIDs[guide.ID] {
			hiddenCount++
			continue
		}
		if len(guides) == maxOpsFinalizerSections {
			truncated = true
			continue
		}
		guides = append(guides, guide)
	}
	return guides, hiddenCount, truncated
}

func trustedOpsNavigationAction(guide ops_guides.Guide, href, locale string, access OpsAccessSnapshot) assistantcontract.Action {
	action := assistantcontract.Action{
		ID: "navigate:" + guide.ID, Type: "navigate", Label: guide.DestinationLabel,
		Target: assistantcontract.ActionTarget{Kind: "dashboard_area", ID: guide.Tab, Href: href},
		State:  "ready", Confirmation: "none",
	}
	var reason string
	switch {
	case !access.EffectivePermissions[guide.RequiredPermission]:
		reason = opsPermissionDeniedReason(locale)
	case access.Suspended:
		reason = opsSuspendedReason(locale)
	}
	if reason != "" {
		action.State = "disabled"
		action.DisabledReason = &reason
	}
	return action
}

func trustedOpsStateSummariesWithValidation(evidence []ToolEvidence, guides []ops_guides.Guide, access OpsAccessSnapshot) (map[string]string, bool) {
	selected := make(map[string]struct{}, len(guides))
	for _, guide := range guides {
		if opsGuideFullyAccessible(guide, access) {
			selected[guide.ID] = struct{}{}
		}
	}
	result := make(map[string]string, len(guides))
	dropped := false
	for _, item := range evidence {
		candidate := opsEvidenceHasStateCandidate(item)
		ownedGuides, knownTool := trustedOpsStateToolGuides[item.Name]
		if !knownTool || item.Data == nil {
			dropped = dropped || candidate
			continue
		}
		guideID, guideOK := item.Data["guide_id"].(string)
		summary, summaryOK := item.Data["state_summary"].(string)
		guideID = strings.TrimSpace(guideID)
		summary = strings.TrimSpace(summary)
		if !guideOK || !summaryOK || guideID == "" || summary == "" {
			dropped = dropped || candidate
			continue
		}
		if _, ok := selected[guideID]; !ok || result[guideID] != "" {
			dropped = true
			continue
		}
		if _, owned := ownedGuides[guideID]; !owned {
			dropped = true
			continue
		}
		if utf8.RuneCountInString(summary) > maxOpsStateSummaryRunes || !safeOpsStateSummary(summary) {
			dropped = true
			continue
		}
		result[guideID] = summary
	}
	return result, dropped
}

func opsEvidenceHasStateCandidate(evidence ToolEvidence) bool {
	if evidence.Data == nil {
		return false
	}
	_, hasGuide := evidence.Data["guide_id"]
	_, hasSummary := evidence.Data["state_summary"]
	return hasGuide || hasSummary
}

func opsGuideFullyAccessible(guide ops_guides.Guide, access OpsAccessSnapshot) bool {
	return !access.HiddenGuideIDs[guide.ID] &&
		access.EffectivePermissions[guide.RequiredPermission] &&
		!access.Suspended
}

func safeOpsStateSummary(summary string) bool {
	lower := strings.ToLower(summary)
	for _, unsafe := range []string{
		"http://", "https://", "javascript:", "<script", "permission granted",
		"permission denied", "access granted", "access denied", "permiso concedido",
		"permiso denegado", "acceso concedido", "acceso denegado", "plan unlocked",
		"plan habilitado",
	} {
		if strings.Contains(lower, unsafe) {
			return false
		}
	}
	for _, pattern := range unsafeOpsStateClaimPatterns {
		if pattern.MatchString(summary) {
			return false
		}
	}
	return true
}

func trustedOpsWorkflow(state *WorkflowState, guides []ops_guides.Guide) *assistantcontract.Workflow {
	if state == nil || strings.TrimSpace(state.ID) == "" || state.StepTotal <= 0 || state.StepIndex < 0 || state.StepIndex >= state.StepTotal {
		return nil
	}
	for _, guide := range guides {
		if guide.WorkflowID == state.ID {
			return &assistantcontract.Workflow{ID: state.ID, StepIndex: state.StepIndex, StepTotal: state.StepTotal}
		}
	}
	return nil
}

func filterHiddenOpsFollowUps(ids []string, hidden map[string]bool) []string {
	filtered := make([]string, 0, len(ids))
	for _, id := range ids {
		if !hidden[id] {
			filtered = append(filtered, id)
		}
	}
	return filtered
}

func canonicalOpsLocale(locale string) string {
	normalized := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(locale)), "_", "-")
	if normalized == "es-ar" || strings.HasPrefix(normalized, "es-ar-") {
		return "es-AR"
	}
	if normalized == "es" || strings.HasPrefix(normalized, "es-") {
		return "es"
	}
	return "en"
}

func multiGuideOpsAnswer(locale string, count int) string {
	if strings.HasPrefix(locale, "es") {
		return fmt.Sprintf("Encontré %d guías operativas verificadas para tu consulta.", count)
	}
	return fmt.Sprintf("I found %d verified operational guides for your request.", count)
}

func hiddenOpsGuidanceAnswer(locale string) string {
	if strings.HasPrefix(locale, "es") {
		return "No puedo mostrar guías para el área solicitada con tu acceso actual."
	}
	return "I cannot show guidance for the requested area with your current access."
}

func opsCurrentStatePrefix(locale string) string {
	if strings.HasPrefix(locale, "es") {
		return "Estado actual: "
	}
	return "Current state: "
}

func opsPermissionDeniedReason(locale string) string {
	switch locale {
	case "es-AR":
		return "No tenés permiso para abrir esta área."
	case "es":
		return "No tienes permiso para abrir esta área."
	default:
		return "You do not have permission to open this area."
	}
}

func opsSuspendedReason(locale string) string {
	if locale == "es-AR" {
		return "La cuenta está suspendida; contactá al administrador del servidor para restaurar el acceso."
	}
	if locale == "es" {
		return "La cuenta está suspendida; contacta al administrador del servidor para restaurar el acceso."
	}
	return "The account is suspended; contact the server administrator to restore access."
}
