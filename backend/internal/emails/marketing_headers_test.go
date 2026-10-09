package emails

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type headerCaptureProvider struct {
	mu   sync.Mutex
	sent []EmailMessage
}

func (p *headerCaptureProvider) Send(_ context.Context, msg EmailMessage) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sent = append(p.sent, msg)
	return nil
}

func newHeaderCaptureServer(t *testing.T) (*EmailServer, *headerCaptureProvider) {
	t.Helper()
	orig := EmailServerInstance
	t.Cleanup(func() { EmailServerInstance = orig })
	provider := &headerCaptureProvider{}
	server, err := NewEmailServer(provider, "test@payverge.io", "updates@payverge.io", resolveTemplatesRoot(t))
	require.NoError(t, err)
	return server, provider
}

// P2-10: marketing-toned sends carry RFC-8058 one-click headers + a tokenized
// footer link (no login required).
func TestMarketingSend_CarriesListUnsubscribeHeadersAndFooterLink(t *testing.T) {
	t.Setenv("UNSUBSCRIBE_TOKEN_SECRET", "header-test-secret")
	t.Setenv("PUBLIC_URL", "https://payverge.io")
	t.Setenv("APP_BASE_URL", "https://api.payverge.io")

	server, provider := newHeaderCaptureServer(t)
	require.NoError(t, server.SendMilestoneFirstOrderEmail([]string{"owner@example.com"}, "Owner", "https://payverge.io/business/1/dashboard", "en"))

	require.Len(t, provider.sent, 1)
	msg := provider.sent[0]
	lu := msg.Headers["List-Unsubscribe"]
	require.Contains(t, lu, "https://api.payverge.io/api/v1/email/unsubscribe?token=", "one-click target is the API endpoint")
	require.Equal(t, "List-Unsubscribe=One-Click", msg.Headers["List-Unsubscribe-Post"], "RFC 8058 one-click marker required")
	require.Contains(t, msg.HTMLBody, "https://payverge.io/unsubscribe?token=", "footer links the no-auth unsubscribe page")
	require.Contains(t, msg.HTMLBody, "lang=eng", "footer link carries the email's language")
}

// Decision under test: transactional emails are exempt from opt-out and
// must NOT advertise an unsubscribe header that would do nothing.
func TestTransactionalSend_CarriesNoListUnsubscribeHeaders(t *testing.T) {
	t.Setenv("UNSUBSCRIBE_TOKEN_SECRET", "header-test-secret")

	server, provider := newHeaderCaptureServer(t)
	require.NoError(t, server.SendGettingStartedEmail([]string{"owner@example.com"}, "Owner", "https://payverge.io/business/1/dashboard", "en"))

	require.Len(t, provider.sent, 1)
	msg := provider.sent[0]
	require.Empty(t, msg.Headers["List-Unsubscribe"], "transactional emails must not carry a do-nothing unsubscribe header")
	require.Empty(t, msg.Headers["List-Unsubscribe-Post"])
	require.NotContains(t, msg.HTMLBody, "/unsubscribe?token=", "transactional footer keeps only the account-preferences link")
}

// Missing secret material degrades gracefully: no headers, no broken link.
func TestMarketingSend_NoSecretFallsBackToAccountLinkOnly(t *testing.T) {
	t.Setenv("UNSUBSCRIBE_TOKEN_SECRET", "")
	t.Setenv("JWT_SECRET_KEY", "")

	server, provider := newHeaderCaptureServer(t)
	require.NoError(t, server.SendMilestoneFirstOrderEmail([]string{"owner@example.com"}, "Owner", "https://payverge.io/business/1/dashboard", "en"))

	require.Len(t, provider.sent, 1)
	require.Empty(t, provider.sent[0].Headers["List-Unsubscribe"])
	require.NotContains(t, strings.ToLower(provider.sent[0].HTMLBody), "/unsubscribe?token=")
}
