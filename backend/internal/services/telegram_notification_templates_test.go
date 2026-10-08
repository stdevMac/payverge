package services

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderTelegramNotification_OrderCreated(t *testing.T) {
	message, err := RenderTelegramNotification(PluginNotificationEvent{
		EventType: PluginEventOrderCreated,
		Payload: map[string]interface{}{
			"order_number": "O-42",
			"table_name":   "Patio 3",
			"item_count":   4,
			"total_cents":  4200,
			"currency":     "USD",
			"notes":        "No onions",
		},
	}, "America/New_York", "USD", "en")

	require.NoError(t, err)
	assert.Contains(t, message, "New order #O-42")
	assert.Contains(t, message, "Table: Patio 3")
	assert.Contains(t, message, "Items: 4")
	assert.Contains(t, message, "Total: USD 42.00")
	assert.Contains(t, message, "Notes: No onions")
}

func TestRenderTelegramNotification_PaymentReceived(t *testing.T) {
	message, err := RenderTelegramNotification(PluginNotificationEvent{
		EventType: PluginEventPaymentReceived,
		Payload: map[string]interface{}{
			"bill_number":    "B-9",
			"amount_cents":   2500,
			"tip_cents":      350,
			"currency":       "USD",
			"payment_method": "card",
		},
	}, "UTC", "USD", "en")

	require.NoError(t, err)
	assert.Contains(t, message, "Payment received")
	assert.Contains(t, message, "Bill: B-9")
	assert.Contains(t, message, "Amount: USD 25.00")
	assert.Contains(t, message, "Tip: USD 3.50")
	assert.Contains(t, message, "Method: card")
}

func TestRenderTelegramNotification_EscapesHTML(t *testing.T) {
	message, err := RenderTelegramNotification(PluginNotificationEvent{
		EventType: PluginEventReservationCreated,
		Payload: map[string]interface{}{
			"customer_name":    "<script>alert(1)</script>",
			"party_size":       2,
			"reservation_time": time.Date(2026, 5, 11, 19, 30, 0, 0, time.UTC).Format(time.RFC3339),
			"status":           "pending",
		},
	}, "UTC", "USD", "en")

	require.NoError(t, err)
	assert.Contains(t, message, "&lt;script&gt;alert(1)&lt;/script&gt;")
	assert.NotContains(t, message, "<script>")
}

func TestRenderTelegramNotification_UsesBusinessTimezone(t *testing.T) {
	message, err := RenderTelegramNotification(PluginNotificationEvent{
		EventType: PluginEventReservationCreated,
		Payload: map[string]interface{}{
			"customer_name":    "Ada",
			"party_size":       2,
			"reservation_time": time.Date(2026, 5, 11, 19, 30, 0, 0, time.UTC).Format(time.RFC3339),
			"status":           "pending",
		},
	}, "America/New_York", "USD", "en")

	require.NoError(t, err)
	assert.Contains(t, message, "2026-05-11 15:30 EDT")
}

func TestRenderTelegramNotification_RejectsUnknownEventType(t *testing.T) {
	_, err := RenderTelegramNotification(PluginNotificationEvent{
		EventType: "unknown",
		Payload:   map[string]interface{}{},
	}, "UTC", "USD", "en")

	require.Error(t, err)
}

func TestRenderTelegramNotification_ReservationIncludesDashboardLink(t *testing.T) {
	payload := map[string]interface{}{
		"customer_name":    "Ada",
		"party_size":       2,
		"reservation_time": time.Date(2026, 5, 11, 19, 30, 0, 0, time.UTC).Format(time.RFC3339),
		"status":           "pending",
		"dashboard_url":    "https://payverge.io/business/7/dashboard?tab=reservations",
	}

	created, err := RenderTelegramNotification(PluginNotificationEvent{
		EventType: PluginEventReservationCreated,
		Payload:   payload,
	}, "UTC", "USD", "en")
	require.NoError(t, err)
	assert.Contains(t, created, "Manage: https://payverge.io/business/7/dashboard?tab=reservations")

	changed, err := RenderTelegramNotification(PluginNotificationEvent{
		EventType: PluginEventReservationStatusChanged,
		Payload:   payload,
	}, "UTC", "USD", "en")
	require.NoError(t, err)
	assert.Contains(t, changed, "Manage: https://payverge.io/business/7/dashboard?tab=reservations")
}

