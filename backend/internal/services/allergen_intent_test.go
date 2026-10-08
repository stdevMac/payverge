package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectAllergenIntent_MultiLocale(t *testing.T) {
	cases := []struct {
		locale, text string
		want         bool
	}{
		{"en", "Does the burger contain peanuts?", true},
		{"en", "Is the salad gluten free?", true},
		{"es", "¿El pan tiene gluten?", true},
		{"es-AR", "¿La torta lleva maní?", true},
		{"fr", "Est-ce que ça contient des arachides ?", true},
		{"de", "Enthält das Erdnüsse?", true},
		{"ja", "これにはアレルギー物質が入っていますか？", true},
		{"en", "Can I get two burgers?", false},
		{"en", "What time do you close?", false},
	}
	for _, c := range cases {
		if got := DetectAllergenIntent(c.locale, c.text); got != c.want {
			t.Fatalf("DetectAllergenIntent(%q,%q)=%v want %v", c.locale, c.text, got, c.want)
		}
	}
}

// #596: allergen keywords must be token-boundary aware. "egg" hiding inside
// "veggie" used to short-circuit a dietary question into the allergen refusal
// before the dietary/recommendation path could run. Deliberate over-trigger
// for real allergen words (stems, plurals, compounds) must keep firing.
func TestDetectAllergenIntent_WordBoundaries(t *testing.T) {
	cases := []struct {
		locale, text string
		want         bool
	}{
		// English: dietary phrasing must not trip the allergen path.
		{"en", "do you have veggie options ?", false}, // "egg" inside "veggie"
		{"en", "vegetarian options?", false},
		{"en", "any vegan mains?", false},
		// English: genuine allergen asks keep firing.
		{"en", "does it contain egg?", true},
		{"en", "nut allergy", true},
		{"en", "is there egg in the pasta?", true},
		{"en", "are there eggs in this?", true}, // prefix stem: egg → eggs
		// German: "ei" (egg) must not fire inside everyday words.
		{"de", "Welche Weine haben Sie?", false}, // "ei" inside "weine"
		{"de", "Haben Sie vegetarische Optionen?", false},
		{"de", "Ich hätte gerne eine Cola", false}, // "ei" inside "eine"
		{"de", "Ist Ei in der Soße?", true},
		{"de", "Sind Eier enthalten?", true},
		{"de", "Ich habe eine Nussallergie", true}, // compound: "nuss" token prefix
		// Dutch: "ei" inside "eigenlijk".
		{"nl", "wat is eigenlijk het beste gerecht?", false},
		{"nl", "zit er ei in dit gerecht?", true},
		// Danish: "nød" inside "nødvendig", "æg" inside "ægte".
		{"da", "er det nødvendigt at bestille?", false},
		{"da", "en ægte klassiker, tak", false},
		{"da", "er der æg i retten?", true},
		{"da", "indeholder den nødder?", true}, // "nødde" stem
		// French: "lait" inside "laitue" (lettuce).
		{"fr", "une salade de laitue, s'il vous plaît", false},
		{"fr", "est-ce qu'il y a du lait dedans ?", true},
		// Vietnamese: "cá" (fish) inside "các" (plural marker).
		{"vi", "các món chay có gì?", false},
		{"vi", "món này có cá không?", true},
		// Chinese: bare "过" (aspect particle, in almost every sentence) must
		// not fire; "过敏" (allergy) must. Unspaced script keeps substring
		// semantics for real keywords.
		{"zh", "我去过很多餐厅，有什么推荐吗", false},
		{"zh", "我对花生过敏", true},
		// Thai: "มี" ("have") was in nearly every availability question;
		// genuine allergen vocabulary still fires substring-style.
		{"th", "มีเมนูมังสวิรัติไหม", false},
		{"th", "ฉันแพ้ถั่ว", true},
		// Arabic keeps substring semantics because clitics attach to the word.
		{"ar", "هل يحتوي على الحليب؟", true}, // "حليب" inside "الحليب"
	}
	for _, c := range cases {
		if got := DetectAllergenIntent(c.locale, c.text); got != c.want {
			t.Errorf("DetectAllergenIntent(%q,%q)=%v want %v", c.locale, c.text, got, c.want)
		}
	}
}

