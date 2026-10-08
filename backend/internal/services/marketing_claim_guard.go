package services

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Marketing claim guard — deterministic, server-side defense-in-depth on
// AI-generated public marketing captions, applied AFTER the model returns and
// BEFORE the caption is cached or returned to the operator for publish.
//
// Threat: the model turns an internal insight (e.g. "slowest window Tue 5–7pm")
// into a public factual claim ("We're open from 8am–9am every Saturday") that
// is not grounded in the business record. A false opening-hours / price /
// offer claim published under a restaurant's name is real-world harm.
//
// Response: extract claim-like patterns (hours, prices, offers, phones,
// addresses), validate each against MarketingClaimFacts, and either strip the
// offending sentence or reject the caption entirely. Never rely on prompt
// instructions alone for a claim that leaves the platform.

// MarketingHourWindow is one open/close range for a day of week.
// DayOfWeek uses Go's time.Weekday: 0=Sunday … 6=Saturday.
type MarketingHourWindow struct {
	DayOfWeek int
	OpenTime  string // "09:00" 24h
	CloseTime string // "17:00" 24h
	IsClosed  bool
}

// MarketingOfferFact is an active offer the caption may truthfully reference.
type MarketingOfferFact struct {
	Name          string
	DiscountType  string // "percentage" | "fixed"
	DiscountValue float64
}

// MarketingClaimFacts is the ground-truth snapshot used to validate AI copy.
type MarketingClaimFacts struct {
	Phone   string
	Street  string
	City    string
	State   string
	Postal  string
	Country string
	Hours   []MarketingHourWindow
	Prices  []float64
	Offers  []MarketingOfferFact
}

// MarketingClaimViolation describes one ungrounded factual claim in copy.
type MarketingClaimViolation struct {
	Kind   string // hours | price | offer | address | phone
	Claim  string
	Reason string
}

// BuildMarketingClaimFacts assembles a validation snapshot from business data.
// Only active offers are included. Callers supply menu prices when available.
func BuildMarketingClaimFacts(
	phone string,
	addr database.BusinessAddress,
	hours []database.BusinessOperatingHours,
	offers []database.Offer,
	prices []float64,
) MarketingClaimFacts {
	facts := MarketingClaimFacts{
		Phone:   strings.TrimSpace(phone),
		Street:  strings.TrimSpace(addr.Street),
		City:    strings.TrimSpace(addr.City),
		State:   strings.TrimSpace(addr.State),
		Postal:  strings.TrimSpace(addr.PostalCode),
		Country: strings.TrimSpace(addr.Country),
		Prices:  append([]float64(nil), prices...),
	}
	for _, h := range hours {
		facts.Hours = append(facts.Hours, MarketingHourWindow{
			DayOfWeek: h.DayOfWeek,
			OpenTime:  strings.TrimSpace(h.OpenTime),
			CloseTime: strings.TrimSpace(h.CloseTime),
			IsClosed:  h.IsClosed,
		})
	}
	for _, o := range offers {
		if !o.IsActive {
			continue
		}
		facts.Offers = append(facts.Offers, MarketingOfferFact{
			Name:          strings.TrimSpace(o.Name),
			DiscountType:  strings.TrimSpace(o.DiscountType),
			DiscountValue: o.DiscountValue,
		})
	}
	return facts
}

// GuardMarketingClaims returns ok=true when caption has no false factual claims.
// On mismatch, ok=false and violations describe why — the caller must not publish.
func GuardMarketingClaims(caption string, facts MarketingClaimFacts) (ok bool, violations []MarketingClaimViolation) {
	caption = strings.TrimSpace(caption)
	if caption == "" {
		return true, nil
	}
	violations = append(violations, detectHoursViolations(caption, facts)...)
	violations = append(violations, detectPriceViolations(caption, facts)...)
	violations = append(violations, detectOfferViolations(caption, facts)...)
	violations = append(violations, detectPhoneViolations(caption, facts)...)
	violations = append(violations, detectAddressViolations(caption, facts)...)
	if len(violations) > 0 {
		return false, violations
	}
	return true, nil
}

