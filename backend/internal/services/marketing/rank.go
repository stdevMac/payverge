package marketing

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// imageRankBoost is added when a suggestion already has a free photo, so
// photo-ready posts surface above same-signal plays that need "add a photo".
const imageRankBoost = 5.0

// applyImageBoost raises rank for suggestions that already carry a usable image
// and records a structured why_factor. Pure — safe for unit tests.
func applyImageBoost(s *CampaignSuggestion) {
	if s == nil || s.ImageURL == "" {
		return
	}
	s.Rank += imageRankBoost
	// Numeric flag only — FE owns "has a photo ready" copy (L4-22).
	s.WhyFactors = append(s.WhyFactors, WhyFactor{
		Key:    "photo_ready",
		Value:  "1",
		Weight: imageRankBoost,
	})
}

// isPlainMenuItemID reports whether targetID refers to a menu item (not a
// bundle:/offer: promo target). Empty IDs are not menu items.
func isPlainMenuItemID(targetID string) bool {
	if targetID == "" {
		return false
	}
	return !strings.Contains(targetID, ":")
}

// shouldSuppressUnavailable returns true when a menu-item play targets a dish
// that the active menu marks unavailable — either explicit is_available:false
// (manual 86) or inventory zero-stock (see menuItemMetaIndex). Missing menu
// meta does not suppress (no data ≠ out of stock). Bundle/offer targets never
// use this path.
func shouldSuppressUnavailable(s *CampaignSuggestion, meta menuMetaIndex) bool {
	if s == nil {
		return false
	}
	if isPlainMenuItemID(s.TargetItemID) {
		item, ok := meta.byID[s.TargetItemID]
		if ok && item.Seen && !item.Available {
			return true
		}
	}
	if mid, ok := offerLinkedMenuItemID(s); ok {
		item, found := meta.byID[mid]
		if found && item.Seen && !item.Available {
			return true
		}
	}
	return false
}

// shouldSuppressUnmakeable returns true when a play targets a dish the kitchen
// cannot produce (recipe OOS / BlocksSale) — the same set AI Waiter uses via
// database.UnrecommendableMenuItemIDs. Catches warn-mode stockouts where the
// dish is still marked available on the menu.
func shouldSuppressUnmakeable(s *CampaignSuggestion, meta menuMetaIndex, unmakeable map[string]bool) bool {
	if s == nil || len(unmakeable) == 0 {
		return false
	}
	if isPlainMenuItemID(s.TargetItemID) && unmakeable[s.TargetItemID] {
		return true
	}
	if mid, ok := offerLinkedMenuItemID(s); ok && unmakeable[mid] {
		return true
	}
	if s.TargetItemID == "" && s.TargetName != "" {
		if id := meta.idByName(s.TargetName); id != "" && unmakeable[id] {
			return true
		}
	}
	return false
}

// offerLinkedMenuItemID reads the optional metrics key stamped when an offer
// targets a single menu item (applicable_to=item).
func offerLinkedMenuItemID(s *CampaignSuggestion) (string, bool) {
	if s == nil || s.Metrics == nil {
		return "", false
	}
	raw, ok := s.Metrics["offer_target_item_id"]
	if !ok {
		return "", false
	}
	id, ok := raw.(string)
	if !ok {
		return "", false
	}
	id = strings.TrimSpace(id)
	if id == "" || !isPlainMenuItemID(id) {
		return "", false
	}
	return id, true
}

// clearUnavailableHero strips a name-only hero (happy_hour / win_back) when the
// resolved dish is 86'd or unmakeable so we don't market a sold-out plate. The
// play itself is kept — the window / CRM signal is still valid without a hero.
func clearUnavailableHero(s *CampaignSuggestion, meta menuMetaIndex, unmakeable map[string]bool) {
	if s == nil || s.TargetItemID != "" || s.TargetName == "" {
		return
	}
	item, ok := meta.lookupByName(s.TargetName)
	if !ok || !item.Seen {
		return
	}
	id := meta.idByName(s.TargetName)
	if item.Available && (id == "" || !unmakeable[id]) {
		return
	}
	s.TargetName = ""
	s.ImageURL = ""
	s.ImageSource = ""
	s.TargetDescription = ""
}

// dedupeMenuTargets keeps at most one play per plain menu-item TargetItemID,
// preferring the higher Rank (and stable order for ties). Promo targets
// (bundle:/offer:) and empty IDs are never collapsed across plays.
func dedupeMenuTargets(in []CampaignSuggestion) []CampaignSuggestion {
	if len(in) <= 1 {
		return in
	}
	// Work on a rank-sorted copy so the first seen wins.
	sorted := make([]CampaignSuggestion, len(in))
	copy(sorted, in)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Rank > sorted[j].Rank })

	seen := make(map[string]struct{}, len(sorted))
	out := make([]CampaignSuggestion, 0, len(sorted))
	for _, s := range sorted {
		if isPlainMenuItemID(s.TargetItemID) {
			if _, ok := seen[s.TargetItemID]; ok {
				continue
			}
			seen[s.TargetItemID] = struct{}{}
		}
		out = append(out, s)
	}
	// Restore rank order (already sorted).
	return out
}

