package emails

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type suppressionProviderStub struct {
	calls int
	err   error
}

func (s *suppressionProviderStub) Send(context.Context, EmailMessage) error {
	s.calls++
	return s.err
}

type failingSuppressionStore struct{ err error }

func (s failingSuppressionStore) Lookup(context.Context, string) (*EmailSuppression, error) {
	return nil, s.err
}

func suppressionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Migrator().CreateTable(&EmailSuppression{}, &EmailDeliveryEvent{}, &EmailOutboundSend{}, &EmailOutbox{}))
	return db
}

func TestEmailServerBlocksDurablySuppressedRecipient(t *testing.T) {
	db := suppressionTestDB(t)
	store := NewGormSuppressionStore(db)
	require.NoError(t, store.Suppress(context.Background(), EmailSuppression{
		Email:        "blocked@example.test",
		Reason:       SuppressionReasonComplaint,
		Provider:     "resend",
		SuppressedAt: time.Now().UTC(),
	}))

	provider := &suppressionProviderStub{}
	server, err := NewEmailServer(provider, "noreply@payverge.io", "updates@payverge.io", resolveTemplatesRoot(t))
	require.NoError(t, err)
	server.SetSuppressionStore(store)

	err = server.SendPasswordResetEmail([]string{"BLOCKED@example.test"}, "https://payverge.io/reset", "en")
	require.ErrorIs(t, err, ErrRecipientSuppressed)
	require.Zero(t, provider.calls, "suppressed mail must never reach the provider")
}

func TestEmailServerFailsClosedWhenSuppressionLookupFails(t *testing.T) {
	provider := &suppressionProviderStub{}
	server, err := NewEmailServer(provider, "noreply@payverge.io", "updates@payverge.io", resolveTemplatesRoot(t))
	require.NoError(t, err)
	server.SetSuppressionStore(failingSuppressionStore{err: errors.New("database unavailable")})

	err = server.SendEmailVerificationEmail([]string{"owner@example.test"}, "Owner", "https://payverge.io/verify", "en")
	require.Error(t, err)
	require.ErrorContains(t, err, "suppression lookup")
	require.Zero(t, provider.calls)
}

type deadlineProvider struct{ sawDeadline bool }

func (p *deadlineProvider) Send(ctx context.Context, _ EmailMessage) error {
	_, p.sawDeadline = ctx.Deadline()
	<-ctx.Done()
	return ctx.Err()
}

func TestEmailServerProviderTimeoutFailsClosed(t *testing.T) {
	provider := &deadlineProvider{}
	server, err := NewEmailServer(provider, "noreply@payverge.io", "updates@payverge.io", resolveTemplatesRoot(t))
	require.NoError(t, err)
	server.sendTimeout = 10 * time.Millisecond

	err = server.SendPasswordResetEmail([]string{"owner@example.test"}, "https://payverge.io/reset", "en")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.True(t, provider.sawDeadline)
}

func TestNewEmailServerRejectsInvalidSender(t *testing.T) {
	_, err := NewEmailServer(&suppressionProviderStub{}, "not-an-email", "updates@payverge.io", resolveTemplatesRoot(t))
	require.ErrorContains(t, err, "transactional sender")
}
