package handlers

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdminResetPasswordEmailUsesInstanceIdentity(t *testing.T) {
	for _, k := range []string{"PRODUCT_NAME", "SUPPORT_EMAIL"} {
		t.Setenv(k, "")
	}
	link := "https://pos.example.com/reset-password?token=abc%2B1"

	htmlBody, textBody := adminResetPasswordEmailBodies(link)
	require.Contains(t, htmlBody, "A Payverge administrator")
	require.Contains(t, htmlBody, "please contact your administrator.")
	require.Contains(t, textBody, link)
	require.Contains(t, textBody, "please contact your administrator.")
	for _, body := range []string{htmlBody, textBody} {
		require.NotContains(t, body, "payverge.io")
	}

	t.Setenv("PRODUCT_NAME", "Tavola <Pro>")
	t.Setenv("SUPPORT_EMAIL", "help@pos.example.com")
	htmlBody, textBody = adminResetPasswordEmailBodies(link)
	require.Contains(t, htmlBody, "A Tavola &lt;Pro&gt; administrator")
	require.Contains(t, htmlBody, `<a href="mailto:help@pos.example.com">help@pos.example.com</a>`)
	require.Contains(t, htmlBody, `href="https://pos.example.com/reset-password?token=abc%2B1"`)
	require.Contains(t, textBody, "A Tavola <Pro> administrator")
	require.Contains(t, textBody, "please contact help@pos.example.com.")
	require.False(t, strings.Contains(htmlBody, "<Pro>"), "product name must be HTML-escaped")
}
