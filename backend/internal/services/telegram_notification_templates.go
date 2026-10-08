package services

import (
	"errors"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/locales"
	"github.com/stdevmac/payverge/backend/internal/money"
)

// RenderTelegramNotification renders an operator-facing Telegram notification in
// the business's language. The language is the raw business.DefaultLanguage tag
// (e.g. "en", "es", "es-AR"); it is resolved through the canonical locale
// registry to one of the three string tables (en / es / es_ar). Anything
// unknown or empty falls back to English, so a weird language value never errors
// and never silences the operator.
func RenderTelegramNotification(event PluginNotificationEvent, businessTimezone string, currency string, language string) (string, error) {
	payload := event.Payload
	if payload == nil {
		payload = map[string]interface{}{}
	}
	if currency = strings.TrimSpace(currency); currency == "" {
		currency = telegramStringValue(payload, "currency", "USD")
	}
	t := telegramTemplatesFor(language)

	switch event.EventType {
	case PluginEventOrderCreated:
		return renderTelegramOrderCreated(payload, currency, t), nil
	case PluginEventPaymentReceived:
		return renderTelegramPaymentReceived(payload, currency, t), nil
	case PluginEventReservationCreated:
		return renderTelegramReservationCreated(payload, businessTimezone, t), nil
	case PluginEventReservationStatusChanged:
		return renderTelegramReservationStatusChanged(payload, businessTimezone, t), nil
	case PluginEventInventoryLowStock:
		return renderTelegramLowStock(payload, t), nil
	case PluginEventDailySummary:
		return renderTelegramDailySummary(payload, currency, t), nil
	case PluginEventSchedulePublished:
		return renderTelegramSchedulePublished(payload, t), nil
	case PluginEventShiftReminder:
		return renderTelegramShiftReminder(payload, businessTimezone, t), nil
	case PluginEventCoverageDecided:
		return renderTelegramCoverageDecided(payload, t), nil
	default:
		return "", errors.New("unsupported telegram notification event type")
	}
}

func renderTelegramOrderCreated(payload map[string]interface{}, currency string, t telegramTemplates) string {
	notes := telegramStringValue(payload, "notes", "")
	if strings.TrimSpace(notes) == "" {
		notes = t.orderNotesFallback
	}
	return strings.Join([]string{
		t.orderTitlePrefix + escapeTelegramHTML(telegramStringValue(payload, "order_number", "unknown")),
		t.orderTableLabel + escapeTelegramHTML(telegramStringValue(payload, "table_name", t.orderTableFallback)),
		fmt.Sprintf("%s%d", t.orderItemsLabel, telegramIntValue(payload, "item_count", 0)),
		t.orderTotalLabel + formatTelegramMoney(telegramInt64Value(payload, "total_cents", 0), telegramStringValue(payload, "currency", currency)),
		t.orderNotesLabel + escapeTelegramHTML(notes),
	}, "\n")
}

func renderTelegramPaymentReceived(payload map[string]interface{}, currency string, t telegramTemplates) string {
	return strings.Join([]string{
		t.paymentTitle,
		t.paymentBillLabel + escapeTelegramHTML(telegramStringValue(payload, "bill_number", "unknown")),
		t.paymentAmountLabel + formatTelegramMoney(telegramInt64Value(payload, "amount_cents", 0), telegramStringValue(payload, "currency", currency)),
		t.paymentTipLabel + formatTelegramMoney(telegramInt64Value(payload, "tip_cents", 0), telegramStringValue(payload, "currency", currency)),
		t.paymentMethodLabel + escapeTelegramHTML(telegramStringValue(payload, "payment_method", "unknown")),
	}, "\n")
}

func renderTelegramReservationCreated(payload map[string]interface{}, businessTimezone string, t telegramTemplates) string {
	lines := []string{
		t.reservationCreatedTitle,
		t.reservationGuestLabel + escapeTelegramHTML(telegramStringValue(payload, "customer_name", t.reservationGuestFallback)),
		fmt.Sprintf("%s%d", t.reservationPartyLabel, telegramIntValue(payload, "party_size", 0)),
		t.reservationTimeLabel + escapeTelegramHTML(formatTelegramEventTime(telegramStringValue(payload, "reservation_time", ""), businessTimezone, t.timeUnknown)),
		t.reservationStatusLabel + escapeTelegramHTML(t.statusWord(telegramStringValue(payload, "status", "pending"))),
	}
	return strings.Join(appendTelegramDashboardLink(lines, payload, t), "\n")
}

