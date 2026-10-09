package emails

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/logger"

	"github.com/sirupsen/logrus"
)

const (
	// logProviderBodyPreviewRunes caps the plain-text body written per email.
	logProviderBodyPreviewRunes = 1000
	// logProviderMaxLinks caps how many links are extracted per email.
	logProviderMaxLinks = 20
)

// LogProvider is the EMAIL_PROVIDER=log transport: it never sends. Each
// message becomes one structured Info log entry.
//
// By default the entry is metadata only: provider, provider_message_id,
// template (when set), recipient_count, a short recipient_hash, and
// attachment_count when the message has attachments. Recipients, sender,
// reply-to, subject, body and links are not written.
//
// Content mode (NewLogProviderWithContent with includeContent, selected when
// EMAIL_LOG_CONTENT parses as true) adds the previous fields: to, from,
// subject, reply_to, a truncated plain-text preview and every http(s) link.
// That is for local evaluation. Production preflight refuses it unless
// EMAIL_PROVIDER_LOG_ALLOW_PRODUCTION=true.
//
// It is the default on a box with no email configuration, so a fresh
// self-host never calls a third-party API. Operators who need delivery
// should configure smtp, resend or postmark.
type LogProvider struct {
	// log overrides the destination (tests). nil means logger.Logger,
	// resolved at send time because logger.InitLogger replaces it.
	log            logrus.FieldLogger
	includeContent bool
}

// NewLogProvider builds the log-only transport in metadata-only mode.
// Pass nil to write to the process logger.
func NewLogProvider(l logrus.FieldLogger) *LogProvider {
	return NewLogProviderWithContent(l, false)
}

// NewLogProviderWithContent builds the log-only transport. includeContent
// also logs recipients, subject, body preview and links.
func NewLogProviderWithContent(l logrus.FieldLogger, includeContent bool) *LogProvider {
	return &LogProvider{log: l, includeContent: includeContent}
}

// logContentEnabled reports EMAIL_LOG_CONTENT. Empty and invalid values are false.
func logContentEnabled() bool {
	v, err := strconv.ParseBool(strings.TrimSpace(os.Getenv("EMAIL_LOG_CONTENT")))
	return err == nil && v
}

func (p *LogProvider) Send(ctx context.Context, msg EmailMessage) error {
	_, err := p.SendWithReceipt(ctx, msg)
	return err
}

// SendWithReceipt logs the message and returns a synthetic "log-<hex>" ID so
// the outbound/outbox bookkeeping behaves exactly as with a real transport.
func (p *LogProvider) SendWithReceipt(ctx context.Context, msg EmailMessage) (SendResult, error) {
	if err := ctx.Err(); err != nil {
		return SendResult{}, err
	}
	id := "log-" + randomHex(8)

	fields := logrus.Fields{
		"provider":            "log",
		"provider_message_id": id,
		"recipient_count":     len(msg.To),
		"recipient_hash":      recipientHash(msg.To),
	}
	if msg.TemplateName != "" {
		fields["template"] = msg.TemplateName
	}
	if n := len(msg.Attachments); n > 0 {
		fields["attachment_count"] = n
	}
	if p.includeContent {
		fields["to"] = strings.Join(msg.To, ", ")
		fields["from"] = msg.From
		fields["subject"] = msg.Subject
		fields["body_preview"] = truncateRunes(plainTextBody(msg), logProviderBodyPreviewRunes)
		if msg.ReplyTo != "" {
			fields["reply_to"] = msg.ReplyTo
		}
		if links := extractLinks(msg, logProviderMaxLinks); len(links) > 0 {
			fields["links"] = strings.Join(links, " ")
		}
	}

	dest := p.log
	if dest == nil {
		dest = logger.Logger
	}
	dest.WithFields(fields).Info("email not sent (EMAIL_PROVIDER=log): metadata logged instead of delivered")
	return SendResult{ProviderMessageID: id}, nil
}

// recipientHash is the first 16 hex chars of SHA-256 over the lowercased,
// trimmed, sorted recipients joined by ",".
func recipientHash(to []string) string {
	norm := make([]string, len(to))
	for i, r := range to {
		norm[i] = strings.ToLower(strings.TrimSpace(r))
	}
	sort.Strings(norm)
	sum := sha256.Sum256([]byte(strings.Join(norm, ",")))
	return hex.EncodeToString(sum[:])[:16]
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(b)
}