func TestRenderTelegramNotification_ReservationOmitsLinkWithoutDashboardURL(t *testing.T) {
	message, err := RenderTelegramNotification(PluginNotificationEvent{
		EventType: PluginEventReservationCreated,
		Payload: map[string]interface{}{
			"customer_name":    "Ada",
			"party_size":       2,
			"reservation_time": time.Date(2026, 5, 11, 19, 30, 0, 0, time.UTC).Format(time.RFC3339),
			"status":           "pending",
		},
	}, "UTC", "USD", "en")

	require.NoError(t, err)
	assert.NotContains(t, message, "Manage:")
}

// reservationTimeRFC3339 is the canonical fixture timestamp used across the
// byte-identity / locale tests below; America/New_York is avoided so the
// numeric format is deterministic in UTC.
var reservationTimeRFC3339 = time.Date(2026, 5, 11, 19, 30, 0, 0, time.UTC).Format(time.RFC3339)

// telegramByteIdentityCase pins the EXACT rendered output per event type so any
// drift in the English message shape is caught — operators may rely on the
// current wording, so English must stay byte-for-byte identical.
type telegramByteIdentityCase struct {
	name   string
	event  PluginNotificationEvent
	expect string
}

func telegramEnglishGoldenCases() []telegramByteIdentityCase {
	return []telegramByteIdentityCase{
		{
			name: "order_created",
			event: PluginNotificationEvent{
				EventType: PluginEventOrderCreated,
				Payload: map[string]interface{}{
					"order_number": "O-42",
					"table_name":   "Patio 3",
					"item_count":   4,
					"total_cents":  4200,
					"currency":     "USD",
					"notes":        "No onions",
				},
			},
			expect: "New order #O-42\nTable: Patio 3\nItems: 4\nTotal: USD 42.00\nNotes: No onions",
		},
		{
			name: "payment_received",
			event: PluginNotificationEvent{
				EventType: PluginEventPaymentReceived,
				Payload: map[string]interface{}{
					"bill_number":    "B-9",
					"amount_cents":   2500,
					"tip_cents":      350,
					"currency":       "USD",
					"payment_method": "card",
				},
			},
			expect: "Payment received\nBill: B-9\nAmount: USD 25.00\nTip: USD 3.50\nMethod: card",
		},
		{
			name: "reservation_created",
			event: PluginNotificationEvent{
				EventType: PluginEventReservationCreated,
				Payload: map[string]interface{}{
					"customer_name":    "Ada",
					"party_size":       2,
					"reservation_time": reservationTimeRFC3339,
					"status":           "pending",
				},
			},
			expect: "New reservation\nGuest: Ada\nParty: 2\nTime: 2026-05-11 19:30 UTC\nStatus: pending",
		},
		{
			name: "reservation_status_changed",
			event: PluginNotificationEvent{
				EventType: PluginEventReservationStatusChanged,
				Payload: map[string]interface{}{
					"customer_name":    "Ada",
					"reservation_time": reservationTimeRFC3339,
					"status":           "confirmed",
				},
			},
			expect: "Reservation confirmed\nGuest: Ada\nTime: 2026-05-11 19:30 UTC",
		},
		{
			name: "low_stock",
			event: PluginNotificationEvent{
				EventType: PluginEventInventoryLowStock,
				Payload: map[string]interface{}{
					"item_name":          "Tomatoes",
					"remaining_quantity": "3",
					"threshold_quantity": "5",
					"unit":               "kg",
				},
			},
			expect: "Low inventory\nItem: Tomatoes\nRemaining: 3 kg\nThreshold: 5 kg",
		},
		{
			name: "daily_summary",
			event: PluginNotificationEvent{
				EventType: PluginEventDailySummary,
				Payload: map[string]interface{}{
					"revenue_cents":     12345,
					"order_count":       8,
					"payment_count":     7,
					"reservation_count": 3,
					"currency":          "USD",
				},
			},
			expect: "Daily summary\nRevenue: USD 123.45\nOrders: 8\nPayments: 7\nReservations: 3",
		},
	}
}

