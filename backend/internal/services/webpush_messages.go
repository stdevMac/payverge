package services

import "fmt"

// Web-push notification localization.
//
// Operator browser push (the Web Push API path in webpush_service.go) used to
// ship hard-coded English titles/bodies at every call site, so an es / es-AR
// operator got English push while Telegram and email were already localized.
// This table is the single home for the localized title/body strings, keyed by
// a small notification-key enum, mirroring the Telegram template-table shape in
// telegram_notification_templates.go.
//
// Locale resolution reuses resolveTelegramLocale (the same EmailFamily axis the
// Telegram and email channels use) so the three channels can never diverge:
// family "es" → es, "es_ar" → es_ar, anything else / unknown / empty → en.

// PushKey identifies one localizable push notification (operator or staff tier).
type PushKey string

const (
	PushKeyNewOrder         PushKey = "new_order"
	PushKeyNewDeliveryOrder PushKey = "new_delivery_order"
	PushKeyNewReservation   PushKey = "new_reservation"
	// Staff "Time & Team" Phase 1 (money-free).
	PushKeyShiftAssigned     PushKey = "shift_assigned"
	PushKeySchedulePublished PushKey = "schedule_published"
	PushKeyShiftReminder     PushKey = "shift_reminder"
	PushKeyCoverageOffer     PushKey = "coverage_offer"
	PushKeyCoverageDecided   PushKey = "coverage_decided"
	PushKeyAnnouncement      PushKey = "announcement"
	// Explicit claim steal (AI waiter conversation or delivery dispatch).
	PushKeyClaimStolen PushKey = "claim_stolen"
)

// PushArgs carries the dynamic values interpolated into a localized push
// title/body. Only the fields a given key needs are read; the rest stay zero.
// Staff-tier fields are locale-neutral strings (ISO dates, plain titles) — no
// money ever crosses this struct for staff notifications.
type PushArgs struct {
	OrderNumber  string // new_order / new_delivery_order body
	CustomerName string // new_reservation body
	ShiftDate    string // shift_assigned body — ISO date (YYYY-MM-DD)
	WeekOf       string // schedule_published body — ISO week-start date
	Title        string // announcement body — the announcement title
	StealerName  string // claim_stolen body — who took the claim
	Resource     string // claim_stolen body — what was taken (chat / delivery #)
}

// pushMessage is the localized title + a body template for one notification key
// in one locale. body is a fmt format string applied to the key's argument(s);
// keys with a static body leave the format verbs out and ignore the args.
type pushMessage struct {
	title string
	// body takes the resolved PushArgs and returns the final body string. Using
	// a closure (rather than a raw fmt string) keeps each key in control of
	// exactly which arg it interpolates, and lets a key vary its body wording per
	// locale without a brittle positional-verb contract.
	body func(PushArgs) string
}

// pushMessagesByLocale is the single source of localized push copy. Each locale
// table is exhaustive over the shipped keys; a missing key returns empty strings
// (see LocalizePush) rather than panicking.
var pushMessagesByLocale = map[telegramLocale]map[PushKey]pushMessage{
	telegramLocaleEN: {
		PushKeyNewOrder: {
			title: "New Order",
			body:  func(a PushArgs) string { return fmt.Sprintf("Order #%s needs approval", a.OrderNumber) },
		},
		PushKeyNewDeliveryOrder: {
			title: "New Delivery Order",
			body:  func(a PushArgs) string { return fmt.Sprintf("Order #%s needs approval", a.OrderNumber) },
		},
		PushKeyNewReservation: {
			title: "New Reservation",
			body:  func(a PushArgs) string { return fmt.Sprintf("Reservation for %s", a.CustomerName) },
		},
		PushKeyShiftAssigned: {
			title: "Shift scheduled",
			body:  func(a PushArgs) string { return fmt.Sprintf("You're on for %s", a.ShiftDate) },
		},
		PushKeySchedulePublished: {
			title: "Schedule posted",
			body:  func(a PushArgs) string { return fmt.Sprintf("Your week of %s is ready", a.WeekOf) },
		},
		PushKeyShiftReminder: {
			title: "Shift reminder",
			body:  func(a PushArgs) string { return fmt.Sprintf("Your shift on %s is coming up", a.ShiftDate) },
		},
		PushKeyCoverageOffer: {
			title: "Shift up for grabs",
			body:  func(a PushArgs) string { return "A shift needs coverage — tap to pick it up" },
		},
		PushKeyCoverageDecided: {
			title: "Coverage update",
			body:  func(a PushArgs) string { return "Your coverage request was updated" },
		},
		PushKeyAnnouncement: {
			title: "New announcement",
			body:  func(a PushArgs) string { return a.Title },
		},
		PushKeyClaimStolen: {
			title: "Claim taken over",
			body: func(a PushArgs) string {
				return fmt.Sprintf("%s took over your %s", a.StealerName, a.Resource)
			},
		},
	},
	telegramLocaleES:   spanishPushMessages(false),
	telegramLocaleESAR: spanishPushMessages(true),
}

