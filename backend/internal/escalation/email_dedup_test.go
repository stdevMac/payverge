package escalation

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/agents"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"

	"github.com/stretchr/testify/require"
)

// countingProvider counts every email actually sent.
type countingProvider struct{ sends int32 }

func (p *countingProvider) Send(context.Context, emails.EmailMessage) error {
	atomic.AddInt32(&p.sends, 1)
	return nil
}

func newCountingEmailServer(t *testing.T) (*emails.EmailServer, *countingProvider) {
	t.Helper()
	// No default ADMIN_EMAILS inbox exists any more; give the test one.
	originalAdmins := emails.AdminsEmails
	emails.AdminsEmails = []string{"ops@example.org"}
	t.Cleanup(func() { emails.AdminsEmails = originalAdmins })
	provider := &countingProvider{}
	srv, err := emails.NewEmailServer(provider, "noreply@test.payverge.io", "updates@test.payverge.io", "../../email/templates")
	require.NoError(t, err)
	return srv, provider
}

// When the escalation tool already emailed admins (email_sent=true), the durable
// Escalate path must NOT email again — otherwise one incident double-notifies.
func TestEscalateSkipsEmailWhenToolAlreadySent(t *testing.T) {
	setupEscalationDB(t)
	srv, provider := newCountingEmailServer(t)

	svc := NewService(srv)
	svc.Escalate(agents.EscalationEvent{
		Source:           "ops",
		BusinessID:       7,
		Issue:            "printer down",
		Reason:           "tool",
		ToolName:         "create_support_escalation",
		EmailAlreadySent: true,
	})

	require.Equal(t, int32(0), atomic.LoadInt32(&provider.sends),
		"Escalate must not re-email when the tool already sent")

	// The row still persists (never silently dropped).
	_, total, err := database.ListEscalations(10, 0)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
}

// When the tool's email failed (email_sent=false), Escalate is the fallback and
// MUST send exactly one email.
func TestEscalateSendsEmailWhenToolDidNotSend(t *testing.T) {
	setupEscalationDB(t)
	srv, provider := newCountingEmailServer(t)

	svc := NewService(srv)
	svc.Escalate(agents.EscalationEvent{
		Source:           "ops",
		BusinessID:       7,
		Issue:            "printer down",
		Reason:           "tool",
		EmailAlreadySent: false,
	})

	require.Equal(t, int32(1), atomic.LoadInt32(&provider.sends),
		"Escalate must send the fallback email when the tool did not")
}

// With ADMIN_EMAILS unset there is no inbox: the escalation row is still
// recorded but no email is sent (in particular not to an upstream default).
func TestEscalateSendsNoEmailWithoutAdminInbox(t *testing.T) {
	setupEscalationDB(t)
	srv, provider := newCountingEmailServer(t)
	emails.AdminsEmails = nil // restored by newCountingEmailServer's cleanup

	svc := NewService(srv)
	svc.Escalate(agents.EscalationEvent{
		Source:           "ops",
		BusinessID:       7,
		Issue:            "printer down",
		Reason:           "tool",
		EmailAlreadySent: false,
	})

	require.Equal(t, int32(0), atomic.LoadInt32(&provider.sends),
		"no ADMIN_EMAILS means no escalation email")
}