func TestRenderTelegramNotification_EnglishOutputIsByteIdentical(t *testing.T) {
	for _, tc := range telegramEnglishGoldenCases() {
		t.Run(tc.name, func(t *testing.T) {
			message, err := RenderTelegramNotification(tc.event, "UTC", "USD", "en")
			require.NoError(t, err)
			assert.Equal(t, tc.expect, message)
		})
	}
}

// runTelegramLocaleGolden renders every English golden fixture under the given
// language and asserts the full localized message. Reusing the English events
// keeps the payloads identical so only the language is varied.
func runTelegramLocaleGolden(t *testing.T, language string, expected map[string]string) {
	t.Helper()
	for _, tc := range telegramEnglishGoldenCases() {
		want, ok := expected[tc.name]
		require.Truef(t, ok, "missing expected output for %q", tc.name)
		t.Run(tc.name, func(t *testing.T) {
			message, err := RenderTelegramNotification(tc.event, "UTC", "USD", language)
			require.NoError(t, err)
			assert.Equal(t, want, message)
		})
	}
}

func TestRenderTelegramNotification_SpanishOutput(t *testing.T) {
	runTelegramLocaleGolden(t, "es", map[string]string{
		"order_created":              "Nuevo pedido #O-42\nMesa: Patio 3\nArtículos: 4\nTotal: USD 42.00\nNotas: No onions",
		"payment_received":           "Pago recibido\nCuenta: B-9\nMonto: USD 25.00\nPropina: USD 3.50\nMétodo: card",
		"reservation_created":        "Nueva reservación\nCliente: Ada\nPersonas: 2\nHora: 2026-05-11 19:30 UTC\nEstado: pendiente",
		"reservation_status_changed": "Reserva confirmada\nCliente: Ada\nHora: 2026-05-11 19:30 UTC",
		"low_stock":                  "Inventario bajo\nArtículo: Tomatoes\nRestante: 3 kg\nUmbral: 5 kg",
		"daily_summary":              "Resumen diario\nIngresos: USD 123.45\nPedidos: 8\nPagos: 7\nReservaciones: 3",
	})
}

func TestRenderTelegramNotification_RioplatenseOutput(t *testing.T) {
	// es-AR mirrors es but prefers "reserva" over "reservación" and "Reservas"
	// over "Reservaciones" in the daily summary count line.
	runTelegramLocaleGolden(t, "es-AR", map[string]string{
		"order_created":              "Nuevo pedido #O-42\nMesa: Patio 3\nArtículos: 4\nTotal: USD 42.00\nNotas: No onions",
		"payment_received":           "Pago recibido\nCuenta: B-9\nMonto: USD 25.00\nPropina: USD 3.50\nMétodo: card",
		"reservation_created":        "Nueva reserva\nCliente: Ada\nPersonas: 2\nHora: 2026-05-11 19:30 UTC\nEstado: pendiente",
		"reservation_status_changed": "Reserva confirmada\nCliente: Ada\nHora: 2026-05-11 19:30 UTC",
		"low_stock":                  "Inventario bajo\nArtículo: Tomatoes\nRestante: 3 kg\nUmbral: 5 kg",
		"daily_summary":              "Resumen diario\nIngresos: USD 123.45\nPedidos: 8\nPagos: 7\nReservas: 3",
	})
}

