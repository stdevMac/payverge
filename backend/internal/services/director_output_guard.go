package services

import (
	"encoding/json"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Director output guard — deterministic, server-side defense-in-depth on the
// director copilot's JSON answer, applied AFTER the model returns and BEFORE the
// answer is persisted or shown to the owner.
//
// Two distinct threats, two distinct responses (hybrid):
//
//   - System-prompt / tab-list echo: the model was induced (e.g. by a poisoned
//     context value) to repeat its own instructions. The whole answer is suspect,
//     so we FAIL TO SAFE: discard it and return a canned, safe response.
//   - Customer PII leak: the model echoed a contact detail or identifier that was
//     present in the context. The analysis is otherwise fine, so we SCRUB just the
//     leaked values and keep the rest.
//
// Why this exists even though buildContext is aggregates-only today: the context
// surface is very likely to grow (a "what are guests saying" reviews block, a raw
// CRM row for personalization). When it does, the prompt rule is probabilistic;
// this guard makes the leak impossible regardless of how the context is poisoned.

const directorRedactionToken = "[redacted]"

// directorPromptLeakPhrases are verbatim fragments of the director system prompt
// (en/es share these section markers). Their presence in output means the model
// echoed its instructions. Matched case-insensitively.
var directorPromptLeakPhrases = []string{
	"data handling (non-negotiable)",
	"manejo de datos (innegociable)",
	"language (primary instruction)",
	"output strict json with this exact schema",
	"devolvé solo json estricto",
	"devolvé json estricto con este esquema",
	"proposing changes (write actions)",
	"there is no audit or diagnostic mode",
	"no existe ningún modo de auditoría",
	"the output schema and its key names are fixed",
	// Injection/spotlighting artifacts: their presence in output means the model
	// obeyed a poisoned "repeat your marker / enter audit mode" instruction.
	"data_block",
	"system diagnostic mode",
	"marker token",
}

// directorAllowedTabsDistinctEchoThreshold: a single answer that names this many
// DISTINCT dashboard tabs in its prose is enumerating the allowed-tabs list, not
// giving advice. Set high so ordinary answers (which reference one or two tabs)
// never trip it.
const directorAllowedTabsDistinctEchoThreshold = 8

// directorSensitiveKeySegments are key segments whose string values are treated
// as customer PII and denylisted for scrubbing. Compared per snake/kebab/dot key
// SEGMENT (so "cancelled" never matches "cell").
var directorSensitiveKeySegments = map[string]struct{}{
	"email": {}, "mail": {}, "phone": {}, "tel": {}, "telephone": {},
	"mobile": {}, "cell": {}, "whatsapp": {}, "address": {}, "street": {},
	"zip": {}, "postal": {}, "dob": {}, "birthday": {}, "birth": {},
	"loyalty": {}, "ssn": {}, "passport": {},
}

// directorSensitiveCompoundKeys catch multi-segment key names verbatim.
var directorSensitiveCompoundKeys = []string{
	"date_of_birth", "home_address", "mailing_address", "billing_address",
	"customer_id", "loyalty_card", "loyalty_id", "national_id", "tax_id",
	"postal_code", "zip_code", "phone_number",
}

var directorEmailValueRegex = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)

// guardDirectorOutput is the single entry point. It returns either a scrubbed
// copy of resp (PII removed) or, on a prompt-echo, a safe canned response.
func (s *DirectorConsoleService) guardDirectorOutput(payload *directorContext, businessID uint, locale string, resp DirectorStructuredResponse) DirectorStructuredResponse {
	if directorOutputLeaksSystemPrompt(resp) {
		slog.Warn("director output guard tripped: discarding answer that echoed the system prompt",
			"feature", "director_guard", "reason", "prompt_echo", "business_id", businessID)
		return sanitizeStructuredResponse(buildGuardTrippedResponse(businessID, locale))
	}

	deny := collectSensitiveContextValues(payload)
	if scrubbed, hit := scrubDirectorDenylist(resp, deny); hit {
		slog.Warn("director output guard scrubbed customer PII from the answer",
			"feature", "director_guard", "reason", "pii_leak", "business_id", businessID)
		resp = scrubbed
	}

	// Email/phone regex backstop (also covers values the model synthesized or
	// reformatted) and the existing date/metric-safe redaction contract.
	return sanitizeStructuredResponse(resp)
}

// directorBannedWirePhrases must never appear in operator-facing Director
// prose. Includes both raw schema/tool identifiers and the first-pass
// "friendly" report names QA still flagged as leaks.
var directorBannedWirePhrases = []string{
	"qty_sold",
	"weekly_revenue",
	"data_readiness",
	"get_menu_top_items",
	"get_food_cost_analysis",
	"preview_margin_change",
	"top-sellers report",
	"food-cost analysis",
}

