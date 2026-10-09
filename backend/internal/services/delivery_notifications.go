package services

import (
	"fmt"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
)

// publicSiteBaseURL returns the frontend origin used in guest-facing links
// (e.g. the payment link emailed on prepay acceptance): the instance
// PUBLIC_URL.
// Production refuses to boot without one, so links are never blank there.
func publicSiteBaseURL() string {
	return config.PublicURL()
}

// NotificationLocale identifies the language a notification is rendered in.
type NotificationLocale string

const (
	LocaleEN NotificationLocale = "en"
	LocaleES NotificationLocale = "es"
)

// lookupLocaleTemplate maps a free-form locale tag ("EN", "es-ES", "es-AR",
// "Spanish") onto the small set of locales we have delivery templates for
// (en/es only — this is an intentional platform contract, NOT all 21 guest
// locales). The second return reports whether a template actually exists for
// the tag: a Japanese ("ja") request resolves (LocaleEN, false) so the caller
// can decide to fall back to the BUSINESS DEFAULT instead of silently emitting
// English. Argentine Spanish ("es-AR") is Rioplatense Spanish, so it maps to
// the Spanish template (es_ar has no distinct delivery template today).
func lookupLocaleTemplate(raw string) (NotificationLocale, bool) {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case s == "":
		return LocaleEN, false
	case strings.HasPrefix(s, "es"), s == "spanish", s == "español", s == "espanol":
		// Covers es, es-ES, es-MX, es-AR, es_AR, … — all Spanish templates.
		return LocaleES, true
	case strings.HasPrefix(s, "en"), s == "english":
		return LocaleEN, true
	default:
		// A locale we have no delivery template for. Report "no template" so the
		// caller can fall back to the business default rather than assuming the
		// customer reads English.
		return LocaleEN, false
	}
}

// guestNotificationLocale resolves the locale used for customer-facing copy.
// Order: explicit per-delivery locale (if we have a template for it) → business
// default language (if we have a template for it) → English. A customer locale
// we cannot serve (e.g. "ja") is NOT silently rendered in English — it falls
// through to the business's own default language first.
func guestNotificationLocale(delivery *database.DeliveryOrder, business *database.Business) NotificationLocale {
	if delivery != nil {
		if locale, ok := lookupLocaleTemplate(delivery.CustomerLocale); ok {
			return locale
		}
	}
	return businessNotificationLocale(business)
}

// businessNotificationLocale resolves the locale used for operator-facing copy,
// and serves as the fallback for guest copy when the customer locale has no
// template. Order: business default language (if we have a template) → English.
func businessNotificationLocale(business *database.Business) NotificationLocale {
	if business != nil {
		if locale, ok := lookupLocaleTemplate(business.DefaultLanguage); ok {
			return locale
		}
	}
	return LocaleEN
}

type notificationTemplates struct {
	driverAssignedTitle     string
	driverAssignedBody      string
	onTheWayTitle           string
	onTheWayBody            string
	almostThereTitle        string
	almostThereBody         string
	deliveredTitle          string
	deliveredBody           string
	cancelledTitle          string
	cancelledBody           string
	cancelledBodyWithReason string

	newDeliveryTitle    string
	newDeliveryBodyFmt  string
	opsAssignedTitle    string
	opsAssignedBodyFmt  string
	opsCancelledTitle   string
	opsCancelledBodyFmt string
}