func renderReservationStatusChangedFirstLine(t *testing.T, status, language string) string {
	t.Helper()
	message, err := RenderTelegramNotification(PluginNotificationEvent{
		EventType: PluginEventReservationStatusChanged,
		Payload: map[string]interface{}{
			"customer_name":    "Ada",
			"reservation_time": reservationTimeRFC3339,
			"status":           status,
		},
	}, "UTC", "USD", language)
	require.NoError(t, err)
	return strings.SplitN(message, "\n", 2)[0]
}

func renderReservationCreatedStatusLine(t *testing.T, status, language string) string {
	t.Helper()
	message, err := RenderTelegramNotification(PluginNotificationEvent{
		EventType: PluginEventReservationCreated,
		Payload: map[string]interface{}{
			"customer_name":    "Ada",
			"party_size":       2,
			"reservation_time": reservationTimeRFC3339,
			"status":           status,
		},
	}, "UTC", "USD", language)
	require.NoError(t, err)
	lines := strings.Split(message, "\n")
	return lines[len(lines)-1] // status is the last line when no dashboard link
}

func TestRenderTelegramNotification_LocalizesReservationStatusWords(t *testing.T) {
	cases := []struct {
		language   string
		status     string
		title      string // first line of reservation_status_changed
		statusLine string // last line of reservation_created
	}{
		{"es", "pending", "Reserva pendiente", "Estado: pendiente"},
		{"es", "confirmed", "Reserva confirmada", "Estado: confirmada"},
		{"es", "cancelled", "Reserva cancelada", "Estado: cancelada"},
		{"es", "seated", "Reserva sentada", "Estado: sentada"},
		{"es", "completed", "Reserva completada", "Estado: completada"},
		{"es", "no_show", "Reserva no se presentó", "Estado: no se presentó"},
		{"es", "waitlist", "Reserva en lista de espera", "Estado: en lista de espera"},
		{"es-AR", "confirmed", "Reserva confirmada", "Estado: confirmada"},
		{"es-AR", "cancelled", "Reserva cancelada", "Estado: cancelada"},
		{"es-AR", "no_show", "Reserva no se presentó", "Estado: no se presentó"},
		// English status words stay raw (byte-identical to pre-localization).
		{"en", "pending", "Reservation pending", "Status: pending"},
		{"en", "no_show", "Reservation no_show", "Status: no_show"},
		{"en", "waitlist", "Reservation waitlist", "Status: waitlist"},
	}
	for _, tc := range cases {
		t.Run(tc.language+"/"+tc.status, func(t *testing.T) {
			assert.Equal(t, tc.title, renderReservationStatusChangedFirstLine(t, tc.status, tc.language))
			assert.Equal(t, tc.statusLine, renderReservationCreatedStatusLine(t, tc.status, tc.language))
		})
	}
}

func TestRenderTelegramNotification_UnknownStatusFallsBackToRaw(t *testing.T) {
	// An unmapped status keeps the raw machine string (escaped at the call site)
	// in every locale, including Spanish.
	for _, language := range []string{"en", "es", "es-AR"} {
		assert.Equal(t,
			map[string]string{"en": "Reservation rescheduled", "es": "Reserva rescheduled", "es-AR": "Reserva rescheduled"}[language],
			renderReservationStatusChangedFirstLine(t, "rescheduled", language),
		)
		assert.Equal(t, "Status: rescheduled",
			renderReservationCreatedStatusLine(t, "rescheduled", "en"))
	}
}

func TestRenderTelegramNotification_UnknownOrEmptyLanguageFallsBackToEnglish(t *testing.T) {
	// Unknown tags, blank/whitespace, and registry locales without a Telegram
	// table (fr/de) all collapse to the English golden output.
	for _, language := range []string{"", "   ", "xx-YY", "klingon", "fr", "de", "zh"} {
		for _, tc := range telegramEnglishGoldenCases() {
			message, err := RenderTelegramNotification(tc.event, "UTC", "USD", language)
			require.NoError(t, err)
			assert.Equalf(t, tc.expect, message, "language=%q event=%q", language, tc.name)
		}
	}
}