// spanishPushMessages builds the Spanish push table. Rioplatense (es-AR) differs
// from neutral Spanish in the reservation title ("Nueva reserva" vs "Nueva
// reservación") and the shift-assigned body ("Tenés" vs "Tienes") — the same
// es vs es_ar split the Telegram and email templates make — so both are derived
// from one builder to stay in lockstep.
func spanishPushMessages(rioplatense bool) map[PushKey]pushMessage {
	newReservationTitle := "Nueva reservación"
	if rioplatense {
		newReservationTitle = "Nueva reserva"
	}
	shiftAssignedBody := func(a PushArgs) string { return fmt.Sprintf("Tienes turno el %s", a.ShiftDate) }
	if rioplatense {
		shiftAssignedBody = func(a PushArgs) string { return fmt.Sprintf("Tenés turno el %s", a.ShiftDate) }
	}
	return map[PushKey]pushMessage{
		PushKeyNewOrder: {
			title: "Nuevo pedido",
			body:  func(a PushArgs) string { return fmt.Sprintf("El pedido N.º %s necesita aprobación", a.OrderNumber) },
		},
		PushKeyNewDeliveryOrder: {
			title: "Nuevo pedido de envío",
			body:  func(a PushArgs) string { return fmt.Sprintf("El pedido N.º %s necesita aprobación", a.OrderNumber) },
		},
		PushKeyNewReservation: {
			title: newReservationTitle,
			body:  func(a PushArgs) string { return fmt.Sprintf("Reserva para %s", a.CustomerName) },
		},
		PushKeyShiftAssigned: {
			title: "Turno asignado",
			body:  shiftAssignedBody,
		},
		PushKeySchedulePublished: {
			title: "Horario publicado",
			body:  func(a PushArgs) string { return fmt.Sprintf("Tu semana del %s ya está", a.WeekOf) },
		},
		PushKeyShiftReminder: {
			title: "Recordatorio de turno",
			body:  func(a PushArgs) string { return fmt.Sprintf("Tu turno del %s se acerca", a.ShiftDate) },
		},
		PushKeyCoverageOffer: {
			title: "Turno disponible",
			body:  func(a PushArgs) string { return "Un turno necesita cobertura — tocá para tomarlo" },
		},
		PushKeyCoverageDecided: {
			title: "Novedad de cobertura",
			body:  func(a PushArgs) string { return "Tu solicitud de cobertura se actualizó" },
		},
		PushKeyAnnouncement: {
			title: "Nuevo anuncio",
			body:  func(a PushArgs) string { return a.Title },
		},
		PushKeyClaimStolen: {
			title: "Te quitaron el reclamo",
			body: func(a PushArgs) string {
				return fmt.Sprintf("%s tomó tu %s", a.StealerName, a.Resource)
			},
		},
	}
}

// LocalizePush resolves the localized (title, body) for a push notification key
// in the operator's language. language is the raw owner language tag (en, es,
// es-AR, es_ar, …); it is resolved through resolveTelegramLocale, so unknown or
// empty values fall back to English. An unknown key returns empty strings.
func LocalizePush(language string, key PushKey, args PushArgs) (title, body string) {
	locale := resolveTelegramLocale(language)
	table, ok := pushMessagesByLocale[locale]
	if !ok {
		table = pushMessagesByLocale[telegramLocaleEN]
	}
	msg, ok := table[key]
	if !ok {
		// Fall back to the English table for this key before giving up, so a
		// locale that somehow misses a key still localizes to English rather
		// than going blank.
		if msg, ok = pushMessagesByLocale[telegramLocaleEN][key]; !ok {
			return "", ""
		}
	}
	return msg.title, msg.body(args)
}
