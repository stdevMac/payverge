package director_actions

import (
	"fmt"
	"unicode"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// ComputeContentEdit deep-copies categories, applies description and/or dietary
// tag changes to the targeted item, enforces caps, and returns the mutated copy
// + a preview diff.
//
// SECURITY: This function MUST NEVER read or write the Allergens field.
//
// Returns: (mutated, preview, warnings, requiresReconfirm, err)
//   - err is non-nil if the item is not found, description is over-length, contains
//     non-printable runes, or a dietary tag is not in the canonical set.
//   - The input categories slice is never mutated.
func ComputeContentEdit(categories []database.MenuCategory, p ContentEditParams) (
	[]database.MenuCategory, ActionPreview, []string, bool, error,
) {
	// Validate description before copying.
	if p.Description != nil {
		runes := []rune(*p.Description)
		if len(runes) > DescriptionMaxRunes {
			return nil, ActionPreview{}, nil, false,
				fmt.Errorf("description exceeds %d rune limit (got %d)", DescriptionMaxRunes, len(runes))
		}
		for _, r := range runes {
			if !unicode.IsPrint(r) {
				return nil, ActionPreview{}, nil, false,
					fmt.Errorf("description contains non-printable rune %U", r)
			}
		}
	}

	// Validate dietary tags.
	if p.DietaryTags != nil {
		for _, tag := range *p.DietaryTags {
			if _, ok := canonicalDietaryTags[tag]; !ok {
				return nil, ActionPreview{}, nil, false,
					fmt.Errorf("unknown dietary tag %q (allowed: vegan, vegetarian, gluten-free, dairy-free, nut-free, mild, low-sodium)", tag)
			}
		}
	}

	copy := deepCopyCategories(categories)
	var examples []PreviewExample
	found := false

	for ci := range copy {
		for ii := range copy[ci].Items {
			item := &copy[ci].Items[ii]
			if item.ID != p.ItemID {
				continue
			}
			found = true

			if p.Description != nil {
				old := item.Description
				item.Description = *p.Description
				examples = append(examples, PreviewExample{
					Name:   item.Name + " (description)",
					Before: old,
					After:  *p.Description,
				})
			}

			if p.DietaryTags != nil {
				old := make([]string, len(item.DietaryTags))
				copy2 := make([]string, len(*p.DietaryTags))
				builtinCopy(old, item.DietaryTags)
				builtinCopy(copy2, *p.DietaryTags)
				item.DietaryTags = copy2
				examples = append(examples, PreviewExample{
					Name:   item.Name + " (dietary_tags)",
					Before: old,
					After:  *p.DietaryTags,
				})
			}

			// NEVER touch Allergens.
			break
		}
		if found {
			break
		}
	}

	if !found {
		return nil, ActionPreview{}, nil, false,
			fmt.Errorf("item %q not found in menu", p.ItemID)
	}

	preview := ActionPreview{
		AffectedCount: 1,
		Examples:      examples,
		Summary:       fmt.Sprintf("Updated item %q.", p.ItemID),
	}
	return copy, preview, nil, false, nil
}

// ContentEditTitle returns a deterministic English title for a content edit proposal.
func ContentEditTitle(p ContentEditParams) string {
	what := []string{}
	if p.Description != nil {
		what = append(what, "description")
	}
	if p.DietaryTags != nil {
		what = append(what, "dietary tags")
	}
	if len(what) == 0 {
		return fmt.Sprintf("Edit item %s", p.ItemID)
	}
	fieldList := what[0]
	if len(what) > 1 {
		fieldList = what[0] + " and " + what[1]
	}
	return fmt.Sprintf("Edit %s for item %s", fieldList, p.ItemID)
}

// ContentEditDescription returns a deterministic English description for a content edit preview.
func ContentEditDescription(preview ActionPreview) string {
	return fmt.Sprintf("Updates content fields for 1 item. %s", preview.Summary)
}

// builtinCopy copies string slices (avoids naming conflict with the built-in copy).
func builtinCopy(dst, src []string) {
	for i, v := range src {
		if i < len(dst) {
			dst[i] = v
		}
	}
}
