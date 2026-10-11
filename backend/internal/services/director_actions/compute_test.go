package director_actions

import (
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// sampleMenu returns a predictable two-item menu for all tests.
func sampleMenu() []database.MenuCategory {
	return []database.MenuCategory{{
		ID:   "cat-1",
		Name: "Mains",
		Items: []database.MenuItem{
			{ID: "i1", Name: "Burger", Price: 10.0, IsAvailable: true,
				Allergens: []string{"gluten"}, DietaryTags: []string{"mild"}},
			{ID: "i2", Name: "Fries", Price: 4.0, IsAvailable: true,
				Allergens: []string{"gluten"}, DietaryTags: []string{}},
		},
	}}
}

// --- Price tests ---

func TestComputePriceChange_PercentUpRoundsAndPreviews(t *testing.T) {
	got, prev, _, reconfirm, err := ComputePriceChange(sampleMenu(),
		PriceChangeParams{Scope: "all", Mode: "percent", Value: 20, Direction: "up"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reconfirm {
		t.Fatal("menu-wide price change (scope all, >1 items) must always require reconfirm (L4-16)")
	}
	burger := got[0].Items[0]
	fries := got[0].Items[1]
	if burger.Price != 12.0 {
		t.Fatalf("burger: want 12.00 got %.2f", burger.Price)
	}
	if fries.Price != 4.80 {
		t.Fatalf("fries: want 4.80 got %.2f", fries.Price)
	}
	if prev.AffectedCount != 2 {
		t.Fatalf("affected_count: want 2 got %d", prev.AffectedCount)
	}
	if len(prev.Examples) == 0 {
		t.Fatal("expected at least one preview example")
	}
}

func TestComputePriceChange_FlatDown(t *testing.T) {
	got, prev, _, _, err := ComputePriceChange(sampleMenu(),
		PriceChangeParams{Scope: "all", Mode: "flat", Value: 1, Direction: "down"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Items[0].Price != 9.0 {
		t.Fatalf("burger: want 9.00 got %.2f", got[0].Items[0].Price)
	}
	if got[0].Items[1].Price != 3.0 {
		t.Fatalf("fries: want 3.00 got %.2f", got[0].Items[1].Price)
	}
	if prev.AffectedCount != 2 {
		t.Fatalf("affected_count: want 2 got %d", prev.AffectedCount)
	}
}

func TestComputePriceChange_RejectsResultLeqZero(t *testing.T) {
	_, _, _, _, err := ComputePriceChange(sampleMenu(),
		PriceChangeParams{Scope: "all", Mode: "flat", Value: 10, Direction: "down"})
	// fries 4 - 10 = -6 <= 0 -> error (hard reject)
	if err == nil {
		t.Fatal("expected reject for resulting price <= 0")
	}
}

func TestComputePriceChange_ReconfirmAboveFiftyPct(t *testing.T) {
	_, _, _, reconfirm, err := ComputePriceChange(sampleMenu(),
		PriceChangeParams{Scope: "all", Mode: "percent", Value: 60, Direction: "up"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reconfirm {
		t.Fatal("expected requires_reconfirm for >50% swing")
	}
}

func TestComputePriceChange_ExactlyFiftyPctNoReconfirm(t *testing.T) {
	// Single-item scope so this exercises ONLY the swing boundary (menu-wide
	// scope now always reconfirms regardless of swing).
	_, _, _, reconfirm, err := ComputePriceChange(sampleMenu(),
		PriceChangeParams{Scope: "item:i1", Mode: "percent", Value: 50, Direction: "up"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reconfirm {
		t.Fatal("exactly 50% should NOT trigger reconfirm (boundary)")
	}
}

func TestComputePriceChange_MenuWideAlwaysReconfirms(t *testing.T) {
	// Blast-radius gate (audit L4-16): a scope-'all' change touching more than
	// one item requires explicit reconfirmation even for a tiny swing, so a
	// menu-wide change the owner never asked for can't ride through on a single
	// click.
	_, _, _, reconfirm, err := ComputePriceChange(sampleMenu(),
		PriceChangeParams{Scope: "all", Mode: "percent", Value: 5, Direction: "up"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reconfirm {
		t.Fatal("scope 'all' affecting >1 items must require reconfirm regardless of swing size")
	}
}

func TestComputePriceChange_ScopeItem(t *testing.T) {
	got, prev, _, _, err := ComputePriceChange(sampleMenu(),
		PriceChangeParams{Scope: "item:i1", Mode: "percent", Value: 20, Direction: "up"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Items[0].Price != 12.0 {
		t.Fatalf("burger: want 12.00 got %.2f", got[0].Items[0].Price)
	}
	// fries untouched
	if got[0].Items[1].Price != 4.0 {
		t.Fatalf("fries should be unchanged: want 4.00 got %.2f", got[0].Items[1].Price)
	}
	if prev.AffectedCount != 1 {
		t.Fatalf("affected_count: want 1 got %d", prev.AffectedCount)
	}
}

func TestComputePriceChange_ScopeCategory(t *testing.T) {
	got, prev, _, _, err := ComputePriceChange(sampleMenu(),
		PriceChangeParams{Scope: "category:cat-1", Mode: "flat", Value: 1, Direction: "up"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Items[0].Price != 11.0 {
		t.Fatalf("burger: want 11.00 got %.2f", got[0].Items[0].Price)
	}
	if got[0].Items[1].Price != 5.0 {
		t.Fatalf("fries: want 5.00 got %.2f", got[0].Items[1].Price)
	}
	if prev.AffectedCount != 2 {
		t.Fatalf("affected_count: want 2 got %d", prev.AffectedCount)
	}
}

func TestComputePriceChange_Rounding(t *testing.T) {
	// 4.0 * 1.333... = 5.333... → rounded to 5.33
	got, _, _, _, err := ComputePriceChange(sampleMenu(),
		PriceChangeParams{Scope: "item:i2", Mode: "percent", Value: 33.333, Direction: "up"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 4.0 + 4.0*(33.333/100) = 4.0 + 1.33332 = 5.33332 → 5.33
	if got[0].Items[1].Price != 5.33 {
		t.Fatalf("fries: want 5.33 got %.4f", got[0].Items[1].Price)
	}
}

func TestComputePriceChange_InputUnchanged(t *testing.T) {
	original := sampleMenu()
	_, _, _, _, _ = ComputePriceChange(original,
		PriceChangeParams{Scope: "all", Mode: "percent", Value: 20, Direction: "up"})
	// Input must be unchanged (deep copy verified)
	if original[0].Items[0].Price != 10.0 {
		t.Fatal("compute mutated the input categories (deep-copy bug)")
	}
	if original[0].Items[1].Price != 4.0 {
		t.Fatal("compute mutated the input categories (deep-copy bug)")
	}
}

func TestComputePriceChange_ScopeItemNotFound(t *testing.T) {
	_, _, warnings, _, err := ComputePriceChange(sampleMenu(),
		PriceChangeParams{Scope: "item:nonexistent", Mode: "percent", Value: 20, Direction: "up"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No items affected but not an error; warnings should mention nothing affected or 0 count.
	_ = warnings
}

// --- Availability tests ---

func TestComputeAvailabilityChange_AllOff(t *testing.T) {
	got, prev, _, reconfirm, err := ComputeAvailabilityChange(sampleMenu(),
		AvailabilityParams{Target: "all", Available: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reconfirm {
		t.Fatal("expected reconfirm when turning all off")
	}
	if prev.AffectedCount != 2 {
		t.Fatalf("affected_count: want 2 got %d", prev.AffectedCount)
	}
	for _, item := range got[0].Items {
		if item.IsAvailable {
			t.Fatalf("expected all items unavailable after all-off")
		}
	}
}

func TestComputeAvailabilityChange_AllOn_NoReconfirm(t *testing.T) {
	// Start with all off
	menu := sampleMenu()
	menu[0].Items[0].IsAvailable = false
	menu[0].Items[1].IsAvailable = false

	_, _, _, reconfirm, err := ComputeAvailabilityChange(menu,
		AvailabilityParams{Target: "all", Available: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reconfirm {
		t.Fatal("turning all ON should not require reconfirm")
	}
}

func TestComputeAvailabilityChange_ScopeItem(t *testing.T) {
	got, prev, _, _, err := ComputeAvailabilityChange(sampleMenu(),
		AvailabilityParams{Target: "item:i1", Available: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Items[0].IsAvailable {
		t.Fatal("burger should be unavailable")
	}
	if !got[0].Items[1].IsAvailable {
		t.Fatal("fries should still be available")
	}
	if prev.AffectedCount != 1 {
		t.Fatalf("affected_count: want 1 got %d", prev.AffectedCount)
	}
}

func TestComputeAvailabilityChange_InputUnchanged(t *testing.T) {
	original := sampleMenu()
	_, _, _, _, _ = ComputeAvailabilityChange(original,
		AvailabilityParams{Target: "all", Available: false})
	if !original[0].Items[0].IsAvailable {
		t.Fatal("compute mutated input availability (deep-copy bug)")
	}
}

// --- Content tests ---

func TestComputeContentEdit_UpdateDescription(t *testing.T) {
	desc := "Juicy beef patty"
	got, prev, _, _, err := ComputeContentEdit(sampleMenu(),
		ContentEditParams{ItemID: "i1", Description: &desc})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Items[0].Description != desc {
		t.Fatalf("description: want %q got %q", desc, got[0].Items[0].Description)
	}
	if prev.AffectedCount != 1 {
		t.Fatalf("affected_count: want 1 got %d", prev.AffectedCount)
	}
}

func TestComputeContentEdit_UpdateDietaryTags(t *testing.T) {
	tags := []string{"vegan", "gluten-free"}
	got, _, _, _, err := ComputeContentEdit(sampleMenu(),
		ContentEditParams{ItemID: "i1", DietaryTags: &tags})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got[0].Items[0].DietaryTags) != 2 {
		t.Fatalf("dietary_tags: want 2 got %d", len(got[0].Items[0].DietaryTags))
	}
}

func TestComputeContentEdit_RejectsOverlongDescription(t *testing.T) {
	long := strings.Repeat("x", DescriptionMaxRunes+1)
	_, _, _, _, err := ComputeContentEdit(sampleMenu(),
		ContentEditParams{ItemID: "i1", Description: &long})
	if err == nil {
		t.Fatal("expected reject for overlong description")
	}
}

func TestComputeContentEdit_RejectsNonPrintableRune(t *testing.T) {
	bad := "Hello\x00World"
	_, _, _, _, err := ComputeContentEdit(sampleMenu(),
		ContentEditParams{ItemID: "i1", Description: &bad})
	if err == nil {
		t.Fatal("expected reject for non-printable rune in description")
	}
}

func TestComputeContentEdit_RejectsNonCanonicalDietaryTag(t *testing.T) {
	tags := []string{"definitely-safe"}
	_, _, _, _, err := ComputeContentEdit(sampleMenu(),
		ContentEditParams{ItemID: "i1", DietaryTags: &tags})
	if err == nil {
		t.Fatal("expected reject for non-canonical dietary tag")
	}
}

func TestComputeContentEdit_NeverTouchesAllergens(t *testing.T) {
	in := sampleMenu()
	in[0].Items[0].Allergens = []string{"gluten"}
	desc := "Juicy"
	got, _, _, _, err := ComputeContentEdit(in,
		ContentEditParams{ItemID: "i1", Description: &desc})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// allergens unchanged on the mutated copy.
	if len(got[0].Items[0].Allergens) != 1 || got[0].Items[0].Allergens[0] != "gluten" {
		t.Fatal("content edit must not modify allergens")
	}
}

func TestComputeContentEdit_AllergenIsolation_MutatedCopyDoesNotShareSlice(t *testing.T) {
	in := sampleMenu()
	in[0].Items[0].Allergens = []string{"gluten"}
	desc := "Juicy"
	got, _, _, _, _ := ComputeContentEdit(in,
		ContentEditParams{ItemID: "i1", Description: &desc})
	// Mutate the original allergen slice after the call; copy must not see the change.
	in[0].Items[0].Allergens[0] = "peanut"
	if got[0].Items[0].Allergens[0] != "gluten" {
		t.Fatal("mutated copy shares allergen slice with input (deep-copy bug)")
	}
}

func TestComputeContentEdit_ItemNotFound(t *testing.T) {
	desc := "New description"
	_, _, _, _, err := ComputeContentEdit(sampleMenu(),
		ContentEditParams{ItemID: "nonexistent", Description: &desc})
	if err == nil {
		t.Fatal("expected error for item not found")
	}
}

func TestComputeContentEdit_InputUnchanged(t *testing.T) {
	original := sampleMenu()
	desc := "Changed"
	_, _, _, _, _ = ComputeContentEdit(original,
		ContentEditParams{ItemID: "i1", Description: &desc})
	if original[0].Items[0].Description != "" {
		t.Fatal("compute mutated input description (deep-copy bug)")
	}
}

// --- DiffSnapshots / RestoreSnapshots tests ---

func TestDiffSnapshots_OnlyChangedItemsAppear(t *testing.T) {
	before := sampleMenu()
	after := sampleMenu()
	// Change only burger price.
	after[0].Items[0].Price = 15.0

	changedBefore, changedAfter := DiffSnapshots(before, after)
	if len(changedBefore) != 1 {
		t.Fatalf("want 1 changed item before, got %d", len(changedBefore))
	}
	if len(changedAfter) != 1 {
		t.Fatalf("want 1 changed item after, got %d", len(changedAfter))
	}
	if changedBefore[0].Price != 10.0 {
		t.Fatalf("before price: want 10.0 got %v", changedBefore[0].Price)
	}
	if changedAfter[0].Price != 15.0 {
		t.Fatalf("after price: want 15.0 got %v", changedAfter[0].Price)
	}
	if changedBefore[0].ItemID != "i1" {
		t.Fatalf("item_id: want i1 got %s", changedBefore[0].ItemID)
	}
}

func TestDiffSnapshots_UnchangedItemsOmitted(t *testing.T) {
	before := sampleMenu()
	after := sampleMenu()
	// No changes at all.
	changedBefore, changedAfter := DiffSnapshots(before, after)
	if len(changedBefore) != 0 || len(changedAfter) != 0 {
		t.Fatalf("expected no diff for identical menus, got %d/%d", len(changedBefore), len(changedAfter))
	}
}

func TestRestoreSnapshots_RestoresPriceAvailabilityDescriptionDietaryTags(t *testing.T) {
	original := sampleMenu()
	// Save snapshot of original state
	snap, _ := DiffSnapshots(original, original)
	// Build a snapshot manually
	snaps := []ItemSnapshot{{
		CategoryID:  "cat-1",
		ItemID:      "i1",
		Price:       10.0,
		IsAvailable: true,
		Description: "Original description",
		DietaryTags: []string{"mild"},
	}}

	// Apply changes to a copy
	modified := sampleMenu()
	modified[0].Items[0].Price = 20.0
	modified[0].Items[0].IsAvailable = false
	modified[0].Items[0].Description = "Modified description"
	modified[0].Items[0].DietaryTags = []string{"vegan"}
	modified[0].Items[0].Allergens = []string{"peanut"}

	restored := RestoreSnapshots(modified, snaps)
	item := restored[0].Items[0]
	if item.Price != 10.0 {
		t.Fatalf("price: want 10.0 got %.1f", item.Price)
	}
	if !item.IsAvailable {
		t.Fatal("is_available: want true")
	}
	if item.Description != "Original description" {
		t.Fatalf("description: want 'Original description' got %q", item.Description)
	}
	if len(item.DietaryTags) != 1 || item.DietaryTags[0] != "mild" {
		t.Fatalf("dietary_tags: want [mild] got %v", item.DietaryTags)
	}
	// Allergens must NOT be restored (allergen stance: AI never touches them).
	if len(item.Allergens) != 1 || item.Allergens[0] != "peanut" {
		t.Fatalf("allergens must NOT be restored, want [peanut] got %v", item.Allergens)
	}
	_ = snap
}

func TestRestoreSnapshots_UnknownItemIDIgnored(t *testing.T) {
	snaps := []ItemSnapshot{{
		CategoryID: "cat-1",
		ItemID:     "nonexistent",
		Price:      99.0,
	}}
	modified := sampleMenu()
	restored := RestoreSnapshots(modified, snaps)
	// Items should be unchanged since snap item_id doesn't exist
	if restored[0].Items[0].Price != 10.0 {
		t.Fatalf("price should be unchanged: want 10.0 got %.1f", restored[0].Items[0].Price)
	}
}

// --- Canonical dietary tag guard ---

func TestDietaryTagsMatchCanonical(t *testing.T) {
	// This set MUST be kept in sync with internal/services/menu_enum_validation.go:canonicalDietaryTagIDs.
	expected := map[string]struct{}{
		"vegan": {}, "vegetarian": {}, "gluten-free": {}, "dairy-free": {},
		"nut-free": {}, "mild": {}, "low-sodium": {},
	}
	for tag := range expected {
		if _, ok := canonicalDietaryTags[tag]; !ok {
			t.Errorf("canonicalDietaryTags missing expected tag %q (diverged from menu_enum_validation.go)", tag)
		}
	}
	for tag := range canonicalDietaryTags {
		if _, ok := expected[tag]; !ok {
			t.Errorf("canonicalDietaryTags has extra tag %q not in expected canonical set", tag)
		}
	}
}

// --- Title/Description helpers ---

func TestPriceChangeTitle(t *testing.T) {
	p := PriceChangeParams{Scope: "all", Mode: "percent", Value: 20, Direction: "up"}
	title := PriceChangeTitle(p)
	if title == "" {
		t.Fatal("expected non-empty title")
	}
}

func TestAvailabilityTitle(t *testing.T) {
	p := AvailabilityParams{Target: "all", Available: false}
	title := AvailabilityChangeTitle(p)
	if title == "" {
		t.Fatal("expected non-empty title")
	}
}

func TestContentEditTitle(t *testing.T) {
	desc := "Juicy"
	p := ContentEditParams{ItemID: "i1", Description: &desc}
	title := ContentEditTitle(p)
	if title == "" {
		t.Fatal("expected non-empty title")
	}
}

// TestComputePriceChange_UsesBusinessCurrencySymbol verifies proposal copy reads
// in the business currency rather than a hardcoded "$" (audit C6), and that the
// default (no currency arg) preserves the legacy USD "$" behavior.
func TestComputePriceChange_UsesBusinessCurrencySymbol(t *testing.T) {
	params := PriceChangeParams{Scope: "all", Mode: "flat", Value: 50, Direction: "up"}

	_, prev, _, _, err := ComputePriceChange(sampleMenu(), params, "AED")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(prev.Summary, "AED 50.00") {
		t.Fatalf("summary should read in AED, got: %q", prev.Summary)
	}
	if strings.Contains(prev.Summary, "$") {
		t.Fatalf("summary must not contain a hardcoded $, got: %q", prev.Summary)
	}
	if title := PriceChangeTitle(params, "AED"); !strings.Contains(title, "AED 50.00") || strings.Contains(title, "$") {
		t.Fatalf("title should read in AED without $, got: %q", title)
	}

	// Default (no currency) keeps the USD "$" glyph for backward compatibility.
	_, prevUSD, _, _, _ := ComputePriceChange(sampleMenu(), params)
	if !strings.Contains(prevUSD.Summary, "$50.00") {
		t.Fatalf("default summary should keep $ for USD, got: %q", prevUSD.Summary)
	}
}
