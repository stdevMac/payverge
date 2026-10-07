package server

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestSchedulePromotionTranslationBackfill_Dedup verifies the singleflight guard
// that replaced the old raw `go backfillPromotionTranslationsForLanguage(...)`:
// a backfill already in flight for the same (business, language) must not launch
// a second goroutine. Deterministic — asserts the in-flight map state without
// depending on goroutine timing.
func TestSchedulePromotionTranslationBackfill_Dedup(t *testing.T) {
	const businessID = 4242
	const lang = "fr"
	key := "4242:fr"
	offers := []database.Offer{{BusinessID: businessID, Name: "Tapas"}}

	// Simulate a backfill already running for this (business, language).
	promotionTranslationBackfillInFlight.Store(key, struct{}{})
	defer promotionTranslationBackfillInFlight.Delete(key)

	// A concurrent schedule must hit the alreadyRunning branch and return
	// without launching: it must NOT delete the in-flight marker we pre-stored.
	schedulePromotionTranslationBackfill(businessID, "en", lang, offers, nil)

	if _, stillInFlight := promotionTranslationBackfillInFlight.Load(key); !stillInFlight {
		t.Fatal("guard failed: a duplicate schedule cleared the in-flight marker (would allow a thundering herd)")
	}
}

// TestSchedulePromotionTranslationBackfill_NoWorkNoKey verifies empty inputs are
// a no-op that never registers an in-flight key (so it can't wedge the guard).
func TestSchedulePromotionTranslationBackfill_NoWorkNoKey(t *testing.T) {
	schedulePromotionTranslationBackfill(7, "en", "", nil, nil) // empty language
	if _, present := promotionTranslationBackfillInFlight.Load("7:"); present {
		t.Fatal("empty-language schedule should not register an in-flight key")
	}

	schedulePromotionTranslationBackfill(8, "en", "de", nil, nil) // no offers/bundles
	if _, present := promotionTranslationBackfillInFlight.Load("8:de"); present {
		t.Fatal("no-offers/no-bundles schedule should not register an in-flight key")
	}
}
