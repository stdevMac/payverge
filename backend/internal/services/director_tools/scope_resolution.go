package director_tools

import (
	"fmt"
	"sort"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// maxScopeSuggestions caps how many candidate names an error message lists.
const maxScopeSuggestions = 8

// resolveScopeFromNames turns owner-facing item/category NAMES into the
// id-keyed scope string the compute layer consumes ("item:<id>" /
// "category:<id>"). The model cannot know menu IDs, so without this a
// named-single-item request degrades to scope "all" — a menu-wide change the
// owner never asked for (audit L4-16). Resolution never broadens: an item name
// always wins over a passed scope, a category name wins next, and a name that
// matches nothing or several things is an error (listing candidates) rather
// than a guess.
//
// Matching is case-insensitive: exact name match first; if none, a unique
// substring match. Ties are ambiguous.
//
// #579: when an item name matches no menu item, it may still be a real
// entity — an inventory ingredient (e.g. "Premium Beef", a stock item that
// backs recipes but is never itself a menu item). Before proposing a menu
// mutation for a name the owner never put on the menu, check whether it is a
// known inventory ingredient and, if so, say so explicitly instead of
// returning a generic "no menu item named" error — that lets the caller
// (and the model) route the owner to Inventory instead of silently treating
// a stock ingredient as a menu item. db may be nil in tests that don't need
// this classification; the check is then skipped, matching prior behavior.
func resolveScopeFromNames(db *database.DB, businessID uint, categories []database.MenuCategory, scope, itemName, categoryName string) (string, error) {
	itemName = strings.TrimSpace(itemName)
	categoryName = strings.TrimSpace(categoryName)

	if itemName != "" {
		id, err := resolveNameToID(itemName, itemNameIndex(categories), "item")
		if err != nil {
			if invErr := classifyAsInventoryIngredient(db, businessID, itemName); invErr != nil {
				return "", invErr
			}
			return "", err
		}
		return "item:" + id, nil
	}
	if categoryName != "" {
		id, err := resolveNameToID(categoryName, categoryNameIndex(categories), "category")
		if err != nil {
			return "", err
		}
		return "category:" + id, nil
	}
	return scope, nil
}

// classifyAsInventoryIngredient reports whether name matches a known,
// active inventory item (stock ingredient) for the business, distinct from
// menu items. It only runs after menu-name resolution has already failed, so
// it never shadows a real menu-item match. Returns nil (no classification —
// caller falls back to the generic "no menu item named" error) when db is
// nil, the query errors, or nothing matches.
func classifyAsInventoryIngredient(db *database.DB, businessID uint, name string) error {
	if db == nil {
		return nil
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	var items []struct{ Name string }
	if err := db.GetGorm().
		Model(&database.InventoryItem{}).
		Where("business_id = ? AND is_active = ?", businessID, true).
		Select("name").
		Find(&items).Error; err != nil {
		return nil
	}
	needle := strings.ToLower(name)
	for _, it := range items {
		haystack := strings.ToLower(strings.TrimSpace(it.Name))
		if haystack == "" {
			continue
		}
		if haystack == needle || strings.Contains(haystack, needle) || strings.Contains(needle, haystack) {
			return fmt.Errorf("%q is an inventory ingredient, not a menu item — it has no menu availability toggle; open the Inventory tab to check or adjust its stock", name)
		}
	}
	return nil
}

// namedID pairs a display name with its menu ID for resolution.
type namedID struct {
	Name string
	ID   string
}

func itemNameIndex(categories []database.MenuCategory) []namedID {
	var out []namedID
	for _, cat := range categories {
		for _, item := range cat.Items {
			out = append(out, namedID{Name: item.Name, ID: item.ID})
		}
	}
	return out
}

func categoryNameIndex(categories []database.MenuCategory) []namedID {
	out := make([]namedID, 0, len(categories))
	for _, cat := range categories {
		out = append(out, namedID{Name: cat.Name, ID: cat.ID})
	}
	return out
}

// resolveNameToID resolves one name against the candidates: unique
// case-insensitive exact match, else unique case-insensitive substring match,
// else an error that lists what DOES exist so the model can retry precisely.
func resolveNameToID(name string, candidates []namedID, kind string) (string, error) {
	needle := strings.ToLower(strings.TrimSpace(name))

	var exact, partial []namedID
	for _, c := range candidates {
		haystack := strings.ToLower(strings.TrimSpace(c.Name))
		switch {
		case haystack == needle:
			exact = append(exact, c)
		case strings.Contains(haystack, needle):
			partial = append(partial, c)
		}
	}

	matches := exact
	if len(matches) == 0 {
		matches = partial
	}
	switch len(matches) {
	case 1:
		return matches[0].ID, nil
	case 0:
		return "", fmt.Errorf("no menu %s named %q — available: %s", kind, name, joinNames(candidates))
	default:
		return "", fmt.Errorf("%s name %q is ambiguous — matches: %s; use the exact name", kind, name, joinNames(matches))
	}
}

func joinNames(list []namedID) string {
	names := make([]string, 0, len(list))
	for _, c := range list {
		names = append(names, c.Name)
	}
	sort.Strings(names)
	if len(names) > maxScopeSuggestions {
		names = append(names[:maxScopeSuggestions], "…")
	}
	return strings.Join(names, ", ")
}