// SanitizeMarketingCaptionClaims drops sentences that contain ungrounded
// claims. Returns the cleaned caption and ok=true when remaining copy is
// non-empty and claim-safe. When the whole caption is unpublishable, ok=false
// and cleaned is empty.
func SanitizeMarketingCaptionClaims(caption string, facts MarketingClaimFacts) (cleaned string, ok bool) {
	caption = strings.TrimSpace(caption)
	if caption == "" {
		return "", true
	}
	if ok, _ := GuardMarketingClaims(caption, facts); ok {
		return caption, true
	}

	sentences := splitCaptionSentences(caption)
	kept := make([]string, 0, len(sentences))
	for _, s := range sentences {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if ok, _ := GuardMarketingClaims(s, facts); ok {
			kept = append(kept, s)
		}
	}
	if len(kept) == 0 {
		return "", false
	}
	out := strings.Join(kept, " ")
	// Re-validate the reassembled caption (cross-sentence patterns shouldn't
	// reappear after per-sentence strip, but fail closed if they do).
	if ok, _ := GuardMarketingClaims(out, facts); !ok {
		return "", false
	}
	return strings.TrimSpace(out), true
}

// ---------------------------------------------------------------------------
// Hours
// ---------------------------------------------------------------------------

// Matches opening-hours style claims, including the audited fixture
// "We're open from 8am–9am every Saturday".
var (
	marketingHoursOpenPattern = regexp.MustCompile(`(?i)(?:we(?:'re|\s+are)\s+open|open(?:ing)?(?:\s+hours?)?|abrimos|abierto(?:s)?)\s+(?:from\s+|de\s+)?` +
		`(\d{1,2}(?::\d{2})?\s*(?:a\.?m\.?|p\.?m\.?)?)\s*(?:[-–—]|to|a|until|hasta)\s*` +
		`(\d{1,2}(?::\d{2})?\s*(?:a\.?m\.?|p\.?m\.?)?)` +
		`(?:\s*(?:every|on|los|las|el|todos(?:\s+los)?)\s+([a-záéíóúñ]+))?`)

	// Bare day + time-range without "open" verb (e.g. "Saturday 8am–9am").
	marketingHoursDayRangePattern = regexp.MustCompile(`(?i)\b(` + marketingDayNameAlternation + `)\b\s+` +
		`(\d{1,2}(?::\d{2})?\s*(?:a\.?m\.?|p\.?m\.?)?)\s*(?:[-–—]|to|a|until|hasta)\s*` +
		`(\d{1,2}(?::\d{2})?\s*(?:a\.?m\.?|p\.?m\.?)?)`)

	marketingDayNameAlternation = `sunday|monday|tuesday|wednesday|thursday|friday|saturday|` +
		`domingo|lunes|martes|mi[eé]rcoles|jueves|viernes|s[aá]bado`
)

var marketingDayNameToWeekday = map[string]int{
	"sunday": 0, "domingo": 0,
	"monday": 1, "lunes": 1,
	"tuesday": 2, "martes": 2,
	"wednesday": 3, "miercoles": 3, "miércoles": 3,
	"thursday": 4, "jueves": 4,
	"friday": 5, "viernes": 5,
	"saturday": 6, "sabado": 6, "sábado": 6,
}