// TestTelegramTemplatesAreComplete is the parity guard that lets this design
// scale to more languages safely: every locale table must fill every label
// (a forgotten field is the Go zero value "", not a compile error, so it would
// otherwise ship a silent blank line), and every non-English locale must map
// all canonical reservation statuses. English intentionally leaves statusWords
// nil and relies on the raw-status fallback, so it is exempt from the status check.
func TestTelegramTemplatesAreComplete(t *testing.T) {
	canonicalStatuses := []string{"pending", "confirmed", "cancelled", "seated", "completed", "no_show", "waitlist"}

	require.NotEmpty(t, telegramTemplatesByLocale)
	for locale, tmpl := range telegramTemplatesByLocale {
		v := reflect.ValueOf(tmpl)
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			if field.Type.Kind() == reflect.String {
				assert.NotEmptyf(t, v.Field(i).String(), "locale %q has empty field %q", locale, field.Name)
			}
		}

		if locale == telegramLocaleEN {
			continue
		}
		for _, status := range canonicalStatuses {
			_, ok := tmpl.statusWords[status]
			assert.Truef(t, ok, "locale %q is missing a status word for %q", locale, status)
		}
	}
}

func TestRenderTelegramNotification_LocalizesManageLink(t *testing.T) {
	url := "https://payverge.io/business/7/dashboard?tab=reservations"
	payload := map[string]interface{}{
		"customer_name":    "Ada",
		"party_size":       2,
		"reservation_time": reservationTimeRFC3339,
		"status":           "pending",
		"dashboard_url":    url,
	}
	render := func(language string) string {
		message, err := RenderTelegramNotification(PluginNotificationEvent{
			EventType: PluginEventReservationCreated,
			Payload:   payload,
		}, "UTC", "USD", language)
		require.NoError(t, err)
		return message
	}

	en := render("en")
	assert.Contains(t, en, "Manage: "+url)
	assert.NotContains(t, en, "Gestionar:")

	es := render("es")
	assert.Contains(t, es, "Gestionar: "+url)
	assert.NotContains(t, es, "Manage:")

	esar := render("es-AR")
	assert.Contains(t, esar, "Gestionar: "+url)
	assert.NotContains(t, esar, "Manage:")
}

// N-3: formatTelegramMoney must respect per-currency decimal digits for display,
// mirroring the outbound money.MajorUnitString conversion. Amounts arrive as
// stored "cents" (major units ×100 for ALL currencies).
func TestFormatTelegramMoney_CurrencyDecimals(t *testing.T) {
	cases := []struct {
		cents    int64
		currency string
		want     string
	}{
		{4200, "USD", "USD 42.00"},  // 2-decimal
		{100000, "JPY", "JPY 1000"}, // zero-decimal: ¥1000 stored as 100000
		{2500, "KRW", "KRW 25"},     // zero-decimal
		{150, "BHD", "BHD 1.500"},   // three-decimal
		{0, "", "USD 0.00"},         // empty currency defaults to USD
	}
	for _, tc := range cases {
		if got := formatTelegramMoney(tc.cents, tc.currency); got != tc.want {
			t.Errorf("formatTelegramMoney(%d, %q) = %q, want %q", tc.cents, tc.currency, got, tc.want)
		}
	}
}

// N-3 end-to-end: a JPY order total renders as whole yen, not divided by 100.
func TestRenderTelegramNotification_ZeroDecimalMoney(t *testing.T) {
	message, err := RenderTelegramNotification(PluginNotificationEvent{
		EventType: PluginEventOrderCreated,
		Payload: map[string]interface{}{
			"order_number": "O-1",
			"item_count":   1,
			"total_cents":  100000, // ¥1000
			"currency":     "JPY",
		},
	}, "UTC", "JPY", "en")
	require.NoError(t, err)
	assert.Contains(t, message, "Total: JPY 1000")
	assert.NotContains(t, message, "10.00")
}

