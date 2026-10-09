package database

import (
	"sort"
	"strings"

	"gorm.io/gorm"
)

// LoyaltyTierDuplicateIDs returns the tier rows that can be safely removed
// from a persisted ladder. Rows are processed per program in deterministic
// sort_order/ID order. Names use the same Unicode-aware trim and case folding
// as the runtime validator. Blank names never participate in name uniqueness,
// but a named row always replaces a blank row when both share a threshold.
//
// The helper returns only duplicate row IDs; it does not guess how to repair
// non-increasing thresholds or otherwise ambiguous ladders.
func LoyaltyTierDuplicateIDs(tiers []LoyaltyTier) []uint {
	ordered := append([]LoyaltyTier(nil), tiers...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].LoyaltyProgramID != ordered[j].LoyaltyProgramID {
			return ordered[i].LoyaltyProgramID < ordered[j].LoyaltyProgramID
		}
		if ordered[i].SortOrder != ordered[j].SortOrder {
			return ordered[i].SortOrder < ordered[j].SortOrder
		}
		return ordered[i].ID < ordered[j].ID
	})

	duplicateIDs := make([]uint, 0)
	for start := 0; start < len(ordered); {
		end := start + 1
		for end < len(ordered) && ordered[end].LoyaltyProgramID == ordered[start].LoyaltyProgramID {
			end++
		}
		duplicateIDs = append(duplicateIDs, loyaltyTierDuplicateIDsForProgram(ordered[start:end])...)
		start = end
	}
	return duplicateIDs
}

// NormalizeLoyaltyTierSortOrders compacts one program's tiers into a stable,
// unique 0..n-1 order. Existing order is authoritative; IDs break ties for
// duplicate persisted values so repair results do not depend on query order.
func NormalizeLoyaltyTierSortOrders(db *gorm.DB, programID uint) error {
	if db == nil {
		return nil
	}

	var tiers []LoyaltyTier
	if err := db.Where("loyalty_program_id = ?", programID).
		Order("sort_order ASC, id ASC").Find(&tiers).Error; err != nil {
		return err
	}
	for order, tier := range tiers {
		if tier.SortOrder == order {
			continue
		}
		if err := db.Model(&LoyaltyTier{}).
			Where("id = ? AND loyalty_program_id = ?", tier.ID, programID).
			Update("sort_order", order).Error; err != nil {
			return err
		}
	}
	return nil
}

func loyaltyTierDuplicateIDsForProgram(ordered []LoyaltyTier) []uint {
	type survivor struct {
		tier LoyaltyTier
	}

	survivors := make([]survivor, 0, len(ordered))
	nameIndex := make(map[string]int, len(ordered))
	thresholdIndex := make(map[int64]int, len(ordered))
	duplicateIDs := make([]uint, 0)

	rebuildIndexes := func() {
		clear(nameIndex)
		clear(thresholdIndex)
		for i, current := range survivors {
			name := NormalizeLoyaltyTierName(current.tier.Name)
			if name != "" {
				nameIndex[name] = i
			}
			thresholdIndex[current.tier.MinLifetimeSpentCents] = i
		}
	}

	for _, tier := range ordered {
		name := NormalizeLoyaltyTierName(tier.Name)
		nameDup := false
		if name != "" {
			_, nameDup = nameIndex[name]
		}
		threshold, thresholdDup := thresholdIndex[tier.MinLifetimeSpentCents]

		if thresholdDup {
			existing := survivors[threshold].tier
			existingName := NormalizeLoyaltyTierName(existing.Name)
			if name != "" && existingName == "" {
				if nameDup {
					// The named candidate cannot be a valid survivor because
					// its name is already used elsewhere. Remove the blank
					// threshold row so it cannot leave an invalid ladder behind.
					duplicateIDs = append(duplicateIDs, existing.ID, tier.ID)
					survivors = append(survivors[:threshold], survivors[threshold+1:]...)
					rebuildIndexes()
					continue
				}

				// Prefer a valid named row to a blank row at the same
				// threshold. Replace the survivor in place so later rows
				// cannot use the deleted blank as a keeper.
				duplicateIDs = append(duplicateIDs, existing.ID)
				survivors[threshold] = survivor{tier: tier}
				rebuildIndexes()
				continue
			}

			duplicateIDs = append(duplicateIDs, tier.ID)
			continue
		}

		if nameDup {
			duplicateIDs = append(duplicateIDs, tier.ID)
			continue
		}

		survivors = append(survivors, survivor{tier: tier})
		index := len(survivors) - 1
		thresholdIndex[tier.MinLifetimeSpentCents] = index
		if name != "" {
			nameIndex[name] = index
		}
	}

	return duplicateIDs
}

// NormalizeLoyaltyTierName is the canonical loyalty-tier name key used by
// startup repair, runtime validation, and all persisted duplicate decisions.
// strings.TrimSpace handles the Unicode whitespace set used by the backend;
// strings.ToLower intentionally uses Go's simple Unicode case mapping.
func NormalizeLoyaltyTierName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
