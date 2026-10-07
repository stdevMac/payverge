package director_actions

import (
	"fmt"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// ComputeAvailabilityChange deep-copies categories, sets IsAvailable on targeted
// items, and returns the mutated copy + a preview diff.
//
// Returns: (mutated, preview, warnings, requiresReconfirm, err)
//   - requiresReconfirm is true when all items are being turned off.
//   - The input categories slice is never mutated.
func ComputeAvailabilityChange(categories []database.MenuCategory, p AvailabilityParams) (
	[]database.MenuCategory, ActionPreview, []string, bool, error,
) {
	copy := deepCopyCategories(categories)
	var warnings []string
	var examples []PreviewExample
	affectedCount := 0

	for ci := range copy {
		cat := &copy[ci]
		inCategoryScope := p.Target == "all" ||
			(strings.HasPrefix(p.Target, "category:") && strings.TrimPrefix(p.Target, "category:") == cat.ID)

		for ii := range cat.Items {
			item := &cat.Items[ii]
			inScope := inCategoryScope ||
				(strings.HasPrefix(p.Target, "item:") && strings.TrimPrefix(p.Target, "item:") == item.ID)
			if !inScope {
				continue
			}

			old := item.IsAvailable
			item.IsAvailable = p.Available
			affectedCount++

			if len(examples) < maxPreviewExamples {
				examples = append(examples, PreviewExample{
					Name:   item.Name,
					Before: old,
					After:  p.Available,
				})
			}
		}
	}

	if affectedCount == 0 {
		warnings = append(warnings, fmt.Sprintf("No items matched target %q", p.Target))
	}

	// Reconfirm when turning ALL items off.
	requiresReconfirm := p.Target == "all" && !p.Available && affectedCount > 0

	status := "available"
	if !p.Available {
		status = "unavailable"
	}
	summary := fmt.Sprintf("%d item(s) → %s.", affectedCount, status)

	preview := ActionPreview{
		AffectedCount: affectedCount,
		Examples:      examples,
		Summary:       summary,
	}
	return copy, preview, warnings, requiresReconfirm, nil
}

// AvailabilityChangeTitle returns a deterministic English title for an availability proposal.
func AvailabilityChangeTitle(p AvailabilityParams) string {
	state := "Enable"
	if !p.Available {
		state = "Disable"
	}
	scope := "all items"
	if strings.HasPrefix(p.Target, "category:") {
		scope = fmt.Sprintf("category %s", strings.TrimPrefix(p.Target, "category:"))
	} else if strings.HasPrefix(p.Target, "item:") {
		scope = fmt.Sprintf("item %s", strings.TrimPrefix(p.Target, "item:"))
	}
	return fmt.Sprintf("%s %s", state, scope)
}

// AvailabilityChangeTitleWithPreview uses the operator-facing item name from
// the dry-run examples instead of the internal menu item id.
func AvailabilityChangeTitleWithPreview(p AvailabilityParams, preview ActionPreview) string {
	return replaceItemIDWithDisplayName(AvailabilityChangeTitle(p), p.Target, preview)
}

// AvailabilityChangeDescription returns a deterministic English description for an availability preview.
func AvailabilityChangeDescription(preview ActionPreview) string {
	if preview.AffectedCount == 0 {
		return "No items would be affected."
	}
	return fmt.Sprintf("Applies to %d item(s). %s", preview.AffectedCount, preview.Summary)
}
