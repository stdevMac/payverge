package emails

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// outboxTestDB is the email package's sqlite fixture (which includes the outbox
// table), named for what these tests exercise.
func outboxTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return suppressionTestDB(t)
}

// outboxProviderStub fails the first failTimes calls, then succeeds.
type outboxProviderStub struct {
	failTimes int
	failErr   error
	calls     int
	keys      []string
}

func (p *outboxProviderStub) Send(ctx context.Context, msg EmailMessage) error {
	_, err := p.SendWithReceipt(ctx, msg)
	return err
}

func (p *outboxProviderStub) SendWithReceipt(_ context.Context, msg EmailMessage) (SendResult, error) {
	p.calls++
	p.keys = append(p.keys, msg.IdempotencyKey)
	if p.calls <= p.failTimes {
		return SendResult{}, p.failErr
	}
	return SendResult{ProviderMessageID: fmt.Sprintf("re_outbox_%d", p.calls)}, nil
}

func (p *outboxProviderStub) ProviderName() string { return "resend" }

func newOutboxTestServer(t *testing.T, db *gorm.DB, provider EmailProvider) (*EmailServer, *GormOutboxStore) {
	t.Helper()
	server, err := NewEmailServer(provider, "noreply@payverge.io", "updates@payverge.io", resolveTemplatesRoot(t))
	require.NoError(t, err)
	server.SetOutboundStore(NewGormOutboundStore(db))
	store := NewGormOutboxStore(db)
	server.SetOutboxStore(store)
	return server, store
}

func loadOutboxRow(t *testing.T, db *gorm.DB, id uint) EmailOutbox {
	t.Helper()
	var row EmailOutbox
	require.NoError(t, db.First(&row, id).Error)
	return row
}

func TestDispatchEnqueuesInsteadOfCallingProvider(t *testing.T) {
	db := outboxTestDB(t)
	provider := &outboxProviderStub{}
	server, _ := newOutboxTestServer(t, db, provider)

	require.NoError(t, server.SendPasswordResetEmail([]string{"guest@example.test"}, "https://payverge.io/reset", "en"))
	require.Zero(t, provider.calls, "the outbox must absorb the send; the worker performs the provider call")

	var rows []EmailOutbox
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	row := rows[0]
	require.Equal(t, OutboxStatusPending, row.Status)
	require.Zero(t, row.AttemptCount)
	require.NotEmpty(t, row.IdempotencyKey)
	require.NotEmpty(t, row.TemplateName)
	require.Equal(t, []string{"guest@example.test"}, row.Payload.To, "the rendered message must survive a restart")
	require.NotEmpty(t, row.Payload.HTMLBody)
	require.Equal(t, row.IdempotencyKey, row.Payload.IdempotencyKey)
	require.NotContains(t, row.RecipientRedacted, "guest@example.test", "the queryable recipient column must stay redacted")
}

func TestOutboxEnqueueIsIdempotent(t *testing.T) {
	db := outboxTestDB(t)
	provider := &outboxProviderStub{}
	server, _ := newOutboxTestServer(t, db, provider)

	body := map[string]interface{}{"business_name": "Test Co", "reset_url": "https://payverge.io/reset"}
	for i := 0; i < 3; i++ {
		require.NoError(t, server.SendTransactionalEmailIdempotent(
			[]string{"guest@example.test"}, "password_reset", body, "en", "pv-fixed-key-1"))
	}

	var count int64
	require.NoError(t, db.Model(&EmailOutbox{}).Count(&count).Error)
	require.EqualValues(t, 1, count, "a repeated logical send must not queue duplicate mail")
}

