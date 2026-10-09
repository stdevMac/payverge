package agents

import (
	"strings"
	"testing"
)

func TestExtractLeakedActions_observedKeyValueRun(t *testing.T) {
	answer := "You can update your menu here.\nhref: /business/2/dashboard?tab=menu kind: primary disabled: false disabled_reason: null"
	clean, actions := extractLeakedActions(answer, 2)

	if len(actions) != 1 {
		t.Fatalf("actions=%d want 1 (%#v)", len(actions), actions)
	}
	got := actions[0]
	if got.Href != "/business/2/dashboard?tab=menu" {
		t.Fatalf("href=%q want /business/2/dashboard?tab=menu", got.Href)
	}
	if got.Kind != "navigate" {
		t.Fatalf("kind=%q want navigate (primary is not a valid kind)", got.Kind)
	}
	if got.Disabled {
		t.Fatalf("disabled=%v want false", got.Disabled)
	}
	if got.DisabledReason != "" {
		t.Fatalf("disabled_reason=%q want empty (null)", got.DisabledReason)
	}
	if got.Label == "" {
		t.Fatalf("label must fall back to preceding text or href segment, got empty")
	}
	if clean != "You can update your menu here." {
		t.Fatalf("clean answer=%q want %q", clean, "You can update your menu here.")
	}
	_ = strings.TrimSpace // keep strings import used across this file's later tests
}

func TestExtractLeakedActions_jsonBlobInAnswer(t *testing.T) {
	answer := `Open the menu builder: {"label":"Edit menu","href":"/business/3/dashboard?tab=menu","kind":"navigate","disabled":false,"disabled_reason":null} and you are set.`
	clean, actions := extractLeakedActions(answer, 3)

	if len(actions) != 1 {
		t.Fatalf("actions=%d want 1 (%#v)", len(actions), actions)
	}
	if actions[0].Href != "/business/3/dashboard?tab=menu" {
		t.Fatalf("href=%q", actions[0].Href)
	}
	if actions[0].Label != "Edit menu" {
		t.Fatalf("label=%q want Edit menu", actions[0].Label)
	}
	if actions[0].Kind != "navigate" {
		t.Fatalf("kind=%q want navigate", actions[0].Kind)
	}
	if strings.Contains(clean, "{") || strings.Contains(clean, "href") {
		t.Fatalf("json blob leaked into clean answer: %q", clean)
	}
	if !strings.Contains(clean, "Open the menu builder") || !strings.Contains(clean, "you are set") {
		t.Fatalf("surrounding prose not preserved: %q", clean)
	}
}

func TestExtractLeakedActions_multipleLeakedActions(t *testing.T) {
	answer := "Menu: href: /business/4/dashboard?tab=menu kind: navigate\nBills: href: /business/4/dashboard?tab=bills kind: navigate"
	_, actions := extractLeakedActions(answer, 4)
	if len(actions) != 2 {
		t.Fatalf("actions=%d want 2 (%#v)", len(actions), actions)
	}
	if actions[0].Href != "/business/4/dashboard?tab=menu" {
		t.Fatalf("actions[0].href=%q", actions[0].Href)
	}
	if actions[1].Href != "/business/4/dashboard?tab=bills" {
		t.Fatalf("actions[1].href=%q", actions[1].Href)
	}
}

func TestExtractLeakedActions_unknownKindMapping(t *testing.T) {
	_, internalActs := extractLeakedActions("Go: href: /business/5/dashboard?tab=menu kind: primary", 5)
	if len(internalActs) != 1 || internalActs[0].Kind != "navigate" {
		t.Fatalf("internal unknown kind not mapped to navigate: %#v", internalActs)
	}
	_, externalActs := extractLeakedActions("Docs: href: https://payverge.io/docs kind: primary", 5)
	if len(externalActs) != 1 || externalActs[0].Kind != "external" {
		t.Fatalf("external unknown kind not mapped to external: %#v", externalActs)
	}
}

func TestExtractLeakedActions_emptyHrefDropped(t *testing.T) {
	answer := `Here: {"label":"Nowhere","href":"","kind":"navigate"}`
	clean, actions := extractLeakedActions(answer, 6)
	if len(actions) != 0 {
		t.Fatalf("empty-href action not dropped: %#v", actions)
	}
	if strings.Contains(clean, "{") {
		t.Fatalf("empty-href blob not stripped: %q", clean)
	}
}

func TestExtractLeakedActions_noLeakPassthrough(t *testing.T) {
	answer := "Bills live under the Serve tab. Ask me anything else."
	clean, actions := extractLeakedActions(answer, 7)
	if len(actions) != 0 {
		t.Fatalf("no-leak input produced actions: %#v", actions)
	}
	if clean != answer {
		t.Fatalf("no-leak answer mutated: got %q want %q", clean, answer)
	}
}

