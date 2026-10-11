package agents

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/stdevmac/payverge/backend/internal/agents/ops_guides"
)

const (
	maxOpsIntents     = 5
	minOpsIntentScore = 35
)

var (
	opsIntentNumberedSeparator = regexp.MustCompile(`(?:^|[ \t])\d{1,3}[.)][ \t]+`)
	opsIntentSeparator         = regexp.MustCompile(`(?i)(?:\s*[,;:\n•·!?…]+(?:\s*\.\.\.)?\s*)|(?:\s*\.\.\.\s*)|(?:\s+/\s+)|(?:\.\s+)|(?:\s+(?:as well as|junto con|y|e|además|ademas|también|tambien|luego|después|despues|más|mas|and|also|plus|then|&)\s+)`)
	opsIntentSpacedSlash       = regexp.MustCompile(`\s+/\s+`)
)

// ResolveOpsIntents resolves an explicitly ordered, bounded set of guide
// matches. Catalog search stays global; the active tab is context, not a
// filter or a reason to reorder the user's requested areas.
func ResolveOpsIntents(catalog *ops_guides.Catalog, query, locale, activeTab string) []ops_guides.GuideMatch {
	if catalog == nil || strings.TrimSpace(query) == "" {
		return nil
	}

	clauses := splitOpsIntentClauses(query)
	if len(clauses) == 0 {
		return nil
	}
	if len(clauses) > 1 && !hasExplicitOpsIntentList(query) {
		if whole, ok := strongWholeOpsIntent(catalog, query, locale, activeTab, clauses); ok {
			return collapseOpsDinerExperience(catalog, query, locale, []ops_guides.GuideMatch{whole})
		}
	}

	resolved := make([]ops_guides.GuideMatch, 0, min(len(clauses), maxOpsIntents))
	seen := make(map[string]struct{}, maxOpsIntents)
	for _, clause := range clauses {
		searchClause := normalizeOpsIntentClause(clause)
		searchClause = expandOpsIntentAliases(searchClause, locale)
		hits := catalog.Search(searchClause, locale, "", maxOpsIntents)
		for _, hit := range hits {
			if !isResolvableOpsIntentHit(clause, hit) {
				continue
			}
			if _, duplicate := seen[hit.Guide.ID]; duplicate {
				continue
			}
			seen[hit.Guide.ID] = struct{}{}
			resolved = append(resolved, hit)
			break
		}
		if len(resolved) == maxOpsIntents {
			break
		}
	}

	// A normal whole-query question with no explicit multi-intent structure
	// remains a single result by construction. If splitting produced only noise,
	// give the whole query one final strong-match opportunity.
	if len(resolved) == 0 && len(clauses) > 1 {
		searchQuery := expandOpsIntentAliases(normalizeOpsIntentClause(query), locale)
		hits := catalog.Search(searchQuery, locale, activeTab, maxOpsIntents)
		for _, hit := range hits {
			if isResolvableOpsIntentHit(query, hit) {
				return collapseOpsDinerExperience(catalog, query, locale, []ops_guides.GuideMatch{hit})
			}
		}
	}

	return collapseOpsDinerExperience(catalog, query, locale, resolved)
}

func splitOpsIntentClauses(query string) []string {
	query = opsIntentNumberedSeparator.ReplaceAllString(query, "\n")
	parts := opsIntentSeparator.Split(query, -1)
	clauses := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(strings.TrimSpace(part), "¿¡-–—")
		if part != "" {
			clauses = append(clauses, part)
		}
	}
	return clauses
}

func hasExplicitOpsIntentList(query string) bool {
	return opsIntentNumberedSeparator.MatchString(query) ||
		strings.ContainsAny(query, ",;:\n•·") ||
		opsIntentSpacedSlash.MatchString(query)
}

func hasExplicitSupportIntent(query string, guide ops_guides.Guide) bool {
	for _, phrase := range guide.Phrases {
		if containsOpsIntentPhrase(query, phrase) {
			return true
		}
	}
	return hasSupplementalSupportIntent(query)
}

func hasSupplementalSupportIntent(query string) bool {
	for _, phrase := range []string{
		"help desk", "human agent", "support agent", "contact payverge",
		"speak to an agent", "conectarme con un agente", "ayuda de una persona",
	} {
		if containsOpsIntentPhrase(query, phrase) {
			return true
		}
	}
	return false
}

func containsOpsIntentPhrase(text, phrase string) bool {
	text = canonicalOpsIntentPhrase(text)
	phrase = canonicalOpsIntentPhrase(phrase)
	return phrase != "" && strings.Contains(" "+text+" ", " "+phrase+" ")
}

func canonicalOpsIntentPhrase(text string) string {
	text = normalizeOpsIntentClause(text)
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	return strings.Join(words, " ")
}

