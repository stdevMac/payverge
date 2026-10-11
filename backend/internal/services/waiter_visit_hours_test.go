package services

import (
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/locales"

	"github.com/stretchr/testify/require"
)

// The demo seeds every venue 00:00-00:00, the "always open" window every
// hours reader treats as 24 hours. The guest waiter used to quote it
// literally ("We're open 00:00–00:00 today."), which reads like a closed venue.

func TestWaiterHoursAnswerEqualWindowSaysOpen24Hours(t *testing.T) {
	now := time.Date(2026, 10, 9, 15, 0, 0, 0, time.UTC)
	hours := []database.BusinessOperatingHours{{DayOfWeek: int(now.Weekday()), OpenTime: "00:00", CloseTime: "00:00"}}
	open, closeAt, closed, known := BuildWaiterHoursFacts(hours, "UTC", now)
	require.True(t, known)
	require.False(t, closed, "an equal open and close is an all-day window, not a closed day")

	facts := WaiterVisitFacts{HoursKnown: known, TodayClosed: closed, TodayOpen: open, TodayClose: closeAt}
	require.Equal(t, "We're open 24 hours today.", WaiterHoursAnswer("en", facts))
	require.Equal(t, "Hoy abrimos las 24 horas.", WaiterHoursAnswer("es", facts))
	for _, code := range waiterDeliveryGuestLocales {
		answer := WaiterHoursAnswer(code, facts)
		require.NotContainsf(t, answer, "00:00", "%s quoted the raw 00:00-00:00 window", code)
		require.NotContainsf(t, answer, "{open}", "%s leaked a placeholder", code)
	}
}

func TestWaiterHoursAnswerAllDayMatchesSecondsAndNonMidnight(t *testing.T) {
	for _, pair := range [][2]string{{"00:00:00", "00:00"}, {"09:00", "09:00"}, {"00:00", "24:00"}} {
		facts := WaiterVisitFacts{HoursKnown: true, TodayOpen: pair[0], TodayClose: pair[1]}
		require.Equalf(t, "We're open 24 hours today.", WaiterHoursAnswer("en", facts), "%v", pair)
	}
}

func TestWaiterHoursAnswerOrdinaryAndOvernightWindowsKeepTimes(t *testing.T) {
	facts := WaiterVisitFacts{HoursKnown: true, TodayOpen: "11:00", TodayClose: "23:00"}
	require.Equal(t, "We're open 11:00–23:00 today.", WaiterHoursAnswer("en", facts))
	facts = WaiterVisitFacts{HoursKnown: true, TodayOpen: "18:00", TodayClose: "02:00"}
	require.Equal(t, "We're open 18:00–02:00 today.", WaiterHoursAnswer("en", facts))
	// An unparseable pair is never promoted to "24 hours".
	facts = WaiterVisitFacts{HoursKnown: true, TodayOpen: "late", TodayClose: "late"}
	require.NotContains(t, WaiterHoursAnswer("en", facts), "24 hours")
}

func TestWaiterHoursAllDayCopyCoversEveryGuestLocale(t *testing.T) {
	for _, code := range waiterDeliveryGuestLocales {
		value, ok := hoursOpenAllDayCopy[code]
		require.Truef(t, ok, "hoursOpenAllDayCopy is missing locale %q", code)
		require.NotEmptyf(t, strings.TrimSpace(value), "hoursOpenAllDayCopy[%q] is empty", code)
		require.NotContainsf(t, value, "{", "hoursOpenAllDayCopy[%q] has a placeholder nothing fills", code)
		if code != "en" {
			require.NotEqualf(t, hoursOpenAllDayCopy["en"], value, "hoursOpenAllDayCopy[%q] is untranslated", code)
		}
	}
	require.Len(t, hoursOpenAllDayCopy, len(waiterDeliveryGuestLocales))
	for _, locale := range locales.GuestLocales() {
		require.Containsf(t, hoursOpenAllDayCopy, locale.Canonical, "guest locale %q has no all-day hours copy", locale.Canonical)
	}
}
