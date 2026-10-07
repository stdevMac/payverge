package services

import (
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/locales"
)

func TestWaiterPromptsCarryNoSecrets(t *testing.T) {
	forbidden := []string{"OPENROUTER_API_KEY", "sk-", "Bearer ", "postgres://", "AWS_SECRET", "://localhost", "127.0.0.1", "internal."}
	for _, loc := range locales.GuestLocales() {
		for _, mode := range []string{"ordering", "concierge", "whatsapp"} {
			sys, _ := buildWaiterSystemPrompt(WaiterChatParams{
				AIName: "Sage", BusinessName: "B", MenuData: "[]", OffersData: "[]", BundlesData: "[]",
				Language: loc.Canonical, Mode: mode,
			})
			for _, f := range forbidden {
				if strings.Contains(sys, f) {
					t.Fatalf("waiter prompt (%s/%s) leaked %q", loc.Canonical, mode, f)
				}
			}
		}
	}
}