func isResolvableOpsIntentHit(query string, hit ops_guides.GuideMatch) bool {
	if !hit.Exact && hit.Score < minOpsIntentScore {
		return false
	}
	// The takeover guide carries generic stop-verb phrases ("stop ai",
	// "turn off ai") that outscore the surface the operator actually named, so
	// "turn off AI recommendations in inventory" beat the Inventory guide. The
	// alias gate already knows when a stop verb belongs to another surface;
	// apply the same judgement to a direct phrase hit.
	if hit.Guide.ID == "ai-waiter-takeover" && opsStopVerbBelongsToAnotherSurface(query) {
		return false
	}
	return hit.Guide.ID != "support-contact" || hasExplicitSupportIntent(query, hit.Guide)
}

func strongWholeOpsIntent(catalog *ops_guides.Catalog, query, locale, activeTab string, clauses []string) (ops_guides.GuideMatch, bool) {
	searchQuery := expandOpsIntentAliases(normalizeOpsIntentClause(query), locale)
	hits := catalog.Search(searchQuery, locale, activeTab, maxOpsIntents)
	if len(hits) == 0 || !isResolvableOpsIntentHit(query, hits[0]) {
		return ops_guides.GuideMatch{}, false
	}
	candidate := hits[0]
	for _, clause := range clauses {
		clauseHits := catalog.Search(expandOpsIntentAliases(normalizeOpsIntentClause(clause), locale), locale, "", maxOpsIntents)
		covered := false
		for _, hit := range clauseHits {
			if hit.Guide.ID == candidate.Guide.ID && isResolvableOpsIntentHit(clause, hit) {
				covered = true
				break
			}
		}
		if !covered {
			return ops_guides.GuideMatch{}, false
		}
	}
	return candidate, true
}

func normalizeOpsIntentClause(clause string) string {
	words := strings.Fields(clause)
	normalized := make([]string, 0, len(words))
	for _, word := range words {
		clean := strings.Trim(strings.ToLower(word), "¿?¡!.,;:")
		switch clean {
		case "a", "an", "the", "un", "una", "el", "la", "los", "las":
			continue
		}
		normalized = append(normalized, clean)
	}
	return strings.Join(normalized, " ")
}

func expandOpsIntentAliases(clause, locale string) string {
	lower := strings.ToLower(clause)
	expanded := clause
	isSpanish := strings.HasPrefix(strings.ToLower(locale), "es")
	mentionsMenu := strings.Contains(lower, "menu") || strings.Contains(lower, "menú")
	mentionsAdding := strings.Contains(lower, "add") ||
		strings.Contains(lower, "agregar") ||
		strings.Contains(lower, "añadir") ||
		strings.Contains(lower, "crear")
	if mentionsMenu && mentionsAdding {
		if isSpanish {
			expanded += " agregar platos"
		} else {
			expanded += " add menu items"
		}
	}
	mentionsGuests := strings.Contains(lower, "guest") ||
		strings.Contains(lower, "guests") ||
		strings.Contains(lower, "diner") ||
		strings.Contains(lower, "comensal") ||
		strings.Contains(lower, "comensales") ||
		strings.Contains(lower, "clientes")
	mentionsOrdering := strings.Contains(lower, "order") ||
		strings.Contains(lower, "ordering") ||
		strings.Contains(lower, "piden") ||
		strings.Contains(lower, "pedir") ||
		strings.Contains(lower, "pedido")
	mentionsDinerExperience := opsDinerExperienceQuery(clause)
	if (mentionsGuests && mentionsOrdering) || mentionsDinerExperience {
		if isSpanish {
			expanded += " cómo piden los comensales"
		} else {
			expanded += " how guests order"
		}
	}
	mentionsAIWaiter := strings.Contains(lower, "ai waiter") ||
		strings.Contains(lower, "camarero ia") ||
		strings.Contains(lower, "mozo ia") ||
		strings.Contains(lower, "mesero ia")
	// #874: naming the AI Waiter used to expand to the *setup* alias no matter
	// what the operator wanted to do with it, so "pause the AI waiter" during a
	// rush answered with the configuration guide. Stopping the AI is its own
	// shipped path (Live Monitor takeover), so route it to its own guide.
	mentionsPauseOrTakeover := opsPauseTakeoverQuery(clause)
	if mentionsAIWaiter && !mentionsDinerExperience && !mentionsPauseOrTakeover {
		if isSpanish {
			expanded += " configurar asistente"
		} else {
			expanded += " ai waiter"
		}
	}
	if mentionsPauseOrTakeover {
		if isSpanish {
			expanded += " pausar ia"
		} else {
			expanded += " pause ai"
		}
	}
	if hasSupplementalSupportIntent(clause) {
		if isSpanish {
			expanded += " soporte"
		} else {
			expanded += " support"
		}
	}
	return expanded
}

