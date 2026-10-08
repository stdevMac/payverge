package emails

import (
	"context"
	"errors"
	"fmt"
	"net/textproto"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type observedProviderStub struct{ err error }

func (s observedProviderStub) Send(context.Context, EmailMessage) error { return s.err }

func TestClassifyEmailFailure(t *testing.T) {
	cases := map[string]string{
		"This API key is not authorized to send emails from payverge.io": "unauthorized_sender",
		"invalid api key":           "authentication",
		"429 too many requests":     "rate_limited",
		"context deadline exceeded": "timeout",
		"provider unavailable":      "upstream",
	}
	for message, want := range cases {
		assert.Equal(t, want, classifyEmailFailure(errors.New(message)), message)
	}
}

// TestClassifySMTPFailure: SMTP status codes decide retryability for the smtp
// provider, which has no bounce webhook. 5xx must not be retried.
func TestClassifySMTPFailure(t *testing.T) {
	cases := []struct {
		code      int
		want      string
		retryable bool
	}{
		{code: 421, want: "upstream", retryable: true},
		{code: 451, want: "upstream", retryable: true},
		{code: 452, want: "upstream", retryable: true},
		{code: 535, want: "authentication", retryable: false},
		{code: 530, want: "authentication", retryable: false},
		{code: 550, want: "rejected", retryable: false},
		{code: 554, want: "rejected", retryable: false},
	}
	for _, tc := range cases {
		err := fmt.Errorf("smtp: RCPT TO: %w", &textproto.Error{Code: tc.code, Msg: "server says no"})
		assert.Equal(t, tc.want, classifyEmailFailure(err), "code %d", tc.code)
		assert.Equal(t, tc.retryable, isRetryableEmailFailure(err), "code %d", tc.code)
	}
}

func TestObservedProviderRecordsFailureWithoutChangingError(t *testing.T) {
	wantErr := errors.New("not authorized to send emails from payverge.io")
	labels := []string{"resend", "transactional", "failed", "unauthorized_sender"}
	before := testutil.ToFloat64(metrics.EmailSendAttempts.WithLabelValues(labels...))
	provider := NewObservedProvider("resend", observedProviderStub{err: wantErr})
	err := provider.Send(context.Background(), EmailMessage{
		From:        "noreply@payverge.io",
		To:          []string{"private@example.com"},
		MessageType: MessageTypeTransactional,
	})
	require.ErrorIs(t, err, wantErr)
	assert.Equal(t, before+1, testutil.ToFloat64(metrics.EmailSendAttempts.WithLabelValues(labels...)))
}