func TestNormalizeOpsResponse_movesLeakedActionOutOfAnswer(t *testing.T) {
	resp := StructuredResponse{
		Answer: "You can update your menu here.\nhref: /business/2/dashboard?tab=menu kind: primary disabled: false disabled_reason: null",
	}
	out := NormalizeOpsResponse(resp, 2, "how do I edit the menu?")

	if strings.Contains(out.Answer, "href:") || strings.Contains(out.Answer, "kind:") {
		t.Fatalf("leaked metadata still in answer: %q", out.Answer)
	}
	if out.Answer != "You can update your menu here." {
		t.Fatalf("answer=%q want %q", out.Answer, "You can update your menu here.")
	}
	if len(out.Actions) != 1 {
		t.Fatalf("actions=%d want 1 (%#v)", len(out.Actions), out.Actions)
	}
	if out.Actions[0].Href != "/business/2/dashboard?tab=menu" {
		t.Fatalf("href=%q", out.Actions[0].Href)
	}
	if out.Actions[0].Kind != "navigate" {
		t.Fatalf("kind=%q want navigate", out.Actions[0].Kind)
	}
}

func TestNormalizeOpsResponse_dedupesByHref(t *testing.T) {
	resp := StructuredResponse{
		Answer: "Edit menu. href: /business/2/dashboard?tab=menu kind: navigate",
		Actions: []ActionLink{{
			Label: "Edit menu",
			Href:  "/business/2/dashboard?tab=menu",
			Kind:  "navigate",
		}},
	}
	out := NormalizeOpsResponse(resp, 2, "edit menu")
	if len(out.Actions) != 1 {
		t.Fatalf("dedupe failed: actions=%d (%#v)", len(out.Actions), out.Actions)
	}
}

func TestNormalizeOpsResponse_normalizesUnknownStructuredKind(t *testing.T) {
	resp := StructuredResponse{
		Answer: "Go to the menu.",
		Actions: []ActionLink{{
			Label: "Menu",
			Href:  "/business/2/dashboard?tab=menu",
			Kind:  "primary",
		}},
	}
	out := NormalizeOpsResponse(resp, 2, "menu")
	if len(out.Actions) != 1 || out.Actions[0].Kind != "navigate" {
		t.Fatalf("unknown structured kind not normalized: %#v", out.Actions)
	}
}

func TestExtractLeakedActions_rejectsUnsafeScheme(t *testing.T) {
	cases := []struct {
		name   string
		answer string
	}{
		{"javascript", "Docs: href: javascript:alert(document.cookie) kind: primary"},
		{"data", "Docs: href: data:text/html,x kind: primary"},
		{"protocolRelative", "Docs: href: //evil.com/x kind: primary"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clean, actions := extractLeakedActions(tc.answer, 5)
			if len(actions) != 0 {
				t.Fatalf("unsafe href produced actions: %#v", actions)
			}
			// The matched text must be preserved in prose, not stripped.
			if clean != tc.answer {
				t.Fatalf("unsafe-href prose stripped: got %q want %q", clean, tc.answer)
			}
		})
	}
}

func TestExtractLeakedActions_proseMentioningHrefPreserved(t *testing.T) {
	answer := "To add a link, type the href: field name in config."
	clean, actions := extractLeakedActions(answer, 2)
	if len(actions) != 0 {
		t.Fatalf("prose mention produced actions: %#v", actions)
	}
	if clean != answer {
		t.Fatalf("prose mention mutated: got %q want %q", clean, answer)
	}
}

func TestExtractLeakedActions_multiWordDisabledReason(t *testing.T) {
	answer := "Menu is limited. href: /business/2/dashboard?tab=menu kind: navigate disabled: true disabled_reason: Owner only. Contact your manager."
	clean, actions := extractLeakedActions(answer, 2)
	if len(actions) != 1 {
		t.Fatalf("actions=%d want 1 (%#v)", len(actions), actions)
	}
	if actions[0].DisabledReason != "Owner only. Contact your manager." {
		t.Fatalf("disabled_reason=%q want %q", actions[0].DisabledReason, "Owner only. Contact your manager.")
	}
	if strings.Contains(clean, "only. Contact") {
		t.Fatalf("multi-word reason stranded in clean answer: %q", clean)
	}
}

func TestExtractLeakedActions_trailingPunctuationTrimmedFromHref(t *testing.T) {
	answer := "Go here: href: /business/2/dashboard?tab=menu."
	_, actions := extractLeakedActions(answer, 2)
	if len(actions) != 1 {
		t.Fatalf("actions=%d want 1 (%#v)", len(actions), actions)
	}
	if actions[0].Href != "/business/2/dashboard?tab=menu" {
		t.Fatalf("href=%q want /business/2/dashboard?tab=menu", actions[0].Href)
	}
}

func TestNormalizeOpsResponse_dropsUnsafeExternalAction(t *testing.T) {
	resp := StructuredResponse{
		Answer: "Go",
		Actions: []ActionLink{{
			Label: "x",
			Href:  "javascript:alert(1)",
			Kind:  "external",
		}},
	}
	out := NormalizeOpsResponse(resp, 2, "go")
	if len(out.Actions) != 0 {
		t.Fatalf("unsafe external action not dropped: %#v", out.Actions)
	}
}

func TestOpsPrompts_forbidLeakedLinksInAnswer(t *testing.T) {
	for _, locale := range []string{"en", "es", "es-ar"} {
		prompt, err := ResolveOpsAssistantPrompt(locale)
		if err != nil {
			t.Fatalf("resolve prompt %q: %v", locale, err)
		}
		if !strings.Contains(prompt, "LINK LEAKAGE") {
			t.Fatalf("prompt %q missing LINK LEAKAGE hardening section", locale)
		}
		if !strings.Contains(prompt, "navigate | external | handoff") {
			t.Fatalf("prompt %q does not restate the kind enum", locale)
		}
	}
}
