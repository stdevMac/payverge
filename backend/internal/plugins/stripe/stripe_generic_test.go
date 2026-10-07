package stripe

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStripeGenericPaymentMethodsRequireSupportedEntryPoints(t *testing.T) {
	plugin := &StripePlugin{}

	paymentID, err := plugin.ProcessPayment(1, 1000, "USD", nil)
	require.Error(t, err)
	require.Empty(t, paymentID)
	require.Contains(t, err.Error(), "unsupported")

	err = plugin.RefundPayment(1, "pi_any", 1000)
	require.Error(t, err)
	require.Contains(t, err.Error(), "plugin service not configured")

	status, err := plugin.GetPaymentStatus(1, "pi_any")
	require.Error(t, err)
	require.Empty(t, status)
	require.Contains(t, err.Error(), "plugin service not configured")
}

func TestStripeSourceHasNoMockPaymentArtifacts(t *testing.T) {
	src, err := os.ReadFile("stripe.go")
	require.NoError(t, err)
	body := string(src)
	for _, banned := range []string{"pi_mock_", "mock_order", "simulatePayment", "MockPayment"} {
		require.NotContains(t, body, banned, "stripe.go must not reintroduce dead mock payment artifact %q", banned)
	}
}
