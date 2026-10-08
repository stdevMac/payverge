package emails

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsReservedRecipient(t *testing.T) {
	for addr, want := range map[string]bool{
		"demo+owner-7@payverge.local": true,
		"Demo <guest@payverge.local>": true,
		"ops@host.localhost":          true,
		"x@example.invalid":           true,
		"cook@kitchen.internal":       true,
		"a@PAYVERGE.LOCAL.":           true,
		"owner@payverge.io":           false,
		"guest@example.test":          false,
		"no-at-sign":                  false,
	} {
		require.Equal(t, want, isReservedRecipient(addr), addr)
	}
}

func TestDispatchSkipsSpecialUseRecipients(t *testing.T) {
	provider := &retryProviderStub{}
	server, err := NewEmailServer(provider, "noreply@payverge.io", "updates@payverge.io", resolveTemplatesRoot(t))
	require.NoError(t, err)
	server.retryBackoff = 0

	// Demo seeding: every recipient is on payverge.local, so nothing reaches
	// the provider and the caller sees success.
	require.NoError(t, server.SendCustomEmail([]string{"demo+owner-1@payverge.local"}, "Low stock", "<p>x</p>", "x"))
	require.Equal(t, 0, provider.calls)

	// Mixed list: the real mailbox is still delivered.
	capture := &captureProvider{}
	server, err = NewEmailServer(capture, "noreply@payverge.io", "updates@payverge.io", resolveTemplatesRoot(t))
	require.NoError(t, err)
	require.NoError(t, server.SendCustomEmail([]string{"demo@payverge.local", "owner@example.test"}, "Low stock", "<p>x</p>", "x"))
	require.Len(t, capture.sent, 1)
	require.Equal(t, []string{"owner@example.test"}, capture.sent[0].To)
}
