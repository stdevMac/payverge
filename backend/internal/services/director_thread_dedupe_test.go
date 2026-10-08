package services

import (
	"context"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"

	"github.com/stretchr/testify/require"
)

// Issue #820: the Director Console sidebar showed the same "thinnest margin"
// thread repeated many times. Every Ask without a thread_id unconditionally
// created a new thread row, so a client retry after a dropped SSE stream (the
// client only learns the thread_id from the terminal response.complete event)
// or a scripted verbatim re-ask spawned one more identical row per attempt.
// resolveThread must reuse the recent thread that already carries the exact
// same question instead of stacking duplicates.

const dedupeQuestion = "Which dish has the thinnest margin on the card right now?"

func scriptedDirectorAnswer() *llm.Response {
	return newTextResponse(`{"summary":"Margins reviewed.","diagnosis":"ok","evidence":[],"actions":[],"expected_impact":"","follow_ups":[]}`)
}

func countDirectorThreads(t *testing.T, db *database.DB, businessID uint) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.GetGorm().Model(&database.DirectorConsoleThread{}).
		Where("business_id = ?", businessID).Count(&n).Error)
	return n
}

// A retry of an ask whose stream died before the client learned the thread_id
// must land in the thread the first attempt already created — including its
// unanswered user turn — instead of minting a new sidebar row.
func TestAsk_VerbatimRetryReusesUnansweredThread(t *testing.T) {
	db := newServiceTestDB(t)
	db.GetGorm().AutoMigrate(&database.Payment{}, &database.Bill{})
	business := createTestBusinessForService(t, db, "Dedupe Retry Restaurant")

	// First attempt: thread + user message persisted, stream died before the
	// assistant answer (client never received a thread_id).
	title := dedupeQuestion
	if len(title) > 70 {
		title = title[:70]
	}
	orphan, err := database.CreateDirectorConsoleThread(business.ID, title, "en")
	require.NoError(t, err)
	require.NoError(t, database.SaveDirectorConsoleMessage(&database.DirectorConsoleMessage{
		ThreadID:   orphan.ID,
		BusinessID: business.ID,
		Role:       database.DirectorMessageRoleUser,
		Locale:     "en",
		Content:    dedupeQuestion,
	}))

	prov := &fakeProvider{scripted: []*llm.Response{scriptedDirectorAnswer(), scriptedDirectorAnswer()}}
	svc := newDirectorServiceWithProvider(t, db, prov)

	res, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: business.ID,
		Message:    dedupeQuestion,
		Locale:     "en",
	})
	require.NoError(t, err)
	require.Equal(t, orphan.ID, res.Thread.ID,
		"a verbatim retry must reuse the unanswered thread, not create a duplicate")
	require.EqualValues(t, 1, countDirectorThreads(t, db, business.ID))

	// The retry must not stack a second identical user bubble either.
	var userTurns int64
	require.NoError(t, db.GetGorm().Model(&database.DirectorConsoleMessage{}).
		Where("thread_id = ? AND role = ? AND content = ?", orphan.ID, database.DirectorMessageRoleUser, dedupeQuestion).
		Count(&userTurns).Error)
	require.EqualValues(t, 1, userTurns, "retry must reuse the pending user turn")
}

