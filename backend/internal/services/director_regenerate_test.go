package services

import (
	"context"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// L4-15 — regenerate must replace the trailing assistant answer safely:
//   - a nil/zero thread_id is rejected BEFORE any thread can be created;
//   - a failed or canned generation (loop error, budget cap) must never
//     destroy the previous answer (the delete is deferred until a successful
//     replacement exists and happens atomically with the insert);
//   - a successful regenerate replaces the trailing assistant in place
//     (old row gone, new row present, no duplicate user turn).

const regenerateOriginalAnswer = "Original Tuesday answer with hard-won detail."

// seedRegenerateThread creates a thread holding one user turn and one
// trailing assistant turn, mimicking a completed exchange.
func seedRegenerateThread(t *testing.T, businessID uint) (*database.DirectorConsoleThread, *database.DirectorConsoleMessage) {
	t.Helper()

	thread, err := database.CreateDirectorConsoleThread(businessID, "regen test", "en")
	require.NoError(t, err)

	userMsg := &database.DirectorConsoleMessage{
		ThreadID:   thread.ID,
		BusinessID: businessID,
		Role:       database.DirectorMessageRoleUser,
		Locale:     "en",
		Content:    "Why was Tuesday slow?",
	}
	require.NoError(t, database.SaveDirectorConsoleMessage(userMsg))

	assistantMsg := &database.DirectorConsoleMessage{
		ThreadID:   thread.ID,
		BusinessID: businessID,
		Role:       database.DirectorMessageRoleAssistant,
		Locale:     "en",
		Content:    regenerateOriginalAnswer,
		ModelName:  "test-model",
	}
	require.NoError(t, database.SaveDirectorConsoleMessage(assistantMsg))

	return thread, assistantMsg
}

func threadCount(t *testing.T, db *database.DB, businessID uint) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.GetGorm().
		Model(&database.DirectorConsoleThread{}).
		Where("business_id = ?", businessID).
		Count(&count).Error)
	return count
}

func TestRegenerate_NilThreadRejectedWithoutCreatingThread(t *testing.T) {
	db := newServiceTestDB(t)
	db.GetGorm().AutoMigrate(&database.Payment{}, &database.Bill{})
	business := createTestBusinessForService(t, db, "Regen NilThread Restaurant")

	final := `{"summary":"unused","diagnosis":"","evidence":[],"actions":[],"expected_impact":"","follow_ups":[]}`
	prov := &fakeProvider{scripted: []*llm.Response{newTextResponse(final), newTextResponse(final)}}
	svc := newDirectorServiceWithProvider(t, db, prov)

	_, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: business.ID,
		Message:    "Why was Tuesday slow?",
		Regenerate: true,
		ThreadID:   nil,
	})
	require.Error(t, err, "regenerate without thread_id must be rejected")
	assert.Contains(t, err.Error(), "thread_id")

	// The rejection must happen BEFORE resolveThread — no orphan thread row.
	assert.Equal(t, int64(0), threadCount(t, db, business.ID),
		"nil-thread regenerate must not create an orphan thread")
}

func TestRegenerate_ZeroThreadRejectedWithoutCreatingThread(t *testing.T) {
	db := newServiceTestDB(t)
	db.GetGorm().AutoMigrate(&database.Payment{}, &database.Bill{})
	business := createTestBusinessForService(t, db, "Regen ZeroThread Restaurant")

	prov := &fakeProvider{}
	svc := newDirectorServiceWithProvider(t, db, prov)

	zero := uint(0)
	_, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: business.ID,
		Message:    "Why was Tuesday slow?",
		Regenerate: true,
		ThreadID:   &zero,
	})
	require.Error(t, err, "regenerate with thread_id=0 must be rejected")
	assert.Equal(t, int64(0), threadCount(t, db, business.ID),
		"zero-thread regenerate must not create an orphan thread")
}

