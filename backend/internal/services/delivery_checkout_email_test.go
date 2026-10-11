package services

import (
	"strings"
	"testing"
)

func TestGuestCheckout_RequiresEmail(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	_, err := svc.GuestDeliveryCheckout(businessID, GuestDeliveryCheckoutRequest{
		CustomerName:  "G",
		CustomerPhone: "1",
		Items:         []DeliveryCheckoutItemInput{{MenuItemName: "Pizza", Quantity: 1, Price: 10}},
	})
	if err == nil || !strings.Contains(err.Error(), "email") {
		t.Fatalf("checkout without email must fail with an email error, got %v", err)
	}

	// Malformed addresses must also be rejected.
	for _, bad := range []string{"user@@host", "user@"} {
		_, err = svc.GuestDeliveryCheckout(businessID, GuestDeliveryCheckoutRequest{
			CustomerName:  "G",
			CustomerPhone: "1",
			CustomerEmail: bad,
			Items:         []DeliveryCheckoutItemInput{{MenuItemName: "Pizza", Quantity: 1, Price: 10}},
		})
		if err == nil || !strings.Contains(err.Error(), "email") {
			t.Fatalf("checkout with malformed email %q must fail with an email error, got %v", bad, err)
		}
	}
}

func TestLocalizedDeliveryLifecycleMessages(t *testing.T) {
	// Every new lifecycle template must exist in both locales and produce
	// non-empty bodies. Additionally, en and es bodies must be distinct —
	// a copy-paste of the English text into the Spanish branch would fail here.
	type templatePair struct {
		name  string
		build func(locale NotificationLocale) (string, string)
	}
	templates := []templatePair{
		{"received", func(l NotificationLocale) (string, string) {
			return localizedReceivedMessage("DEL-X", l)
		}},
		{"acceptedPay", func(l NotificationLocale) (string, string) {
			return localizedAcceptedPayMessage("DEL-X", "https://payverge.io/delivery/DEL-X/pay", 15, l)
		}},
		{"acceptedCOD", func(l NotificationLocale) (string, string) {
			return localizedAcceptedCODMessage("DEL-X", "$17.00", l)
		}},
		{"paymentReceived", func(l NotificationLocale) (string, string) {
			return localizedPaymentReceivedMessage("DEL-X", l)
		}},
		{"expired", func(l NotificationLocale) (string, string) {
			return localizedExpiredMessage("DEL-X", l)
		}},
	}

	for _, tmpl := range templates {
		enTitle, enBody := tmpl.build(LocaleEN)
		esTitle, esBody := tmpl.build(LocaleES)

		for _, locale := range []NotificationLocale{LocaleEN, LocaleES} {
			title, body := tmpl.build(locale)
			if strings.TrimSpace(title) == "" || strings.TrimSpace(body) == "" {
				t.Fatalf("template %q: empty output for locale %v", tmpl.name, locale)
			}
		}

		// en and es must produce distinct body copy (and titles for most templates).
		if enBody == esBody {
			t.Errorf("template %q: en and es bodies are identical — missing Spanish translation", tmpl.name)
		}
		if enTitle == esTitle {
			t.Errorf("template %q: en and es titles are identical — missing Spanish translation", tmpl.name)
		}
	}
}
