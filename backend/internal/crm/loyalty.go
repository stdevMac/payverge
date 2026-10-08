package crm

import (
	"errors"
	"math"
	"sort"

	"github.com/stdevmac/payverge/backend/internal/database"
	"gorm.io/gorm"
)

// ErrInvalidLoyaltyTierLadder indicates that persisted loyalty tiers remain
// invalid after safe duplicate-row repair. Settlement must reject this state
// rather than assigning a tier from an ambiguous ladder.
var ErrInvalidLoyaltyTierLadder = errors.New("invalid loyalty tier ladder")

// maxLoyaltyTierThresholdCents is 1,000,000,000.00 dollars. Thresholds outside
// [0, max] are rejected so a bad client cannot persist an unbounded ladder.
const maxLoyaltyTierThresholdCents int64 = 100_000_000_000

// validateTierLadder enforces the same invariants the tier editor promises the
// operator: tier names must be unique (case-insensitive) and no two tiers may
// share a min_lifetime_spent threshold. Duplicate/overlapping tiers make
// computeTier's "first match wins" result ambiguous and produce the duplicate
// "Bronze at $0" rows seen in the demo. The client validates too, but this is
// the authoritative guard — never trust the posted list.
func validateTierLadder(tiers []database.LoyaltyTier) error {
	seenNames := make(map[string]struct{}, len(tiers))
	seenThresholds := make(map[int64]struct{}, len(tiers))
	for _, t := range tiers {
		if t.MinLifetimeSpentCents < 0 || t.MinLifetimeSpentCents > maxLoyaltyTierThresholdCents {
			return errors.New("loyalty tier spend thresholds must be between 0 and 1,000,000,000.00")
		}
		name := database.NormalizeLoyaltyTierName(t.Name)
		if name == "" {
			return errors.New("each loyalty tier must have a name")
		}
		if _, dup := seenNames[name]; dup {
			return errors.New("loyalty tiers must have unique names")
		}
		seenNames[name] = struct{}{}
		if _, dup := seenThresholds[t.MinLifetimeSpentCents]; dup {
			return errors.New("loyalty tiers must have distinct spend thresholds")
		}
		seenThresholds[t.MinLifetimeSpentCents] = struct{}{}
	}
	ordered := append([]database.LoyaltyTier(nil), tiers...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].SortOrder < ordered[j].SortOrder
	})
	for i := 1; i < len(ordered); i++ {
		if ordered[i].MinLifetimeSpentCents < ordered[i-1].MinLifetimeSpentCents {
			return errors.New("loyalty tier spend thresholds must increase with tier order")
		}
	}
	return nil
}

// repairDuplicateTierRows removes only rows that are unambiguously duplicate
// by normalized name or threshold. The deterministic survivor policy is shared
// with startup repair: a named row wins over a blank row at the same threshold,
// then sort_order/ID order wins. Ambiguous drift such as a non-increasing
// ladder is left for validation to report rather than guessing which
// operator-authored threshold should change.
func repairDuplicateTierRows(db *gorm.DB, program *database.LoyaltyProgram) (bool, error) {
	if db == nil || program == nil || len(program.Tiers) < 2 {
		return false, nil
	}

	duplicateIDs := database.LoyaltyTierDuplicateIDs(program.Tiers)

	if len(duplicateIDs) == 0 {
		return false, nil
	}
	if err := db.Where("loyalty_program_id = ? AND id IN ?", program.ID, duplicateIDs).
		Delete(&database.LoyaltyTier{}).Error; err != nil {
		return false, err
	}
	if err := database.NormalizeLoyaltyTierSortOrders(db, program.ID); err != nil {
		return false, err
	}
	return true, nil
}

// canonicalizeTierSortOrders preserves the submitted ladder order while
// removing gaps and duplicate metadata before it is persisted. Stable sorting
// keeps the request's array order as the deterministic tie-breaker.
func canonicalizeTierSortOrders(tiers []database.LoyaltyTier) []database.LoyaltyTier {
	ordered := append([]database.LoyaltyTier(nil), tiers...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].SortOrder < ordered[j].SortOrder
	})
	for i := range ordered {
		ordered[i].SortOrder = i
	}
	return ordered
}

// lifetimeSpentCents converts a stored TotalSpent (float64 USD) into integer
// cents, rounding rather than truncating. A bare int64(dollars*100) truncates
// because most cent values are inexact in float64 (e.g. 0.57*100 == 56.9999…),
// which would drop a cent and leave a customer just short of a tier threshold.
func lifetimeSpentCents(totalSpentDollars float64) int64 {
	return int64(math.Round(totalSpentDollars * 100))
}

// computePointsEarned converts spend (in cents) into loyalty points for the
// given program. Disabled programs and a nil program both yield 0 — settlement
// code can call this unconditionally without nil-guarding.
func computePointsEarned(amountCents int64, program *database.LoyaltyProgram) int {
	if program == nil || !program.Enabled {
		return 0
	}
	return int((float64(amountCents) / 100.0) * program.PointsPerDollar)
}

// computeTier returns the tier name the customer has earned given their
// lifetime spend (in cents). Ties go to the higher tier. Returns "" when the
// program has no tiers configured so callers can leave loyalty_tier blank
// rather than committing an arbitrary default.
func computeTier(lifetimeSpentCents int64, program *database.LoyaltyProgram) string {
	if program == nil || len(program.Tiers) == 0 {
		return ""
	}
	tiers := make([]database.LoyaltyTier, len(program.Tiers))
	copy(tiers, program.Tiers)
	sort.Slice(tiers, func(i, j int) bool {
		return tiers[i].MinLifetimeSpentCents > tiers[j].MinLifetimeSpentCents
	})
	for _, t := range tiers {
		if lifetimeSpentCents >= t.MinLifetimeSpentCents {
			return t.Name
		}
	}
	return ""
}