func TestAllergenDisclaimer_AllGuestLocalesNonEmpty(t *testing.T) {
	for _, code := range allGuestCodesForTest() {
		if AllergenDisclaimer(code) == "" {
			t.Fatalf("empty allergen disclaimer for %q", code)
		}
		if AllergenRefusal(code) == "" {
			t.Fatalf("empty allergen refusal for %q", code)
		}
	}
}

func TestAppendAllergenDisclaimer_Idempotent(t *testing.T) {
	base := "The Caesar Salad lists dairy."
	once := AppendAllergenDisclaimer("en", base)
	twice := AppendAllergenDisclaimer("en", once)
	if once != twice {
		t.Fatalf("disclaimer not idempotent:\n once=%q\n twice=%q", once, twice)
	}
	if once == base {
		t.Fatalf("disclaimer was not appended")
	}
}

func allGuestCodesForTest() []string {
	return []string{"ar", "da", "de", "en", "es", "es-AR", "fr", "hi", "it", "ja", "ko", "nl", "no", "pl", "pt", "ru", "sv", "th", "tr", "vi", "zh"}
}

func TestWaiterCopyAllGuestLocalesNonEmpty(t *testing.T) {
	for _, code := range allGuestCodesForTest() {
		if WaiterOffTopicRedirect(code) == "" {
			t.Fatalf("empty off-topic redirect for %q", code)
		}
		if WaiterAbuseDecline(code) == "" {
			t.Fatalf("empty abuse decline for %q", code)
		}
		if WaiterHumanAssisting(code) == "" {
			t.Fatalf("empty human-assisting ack for %q", code)
		}
	}
}

func TestClarifyItemMessage_LocalizedWithFallback(t *testing.T) {
	for _, code := range allGuestCodesForTest() {
		if ClarifyItemMessage(code) == "" {
			t.Fatalf("empty clarify message for %q", code)
		}
	}
	// A non-English locale must not fall through to the English copy.
	if ClarifyItemMessage("ja") == ClarifyItemMessage("en") {
		t.Fatal("ja clarify message must be localized, not English")
	}
	// An unknown locale falls back to English.
	if ClarifyItemMessage("xx") != ClarifyItemMessage("en") {
		t.Fatal("unknown locale should fall back to English clarify message")
	}
}

func TestMentionsStaffConfirmation(t *testing.T) {
	// exact canonical disclaimer already present
	if !MentionsStaffConfirmation("en", "Foo. "+AllergenDisclaimer("en")) {
		t.Error("exact disclaimer should match")
	}
	// model paraphrase (staff word + confirm word) -> already covered
	if !MentionsStaffConfirmation("en", "I can't be sure — please confirm with our staff before ordering.") {
		t.Error("en paraphrase should match")
	}
	if !MentionsStaffConfirmation("es", "No estoy seguro; por favor confirmá con el personal antes de pedir.") {
		t.Error("es paraphrase should match")
	}
	if !MentionsStaffConfirmation("fr", "Veuillez vérifier auprès du personnel.") {
		t.Error("fr paraphrase should match")
	}
	// a lone staff mention is NOT a confirmation -> must NOT suppress (disclaimer still needed)
	if MentionsStaffConfirmation("en", "Our staff made this fresh today.") {
		t.Error("lone 'staff' must not count as confirmation")
	}
	// unrelated text -> false
	if MentionsStaffConfirmation("en", "The pasta has egg in it.") {
		t.Error("no confirmation present")
	}
}

func TestOrderingPausedMessage_LocalizedWithFallback(t *testing.T) {
	en := OrderingPausedMessage("en")
	if en == "" || !strings.Contains(strings.ToLower(en), "paused") {
		t.Fatalf("english ordering-paused message missing: %q", en)
	}
	if OrderingPausedMessage("xx") != en {
		t.Fatalf("unknown locale must fall back to english")
	}
	ja := OrderingPausedMessage("ja")
	if ja == "" || ja == en {
		t.Fatalf("japanese ordering-paused must be localized, got %q", ja)
	}
}