func TestRegenerate_GenerationErrorPreservesTrailingAnswer(t *testing.T) {
	db := newServiceTestDB(t)
	db.GetGorm().AutoMigrate(&database.Payment{}, &database.Bill{})
	business := createTestBusinessForService(t, db, "Regen Error Restaurant")
	thread, _ := seedRegenerateThread(t, business.ID)

	// A provider with no scripted responses fails every Generate call, driving
	// the loop into its fallback-error path.
	prov := &fakeProvider{}
	svc := newDirectorServiceWithProvider(t, db, prov)

	tid := thread.ID
	res, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: business.ID,
		Message:    "Why was Tuesday slow?",
		Regenerate: true,
		ThreadID:   &tid,
	})
	require.NoError(t, err)
	require.Equal(t, "fallback_error", res.Usage.Model,
		"scenario sanity: the loop must have failed into the fallback path")

	msgs, err := database.ListDirectorConsoleMessages(business.ID, thread.ID, 50)
	require.NoError(t, err)

	var sawOriginal bool
	for _, m := range msgs {
		if m.Role == database.DirectorMessageRoleAssistant && m.Content == regenerateOriginalAnswer {
			sawOriginal = true
		}
	}
	assert.True(t, sawOriginal,
		"a failed regeneration must NOT destroy the previous answer (hard delete before generation)")
}

func TestRegenerate_BudgetCapPreservesTrailingAnswer(t *testing.T) {
	db := newServiceTestDB(t)
	db.GetGorm().AutoMigrate(&database.Payment{}, &database.Bill{})
	business := createTestBusinessForService(t, db, "Regen Budget Restaurant")
	thread, _ := seedRegenerateThread(t, business.ID)

	final := `{"summary":"unused","diagnosis":"","evidence":[],"actions":[],"expected_impact":"","follow_ups":[]}`
	prov := &fakeProvider{scripted: []*llm.Response{newTextResponse(final), newTextResponse(final)}}
	svc := newDirectorServiceWithProvider(t, db, prov).WithCostGate(stubBudgetGate{over: true})

	tid := thread.ID
	res, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: business.ID,
		Message:    "Why was Tuesday slow?",
		Regenerate: true,
		ThreadID:   &tid,
	})
	require.NoError(t, err)
	require.Equal(t, "budget", res.Usage.Model,
		"scenario sanity: the canned budget notice must have been served")

	msgs, err := database.ListDirectorConsoleMessages(business.ID, thread.ID, 50)
	require.NoError(t, err)

	var sawOriginal bool
	for _, m := range msgs {
		if m.Role == database.DirectorMessageRoleAssistant && m.Content == regenerateOriginalAnswer {
			sawOriginal = true
		}
	}
	assert.True(t, sawOriginal,
		"the budget-cap canned message must NOT replace (destroy) the previous answer")
}

func TestRegenerate_SuccessReplacesTrailingAnswerInPlace(t *testing.T) {
	db := newServiceTestDB(t)
	db.GetGorm().AutoMigrate(&database.Payment{}, &database.Bill{})
	business := createTestBusinessForService(t, db, "Regen Success Restaurant")
	thread, oldAssistant := seedRegenerateThread(t, business.ID)

	final := `{"summary":"Regenerated Tuesday answer.","diagnosis":"fresh look","evidence":[],"actions":[],"expected_impact":"","follow_ups":[]}`
	cap := &capturingDirectorProvider{scripted: []*llm.Response{
		newTextResponse(final), // tool-allowed probe
		newTextResponse(final), // schema finalization
	}}
	svc := newDirectorServiceWithProvider(t, db, cap)

	tid := thread.ID
	res, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: business.ID,
		Message:    "Why was Tuesday slow?",
		Regenerate: true,
		ThreadID:   &tid,
	})
	require.NoError(t, err)
	assert.Equal(t, "Regenerated Tuesday answer.", res.Response.Summary)

	msgs, err := database.ListDirectorConsoleMessages(business.ID, thread.ID, 50)
	require.NoError(t, err)

	// Delete + insert land together: exactly one user turn and exactly one
	// assistant turn — the new one. The old answer is gone; no duplicate user.
	var users, assistants int
	for _, m := range msgs {
		switch m.Role {
		case database.DirectorMessageRoleUser:
			users++
		case database.DirectorMessageRoleAssistant:
			assistants++
			assert.NotEqual(t, oldAssistant.ID, m.ID, "old assistant row must be deleted")
			assert.Equal(t, "Regenerated Tuesday answer.", m.Content)
		}
	}
	assert.Equal(t, 1, users, "regenerate must not append a duplicate user turn")
	assert.Equal(t, 1, assistants, "the trailing answer must be replaced in place")

	// The replaced answer must not leak into the regeneration prompt as memory
	// (it was previously deleted up front, keeping it out of prior turns).
	for _, reqCap := range cap.requests {
		for _, msg := range reqCap.Messages {
			assert.NotContains(t, msg.Text, regenerateOriginalAnswer,
				"the answer being replaced must not be replayed as a prior turn")
		}
	}
}