func detectHoursViolations(caption string, facts MarketingClaimFacts) []MarketingClaimViolation {
	var out []MarketingClaimViolation
	seen := map[string]struct{}{}

	add := func(claim, reason string) {
		key := claim + "|" + reason
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, MarketingClaimViolation{Kind: "hours", Claim: claim, Reason: reason})
	}

	for _, m := range marketingHoursOpenPattern.FindAllStringSubmatch(caption, -1) {
		claim := strings.TrimSpace(m[0])
		openRaw, closeRaw := m[1], m[2]
		dayName := ""
		if len(m) > 3 {
			dayName = m[3]
		}
		if !hoursClaimMatches(facts, openRaw, closeRaw, dayName) {
			add(claim, "opening hours do not match business record")
		}
	}

	for _, m := range marketingHoursDayRangePattern.FindAllStringSubmatch(caption, -1) {
		claim := strings.TrimSpace(m[0])
		// Skip if already covered by the open-pattern (subset of same span).
		already := false
		for _, v := range out {
			if strings.Contains(strings.ToLower(v.Claim), strings.ToLower(claim)) ||
				strings.Contains(strings.ToLower(claim), strings.ToLower(v.Claim)) {
				already = true
				break
			}
		}
		if already {
			continue
		}
		dayName, openRaw, closeRaw := m[1], m[2], m[3]
		if !hoursClaimMatches(facts, openRaw, closeRaw, dayName) {
			add(claim, "opening hours do not match business record")
		}
	}
	return out
}

func hoursClaimMatches(facts MarketingClaimFacts, openRaw, closeRaw, dayName string) bool {
	openMin, okOpen := parseClockToMinutes(openRaw)
	closeMin, okClose := parseClockToMinutes(closeRaw)
	if !okOpen || !okClose {
		// Unparseable clock → treat as non-claim (avoid false positives).
		return true
	}
	if len(facts.Hours) == 0 {
		return false
	}

	targetDay := -1
	if dayName != "" {
		key := normalizeDayName(dayName)
		if d, ok := marketingDayNameToWeekday[key]; ok {
			targetDay = d
		}
	}

	for _, h := range facts.Hours {
		if targetDay >= 0 && h.DayOfWeek != targetDay {
			continue
		}
		if h.IsClosed {
			if targetDay >= 0 {
				return false
			}
			continue
		}
		hOpen, ok1 := parseClockToMinutes(h.OpenTime)
		hClose, ok2 := parseClockToMinutes(h.CloseTime)
		if !ok1 || !ok2 {
			continue
		}
		if hOpen == openMin && hClose == closeMin {
			return true
		}
	}
	// Day-specific claim that never matched → false. Day-unspecified claim
	// that never matched any window → false.
	return false
}

func normalizeDayName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	// Strip trailing plural 's' for "Saturdays".
	s = strings.TrimSuffix(s, "s")
	// Fold accented Spanish day names into map keys.
	replacer := strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u",
	)
	folded := replacer.Replace(s)
	// Keep both accented and folded lookups possible via map.
	if _, ok := marketingDayNameToWeekday[s]; ok {
		return s
	}
	return folded
}

// parseClockToMinutes parses "8am", "8:00 am", "08:00", "20:00", "9pm" → minutes.
func parseClockToMinutes(raw string) (int, bool) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return 0, false
	}
	s = strings.ReplaceAll(s, ".", "")
	s = strings.Join(strings.Fields(s), "")

	ampm := 0 // 0 unknown, 1 am, 2 pm
	if strings.HasSuffix(s, "am") {
		ampm = 1
		s = strings.TrimSuffix(s, "am")
	} else if strings.HasSuffix(s, "pm") {
		ampm = 2
		s = strings.TrimSuffix(s, "pm")
	}

	var hour, minute int
	if strings.Contains(s, ":") {
		parts := strings.SplitN(s, ":", 2)
		h, err1 := strconv.Atoi(parts[0])
		m, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			return 0, false
		}
		hour, minute = h, m
	} else {
		h, err := strconv.Atoi(s)
		if err != nil {
			return 0, false
		}
		hour = h
	}
	if minute < 0 || minute > 59 {
		return 0, false
	}
	switch ampm {
	case 1: // am
		if hour == 12 {
			hour = 0
		}
		if hour < 0 || hour > 11 {
			return 0, false
		}
	case 2: // pm
		if hour == 12 {
			// noon
		} else if hour >= 1 && hour <= 11 {
			hour += 12
		} else {
			return 0, false
		}
	default: // 24h
		if hour < 0 || hour > 23 {
			return 0, false
		}
	}
	return hour*60 + minute, true
}

