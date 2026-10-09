package server

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
)

// Boot finding F6: the basic-mode waiter answered "Show me the menu" and
// "What pizzas do you have?" with "I couldn't find that on the menu".
func TestClassifyWaiterV2Intent_MenuListingAsks(t *testing.T) {
	snapshot := waiterV2TestSnapshot(t, "en")
	for _, msg := range []string{"Show me the menu", "What's on the menu?", "what do you have?", "Menu please"} {
		intent, _ := classifyWaiterV2Intent("en", msg, snapshot)
		assert.Equal(t, waiterIntentFullMenu, intent, msg)
	}

	pizza := waiterV2SnapshotWithItems(t, "en", []database.MenuItem{
		{ID: "margherita", Name: "Margherita", Price: 12, Currency: "USD", IsAvailable: true, SortOrder: 1},
	}, "Pizza")
	intent, _ := classifyWaiterV2Intent("en", "What pizzas do you have?", pizza)
	assert.Equal(t, waiterIntentCategory, intent)
}

// FBE-1: broad openers only mean "full menu" as the whole message; as a
// prefix they must not hijack recommendation, dietary or category asks.
func TestClassifyWaiterV2Intent_BroadOpenersDoNotHijackOtherIntents(t *testing.T) {
	snapshot := waiterV2TestSnapshot(t, "en")
	for _, msg := range []string{"What do you have?", "full menu", "What do you serve?"} {
		intent, _ := classifyWaiterV2Intent("en", msg, snapshot)
		assert.Equal(t, waiterIntentFullMenu, intent, msg)
	}
	for _, msg := range []string{"what do you have that is good", "what do you have available", "what do you have that's vegan"} {
		intent, _ := classifyWaiterV2Intent("en", msg, snapshot)
		assert.Equal(t, waiterIntentRecommendation, intent, msg)
	}
	dessert := waiterV2SnapshotWithItems(t, "en", []database.MenuItem{
		{ID: "flan", Name: "Flan", Price: 6, Currency: "USD", IsAvailable: true, SortOrder: 1},
	}, "Dessert")
	for _, msg := range []string{"what do you have for dessert", "what do you serve for dessert"} {
		intent, _ := classifyWaiterV2Intent("en", msg, dessert)
		assert.Equal(t, waiterIntentCategory, intent, msg)
	}
}

func BenchmarkClassifyWaiterV2Intent_Unmatched(b *testing.B) {
	snapshot := waiterV2TestSnapshot(&testing.T{}, "en")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		classifyWaiterV2Intent("en", "what pizzas do you have tonight", snapshot)
	}
}
