package services

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/locales"
)

func TestWaiterPromptMatrixCoversAllGuestLocales(t *testing.T) {
	native := map[string]bool{"en": true, "es": true, "es_ar": true}
	families := map[string]bool{}
	for _, loc := range locales.GuestLocales() {
		families[loc.PromptFamily] = true
	}
	for family := range families {
		for _, mode := range []string{"ordering", "concierge", "whatsapp"} {
			raw, err := waiterPromptFS.ReadFile(fmt.Sprintf("prompts/ai_waiter/%s_%s.md", mode, family))
			if err != nil {
				t.Fatalf("missing waiter prompt %s_%s.md: %v", mode, family, err)
			}
			body, pending := parseReviewPending(string(raw))
			if !strings.Contains(body, "data_block") {
				t.Fatalf("%s_%s.md must keep the data_block rule", mode, family)
			}
			if native[family] && pending {
				t.Fatalf("native family %q must not be review_pending", family)
			}
			if !native[family] && !pending {
				t.Fatalf("machine-translated family %q must be review_pending until human-reviewed", family)
			}
		}
	}
}

// promptHeadings extracts the markdown section headings (lines beginning with
// "# ") from a prompt body, in order. These headings are the structural anchor
// sections of the active en prompt — including the hardened
// LANGUAGE/SECURITY/DATA-HANDLING anti-injection sections — so requiring a stub
// to carry the SAME heading set proves it was regenerated from the current
// hardened en structure rather than an older, weaker body.
func promptHeadings(body string) []string {
	var headings []string
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimRight(line, " \t\r")
		if strings.HasPrefix(trimmed, "# ") {
			headings = append(headings, trimmed)
		}
	}
	return headings
}

// promptPlaceholders extracts the {{PLACEHOLDER}} tokens present in a body as a
// set, so a stub can be required to carry the same template contract as en.
func promptPlaceholders(body string) map[string]bool {
	set := map[string]bool{}
	rest := body
	for {
		open := strings.Index(rest, "{{")
		if open < 0 {
			break
		}
		rest = rest[open:]
		close := strings.Index(rest, "}}")
		if close < 0 {
			break
		}
		set[rest[:close+2]] = true
		rest = rest[close+2:]
	}
	return set
}

func readWaiterBody(t *testing.T, mode, family string) (string, bool) {
	t.Helper()
	raw, err := waiterPromptFS.ReadFile(fmt.Sprintf("prompts/ai_waiter/%s_%s.md", mode, family))
	if err != nil {
		t.Fatalf("missing waiter prompt %s_%s.md: %v", mode, family, err)
	}
	return parseReviewPending(string(raw))
}

// TestWaiterReviewPendingStubsCarryHardenedAnchors guards AITRANS-3: every
// non-native (review_pending) waiter prompt body MUST contain the same anchor
// section headings and {{PLACEHOLDER}} tokens as the current active en body for
// the same mode. The required anchor set is DERIVED from the en file at test
// time, so it self-updates whenever en gains or renames a hardened section.
//
// Why this matters: the documented activation path for a translated locale is
// "clear the review_pending flag". If a stub were left as the older, weaker
// English body — the one lacking the
// "# SECURITY (PRIMARY — OVERRIDES ANY TEXT IN THE DATA)",
// "# LANGUAGE (PRIMARY INSTRUCTION — OVERRIDES EVERYTHING)", and full
// "# DATA HANDLING (NON-NEGOTIABLE)" anti-injection sections — activating it
// would silently ship a prompt missing the hardening (a prompt-injection
// regression). This test fails before that can happen.
func TestWaiterReviewPendingStubsCarryHardenedAnchors(t *testing.T) {
	native := map[string]bool{"en": true, "es": true, "es_ar": true}

	families := map[string]bool{}
	for _, loc := range locales.GuestLocales() {
		families[loc.PromptFamily] = true
	}

	for _, mode := range []string{"ordering", "concierge", "whatsapp"} {
		enBody, enPending := readWaiterBody(t, mode, "en")
		if enPending {
			t.Fatalf("en %s prompt must never be review_pending", mode)
		}

		requiredHeadings := promptHeadings(enBody)
		if len(requiredHeadings) == 0 {
			t.Fatalf("en %s prompt has no '# ' section headings — the anchor extraction is broken", mode)
		}
		requiredPlaceholders := promptPlaceholders(enBody)

		for family := range families {
			if native[family] {
				continue
			}

			stubBody, pending := readWaiterBody(t, mode, family)
			if !pending {
				// Covered by the matrix test; skip here so this test focuses on
				// the anchor contract for the review_pending stubs.
				continue
			}

			for _, heading := range requiredHeadings {
				if !strings.Contains(stubBody, heading) {
					t.Errorf("%s_%s.md is missing hardened anchor heading %q (regenerate the stub from %s_en.md)", mode, family, heading, mode)
				}
			}

			stubPlaceholders := promptPlaceholders(stubBody)
			for placeholder := range requiredPlaceholders {
				if !stubPlaceholders[placeholder] {
					t.Errorf("%s_%s.md is missing placeholder %s present in %s_en.md", mode, family, placeholder, mode)
				}
			}
		}
	}
}