func TestOutboxWorkerDeliversAndPersistsProviderMessageID(t *testing.T) {
	db := outboxTestDB(t)
	provider := &outboxProviderStub{}
	server, store := newOutboxTestServer(t, db, provider)

	require.NoError(t, server.SendPasswordResetEmail([]string{"guest@example.test"}, "https://payverge.io/reset", "en"))

	worker := NewOutboxWorker(store, server)
	processed, err := worker.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, processed)
	require.Equal(t, 1, provider.calls)

	var row EmailOutbox
	require.NoError(t, db.First(&row).Error)
	require.Equal(t, OutboxStatusSent, row.Status)
	require.Equal(t, 1, row.AttemptCount)
	require.Equal(t, "resend", row.Provider)
	require.Equal(t, "re_outbox_1", row.ProviderMessageID, "the provider message id is the bounce join key (#562)")
	require.NotNil(t, row.SentAt)

	// The redacted outbound ledger the delivery webhook reads still gets its row.
	var ledger EmailOutboundSend
	require.NoError(t, db.Where("status = ?", OutboundStatusSent).First(&ledger).Error)
	require.Equal(t, "re_outbox_1", ledger.ProviderMessageID)

	// A second pass has nothing to do.
	processed, err = worker.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Zero(t, processed)
	require.Equal(t, 1, provider.calls)
}

func TestOutboxWorkerRetryBackoffScheduleThenDeadLetters(t *testing.T) {
	db := outboxTestDB(t)
	provider := &outboxProviderStub{failTimes: 100, failErr: errors.New("resend 503 upstream")}
	server, store := newOutboxTestServer(t, db, provider)

	require.NoError(t, server.SendPasswordResetEmail([]string{"guest@example.test"}, "https://payverge.io/reset", "en"))
	var seeded EmailOutbox
	require.NoError(t, db.First(&seeded).Error)

	clock := time.Now().UTC().Add(time.Hour)
	worker := NewOutboxWorker(store, server)
	worker.Now = func() time.Time { return clock }

	// Mirrors the Telegram outbox: 1m, 5m, 15m, 1h, then a terminal dead letter
	// on the 5th attempt (MaxAttempts).
	expected := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour}
	for i, backoff := range expected {
		processed, err := worker.ProcessDue(context.Background())
		require.NoError(t, err)
		require.Equal(t, 1, processed, "attempt %d should claim the row", i+1)

		row := loadOutboxRow(t, db, seeded.ID)
		require.Equal(t, OutboxStatusPending, row.Status, "attempt %d must stay retryable", i+1)
		require.Equal(t, i+1, row.AttemptCount)
		require.Equal(t, "upstream", row.LastErrorCode)
		require.WithinDuration(t, clock.Add(backoff), row.NextAttemptAt, time.Second,
			"attempt %d must be rescheduled %s out", i+1, backoff)

		// Not yet due: the worker must not burn an attempt early.
		beforeDue, err := worker.ProcessDue(context.Background())
		require.NoError(t, err)
		require.Zero(t, beforeDue, "attempt %d must not be reclaimed before its backoff elapses", i+1)

		clock = clock.Add(backoff)
	}

	processed, err := worker.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, processed)

	row := loadOutboxRow(t, db, seeded.ID)
	require.Equal(t, OutboxStatusFailed, row.Status, "attempts must be bounded by a dead-letter state")
	require.Equal(t, 5, row.AttemptCount)
	require.Equal(t, "upstream", row.LastErrorCode)
	require.NotEmpty(t, row.LastErrorMessage)
	require.NotNil(t, row.FailedAt)
	require.Equal(t, 5, provider.calls)
	for i := range provider.keys {
		require.Equal(t, provider.keys[0], provider.keys[i], "every retry must reuse the same idempotency key")
	}

	// The dead letter is visible in the redacted outbound ledger too.
	var ledger EmailOutboundSend
	require.NoError(t, db.Where("status = ?", OutboundStatusFailed).First(&ledger).Error)
	require.Equal(t, "upstream", ledger.LastEventType)

	// Terminal rows are never re-claimed.
	clock = clock.Add(24 * time.Hour)
	processed, err = worker.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Zero(t, processed)
}

