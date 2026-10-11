package llm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIErrorStringDoesNotExposeProviderMessage(t *testing.T) {
	providerPayload := `invalid prompt for owner@example.com dish="Family Recipe"`
	err := &APIError{Class: ErrUpstream, Status: 400, Message: providerPayload}

	require.Contains(t, err.Error(), ErrUpstream.Error())
	require.Contains(t, err.Error(), "status 400")
	require.NotContains(t, err.Error(), providerPayload)
	// The structured field remains available to an explicit, access-controlled
	// diagnostic path; ordinary %v logging must not expose it implicitly.
	require.Equal(t, providerPayload, err.Message)
}