var notificationTemplatesByLocale = map[NotificationLocale]notificationTemplates{
	LocaleEN: {
		driverAssignedTitle:     "Driver Assigned",
		driverAssignedBody:      "Your delivery has been assigned to a driver.",
		onTheWayTitle:           "On the Way",
		onTheWayBody:            "Your delivery is on the way.",
		almostThereTitle:        "Almost There",
		almostThereBody:         "Your delivery is nearby.",
		deliveredTitle:          "Delivered",
		deliveredBody:           "Your delivery has been completed. Thank you!",
		cancelledTitle:          "Delivery Cancelled",
		cancelledBody:           "Your delivery has been cancelled.",
		cancelledBodyWithReason: "Your delivery has been cancelled. Reason: %s",

		newDeliveryTitle:    "New Delivery Order",
		newDeliveryBodyFmt:  "New delivery order %s from %s to %s.",
		opsAssignedTitle:    "Driver Assigned",
		opsAssignedBodyFmt:  "Driver assigned to delivery %s for customer %s.",
		opsCancelledTitle:   "Delivery Cancelled",
		opsCancelledBodyFmt: "Delivery %s for %s has been cancelled.",
	},
	LocaleES: {
		driverAssignedTitle:     "Repartidor asignado",
		driverAssignedBody:      "Tu entrega ha sido asignada a un repartidor.",
		onTheWayTitle:           "En camino",
		onTheWayBody:            "Tu entrega está en camino.",
		almostThereTitle:        "Casi llegamos",
		almostThereBody:         "Tu entrega está cerca.",
		deliveredTitle:          "Entregado",
		deliveredBody:           "Tu entrega ha sido completada. ¡Gracias!",
		cancelledTitle:          "Entrega cancelada",
		cancelledBody:           "Tu entrega ha sido cancelada.",
		cancelledBodyWithReason: "Tu entrega ha sido cancelada. Motivo: %s",

		newDeliveryTitle:    "Nuevo pedido de entrega",
		newDeliveryBodyFmt:  "Nuevo pedido de entrega %s de %s a %s.",
		opsAssignedTitle:    "Repartidor asignado",
		opsAssignedBodyFmt:  "Repartidor asignado a la entrega %s para el cliente %s.",
		opsCancelledTitle:   "Entrega cancelada",
		opsCancelledBodyFmt: "La entrega %s para %s ha sido cancelada.",
	},
}

func templatesFor(locale NotificationLocale) notificationTemplates {
	if t, ok := notificationTemplatesByLocale[locale]; ok {
		return t
	}
	return notificationTemplatesByLocale[LocaleEN]
}

// isGuestVisibleStatus is the locale-independent gate for whether a lifecycle
// transition produces a customer-facing notification. Single source of truth
// for "should we even bother loading the business / rendering a body?".
func isGuestVisibleStatus(status database.DeliveryStatus) bool {
	switch status {
	case database.DeliveryStatusAssigned,
		database.DeliveryStatusPickedUp,
		database.DeliveryStatusNearby,
		database.DeliveryStatusDelivered,
		database.DeliveryStatusCancelled:
		return true
	default:
		return false
	}
}

// localizedGuestMessage returns the customer-facing message for a lifecycle
// transition, or ("", "", false) for operational-only transitions. Visibility
// matches isGuestVisibleStatus by construction.
func localizedGuestMessage(status database.DeliveryStatus, reason string, locale NotificationLocale) (title, body string, visible bool) {
	t := templatesFor(locale)
	switch status {
	case database.DeliveryStatusAssigned:
		return t.driverAssignedTitle, t.driverAssignedBody, true
	case database.DeliveryStatusPickedUp:
		return t.onTheWayTitle, t.onTheWayBody, true
	case database.DeliveryStatusNearby:
		return t.almostThereTitle, t.almostThereBody, true
	case database.DeliveryStatusDelivered:
		return t.deliveredTitle, t.deliveredBody, true
	case database.DeliveryStatusCancelled:
		if strings.TrimSpace(reason) != "" {
			return t.cancelledTitle, fmt.Sprintf(t.cancelledBodyWithReason, reason), true
		}
		return t.cancelledTitle, t.cancelledBody, true
	default:
		return "", "", false
	}
}

func localizedNewDeliveryMessage(deliveryNumber, customerName, addressStreet string, locale NotificationLocale) (title, body string) {
	t := templatesFor(locale)
	return t.newDeliveryTitle, fmt.Sprintf(t.newDeliveryBodyFmt, deliveryNumber, customerName, addressStreet)
}

