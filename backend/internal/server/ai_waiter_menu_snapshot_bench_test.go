package server

import (
	"fmt"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// benchWaiterMenu builds a realistically sized restaurant menu (categories x
// items) in both a source and a translated projection, so the benchmarks below
// measure the guest-chat path with the alias/dietary index actually populated.
func benchWaiterMenu(categories, itemsPerCategory int) (source, display []database.MenuCategory) {
	source = make([]database.MenuCategory, 0, categories)
	display = make([]database.MenuCategory, 0, categories)
	tags := [][]string{nil, {"vegetarian"}, {"vegan", "vegetarian"}, {"gluten-free"}}
	for c := 0; c < categories; c++ {
		srcItems := make([]database.MenuItem, 0, itemsPerCategory)
		dspItems := make([]database.MenuItem, 0, itemsPerCategory)
		for i := 0; i < itemsPerCategory; i++ {
			id := fmt.Sprintf("item-%d-%d", c, i)
			srcItems = append(srcItems, database.MenuItem{
				ID: id, Name: fmt.Sprintf("Signature Dish %d %d", c, i), Price: 12.5,
				IsAvailable: i%7 != 0, DietaryTags: tags[i%len(tags)], SortOrder: i + 1,
			})
			dspItems = append(dspItems, database.MenuItem{
				ID: id, Name: fmt.Sprintf("Plato especial %d %d", c, i), Price: 12.5,
				IsAvailable: i%7 != 0, DietaryTags: tags[i%len(tags)], SortOrder: i + 1,
			})
		}
		source = append(source, database.MenuCategory{
			ID: fmt.Sprintf("cat-%d", c), Name: fmt.Sprintf("Category %d", c), SortOrder: c + 1, Items: srcItems,
		})
		display = append(display, database.MenuCategory{
			ID: fmt.Sprintf("cat-%d", c), Name: fmt.Sprintf("Categoria %d", c), SortOrder: c + 1, Items: dspItems,
		})
	}
	return source, display
}

func benchWaiterSnapshotInput(withSource bool) WaiterMenuSnapshotInput {
	source, display := benchWaiterMenu(12, 12)
	in := WaiterMenuSnapshotInput{
		Business:      &database.Business{ID: 85, DefaultLanguage: "en", IsActive: true, KitchenEnabled: true, OrdersEnabled: true},
		Locale:        "es-AR",
		Mode:          "concierge",
		Categories:    display,
		HiddenItemIDs: map[string]bool{},
	}
	if withSource {
		in.SourceCategories = source
	}
	return in
}

// BenchmarkBuildWaiterMenuSnapshot_WithoutSourceIndex is the pre-#577 baseline:
// the snapshot build with no source-name/dietary index.
func BenchmarkBuildWaiterMenuSnapshot_WithoutSourceIndex(b *testing.B) {
	in := benchWaiterSnapshotInput(false)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := BuildWaiterMenuSnapshot(in); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBuildWaiterMenuSnapshot_WithSourceIndex measures the #577 cost of
// indexing source aliases + canonical dietary tags at snapshot build time.
func BenchmarkBuildWaiterMenuSnapshot_WithSourceIndex(b *testing.B) {
	in := benchWaiterSnapshotInput(true)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := BuildWaiterMenuSnapshot(in); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWaiterMenuSnapshot_MatchEntitiesInText covers the per-turn entity
// resolution used by pairing/alternative intents.
func BenchmarkWaiterMenuSnapshot_MatchEntitiesInText(b *testing.B) {
	snapshot, err := BuildWaiterMenuSnapshot(benchWaiterSnapshotInput(true))
	if err != nil {
		b.Fatal(err)
	}
	const message = "Que me conseillez-vous avec le Signature Dish 7 5 ?"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if len(snapshot.MatchEntitiesInText(message)) == 0 {
			b.Fatal("expected a match")
		}
	}
}

// BenchmarkWaiterMenuSnapshot_RecommendableEntities covers the per-turn
// recommendation retrieval, including the dietary-filtered variant.
func BenchmarkWaiterMenuSnapshot_RecommendableEntities(b *testing.B) {
	snapshot, err := BuildWaiterMenuSnapshot(benchWaiterSnapshotInput(true))
	if err != nil {
		b.Fatal(err)
	}
	b.Run("unfiltered", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if len(snapshot.RecommendableEntities(nil, maxWaiterRecommendationEntities)) == 0 {
				b.Fatal("empty recommendation retrieval")
			}
		}
	})
	b.Run("vegetarian", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if len(snapshot.RecommendableEntities([]string{"vegetarian"}, maxWaiterRecommendationEntities)) == 0 {
				b.Fatal("empty dietary recommendation retrieval")
			}
		}
	})
}