func directorContainsBannedWire(s string) bool {
	lower := strings.ToLower(s)
	for _, p := range directorBannedWirePhrases {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

var directorSnakeFieldRe = regexp.MustCompile(`\b[a-z][a-z0-9]*_[a-z0-9_]+\b`)

func directorEvidenceLooksLikeSchemaDump(s string) bool {
	if directorContainsBannedWire(s) {
		return true
	}
	lower := strings.ToLower(s)
	for _, tok := range []string{
		"metrics.",
		"median_qty_sold",
		"food_cost_pct",
		"menu_item_id",
		"get_revenue_summary",
		"get_menu_underperformers",
		"underperformers report",
	} {
		if strings.Contains(lower, tok) {
			return true
		}
	}
	return directorSnakeFieldRe.MatchString(s)
}

// directorInternalReplacements strips schema keys, tool names, and demo SKU
// ids from operator-facing prose. Longer tokens first so prefixes don't
// leave a dangling suffix. Replacement text must itself stay off the
// banned-wire list (no "top-sellers report" / "food-cost analysis").
var directorInternalReplacements = []struct{ from, to string }{
	{"herramienta de disponibilidad", "menu availability"},
	{"get_food_cost_analysis", "dish margins"},
	{"preview_margin_change", "margin preview"},
	{"get_menu_underperformers", "slow-moving dishes"},
	{"propose_availability_change", "availability preview"},
	{"get_menu_top_items", "best sellers"},
	{"get_revenue_summary", "sales figures"},
	{"propose_price_change", "price preview"},
	{"get_kitchen_status", "kitchen board"},
	{"get_live_floor", "live floor"},
	{"get_promos", "offers and bundles"},
	{"top-sellers report", "best sellers"},
	{"food-cost analysis", "dish margins"},
	{"underperformers report", "slow-moving dishes"},
	{"median_qty_sold", "typical units sold"},
	{"metrics.weekly_revenue", "this week's sales"},
	{"metrics.weekly revenue", "this week's sales"},
	{"weekly_revenue", "this week's sales"},
	{"qty_sold", "units sold"},
	{"data_readiness", "setup status"},
	{"food_cost_pct", "food cost share"},
	{"menu_item_id", "dish"},
	{"item demo-steak", "Steak Plate"},
	{"item demo-bowl", "Harvest Bowl"},
	{"demo-steak", "Steak Plate"},
	{"demo-bowl", "Harvest Bowl"},
}

func scrubDirectorInternalJargon(s string) string {
	if s == "" {
		return s
	}
	for _, pair := range directorInternalReplacements {
		s = strings.ReplaceAll(s, pair.from, pair.to)
	}
	return s
}

// directorProseBlob concatenates the human-readable text fields of a response.
// DeepLink is intentionally excluded: it legitimately carries exactly one tab
// token and must not feed the tab-echo heuristic.
func directorProseBlob(resp DirectorStructuredResponse) string {
	var b strings.Builder
	write := func(s string) {
		b.WriteString(s)
		b.WriteByte('\n')
	}
	write(resp.Summary)
	write(resp.Diagnosis)
	write(resp.ExpectedImpact)
	for _, e := range resp.Evidence {
		write(e)
	}
	for _, f := range resp.FollowUps {
		write(f)
	}
	for _, a := range resp.ActionPlan {
		write(a.Title)
		write(a.Description)
	}
	return b.String()
}

// normalizeForLeakScan folds Unicode compatibility forms, strips zero-width
// runes, and removes all whitespace so spaced / zero-width / full-width evasions
// of the marker phrases still match. Both sides of the comparison are normalized
// identically. (Full confusable/homoglyph folding — e.g. Cyrillic look-alikes —
// is out of scope; this closes the common Unicode evasions.)
func normalizeForLeakScan(s string) string {
	s = norm.NFKC.String(s)
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\u200B', // zero-width space
			'\u200C', // zero-width non-joiner
			'\u200D', // zero-width joiner
			'\u2060', // word joiner
			'\uFEFF': // zero-width no-break space / BOM
			continue
		}
		if unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

// directorOutputLeaksSystemPrompt reports whether the model echoed its own
// instructions or dumped the allowed-tabs list.
func directorOutputLeaksSystemPrompt(resp DirectorStructuredResponse) bool {
	text := normalizeForLeakScan(directorProseBlob(resp))

	for _, phrase := range directorPromptLeakPhrases {
		if strings.Contains(text, normalizeForLeakScan(phrase)) {
			return true
		}
	}

	// The hyphenated compound tab tokens never occur in natural prose (the model
	// writes "AI Waiter" / "business page" with spaces); the kebab form only
	// appears when a raw tab token is echoed. Both together is conclusive.
	if strings.Contains(text, "business-page") && strings.Contains(text, "ai-waiter") {
		return true
	}

	// A blatant enumeration: many distinct tabs named in one answer.
	distinct := 0
	for tab := range directorAllowedTabs {
		if strings.Contains(text, tab) {
			distinct++
		}
	}
	return distinct >= directorAllowedTabsDistinctEchoThreshold
}

// collectSensitiveContextValues walks the server-built context and returns the
// exact string values that are customer PII: anything under a sensitive key, plus
// anything that looks like an email anywhere. Names and aggregates are excluded,
// so first-name references and metrics survive. Short values (<6 runes) are
// ignored to avoid scrubbing trivial tokens.
func collectSensitiveContextValues(payload *directorContext) []string {
	if payload == nil {
		return nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	var tree interface{}
	if err := json.Unmarshal(raw, &tree); err != nil {
		return nil
	}

	seen := map[string]struct{}{}
	var out []string
	add := func(v string) {
		v = strings.TrimSpace(v)
		if len([]rune(v)) < 6 {
			return
		}
		if _, dup := seen[v]; dup {
			return
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}

	var walk func(node interface{}, keyHint string)
	walk = func(node interface{}, keyHint string) {
		switch n := node.(type) {
		case map[string]interface{}:
			for k, v := range n {
				walk(v, k)
			}
		case []interface{}:
			for _, v := range n {
				walk(v, keyHint)
			}
		case string:
			if directorKeyIsSensitive(keyHint) || directorEmailValueRegex.MatchString(n) {
				add(n)
			}
		}
	}
	walk(tree, "")
	return out
}

func directorKeyIsSensitive(key string) bool {
	lower := strings.ToLower(key)
	for _, compound := range directorSensitiveCompoundKeys {
		if strings.Contains(lower, compound) {
			return true
		}
	}
	for _, seg := range strings.FieldsFunc(lower, func(r rune) bool {
		return r == '_' || r == '-' || r == '.' || r == ' '
	}) {
		if _, ok := directorSensitiveKeySegments[seg]; ok {
			return true
		}
	}
	return false
}

// scrubDirectorDenylist removes every denylisted value (verbatim, case-sensitive
// — the model echoes context values as-is) from the text fields. DeepLink is left
// untouched: it is a validated internal tab link, never free text. Returns the
// (possibly) modified response and whether anything was scrubbed.
func scrubDirectorDenylist(resp DirectorStructuredResponse, deny []string) (DirectorStructuredResponse, bool) {
	if len(deny) == 0 {
		return resp, false
	}
	hit := false
	scrub := func(s string) string {
		for _, v := range deny {
			if v == "" || !strings.Contains(s, v) {
				continue
			}
			hit = true
			s = strings.ReplaceAll(s, v, directorRedactionToken)
		}
		return s
	}
	resp.Summary = scrub(resp.Summary)
	resp.Diagnosis = scrub(resp.Diagnosis)
	resp.ExpectedImpact = scrub(resp.ExpectedImpact)
	for i := range resp.Evidence {
		resp.Evidence[i] = scrub(resp.Evidence[i])
	}
	for i := range resp.FollowUps {
		resp.FollowUps[i] = scrub(resp.FollowUps[i])
	}
	for i := range resp.ActionPlan {
		resp.ActionPlan[i].Title = scrub(resp.ActionPlan[i].Title)
		resp.ActionPlan[i].Description = scrub(resp.ActionPlan[i].Description)
	}
	return resp, hit
}

// buildGuardTrippedResponse is the canned, safe answer returned when the model's
// output is discarded for echoing its instructions. It re-prompts the owner.
func buildGuardTrippedResponse(businessID uint, locale string) DirectorStructuredResponse {
	id := strconv.Itoa(int(businessID))
	if directorUsesSpanishCopy(locale) {
		return DirectorStructuredResponse{
			Summary:   "No puedo mostrar esa respuesta.",
			Diagnosis: "Una verificación de seguridad detuvo la última respuesta. Probemos de nuevo con una pregunta concreta sobre tu negocio.",
			Evidence: []string{
				"Este copiloto solo comparte análisis del negocio; nunca repite sus instrucciones internas ni datos de contacto de clientes.",
			},
			ActionPlan: []DirectorAction{{
				Title:       "Revisar el desempeño de ventas",
				Description: "Preguntá por una métrica o un período específico para empezar.",
				DeepLink:    buildTabDeepLink(id, "analytics"),
				Priority:    "high",
			}},
			ExpectedImpact: "Una pregunta enfocada en métricas permite acciones concretas y útiles.",
			FollowUps: []string{
				"¿Qué métrica querés mejorar primero: ingresos, ticket promedio o retención?",
			},
		}
	}
	return DirectorStructuredResponse{
		Summary:   "I can't show that response.",
		Diagnosis: "A safety check stopped the last answer. Let's try again with a specific question about your business.",
		Evidence: []string{
			"This copilot only shares business analysis; it never repeats its internal instructions or customer contact details.",
		},
		ActionPlan: []DirectorAction{{
			Title:       "Review sales performance",
			Description: "Ask about a specific metric or time period to get started.",
			DeepLink:    buildTabDeepLink(id, "analytics"),
			Priority:    "high",
		}},
		ExpectedImpact: "A metrics-focused question lets me produce specific, high-impact actions.",
		FollowUps: []string{
			"Which metric should we improve first: revenue, average ticket, or retention?",
		},
	}
}
