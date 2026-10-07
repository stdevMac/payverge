package services

import (
	"strings"
	"testing"
)

func TestWaiterGreeting_AllGuestLocalesIncludeDisclosure(t *testing.T) {
	codes := []string{"ar", "da", "de", "en", "es", "es-AR", "fr", "hi", "it", "ja", "ko", "nl", "no", "pl", "pt", "ru", "sv", "th", "tr", "vi", "zh"}
	for _, code := range codes {
		g := WaiterGreeting(code, "Sage", "Bistro", "ordering", false)
		if g == "" {
			t.Fatalf("empty greeting for %q", code)
		}
		if !strings.Contains(g, "Sage") || !strings.Contains(g, "Bistro") {
			t.Fatalf("greeting for %q did not interpolate name/business: %q", code, g)
		}
	}
}

func TestWaiterGreeting_EnglishMentionsAI(t *testing.T) {
	g := WaiterGreeting("en", "Sage", "Bistro", "ordering", false)
	if !strings.Contains(strings.ToLower(g), "ai") {
		t.Fatalf("english greeting must carry the AI-disclosure sentence: %q", g)
	}
}

func TestWaiterGreeting_ClosedBrowseOnlyDoesNotOfferOrdering(t *testing.T) {
	codes := []string{"ar", "da", "de", "en", "es", "es-AR", "fr", "hi", "it", "ja", "ko", "nl", "no", "pl", "pt", "ru", "sv", "th", "tr", "vi", "zh"}
	// Phrases that claim the guest can place an order now (open templates).
	// Closed templates must not contain these.
	orderingOffers := []string{
		"help ordering",
		"hjælp til at bestille",
		"Hilfe bei der Bestellung",
		"ayuda para pedir",
		"aide pour commander",
		"aiuto per ordinare",
		"ajuda para pedir",
		"hulp bij het bestellen",
		"hjelp til å bestille",
		"hjälp att beställa",
		"pomoc w zamówieniu",
		"помощь с заказом",
		"sipariş yardımı",
		"مساعدة في الطلب",
		"ऑर्डर करने में मदद",
		"注文のお手伝い",
		"주문 도움",
		"点餐帮助",
		"ความช่วยเหลือในการสั่ง",
		"trợ giúp gọi món",
	}
	for _, code := range codes {
		g := WaiterGreeting(code, "Sage", "Bistro", "ordering", true)
		if g == "" {
			t.Fatalf("empty closed greeting for %q", code)
		}
		if !strings.Contains(g, "Sage") || !strings.Contains(g, "Bistro") {
			t.Fatalf("closed greeting for %q did not interpolate: %q", code, g)
		}
		lower := strings.ToLower(g)
		for _, phrase := range orderingOffers {
			if strings.Contains(lower, strings.ToLower(phrase)) {
				t.Fatalf("closed greeting for %q still offers ordering via %q: %q", code, phrase, g)
			}
		}
	}
	// English closed copy must still disclose AI and point at the menu.
	en := WaiterGreeting("en", "Sage", "Bistro", "ordering", true)
	if !strings.Contains(strings.ToLower(en), "ai") {
		t.Fatalf("closed english greeting must disclose AI: %q", en)
	}
	if !strings.Contains(strings.ToLower(en), "menu") {
		t.Fatalf("closed english greeting must invite menu questions: %q", en)
	}
	if strings.Contains(strings.ToLower(en), "help ordering") {
		t.Fatalf("closed english greeting must not offer help ordering: %q", en)
	}
}

func TestWaiterGreeting_ConciergeIgnoresBrowseOnlyFlag(t *testing.T) {
	// Concierge never takes orders; closed flag must not force a missing closed template path.
	g := WaiterGreeting("en", "Sage", "Bistro", "concierge", true)
	if g == "" {
		t.Fatal("empty concierge greeting")
	}
	lower := strings.ToLower(g)
	if strings.Contains(lower, "help ordering") {
		t.Fatalf("concierge greeting must not offer table ordering: %q", g)
	}
	if !strings.Contains(lower, "hours") && !strings.Contains(lower, "reserv") {
		t.Fatalf("concierge greeting should mention hours or reservations: %q", g)
	}
}

func TestWaiterGreeting_ConciergeAllGuestLocales(t *testing.T) {
	codes := []string{"ar", "da", "de", "en", "es", "es-AR", "fr", "hi", "it", "ja", "ko", "nl", "no", "pl", "pt", "ru", "sv", "th", "tr", "vi", "zh"}
	for _, code := range codes {
		g := WaiterGreeting(code, "Sage", "Bistro", "concierge", false)
		if g == "" {
			t.Fatalf("empty concierge greeting for %q", code)
		}
		if !strings.Contains(g, "Sage") || !strings.Contains(g, "Bistro") {
			t.Fatalf("concierge greeting for %q did not interpolate: %q", code, g)
		}
	}
}

func TestWaiterScriptedGreeting_AllGuestLocalesDenyAI(t *testing.T) {
	codes := []string{"ar", "da", "de", "en", "es", "es-AR", "fr", "hi", "it", "ja", "ko", "nl", "no", "pl", "pt", "ru", "sv", "th", "tr", "vi", "zh"}
	for _, code := range codes {
		raw, err := waiterScriptedGreetingFS.ReadFile("prompts/ai_waiter/greetings/scripted/" + code + ".md")
		if err != nil {
			t.Fatalf("missing scripted greeting asset for %q: %v", code, err)
		}
		if len(strings.TrimSpace(string(raw))) == 0 {
			t.Fatalf("empty scripted greeting asset for %q", code)
		}
		g := WaiterScriptedGreeting(code, "Mozo", "Bistro")
		if !strings.Contains(g, "Mozo") || !strings.Contains(g, "Bistro") {
			t.Fatalf("scripted greeting for %q did not interpolate: %q", code, g)
		}
	}
	en := WaiterScriptedGreeting("en", "Mozo", "Bistro")
	lower := strings.ToLower(en)
	if strings.Contains(lower, "ai assistant") || strings.Contains(lower, "chatting with an ai") {
		t.Fatalf("scripted english greeting must not present the helper as AI: %q", en)
	}
	if !strings.Contains(lower, "not an ai") {
		t.Fatalf("scripted english greeting must say it is not an AI: %q", en)
	}
	if strings.Contains(lower, "help ordering") {
		t.Fatalf("scripted greeting must not offer ordering help: %q", en)
	}
}
