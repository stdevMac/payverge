package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

const (
	promptTokenCeiling = 24000
	cacheMinTokenFloor = 1024
)

func estTokens(s string) int { return len(s) / 4 }

func readFixtureMenuFile() (string, error) {
	b, err := os.ReadFile(filepath.Join("..", "llm", "testdata", "fixture_menu.json"))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func loadFixtureMenu(t *testing.T) string {
	t.Helper()
	s, err := readFixtureMenuFile()
	if err != nil {
		t.Fatalf("read fixture menu: %v", err)
	}
	return s
}

type captureProvider struct{ last llm.GenerateRequest }

func (c *captureProvider) Generate(_ context.Context, req llm.GenerateRequest) (*llm.Response, error) {
	c.last = req
	return &llm.Response{Text: "ok", Model: req.Model, Usage: llm.Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2}}, nil
}

func renderWaiterSystemPrompt(t *testing.T, menu string) string {
	t.Helper()
	cp := &captureProvider{}
	svc, err := NewAIService(cp, llm.ModelConfig{Chat: "google/gemini-2.5-flash"})
	if err != nil {
		t.Fatalf("NewAIService: %v", err)
	}
	_, err = svc.ChatWithWaiter(context.Background(), WaiterChatParams{
		AIName:              "Sage",
		AIPriority:          "balanced",
		SpecialInstructions: "Be warm.",
		BillContext:         "",
		BusinessName:        "Fixture Bistro",
		BusinessDescription: "A cozy fixture restaurant.",
		BusinessAddress:     "123 Test St",
		ReservationContext:  "Reservations available",
		DeliveryContext:     "Delivery available",
		MenuData:            menu,
		OffersData:          "[]",
		BundlesData:         "[]",
		Language:            "es",
		Mode:                "ordering",
		History:             []WaiterMessage{{Role: "user", Content: "What do you recommend?"}},
	})
	if err != nil {
		t.Fatalf("ChatWithWaiter: %v", err)
	}
	return cp.last.System
}

func TestWaiterPromptUnderTokenCeiling(t *testing.T) {
	sys := renderWaiterSystemPrompt(t, loadFixtureMenu(t))
	if got := estTokens(sys); got >= promptTokenCeiling {
		t.Fatalf("waiter system prompt = ~%d tokens, exceeds ceiling %d", got, promptTokenCeiling)
	}
}

func TestWaiterStaticPrefixExceedsCacheMinimum(t *testing.T) {
	menu := loadFixtureMenu(t)
	sys := renderWaiterSystemPrompt(t, menu)
	idx := strings.Index(sys, "Fixture Dish 80")
	if idx < 0 {
		t.Fatalf("menu JSON not found in system prompt; static-first ordering broken")
	}
	prefix := sys[:idx+len("Fixture Dish 80")]
	if got := estTokens(prefix); got < cacheMinTokenFloor {
		t.Fatalf("static cache prefix = ~%d tokens, below cache floor %d", got, cacheMinTokenFloor)
	}
}

func TestWaiterStaticContentPrecedesVariableContent(t *testing.T) {
	menu := loadFixtureMenu(t)
	const billSentinel = "BILLCTX-SENTINEL-7F3A"
	cp := &captureProvider{}
	svc, err := NewAIService(cp, llm.ModelConfig{Chat: "google/gemini-2.5-flash"})
	if err != nil {
		t.Fatalf("NewAIService: %v", err)
	}
	_, err = svc.ChatWithWaiter(context.Background(), WaiterChatParams{
		AIName:              "Sage",
		AIPriority:          "balanced",
		SpecialInstructions: "Be warm.",
		BillContext:         billSentinel,
		BusinessName:        "Fixture Bistro",
		BusinessDescription: "A cozy fixture restaurant.",
		BusinessAddress:     "123 Test St",
		ReservationContext:  "Reservations available",
		DeliveryContext:     "Delivery available",
		MenuData:            menu,
		OffersData:          "[]",
		BundlesData:         "[]",
		Language:            "es",
		Mode:                "ordering",
		History:             []WaiterMessage{{Role: "user", Content: "What do you recommend?"}},
	})
	if err != nil {
		t.Fatalf("ChatWithWaiter: %v", err)
	}
	sys := cp.last.System
	menuIdx := strings.LastIndex(sys, "Fixture Dish 80")
	if menuIdx < 0 {
		t.Fatalf("menu region not found in system prompt")
	}
	billIdx := strings.Index(sys, billSentinel)
	if billIdx < 0 {
		t.Skipf("bill context not present in system string (moved to messages) — prefix is fully static, OK")
	}
	if billIdx < menuIdx {
		t.Fatalf("variable bill context (idx %d) precedes the menu region (idx %d); "+
			"static-first ordering broken — variable content must follow the cacheable prefix", billIdx, menuIdx)
	}
}

func TestWaiterRequestEnablesCacheControl(t *testing.T) {
	cp := &captureProvider{}
	svc, err := NewAIService(cp, llm.ModelConfig{Chat: "google/gemini-2.5-flash"})
	if err != nil {
		t.Fatalf("NewAIService: %v", err)
	}
	_, err = svc.ChatWithWaiter(context.Background(), WaiterChatParams{
		AIName:              "Sage",
		AIPriority:          "balanced",
		BusinessName:        "Fixture Bistro",
		BusinessDescription: "desc",
		BusinessAddress:     "addr",
		ReservationContext:  "res",
		DeliveryContext:     "del",
		MenuData:            loadFixtureMenu(t),
		OffersData:          "[]",
		BundlesData:         "[]",
		Language:            "es",
		Mode:                "ordering",
		History:             []WaiterMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("ChatWithWaiter: %v", err)
	}
	if !cp.last.CacheControl {
		t.Fatalf("waiter request must set CacheControl=true for prompt caching")
	}
}
