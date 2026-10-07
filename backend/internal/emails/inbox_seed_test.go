package emails

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSendCriticalInboxSeedCoversBothMailboxFamilies(t *testing.T) {
	provider := &captureProvider{}
	server, err := NewEmailServer(provider, "noreply@payverge.io", "updates@payverge.io", resolveTemplatesRoot(t))
	require.NoError(t, err)

	require.NoError(t, SendCriticalInboxSeed(server, []string{
		"payverge-seed@gmail.example", "payverge-seed@outlook.example",
	}, "seed-contract-20260801"))
	require.Len(t, provider.sent, 8)
	for _, recipient := range []string{"payverge-seed@gmail.example", "payverge-seed@outlook.example"} {
		count := 0
		for _, msg := range provider.sent {
			if len(msg.To) == 1 && msg.To[0] == recipient {
				count++
				require.Contains(t, msg.HTMLBody, "seed-contract-20260801")
			}
		}
		require.Equal(t, 4, count)
	}
}
