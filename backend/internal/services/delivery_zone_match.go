package services

import (
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// normalizeZoneToken lowercases, strips diacritics, and removes whitespace
// and hyphens so "C1407-ABC" and "San Martín" compare predictably.
func normalizeZoneToken(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	out, _, err := transform.String(t, s)
	if err != nil {
		out = s
	}
	out = strings.ToLower(out)
	out = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '-' {
			return -1
		}
		return r
	}, out)
	return out
}

// postalMatches matches a configured code against the guest's postal code.
// "1407*" is a prefix wildcard; a bare code must equal the normalized guest
// code OR appear as a delimited token inside it (Argentine "C1407ABC"-style
// codes embed the numeric code). Substring matching of arbitrary codes (the
// old behavior — "140" matched "1407") is gone.
func postalMatches(configured, guest string) bool {
	c := normalizeZoneToken(configured)
	g := normalizeZoneToken(guest)
	if c == "" || g == "" {
		return false
	}
	if strings.HasSuffix(c, "*") {
		prefix := strings.TrimSuffix(c, "*")
		if prefix == "" {
			return false
		}
		if strings.HasPrefix(g, prefix) {
			return true
		}
		// CABA-style "C"+digits: allow the prefix to start after the leading
		// rune when that rune is a letter (e.g. "C1407ABC" matches prefix "1407").
		r0, sz := utf8.DecodeRuneInString(g)
		return len(g) > sz && !unicode.IsDigit(r0) && strings.HasPrefix(g[sz:], prefix)
	}
	if g == c {
		return true
	}
	// Embedded full-code match for letter-wrapped formats (C1407ABC vs 1407
	// is NOT a full-code match; C1407 vs 1407 is). Require the configured
	// code to appear and be followed by a non-digit (or end).
	if idx := strings.Index(g, c); idx >= 0 {
		var afterOK bool
		if idx+len(c) == len(g) {
			afterOK = true
		} else {
			rAfter, _ := utf8.DecodeRuneInString(g[idx+len(c):])
			afterOK = !unicode.IsDigit(rAfter)
		}
		var beforeOK bool
		if idx == 0 {
			beforeOK = true
		} else {
			rBefore, _ := utf8.DecodeLastRuneInString(g[:idx])
			beforeOK = !unicode.IsDigit(rBefore)
		}
		return afterOK && beforeOK
	}
	return false
}

func cityMatches(configured, guest string) bool {
	c := canonicalCityToken(configured)
	g := canonicalCityToken(guest)
	return c != "" && g != "" && c == g
}

// canonicalCityToken folds Argentine CABA aliases onto one token so a zone
// that lists "Buenos Aires" still matches a guest who typed "CABA".
func canonicalCityToken(s string) string {
	n := normalizeZoneToken(s)
	switch n {
	case "caba", "buenosaires", "capitalfederal",
		"ciudadautonomadebuenosaires", "ciudaddebuenosaires":
		return "caba"
	default:
		return n
	}
}

// addressMatchesVenue reports whether the guest is quoting the business's own
// stored street (the live Bodegón dinner-rush: Defensa 1148 / C1065). The
// printed venue street may carry a barrio suffix ("Defensa 1148, San Telmo")
// that the guest form does not.
func addressMatchesVenue(addr database.DeliveryAddress, biz *database.Business) bool {
	if biz == nil || !looksLikeStreet(addr.Street) {
		return false
	}
	if !streetsMatch(addr.Street, biz.Address.Street) {
		return false
	}
	guestPostal := strings.TrimSpace(addr.PostalCode)
	venuePostal := strings.TrimSpace(biz.Address.PostalCode)
	if guestPostal != "" && venuePostal != "" {
		return postalMatches(venuePostal, addr.PostalCode) ||
			postalMatches(guestPostal, biz.Address.PostalCode) ||
			normalizeZoneToken(guestPostal) == normalizeZoneToken(venuePostal)
	}
	return cityMatches(biz.Address.City, addr.City) ||
		cityMatches(biz.Address.State, addr.City) ||
		cityMatches(biz.Address.City, addr.State)
}

