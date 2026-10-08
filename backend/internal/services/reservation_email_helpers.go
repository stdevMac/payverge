package services

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/locales"
)

// matchGuestLocale returns the canonical guest locale for lang, or "" when the
// input is empty or is not a shipped guest locale. Callers that need a usable
// code fall back themselves, which lets a caller distinguish "the guest did not
// choose" from "the guest chose English".
func matchGuestLocale(lang string) string {
	lang = strings.TrimSpace(lang)
	if lang == "" {
		return ""
	}
	if locales.IsGuestLocale(lang) {
		return lang
	}
	for _, loc := range locales.GuestLocales() {
		if strings.EqualFold(loc.Canonical, lang) {
			return loc.Canonical
		}
	}
	return ""
}

// NormalizeGuestLang returns a supported guest locale code, or "en" when the
// input is empty/unknown. Preserves region tags (e.g. "es-AR").
func NormalizeGuestLang(lang string) string {
	if matched := matchGuestLocale(lang); matched != "" {
		return matched
	}
	return "en"
}

// ResolveBookingLanguage picks the locale stamped on a brand-new reservation:
// an explicit, supported guest choice (the booking page's locale) first, then
// the venue's default language, then English. Without the venue step a Spanish
// venue's walk-in/host bookings get stamped "en" and every downstream guest
// email goes out in the wrong language.
func ResolveBookingLanguage(requested string, business *database.Business) string {
	if matched := matchGuestLocale(requested); matched != "" {
		return matched
	}
	if business != nil {
		if matched := matchGuestLocale(business.DefaultLanguage); matched != "" {
			return matched
		}
	}
	return "en"
}

// ResolveReservationEmailLanguage prefers the language stored on the
// reservation row, then the business default, then English.
func ResolveReservationEmailLanguage(reservation *database.TableReservation, business *database.Business) string {
	if reservation != nil {
		if trimmed := strings.TrimSpace(reservation.Language); trimmed != "" {
			return NormalizeGuestLang(trimmed)
		}
	}
	if business != nil {
		if trimmed := strings.TrimSpace(business.DefaultLanguage); trimmed != "" {
			return NormalizeGuestLang(trimmed)
		}
	}
	return "en"
}

func normalizeReservationPublicURL(baseURL, confirmationCode string) (string, string) {
	base := strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	code := url.PathEscape(strings.TrimSpace(confirmationCode))
	return base, code
}

// WithLangParam appends ?lang=<normalized> (or &lang=) so email deep links
// open the guest confirmation/cancel pages in the booking language.
func WithLangParam(rawURL, lang string) string {
	lang = NormalizeGuestLang(lang)
	if strings.TrimSpace(rawURL) == "" || lang == "" {
		return rawURL
	}
	// Always attach even for "en" so the confirmation page does not fall back
	// to a browser-default locale when the guest booked in English.
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return rawURL + sep + "lang=" + url.QueryEscape(lang)
}

func BuildReservationDetailsURL(baseURL, confirmationCode string, language ...string) string {
	base, code := normalizeReservationPublicURL(baseURL, confirmationCode)
	if base == "" || code == "" {
		return ""
	}

	out := fmt.Sprintf("%s/reservations/%s", base, code)
	if len(language) > 0 {
		out = WithLangParam(out, language[0])
	}
	return out
}

func BuildReservationCancellationURL(baseURL, confirmationCode string, language ...string) string {
	base, code := normalizeReservationPublicURL(baseURL, confirmationCode)
	if base == "" || code == "" {
		return ""
	}

	out := fmt.Sprintf("%s/reservations/%s/cancel", base, code)
	if len(language) > 0 {
		out = WithLangParam(out, language[0])
	}
	return out
}

var spanishWeekdays = [...]string{
	"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado",
}

var spanishMonths = [...]string{
	"enero", "febrero", "marzo", "abril", "mayo", "junio",
	"julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre",
}

// FormatReservationEmailDateTime renders the reservation date and time for
// guest emails in the business's timezone and email language. The es family
// (es, es-AR/es_ar) gets Spanish day/month names and 24-hour time; everything
// else falls back to English (Go's only built-in month/day names).
func FormatReservationEmailDateTime(reservationTime time.Time, timezone, language string) (string, string) {
	loc := time.UTC
	if strings.TrimSpace(timezone) != "" {
		if loaded, err := time.LoadLocation(strings.TrimSpace(timezone)); err == nil {
			loc = loaded
		}
	}

	localTime := reservationTime.In(loc)
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(language)), "es") {
		date := fmt.Sprintf("%s, %d de %s de %d",
			spanishWeekdays[localTime.Weekday()],
			localTime.Day(),
			spanishMonths[localTime.Month()-1],
			localTime.Year(),
		)
		return date, localTime.Format("15:04")
	}
	return localTime.Format("Monday, January 2, 2006"), localTime.Format("3:04 PM")
}

func FormatBusinessAddress(business *database.Business) string {
	if business == nil {
		return ""
	}

	parts := make([]string, 0, 4)
	if street := strings.TrimSpace(business.Address.Street); street != "" {
		parts = append(parts, street)
	}
	if city := strings.TrimSpace(business.Address.City); city != "" {
		parts = append(parts, city)
	}
	if state := strings.TrimSpace(business.Address.State); state != "" {
		parts = append(parts, state)
	}

	address := strings.Join(parts, ", ")
	if postalCode := strings.TrimSpace(business.Address.PostalCode); postalCode != "" {
		if address != "" {
			address += " "
		}
		address += postalCode
	}
	if country := strings.TrimSpace(business.Address.Country); country != "" {
		if address != "" {
			address += ", "
		}
		address += country
	}

	return address
}