// A verbatim re-ask shortly after an answered ask (dropped stream after the
// server persisted the answer, scripted QA prompts) appends to the same
// conversation. Nothing is hidden or deleted — the prior turns stay in the
// thread — so this does not conflict with #730's ban on title-collapse in the
// sidebar, which dropped rows.
func TestAsk_VerbatimReaskWithinWindowAppendsToSameThread(t *testing.T) {
	db := newServiceTestDB(t)
	db.GetGorm().AutoMigrate(&database.Payment{}, &database.Bill{})
	business := createTestBusinessForService(t, db, "Dedupe Append Restaurant")

	prov := &fakeProvider{scripted: []*llm.Response{
		scriptedDirectorAnswer(), scriptedDirectorAnswer(),
		scriptedDirectorAnswer(), scriptedDirectorAnswer(),
	}}
	svc := newDirectorServiceWithProvider(t, db, prov)

	first, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: business.ID, Message: dedupeQuestion, Locale: "en",
	})
	require.NoError(t, err)

	second, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: business.ID, Message: dedupeQuestion, Locale: "en",
	})
	require.NoError(t, err)

	require.Equal(t, first.Thread.ID, second.Thread.ID,
		"an immediate verbatim re-ask must append to the existing thread")
	require.EqualValues(t, 1, countDirectorThreads(t, db, business.ID))

	// Both turns survive: 2 user + 2 assistant messages, no history lost.
	var msgs int64
	require.NoError(t, db.GetGorm().Model(&database.DirectorConsoleMessage{}).
		Where("thread_id = ?", first.Thread.ID).Count(&msgs).Error)
	require.EqualValues(t, 4, msgs)
}

// #730 guard: an old answered thread with the same question is a DIFFERENT
// conversation ("how much did we sell tonight" again tomorrow). Reuse is
// bounded by a recency window; outside it a fresh thread is created.
func TestAsk_VerbatimReaskOutsideWindowCreatesNewThread(t *testing.T) {
	db := newServiceTestDB(t)
	db.GetGorm().AutoMigrate(&database.Payment{}, &database.Bill{})
	business := createTestBusinessForService(t, db, "Dedupe Window Restaurant")

	prov := &fakeProvider{scripted: []*llm.Response{
		scriptedDirectorAnswer(), scriptedDirectorAnswer(),
		scriptedDirectorAnswer(), scriptedDirectorAnswer(),
	}}
	svc := newDirectorServiceWithProvider(t, db, prov)

	first, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: business.ID, Message: dedupeQuestion, Locale: "en",
	})
	require.NoError(t, err)

	// Age the first conversation past the reuse window.
	stale := time.Now().Add(-directorThreadReuseWindow - time.Hour)
	require.NoError(t, db.GetGorm().Model(&database.DirectorConsoleThread{}).
		Where("id = ?", first.Thread.ID).
		Update("last_message_at", stale).Error)

	second, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: business.ID, Message: dedupeQuestion, Locale: "en",
	})
	require.NoError(t, err)
	require.NotEqual(t, first.Thread.ID, second.Thread.ID,
		"the same question hours later is a new conversation (#730)")
	require.EqualValues(t, 2, countDirectorThreads(t, db, business.ID))
}

// The pinned demo briefing thread shares nothing with live asks: even a
// verbatim match must never be appended into a pinned (seeded) thread.
func TestAsk_NeverReusesPinnedThread(t *testing.T) {
	db := newServiceTestDB(t)
	db.GetGorm().AutoMigrate(&database.Payment{}, &database.Bill{})
	business := createTestBusinessForService(t, db, "Dedupe Pinned Restaurant")

	pinned, err := database.CreateDirectorConsoleThread(business.ID, dedupeQuestion, "en")
	require.NoError(t, err)
	require.NoError(t, db.GetGorm().Model(&database.DirectorConsoleThread{}).
		Where("id = ?", pinned.ID).Update("pinned", true).Error)
	require.NoError(t, database.SaveDirectorConsoleMessage(&database.DirectorConsoleMessage{
		ThreadID:   pinned.ID,
		BusinessID: business.ID,
		Role:       database.DirectorMessageRoleUser,
		Locale:     "en",
		Content:    dedupeQuestion,
	}))

	prov := &fakeProvider{scripted: []*llm.Response{scriptedDirectorAnswer(), scriptedDirectorAnswer()}}
	svc := newDirectorServiceWithProvider(t, db, prov)

	res, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: business.ID, Message: dedupeQuestion, Locale: "en",
	})
	require.NoError(t, err)
	require.NotEqual(t, pinned.ID, res.Thread.ID, "pinned seed threads are never reused")
}