// #950: natural Spanish/Portuguese allergen phrasing is ACCENTED ("soy
// alérgico", "¿tiene alérgenos?"), but the vocabulary was matched literally
// against unaccented stems ("alerg"), so "alérg" never hit and the turn routed
// as a generic food question with no staff-confirmation disclaimer attached.
// Matching now folds Latin diacritics on BOTH sides, so accented text hits
// unaccented stems AND unaccented text hits accented keywords ("glúten",
// "lácteo"). Token boundaries and the deliberate stem over-trigger are
// unchanged.
func TestDetectAllergenIntent_LatinDiacriticFolding(t *testing.T) {
	cases := []struct {
		name         string
		locale, text string
		want         bool
	}{
		// --- #950 core: accented Spanish must fire ---
		{"es accented masculine", "es", "soy alérgico", true},
		{"es accented feminine", "es", "soy alérgica", true},
		{"es accented noun question", "es", "¿tiene alérgenos?", true},
		{"es accented tree nuts", "es", "alérgico a los frutos secos", true},
		{"es_ar accented noun question", "es-AR", "¿esto tiene alérgenos?", true},
		{"pt accented masculine", "pt", "sou alérgico", true},

		// --- already-working unaccented forms must keep firing ---
		{"es unaccented alergia", "es", "tengo alergia al maní", true},
		{"es unaccented gluten", "es", "¿el pan tiene gluten?", true},
		{"en peanut", "en", "Does the burger contain peanuts?", true},
		{"fr allergique", "fr", "je suis allergique", true},

		// --- folding is symmetric: unaccented guest text hits accented keywords ---
		{"pt unaccented gluten hits glúten", "pt", "tem gluten?", true},
		{"es unaccented lacteos hits lácteo", "es", "¿lleva lacteos?", true},
		{"de unaccented nusse hits nüsse", "de", "sind da nusse drin?", true},

		// --- must NOT trigger: ordinary menu talk ---
		{"es popular dish", "es", "¿cuál es el plato más popular?", false},
		{"es closing time", "es", "¿a qué hora cierran?", false},
		{"es plain order", "es", "quiero pedir una pizza margarita", false},
		{"en ordinary order", "en", "Can I get two burgers?", false},

		// --- must NOT trigger: keyword letters hiding inside another word ---
		{"pt ovo inside novo", "pt", "tem algum prato novo?", false},
		{"de ei inside eine", "de", "Ich hätte gerne eine Cola", false},
		{"fr lait inside laitue", "fr", "une salade de laitue, s'il vous plaît", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, DetectAllergenIntent(c.locale, c.text),
				"DetectAllergenIntent(%q, %q)", c.locale, c.text)
		})
	}
}

// #950 guard: diacritic folding must stay LATIN-ONLY. Thai tone marks,
// Devanagari matras and Arabic harakat are Mn but load-bearing letters — a
// blanket runes.Remove(unicode.Mn) collapses "มีเมนู…" so that "นม" (milk)
// appears where the guest never wrote it. Non-Latin detection must be
// byte-for-byte identical to the pre-#950 behavior.
func TestDetectAllergenIntent_FoldingLeavesNonLatinScriptsIntact(t *testing.T) {
	cases := []struct {
		name         string
		locale, text string
		want         bool
	}{
		{"th vegetarian ask is not milk", "th", "มีเมนูมังสวิรัติไหม", false},
		{"th genuine peanut allergy", "th", "ฉันแพ้ถั่ว", true},
		{"hi genuine allergy", "hi", "मुझे मूंगफली से एलर्जी है", true},
		{"ar milk with attached clitic", "ar", "هل يحتوي على الحليب؟", true},
		{"zh aspect particle is not allergy", "zh", "我去过很多餐厅，有什么推荐吗", false},
		{"zh genuine peanut allergy", "zh", "我对花生过敏", true},
		{"ja allergen ask", "ja", "これにはアレルギー物質が入っていますか？", true},
		// Vietnamese IS Latin: folding drops the tone marks on both sides, so
		// the whole-token guard for "cá" must still reject "các".
		{"vi cac plural marker is not fish", "vi", "các món chay có gì?", false},
		{"vi genuine fish ask", "vi", "món này có cá không?", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, DetectAllergenIntent(c.locale, c.text),
				"DetectAllergenIntent(%q, %q)", c.locale, c.text)
		})
	}
}
