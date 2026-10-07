package ops_guides

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
)

// Guide is one deterministic Ops how-to entry. Stable fields (ID/tab/destination/
// permission/plan/workflow/follow-ups) are locale-invariant; copy fields vary.
type Guide struct {
	ID                 string   `json:"id"`
	Tab                string   `json:"tab"`
	Phrases            []string `json:"phrases"`
	Answer             string   `json:"answer"`
	Steps              []string `json:"steps"`
	Destination        string   `json:"destination"`
	RequiredPermission string   `json:"required_permission"`
	WorkflowID         string   `json:"workflow_id,omitempty"`
	FollowUpIDs        []string `json:"follow_up_ids"`
	FollowUpLabel      string   `json:"follow_up_label"`
	FollowUpPrompt     string   `json:"follow_up_prompt"`
	DestinationLabel   string   `json:"destination_label"`
}

// GuideMatch is a scored retrieval hit.
type GuideMatch struct {
	Guide Guide
	Score float64
	Exact bool
}

// Catalog is a locale-keyed guide set.
type Catalog struct {
	byLocale map[string][]Guide
	byID     map[string]map[string]Guide // locale -> id -> guide
}

// RequiredGuideIDs must exist in every locale.
var RequiredGuideIDs = []string{
	"overview-get-started", "bills-open-close", "cash-register-shift", "printers-connect",
	"kitchen-order-flow", "reservations-manage", "menu-add-item", "tables-create-qr",
	"guest-order-experience",
	"ai-waiter-configure", "ai-waiter-takeover", "director-boundary", "marketing-create-draft",
	"analytics-read", "crm-customer", "delivery-configure", "counter-use",
	"inventory-stock", "staff-invite", "schedule-build", "business-page-publish",
	"accounting-review", "fiscal-setup", "plugins-connect",
	"settings-update", "support-contact",
}

// NewDefaultCatalog builds the seeded multi-locale catalog.
func NewDefaultCatalog() *Catalog {
	c := &Catalog{
		byLocale: map[string][]Guide{
			"en":    enGuides(),
			"es":    esGuides(),
			"es-AR": esARGuides(),
		},
		byID: map[string]map[string]Guide{},
	}
	for loc, guides := range c.byLocale {
		c.byID[loc] = make(map[string]Guide, len(guides))
		for _, g := range guides {
			c.byID[loc][g.ID] = g
		}
	}
	return c
}

// Search finds guides globally. activeTab only boosts score; it never filters.
func (c *Catalog) Search(query, locale, activeTab string, limit int) []GuideMatch {
	if limit <= 0 {
		limit = 5
	}
	locale = normalizeLocale(locale)
	guides := c.byLocale[locale]
	if len(guides) == 0 {
		guides = c.byLocale["en"]
	}
	q := strings.ToLower(strings.TrimSpace(query))
	tokens := tokenize(q)
	activeTab = strings.TrimSpace(strings.ToLower(activeTab))

	var hits []GuideMatch
	for _, g := range guides {
		score, exact := scoreGuide(q, tokens, g, activeTab)
		if score <= 0 {
			continue
		}
		hits = append(hits, GuideMatch{Guide: g, Score: score, Exact: exact})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			return hits[i].Guide.ID < hits[j].Guide.ID
		}
		return hits[i].Score > hits[j].Score
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// Get returns a guide by ID for a locale (falls back to en).
func (c *Catalog) Get(locale, id string) (Guide, bool) {
	locale = normalizeLocale(locale)
	if m, ok := c.byID[locale]; ok {
		if g, ok := m[id]; ok {
			return g, true
		}
	}
	if m, ok := c.byID["en"]; ok {
		g, ok := m[id]
		return g, ok
	}
	return Guide{}, false
}

// ResolveFollowUps converts stable guide IDs into localized, user-visible
// follow-ups. Unknown IDs reject the whole set so an internal identifier is
// never displayed as fallback copy.
func (c *Catalog) ResolveFollowUps(locale string, ids []string) ([]assistantcontract.FollowUp, error) {
	resolved := make([]assistantcontract.FollowUp, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		guide, ok := c.Get(locale, id)
		if !ok {
			return nil, errf("unknown follow-up guide %s", id)
		}
		if _, exists := seen[guide.ID]; exists {
			continue
		}
		if guide.FollowUpLabel == "" || guide.FollowUpPrompt == "" {
			return nil, errf("missing follow-up copy for %s", id)
		}
		seen[guide.ID] = struct{}{}
		resolved = append(resolved, assistantcontract.FollowUp{
			ID:     guide.ID,
			Label:  guide.FollowUpLabel,
			Prompt: guide.FollowUpPrompt,
		})
	}
	return resolved, nil
}

func normalizeLocale(locale string) string {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		return "en"
	}
	if strings.EqualFold(locale, "es-ar") || strings.EqualFold(locale, "es_ar") {
		return "es-AR"
	}
	if i := strings.IndexByte(locale, '-'); i > 0 {
		base := strings.ToLower(locale[:i])
		if base == "es" && strings.Contains(strings.ToLower(locale), "ar") {
			return "es-AR"
		}
		return base
	}
	return strings.ToLower(locale)
}

