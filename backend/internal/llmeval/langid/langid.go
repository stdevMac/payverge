// Package langid is a coarse, dependency-free language detector intended for
// LLM response-language ADHERENCE checks (does this answer look like it is in
// the requested language?), not high-precision linguistics. It detects
// non-Latin languages by Unicode script ranges and Latin-script languages by
// a stopword-frequency vote across en/es/fr/de/it/pt/nl/tr.
package langid

import (
	"strings"
	"unicode"
)

// Accuracy expectations (coarse, by design):
//   - Non-Latin scripts (ar/ru/ja/ko/zh/hi/th): near-perfect on >=1 sentence,
//     since they key on dominant Unicode script ranges.
//   - Latin scripts (en/es/fr/de/it/pt/nl/tr): reliable on a full sentence
//     (>=8 words); unreliable on 1-3 tokens (returns low confidence). zh vs ja
//     is decided by the presence of kana. This is sufficient for "did the model
//     answer in the requested language?" gating with a confidence floor; it is
//     NOT a substitute for a full CLD-style detector.

// stopwords are short, very common function words per Latin-script language.
// They are intentionally small (high-frequency, low cross-language collision).
var stopwords = map[string][]string{
	"en": {"the", "and", "you", "with", "your", "for", "how", "can", "today", "our", "help"},
	"es": {"el", "la", "los", "las", "con", "para", "qué", "puedo", "tu", "nuestro", "ayudarte", "pedido"},
	"fr": {"le", "la", "les", "vous", "avec", "votre", "comment", "puis", "dans", "notre", "aider", "commande"},
	"de": {"der", "die", "das", "und", "mit", "ihrer", "wie", "kann", "ich", "unserem", "ihnen", "bestellung"},
	"it": {"il", "la", "con", "come", "posso", "tuo", "nostro", "oggi", "aiutarti", "ordine", "nel", "del"},
	"pt": {"o", "a", "com", "como", "posso", "você", "seu", "hoje", "nosso", "pedido", "ajudar", "ao"},
	"nl": {"de", "het", "een", "met", "uw", "hoe", "kan", "ik", "ons", "helpen", "bestelling", "vandaag"},
	"tr": {"ve", "ile", "nasıl", "size", "bir", "için", "bugün", "geldiniz", "yardımcı", "siparişinizle", "olabilirim"},
}

// Detect returns the best language code and a confidence in [0,1].
// Empty / whitespace-only input returns ("", 0).
func Detect(s string) (string, float64) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", 0
	}
	if lang, conf := detectScript(s); lang != "" {
		return lang, conf
	}
	return detectLatin(s)
}

// detectScript returns a non-Latin language when a dominant non-Latin script
// is present. Confidence is the fraction of letters in that script.
func detectScript(s string) (string, float64) {
	var total, arabic, cyrillic, hangul, hiraKata, han, devanagari, thai int
	for _, r := range s {
		if !unicode.IsLetter(r) {
			continue
		}
		total++
		switch {
		case unicode.Is(unicode.Arabic, r):
			arabic++
		case unicode.Is(unicode.Cyrillic, r):
			cyrillic++
		case unicode.Is(unicode.Hangul, r):
			hangul++
		case unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r):
			hiraKata++
		case unicode.Is(unicode.Han, r):
			han++
		case unicode.Is(unicode.Devanagari, r):
			devanagari++
		case unicode.Is(unicode.Thai, r):
			thai++
		}
	}
	if total == 0 {
		return "", 0
	}
	frac := func(n int) float64 { return float64(n) / float64(total) }
	switch {
	case frac(arabic) >= 0.5:
		return "ar", frac(arabic)
	case frac(cyrillic) >= 0.5:
		return "ru", frac(cyrillic)
	case frac(hangul) >= 0.5:
		return "ko", frac(hangul)
	case frac(thai) >= 0.5:
		return "th", frac(thai)
	case frac(devanagari) >= 0.5:
		return "hi", frac(devanagari)
	case hiraKata > 0 && frac(hiraKata+han) >= 0.5:
		// Japanese mixes kana with Han; presence of kana distinguishes it from zh.
		return "ja", frac(hiraKata + han)
	case frac(han) >= 0.5:
		return "zh", frac(han)
	}
	return "", 0
}

// detectLatin votes by counting stopword hits per language as whole tokens.
// Confidence is the winner's share of total stopword hits (0 if none hit).
func detectLatin(s string) (string, float64) {
	tokens := tokenize(strings.ToLower(s))
	if len(tokens) == 0 {
		return "", 0
	}
	set := make(map[string]struct{}, len(tokens))
	for _, t := range tokens {
		set[t] = struct{}{}
	}
	scores := make(map[string]int, len(stopwords))
	total := 0
	for lang, words := range stopwords {
		for _, w := range words {
			if _, ok := set[w]; ok {
				scores[lang]++
				total++
			}
		}
	}
	if total == 0 {
		// No stopword signal: default to en with low confidence so callers can
		// treat short/ambiguous Latin text as "probably fine, low evidence".
		return "en", 0.0
	}
	bestLang, best := "", 0
	for lang, sc := range scores {
		if sc > best || (sc == best && lang < bestLang) {
			best, bestLang = sc, lang
		}
	}
	return bestLang, float64(best) / float64(total)
}

// tokenize splits on any non-letter rune, keeping unicode letters (so Turkish
// ı/ş and accented Latin letters survive).
func tokenize(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r)
	})
}
