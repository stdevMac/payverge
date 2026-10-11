package agents

import (
	"fmt"
	"strings"
)

// NormalizeOpsResponse repairs common model shape drift, extracts action
// metadata the model leaked into the answer string, blocks prompt-echo answers,
// normalizes action kinds, and dedupes actions by href.
func NormalizeOpsResponse(resp StructuredResponse, businessID uint, userMessage string) StructuredResponse {
	userMessage = strings.TrimSpace(userMessage)
	if userMessage != "" && strings.EqualFold(strings.TrimSpace(resp.Answer), userMessage) {
		resp.Answer = "I can help you find the right screen or steps in Payverge. What would you like to do?"
		resp.Steps = []string{}
	}

	// Pull any leaked action metadata out of the answer prose before rendering.
	cleanAnswer, leaked := extractLeakedActions(resp.Answer, businessID)
	resp.Answer = cleanAnswer

	// Normalize the model's own actions, then append the leaked ones. Dedupe by
	// href, keeping the first occurrence (model-declared actions win over the
	// recovered copies).
	merged := make([]ActionLink, 0, len(resp.Actions)+len(leaked))
	merged = append(merged, resp.Actions...)
	merged = append(merged, leaked...)

	out := make([]ActionLink, 0, len(merged))
	seen := make(map[string]bool, len(merged))
	for _, a := range merged {
		norm := normalizeActionLink(a, businessID)
		if norm.Href == "" || !isSafeHref(norm.Href) {
			continue // drop empty-href or unsafe-scheme actions
		}
		if seen[norm.Href] {
			continue // dedupe by href
		}
		seen[norm.Href] = true
		out = append(out, norm)
	}
	resp.Actions = out

	if resp.Steps == nil {
		resp.Steps = []string{}
	}
	if resp.FollowUps == nil {
		resp.FollowUps = []string{}
	}
	return resp
}

func normalizeActionLink(a ActionLink, businessID uint) ActionLink {
	href := strings.TrimSpace(a.Href)
	tab := strings.TrimSpace(a.Target)
	if href == "" && tab != "" {
		href = fmt.Sprintf("/business/%d/dashboard?tab=%s", businessID, tab)
	}
	kind := normalizeKind(a.Kind, href)
	label := strings.TrimSpace(a.Label)
	if label == "" && href != "" {
		label = "Open"
	}
	return ActionLink{
		Label:          label,
		Href:           href,
		Kind:           kind,
		Disabled:       a.Disabled,
		DisabledReason: a.DisabledReason,
	}
}