// ---------------------------------------------------------------------------
// Prices
// ---------------------------------------------------------------------------

// Currency-marked amounts only — bare integers like "18 times" must not match.
var marketingPricePattern = regexp.MustCompile(`(?i)(?:` +
	`(?:[$€£]\s*\d{1,6}(?:[.,]\d{1,2})?)|` + // $12 / €12.50
	`(?:\d{1,6}(?:[.,]\d{1,2})?\s*[$€£])|` + // 12$ / 12,50€
	`(?:(?:usd|eur|ars|gbp)\s*\d{1,6}(?:[.,]\d{1,2})?)|` +
	`(?:\d{1,6}(?:[.,]\d{1,2})?\s*(?:usd|eur|ars|gbp))` +
	`)`)

func detectPriceViolations(caption string, facts MarketingClaimFacts) []MarketingClaimViolation {
	var out []MarketingClaimViolation
	matches := marketingPricePattern.FindAllString(caption, -1)
	if len(matches) == 0 {
		return nil
	}
	for _, m := range matches {
		amount, ok := parseMoneyAmount(m)
		if !ok {
			continue
		}
		if !priceInFacts(amount, facts.Prices) {
			reason := "price is not on the business menu"
			if len(facts.Prices) == 0 {
				reason = "price claim with no known menu prices"
			}
			out = append(out, MarketingClaimViolation{
				Kind:   "price",
				Claim:  strings.TrimSpace(m),
				Reason: reason,
			})
		}
	}
	return out
}

func parseMoneyAmount(raw string) (float64, bool) {
	s := strings.ToLower(strings.TrimSpace(raw))
	// Keep digits, comma, dot.
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) || r == '.' || r == ',' {
			b.WriteRune(r)
		}
	}
	num := b.String()
	if num == "" {
		return 0, false
	}
	// European "12,50" → 12.50; "1.250,50" is rare in captions — take last
	// separator as decimal when both present.
	if strings.Contains(num, ",") && strings.Contains(num, ".") {
		if strings.LastIndex(num, ",") > strings.LastIndex(num, ".") {
			num = strings.ReplaceAll(num, ".", "")
			num = strings.ReplaceAll(num, ",", ".")
		} else {
			num = strings.ReplaceAll(num, ",", "")
		}
	} else if strings.Contains(num, ",") {
		// Single comma: decimal if 1–2 digits after, else thousands.
		parts := strings.Split(num, ",")
		if len(parts) == 2 && len(parts[1]) <= 2 {
			num = parts[0] + "." + parts[1]
		} else {
			num = strings.ReplaceAll(num, ",", "")
		}
	}
	v, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func priceInFacts(amount float64, prices []float64) bool {
	const eps = 0.009 // half-cent tolerance after float parse
	for _, p := range prices {
		if absFloat(p-amount) <= eps {
			return true
		}
	}
	return false
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// ---------------------------------------------------------------------------
// Offers / discounts
// ---------------------------------------------------------------------------

var (
	marketingPercentOffPattern = regexp.MustCompile(`(?i)(\d{1,3})\s*%\s*(?:off|descuento|dto\.?|de\s+descuento)`)
	marketingBOGOPattern       = regexp.MustCompile(`(?i)\b(?:buy\s+one\s+get\s+one|bogo|2x1|dos\s+por\s+uno)\b`)
)

func detectOfferViolations(caption string, facts MarketingClaimFacts) []MarketingClaimViolation {
	var out []MarketingClaimViolation

	for _, m := range marketingPercentOffPattern.FindAllStringSubmatch(caption, -1) {
		claim := strings.TrimSpace(m[0])
		pct, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			continue
		}
		if !offerPercentInFacts(pct, facts.Offers) {
			reason := "discount percent does not match an active offer"
			if len(facts.Offers) == 0 {
				reason = "discount claim with no active offers"
			}
			out = append(out, MarketingClaimViolation{
				Kind:   "offer",
				Claim:  claim,
				Reason: reason,
			})
		}
	}

	if marketingBOGOPattern.MatchString(caption) {
		if !offerNameHintsBOGO(facts.Offers) {
			out = append(out, MarketingClaimViolation{
				Kind:   "offer",
				Claim:  marketingBOGOPattern.FindString(caption),
				Reason: "BOGO-style claim with no matching active offer",
			})
		}
	}
	return out
}