func TestOutboxWorkerDeadLettersNonRetryableFailureImmediately(t *testing.T) {
	db := outboxTestDB(t)
	provider := &outboxProviderStub{
		failTimes: 100,
		failErr:   errors.New("not authorized to send emails from noreply@payverge.io"),
	}
	server, store := newOutboxTestServer(t, db, provider)

	require.NoError(t, server.SendPasswordResetEmail([]string{"guest@example.test"}, "https://payverge.io/reset", "en"))

	worker := NewOutboxWorker(store, server)
	processed, err := worker.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, processed)

	var row EmailOutbox
	require.NoError(t, db.First(&row).Error)
	require.Equal(t, OutboxStatusFailed, row.Status)
	require.Equal(t, 1, row.AttemptCount, "a configuration failure must not consume the retry budget")
	require.Equal(t, "unauthorized_sender", row.LastErrorCode)
	require.Equal(t, 1, provider.calls)
}

func TestOutboxTerminalPayloadDropsSecrets(t *testing.T) {
	t.Run("sent", func(t *testing.T) {
		db := outboxTestDB(t)
		provider := &outboxProviderStub{}
		server, store := newOutboxTestServer(t, db, provider)
		require.NoError(t, server.SendPasswordResetEmail([]string{"guest@example.test"}, "https://payverge.io/reset", "en"))

		worker := NewOutboxWorker(store, server)
		processed, err := worker.ProcessDue(context.Background())
		require.NoError(t, err)
		require.Equal(t, 1, processed)

		var row EmailOutbox
		require.NoError(t, db.First(&row).Error)
		require.Equal(t, OutboxStatusSent, row.Status)
		assertRedactedOutboxPayload(t, db, row.ID)
	})

	t.Run("failed", func(t *testing.T) {
		db := outboxTestDB(t)
		provider := &outboxProviderStub{
			failTimes: 100,
			failErr:   errors.New("not authorized to send emails from noreply@payverge.io"),
		}
		server, store := newOutboxTestServer(t, db, provider)
		require.NoError(t, server.SendPasswordResetEmail([]string{"guest@example.test"}, "https://payverge.io/reset", "en"))

		worker := NewOutboxWorker(store, server)
		processed, err := worker.ProcessDue(context.Background())
		require.NoError(t, err)
		require.Equal(t, 1, processed)

		var row EmailOutbox
		require.NoError(t, db.First(&row).Error)
		require.Equal(t, OutboxStatusFailed, row.Status)
		assertRedactedOutboxPayload(t, db, row.ID)
	})
}

func assertRedactedOutboxPayload(t *testing.T, db *gorm.DB, id uint) {
	t.Helper()
	row := loadOutboxRow(t, db, id)
	require.Empty(t, row.Payload.To)
	require.Empty(t, row.Payload.HTMLBody)
	require.Empty(t, row.Payload.TextBody)
	require.Empty(t, row.Payload.Headers)
	require.Empty(t, row.Payload.Attachments)
	require.NotEmpty(t, row.Payload.Subject)
	require.NotEmpty(t, row.Payload.TemplateName)

	var raw string
	require.NoError(t, db.Raw("SELECT payload FROM email_outbox WHERE id = ?", id).Scan(&raw).Error)
	require.NotContains(t, raw, "guest@example.test")
	require.NotContains(t, raw, "https://payverge.io/reset")
}

func TestOutboxWorkerDropsRecipientSuppressedAfterEnqueue(t *testing.T) {
	db := outboxTestDB(t)
	provider := &outboxProviderStub{}
	server, store := newOutboxTestServer(t, db, provider)
	suppressions := NewGormSuppressionStore(db)
	server.SetSuppressionStore(suppressions)

	require.NoError(t, server.SendPasswordResetEmail([]string{"guest@example.test"}, "https://payverge.io/reset", "en"))

	// The address bounces between enqueue and delivery.
	require.NoError(t, suppressions.Suppress(context.Background(), EmailSuppression{
		Email:        "guest@example.test",
		Reason:       SuppressionReasonBounce,
		Provider:     "resend",
		SuppressedAt: time.Now().UTC(),
	}))

	worker := NewOutboxWorker(store, server)
	processed, err := worker.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, processed)
	require.Zero(t, provider.calls, "a newly suppressed recipient must never reach the provider")

	var row EmailOutbox
	require.NoError(t, db.First(&row).Error)
	require.Equal(t, OutboxStatusDropped, row.Status)
	require.Equal(t, "suppressed", row.LastErrorCode)
}

