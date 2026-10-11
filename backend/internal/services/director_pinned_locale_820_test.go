package services

import (
	"context"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"

	"github.com/stretchr/testify/require"
)

// Issue #820, second half: live thread 10 on the AI-Pro demo venue is the
// pinned "Demo Director Briefing" card — English seeded copy — carrying
// locale es-AR. Ask() used to stamp the answering locale onto EVERY thread it
// touched, so one Spanish follow-up asked inside the pinned card relabeled the
// English briefing for good (the demo reseed keys the card on its TITLE, so it
// never converged back). The card's locale describes the briefing it renders;
// each answer keeps its own per-message locale.

func seedPinnedBriefing(t *testing.T, db *database.DB, businessID uint, locale string) *database.DirectorConsoleThread {
	t.Helper()
	thread, err := database.CreateDirectorConsoleThread(businessID, "Demo Director Briefing", locale)
	require.NoError(t, err)
	require.NoError(t, db.GetGorm().Model(&database.DirectorConsoleThread{}).
		Where("id = ?", thread.ID).Update("pinned", true).Error)
	require.NoError(t, database.SaveDirectorConsoleMessage(&database.DirectorConsoleMessage{
		ThreadID:   thread.ID,
		BusinessID: businessID,
		Role:       database.DirectorMessageRoleAssistant,
		Locale:     locale,
		Content:    "Dinner revenue is tracking above the 30-day median. Push cocktails before 8pm.",
		ModelName:  "demo-seed",
	}))
	thread.Pinned = true
	return thread
}

func threadLocale(t *testing.T, db *database.DB, threadID uint) string {
	t.Helper()
	var row database.DirectorConsoleThread
	require.NoError(t, db.GetGorm().First(&row, threadID).Error)
	return row.Locale
}

func TestAsk_PinnedBriefingKeepsItsSeededLocale(t *testing.T) {
	db := newServiceTestDB(t)
	db.GetGorm().AutoMigrate(&database.Payment{}, &database.Bill{})
	business := createTestBusinessForService(t, db, "Pinned Briefing Restaurant")

	pinned := seedPinnedBriefing(t, db, business.ID, "en")

	prov := &fakeProvider{scripted: []*llm.Response{scriptedDirectorAnswer()}}
	svc := newDirectorServiceWithProvider(t, db, prov)

	res, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: business.ID,
		Message:    "cerrá la caja",
		ThreadID:   &pinned.ID,
		Locale:     "es-AR",
	})
	require.NoError(t, err)
	require.Equal(t, pinned.ID, res.Thread.ID)

	require.Equal(t, "en", threadLocale(t, db, pinned.ID),
		"a Spanish follow-up must not relabel the English seeded briefing card (#820, live thread 10)")

	// The answer itself is still Spanish — only the card's label is pinned to
	// the briefing it renders.
	require.Equal(t, "es-AR", res.AssistantMessage.Locale,
		"the follow-up answer keeps the language it was written in")

	var seeded database.DirectorConsoleMessage
	require.NoError(t, db.GetGorm().Where("thread_id = ? AND model_name = ?", pinned.ID, "demo-seed").
		First(&seeded).Error)
	require.Equal(t, "en", seeded.Locale, "the seeded briefing message is untouched")
}

// The exemption is narrow: an ordinary (unpinned) conversation still follows
// the operator's current UI language, which is what the stamp exists for.
func TestAsk_UnpinnedThreadStillFollowsAskLocale(t *testing.T) {
	db := newServiceTestDB(t)
	db.GetGorm().AutoMigrate(&database.Payment{}, &database.Bill{})
	business := createTestBusinessForService(t, db, "Unpinned Thread Restaurant")

	thread, err := database.CreateDirectorConsoleThread(business.ID, "how much did we sell tonight", "en")
	require.NoError(t, err)

	prov := &fakeProvider{scripted: []*llm.Response{scriptedDirectorAnswer()}}
	svc := newDirectorServiceWithProvider(t, db, prov)

	_, err = svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: business.ID,
		Message:    "¿cuánto vendimos hoy?",
		ThreadID:   &thread.ID,
		Locale:     "es-AR",
	})
	require.NoError(t, err)

	require.Equal(t, "es-AR", threadLocale(t, db, thread.ID),
		"a normal thread must still re-stamp when the operator switches language mid-conversation")
}