func offerPercentInFacts(pct float64, offers []MarketingOfferFact) bool {
	const eps = 0.01
	for _, o := range offers {
		if !strings.EqualFold(o.DiscountType, "percentage") {
			continue
		}
		if absFloat(o.DiscountValue-pct) <= eps {
			return true
		}
	}
	return false
}

func offerNameHintsBOGO(offers []MarketingOfferFact) bool {
	for _, o := range offers {
		name := strings.ToLower(o.Name)
		if strings.Contains(name, "bogo") ||
			strings.Contains(name, "buy one") ||
			strings.Contains(name, "2x1") ||
			strings.Contains(name, "two for one") ||
			strings.Contains(name, "dos por uno") {
			return true
		}
		// 100% off second item sometimes stored as percentage 50 (half price BOGO).
		if strings.EqualFold(o.DiscountType, "percentage") && absFloat(o.DiscountValue-50) < 0.01 &&
			(strings.Contains(name, "second") || strings.Contains(name, "bogo")) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Phone
// ---------------------------------------------------------------------------

// Broad phone-like sequences: optional +, then 7–15 digits with separators.
var marketingPhonePattern = regexp.MustCompile(`(?i)(?:\+\d{1,3}[\s\-.]*)?(?:\(?\d{2,4}\)?[\s\-.]*){2,5}\d{2,4}`)

func detectPhoneViolations(caption string, facts MarketingClaimFacts) []MarketingClaimViolation {
	matches := marketingPhonePattern.FindAllString(caption, -1)
	if len(matches) == 0 {
		return nil
	}
	factDigits := digitsOnly(facts.Phone)
	var out []MarketingClaimViolation
	for _, m := range matches {
		claim := strings.TrimSpace(m)
		// Skip short numeric fragments that are clearly not phones (years, etc.).
		d := digitsOnly(claim)
		if len(d) < 7 || len(d) > 15 {
			continue
		}
		// Avoid treating plain prices/percents already handled elsewhere: if the
		// match is only digits with a leading currency we already require $ etc.
		if looksLikeYearOrCount(claim, d) {
			continue
		}
		if factDigits == "" || !phoneDigitsMatch(d, factDigits) {
			reason := "phone number does not match business record"
			if factDigits == "" {
				reason = "phone claim with no business phone on record"
			}
			out = append(out, MarketingClaimViolation{
				Kind:   "phone",
				Claim:  claim,
				Reason: reason,
			})
		}
	}
	return out
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func phoneDigitsMatch(claim, fact string) bool {
	if claim == fact {
		return true
	}
	// Allow claim to omit country code when fact has it (or vice versa).
	if strings.HasSuffix(fact, claim) || strings.HasSuffix(claim, fact) {
		return true
	}
	// US: compare last 10 digits.
	if len(claim) >= 10 && len(fact) >= 10 {
		return claim[len(claim)-10:] == fact[len(fact)-10:]
	}
	return false
}

func looksLikeYearOrCount(raw, digits string) bool {
	// Pure 4-digit years, or matches that have no separators and are short.
	if len(digits) <= 4 {
		return true
	}
	// "18 times" style — the phone regex shouldn't match, but guard anyway.
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "time") || strings.Contains(lower, "order") {
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Address
// ---------------------------------------------------------------------------

var (
	marketingAddressCuePattern = regexp.MustCompile(`(?i)\b(?:find us at|visit us at|located at|we're at|we are at|en\s+[^,]{3,40},\s*[a-záéíóúñ]+|at\s+\d{1,5}\s+[a-z][a-z0-9 .'-]{2,40})`)
	// Street-number + word after an address cue is extracted more carefully below.
	marketingStreetNumberPattern = regexp.MustCompile(`(?i)\b(\d{1,5})\s+([A-Za-zÁÉÍÓÚÑáéíóúñ][A-Za-zÁÉÍÓÚÑáéíóúñ0-9 .'-]{1,40})`)
)

func detectAddressViolations(caption string, facts MarketingClaimFacts) []MarketingClaimViolation {
	// Only evaluate when the copy frames something as a location claim.
	if !marketingAddressCuePattern.MatchString(caption) &&
		!regexp.MustCompile(`(?i)\b(?:find us|visit us|located|direcci[oó]n)\b`).MatchString(caption) {
		return nil
	}

	factStreet := strings.ToLower(strings.TrimSpace(facts.Street))
	factCity := strings.ToLower(strings.TrimSpace(facts.City))
	if factStreet == "" && factCity == "" {
		// Fail closed on location cues with no address on file when a street
		// number is asserted.
		if m := marketingStreetNumberPattern.FindString(caption); m != "" {
			return []MarketingClaimViolation{{
				Kind:   "address",
				Claim:  strings.TrimSpace(m),
				Reason: "address claim with no business address on record",
			}}
		}
		return nil
	}

	lower := strings.ToLower(caption)
	// If the caption contains the real street or city, accept.
	if factStreet != "" && strings.Contains(lower, factStreet) {
		return nil
	}
	if factCity != "" && strings.Contains(lower, factCity) {
		// City alone is weak; if a different street number is present, reject.
		if m := marketingStreetNumberPattern.FindStringSubmatch(caption); len(m) > 0 {
			claimed := strings.ToLower(strings.TrimSpace(m[0]))
			if factStreet != "" && !strings.Contains(claimed, strings.Fields(factStreet)[0]) {
				// Street number in claim doesn't align with fact street start.
				factNum := ""
				if fields := strings.Fields(factStreet); len(fields) > 0 {
					factNum = fields[0]
				}
				if factNum != "" && !strings.Contains(claimed, factNum) {
					return []MarketingClaimViolation{{
						Kind:   "address",
						Claim:  strings.TrimSpace(m[0]),
						Reason: "address does not match business record",
					}}
				}
			}
		}
		return nil
	}

	// Location cue present, but neither street nor city appears → invented.
	claim := marketingAddressCuePattern.FindString(caption)
	if claim == "" {
		if m := marketingStreetNumberPattern.FindString(caption); m != "" {
			claim = m
		} else {
			claim = "location claim"
		}
	}
	return []MarketingClaimViolation{{
		Kind:   "address",
		Claim:  strings.TrimSpace(claim),
		Reason: "address does not match business record",
	}}
}

// ---------------------------------------------------------------------------
// Sentence split (for sanitize)
// ---------------------------------------------------------------------------

func splitCaptionSentences(caption string) []string {
	// Split on . ! ? while keeping hashtags attached to the preceding sentence.
	var sentences []string
	var b strings.Builder
	runes := []rune(caption)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		b.WriteRune(r)
		if r == '.' || r == '!' || r == '?' {
			// Don't split on decimal points inside prices ($9.50).
			if r == '.' && i > 0 && i+1 < len(runes) &&
				unicode.IsDigit(runes[i-1]) && unicode.IsDigit(runes[i+1]) {
				continue
			}
			sentences = append(sentences, b.String())
			b.Reset()
		}
	}
	if b.Len() > 0 {
		sentences = append(sentences, b.String())
	}
	return sentences
}

// FormatMarketingClaimViolations is a compact log helper.
func FormatMarketingClaimViolations(v []MarketingClaimViolation) string {
	if len(v) == 0 {
		return ""
	}
	parts := make([]string, 0, len(v))
	for _, x := range v {
		parts = append(parts, fmt.Sprintf("%s:%q (%s)", x.Kind, x.Claim, x.Reason))
	}
	return strings.Join(parts, "; ")
}
