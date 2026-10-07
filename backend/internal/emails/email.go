package emails

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/metrics"
)

var EmailServerInstance *EmailServer

// AdminsEmails is the operator inbox for internal alerts (new signups, stuck
// checkouts, contact/intake forms, escalations): the comma-separated
// ADMIN_EMAILS, else SUPPORT_EMAIL, else the bootstrap ADMIN_EMAIL, else
// empty. There is no upstream default inbox, so a self-hosted instance never
// mails its operators' leads to the original project. See admin_recipients.go.
var AdminsEmails = adminEmailsFromEnv()

func init() {
	initAdminEmails()
}

type EmailServer struct {
	FromTransactional string
	FromUpdates       string
	provider          EmailProvider
	templates         *TemplateManager
	suppressionStore  SuppressionStore
	outboundStore     OutboundStore
	outboxStore       OutboxStore
	sendTimeout       time.Duration
	maxSendAttempts   int
	retryBackoff      time.Duration
	// origin is the entity this server instance sends on behalf of. It is set
	// per call site by WithOrigin (which returns a copy), never mutated on the
	// shared singleton, and is stamped onto queued rows so a later bounce can
	// be attributed back to the reservation/bill/order (#562).
	origin EmailOrigin
	// tenantBudget bounds tenant-triggered (origin-stamped) mail; see
	// tenant_budget.go. Nil disables it.
	tenantBudget TenantMailBudget
}

const (
	defaultEmailSendTimeout  = 30 * time.Second
	defaultEmailSendAttempts = 3
	defaultEmailRetryBackoff = 200 * time.Millisecond
)

func NewEmailServer(provider EmailProvider, fromTransactional, fromUpdates, templatesDir string) (*EmailServer, error) {
	if provider == nil {
		return nil, fmt.Errorf("email provider is required")
	}
	if _, err := mail.ParseAddress(strings.TrimSpace(fromTransactional)); err != nil {
		return nil, fmt.Errorf("invalid transactional sender: %w", err)
	}
	if _, err := mail.ParseAddress(strings.TrimSpace(fromUpdates)); err != nil {
		return nil, fmt.Errorf("invalid updates sender: %w", err)
	}
	tm, err := NewTemplateManager(templatesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize template manager: %w", err)
	}
	EmailServerInstance = &EmailServer{
		FromTransactional: fromTransactional,
		FromUpdates:       fromUpdates,
		provider:          provider,
		templates:         tm,
		sendTimeout:       defaultEmailSendTimeout,
		maxSendAttempts:   defaultEmailSendAttempts,
		retryBackoff:      defaultEmailRetryBackoff,
	}
	return EmailServerInstance, nil
}

// SetSuppressionStore enables application-owned suppression checks. It is
// called after versioned migrations complete and before HTTP serving starts.
func (e *EmailServer) SetSuppressionStore(store SuppressionStore) {
	e.suppressionStore = store
}

// SetOutboundStore enables persistence of provider message IDs after a
// successful send so delivery webhooks can attribute bounces.
func (e *EmailServer) SetOutboundStore(store OutboundStore) {
	e.outboundStore = store
}

// SetOutboxStore switches transactional sends onto the durable outbox (#551):
// dispatch persists the rendered message and returns, and OutboxWorker performs
// the provider call with retries, backoff and a terminal dead-letter state.
// Without it, dispatch keeps the legacy in-process retry behaviour.
func (e *EmailServer) SetOutboxStore(store OutboxStore) {
	e.outboxStore = store
}