func renderTelegramReservationStatusChanged(payload map[string]interface{}, businessTimezone string, t telegramTemplates) string {
	lines := []string{
		t.reservationChangedTitlePrefix + escapeTelegramHTML(t.statusWord(telegramStringValue(payload, "status", "updated"))),
		t.reservationGuestLabel + escapeTelegramHTML(telegramStringValue(payload, "customer_name", t.reservationGuestFallback)),
		t.reservationTimeLabel + escapeTelegramHTML(formatTelegramEventTime(telegramStringValue(payload, "reservation_time", ""), businessTimezone, t.timeUnknown)),
	}
	return strings.Join(appendTelegramDashboardLink(lines, payload, t), "\n")
}

// appendTelegramDashboardLink adds a deep link into the operator dashboard
// when the event payload carries one, so a tap on the notification lands on
// the screen where the reservation can be acted on (approve/decline/seat).
func appendTelegramDashboardLink(lines []string, payload map[string]interface{}, t telegramTemplates) []string {
	url := strings.TrimSpace(telegramStringValue(payload, "dashboard_url", ""))
	if url == "" {
		return lines
	}
	return append(lines, t.dashboardManageLabel+escapeTelegramHTML(url))
}

func renderTelegramLowStock(payload map[string]interface{}, t telegramTemplates) string {
	unit := telegramStringValue(payload, "unit", "")
	return strings.Join([]string{
		t.lowStockTitle,
		t.lowStockItemLabel + escapeTelegramHTML(telegramStringValue(payload, "item_name", t.lowStockItemFallback)),
		t.lowStockRemainingLabel + escapeTelegramHTML(fmt.Sprintf("%s %s", telegramStringValue(payload, "remaining_quantity", "0"), unit)),
		t.lowStockThresholdLabel + escapeTelegramHTML(fmt.Sprintf("%s %s", telegramStringValue(payload, "threshold_quantity", "0"), unit)),
	}, "\n")
}

func renderTelegramDailySummary(payload map[string]interface{}, currency string, t telegramTemplates) string {
	return strings.Join([]string{
		t.dailySummaryTitle,
		t.dailyRevenueLabel + formatTelegramMoney(telegramInt64Value(payload, "revenue_cents", 0), telegramStringValue(payload, "currency", currency)),
		fmt.Sprintf("%s%d", t.dailyOrdersLabel, telegramIntValue(payload, "order_count", 0)),
		fmt.Sprintf("%s%d", t.dailyPaymentsLabel, telegramIntValue(payload, "payment_count", 0)),
		fmt.Sprintf("%s%d", t.dailyReservationsLabel, telegramIntValue(payload, "reservation_count", 0)),
	}, "\n")
}

// renderTelegramSchedulePublished renders the operator's "schedule published"
// summary. One message per publish (not per staffer): the assigned-staff members
// are reminded on their own staff channel; the operator Telegram chat gets a
// single roll-up.
func renderTelegramSchedulePublished(payload map[string]interface{}, t telegramTemplates) string {
	return strings.Join([]string{
		t.schedulePublishedTitle,
		t.scheduleWeekLabel + escapeTelegramHTML(formatTelegramDate(telegramStringValue(payload, "week_start", ""), t.timeUnknown)),
		fmt.Sprintf("%s%d", t.scheduleShiftsLabel, telegramIntValue(payload, "shift_count", 0)),
		fmt.Sprintf("%s%d", t.scheduleStaffLabel, telegramIntValue(payload, "staff_count", 0)),
	}, "\n")
}

func renderTelegramShiftReminder(payload map[string]interface{}, businessTimezone string, t telegramTemplates) string {
	return strings.Join([]string{
		t.shiftReminderTitle,
		t.shiftReminderStartsLabel + escapeTelegramHTML(formatTelegramEventTime(telegramStringValue(payload, "starts_at", ""), businessTimezone, t.timeUnknown)),
	}, "\n")
}

func renderTelegramCoverageDecided(payload map[string]interface{}, t telegramTemplates) string {
	status := t.coverageStatusWord(telegramStringValue(payload, "status", "updated"))
	return t.coverageDecidedTitlePrefix + escapeTelegramHTML(status)
}

func escapeTelegramHTML(value string) string {
	return html.EscapeString(value)
}

// formatTelegramMoney renders a stored "cents" amount (major units ×100 for
// ALL currencies) in the currency's real number of decimals. Zero-decimal
// currencies (JPY, KRW, …) render with no fractional part ("JPY 1000"), and
// three-decimal currencies (BHD, KWD, …) with three ("BHD 1.500"), matching the
// provider-boundary conversion used everywhere else (money.MajorUnitString).
func formatTelegramMoney(cents int64, currency string) string {
	currency = strings.TrimSpace(currency)
	if currency == "" {
		currency = "USD"
	}
	return currency + " " + money.MajorUnitString(cents, currency)
}

