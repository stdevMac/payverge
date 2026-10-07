package emails

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type retryProviderStub struct {
	failTimes int
	failErr   error
	calls     int
	keys      []string
}

func (p *retryProviderStub) Send(ctx context.Context, msg EmailMessage) error {
	_, err := p.SendWithReceipt(ctx, msg)
	return err
}

func (p *retryProviderStub) SendWithReceipt(_ context.Context, msg EmailMessage) (SendResult, error) {
	p.calls++
	p.keys = append(p.keys, msg.IdempotencyKey)
	if p.calls <= p.failTimes {
		return SendResult{}, p.failErr
	}
	return SendResult{ProviderMessageID: fmt.Sprintf("re_ok_%d", p.calls)}, nil
}

func (p *retryProviderStub) ProviderName() string { return "resend" }

func TestDispatchRetriesRetryableFailureThenDelivers(t *testing.T) {
	db := suppressionTestDB(t)
	provider := &retryProviderStub{
		failTimes: 1,
		failErr:   errors.New("429 too many requests"),
	}
	server, err := NewEmailServer(provider, "noreply@payverge.io", "updates@payverge.io", resolveTemplatesRoot(t))
	require.NoError(t, err)
	server.SetOutboundStore(NewGormOutboundStore(db))
	server.retryBackoff = 0

	require.NoError(t, server.SendPasswordResetEmail([]string{"guest@example.test"}, "https://payverge.io/reset", "en"))
	require.Equal(t, 2, provider.calls)
	require.Len(t, provider.keys, 2)
	require.NotEmpty(t, provider.keys[0])
	require.Equal(t, provider.keys[0], provider.keys[1], "retries must reuse the same idempotency key")

	var rec EmailOutboundSend
	require.NoError(t, db.Where("status = ?", OutboundStatusSent).First(&rec).Error)
	require.Equal(t, "re_ok_2", rec.ProviderMessageID)
}

func TestDispatchDoesNotRetryUnauthorizedSender(t *testing.T) {
	db := suppressionTestDB(t)
	provider := &retryProviderStub{
		failTimes: 5,
		failErr:   errors.New("not authorized to send emails from noreply@payverge.io"),
	}
	server, err := NewEmailServer(provider, "noreply@payverge.io", "updates@payverge.io", resolveTemplatesRoot(t))
	require.NoError(t, err)
	server.SetOutboundStore(NewGormOutboundStore(db))
	server.retryBackoff = 0

	err = server.SendPasswordResetEmail([]string{"guest@example.test"}, "https://payverge.io/reset", "en")
	require.Error(t, err)
	require.Equal(t, 1, provider.calls)

	var rec EmailOutboundSend
	require.NoError(t, db.Where("status = ?", OutboundStatusFailed).First(&rec).Error)
	require.Equal(t, "unauthorized_sender", rec.LastEventType)
	require.True(t, stringsHasPrefix(rec.ProviderMessageID, "local-failed-"))
}

func TestDispatchExhaustsRetryableFailures(t *testing.T) {
	db := suppressionTestDB(t)
	provider := &retryProviderStub{
		failTimes: 10,
		failErr:   errors.New("resend 503 upstream"),
	}
	server, err := NewEmailServer(provider, "noreply@payverge.io", "updates@payverge.io", resolveTemplatesRoot(t))
	require.NoError(t, err)
	server.SetOutboundStore(NewGormOutboundStore(db))
	server.retryBackoff = 0

	err = server.SendPasswordResetEmail([]string{"guest@example.test"}, "https://payverge.io/reset", "en")
	require.Error(t, err)
	require.Equal(t, defaultEmailSendAttempts, provider.calls)
	require.Len(t, provider.keys, defaultEmailSendAttempts)
	for i := 1; i < len(provider.keys); i++ {
		require.Equal(t, provider.keys[0], provider.keys[i])
	}

	var rec EmailOutboundSend
	require.NoError(t, db.Where("status = ?", OutboundStatusFailed).First(&rec).Error)
	require.Equal(t, "upstream", rec.LastEventType)
}

func stringsHasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
