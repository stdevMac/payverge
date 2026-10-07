package services

import (
	"testing"
	"time"
)

// 2026-05-11 22:30 UTC == 19:30 in Buenos Aires (UTC-3), a Monday.
var emailDTSample = time.Date(2026, 5, 11, 22, 30, 0, 0, time.UTC)

func TestFormatReservationEmailDateTime_EnglishDefault(t *testing.T) {
	date, clock := FormatReservationEmailDateTime(emailDTSample, "America/Argentina/Buenos_Aires", "en")
	if date != "Monday, May 11, 2026" {
		t.Fatalf("date = %q", date)
	}
	if clock != "7:30 PM" {
		t.Fatalf("clock = %q", clock)
	}
}

func TestFormatReservationEmailDateTime_SpanishFamily(t *testing.T) {
	for _, lang := range []string{"es", "es-AR", "es_ar", "ES"} {
		date, clock := FormatReservationEmailDateTime(emailDTSample, "America/Argentina/Buenos_Aires", lang)
		if date != "lunes, 11 de mayo de 2026" {
			t.Fatalf("lang %s: date = %q", lang, date)
		}
		if clock != "19:30" {
			t.Fatalf("lang %s: clock = %q", lang, clock)
		}
	}
}

func TestFormatReservationEmailDateTime_UnknownLanguageFallsBackToEnglish(t *testing.T) {
	date, clock := FormatReservationEmailDateTime(emailDTSample, "UTC", "fr")
	if date != "Monday, May 11, 2026" {
		t.Fatalf("date = %q", date)
	}
	if clock != "10:30 PM" {
		t.Fatalf("clock = %q", clock)
	}
}

func TestFormatReservationEmailDateTime_BadTimezoneFallsBackToUTC(t *testing.T) {
	date, clock := FormatReservationEmailDateTime(emailDTSample, "Not/AZone", "es")
	if date != "lunes, 11 de mayo de 2026" {
		t.Fatalf("date = %q", date)
	}
	if clock != "22:30" {
		t.Fatalf("clock = %q", clock)
	}
}

func TestNormalizeGuestLang(t *testing.T) {
	if got := NormalizeGuestLang("  fr "); got != "fr" {
		t.Fatalf("fr = %q", got)
	}
	if got := NormalizeGuestLang(""); got != "en" {
		t.Fatalf("empty = %q", got)
	}
	if got := NormalizeGuestLang("nope"); got != "en" {
		t.Fatalf("unknown = %q", got)
	}
}

func TestWithLangParamAndBuildURLs(t *testing.T) {
	if got := WithLangParam("https://payverge.io/reservations/ABC", "es"); got != "https://payverge.io/reservations/ABC?lang=es" {
		t.Fatalf("WithLangParam = %q", got)
	}
	if got := WithLangParam("https://payverge.io/x?foo=1", "ja"); got != "https://payverge.io/x?foo=1&lang=ja" {
		t.Fatalf("WithLangParam existing query = %q", got)
	}
	if got := BuildReservationDetailsURL("https://payverge.io", "ABC123", "fr"); got != "https://payverge.io/reservations/ABC123?lang=fr" {
		t.Fatalf("details URL = %q", got)
	}
	if got := BuildReservationCancellationURL("https://payverge.io", "ABC123", "es-AR"); got != "https://payverge.io/reservations/ABC123/cancel?lang=es-AR" {
		t.Fatalf("cancel URL = %q", got)
	}
	// Back-compat: no language arg leaves URL bare.
	if got := BuildReservationDetailsURL("https://payverge.io", "ABC123"); got != "https://payverge.io/reservations/ABC123" {
		t.Fatalf("details without lang = %q", got)
	}
}
