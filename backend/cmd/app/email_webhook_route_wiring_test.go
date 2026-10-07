package main

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestResendWebhookIsWiredUnderPublicWebhookGroup: the Resend delivery-event
// webhook stays mounted on the public /api/v1/webhooks group (outside auth
// middleware), now through registerEmailWebhookRoutes so it exists only when
// EMAIL_PROVIDER resolves to resend.
func TestResendWebhookIsWiredUnderPublicWebhookGroup(t *testing.T) {
	source, err := os.ReadFile("main.go")
	require.NoError(t, err)
	body := string(source)

	group := strings.Index(body, `webhookRoutes := r.Group("/api/v1/webhooks")`)
	route := strings.Index(body, `registerEmailWebhookRoutes(webhookRoutes, emailProviderName)`)
	require.NotEqual(t, -1, group)
	require.Greater(t, route, group)

	wiring, err := os.ReadFile("oss_email.go")
	require.NoError(t, err)
	require.Contains(t, string(wiring), "NewResendWebhookHandler")
	require.Contains(t, string(wiring), `group.POST("/email/resend", resendEmailWebhookHandler.Handle)`)
}
