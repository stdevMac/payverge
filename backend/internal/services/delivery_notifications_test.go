package services

import (
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestNormalizeLocale(t *testing.T) {
	cases := map[string]NotificationLocale{
		"":           LocaleEN,
		"en":         LocaleEN,
		"EN":         LocaleEN,
		"en-US":      LocaleEN,
		"english":    LocaleEN,
		"fr":         LocaleEN, // unsupported → fallback
		"es":         LocaleES,
		"ES":         LocaleES,
		"es-ES":      LocaleES,
		"es-MX":      LocaleES,
		"  Spanish ": LocaleES,
		"Español":    LocaleES,
	}
	for raw, expected := range cases {
		got, _ := lookupLocaleTemplate(raw)
		if got != expected {
			t.Errorf("lookupLocaleTemplate(%q) = %q, want %q", raw, got, expected)
		}
	}
}

func TestGuestNotificationLocale_PrefersDeliveryThenBusiness(t *testing.T) {
	business := &database.Business{DefaultLanguage: "es"}
	delivery := &database.DeliveryOrder{CustomerLocale: "en"}

	if locale := guestNotificationLocale(delivery, business); locale != LocaleEN {
		t.Errorf("delivery locale should win, got %q", locale)
	}

	delivery.CustomerLocale = ""
	if locale := guestNotificationLocale(delivery, business); locale != LocaleES {
		t.Errorf("business locale should win when delivery is empty, got %q", locale)
	}

	delivery = nil
	business = nil
	if locale := guestNotificationLocale(delivery, business); locale != LocaleEN {
		t.Errorf("nil inputs should fall back to English, got %q", locale)
	}
}

// AITRANS-1 residual: es-AR must resolve to the Spanish template (Rioplatense
// is Spanish), not silently collapse to English.
func TestNormalizeLocale_PreservesArgentineSpanish(t *testing.T) {
	for _, raw := range []string{"es-AR", "es_AR", "es-ar"} {
		if got, _ := lookupLocaleTemplate(raw); got != LocaleES {
			t.Errorf("lookupLocaleTemplate(%q) = %q, want es (Rioplatense is Spanish)", raw, got)
		}
	}
}

// AITRANS-1 residual: a customer locale we have no template for (e.g. "ja")
// must NOT be silently treated as English — it must fall back to the BUSINESS
// DEFAULT language's template, and only then to English.
func TestGuestNotificationLocale_UnknownCustomerLocaleFallsBackToBusinessDefault(t *testing.T) {
	// Customer asked for Japanese (no delivery template); business default is
	// Spanish → the guest should get the Spanish template, not English.
	business := &database.Business{DefaultLanguage: "es"}
	delivery := &database.DeliveryOrder{CustomerLocale: "ja"}
	if locale := guestNotificationLocale(delivery, business); locale != LocaleES {
		t.Errorf("unknown customer locale should fall back to business default (es), got %q", locale)
	}

	// No business default either → English is the final fallback.
	business = &database.Business{}
	if locale := guestNotificationLocale(delivery, business); locale != LocaleEN {
		t.Errorf("unknown customer locale with no business default should fall back to English, got %q", locale)
	}

	// A customer locale we DO have a template for must still win over the
	// business default.
	business = &database.Business{DefaultLanguage: "es"}
	delivery = &database.DeliveryOrder{CustomerLocale: "en"}
	if locale := guestNotificationLocale(delivery, business); locale != LocaleEN {
		t.Errorf("known customer locale should win over business default, got %q", locale)
	}
}

func TestBusinessNotificationLocale_FallbackChain(t *testing.T) {
	if locale := businessNotificationLocale(nil); locale != LocaleEN {
		t.Errorf("nil business should fall back to English, got %q", locale)
	}
	if locale := businessNotificationLocale(&database.Business{}); locale != LocaleEN {
		t.Errorf("empty default_language should fall back to English, got %q", locale)
	}
	if locale := businessNotificationLocale(&database.Business{DefaultLanguage: "es-MX"}); locale != LocaleES {
		t.Errorf("Spanish default_language should resolve to es, got %q", locale)
	}
}

func TestLocalizedGuestMessage_AllLifecycleStates(t *testing.T) {
	for _, locale := range []NotificationLocale{LocaleEN, LocaleES} {
		for _, status := range []database.DeliveryStatus{
			database.DeliveryStatusAssigned,
			database.DeliveryStatusPickedUp,
			database.DeliveryStatusNearby,
			database.DeliveryStatusDelivered,
			database.DeliveryStatusCancelled,
		} {
			title, body, visible := localizedGuestMessage(status, "", locale)
			if !visible {
				t.Errorf("[%s] status %s should be guest-visible", locale, status)
			}
			if title == "" || body == "" {
				t.Errorf("[%s] status %s: empty title/body", locale, status)
			}
		}
	}
}

func TestLocalizedGuestMessage_ReasonPropagatesPerLocale(t *testing.T) {
	_, bodyEN, _ := localizedGuestMessage(database.DeliveryStatusCancelled, "out of stock", LocaleEN)
	if !strings.Contains(bodyEN, "Reason: out of stock") {
		t.Errorf("EN cancellation body should include reason, got: %q", bodyEN)
	}
	_, bodyES, _ := localizedGuestMessage(database.DeliveryStatusCancelled, "sin stock", LocaleES)
	if !strings.Contains(bodyES, "Motivo: sin stock") {
		t.Errorf("ES cancellation body should include reason, got: %q", bodyES)
	}
}

func TestLocalizedGuestMessage_DistinguishesLocales(t *testing.T) {
	_, bodyEN, _ := localizedGuestMessage(database.DeliveryStatusPickedUp, "", LocaleEN)
	_, bodyES, _ := localizedGuestMessage(database.DeliveryStatusPickedUp, "", LocaleES)
	if bodyEN == bodyES {
		t.Errorf("EN and ES bodies should differ for picked_up, both got %q", bodyEN)
	}
	if !strings.Contains(strings.ToLower(bodyES), "camino") {
		t.Errorf("ES picked_up body should reference 'camino', got %q", bodyES)
	}
}

func TestLocalizedOpsMessages_FormatArguments(t *testing.T) {
	titleEN, bodyEN := localizedOpsDriverAssignedMessage("DEL-1", "Alice", LocaleEN)
	if titleEN == "" || !strings.Contains(bodyEN, "DEL-1") || !strings.Contains(bodyEN, "Alice") {
		t.Errorf("EN ops driver-assigned message missing fields: title=%q body=%q", titleEN, bodyEN)
	}
	titleES, bodyES := localizedOpsCancellationMessage("DEL-2", "Bob", LocaleES)
	if titleES == "" || !strings.Contains(bodyES, "DEL-2") || !strings.Contains(bodyES, "Bob") {
		t.Errorf("ES ops cancellation message missing fields: title=%q body=%q", titleES, bodyES)
	}
}

func TestLocalizedNewDeliveryMessage_FormatArguments(t *testing.T) {
	titleEN, bodyEN := localizedNewDeliveryMessage("DEL-9", "Carol", "5 Main St", LocaleEN)
	if titleEN == "" {
		t.Error("EN new-delivery title is empty")
	}
	for _, want := range []string{"DEL-9", "Carol", "5 Main St"} {
		if !strings.Contains(bodyEN, want) {
			t.Errorf("EN new-delivery body missing %q: %q", want, bodyEN)
		}
	}
	_, bodyES := localizedNewDeliveryMessage("DEL-9", "Carol", "5 Main St", LocaleES)
	if !strings.Contains(bodyES, "Nuevo pedido") {
		t.Errorf("ES new-delivery body should start with Spanish prefix, got %q", bodyES)
	}
}
