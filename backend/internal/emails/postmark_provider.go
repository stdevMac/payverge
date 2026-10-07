package emails

import (
	"context"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/mattevans/postmark-go"
)

// PostmarkProvider implements EmailProvider against the Postmark API.
type PostmarkProvider struct {
	client *postmark.Client
}

func NewPostmarkProvider(token string) *PostmarkProvider {
	client := postmark.NewClient(
		postmark.WithClient(&http.Client{
			Transport: &postmark.AuthTransport{Token: token},
			// Bound the send so a hung Postmark endpoint can't block the
			// (often synchronous) email-send goroutine indefinitely.
			Timeout: 30 * time.Second,
		}),
	)
	return &PostmarkProvider{client: client}
}

func (p *PostmarkProvider) Send(ctx context.Context, msg EmailMessage) error {
	_, err := p.SendWithReceipt(ctx, msg)
	return err
}

// postmarkRecipients joins validated bare addresses. A single slice entry must
// not be a comma/semicolon list or contain CR/LF: Postmark treats To as a
// header, so either one fans the message out or injects extra headers.
func postmarkRecipients(to []string) (string, error) {
	if len(to) == 0 {
		return "", fmt.Errorf("postmark: invalid recipient: empty recipient list")
	}
	addrs := make([]string, 0, len(to))
	for _, raw := range to {
		if strings.ContainsAny(raw, ",;\r\n") {
			return "", fmt.Errorf("postmark: invalid recipient: entry contains a list separator or line break")
		}
		parsed, err := mail.ParseAddress(raw)
		if err != nil {
			return "", fmt.Errorf("postmark: invalid recipient: %w", err)
		}
		if parsed.Address == "" {
			return "", fmt.Errorf("postmark: invalid recipient: empty address")
		}
		addrs = append(addrs, parsed.Address)
	}
	return strings.Join(addrs, ","), nil
}

func (p *PostmarkProvider) SendWithReceipt(_ context.Context, msg EmailMessage) (SendResult, error) {
	to, err := postmarkRecipients(msg.To)
	if err != nil {
		return SendResult{}, err
	}
	replyTo := ""
	if msg.ReplyTo != "" {
		replyTo, err = postmarkRecipients([]string{msg.ReplyTo})
		if err != nil {
			return SendResult{}, err
		}
	}
	pmEmail := &postmark.Email{
		From:          msg.From,
		To:            to,
		Subject:       msg.Subject,
		HTMLBody:      msg.HTMLBody,
		TextBody:      msg.TextBody,
		Tag:           msg.Tag,
		ReplyTo:       replyTo,
		MessageStream: postmarkStreamFor(msg.MessageType),
	}
	for i := range msg.Attachments {
		a := msg.Attachments[i]
		ct := a.ContentType
		pmEmail.Attachments = append(pmEmail.Attachments, postmark.EmailAttachment{
			Name:        a.Filename,
			Content:     a.Content,
			ContentType: &ct,
		})
	}
	// P2-10: List-Unsubscribe headers on marketing-toned sends (rollback path).
	for name, value := range msg.Headers {
		pmEmail.Headers = append(pmEmail.Headers, postmark.EmailHeader{
			Name:  name,
			Value: value,
		})
	}
	// Postmark has no native request-idempotency option. Preserve the logical
	// key as a trace header for incident correlation; production scheduled
	// reports use Resend's native Idempotency-Key enforcement.
	if strings.TrimSpace(msg.IdempotencyKey) != "" {
		pmEmail.Headers = append(pmEmail.Headers, postmark.EmailHeader{
			Name:  "X-Payverge-Idempotency-Key",
			Value: msg.IdempotencyKey,
		})
	}

	response, resp, err := p.client.Email.Send(pmEmail)
	if err != nil {
		return SendResult{}, err
	}
	// Defense in depth: postmark-go's Do() already runs CheckResponse() and
	// turns any non-2xx status into a non-nil err above. This branch only fires
	// if the SDK ever changes to return (resp, nil) on non-2xx.
	if resp.StatusCode != http.StatusOK {
		return SendResult{}, fmt.Errorf("postmark send failed: status=%s", resp.Status)
	}
	id := ""
	if response != nil {
		id = strings.TrimSpace(response.MessageID)
	}
	return SendResult{ProviderMessageID: id}, nil
}

func postmarkStreamFor(t MessageType) string {
	switch t {
	case MessageTypeBroadcast:
		return "broadcast"
	case MessageTypeTransactional:
		return "outbound"
	default:
		return "outbound"
	}
}

// NewProvider is the factory. Unknown names return a loud error so misconfig
// fails at startup instead of silently swallowing emails.
//
//   - "log":      never sends; logs metadata, or message content when EMAIL_LOG_CONTENT=true.
//   - "smtp":     net/smtp relay configured from SMTP_* env (apiKey unused).
//   - "resend":   Resend HTTP API; requires apiKey.
//   - "postmark": Postmark HTTP API; requires apiKey.
//   - "":         legacy blank name — resend when apiKey is set, else log.
//
// A Resend/Postmark provider is never built with an empty key: that used to
// make a real, unauthenticated API call on every send from a fresh box.
// Callers resolve the name with config.EmailProvider().
func NewProvider(name, apiKey string) (EmailProvider, error) {
	apiKey = strings.TrimSpace(apiKey)
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "":
		if apiKey == "" {
			return NewLogProviderWithContent(nil, logContentEnabled()), nil
		}
		return NewResendProvider(apiKey), nil
	case "resend":
		if apiKey == "" {
			return nil, fmt.Errorf("email provider resend requires an API key (EMAIL_API_KEY or RESEND_API_KEY); set EMAIL_PROVIDER=log or smtp to run without Resend")
		}
		return NewResendProvider(apiKey), nil
	case "postmark":
		if apiKey == "" {
			return nil, fmt.Errorf("email provider postmark requires an API key (EMAIL_API_KEY)")
		}
		return NewPostmarkProvider(apiKey), nil
	case "smtp":
		cfg, err := SMTPConfigFromEnv()
		if err != nil {
			return nil, err
		}
		provider, err := NewSMTPProvider(cfg)
		if err != nil {
			return nil, err
		}
		return provider, nil
	case "log":
		return NewLogProviderWithContent(nil, logContentEnabled()), nil
	default:
		return nil, fmt.Errorf("unknown email provider %q (supported: log, smtp, resend, postmark)", name)
	}
}
