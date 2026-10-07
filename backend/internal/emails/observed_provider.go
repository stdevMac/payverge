package emails

import (
	"context"
	"errors"
	"net/textproto"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stdevmac/payverge/backend/internal/metrics"
)

type ObservedProvider struct {
	name  string
	inner EmailProvider
}

func NewObservedProvider(name string, inner EmailProvider) *ObservedProvider {
	return &ObservedProvider{name: strings.ToLower(strings.TrimSpace(name)), inner: inner}
}

func (p *ObservedProvider) ProviderName() string {
	if p == nil {
		return ""
	}
	return p.name
}

func classifyEmailFailure(err error) string {
	if err == nil {
		return "none"
	}
	// SMTP replies carry a status code: 4xx is transient (retry), 5xx is a
	// permanent refusal that retrying cannot fix. There is no bounce webhook
	// for SMTP, so this synchronous reply is the only delivery signal the
	// smtp provider gets.
	// The message went out but its reply was lost: retrying risks a
	// duplicate, so this is its own non-retryable reason.
	if errors.Is(err, ErrSMTPOutcomeUnknown) {
		return "outcome_unknown"
	}
	var smtpErr *textproto.Error
	if errors.As(err, &smtpErr) {
		switch {
		case smtpErr.Code == 530 || smtpErr.Code == 534 || smtpErr.Code == 535 || smtpErr.Code == 538:
			return "authentication"
		case smtpErr.Code >= 400 && smtpErr.Code < 500:
			return "upstream"
		case smtpErr.Code >= 500:
			return "rejected"
		}
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "not authorized to send emails from"):
		return "unauthorized_sender"
	case strings.Contains(message, "api key"), strings.Contains(message, "unauthorized"), strings.Contains(message, "forbidden"):
		return "authentication"
	case strings.Contains(message, "429"), strings.Contains(message, "rate limit"), strings.Contains(message, "too many requests"):
		return "rate_limited"
	case errors.Is(err, context.DeadlineExceeded), strings.Contains(message, "deadline exceeded"), strings.Contains(message, "timeout"):
		return "timeout"
	default:
		return "upstream"
	}
}

func normalizedMessageType(value MessageType) string {
	if value == MessageTypeBroadcast {
		return string(MessageTypeBroadcast)
	}
	return string(MessageTypeTransactional)
}

func (p *ObservedProvider) Send(ctx context.Context, msg EmailMessage) error {
	_, err := p.SendWithReceipt(ctx, msg)
	return err
}

func (p *ObservedProvider) SendWithReceipt(ctx context.Context, msg EmailMessage) (SendResult, error) {
	started := time.Now()
	var result SendResult
	var err error
	if rp, ok := p.inner.(EmailReceiptProvider); ok {
		result, err = rp.SendWithReceipt(ctx, msg)
	} else {
		err = p.inner.Send(ctx, msg)
	}
	outcome, reason := "success", "none"
	if err != nil {
		outcome, reason = "failed", classifyEmailFailure(err)
	}
	messageType := normalizedMessageType(msg.MessageType)
	metrics.EmailSendAttempts.WithLabelValues(p.name, messageType, outcome, reason).Inc()
	metrics.EmailSendDuration.WithLabelValues(p.name, outcome).Observe(time.Since(started).Seconds())
	fields := logrus.Fields{
		"provider":        p.name,
		"message_type":    messageType,
		"outcome":         outcome,
		"reason":          reason,
		"recipient_count": len(msg.To),
	}
	if mid := strings.TrimSpace(result.ProviderMessageID); mid != "" {
		fields["provider_message_id"] = mid
	}
	if err != nil {
		logrus.WithFields(fields).WithError(err).Error("email delivery failed")
	} else {
		logrus.WithFields(fields).Info("email delivered")
	}
	return result, err
}
