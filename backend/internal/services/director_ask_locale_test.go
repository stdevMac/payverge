package services

import (
	"context"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveDirectorAskLocale_FallsBackToOwnerLanguage proves the M4 fix: when
// the Director Console Ask carries no explicit locale, the answer locale is the
// business owner's UI language — not the English default.
func TestResolveDirectorAskLocale_FallsBackToOwnerLanguage(t *testing.T) {
	esAR := &database.Business{DefaultLanguage: "es-AR"}

	// Blank request locale → owner language (es-AR canonicalizes to es-AR).
	assert.Equal(t, "es-AR", resolveDirectorAskLocale("", esAR),
		"blank request locale must resolve to the es-AR owner's language, not English")

	// Explicit request locale always wins over the owner language.
	assert.Equal(t, "en", resolveDirectorAskLocale("en", esAR),
		"an explicit request locale must be honored over the owner language")

	// Owner with no language at all → English default.
	assert.Equal(t, "en", resolveDirectorAskLocale("", &database.Business{}))
}

// TestAsk_BlankLocaleUsesOwnerLanguage is the end-to-end regression: an Ask with
// an empty locale for an es-AR owner stamps the thread (and thus the answer) in
// Spanish rather than falling through to English.
func TestAsk_BlankLocaleUsesOwnerLanguage(t *testing.T) {
	db := newServiceTestDB(t)
	db.GetGorm().AutoMigrate(&database.Payment{}, &database.Bill{})

	business := database.Business{
		Name:            "Owner Language Restaurant",
		SettlementAddr:  "settlement-owner-lang",
		TippingAddr:     "tipping-owner-lang",
		DefaultLanguage: "es-AR",
	}
	require.NoError(t, db.GetGorm().Create(&business).Error)

	final := `{"summary":"Las ventas crecen.","diagnosis":"","evidence":[],"actions":[],"expected_impact":"","follow_ups":[]}`
	prov := &fakeProvider{scripted: []*llm.Response{newTextResponse(final), newTextResponse(final)}}
	svc := newDirectorServiceWithProvider(t, db, prov)

	res, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: business.ID,
		Message:    "¿Cómo van las ventas?",
		Locale:     "", // client sends no locale
	})
	require.NoError(t, err)

	reloaded, err := database.GetDirectorConsoleThreadByID(business.ID, res.Thread.ID)
	require.NoError(t, err)
	assert.Equal(t, "es-AR", reloaded.Locale,
		"a blank-locale Ask must adopt the es-AR owner's language, not the English default")
}
