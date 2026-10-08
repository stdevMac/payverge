package stripetest

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSimulatorProducesStablePayloadAndSignature(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	secret := os.Getenv("STRIPE_SIMULATOR_SECRET")
	if secret == "" {
		secret = "local-ci-secret"
	}
	a := New(secret, now)
	b := New(secret, now)

	payloadA, signatureA, err := a.Event("evt_1", "checkout.session.completed", map[string]interface{}{"id": "cs_1"})
	require.NoError(t, err)
	payloadB, signatureB, err := b.Event("evt_1", "checkout.session.completed", map[string]interface{}{"id": "cs_1"})
	require.NoError(t, err)

	require.Equal(t, payloadA, payloadB)
	require.Equal(t, signatureA, signatureB)
	require.NotContains(t, string(payloadA), secret)
	require.NotContains(t, signatureA, secret)
}