// opsPauseTakeoverQuery reports whether the operator wants to stop the AI or
// answer a guest themselves, rather than set the AI up. It requires both a
// stop-style verb and an AI subject so "pause my subscription" is not read as
// an AI takeover; a query that is nothing but the verb is the AI by context,
// since the Ops Assistant is the only thing being addressed.
func opsPauseTakeoverQuery(query string) bool {
	switch canonicalOpsIntentPhrase(query) {
	case "pause", "unpause", "resume", "takeover", "take over",
		"pausar", "despausar", "reanudar", "intervenir":
		return true
	}
	stopIntent := false
	for _, verb := range []string{
		"pause", "unpause", "resume", "takeover", "take over", "turn off", "shut off", "stop",
		"pausar", "pauso", "despausar", "reanudar", "intervenir", "intervengo", "apagar", "detener", "frenar",
	} {
		if containsOpsIntentPhrase(query, verb) {
			stopIntent = true
			break
		}
	}
	if !stopIntent {
		return false
	}
	if opsStopVerbBelongsToAnotherSurface(query) {
		return false
	}
	for _, subject := range []string{
		"ai", "ia", "assistant", "asistente", "sage", "bot", "chat",
		"waiter", "camarero", "mozo", "mesero",
	} {
		if containsOpsIntentPhrase(query, subject) {
			return true
		}
	}
	return false
}

// opsStopVerbBelongsToAnotherSurface reports whether a stop-style verb is aimed
// at a Payverge surface that owns it, rather than at the AI Waiter. Without
// this the takeover alias swallows "pause my AI plan", which is not a question
// about the AI Waiter. The same greediness pulled "turn off AI recommendations in inventory" (an
// Inventory question), "stop chat notifications for my team" and "stop a waiter
// from clocking in" away from the guides they resolved to before #874, because
// "ai", "chat" and "waiter" were being read as the AI subject when they only
// qualify another surface's noun.
func opsStopVerbBelongsToAnotherSurface(query string) bool {
	for _, phrase := range []string{
		// Plan/billing nouns are never an AI takeover.
		"subscription", "subscriptions", "plan", "plans", "billing", "invoice", "renewal",
		"suscripción", "suscripcion", "suscripciones", "facturación", "facturacion",
		"renovación", "renovacion",
		// Inventory owns turning off item suggestions.
		"inventory", "inventario", "stock", "existencias",
		// Notifications own being switched off.
		"notification", "notifications", "notificación", "notificacion", "notificaciones",
		"alert", "alerts", "alerta", "alertas",
		// Time clock owns stopping a person from clocking in.
		"clock in", "clock out", "clocking in", "clocking out", "clocking",
		"fichar", "fichaje", "marcar entrada", "marcar salida",
	} {
		if containsOpsIntentPhrase(query, phrase) {
			return true
		}
	}
	return false
}

func opsDinerExperienceQuery(query string) bool {
	if containsOpsIntentPhrase(query, "what will diners see") ||
		containsOpsIntentPhrase(query, "what diners see") ||
		containsOpsIntentPhrase(query, "diner ui") ||
		containsOpsIntentPhrase(query, "sage greeting") ||
		containsOpsIntentPhrase(query, "qué ven los comensales") ||
		containsOpsIntentPhrase(query, "que ven los comensales") ||
		containsOpsIntentPhrase(query, "qué ven los clientes") ||
		containsOpsIntentPhrase(query, "que ven los clientes") {
		return true
	}
	mentionsGuests := containsOpsIntentPhrase(query, "guest") ||
		containsOpsIntentPhrase(query, "guests") ||
		containsOpsIntentPhrase(query, "diner") ||
		containsOpsIntentPhrase(query, "diners") ||
		containsOpsIntentPhrase(query, "comensal") ||
		containsOpsIntentPhrase(query, "comensales")
	mentionsSee := containsOpsIntentPhrase(query, "see") ||
		containsOpsIntentPhrase(query, "ven") ||
		containsOpsIntentPhrase(query, "ver") ||
		containsOpsIntentPhrase(query, "greeting") ||
		containsOpsIntentPhrase(query, "saludo")
	return mentionsGuests && mentionsSee
}

func collapseOpsDinerExperience(catalog *ops_guides.Catalog, query, locale string, matches []ops_guides.GuideMatch) []ops_guides.GuideMatch {
	if catalog == nil || !opsDinerExperienceQuery(query) {
		return matches
	}
	kept := make([]ops_guides.GuideMatch, 0, len(matches))
	var guest *ops_guides.GuideMatch
	for i := range matches {
		switch matches[i].Guide.ID {
		case "guest-order-experience":
			copyMatch := matches[i]
			guest = &copyMatch
		case "ai-waiter-configure", "overview-get-started":
			continue
		default:
			kept = append(kept, matches[i])
		}
	}
	if guest == nil {
		if guide, ok := catalog.Get(locale, "guest-order-experience"); ok {
			guest = &ops_guides.GuideMatch{Guide: guide, Score: 90, Exact: true}
		}
	}
	if guest == nil {
		return matches
	}
	out := make([]ops_guides.GuideMatch, 0, 1+len(kept))
	out = append(out, *guest)
	out = append(out, kept...)
	if len(out) > maxOpsIntents {
		out = out[:maxOpsIntents]
	}
	return out
}