func (e *EmailServer) dispatch(msg EmailMessage) error {
	timeout := e.sendTimeout
	if timeout <= 0 {
		timeout = defaultEmailSendTimeout
	}
	if !hasRecipient(msg.To) {
		return ErrNoRecipients
	}
	// Special-use domains (demo+...@payverge.local) have no mail exchanger:
	// drop them before the provider ever sees them. A message left with no
	// real recipient is a successful no-op, not a failure, so demo seeding
	// and local fixtures stay quiet.
	if kept, dropped := withoutReservedRecipients(msg.To); dropped > 0 {
		if !hasRecipient(kept) {
			logrus.WithField("template_name", msg.TemplateName).
				Debug("email skipped: every recipient is on a special-use domain")
			return nil
		}
		msg.To = kept
	}
	// Strip CR/LF before the outbox payload is stored. SMTP already did this
	// at the wire; Resend and Postmark received the raw subject.
	msg.Subject = sanitizeHeaderValue(msg.Subject)
	ensureEmailIdempotencyKey(&msg)

	// Fail fast on a suppressed recipient so mail we must not send is never
	// queued. The worker re-checks at delivery time (an address can bounce
	// between enqueue and send).
	lookupCtx, lookupCancel := context.WithTimeout(context.Background(), timeout)
	defer lookupCancel()
	if err := e.checkSuppressed(lookupCtx, msg); err != nil {
		return err
	}
	// The single tenant outbound budget choke point (tenant_budget.go).
	claim, err := e.claimTenantMail(lookupCtx, msg)
	if err != nil {
		return err
	}
	if claim.Duplicate {
		if e.origin.ReportDuplicate {
			return ErrTenantMailDuplicate
		}
		return nil
	}

	if e.outboxStore != nil {
		if err := e.enqueueOutbox(context.Background(), msg); err == nil || errors.Is(err, ErrOutboxDuplicate) {
			if err != nil {
				e.releaseTenantMail(claim)
			}
			return nil
		} else {
			// Never lose the message because the queue write failed: fall back
			// to the in-process send. The idempotency key is already stamped,
			// so a partially-written row cannot cause a double send.
			logrus.WithFields(logrus.Fields{
				"template_name": msg.TemplateName,
			}).WithError(err).Error("email outbox enqueue failed; falling back to a synchronous send")
		}
	}

	if err := e.sendWithRetry(msg, timeout); err != nil {
		e.releaseTenantMail(claim)
		return err
	}
	return nil
}

// checkSuppressed returns ErrRecipientSuppressed when any recipient is on the
// application-owned suppression list.
func (e *EmailServer) checkSuppressed(ctx context.Context, msg EmailMessage) error {
	if e.suppressionStore == nil {
		return nil
	}
	for _, recipient := range msg.To {
		suppression, err := e.suppressionStore.Lookup(ctx, recipient)
		if err != nil {
			return fmt.Errorf("email suppression lookup failed: %w", err)
		}
		if suppression != nil {
			return ErrRecipientSuppressed
		}
	}
	return nil
}

// enqueueOutbox persists the rendered message for the outbox worker.
func (e *EmailServer) enqueueOutbox(ctx context.Context, msg EmailMessage) error {
	row := &EmailOutbox{
		IdempotencyKey:    strings.TrimSpace(msg.IdempotencyKey),
		Status:            OutboxStatusPending,
		TemplateName:      strings.TrimSpace(msg.TemplateName),
		Tag:               strings.TrimSpace(msg.Tag),
		MessageType:       normalizedMessageType(msg.MessageType),
		RecipientRedacted: logger.RedactEmails(msg.To),
		Payload:           msg,
		NextAttemptAt:     time.Now().UTC(),
	}
	e.applyOutboxOrigin(row)
	if err := e.outboxStore.Enqueue(ctx, row); err != nil {
		return err
	}
	metrics.EmailOutboxEnqueued.WithLabelValues(row.TemplateName).Inc()
	return nil
}

// DeliverOutboxMessage performs one provider call for a queued message. It is
// the OutboxSender half of the worker contract: retries, backoff and terminal
// state live in OutboxWorker, transport and bookkeeping live here.
func (e *EmailServer) DeliverOutboxMessage(ctx context.Context, msg EmailMessage) (SendResult, error) {
	timeout := e.sendTimeout
	if timeout <= 0 {
		timeout = defaultEmailSendTimeout
	}
	lookupCtx, lookupCancel := context.WithTimeout(ctx, timeout)
	if err := e.checkSuppressed(lookupCtx, msg); err != nil {
		lookupCancel()
		return SendResult{}, err
	}
	lookupCancel()

	sendCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := e.sendOnce(sendCtx, msg)
	if err != nil {
		return SendResult{}, err
	}
	e.recordOutboundSend(ctx, msg, result)
	return result, nil
}