func tokenize(q string) []string {
	parts := strings.FieldsFunc(q, func(r rune) bool {
		return r == ' ' || r == ',' || r == '.' || r == '?' || r == '!' || r == ';' || r == ':'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if len(p) < 2 {
			continue
		}
		out = append(out, p)
	}
	return out
}

func scoreGuide(q string, tokens []string, g Guide, activeTab string) (float64, bool) {
	if q == "" {
		return 0, false
	}
	// Exact ID match
	if strings.EqualFold(q, g.ID) {
		return 100, true
	}
	score := 0.0
	exact := false
	for _, phrase := range g.Phrases {
		p := strings.ToLower(phrase)
		if q == p {
			return 90, true
		}
		if guidePhraseMatchesQuery(q, p) {
			score += 40
			exact = true
		}
	}
	// Token coverage
	matched := 0
	blob := strings.ToLower(strings.Join(g.Phrases, " ") + " " + g.Answer + " " + g.ID)
	for _, tok := range tokens {
		if strings.Contains(blob, tok) {
			matched++
		}
	}
	if len(tokens) > 0 {
		score += 20 * float64(matched) / float64(len(tokens))
	}
	// Active tab is a mild boost only.
	if activeTab != "" && g.Tab == activeTab {
		score += 3
	}
	// Support keywords hard-boost support-contact.
	if g.ID == "support-contact" {
		for _, kw := range []string{"support", "soporte", "help desk", "human", "agent", "contact"} {
			if strings.Contains(q, kw) {
				score += 50
				exact = true
			}
		}
	}
	if score < 8 {
		return 0, false
	}
	return score, exact
}

// guidePhraseMatchesQuery decides whether a guide phrase is present in the
// query. Everything but a bare number is plain substring containment, in either
// direction, as it always was.
func guidePhraseMatchesQuery(q, p string) bool {
	if numericGuidePhrase(p) {
		return numericPhraseReadsAsSlang(q, p)
	}
	return strings.Contains(q, p) || strings.Contains(p, q)
}

// numericGuidePhrase reports whether a phrase is nothing but digits — today only
// the restaurant slang "86".
func numericGuidePhrase(p string) bool {
	if p == "" {
		return false
	}
	for _, r := range p {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// numericPhraseReadsAsSlang reports whether a bare number in the query is the
// sold-out slang rather than a quantity, a table number, an amount of money or
// part of a longer number. Substring matching cannot tell these apart: it fires
// on "186", on "$86" and on "la mesa 86 pidió la cuenta", which is how the ES
// inventory guide started swallowing four unrelated questions.
//
// The slang has three shapes, and only these three: the number opens the
// sentence as a verb ("86 the ribeye", "86 el bife de chorizo"), it closes the
// sentence as a state ("¿qué está 86?"), or a copula puts it in the state
// position ("what is 86'd?").
func numericPhraseReadsAsSlang(q, p string) bool {
	words := strings.FieldsFunc(q, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	for i, word := range words {
		if word != p {
			continue
		}
		if i == 0 || i == len(words)-1 {
			return true
		}
		switch words[i-1] {
		case "is", "are", "was", "were", "be", "been", "got",
			"esta", "está", "estan", "están", "estaba", "estaban",
			"queda", "quedan", "quedo", "quedó", "quedaron", "sigue", "siguen":
			return true
		}
	}
	return false
}

// ValidateCatalog fails if any invariant is broken.
func (c *Catalog) ValidateCatalog() error {
	for _, loc := range []string{"en", "es", "es-AR"} {
		guides := c.byLocale[loc]
		if len(guides) == 0 {
			return errf("missing locale %s", loc)
		}
		seen := map[string]bool{}
		for _, g := range guides {
			if seen[g.ID] {
				return errf("duplicate id %s in %s", g.ID, loc)
			}
			seen[g.ID] = true
			if _, ok := LookupTab(g.Tab); !ok && g.Tab != "support" {
				// support-contact may use overview as tab.
				if g.Tab != "overview" {
					return errf("unknown tab %q for %s", g.Tab, g.ID)
				}
			}
			if g.Answer == "" {
				return errf("empty answer %s/%s", loc, g.ID)
			}
			if g.FollowUpLabel == "" || g.FollowUpPrompt == "" {
				return errf("missing follow-up copy %s/%s", loc, g.ID)
			}
			if g.DestinationLabel == "" {
				return errf("missing destination label %s/%s", loc, g.ID)
			}
			if utf8.RuneCountInString(g.Answer) > 600 {
				return errf("answer too long %s/%s", loc, g.ID)
			}
			if len(g.Steps) > 5 {
				return errf("too many steps %s/%s", loc, g.ID)
			}
			for _, s := range g.Steps {
				if utf8.RuneCountInString(s) > 120 {
					return errf("step too long %s/%s", loc, g.ID)
				}
			}
			if g.Destination != "" && !strings.HasPrefix(g.Destination, "tab:") && !strings.HasPrefix(g.Destination, "route:") {
				return errf("invalid destination %s for %s", g.Destination, g.ID)
			}
			if g.RequiredPermission == "" {
				return errf("missing permission %s", g.ID)
			}
		}
		for _, id := range RequiredGuideIDs {
			if !seen[id] {
				return errf("missing required guide %s in %s", id, loc)
			}
		}
	}
	// Stable fields match across locales.
	en := c.byID["en"]
	for _, loc := range []string{"es", "es-AR"} {
		for id, eg := range en {
			og, ok := c.byID[loc][id]
			if !ok {
				continue
			}
			if eg.Tab != og.Tab || eg.Destination != og.Destination ||
				eg.RequiredPermission != og.RequiredPermission {
				return errf("stable field mismatch for %s across %s", id, loc)
			}
		}
	}
	return nil
}

type catalogError string

func (e catalogError) Error() string { return string(e) }

func errf(format string, args ...any) error {
	return catalogError(fmt.Sprintf(format, args...))
}
