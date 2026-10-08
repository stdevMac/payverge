package formatters

import (
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/locales"
)

// printLang is the small set of locales we ship printed-ticket label copy for.
// Everything else collapses to English (the historical default — no regression
// for unknown/empty tags). The es-AR seam is kept distinct from es so the
// Rioplatense bundle can diverge later (e.g. voseo) even though for these short
// noun labels es and es-AR are currently identical.
type printLang string

const (
	printLangEN   printLang = "en"
	printLangES   printLang = "es"
	printLangESAR printLang = "es-AR"
)

// resolvePrintLang maps a raw business locale tag onto one of the three label
// bundles. It keys on the canonical locale registry's EmailFamily — the same
// family axis the email and Telegram tiers use — so the printer label tier
// stays aligned with the rest of the localized backend: family "es" → es,
// "es_ar" → es-AR, anything else (or unknown/empty) → en.
//
// The registry's Lookup is an exact-key match on canonical codes (e.g. "es-AR"),
// so we normalize separator + case first and try a couple of canonical spellings
// before falling back to English. This makes the seam tolerant of the spellings
// that actually reach the print queue (es-AR / es_ar / es-ar / ES).
func resolvePrintLang(language string) printLang {
	code := strings.TrimSpace(language)
	if code == "" {
		return printLangEN
	}
	if fam, ok := emailFamilyForTag(code); ok {
		switch fam {
		case "es":
			return printLangES
		case "es_ar":
			return printLangESAR
		}
	}
	return printLangEN
}

// emailFamilyForTag resolves a loosely-spelled locale tag to its registry
// EmailFamily. It tries the tag as-is, then a "-"-separated lowercase form, then
// the same with an upper-cased region subtag (the canonical es-AR spelling).
func emailFamilyForTag(code string) (string, bool) {
	candidates := []string{code}
	dashed := strings.ReplaceAll(strings.ToLower(code), "_", "-")
	candidates = append(candidates, dashed)
	if i := strings.Index(dashed, "-"); i >= 0 {
		candidates = append(candidates, dashed[:i]+"-"+strings.ToUpper(dashed[i+1:]))
	}
	for _, c := range candidates {
		if loc, ok := locales.Lookup(c); ok {
			return loc.EmailFamily, true
		}
	}
	return "", false
}

// printLabels is the exhaustive bundle of operator-/guest-visible static label
// strings for one locale. Using explicit struct fields (rather than a
// map[key]string) means a missing translation is a compile-time hole, not a
// silent blank label on a printed receipt or kitchen ticket. NOTE: only label
// TEXT is localized here — amounts, currency codes, and layout are untouched.
type printLabels struct {
	// Shared across bill + receipt.
	Bill     string // "Bill"
	Table    string // "Table"
	Date     string // "Date"
	Subtotal string // "Subtotal"
	Tax      string // "Tax"
	Service  string // "Service"
	Total    string // "Total" (bill grand total)

	// Receipt-only.
	Receipt   string // "RECEIPT"
	Tip       string // "Tip"
	TotalPaid string // "Total Paid"
	Method    string // "Method"
	Txn       string // "Txn"
	Paid      string // "*** PAID ***"
	ThankYou  string // "Thank you!"

	// Bill footer.
	PreBill string // "PRE-BILL — Not a receipt"

	// Kitchen ticket.
	Kitchen     string // "KITCHEN"
	OrderNum    string // "Order #" — glued immediately before the order number, so the label must carry its own trailing separator ("#" for en, a trailing space for es/es-AR's "Pedido N.º ")
	Allergens   string // "! ALLERGENS:" — safety banner; MUST localize
	EndOfTicket string // "-- end of ticket --"
}