// streetTokens splits an address line into normalized comparison tokens. The
// segment after the first comma is a barrio/neighborhood suffix on the printed
// venue address ("Defensa 1148, San Telmo") that the guest form does not carry,
// so it is dropped. Tokens keep their own boundaries — unlike the whitespace-
// stripped normalizeZoneToken form, which silently glues a house number onto
// the street name.
func streetTokens(street string) []string {
	line := street
	if idx := strings.Index(line, ","); idx >= 0 {
		line = line[:idx]
	}
	fields := strings.FieldsFunc(line, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	tokens := make([]string, 0, len(fields))
	for _, field := range fields {
		if token := normalizeZoneToken(field); token != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

func hasDigit(token string) bool {
	for _, r := range token {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// firstNumberToken returns the first token that carries a digit — the house
// number in every ordering this product sees.
func firstNumberToken(tokens []string) (string, bool) {
	for _, token := range tokens {
		if hasDigit(token) {
			return token, true
		}
	}
	return "", false
}

// streetsMatch compares a guest address line against the venue's stored one.
//
// House numbers are compared as whole tokens: "Defensa 114" is a DIFFERENT
// address from "Defensa 1148" (#892). The previous character-prefix compare
// ran on the whitespace-stripped form ("defensa114" vs "defensa1148") and so
// handed a neighbor's quote to anyone one digit short. Word tokens still allow
// one line to extend the other, which is what lets a printed venue street with
// a barrio suffix, or a guest line with a unit suffix, still match.
func streetsMatch(guest, venue string) bool {
	g := streetTokens(guest)
	v := streetTokens(venue)
	if len(g) == 0 || len(v) == 0 {
		return false
	}

	shared := len(g)
	if len(v) < shared {
		shared = len(v)
	}
	for i := 0; i < shared; i++ {
		if g[i] != v[i] {
			return false
		}
	}

	// Both lines must name the same house number. A line with no number at all
	// cannot authorize a match against one that has a number.
	guestNumber, guestHasNumber := firstNumberToken(g)
	venueNumber, venueHasNumber := firstNumberToken(v)
	if !guestHasNumber || !venueHasNumber {
		return false
	}
	return guestNumber == venueNumber
}

// zoneRulesExcludeAddress reports whether the operator's zone data was actually
// evaluated against this address and said no.
//
// Only readable rules count, and only on the dimension they configure: a zone
// listing postal_codes is an authority on a supplied postal code, a zone
// listing cities is an authority on a supplied city. GeoJSON / empty
// boundaries are not authorities on anything — the matcher cannot read them
// (prod 141), which is the whole reason the venue fallback exists.
//
// When a readable rule DOES cover the address the answer is "not excluded"
// even though resolveDeliveryZone missed: a cities-only zone never matches a
// guest who supplied a postal code, and that is a data-shape gap, not an
// operator decision.
func zoneRulesExcludeAddress(zones []DeliveryZoneDTO, address database.DeliveryAddress) bool {
	postalSupplied := strings.TrimSpace(address.PostalCode) != ""
	citySupplied := strings.TrimSpace(address.City) != ""
	postalEvaluated := false
	cityEvaluated := false

	for i := range zones {
		z := zones[i]
		if !z.IsActive || !zoneCoverageReady(z) {
			continue
		}
		var boundaries zoneBoundaries
		if err := json.Unmarshal(z.Boundaries, &boundaries); err != nil {
			continue
		}
		if postalSupplied && len(boundaries.PostalCodes) > 0 {
			for _, code := range boundaries.PostalCodes {
				if postalMatches(code, address.PostalCode) {
					return false
				}
			}
			postalEvaluated = true
		}
		if citySupplied && len(boundaries.Cities) > 0 {
			for _, city := range boundaries.Cities {
				if cityMatches(city, address.City) {
					return false
				}
			}
			cityEvaluated = true
		}
	}

	return postalEvaluated || cityEvaluated
}

// venueFallbackZone picks a fee/ETA source when the guest is quoting the
// venue's OWN address and no zone matched.
//
// It fires only when the operator's zone rules never actually judged the
// address — every readable rule set is silent on it, because the stored
// boundaries are GeoJSON or empty (#892 / prod 141). If a readable rule set DID
// judge the address and excluded it, the operator's decision stands and the
// quote fails closed: the venue does not get to borrow an unrelated zone's fee
// for a postal code it deliberately left out.
//
// When it does fire, the zone is chosen with the SAME comparator
// resolveDeliveryZone publishes — lowest fee, ties to the faster ETA, with the
// minimum order raised to the highest among the candidates — instead of the
// first row the query happened to return.
func venueFallbackZone(zones []DeliveryZoneDTO, address database.DeliveryAddress) *DeliveryZoneDTO {
	if zoneRulesExcludeAddress(zones, address) {
		return nil
	}
	candidates := make([]DeliveryZoneDTO, 0, len(zones))
	for i := range zones {
		if zones[i].IsActive {
			candidates = append(candidates, zones[i])
		}
	}
	return bestZoneByOverlapRules(candidates)
}

// looksLikeStreet requires a non-empty street that contains at least one
// letter and one digit. City or postal tokens alone must not authorize
// delivery; a name-only line ("Main Street") or a bare number is not enough.
func looksLikeStreet(street string) bool {
	var hasLetter, hasDigit bool
	for _, r := range strings.TrimSpace(street) {
		if unicode.IsLetter(r) {
			hasLetter = true
		} else if unicode.IsDigit(r) {
			hasDigit = true
		}
		if hasLetter && hasDigit {
			return true
		}
	}
	return false
}

// usCityRequiredState is a tiny contradiction map, not a gazetteer.
// Keys and values are normalizeZoneToken forms (spaces/hyphens stripped).
var usCityRequiredState = map[string]string{
	"newyork": "ny",
}

func normalizeUSState(state string) string {
	n := normalizeZoneToken(state)
	n = strings.Map(func(r rune) rune {
		if r == '.' {
			return -1
		}
		return r
	}, n)
	if n == "newyork" {
		return "ny"
	}
	return n
}

func isUSCountry(country string) bool {
	c := normalizeZoneToken(country)
	return c == "" || c == "us" || c == "usa" || c == "unitedstates"
}

// cityStateContradicts rejects known city/state mismatches when the guest
// supplies a US state (e.g. New York city + CA). Unknown cities are left
// to zone postal/city matching.
func cityStateContradicts(address database.DeliveryAddress) bool {
	if strings.TrimSpace(address.State) == "" || !isUSCountry(address.Country) {
		return false
	}
	required, ok := usCityRequiredState[normalizeZoneToken(address.City)]
	if !ok {
		return false
	}
	return normalizeUSState(address.State) != required
}

// zoneCoverageReady reports whether an active zone can ever match a guest
// address under zoneMatchesAddress. GeoJSON polygons and empty objects cannot.
func zoneCoverageReady(z DeliveryZoneDTO) bool {
	if !z.IsActive {
		return false
	}
	raw := z.Boundaries
	if len(raw) == 0 || string(raw) == "{}" || string(raw) == "null" {
		return false
	}
	var boundaries zoneBoundaries
	if err := json.Unmarshal(raw, &boundaries); err != nil {
		return false
	}
	return len(boundaries.PostalCodes) > 0 || len(boundaries.Cities) > 0
}

func zoneMatchesAddress(z DeliveryZoneDTO, address database.DeliveryAddress) bool {
	if !z.IsActive {
		return false
	}
	if !looksLikeStreet(address.Street) {
		return false
	}
	if cityStateContradicts(address) {
		return false
	}
	raw := z.Boundaries
	if len(raw) == 0 || string(raw) == "{}" || string(raw) == "null" {
		// An empty boundary set matches NOTHING (R16). Incomplete zones are
		// flagged in the editor and skipped here.
		return false
	}
	// Note: legacy boundary keys (states, countries, keywords) are deliberately
	// ignored — the zone editor has only ever written postal_codes and cities.
	var boundaries zoneBoundaries
	if err := json.Unmarshal(raw, &boundaries); err != nil {
		return false
	}
	if len(boundaries.PostalCodes) == 0 && len(boundaries.Cities) == 0 {
		return false
	}
	postalSupplied := strings.TrimSpace(address.PostalCode) != ""
	for _, code := range boundaries.PostalCodes {
		if postalMatches(code, address.PostalCode) {
			return true
		}
	}
	// A supplied postal that does not match the zone cannot fall back to city.
	if postalSupplied {
		return false
	}
	for _, city := range boundaries.Cities {
		if cityMatches(city, address.City) {
			return true
		}
	}
	return false
}

// resolveDeliveryZone applies the published overlap rules across all matching
// active zones: lowest fee wins (ties break to faster ETA); the minimum order
// is the HIGHEST minimum among matches (Flipdish rule); the returned zone is
// the fee winner with its MinimumOrderAmount raised to that highest minimum.
func resolveDeliveryZone(zones []DeliveryZoneDTO, address database.DeliveryAddress) *DeliveryZoneDTO {
	candidates := make([]DeliveryZoneDTO, 0, len(zones))
	for i := range zones {
		if zoneMatchesAddress(zones[i], address) {
			candidates = append(candidates, zones[i])
		}
	}
	return bestZoneByOverlapRules(candidates)
}

// bestZoneByOverlapRules is the single implementation of the published overlap
// rules, shared by the normal matcher and the venue own-address fallback so
// neither path can drift into "first row wins".
func bestZoneByOverlapRules(candidates []DeliveryZoneDTO) *DeliveryZoneDTO {
	var winner *DeliveryZoneDTO
	highestMinimum := 0.0
	for i := range candidates {
		z := candidates[i]
		if z.MinimumOrderAmount > highestMinimum {
			highestMinimum = z.MinimumOrderAmount
		}
		if winner == nil ||
			z.DeliveryFee < winner.DeliveryFee ||
			(z.DeliveryFee == winner.DeliveryFee && z.EstimatedTime < winner.EstimatedTime) {
			copied := z
			winner = &copied
		}
	}
	if winner != nil {
		winner.MinimumOrderAmount = highestMinimum
	}
	return winner
}
