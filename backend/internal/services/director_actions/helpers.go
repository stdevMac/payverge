package director_actions

import (
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// replaceItemIDWithDisplayName swaps "item <internal-id>" in proposal titles
// for the operator-facing dish name from the dry-run preview.
func replaceItemIDWithDisplayName(title, scope string, preview ActionPreview) string {
	if len(preview.Examples) == 0 {
		return title
	}
	name := strings.TrimSpace(preview.Examples[0].Name)
	if name == "" {
		return title
	}
	id := ""
	switch {
	case strings.HasPrefix(scope, "item:"):
		id = strings.TrimPrefix(scope, "item:")
	case strings.HasPrefix(scope, "category:"):
		return title
	}
	if id == "" {
		return title
	}
	title = strings.ReplaceAll(title, "item "+id+" price", name+" price")
	title = strings.ReplaceAll(title, "item "+id, name)
	return title
}

// deepCopyCategories returns a deep copy of the category slice, including all
// nested item []string fields (Allergens, DietaryTags, Images, Options).
// Every Compute* function MUST call this first to ensure the caller's input is
// never mutated.
func deepCopyCategories(cats []database.MenuCategory) []database.MenuCategory {
	if cats == nil {
		return nil
	}
	out := make([]database.MenuCategory, len(cats))
	for ci, cat := range cats {
		newCat := cat // shallow copy of the struct value
		newCat.Items = make([]database.MenuItem, len(cat.Items))
		for ii, item := range cat.Items {
			newItem := item // shallow copy of the struct value
			// Deep-copy all []string and []MenuItemOption fields.
			newItem.Allergens = copyStrings(item.Allergens)
			newItem.DietaryTags = copyStrings(item.DietaryTags)
			newItem.Images = copyStrings(item.Images)
			if item.Options != nil {
				newItem.Options = make([]database.MenuItemOption, len(item.Options))
				for oi, opt := range item.Options {
					newItem.Options[oi] = opt // MenuItemOption has no slice fields
				}
			}
			newCat.Items[ii] = newItem
		}
		out[ci] = newCat
	}
	return out
}

// copyStrings returns a new slice with the same contents, or nil for nil input.
func copyStrings(s []string) []string {
	if s == nil {
		return nil
	}
	dst := make([]string, len(s))
	copy(dst, s)
	return dst
}

// snapshotItem captures the mutable fields of one item.
// NEVER includes Allergens — the AI may never touch them.
func snapshotItem(catID string, item database.MenuItem) ItemSnapshot {
	return ItemSnapshot{
		CategoryID:  catID,
		ItemID:      item.ID,
		Price:       item.Price,
		IsAvailable: item.IsAvailable,
		Description: item.Description,
		DietaryTags: copyStrings(item.DietaryTags),
	}
}

// DiffSnapshots compares before and after category trees and returns snapshots
// of items whose mutable fields changed. Items that are identical in both trees
// are omitted. The Allergens field is never included in the comparison because
// the compute functions never touch it.
//
// Returns: (changedBefore, changedAfter) — parallel slices, same length.
func DiffSnapshots(before, after []database.MenuCategory) (changedBefore, changedAfter []ItemSnapshot) {
	// Index before items by item id.
	type key struct{ catID, itemID string }
	beforeMap := make(map[key]ItemSnapshot)
	for _, cat := range before {
		for _, item := range cat.Items {
			beforeMap[key{cat.ID, item.ID}] = snapshotItem(cat.ID, item)
		}
	}

	for _, cat := range after {
		for _, item := range cat.Items {
			k := key{cat.ID, item.ID}
			bsnap, exists := beforeMap[k]
			if !exists {
				// New item — include as a change with zero before.
				changedBefore = append(changedBefore, ItemSnapshot{CategoryID: cat.ID, ItemID: item.ID})
				changedAfter = append(changedAfter, snapshotItem(cat.ID, item))
				continue
			}
			asnap := snapshotItem(cat.ID, item)
			if snapshotsDiffer(bsnap, asnap) {
				changedBefore = append(changedBefore, bsnap)
				changedAfter = append(changedAfter, asnap)
			}
		}
	}
	return changedBefore, changedAfter
}

// snapshotsDiffer returns true if any mutable field differs between two snapshots.
func snapshotsDiffer(a, b ItemSnapshot) bool {
	if a.Price != b.Price || a.IsAvailable != b.IsAvailable || a.Description != b.Description {
		return true
	}
	if len(a.DietaryTags) != len(b.DietaryTags) {
		return true
	}
	for i := range a.DietaryTags {
		if a.DietaryTags[i] != b.DietaryTags[i] {
			return true
		}
	}
	return false
}

// RestoreSnapshots deep-copies categories and restores each item's mutable fields
// (price, is_available, description, dietary_tags) from the snapshot set.
// Allergens are NEVER touched — they remain as-is on the live menu.
func RestoreSnapshots(categories []database.MenuCategory, snaps []ItemSnapshot) []database.MenuCategory {
	if len(snaps) == 0 {
		return deepCopyCategories(categories)
	}

	// Index snaps by item_id for O(1) lookup.
	snapByItemID := make(map[string]ItemSnapshot, len(snaps))
	for _, s := range snaps {
		snapByItemID[s.ItemID] = s
	}

	restored := deepCopyCategories(categories)
	for ci := range restored {
		for ii := range restored[ci].Items {
			item := &restored[ci].Items[ii]
			snap, ok := snapByItemID[item.ID]
			if !ok {
				continue
			}
			item.Price = snap.Price
			item.IsAvailable = snap.IsAvailable
			item.Description = snap.Description
			item.DietaryTags = copyStrings(snap.DietaryTags)
			// Allergens intentionally NOT restored.
		}
	}
	return restored
}
