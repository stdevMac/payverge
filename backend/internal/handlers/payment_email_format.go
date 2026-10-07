package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/money"
)

// i18n Batch C (C2-lite): formatting helpers for the guest payment-receipt /
// thank-you emails. The template FAMILY (eng/es/es_ar) is already selected by
// the emails package from the language tag; these helpers make the VALUES the
// templates render verbatim (payment_date, line_total, total_amount) match
// that family and the business currency.

// spanishMonths are the lowercase month names used in the Spanish
// "8 de julio de 2026" date convention.
var spanishMonths = [12]string{
	"enero", "febrero", "marzo", "abril", "mayo", "junio",
	"julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre",
}

// receiptEmailLanguageIsSpanish reports whether a business default-language
// tag resolves to the Spanish email family (es or es_ar). Mirrors the tolerant
// spellings the emails template manager accepts (es, es-AR, es_ar, ES…).
func receiptEmailLanguageIsSpanish(language string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "es")
}

// receiptEmailDate renders the receipt timestamp for the email's language
// family: English keeps the historical "January 2, 2006 at 3:04 PM" layout;
// the Spanish families use "2 de enero de 2006, 15:04" (24h).
func receiptEmailDate(t time.Time, language string) string {
	if receiptEmailLanguageIsSpanish(language) {
		return fmt.Sprintf("%d de %s de %d, %02d:%02d",
			t.Day(), spanishMonths[t.Month()-1], t.Year(), t.Hour(), t.Minute())
	}
	return t.Format("January 2, 2006 at 3:04 PM")
}

// receiptEmailMoney renders a stored "cents" amount (major units ×100 for ALL
// currencies) in the business currency with its real minor-unit count —
// "USD 24.00", "ARS 1500.00", "JPY 1000" — mirroring formatDeliveryMoney and
// the provider boundary (money.MajorUnitString). Never a hardcoded "$".
func receiptEmailMoney(cents int64, currency string) string {
	c := strings.ToUpper(strings.TrimSpace(currency))
	if c == "" {
		c = "USD"
	}
	return c + " " + money.MajorUnitString(cents, c)
}
