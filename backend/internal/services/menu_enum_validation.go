package services

import (
	"log"
	"strconv"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Canonical allergen IDs — must match frontend/src/constants/menu-tags.ts ALLERGENS.
var canonicalAllergenIDs = map[string]struct{}{
	"celery": {}, "crustaceans": {}, "dairy": {}, "eggs": {}, "fish": {},
	"gluten": {}, "lupin": {}, "mollusc": {}, "mustard": {}, "peanut": {},
	"sesame": {}, "so2": {}, "soya": {}, "treenuts": {},
}

// allergenAliases maps near-canonical / common LLM extract near-miss IDs onto
// the frontend ALLERGENS set. Keys are lowercase with spaces already folded to
// underscores (see normalizeEnumKey). Without this map, imports silently drop
// critical labels (peanuts vs peanut, tree_nuts vs treenuts, sulfites vs so2).
var allergenAliases = map[string]string{
	"peanuts":    "peanut",
	"tree_nuts":  "treenuts",
	"tree-nuts":  "treenuts",
	"treenut":    "treenuts",
	"nuts":       "treenuts",
	"shellfish":  "crustaceans",
	"sulfites":   "so2",
	"sulphites":  "so2",
	"sulfite":    "so2",
	"sulphite":   "so2",
	"soy":        "soya",
	"soybean":    "soya",
	"soybeans":   "soya",
	"molluscs":   "mollusc",
	"mollusks":   "mollusc",
	"mollusk":    "mollusc",
	"egg":        "eggs",
	"crustacean": "crustaceans",
}

// Canonical dietary tag IDs — must match frontend DIETARY_TAGS.
var canonicalDietaryTagIDs = map[string]struct{}{
	"vegan": {}, "vegetarian": {}, "gluten-free": {}, "dairy-free": {},
	"nut-free": {}, "mild": {}, "low-sodium": {},
}

// dietaryAliases maps near-miss dietary tag IDs (underscores, plurals) to
// the frontend DIETARY_TAGS set.
var dietaryAliases = map[string]string{
	"gluten_free": "gluten-free",
	"dairy_free":  "dairy-free",
	"nut_free":    "nut-free",
	"low_sodium":  "low-sodium",
	"glutenfree":  "gluten-free",
	"dairyfree":   "dairy-free",
	"nutfree":     "nut-free",
	"lowsodium":   "low-sodium",
}

type MenuSanitizeDecision struct {
	CategoryIndex  int    `json:"category_index"`
	CategoryName   string `json:"category_name"`
	ItemIndex      int    `json:"item_index"`
	ItemName       string `json:"item_name"`
	Field          string `json:"field"`
	Value          string `json:"value"`
	CanonicalValue string `json:"canonical_value,omitempty"`
	Reason         string `json:"reason"`
}

type GeneratedMenuSanitizeReport struct {
	DroppedAllergens   int                    `json:"dropped_allergens"`
	DroppedDietaryTags int                    `json:"dropped_dietary_tags"`
	DroppedItems       int                    `json:"dropped_items"`
	Retained           []MenuSanitizeDecision `json:"retained"`
	Dropped            []MenuSanitizeDecision `json:"dropped"`
}

func (r GeneratedMenuSanitizeReport) HasDrops() bool {
	return r.DroppedAllergens+r.DroppedDietaryTags+r.DroppedItems > 0
}

// normalizeEnumKey lowercases, trims, and folds spaces to underscores so
// "Tree Nuts" / " tree_nuts " share one alias lookup key.
func normalizeEnumKey(v string) string {
	key := strings.ToLower(strings.TrimSpace(v))
	key = strings.ReplaceAll(key, " ", "_")
	return key
}

// resolveAllergenID returns the canonical allergen ID for v, or ("", false)
// when v is not a known allergen or alias.
func resolveAllergenID(v string) (string, bool) {
	key := normalizeEnumKey(v)
	if key == "" {
		return "", false
	}
	if _, ok := canonicalAllergenIDs[key]; ok {
		return key, true
	}
	// Hyphenated form of underscore keys (tree-nuts already in aliases).
	if canon, ok := allergenAliases[key]; ok {
		return canon, true
	}
	hyphenated := strings.ReplaceAll(key, "_", "-")
	if canon, ok := allergenAliases[hyphenated]; ok {
		return canon, true
	}
	return "", false
}

// resolveDietaryTagID returns the canonical dietary tag ID for v.
func resolveDietaryTagID(v string) (string, bool) {
	key := normalizeEnumKey(v)
	if key == "" {
		return "", false
	}
	// Canonical dietary tags use hyphens (gluten-free), not underscores.
	hyphenated := strings.ReplaceAll(key, "_", "-")
	if _, ok := canonicalDietaryTagIDs[hyphenated]; ok {
		return hyphenated, true
	}
	if _, ok := canonicalDietaryTagIDs[key]; ok {
		return key, true
	}
	if canon, ok := dietaryAliases[key]; ok {
		return canon, true
	}
	if canon, ok := dietaryAliases[hyphenated]; ok {
		return canon, true
	}
	// Compact form: glutenfree
	compact := strings.ReplaceAll(hyphenated, "-", "")
	if canon, ok := dietaryAliases[compact]; ok {
		return canon, true
	}
	return "", false
}

// CanonicalDietaryTagID exposes the canonical dietary tag resolver to callers
// that need to compare stored (possibly aliased or translated) tags against the
// canonical DIETARY_TAGS vocabulary — for example the AI waiter's deterministic
// "what is vegetarian here?" filter.
func CanonicalDietaryTagID(v string) (string, bool) { return resolveDietaryTagID(v) }

// SanitizeMenuCategories is the shared menu-definition sanitizer used by both the
// AI generation path (via sanitizeGeneratedMenu) and the AI import + wizard rails.
// It drops items whose price is out of the (0, 10000) dollar range and rewrites
// allergen / dietary-tag IDs to the canonical frontend sets (with a small alias
// map for near-miss extract IDs), returning the cleaned categories and a report
// of values that could not be mapped and items that were dropped. Prices are
// float64 DOLLARS (menu definitions), never cents — do not convert.
func SanitizeMenuCategories(categories []database.MenuCategory) ([]database.MenuCategory, GeneratedMenuSanitizeReport) {
	r := GeneratedMenuSanitizeReport{
		Retained: make([]MenuSanitizeDecision, 0),
		Dropped:  make([]MenuSanitizeDecision, 0),
	}
	for ci := range categories {
		kept := make([]database.MenuItem, 0, len(categories[ci].Items))
		for ii, item := range categories[ci].Items {
			if !(item.Price > 0 && item.Price < 10000) {
				r.DroppedItems++
				r.Dropped = append(r.Dropped, MenuSanitizeDecision{
					CategoryIndex: ci,
					CategoryName:  categories[ci].Name,
					ItemIndex:     ii,
					ItemName:      item.Name,
					Field:         "price",
					Value:         strconv.FormatFloat(item.Price, 'f', -1, 64),
					Reason:        "price_out_of_range",
				})
				continue
			}
			a, da := sanitizeEnumValues(item.Allergens, "allergens", ci, ii, categories[ci].Name, item.Name, resolveAllergenID, &r)
			d, dd := sanitizeEnumValues(item.DietaryTags, "dietary_tags", ci, ii, categories[ci].Name, item.Name, resolveDietaryTagID, &r)
			item.Allergens = a
			item.DietaryTags = d
			r.DroppedAllergens += da
			r.DroppedDietaryTags += dd
			kept = append(kept, item)
		}
		categories[ci].Items = kept
	}
	if r.DroppedAllergens+r.DroppedDietaryTags+r.DroppedItems > 0 {
		log.Printf("SanitizeMenuCategories: dropped %d allergen IDs, %d dietary tags, %d out-of-range items",
			r.DroppedAllergens, r.DroppedDietaryTags, r.DroppedItems)
	}
	return categories, r
}

func sanitizeEnumValues(
	values []string,
	field string,
	categoryIndex, itemIndex int,
	categoryName, itemName string,
	resolve func(string) (string, bool),
	report *GeneratedMenuSanitizeReport,
) ([]string, int) {
	kept := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	dropped := 0
	for _, value := range values {
		canonical, ok := resolve(value)
		decision := MenuSanitizeDecision{
			CategoryIndex: categoryIndex,
			CategoryName:  categoryName,
			ItemIndex:     itemIndex,
			ItemName:      itemName,
			Field:         field,
			Value:         value,
		}
		if !ok {
			decision.Reason = "unknown_enum_value"
			report.Dropped = append(report.Dropped, decision)
			dropped++
			continue
		}
		if _, duplicate := seen[canonical]; duplicate {
			continue
		}
		seen[canonical] = struct{}{}
		kept = append(kept, canonical)
		decision.CanonicalValue = canonical
		if value == canonical {
			decision.Reason = "canonical_value"
		} else {
			decision.Reason = "alias_normalized"
		}
		report.Retained = append(report.Retained, decision)
	}
	return kept, dropped
}

func sanitizeGeneratedMenu(menu *GeneratedMenu) GeneratedMenuSanitizeReport {
	cats, r := SanitizeMenuCategories(menu.Categories)
	menu.Categories = cats
	return r
}