// N-2: the three workforce events now render (previously returned an error,
// producing dead outbox rows).
func TestRenderTelegramNotification_SchedulePublished(t *testing.T) {
	message, err := RenderTelegramNotification(PluginNotificationEvent{
		EventType: PluginEventSchedulePublished,
		Payload: map[string]interface{}{
			"week_start":  time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
			"shift_count": 12,
			"staff_count": 4,
		},
	}, "UTC", "USD", "en")
	require.NoError(t, err)
	assert.Contains(t, message, "New schedule published")
	assert.Contains(t, message, "Week of: 2026-05-11")
	assert.Contains(t, message, "Shifts: 12")
	assert.Contains(t, message, "Staff: 4")

	es, err := RenderTelegramNotification(PluginNotificationEvent{
		EventType: PluginEventSchedulePublished,
		Payload:   map[string]interface{}{"week_start": "2026-05-11", "shift_count": 12, "staff_count": 4},
	}, "UTC", "USD", "es")
	require.NoError(t, err)
	assert.Contains(t, es, "Nuevo horario publicado")
	assert.Contains(t, es, "Turnos: 12")
}

func TestRenderTelegramNotification_ShiftReminder(t *testing.T) {
	message, err := RenderTelegramNotification(PluginNotificationEvent{
		EventType: PluginEventShiftReminder,
		Payload: map[string]interface{}{
			"shift_id":  7,
			"staff_id":  3,
			"starts_at": time.Date(2026, 5, 11, 19, 30, 0, 0, time.UTC).Format(time.RFC3339),
		},
	}, "America/New_York", "USD", "en")
	require.NoError(t, err)
	assert.Contains(t, message, "Upcoming shift")
	assert.Contains(t, message, "Starts: 2026-05-11 15:30 EDT")
}

func TestRenderTelegramNotification_CoverageDecided(t *testing.T) {
	en, err := RenderTelegramNotification(PluginNotificationEvent{
		EventType: PluginEventCoverageDecided,
		Payload:   map[string]interface{}{"recipient_staff_id": 3, "status": "approved"},
	}, "UTC", "USD", "en")
	require.NoError(t, err)
	assert.Contains(t, en, "Coverage request approved")

	es, err := RenderTelegramNotification(PluginNotificationEvent{
		EventType: PluginEventCoverageDecided,
		Payload:   map[string]interface{}{"recipient_staff_id": 3, "status": "denied"},
	}, "UTC", "USD", "es")
	require.NoError(t, err)
	assert.Contains(t, es, "Solicitud de cobertura denegada")
}

// N-2: workforce events must be opt-in (default off) so enabling Telegram does
// not suddenly spam every business's operator chat with staff-scheduling events.
func TestTelegramWorkforceEventsDefaultOff(t *testing.T) {
	config := map[string]interface{}{} // no notifications block → defaults apply
	for _, event := range []string{PluginEventSchedulePublished, PluginEventShiftReminder, PluginEventCoverageDecided} {
		if TelegramEventNotificationEnabled(config, event) {
			t.Errorf("event %q should default OFF", event)
		}
	}
	// And can be turned on via the notifications preference block.
	on := map[string]interface{}{"notifications": map[string]interface{}{
		"schedule_published": true, "shift_reminder": true, "coverage_decided": true,
	}}
	for _, event := range []string{PluginEventSchedulePublished, PluginEventShiftReminder, PluginEventCoverageDecided} {
		if !TelegramEventNotificationEnabled(on, event) {
			t.Errorf("event %q should be enabled when its preference key is true", event)
		}
	}
}