// printLabelBundles holds one fully-populated label set per shipped locale.
// es-AR currently reuses the es copy (the labels are identical noun forms),
// but it owns its own entry so it can diverge without touching es.
var printLabelBundles = map[printLang]printLabels{
	printLangEN: {
		Bill:        "Bill",
		Table:       "Table",
		Date:        "Date",
		Subtotal:    "Subtotal",
		Tax:         "Tax",
		Service:     "Service",
		Total:       "Total",
		Receipt:     "RECEIPT",
		Tip:         "Tip",
		TotalPaid:   "Total Paid",
		Method:      "Method",
		Txn:         "Txn",
		Paid:        "*** PAID ***",
		ThankYou:    "Thank you!",
		PreBill:     "PRE-BILL — Not a receipt",
		Kitchen:     "KITCHEN",
		OrderNum:    "Order #",
		Allergens:   "! ALLERGENS:",
		EndOfTicket: "-- end of ticket --",
	},
	printLangES: {
		Bill:        "Cuenta",
		Table:       "Mesa",
		Date:        "Fecha",
		Subtotal:    "Subtotal",
		Tax:         "Impuesto",
		Service:     "Servicio",
		Total:       "Total",
		Receipt:     "RECIBO",
		Tip:         "Propina",
		TotalPaid:   "Total pagado",
		Method:      "Método",
		Txn:         "Transacción",
		Paid:        "*** PAGADO ***",
		ThankYou:    "¡Gracias!",
		PreBill:     "PRECUENTA — No es un recibo",
		Kitchen:     "COCINA",
		OrderNum:    "Pedido N.º ",
		Allergens:   "! ALÉRGENOS:",
		EndOfTicket: "-- fin del ticket --",
	},
	printLangESAR: {
		Bill:        "Cuenta",
		Table:       "Mesa",
		Date:        "Fecha",
		Subtotal:    "Subtotal",
		Tax:         "Impuesto",
		Service:     "Servicio",
		Total:       "Total",
		Receipt:     "RECIBO",
		Tip:         "Propina",
		TotalPaid:   "Total pagado",
		Method:      "Método",
		Txn:         "Transacción",
		Paid:        "*** PAGADO ***",
		ThankYou:    "¡Gracias!",
		PreBill:     "PRECUENTA — No es un recibo",
		Kitchen:     "COCINA",
		OrderNum:    "Pedido N.º ",
		Allergens:   "! ALÉRGENOS:",
		EndOfTicket: "-- fin del ticket --",
	},
}

// labelsFor resolves a raw locale tag to its label bundle, falling back to
// English for any unknown/empty tag.
func labelsFor(language string) printLabels {
	if b, ok := printLabelBundles[resolvePrintLang(language)]; ok {
		return b
	}
	return printLabelBundles[printLangEN]
}

// CanonicalPrintLanguage maps any locale tag onto the canonical label-bundle
// tag the print tier ships: "en", "es", or "es-AR". Empty/unknown tags (and
// guest locales without a label bundle) collapse to "en". This is the single
// normalization seam for print-job Language values — call sites store the
// canonical tag so Reprint (which copies the original job's language) and the
// operator queue UI always see one spelling.
func CanonicalPrintLanguage(tag string) string {
	return string(resolvePrintLang(tag))
}

// paymentMethodLabels maps the raw PaymentMethod values stored on Payment rows
// ("cash", "crypto", "cross-chain", "plugin", …) and first-party plugin names
// onto printed display labels per label-bundle locale. Lookup is
// case-insensitive on the method; unknown methods pass through unchanged.
var paymentMethodLabels = map[printLang]map[string]string{
	printLangEN: {
		"cash":        "Cash",
		"card":        "Card",
		"crypto":      "Crypto",
		"cross-chain": "Crypto (cross-chain)",
		"usdc":        "USDC",
		"plugin":      "Online payment",
		"stripe":      "Card (Stripe)",
		"paypal":      "PayPal",
		"mercadopago": "Mercado Pago",
	},
	printLangES: {
		"cash":        "Efectivo",
		"card":        "Tarjeta",
		"crypto":      "Cripto",
		"cross-chain": "Cripto (multi-cadena)",
		"usdc":        "USDC",
		"plugin":      "Pago en línea",
		"stripe":      "Tarjeta (Stripe)",
		"paypal":      "PayPal",
		"mercadopago": "Mercado Pago",
	},
	printLangESAR: {
		"cash":        "Efectivo",
		"card":        "Tarjeta",
		"crypto":      "Cripto",
		"cross-chain": "Cripto (multi-cadena)",
		"usdc":        "USDC",
		"plugin":      "Pago en línea",
		"stripe":      "Tarjeta (Stripe)",
		"paypal":      "PayPal",
		"mercadopago": "Mercado Pago",
	},
}

// PaymentMethodLabel resolves the printed display label for a raw payment
// method value in the given language. Empty methods stay empty; unknown
// methods pass through unchanged.
func PaymentMethodLabel(language, method string) string {
	m := strings.TrimSpace(method)
	if m == "" {
		return ""
	}
	if label, ok := paymentMethodLabels[resolvePrintLang(language)][strings.ToLower(m)]; ok {
		return label
	}
	return method
}

// FormatTicketTime renders a printed-ticket timestamp per label-bundle locale:
// en (and any fallback) keeps the historical "2006-01-02 15:04" layout;
// es / es-AR use the day-first "02/01/2006 15:04" convention.
func FormatTicketTime(language string, t time.Time) string {
	switch resolvePrintLang(language) {
	case printLangES, printLangESAR:
		return t.Format("02/01/2006 15:04")
	default:
		return t.Format("2006-01-02 15:04")
	}
}