// formatTelegramDate renders an RFC3339 (or date-only) timestamp string as a
// locale-neutral calendar date. Unparseable input yields the localized unknown
// word, mirroring formatTelegramEventTime.
func formatTelegramDate(raw string, unknownLabel string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return unknownLabel
	}
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed.Format("2006-01-02")
	}
	if parsed, err := time.Parse("2006-01-02", raw); err == nil {
		return parsed.Format("2006-01-02")
	}
	return unknownLabel
}

// formatTelegramEventTime renders the reservation timestamp in the business
// timezone using a locale-neutral numeric format. Only the parse-failure
// fallback word is localized (unknownLabel); month/day names stay numeric so we
// never need per-locale calendars.
func formatTelegramEventTime(raw string, timezone string, unknownLabel string) string {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	if err != nil {
		return unknownLabel
	}
	loc := time.UTC
	if strings.TrimSpace(timezone) != "" {
		if loaded, err := time.LoadLocation(timezone); err == nil {
			loc = loaded
		}
	}
	return parsed.In(loc).Format("2006-01-02 15:04 MST")
}

func telegramStringValue(payload map[string]interface{}, key string, fallback string) string {
	value, ok := payload[key]
	if !ok || value == nil {
		return fallback
	}
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return fallback
		}
		return typed
	case fmt.Stringer:
		return typed.String()
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func telegramIntValue(payload map[string]interface{}, key string, fallback int) int {
	value, ok := payload[key]
	if !ok || value == nil {
		return fallback
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func telegramInt64Value(payload map[string]interface{}, key string, fallback int64) int64 {
	value, ok := payload[key]
	if !ok || value == nil {
		return fallback
	}
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

// telegramLocale is the small set of languages we ship operator notification
// copy for. Everything else collapses to English.
type telegramLocale string

const (
	telegramLocaleEN   telegramLocale = "en"
	telegramLocaleES   telegramLocale = "es"
	telegramLocaleESAR telegramLocale = "es_ar"
)

// resolveTelegramLocale maps a raw business.DefaultLanguage tag onto one of the
// three notification tables via the canonical locale registry. We key on the
// registry's EmailFamily (the same family axis the email templates use) so the
// Telegram tier stays aligned with the rest of the localized backend: family
// "eng" → en, "es" → es, "es_ar" → es_ar, anything else (or unknown/empty) → en.
//
// business.DefaultLanguage is stored verbatim (no canonicalization), so the same
// stored value can arrive as "es-AR", "es_ar", "es-ar", or "ES". We resolve the
// EmailFamily with the SAME tolerance the email channel uses
// (emails.normalizeTemplateLanguage): lowercase + trim, treat "_" and "-" as
// equivalent, and case-fold against the canonical registry keys. A plain
// locales.Lookup is exact (case/form sensitive), so it would resolve these
// non-canonical forms to English Telegram while the email channel localizes them
// to Spanish — the two channels would silently diverge. Normalizing here keeps
// them aligned without changing locales.Lookup (other callers rely on its
// exactness).
func resolveTelegramLocale(language string) telegramLocale {
	switch telegramEmailFamilyFor(language) {
	case "es":
		return telegramLocaleES
	case "es_ar":
		return telegramLocaleESAR
	default:
		return telegramLocaleEN
	}
}

// telegramEmailFamilyFor resolves a stored language tag to the registry's
// EmailFamily with case/form tolerance, mirroring the email channel so the two
// channels can never diverge. Empty/unknown collapse to the default family.
func telegramEmailFamilyFor(language string) string {
	normalized := strings.ToLower(strings.TrimSpace(language))
	if normalized == "" {
		return locales.Default().EmailFamily
	}
	// An exact canonical key (already correct form/case) is the fast path.
	if locale, ok := locales.Lookup(normalized); ok {
		return locale.EmailFamily
	}
	// Fold "_" → "-" and compare case-insensitively against canonical codes so
	// "es_ar"/"es-ar"/"ES-AR" all resolve like the canonical "es-AR".
	folded := strings.ReplaceAll(normalized, "_", "-")
	for _, locale := range locales.AllLocales() {
		if strings.EqualFold(locale.Canonical, folded) {
			return locale.EmailFamily
		}
	}
	return locales.Default().EmailFamily
}

func telegramTemplatesFor(language string) telegramTemplates {
	if t, ok := telegramTemplatesByLocale[resolveTelegramLocale(language)]; ok {
		return t
	}
	return telegramTemplatesByLocale[telegramLocaleEN]
}

// telegramTemplates holds every operator-visible static string for one locale.
// Exhaustive struct fields (no map[string]string label lookups) so a missing
// translation is a compile-time hole, not a silent runtime miss.
type telegramTemplates struct {
	// order created
	orderTitlePrefix   string // includes the trailing "#"
	orderTableLabel    string
	orderTableFallback string
	orderItemsLabel    string
	orderTotalLabel    string
	orderNotesLabel    string
	orderNotesFallback string

	// payment received
	paymentTitle       string
	paymentBillLabel   string
	paymentAmountLabel string
	paymentTipLabel    string
	paymentMethodLabel string

	// reservation created
	reservationCreatedTitle  string
	reservationGuestLabel    string
	reservationGuestFallback string
	reservationPartyLabel    string
	reservationTimeLabel     string
	reservationStatusLabel   string

	// reservation status changed — the title is prefix + localized status word
	reservationChangedTitlePrefix string

	// low stock
	lowStockTitle          string
	lowStockItemLabel      string
	lowStockItemFallback   string
	lowStockRemainingLabel string
	lowStockThresholdLabel string

	// daily summary
	dailySummaryTitle      string
	dailyRevenueLabel      string
	dailyOrdersLabel       string
	dailyPaymentsLabel     string
	dailyReservationsLabel string

	// schedule published (operator roll-up, one per publish)
	schedulePublishedTitle string
	scheduleWeekLabel      string
	scheduleShiftsLabel    string
	scheduleStaffLabel     string

	// shift reminder
	shiftReminderTitle       string
	shiftReminderStartsLabel string

	// coverage decided — title is prefix + localized status word
	coverageDecidedTitlePrefix string
	// coverageStatusWords maps a coverage decision status (approved/denied/
	// cancelled/withdrawn) onto its localized word. English leaves this nil and
	// relies on the raw status (already the English word).
	coverageStatusWords map[string]string

	// shared
	dashboardManageLabel string
	timeUnknown          string

	// statusWords maps the raw machine reservation status onto its localized
	// word. Unmapped statuses fall back to the raw string (escaped at the call
	// site, as before). English leaves this nil and relies on the raw fallback,
	// which keeps the English output byte-identical to the pre-localization code.
	statusWords map[string]string
}

// statusWord returns the localized reservation status word, or the raw status
// when this locale has no mapping for it.
func (t telegramTemplates) statusWord(raw string) string {
	if t.statusWords != nil {
		if word, ok := t.statusWords[raw]; ok {
			return word
		}
	}
	return raw
}

// coverageStatusWord returns the localized coverage-decision status word, or the
// raw status when this locale has no mapping for it.
func (t telegramTemplates) coverageStatusWord(raw string) string {
	if t.coverageStatusWords != nil {
		if word, ok := t.coverageStatusWords[raw]; ok {
			return word
		}
	}
	return raw
}

var telegramTemplatesByLocale = map[telegramLocale]telegramTemplates{
	telegramLocaleEN: {
		orderTitlePrefix:   "New order #",
		orderTableLabel:    "Table: ",
		orderTableFallback: "Unassigned",
		orderItemsLabel:    "Items: ",
		orderTotalLabel:    "Total: ",
		orderNotesLabel:    "Notes: ",
		orderNotesFallback: "None",

		paymentTitle:       "Payment received",
		paymentBillLabel:   "Bill: ",
		paymentAmountLabel: "Amount: ",
		paymentTipLabel:    "Tip: ",
		paymentMethodLabel: "Method: ",

		reservationCreatedTitle:  "New reservation",
		reservationGuestLabel:    "Guest: ",
		reservationGuestFallback: "Guest",
		reservationPartyLabel:    "Party: ",
		reservationTimeLabel:     "Time: ",
		reservationStatusLabel:   "Status: ",

		reservationChangedTitlePrefix: "Reservation ",

		lowStockTitle:          "Low inventory",
		lowStockItemLabel:      "Item: ",
		lowStockItemFallback:   "Unknown item",
		lowStockRemainingLabel: "Remaining: ",
		lowStockThresholdLabel: "Threshold: ",

		dailySummaryTitle:      "Daily summary",
		dailyRevenueLabel:      "Revenue: ",
		dailyOrdersLabel:       "Orders: ",
		dailyPaymentsLabel:     "Payments: ",
		dailyReservationsLabel: "Reservations: ",

		schedulePublishedTitle: "New schedule published",
		scheduleWeekLabel:      "Week of: ",
		scheduleShiftsLabel:    "Shifts: ",
		scheduleStaffLabel:     "Staff: ",

		shiftReminderTitle:       "Upcoming shift",
		shiftReminderStartsLabel: "Starts: ",

		coverageDecidedTitlePrefix: "Coverage request ",

		dashboardManageLabel: "Manage: ",
		timeUnknown:          "unknown",

		// statusWords / coverageStatusWords intentionally nil for English: the
		// raw machine status is already the English word, and raw fallback
		// guarantees byte-identity.
	},
	telegramLocaleES:   spanishTelegramTemplates(false),
	telegramLocaleESAR: spanishTelegramTemplates(true),
}

// spanishReservationStatusWords maps the raw machine reservation status onto its
// Spanish word. The forms are feminine to agree with "reserva" in the
// status-changed title ("Reserva confirmada", "Reserva cancelada"). Word choices
// mirror the es/es_ar reservation email templates. Rioplatense uses the same
// participles (voseo does not affect them), so es and es_ar share this map.
var spanishReservationStatusWords = map[string]string{
	"pending":   "pendiente",
	"confirmed": "confirmada",
	"cancelled": "cancelada",
	"seated":    "sentada",
	"completed": "completada",
	"no_show":   "no se presentó",
	"waitlist":  "en lista de espera",
}

// spanishCoverageStatusWords maps a coverage-decision status onto its Spanish
// word. Feminine forms agree with "solicitud" in the coverage-decided title
// ("Solicitud de cobertura aprobada"). Shared by es and es_ar (voseo does not
// affect these participles).
var spanishCoverageStatusWords = map[string]string{
	"approved":  "aprobada",
	"denied":    "denegada",
	"cancelled": "cancelada",
	"withdrawn": "retirada",
}

// spanishTelegramTemplates builds the Spanish notification table. Rioplatense
// (es-AR) is identical to neutral Spanish except it prefers "reserva" over
// "reservación" and "Reservas" over "Reservaciones" — the same split the es vs
// es_ar email templates make. Every other label matches, so the two tables are
// derived from one builder to keep them in lockstep.
func spanishTelegramTemplates(rioplatense bool) telegramTemplates {
	reservationTitle := "Nueva reservación"
	reservationsCountLabel := "Reservaciones: "
	if rioplatense {
		reservationTitle = "Nueva reserva"
		reservationsCountLabel = "Reservas: "
	}
	return telegramTemplates{
		orderTitlePrefix:   "Nuevo pedido #",
		orderTableLabel:    "Mesa: ",
		orderTableFallback: "Sin asignar",
		orderItemsLabel:    "Artículos: ",
		orderTotalLabel:    "Total: ",
		orderNotesLabel:    "Notas: ",
		orderNotesFallback: "Ninguna",

		paymentTitle:       "Pago recibido",
		paymentBillLabel:   "Cuenta: ",
		paymentAmountLabel: "Monto: ",
		paymentTipLabel:    "Propina: ",
		paymentMethodLabel: "Método: ",

		reservationCreatedTitle:  reservationTitle,
		reservationGuestLabel:    "Cliente: ",
		reservationGuestFallback: "Invitado",
		reservationPartyLabel:    "Personas: ",
		reservationTimeLabel:     "Hora: ",
		reservationStatusLabel:   "Estado: ",

		reservationChangedTitlePrefix: "Reserva ",

		lowStockTitle:          "Inventario bajo",
		lowStockItemLabel:      "Artículo: ",
		lowStockItemFallback:   "Artículo desconocido",
		lowStockRemainingLabel: "Restante: ",
		lowStockThresholdLabel: "Umbral: ",

		dailySummaryTitle:      "Resumen diario",
		dailyRevenueLabel:      "Ingresos: ",
		dailyOrdersLabel:       "Pedidos: ",
		dailyPaymentsLabel:     "Pagos: ",
		dailyReservationsLabel: reservationsCountLabel,

		schedulePublishedTitle: "Nuevo horario publicado",
		scheduleWeekLabel:      "Semana del: ",
		scheduleShiftsLabel:    "Turnos: ",
		scheduleStaffLabel:     "Personal: ",

		shiftReminderTitle:       "Próximo turno",
		shiftReminderStartsLabel: "Comienza: ",

		coverageDecidedTitlePrefix: "Solicitud de cobertura ",

		dashboardManageLabel: "Gestionar: ",
		timeUnknown:          "desconocida",

		statusWords:         spanishReservationStatusWords,
		coverageStatusWords: spanishCoverageStatusWords,
	}
}
