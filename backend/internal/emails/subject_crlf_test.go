package emails

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmailServerStripsCRLFFromSubject(t *testing.T) {
	provider := &captureProvider{}
	server, err := NewEmailServer(provider, "noreply@payverge.io", "updates@payverge.io", resolveTemplatesRoot(t))
	require.NoError(t, err)

	err = server.SendCustomEmail(
		[]string{"guest@example.test"},
		"Hello\r\nBcc: evil@example.com",
		"<p>hi</p>",
		"hi",
	)
	require.NoError(t, err)
	require.Len(t, provider.sent, 1)

	got := provider.sent[0].Subject
	require.NotContains(t, got, "\r")
	require.NotContains(t, got, "\n")
	require.Equal(t, "Hello Bcc: evil@example.com", got)
}