// formatPct formats 0..1 as a whole-number percent string for why_factors.
func formatPct(frac float64) string {
	return fmt.Sprintf("%.0f%%", math.Round(frac*100))
}

// formatCount formats an integer count for why_factors values.
func formatCount(n int) string {
	return fmt.Sprintf("%d", n)
}

// formatMoney formats a dollar amount for why_factors (no currency symbol —
// locale-agnostic; FE can re-label).
func formatMoney(v float64) string {
	if math.Abs(v-math.Round(v)) < 0.005 {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.2f", v)
}

// stampRankingVersion marks a suggestion as produced by the S2 ranker.
func stampRankingVersion(s *CampaignSuggestion) {
	if s == nil {
		return
	}
	s.RankingVersion = RankingVersionS2
}

// hasWhyFactor reports whether s already carries a why_factor with the given key.
func hasWhyFactor(s *CampaignSuggestion, key string) bool {
	if s == nil {
		return false
	}
	for _, f := range s.WhyFactors {
		if f.Key == key {
			return true
		}
	}
	return false
}

// blockedMenuItemIDs unions explicit 86s (menu is_available:false / stock-out
// flagged on the index) with the inventory unmakeable set.
func blockedMenuItemIDs(meta menuMetaIndex, unmakeable map[string]bool) map[string]bool {
	out := make(map[string]bool)
	for id, item := range meta.byID {
		if item.Seen && !item.Available {
			out[id] = true
		}
	}
	for id, flag := range unmakeable {
		id = strings.TrimSpace(id)
		if flag && id != "" {
			out[id] = true
		}
	}
	return out
}

// targetsSpecificMenuItem reports whether the play is about one dish (featured,
// move, or an item-scoped offer / happy-hour attached to one). Generic "all"
// offers, bundles, and name-only heroes are not specific.
func targetsSpecificMenuItem(s *CampaignSuggestion) bool {
	if s == nil {
		return false
	}
	if isPlainMenuItemID(s.TargetItemID) {
		return true
	}
	_, ok := offerLinkedMenuItemID(s)
	return ok
}

// menuImageURLSet is the set of dish photos on the active menu. Used to detect
// a generic offer/happy-hour that borrowed a plate photo (#208).
func menuImageURLSet(meta menuMetaIndex) map[string]struct{} {
	out := make(map[string]struct{})
	for _, item := range meta.byID {
		if u := strings.TrimSpace(item.ImageURL); u != "" {
			out[u] = struct{}{}
		}
	}
	return out
}

func dropPhotoReadyBoost(s *CampaignSuggestion) {
	if s == nil || !hasWhyFactor(s, "photo_ready") {
		return
	}
	kept := make([]WhyFactor, 0, len(s.WhyFactors))
	for _, f := range s.WhyFactors {
		if f.Key == "photo_ready" {
			if f.Weight != 0 {
				s.Rank -= f.Weight
			} else {
				s.Rank -= imageRankBoost
			}
			continue
		}
		kept = append(kept, f)
	}
	s.WhyFactors = kept
}

func clearSuggestionPhoto(s *CampaignSuggestion) {
	if s == nil || s.ImageURL == "" {
		return
	}
	s.ImageURL = ""
	s.ImageSource = ""
	dropPhotoReadyBoost(s)
}

// stripUnattributedPhoto drops a menu-dish photo from a play that is not
// actually about that dish. Item-scoped plays keep their photo; generic
// lunch/happy-hour cards must not pretend Harvest Bowl is the offer (#208).
func stripUnattributedPhoto(s *CampaignSuggestion, menuImages map[string]struct{}) {
	if s == nil || s.ImageURL == "" || len(menuImages) == 0 {
		return
	}
	if targetsSpecificMenuItem(s) {
		return
	}
	if _, borrowed := menuImages[s.ImageURL]; borrowed {
		clearSuggestionPhoto(s)
	}
}

// dedupeSharedPhotos keeps the first (highest-rank) card that uses a URL and
// clears later copies so two live posts are not visually identical (#208).
func dedupeSharedPhotos(in []CampaignSuggestion) {
	seen := make(map[string]struct{}, len(in))
	for i := range in {
		url := strings.TrimSpace(in[i].ImageURL)
		if url == "" {
			continue
		}
		if _, dup := seen[url]; dup {
			clearSuggestionPhoto(&in[i])
			continue
		}
		seen[url] = struct{}{}
	}
}

func inventoryBlockName(s *CampaignSuggestion, meta menuMetaIndex) string {
	if s == nil {
		return ""
	}
	if isPlainMenuItemID(s.TargetItemID) {
		if item, ok := meta.byID[s.TargetItemID]; ok {
			if name := strings.TrimSpace(item.Name); name != "" {
				return name
			}
		}
	}
	if id, ok := offerLinkedMenuItemID(s); ok {
		if item, found := meta.byID[id]; found {
			if name := strings.TrimSpace(item.Name); name != "" {
				return name
			}
		}
	}
	return strings.TrimSpace(s.TargetName)
}

func appendUniqueName(dst []string, name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return dst
	}
	for _, existing := range dst {
		if strings.EqualFold(existing, name) {
			return dst
		}
	}
	return append(dst, name)
}
