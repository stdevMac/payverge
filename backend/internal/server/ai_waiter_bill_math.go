package server

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/services"
)

// waiterBillMoney is the server-owned numeric shape of the table's open bill.
// TotalCents is the DB wire shape (int64 cents); Currency is the business
// currency code. Only the handler fills it, from the same bill row that builds
// the human bill summary — the model never contributes a number here.
type waiterBillMoney struct {
	TotalCents int64
	Currency   string
}

// waiterBillMathAskResult describes the check math a guest asked for
// ("dividí entre 2", "add a 10% tip"). Zero values mean "not asked".
type waiterBillMathAskResult struct {
	SplitCount int
	TipPercent float64
	TipAsked   bool
}

func (ask waiterBillMathAskResult) requested() bool {
	return ask.SplitCount > 1 || ask.TipAsked
}

// Locale families follow the services waiter vocabularies: the session locale
// is scanned first and English always as well, because guests mix languages on
// a Spanish (or any other) session.
var waiterSplitKeywords = map[string][]string{
	"en": {"split the bill", "split the check", "split the tab", "split it", "split this", "separate checks", "separate check", "divide the bill", "divide the check", "ways", "go dutch", "in half", "50/50"},
	"es": {"dividir la cuenta", "dividi", "dividí", "divide la cuenta", "dividimos", "repartir la cuenta", "separar la cuenta", "cuentas separadas", "a medias", "mitades", "por mitades"},
}

var waiterTipKeywords = map[string][]string{
	"en": {"tip", "tips", "gratuity", "service charge"},
	"es": {"propina", "propinas"},
}

var (
	waiterPercentPattern    = regexp.MustCompile(`(\d{1,3})(?:[.,](\d{1,2}))?\s*(?:%|por ciento|percent|pct)`)
	waiterSplitLeadPattern  = regexp.MustCompile(`(?:entre|between|among|dividir en|dividi en|dividí en|split|ways?)\s*(\d{1,2})`)
	waiterSplitTrailPattern = regexp.MustCompile(`(\d{1,2})\s*(?:ways|way|personas|persona|people|person|partes|comensales|guests)`)
)

// waiterBillMathAsk parses the check math out of a guest message. It never
// looks at the menu or the model — only at what the guest literally asked for.
func waiterBillMathAsk(locale, userMessage string) waiterBillMathAskResult {
	lower := strings.ToLower(strings.TrimSpace(userMessage))
	if lower == "" {
		return waiterBillMathAskResult{}
	}
	ask := waiterBillMathAskResult{}
	if waiterBillVocabularyMatch(waiterTipKeywords, locale, lower) {
		ask.TipAsked = true
		ask.TipPercent = waiterParsePercent(lower)
	}
	if waiterBillVocabularyMatch(waiterSplitKeywords, locale, lower) {
		ask.SplitCount = waiterParseSplitCount(lower)
	}
	return ask
}

func waiterBillVocabularyMatch(table map[string][]string, locale, lower string) bool {
	if waiterBillKeywordsMatch(table[waiterBillVocabularyFamily(locale)], lower) {
		return true
	}
	return waiterBillKeywordsMatch(table["en"], lower)
}

func waiterBillVocabularyFamily(locale string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "es") {
		return "es"
	}
	return "en"
}