func localizedOpsDriverAssignedMessage(deliveryNumber, customerName string, locale NotificationLocale) (title, body string) {
	t := templatesFor(locale)
	return t.opsAssignedTitle, fmt.Sprintf(t.opsAssignedBodyFmt, deliveryNumber, customerName)
}

func localizedOpsCancellationMessage(deliveryNumber, customerName string, locale NotificationLocale) (title, body string) {
	t := templatesFor(locale)
	return t.opsCancelledTitle, fmt.Sprintf(t.opsCancelledBodyFmt, deliveryNumber, customerName)
}

// localizedReceivedMessage is sent to the guest immediately after a successful
// checkout — confirms the order landed and sets expectations for next steps.
func localizedReceivedMessage(deliveryNumber string, locale NotificationLocale) (string, string) {
	if locale == LocaleES {
		return "Pedido recibido",
			fmt.Sprintf("Recibimos tu pedido %s. Te avisaremos por este medio cuando el restaurante lo confirme.", deliveryNumber)
	}
	return "Order received",
		fmt.Sprintf("We received your order %s. We'll email you as soon as the restaurant confirms it.", deliveryNumber)
}

// localizedAcceptedPayMessage is sent on prepay acceptance: the kitchen is
// waiting and the guest has deliveryPaymentWindow minutes to pay.
func localizedAcceptedPayMessage(deliveryNumber, payURL string, minutes int, locale NotificationLocale) (string, string) {
	if locale == LocaleES {
		return "Pedido aceptado — realiza el pago para empezar",
			fmt.Sprintf("Tu pedido %s fue aceptado. Realiza el pago en los próximos %d minutos para que la cocina empiece a prepararlo: %s", deliveryNumber, minutes, payURL)
	}
	return "Order accepted — pay to start preparation",
		fmt.Sprintf("Your order %s was accepted. Pay within %d minutes so the kitchen can start preparing it: %s", deliveryNumber, minutes, payURL)
}

// localizedAcceptedCODMessage is sent on COD acceptance: the kitchen starts
// now and the guest pays in cash on delivery.
func localizedAcceptedCODMessage(deliveryNumber, totalDisplay string, locale NotificationLocale) (string, string) {
	if locale == LocaleES {
		return "Pedido aceptado",
			fmt.Sprintf("Tu pedido %s fue aceptado y la cocina ya lo está preparando. Pagarás %s en efectivo al recibirlo.", deliveryNumber, totalDisplay)
	}
	return "Order accepted",
		fmt.Sprintf("Your order %s was accepted and the kitchen is preparing it. Pay %s in cash on delivery.", deliveryNumber, totalDisplay)
}

// localizedPaymentReceivedMessage is sent after a prepay payment is confirmed
// and the kitchen starts the order.
func localizedPaymentReceivedMessage(deliveryNumber string, locale NotificationLocale) (string, string) {
	if locale == LocaleES {
		return "Pago recibido",
			fmt.Sprintf("Recibimos tu pago del pedido %s. La cocina ya lo está preparando.", deliveryNumber)
	}
	return "Payment received",
		fmt.Sprintf("We received your payment for order %s. The kitchen is preparing it now.", deliveryNumber)
}

// localizedExpiredMessage is sent when a prepay order's payment window lapses
// with no payment — no charge was made.
func localizedExpiredMessage(deliveryNumber string, locale NotificationLocale) (string, string) {
	if locale == LocaleES {
		return "Pedido vencido",
			fmt.Sprintf("Tu pedido %s venció porque no se completó el pago a tiempo. No se realizó ningún cargo. Podrás volver a pedir cuando quieras.", deliveryNumber)
	}
	return "Order expired",
		fmt.Sprintf("Your order %s expired because payment wasn't completed in time. Nothing was charged. You're welcome to order again anytime.", deliveryNumber)
}
