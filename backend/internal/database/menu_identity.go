package database

import (
	"strings"

	"github.com/google/uuid"
)

// ensureMenuEntityIDs makes category and item identities a server-owned
// invariant at every whole-menu persistence boundary. Browser clients omit IDs
// for new entities, while imports and legacy callers may also provide blanks;
// existing non-empty IDs are preserved for stable links and optimistic edits.
func ensureMenuEntityIDs(categories []MenuCategory) {
	for categoryIndex := range categories {
		category := &categories[categoryIndex]
		if strings.TrimSpace(category.ID) == "" {
			category.ID = uuid.NewString()
		}
		for itemIndex := range category.Items {
			item := &category.Items[itemIndex]
			if strings.TrimSpace(item.ID) == "" {
				item.ID = uuid.NewString()
			}
		}
	}
}