func waiterBillKeywordsMatch(keywords []string, lower string) bool {
	for _, keyword := range keywords {
		if keyword == "" {
			continue
		}
		// Short keywords ("tip", "ways") must match as words so "tips included
		// in multiple" or an item named "Tipsy Cake" never reads as a tip ask.
		if len(keyword) <= 5 && !strings.ContainsAny(keyword, " /") {
			if containsWaiterPhrase(lower, keyword) {
				return true
			}
			continue
		}
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

func waiterParsePercent(lower string) float64 {
	match := waiterPercentPattern.FindStringSubmatch(lower)
	if match == nil {
		return 0
	}
	raw := match[1]
	if match[2] != "" {
		raw += "." + match[2]
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 || value > 100 {
		return 0
	}
	return value
}

func waiterParseSplitCount(lower string) int {
	// Strip percentages first so "split between 2 and add 10% tip" never reads
	// the tip percentage as the number of guests.
	stripped := waiterPercentPattern.ReplaceAllString(lower, " ")
	if match := waiterSplitTrailPattern.FindStringSubmatch(stripped); match != nil {
		if count := waiterBoundedSplitCount(match[1]); count > 0 {
			return count
		}
	}
	if match := waiterSplitLeadPattern.FindStringSubmatch(stripped); match != nil {
		if count := waiterBoundedSplitCount(match[1]); count > 0 {
			return count
		}
	}
	if strings.Contains(stripped, "a medias") || strings.Contains(stripped, "mitades") ||
		strings.Contains(stripped, "in half") || strings.Contains(stripped, "50 50") ||
		strings.Contains(stripped, "50/50") {
		return 2
	}
	return 0
}

func waiterBoundedSplitCount(raw string) int {
	count, err := strconv.Atoi(raw)
	if err != nil || count < 2 || count > 20 {
		return 0
	}
	return count
}

// waiterSplitShareCents divides an int64-cent total into equal shares. When the
// total does not divide evenly the share is rounded UP so the shares always
// cover the bill (the waiter never quotes a figure that leaves the venue short);
// the boolean reports whether the split was exact so the answer can say so.
func waiterSplitShareCents(totalCents int64, parts int) (int64, bool) {
	if totalCents <= 0 || parts < 2 {
		return 0, false
	}
	divisor := int64(parts)
	if totalCents%divisor == 0 {
		return totalCents / divisor, true
	}
	return (totalCents + divisor - 1) / divisor, false
}

func waiterTipCents(totalCents int64, percent float64) int64 {
	if totalCents <= 0 || percent <= 0 {
		return 0
	}
	return int64(math.Round(float64(totalCents) * percent / 100))
}

func waiterMoneyLabel(cents int64, currency string) string {
	return aiWaiterCurrencySymbol(currency) + fmt.Sprintf("%.2f", float64(cents)/100.0)
}

func waiterPercentLabel(percent float64) string {
	return strconv.FormatFloat(percent, 'f', -1, 64)
}

// waiterBillAnswerWithMath appends the requested split / tip arithmetic to the
// server-owned bill summary. Every number comes from the bill row's int64-cent
// total, so the waiter can close and split a check instead of answering
// "I couldn't find that on the menu" (issue 943). With no open bill or no
// numeric total, the plain bill answer stands — no invented figures.
func waiterBillAnswerWithMath(locale string, in WaiterFinalizeInput, copy waiterV2Copy) string {
	answer := services.WaiterBillAnswer(locale, in.Visit)
	ask := waiterBillMathAsk(locale, in.UserMessage)
	if !ask.requested() || !in.Visit.HasOpenBill || in.Bill.TotalCents <= 0 {
		return answer
	}
	currency := in.Bill.Currency
	lines := []string{answer}
	splitBase := in.Bill.TotalCents
	if ask.TipPercent > 0 {
		tip := waiterTipCents(in.Bill.TotalCents, ask.TipPercent)
		splitBase = in.Bill.TotalCents + tip
		line := strings.ReplaceAll(copy.billTip, waiterPercentPlaceholder, waiterPercentLabel(ask.TipPercent))
		line = strings.ReplaceAll(line, waiterAmountPlaceholder, waiterMoneyLabel(tip, currency))
		line = strings.ReplaceAll(line, waiterTotalPlaceholder, waiterMoneyLabel(splitBase, currency))
		lines = append(lines, line)
	}
	if ask.SplitCount > 1 {
		share, exact := waiterSplitShareCents(splitBase, ask.SplitCount)
		shareLabel := waiterMoneyLabel(share, currency)
		if !exact {
			shareLabel = "≈ " + shareLabel
		}
		line := strings.ReplaceAll(copy.billSplit, waiterCountPlaceholder, strconv.Itoa(ask.SplitCount))
		line = strings.ReplaceAll(line, waiterAmountPlaceholder, shareLabel)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