// ProviderLabel is the configured transport name (resend/postmark).
func (e *EmailServer) ProviderLabel() string { return providerNameOf(e.provider) }

// RecordOutboxFailure writes a dead-lettered send to the redacted outbound
// ledger and the exhausted-send metric, so the outbox path keeps the exact
// operator surface the legacy synchronous path had.
func (e *EmailServer) RecordOutboxFailure(ctx context.Context, msg EmailMessage, sendErr error) {
	e.recordOutboundFailure(ctx, msg, sendErr)
}

func (e *EmailServer) sendWithRetry(msg EmailMessage, timeout time.Duration) error {
	attempts := e.sendAttempts()
	var last error
	for n := 1; n <= attempts; n++ {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		result, err := e.sendOnce(ctx, msg)
		cancel()
		if err == nil {
			e.recordOutboundSend(context.Background(), msg, result)
			return nil
		}
		last = err
		if !isRetryableEmailFailure(err) || n == attempts {
			e.recordOutboundFailure(context.Background(), msg, err)
			return err
		}
		if pause := e.retryPause(); pause > 0 {
			time.Sleep(pause)
		}
	}
	return last
}

func (e *EmailServer) sendOnce(ctx context.Context, msg EmailMessage) (SendResult, error) {
	msg.Subject = sanitizeHeaderValue(msg.Subject)
	if rp, ok := e.provider.(EmailReceiptProvider); ok {
		return rp.SendWithReceipt(ctx, msg)
	}
	return SendResult{}, e.provider.Send(ctx, msg)
}

func (e *EmailServer) sendAttempts() int {
	if e != nil && e.maxSendAttempts > 0 {
		return e.maxSendAttempts
	}
	return defaultEmailSendAttempts
}

func (e *EmailServer) retryPause() time.Duration {
	if e == nil {
		return defaultEmailRetryBackoff
	}
	return e.retryBackoff
}

func isRetryableEmailFailure(err error) bool {
	if err == nil || errors.Is(err, ErrRecipientSuppressed) {
		return false
	}
	switch classifyEmailFailure(err) {
	case "rate_limited", "timeout", "upstream":
		return true
	default:
		return false
	}
}

func ensureEmailIdempotencyKey(msg *EmailMessage) {
	if strings.TrimSpace(msg.IdempotencyKey) != "" {
		return
	}
	name := strings.TrimSpace(msg.TemplateName)
	if name == "" {
		name = strings.TrimSpace(msg.Tag)
	}
	if name == "" {
		name = "email"
	}
	msg.IdempotencyKey = fmt.Sprintf("pv-%s-%d", sanitizeIdempotency(name), time.Now().UnixNano())
}

func sanitizeIdempotency(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := b.String()
	if out == "" {
		return "email"
	}
	return out
}

func (e *EmailServer) recordOutboundSend(ctx context.Context, msg EmailMessage, result SendResult) {
	if e.outboundStore == nil || strings.TrimSpace(result.ProviderMessageID) == "" {
		return
	}
	rec := EmailOutboundSend{
		Provider:          providerNameOf(e.provider),
		ProviderMessageID: result.ProviderMessageID,
		RecipientRedacted: logger.RedactEmails(msg.To),
		TemplateName:      strings.TrimSpace(msg.TemplateName),
		Tag:               strings.TrimSpace(msg.Tag),
		MessageType:       normalizedMessageType(msg.MessageType),
		Status:            OutboundStatusSent,
		SentAt:            time.Now().UTC(),
	}
	if err := e.outboundStore.RecordSend(ctx, rec); err != nil {
		logrus.WithFields(logrus.Fields{
			"provider":            rec.Provider,
			"provider_message_id": rec.ProviderMessageID,
			"template_name":       rec.TemplateName,
		}).WithError(err).Warn("email outbound send record failed")
	}
}

