package services

import (
	"context"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestThreadLocale_StampedOnLocaleChange(t *testing.T) {
	db := newServiceTestDB(t)
	db.GetGorm().AutoMigrate(&database.Payment{}, &database.Bill{})
	business := createTestBusinessForService(t, db, "Locale Thread Restaurant")
	thread, err := database.CreateDirectorConsoleThread(business.ID, "first", "en")
	require.NoError(t, err)
	assert.Equal(t, "en", thread.Locale)

	final := `{"summary":"Los ingresos crecen.","diagnosis":"","evidence":[],"actions":[],"expected_impact":"","follow_ups":[]}`
	prov := &fakeProvider{scripted: []*llm.Response{newTextResponse(final), newTextResponse(final)}}
	svc := newDirectorServiceWithProvider(t, db, prov)

	tid := thread.ID
	_, err = svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: business.ID, ThreadID: &tid, Message: "¿Cómo van las ventas?", Locale: "es",
	})
	require.NoError(t, err)

	reloaded, err := database.GetDirectorConsoleThreadByID(business.ID, thread.ID)
	require.NoError(t, err)
	assert.Equal(t, "es", reloaded.Locale, "thread locale must be re-stamped on the new ask locale")
}

func TestThreadLocale_NoUpdateWhenUnchanged(t *testing.T) {
	db := newServiceTestDB(t)
	db.GetGorm().AutoMigrate(&database.Payment{}, &database.Bill{})
	business := createTestBusinessForService(t, db, "Same Locale Restaurant")
	thread, err := database.CreateDirectorConsoleThread(business.ID, "first", "es")
	require.NoError(t, err)
	final := `{"summary":"ok","diagnosis":"","evidence":[],"actions":[],"expected_impact":"","follow_ups":[]}`
	prov := &fakeProvider{scripted: []*llm.Response{newTextResponse(final), newTextResponse(final)}}
	svc := newDirectorServiceWithProvider(t, db, prov)
	tid := thread.ID
	_, err = svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: business.ID, ThreadID: &tid, Message: "¿Ventas?", Locale: "es",
	})
	require.NoError(t, err)
	reloaded, err := database.GetDirectorConsoleThreadByID(business.ID, thread.ID)
	require.NoError(t, err)
	assert.Equal(t, "es", reloaded.Locale)
}
