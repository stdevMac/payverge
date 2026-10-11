package paymentcontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func contractEvent(action, provider, testCase string) string {
	event := map[string]string{
		"Action":  action,
		"Package": "github.com/stdevmac/payverge/backend/internal/handlers",
		"Test":    fmt.Sprintf("TestProductionPaymentProviderContract/%s/signature/%s", provider, testCase),
	}
	encoded, _ := json.Marshal(event)
	return string(encoded) + "\n"
}

func completeExecutableEvents() []byte {
	var events bytes.Buffer
	for _, provider := range ProductionProviders {
		for _, testCase := range ExecutableCases {
			event := map[string]string{
				"Action":  "pass",
				"Package": "github.com/stdevmac/payverge/backend/internal/handlers",
				"Test":    fmt.Sprintf("TestProductionPaymentProviderContract/%s/%s", provider, testCase.ID),
			}
			encoded, _ := json.Marshal(event)
			events.Write(encoded)
			events.WriteByte('\n')
		}
	}
	return events.Bytes()
}

func TestBuildHermeticProofReportsCompleteExecutableMatrix(t *testing.T) {
	generatedAt := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	proof, err := BuildHermeticProof(bytes.NewReader(completeExecutableEvents()), "0123456789abcdef0123456789abcdef01234567", generatedAt)
	require.NoError(t, err)
	require.Equal(t, "pass", proof.Result)
	require.Equal(t, RequiredChecks, proof.ExecutedChecks)
	require.Empty(t, proof.MissingChecks)
	require.Len(t, proof.Providers, 3)
	for _, provider := range proof.Providers {
		require.Equal(t, "pass", provider.HermeticContract.Result)
		require.Equal(t, RequiredChecks, provider.HermeticContract.Checks)
		require.NotNil(t, provider.HermeticContract.MissingChecks, "empty proof arrays must serialize as [] rather than null")
		require.Len(t, provider.HermeticContract.Cases, 25)
	}
	encoded, err := json.Marshal(proof)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), `"missing_checks":null`)
}

func TestBuildHermeticProofIgnoresPassingParentTestEvents(t *testing.T) {
	input := append([]byte(nil), completeExecutableEvents()...)
	input = append(input, []byte(`{"Action":"pass","Package":"github.com/stdevmac/payverge/backend/internal/handlers","Test":"TestProductionPaymentProviderContract/paypal/signature"}`+"\n")...)

	_, err := BuildHermeticProof(bytes.NewReader(input), "0123456789abcdef0123456789abcdef01234567", time.Now().UTC())
	require.NoError(t, err)
}

func TestBuildHermeticProofRejectsMissingFailedAndSkippedCases(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef01234567"
	now := time.Now().UTC()

	t.Run("missing", func(t *testing.T) {
		input := bytes.Replace(completeExecutableEvents(), []byte(contractEvent("pass", "stripe", "valid")), nil, 1)
		_, err := BuildHermeticProof(bytes.NewReader(input), sha, now)
		require.ErrorContains(t, err, "missing passing contract case")
	})

	t.Run("failed", func(t *testing.T) {
		input := bytes.Replace(completeExecutableEvents(), []byte(contractEvent("pass", "paypal", "invalid")), []byte(contractEvent("fail", "paypal", "invalid")), 1)
		_, err := BuildHermeticProof(bytes.NewReader(input), sha, now)
		require.ErrorContains(t, err, "did not pass")
	})

	t.Run("skipped", func(t *testing.T) {
		input := bytes.Replace(completeExecutableEvents(), []byte(contractEvent("pass", "mercadopago", "missing")), []byte(contractEvent("skip", "mercadopago", "missing")), 1)
		_, err := BuildHermeticProof(bytes.NewReader(input), sha, now)
		require.ErrorContains(t, err, "did not pass")
	})
}

func TestBuildHermeticProofRejectsInvalidSourceSHA(t *testing.T) {
	_, err := BuildHermeticProof(bytes.NewReader(completeExecutableEvents()), "candidate", time.Now().UTC())
	require.ErrorContains(t, err, "source SHA")
}

func TestBuildHermeticProofRejectsContractEventsFromAnotherPackage(t *testing.T) {
	input := bytes.Replace(
		completeExecutableEvents(),
		[]byte(`"Package":"github.com/stdevmac/payverge/backend/internal/handlers"`),
		[]byte(`"Package":"example.invalid/handlers"`),
		1,
	)
	_, err := BuildHermeticProof(bytes.NewReader(input), "0123456789abcdef0123456789abcdef01234567", time.Now().UTC())
	require.ErrorContains(t, err, "unexpected package")
}