func TestOutboxStoreReclaimsStaleProcessingRows(t *testing.T) {
	db := outboxTestDB(t)
	store := NewGormOutboxStore(db)
	ctx := context.Background()
	now := time.Now().UTC()
	stale := now.Add(-10 * time.Minute)

	retryable := &EmailOutbox{IdempotencyKey: "pv-stale-retryable", TemplateName: "password_reset", NextAttemptAt: now}
	exhausted := &EmailOutbox{IdempotencyKey: "pv-stale-exhausted", TemplateName: "password_reset", NextAttemptAt: now}
	fresh := &EmailOutbox{IdempotencyKey: "pv-stale-fresh", TemplateName: "password_reset", NextAttemptAt: now}
	require.NoError(t, store.Enqueue(ctx, retryable))
	require.NoError(t, store.Enqueue(ctx, exhausted))
	require.NoError(t, store.Enqueue(ctx, fresh))

	// Simulate three workers that died mid-send with different attempt budgets.
	require.NoError(t, db.Model(&EmailOutbox{}).Where("id = ?", retryable.ID).
		Updates(map[string]interface{}{"status": OutboxStatusProcessing, "attempt_count": 2, "last_attempt_at": stale}).Error)
	require.NoError(t, db.Model(&EmailOutbox{}).Where("id = ?", exhausted.ID).
		Updates(map[string]interface{}{"status": OutboxStatusProcessing, "attempt_count": 5, "last_attempt_at": stale}).Error)
	require.NoError(t, db.Model(&EmailOutbox{}).Where("id = ?", fresh.ID).
		Updates(map[string]interface{}{"status": OutboxStatusProcessing, "attempt_count": 1, "last_attempt_at": now}).Error)

	reclaimed, err := store.ReclaimStale(ctx, 2*time.Minute, 5, now)
	require.NoError(t, err)
	require.EqualValues(t, 2, reclaimed)

	require.Equal(t, OutboxStatusPending, loadOutboxRow(t, db, retryable.ID).Status)
	require.Equal(t, "reclaimed_stale", loadOutboxRow(t, db, retryable.ID).LastErrorCode)
	require.Equal(t, OutboxStatusFailed, loadOutboxRow(t, db, exhausted.ID).Status)
	require.Equal(t, "reclaimed_exhausted", loadOutboxRow(t, db, exhausted.ID).LastErrorCode)
	require.Equal(t, OutboxStatusProcessing, loadOutboxRow(t, db, fresh.ID).Status,
		"an in-flight send must not be yanked out from under a live worker")
}

