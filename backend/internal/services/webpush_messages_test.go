package services

import "testing"

// TestLocalizePushNewOrder asserts the guest-order push title/body localize for
// en / es / es-AR and fall back to English for an unknown language.
func TestLocalizePushNewOrder(t *testing.T) {
	cases := []struct {
		language  string
		wantTitle string
		wantBody  string
	}{
		{"en", "New Order", "Order #A100 needs approval"},
		{"es", "Nuevo pedido", "El pedido N.º A100 necesita aprobación"},
		{"es-AR", "Nuevo pedido", "El pedido N.º A100 necesita aprobación"},
		{"es_ar", "Nuevo pedido", "El pedido N.º A100 necesita aprobación"},
		{"fr", "New Order", "Order #A100 needs approval"}, // unknown → English
		{"", "New Order", "Order #A100 needs approval"},   // empty → English
	}
	for _, tc := range cases {
		title, body := LocalizePush(tc.language, PushKeyNewOrder, PushArgs{OrderNumber: "A100"})
		if title != tc.wantTitle {
			t.Errorf("language %q: title = %q, want %q", tc.language, title, tc.wantTitle)
		}
		if body != tc.wantBody {
			t.Errorf("language %q: body = %q, want %q", tc.language, body, tc.wantBody)
		}
	}
}

// TestLocalizePushNewDeliveryOrder asserts the delivery-order push title/body.
func TestLocalizePushNewDeliveryOrder(t *testing.T) {
	cases := []struct {
		language  string
		wantTitle string
		wantBody  string
	}{
		{"en", "New Delivery Order", "Order #D55 needs approval"},
		{"es", "Nuevo pedido de envío", "El pedido N.º D55 necesita aprobación"},
		{"es-AR", "Nuevo pedido de envío", "El pedido N.º D55 necesita aprobación"},
		{"de", "New Delivery Order", "Order #D55 needs approval"}, // unknown → English
	}
	for _, tc := range cases {
		title, body := LocalizePush(tc.language, PushKeyNewDeliveryOrder, PushArgs{OrderNumber: "D55"})
		if title != tc.wantTitle {
			t.Errorf("language %q: title = %q, want %q", tc.language, title, tc.wantTitle)
		}
		if body != tc.wantBody {
			t.Errorf("language %q: body = %q, want %q", tc.language, body, tc.wantBody)
		}
	}
}

// TestLocalizePushNewReservation asserts the reservation push title/body. es-AR
// prefers "reserva" (Rioplatense) over neutral "reservación", matching the
// Telegram and email seams.
func TestLocalizePushNewReservation(t *testing.T) {
	cases := []struct {
		language  string
		wantTitle string
		wantBody  string
	}{
		{"en", "New Reservation", "Reservation for Jordan"},
		{"es", "Nueva reservación", "Reserva para Jordan"},
		{"es-AR", "Nueva reserva", "Reserva para Jordan"},
		{"es_ar", "Nueva reserva", "Reserva para Jordan"},
		{"it", "New Reservation", "Reservation for Jordan"}, // unknown → English
	}
	for _, tc := range cases {
		title, body := LocalizePush(tc.language, PushKeyNewReservation, PushArgs{CustomerName: "Jordan"})
		if title != tc.wantTitle {
			t.Errorf("language %q: title = %q, want %q", tc.language, title, tc.wantTitle)
		}
		if body != tc.wantBody {
			t.Errorf("language %q: body = %q, want %q", tc.language, body, tc.wantBody)
		}
	}
}

// TestLocalizePushUnknownKeyFallsBackEmpty guards against a silent panic if a
// new key is ever requested without a table entry: it must return empty strings,
// never crash.
func TestLocalizePushUnknownKey(t *testing.T) {
	title, body := LocalizePush("en", PushKey("does-not-exist"), PushArgs{})
	if title != "" || body != "" {
		t.Errorf("unknown key: got (%q, %q), want empty strings", title, body)
	}
}

// TestLocalizePushStaffKeys asserts the five staff notification push keys
// localize for en/es/es-AR, with the es-AR voseo delta on shift_assigned.
func TestLocalizePushStaffKeys(t *testing.T) {
	cases := []struct {
		lang     string
		key      PushKey
		args     PushArgs
		wantBody string // substring that must appear
	}{
		{"en", PushKeyShiftAssigned, PushArgs{ShiftDate: "2026-07-03"}, "2026-07-03"},
		{"es", PushKeyShiftAssigned, PushArgs{ShiftDate: "2026-07-03"}, "Tienes"},
		{"es-AR", PushKeyShiftAssigned, PushArgs{ShiftDate: "2026-07-03"}, "Tenés"}, // voseo delta
		{"en", PushKeySchedulePublished, PushArgs{WeekOf: "2026-06-29"}, "2026-06-29"},
		{"en", PushKeyCoverageOffer, PushArgs{}, "coverage"},
		{"en", PushKeyCoverageDecided, PushArgs{}, "coverage"},
		{"en", PushKeyAnnouncement, PushArgs{Title: "All-hands Friday"}, "All-hands Friday"},
	}
	for _, tc := range cases {
		title, body := LocalizePush(tc.lang, tc.key, tc.args)
		if title == "" {
			t.Errorf("%s/%s must have a title", tc.lang, tc.key)
		}
		if !contains(t, body, tc.wantBody) {
			t.Errorf("%s/%s body = %q, want containing %q", tc.lang, tc.key, body, tc.wantBody)
		}
		// Money-free: no staff push copy carries a dollar sign.
		if contains(t, title+body, "$") {
			t.Errorf("%s/%s contains '$'", tc.lang, tc.key)
		}
	}
}

func contains(t *testing.T, s, substr string) bool {
	t.Helper()
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