func (e *EmailServer) recordOutboundFailure(ctx context.Context, msg EmailMessage, sendErr error) {
	reason := classifyEmailFailure(sendErr)
	metrics.EmailSendExhausted.WithLabelValues(providerNameOf(e.provider), reason).Inc()
	if e.outboundStore == nil {
		return
	}
	rec := EmailOutboundSend{
		Provider:          providerNameOf(e.provider),
		ProviderMessageID: fmt.Sprintf("local-failed-%d", time.Now().UnixNano()),
		RecipientRedacted: logger.RedactEmails(msg.To),
		TemplateName:      strings.TrimSpace(msg.TemplateName),
		Tag:               strings.TrimSpace(msg.Tag),
		MessageType:       normalizedMessageType(msg.MessageType),
		Status:            OutboundStatusFailed,
		LastEventType:     reason,
		SentAt:            time.Now().UTC(),
	}
	now := rec.SentAt
	rec.LastEventAt = &now
	if err := e.outboundStore.RecordSend(ctx, rec); err != nil {
		logrus.WithFields(logrus.Fields{
			"provider":       rec.Provider,
			"template_name":  rec.TemplateName,
			"failure_reason": reason,
		}).WithError(err).Warn("email outbound failure record failed")
	}
}

func (e *EmailServer) addUnsubscribeURL(templateBody map[string]interface{}, emails []string) {
	if len(emails) > 0 {
		// Operators manage email notifications on the account page's
		// Notifications tab (account-level email toggles + per-business alerts).
		// The legacy /profile?tab=notifications target was never built and 404'd.
		templateBody["unsubscribe_url"] = appBaseURL() + "/account?tab=notifications"
	}
}

// sendEmail renders the template and dispatches the resulting EmailMessage.
func (e *EmailServer) sendEmail(to []string, from, tag, templateName, language string, templateBody map[string]interface{}, msgType MessageType) error {
	return e.sendEmailWithAttachments(to, from, tag, templateName, language, templateBody, msgType, nil)
}

// sendEmailWithAttachments renders the template and dispatches the resulting
// EmailMessage with the supplied attachments. sendEmail delegates here with no
// attachments.
func (e *EmailServer) sendEmailWithAttachments(to []string, from, tag, templateName, language string, templateBody map[string]interface{}, msgType MessageType, attachments []EmailAttachment) error {
	return e.sendEmailWithAttachmentsIdempotent(to, from, tag, templateName, language, templateBody, msgType, attachments, "")
}

func (e *EmailServer) sendEmailWithAttachmentsIdempotent(to []string, from, tag, templateName, language string, templateBody map[string]interface{}, msgType MessageType, attachments []EmailAttachment, idempotencyKey string) error {
	resolvedLanguage := normalizeTemplateLanguage(language)

	addInstanceTemplateFields(templateBody)
	templateBody["footer_variant"] = footerVariant(templateName, msgType)

	e.addUnsubscribeURL(templateBody, to)

	// P2-10: marketing-toned sends get a tokenized, no-login unsubscribe —
	// footer page link + RFC-8058 one-click headers. Billing/guest mail: none.
	var extraHeaders map[string]string
	if isMarketingToned(templateName, msgType) && len(to) > 0 {
		if token, ok := SignUnsubscribeToken(to[0]); ok {
			templateBody["marketing_unsubscribe_url"] = fmt.Sprintf(
				"%s/unsubscribe?token=%s&lang=%s", appBaseURL(), url.QueryEscape(token), resolvedLanguage)
			extraHeaders = map[string]string{
				"List-Unsubscribe":      fmt.Sprintf("<%s/api/v1/email/unsubscribe?token=%s>", apiPublicBaseURL(), url.QueryEscape(token)),
				"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
			}
		}
	}

	htmlBody, err := e.templates.Render(language, templateName, templateBody)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"template_name":   templateName,
			"language":        resolvedLanguage,
			"recipient_count": len(to),
		}).WithError(err).Error("Failed to render email template")
		return fmt.Errorf("failed to render template %s: %w", templateName, err)
	}

	subject, err := e.templates.renderSubject(language, templateName, templateBody)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"template_name": templateName,
			"language":      resolvedLanguage,
		}).WithError(err).Warn("Failed to render email subject; using default")
		subject = ""
	}
	if strings.TrimSpace(subject) == "" {
		subject = "Notification from Payverge"
	}

	msg := EmailMessage{
		From:           from,
		To:             to,
		Subject:        subject,
		HTMLBody:       htmlBody,
		ReplyTo:        replyToAddress(),
		Tag:            tag,
		MessageType:    msgType,
		Attachments:    attachments,
		Headers:        extraHeaders,
		IdempotencyKey: strings.TrimSpace(idempotencyKey),
		TemplateName:   templateName,
	}
	return e.dispatch(msg)
}

