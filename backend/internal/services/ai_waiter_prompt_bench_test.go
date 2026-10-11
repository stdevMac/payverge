package services

import (
	"context"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

func BenchmarkBuildWaiterSystemPrompt(b *testing.B) {
	menu := "[" + strings.Repeat(`{"name":"Item","description":"desc","allergens":["gluten"],"price":9.5},`, 80) + `{"name":"Last"}]`
	p := WaiterChatParams{
		AIName: "Sage", AIPriority: "balanced", SpecialInstructions: "be kind",
		BusinessName: "Bistro", BusinessDescription: "Great food", BusinessAddress: "1 St",
		ReservationContext: "ENABLED", DeliveryContext: "ENABLED",
		MenuData: menu, OffersData: "[]", BundlesData: "[]",
		Language: "ja", Mode: "ordering",
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = buildWaiterSystemPrompt(p)
	}
}

func BenchmarkChatWithWaiterAssembly(b *testing.B) {
	rec := &recordingProvider{}
	svc, _ := NewAIService(rec, llm.ModelConfig{Chat: "m", Image: "i", Director: "d", Menu: "mn"})
	menu := "[" + strings.Repeat(`{"name":"Item","allergens":["gluten"]},`, 80) + `{"name":"Last"}]`
	p := WaiterChatParams{AIName: "Sage", BusinessName: "B", MenuData: menu, OffersData: "[]", BundlesData: "[]",
		Language: "es", Mode: "ordering", History: []WaiterMessage{{Role: "user", Content: "hola"}}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.ChatWithWaiter(context.Background(), p)
	}
}
