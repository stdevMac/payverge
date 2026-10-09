package services

import (
	"context"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

func tenTurnHistory() []WaiterMessage {
	h := make([]WaiterMessage, 0, 20)
	qs := []string{
		"What do you recommend?", "Any vegetarian mains?", "What's in Fixture Dish 12?",
		"Does Fixture Dish 9 contain nuts?", "Add Fixture Dish 3 to my cart",
		"What pairs with it?", "How much is my order?", "Any desserts?",
		"What's the most popular item?", "Add a drink please",
	}
	for _, q := range qs {
		h = append(h, WaiterMessage{Role: "user", Content: q})
		h = append(h, WaiterMessage{Role: "assistant", Content: "Sure — here is a helpful reply about " + q})
	}
	return h
}

func BenchmarkWaiterBuildRequest(b *testing.B) {
	cp := &captureProvider{}
	svc, _ := NewAIService(cp, llm.ModelConfig{Chat: "google/gemini-2.5-flash"})
	menu := mustFixtureMenu(b)
	hist := tenTurnHistory()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.ChatWithWaiter(context.Background(), WaiterChatParams{
			AIName:              "Sage",
			AIPriority:          "balanced",
			BusinessName:        "Fixture Bistro",
			BusinessDescription: "desc",
			BusinessAddress:     "addr",
			ReservationContext:  "res",
			DeliveryContext:     "del",
			MenuData:            menu,
			OffersData:          "[]",
			BundlesData:         "[]",
			Language:            "es",
			Mode:                "ordering",
			History:             hist,
		})
	}
}

func TestTenTurnInputTokenModel(t *testing.T) {
	menu := loadFixtureMenu(t)
	sys := renderWaiterSystemPrompt(t, menu)
	prefixTokens := estTokens(sys)

	var totalInput, totalCacheable int
	hist := tenTurnHistory()
	for turn := 1; turn <= 10; turn++ {
		tail := 0
		for i := 0; i < turn*2 && i < len(hist); i++ {
			tail += estTokens(hist[i].Content)
		}
		totalInput += prefixTokens + tail
		if turn > 1 {
			totalCacheable += prefixTokens
		}
	}
	reuse := float64(totalCacheable) / float64(totalInput)
	if reuse < 0.80 {
		t.Fatalf("cacheable input reuse = %.2f, want >= 0.80 (caching value too low; "+
			"static prefix may be too small or tail too large)", reuse)
	}
	t.Logf("10-turn totalInput=%d cacheableReuse=%.2f%% prefix=%d tok",
		totalInput, reuse*100, prefixTokens)
}

func mustFixtureMenu(b *testing.B) string {
	b.Helper()
	data, err := readFixtureMenuFile()
	if err != nil {
		b.Fatalf("fixture menu: %v", err)
	}
	return data
}
