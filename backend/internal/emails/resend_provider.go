package emails

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/resend/resend-go/v3"
)

// ResendProvider implements EmailProvider against the Resend API.
type ResendProvider struct {
	client *resend.Client
}

// NewResendProvider builds a Resend-backed provider. It bounds the send with a
// 30s timeout so a hung endpoint can't block the (often synchronous) email-send
// goroutine indefinitely — matching PostmarkProvider's behavior.
func NewResendProvider(apiKey string) *ResendProvider {
	httpClient := &http.Client{Timeout: 30 * time.Second}
	return &ResendProvider{client: resend.NewCustomClient(httpClient, apiKey)}
}

func (p *ResendProvider) Send(ctx context.Context, msg EmailMessage) error {
	_, err := p.SendWithReceipt(ctx, msg)
	return err
}

func (p *ResendProvider) SendWithReceipt(ctx context.Context, msg EmailMessage) (SendResult, error) {
	req := &resend.SendEmailRequest{
		From:    msg.From,
		To:      msg.To,
		Subject: msg.Subject,
		Html:    msg.HTMLBody,
		Text:    msg.TextBody,
		ReplyTo: msg.ReplyTo,
		Tags:    resendTagsFor(msg.Tag, msg.MessageType),
	}
	for i := range msg.Attachments {
		a := msg.Attachments[i]
		req.Attachments = append(req.Attachments, &resend.Attachment{
			Filename:    a.Filename,
			Content:     a.Content,
			ContentType: a.ContentType,
		})
	}
	if len(msg.Headers) > 0 {
		req.Headers = msg.Headers
	}

	var (
		resp *resend.SendEmailResponse
		err  error
	)
	if strings.TrimSpace(msg.IdempotencyKey) != "" {
		resp, err = p.client.Emails.SendWithOptions(ctx, req, &resend.SendEmailOptions{IdempotencyKey: msg.IdempotencyKey})
	} else {
		resp, err = p.client.Emails.SendWithContext(ctx, req)
	}
	if err != nil {
		return SendResult{}, err
	}
	id := ""
	if resp != nil {
		id = strings.TrimSpace(resp.Id)
	}
	return SendResult{ProviderMessageID: id}, nil
}

// resendTagsFor encodes the provider-agnostic MessageType and Tag as Resend
// tags. Resend has no MessageStream concept, so the transactional/broadcast
// distinction is preserved as a "message_type" tag. The free-form Tag is only
// attached when present, since Resend rejects empty tag values.
func resendTagsFor(tag string, t MessageType) []resend.Tag {
	tags := []resend.Tag{
		{Name: "message_type", Value: resendMessageType(t)},
	}
	if v := sanitizeResendTagValue(tag); v != "" {
		tags = append(tags, resend.Tag{Name: "tag", Value: v})
	}
	return tags
}

func resendMessageType(t MessageType) string {
	switch t {
	case MessageTypeBroadcast:
		return "broadcast"
	case MessageTypeTransactional:
		return "transactional"
	default:
		return "transactional"
	}
}

// sanitizeResendTagValue coerces a value into Resend's allowed tag charset:
// ASCII letters, digits, underscores, and dashes. Anything else becomes an
// underscore so a stray character can't get the whole send rejected.
func sanitizeResendTagValue(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
