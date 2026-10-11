package escalation

import (
	"context"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/agents"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

// Service persists escalations and fans them out to Telegram (dedicated bot) and
// email. It implements agents.Escalator so it can be attached to the agent loop.
type Service struct {
	email    *emails.EmailServer
	telegram *TelegramNotifier
}

// NewService builds the escalation service. The Telegram notifier is loaded from
// the escalation-specific env vars and is a silent no-op when unconfigured.
func NewService(email *emails.EmailServer) *Service {
	return &Service{email: email, telegram: NewTelegramNotifierFromEnv()}
}

var _ agents.Escalator = (*Service)(nil)

// Escalate persists a durable escalation row, then best-effort notifies via
// Telegram and email. Delivery failures are logged, never fatal — the row is
// the source of truth (admins can read it via the admin endpoint).
func (s *Service) Escalate(ev agents.EscalationEvent) {
	if s == nil {
		return
	}
	row := &database.Escalation{
		Source:            sourceOf(ev.Source),
		SessionRef:        sessionRef(ev),
		Issue:             ev.Issue,
		TranscriptSummary: ev.Transcript,
		ContactEmail:      ev.ContactEmail,
		Status:            "open",
	}
	if ev.BusinessID != 0 {
		bid := ev.BusinessID
		row.BusinessID = &bid
	}
	if err := database.CreateEscalation(row); err != nil {
		logger.Logger.Warnf("escalation: persist failed (source=%s): %v", ev.Source, err)
	}

	s.notify(ev, row)
}

func (s *Service) notify(ev agents.EscalationEvent, row *database.Escalation) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if s.telegram.Enabled() {
		if err := s.telegram.Send(ctx, telegramText(ev, row)); err != nil {
			logger.Logger.Warnf("escalation: telegram send failed: %v", err)
		}
	}

	// Single-send guarantee: if the raising path (the SupportEscalationTool)
	// already emailed admins, do not email again — the row + Telegram fallback
	// above still fire, so the escalation is never silently dropped, but the
	// incident is not double-notified.
	if s.email != nil && !ev.EmailAlreadySent {
		subject := fmt.Sprintf("Escalation [%s] — %s", ev.Source, escalationTitle(ev))
		htmlBody := emailHTML(ev, row)
		text := fmt.Sprintf("Escalation source=%s business=%v reason=%s issue=%s",
			ev.Source, businessLabel(ev.BusinessID), ev.Reason, ev.Issue)
		if err := s.email.SendCustomEmail(emails.AdminsEmails, subject, htmlBody, text); err != nil {
			logger.Logger.Warnf("escalation: email send failed: %v", err)
		}
	}
}

// sourceOf maps an event's surface label to its persisted source. The Ops
// Assistant is the only escalating surface left, so unknown labels file as
// ops; historical 'concierge' rows stay readable as plain strings.
func sourceOf(string) database.EscalationSource {
	return database.EscalationSourceOps
}

func sessionRef(ev agents.EscalationEvent) string {
	if ev.ThreadID != 0 {
		return fmt.Sprintf("thread:%d", ev.ThreadID)
	}
	return ev.SessionRef
}

func businessLabel(id uint) string {
	if id == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%d", id)
}

func escalationTitle(ev agents.EscalationEvent) string {
	issue := strings.TrimSpace(ev.Issue)
	if issue == "" {
		issue = "(no issue text)"
	}
	if len(issue) > 60 {
		issue = issue[:60] + "…"
	}
	return issue
}

func telegramText(ev agents.EscalationEvent, row *database.Escalation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🚨 Payverge escalation (%s)\n", ev.Source)
	fmt.Fprintf(&b, "Reason: %s\n", ev.Reason)
	fmt.Fprintf(&b, "Business: %s\n", businessLabel(ev.BusinessID))
	if ref := sessionRef(ev); ref != "" {
		fmt.Fprintf(&b, "Ref: %s\n", ref)
	}
	if ev.ContactEmail != "" {
		fmt.Fprintf(&b, "Contact: %s\n", ev.ContactEmail)
	}
	fmt.Fprintf(&b, "Issue: %s\n", ev.Issue)
	if strings.TrimSpace(ev.Transcript) != "" {
		fmt.Fprintf(&b, "Transcript: %s\n", ev.Transcript)
	}
	if row != nil && row.ID != 0 {
		fmt.Fprintf(&b, "Escalation #%d", row.ID)
	}
	return b.String()
}

func emailHTML(ev agents.EscalationEvent, row *database.Escalation) string {
	rowFn := func(label, val string) string {
		if val == "" {
			val = "-"
		}
		return "<p><strong>" + html.EscapeString(label) + ":</strong> " + html.EscapeString(val) + "</p>"
	}
	idLabel := ""
	if row != nil && row.ID != 0 {
		idLabel = fmt.Sprintf("#%d", row.ID)
	}
	return "<h2>Payverge escalation " + html.EscapeString(idLabel) + "</h2>" +
		rowFn("Source", ev.Source) +
		rowFn("Reason", ev.Reason) +
		rowFn("Business", businessLabel(ev.BusinessID)) +
		rowFn("Ref", sessionRef(ev)) +
		rowFn("Contact", ev.ContactEmail) +
		rowFn("Issue", ev.Issue) +
		rowFn("Transcript", ev.Transcript)
}