func TestOutboxJanitorExpiresPendingAndPurgesTerminal(t *testing.T) {
	db := outboxTestDB(t)
	store := NewGormOutboxStore(db)
	ctx := context.Background()
	now := time.Now().UTC()

	secretPayload := EmailMessage{
		To:       []string{"guest@example.com"},
		Subject:  "Reset your password",
		HTMLBody: `<a href="https://payverge.io/reset?token=SECRET">reset</a>`,
		TextBody: "https://payverge.io/reset?token=SECRET",
	}
	old := &EmailOutbox{IdempotencyKey: "pv-old-pending", TemplateName: "password_reset", NextAttemptAt: now, Payload: secretPayload}
	recent := &EmailOutbox{IdempotencyKey: "pv-recent-pending", TemplateName: "password_reset", NextAttemptAt: now, Payload: secretPayload}
	require.NoError(t, store.Enqueue(ctx, old))
	require.NoError(t, store.Enqueue(ctx, recent))
	require.NoError(t, db.Model(&EmailOutbox{}).Where("id = ?", old.ID).
		Update("created_at", now.Add(-48*time.Hour)).Error)

	expired, err := store.ExpirePending(ctx, now.Add(-DefaultOutboxTTL), now)
	require.NoError(t, err)
	require.EqualValues(t, 1, expired)
	require.Equal(t, OutboxStatusDropped, loadOutboxRow(t, db, old.ID).Status)
	require.Equal(t, "expired", loadOutboxRow(t, db, old.ID).LastErrorCode)
	require.Equal(t, OutboxStatusPending, loadOutboxRow(t, db, recent.ID).Status)
	// Expiry redacts the payload immediately rather than waiting for purge.
	expiredRow := loadOutboxRow(t, db, old.ID)
	require.Empty(t, expiredRow.Payload.To)
	require.Empty(t, expiredRow.Payload.HTMLBody)
	require.Empty(t, expiredRow.Payload.TextBody)
	var rawPayload string
	require.NoError(t, db.Raw("SELECT payload FROM email_outbox WHERE id = ?", old.ID).Scan(&rawPayload).Error)
	require.NotContains(t, rawPayload, "SECRET")
	require.NotContains(t, rawPayload, "guest@example.com")
	// A still-pending row keeps its payload: the worker must be able to send it.
	require.Equal(t, secretPayload.To, loadOutboxRow(t, db, recent.ID).Payload.To)

	// The payload holds plaintext PII, so terminal rows are purged on retention.
	require.NoError(t, db.Model(&EmailOutbox{}).Where("id = ?", old.ID).
		Update("updated_at", now.Add(-30*24*time.Hour)).Error)
	purged, err := store.PurgeTerminal(ctx, now.Add(-DefaultOutboxRetention))
	require.NoError(t, err)
	require.EqualValues(t, 1, purged)

	var remaining int64
	require.NoError(t, db.Model(&EmailOutbox{}).Count(&remaining).Error)
	require.EqualValues(t, 1, remaining)
}

// failingOutboxStore makes Enqueue fail so dispatch must fall back to a
// synchronous send rather than silently dropping the mail.
type failingOutboxStore struct{ OutboxStore }

func (failingOutboxStore) Enqueue(context.Context, *EmailOutbox) error {
	return errors.New("outbox table unavailable")
}

func TestDispatchFallsBackToSynchronousSendWhenEnqueueFails(t *testing.T) {
	db := outboxTestDB(t)
	provider := &outboxProviderStub{}
	server, err := NewEmailServer(provider, "noreply@payverge.io", "updates@payverge.io", resolveTemplatesRoot(t))
	require.NoError(t, err)
	server.SetOutboundStore(NewGormOutboundStore(db))
	server.SetOutboxStore(failingOutboxStore{OutboxStore: NewGormOutboxStore(db)})

	require.NoError(t, server.SendPasswordResetEmail([]string{"guest@example.test"}, "https://payverge.io/reset", "en"))
	require.Equal(t, 1, provider.calls, "a queue write failure must not lose the message")

	var queued int64
	require.NoError(t, db.Model(&EmailOutbox{}).Count(&queued).Error)
	require.Zero(t, queued)
}

func TestDispatchNeverQueuesASuppressedRecipient(t *testing.T) {
	db := outboxTestDB(t)
	provider := &outboxProviderStub{}
	server, _ := newOutboxTestServer(t, db, provider)
	suppressions := NewGormSuppressionStore(db)
	server.SetSuppressionStore(suppressions)
	require.NoError(t, suppressions.Suppress(context.Background(), EmailSuppression{
		Email:        "blocked@example.test",
		Reason:       SuppressionReasonComplaint,
		Provider:     "resend",
		SuppressedAt: time.Now().UTC(),
	}))

	err := server.SendPasswordResetEmail([]string{"BLOCKED@example.test"}, "https://payverge.io/reset", "en")
	require.ErrorIs(t, err, ErrRecipientSuppressed)

	var queued int64
	require.NoError(t, db.Model(&EmailOutbox{}).Count(&queued).Error)
	require.Zero(t, queued, "suppressed mail must not even be queued")
	require.Zero(t, provider.calls)
}
