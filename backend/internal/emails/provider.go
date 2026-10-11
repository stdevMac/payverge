package emails

import (
	"context"
	"strings"
)

// MessageType is a provider-agnostic classification of an outgoing message.
// Providers map this to their native transactional stream/tag metadata.
type MessageType string

const (
	MessageTypeTransactional MessageType = "transactional"
	MessageTypeBroadcast     MessageType = "broadcast"
)

// EmailAttachment is a provider-agnostic file attachment. Content is the raw
// bytes (providers base64-encode as needed); ContentType is the MIME type.
type EmailAttachment struct {
	Filename    string
	Content     []byte
	ContentType string
}

// EmailMessage is the provider-agnostic representation of an outgoing email.
// Templates are rendered upstream in EmailServer; providers handle only
// wire format and transport.
type EmailMessage struct {
	From        string
	To          []string
	Subject     string
	HTMLBody    string
	TextBody    string
	ReplyTo     string
	Tag         string
	MessageType MessageType
	Metadata    map[string]string
	Attachments []EmailAttachment
	// IdempotencyKey is a stable logical-send key. Providers with native HTTP
	// idempotency (Resend) must use it for request deduplication; rollback
	// providers may preserve it as a trace header.
	IdempotencyKey string
	// Headers are provider-agnostic extra SMTP headers (e.g. List-Unsubscribe
	// / List-Unsubscribe-Post on marketing-toned sends — P2-10). Providers
	// that cannot set custom headers may ignore them.
	Headers map[string]string
	// TemplateName is the logical template used to render this message
	// (e.g. reservation_confirmation). It is not sent to the provider; it
	// is recorded on the outbound send so a later bounce can be attributed.
	TemplateName string
}

// SendResult is the provider acknowledgement for a successful send.
type SendResult struct {
	ProviderMessageID string
}

// EmailProvider is the transport-layer interface.
type EmailProvider interface {
	Send(ctx context.Context, msg EmailMessage) error
}

// EmailReceiptProvider is implemented by transports that can return a
// provider message ID. EmailServer uses it when available so bounce
// webhooks can join to the originating send.
type EmailReceiptProvider interface {
	SendWithReceipt(ctx context.Context, msg EmailMessage) (SendResult, error)
}

// namedEmailProvider is implemented by ObservedProvider so outbound
// attribution records the configured transport name (resend/postmark).
type namedEmailProvider interface {
	ProviderName() string
}

func providerNameOf(p EmailProvider) string {
	if named, ok := p.(namedEmailProvider); ok {
		if name := strings.TrimSpace(named.ProviderName()); name != "" {
			return strings.ToLower(name)
		}
	}
	return "unknown"
}
