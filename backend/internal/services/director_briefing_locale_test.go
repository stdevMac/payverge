package services

import (
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// L1-23 regression: Director briefing/answer locale follows the operator
// language. Watched ALREADY-FIXED — these paths must stay green without a
// production code change. An English body under Spanish headers returns only
// if resolveDirectorAskLocale defaults to English for Spanish businesses.

func TestResolveDirectorAskLocale_UsesOwnerLanguageWhenRequestBlank(t *testing.T) {
	t.Parallel()
	biz := &database.Business{DefaultLanguage: "es"}
	got := resolveDirectorAskLocale("", biz)
	if got != "es" && !strings.HasPrefix(got, "es") {
		t.Fatalf("blank request with Spanish owner: got %q, want es*", got)
	}
}

func TestResolveDirectorAskLocale_ExplicitRequestWins(t *testing.T) {
	t.Parallel()
	biz := &database.Business{DefaultLanguage: "en"}
	got := resolveDirectorAskLocale("es", biz)
	if got != "es" && !strings.HasPrefix(got, "es") {
		t.Fatalf("explicit es request: got %q", got)
	}
}

func TestDirectorUsesSpanishCopy_ForEsOperator(t *testing.T) {
	t.Parallel()
	if !directorUsesSpanishCopy("es") {
		t.Fatal("es must use Spanish copy paths")
	}
	if !directorUsesSpanishCopy("es-AR") && !directorUsesSpanishCopy("es_ar") {
		// either form is accepted depending on normalize
		if !directorUsesSpanishCopy("es-ar") {
			t.Fatal("es-AR family must use Spanish copy")
		}
	}
	if directorUsesSpanishCopy("en") {
		t.Fatal("en must not use Spanish copy")
	}
}

func TestDirectorLanguageInstruction_SpanishIsNotEnglish(t *testing.T) {
	t.Parallel()
	loc := resolvePromptLocale("es")
	instr := directorLanguageInstruction(loc)
	if !strings.Contains(strings.ToLower(instr), "spanish") &&
		!strings.Contains(strings.ToLower(instr), "español") {
		t.Fatalf("Spanish instruction missing Spanish cue: %q", instr)
	}
	if strings.Contains(instr, "Respond in English") {
		t.Fatalf("Spanish locale must not instruct English: %q", instr)
	}
}
