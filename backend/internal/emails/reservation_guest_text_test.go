package emails

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// capturingProvider records every message the dispatcher hands over.
type capturingProvider struct {
	mu   sync.Mutex
	msgs []EmailMessage
}

func (p *capturingProvider) Send(_ context.Context, msg EmailMessage) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.msgs = append(p.msgs, msg)
	return nil
}

const (
	relayNameMarker     = "MALLORY-NAME-Your-account-is-locked"
	relayRequestsMarker = "MALLORY-REQUESTS-visit-evil-example"
	relayPhoneMarker    = "99 MALLORY-PHONE"
	venuePhoneMarker    = "011 4000-0000"
)

// An anonymous guest can book with any address, so every guest-facing
// reservation email must leave the guest's own free text out: otherwise the
// booking form is a relay that puts attacker sentences in a victim's inbox
// from the venue's sending domain (H-relay).
func TestGuestReservationEmailsDoNotRelayGuestText(t *testing.T) {
	provider := &capturingProvider{}
	server := newBudgetTestServer(t, provider, nil)
	to := []string{"victim@example.test"}

	for _, lang := range []string{"en", "es", "es-AR"} {
		sends := map[string]func() error{
			"confirmation": func() error {
				return server.SendReservationConfirmationEmail(to, relayNameMarker, "Bistro", "Main St 1", "Mon 1", "20:00", 2, "T1",
					relayRequestsMarker, "https://example.test/r", "https://example.test/c", relayPhoneMarker, lang)
			},
			"pending": func() error {
				return server.SendReservationPendingEmail(to, relayNameMarker, "Bistro", "Main St 1", "Mon 1", "20:00", 2, "T1",
					relayRequestsMarker, "Mon 1 18:00", "https://example.test/c", relayPhoneMarker, lang)
			},
			"reminder": func() error {
				return server.SendReservationReminderEmail(to, relayNameMarker, "Bistro", "Main St 1", "Mon 1", "20:00", 2, "T1",
					relayRequestsMarker, "https://example.test/r", "https://example.test/c", venuePhoneMarker, lang)
			},
			"updated": func() error {
				return server.SendReservationUpdatedEmail(to, relayNameMarker, "Bistro", "Main St 1", "Mon 1", "20:00", 2, "T1",
					relayRequestsMarker, "https://example.test/r", "https://example.test/c", relayPhoneMarker, lang)
			},
			"cancelled": func() error {
				return server.SendReservationCancelledEmail(to, relayNameMarker, "Bistro", "Mon 1", "20:00", venuePhoneMarker, lang)
			},
			"noshow": func() error {
				return server.SendReservationNoShowEmail(to, relayNameMarker, "Bistro", "Mon 1", "20:00", venuePhoneMarker, lang)
			},
			"declined": func() error {
				return server.SendReservationDeclinedEmail(to, relayNameMarker, "Bistro", "Mon 1", "20:00", "Fully booked",
					"https://example.test/book", venuePhoneMarker, lang)
			},
		}
		for name, send := range sends {
			before := len(provider.msgs)
			require.NoError(t, send(), "%s/%s", lang, name)
			require.Len(t, provider.msgs, before+1, "%s/%s must be delivered", lang, name)
			msg := provider.msgs[before]
			for _, part := range []string{msg.Subject, msg.HTMLBody, msg.TextBody} {
				for _, marker := range []string{relayNameMarker, relayRequestsMarker, relayPhoneMarker, "MALLORY"} {
					require.NotContains(t, part, marker, "%s/%s relays guest text", lang, name)
				}
			}
			require.Contains(t, msg.HTMLBody, "Bistro", "%s/%s still names the venue", lang, name)
		}
	}

	// The venue's phone still reaches the guest where the template offers it.
	before := len(provider.msgs)
	require.NoError(t, server.SendReservationNoShowEmail(to, relayNameMarker, "Bistro", "Mon 1", "20:00", venuePhoneMarker, "en"))
	require.Contains(t, provider.msgs[before].HTMLBody, venuePhoneMarker)
}

// The operator-facing approval reminder goes to the venue, which is where the
// guest's name and requests belong.
func TestApprovalReminderStillShowsGuestTextToTheVenue(t *testing.T) {
	provider := &capturingProvider{}
	server := newBudgetTestServer(t, provider, nil)
	require.NoError(t, server.SendReservationApprovalReminderEmail([]string{"owner@example.test"}, "Owner", "Bistro",
		"Ana Guest", 2, "Mon 1", "20:00", "Mon 1 18:00", "Window seat please", "https://example.test/dash", "en"))
	require.Len(t, provider.msgs, 1)
	require.Contains(t, provider.msgs[0].HTMLBody, "Ana Guest")
	require.Contains(t, provider.msgs[0].HTMLBody, "Window seat please")
}

// Second layer: even if a sender regressed and passed the keys again, the
// guest templates themselves have no slot for guest-typed text.
func TestGuestReservationTemplatesHaveNoGuestTextSlots(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Join(filepath.Dir(currentFile), "..", "..", "email", "templates")
	langs, err := os.ReadDir(root)
	require.NoError(t, err)
	checked := 0
	for _, lang := range langs {
		if !lang.IsDir() {
			continue
		}
		for name := range guestTemplateNames {
			if !strings.HasPrefix(name, "reservation_") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(root, lang.Name(), name+".html"))
			require.NoError(t, err, "%s/%s", lang.Name(), name)
			for _, slot := range []string{".customer_name", ".special_requests"} {
				require.NotContains(t, string(raw), slot, "%s/%s renders guest text", lang.Name(), name)
			}
			checked++
		}
	}
	require.GreaterOrEqual(t, checked, 21, "eng/es/es_ar x 7 guest reservation templates")
}