func (e *EmailServer) SendTransactionalEmail(to []string, templateName string, templateBody map[string]interface{}, language string) error {
	return e.sendEmail(to, e.FromTransactional, "transactional", templateName, language, templateBody, MessageTypeTransactional)
}

// SendTransactionalEmailIdempotent renders a transactional template and
// propagates a stable logical-send key to transports that support deduplication.
func (e *EmailServer) SendTransactionalEmailIdempotent(to []string, templateName string, templateBody map[string]interface{}, language, idempotencyKey string) error {
	return e.sendEmailWithAttachmentsIdempotent(
		to, e.FromTransactional, "transactional", templateName, language,
		templateBody, MessageTypeTransactional, nil, idempotencyKey,
	)
}

// SendTransactionalEmailWithAttachments is SendTransactionalEmail with file
// attachments (e.g. a fiscal receipt PDF).
func (e *EmailServer) SendTransactionalEmailWithAttachments(to []string, templateName string, templateBody map[string]interface{}, language string, attachments []EmailAttachment) error {
	return e.sendEmailWithAttachments(to, e.FromTransactional, "transactional", templateName, language, templateBody, MessageTypeTransactional, attachments)
}

func (e *EmailServer) SendUpdatesEmail(to []string, templateName string, templateBody map[string]interface{}, language string) error {
	return e.sendEmail(to, e.FromUpdates, "updates", templateName, language, templateBody, MessageTypeBroadcast)
}

// SendCustomEmail sends an HTML email without using a template.
// The non-200 error path here was buggy in the previous implementation
// (returned nil); the provider now correctly returns a wrapped error.
func (e *EmailServer) SendCustomEmail(to []string, subject, htmlBody, textBody string) error {
	msg := EmailMessage{
		From:        e.FromTransactional,
		To:          to,
		Subject:     subject,
		HTMLBody:    htmlBody,
		TextBody:    textBody,
		ReplyTo:     replyToAddress(),
		Tag:         "tools",
		MessageType: MessageTypeTransactional,
	}
	return e.dispatch(msg)
}

// replyToAddress is EMAIL_REPLY_TO, else SUPPORT_EMAIL (config.SupportEmail:
// a valid bare address, else nothing), else empty (no Reply-To header, so
// replies reach the instance's own From address). There is no upstream
// default: a fork's customers must never reply to the original project's
// inbox.
func replyToAddress() string {
	if v := strings.TrimSpace(os.Getenv("EMAIL_REPLY_TO")); v != "" {
		return v
	}
	return config.SupportEmail()
}

// appBaseURL is the instance's public (frontend) origin used in email links.
func appBaseURL() string {
	return config.PublicURL()
}

// apiPublicBaseURL is the public base of the API host (one-click unsubscribe
// POSTs land here, not on the frontend): APP_BASE_URL, else PUBLIC_URL —
// same-origin deploys serve the API under /api/v1.
func apiPublicBaseURL() string {
	return config.APIBaseURL()
}
