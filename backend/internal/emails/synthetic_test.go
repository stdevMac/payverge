package emails

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSyntheticMessage(t *testing.T) {
	msg, err := BuildSyntheticMessage("Payverge <noreply@payverge.io>", "monitor@payverge.io")
	require.NoError(t, err)
	assert.Equal(t, []string{"monitor@payverge.io"}, msg.To)
	assert.Equal(t, "payverge_email_healthcheck", msg.Tag)
	assert.Equal(t, MessageTypeTransactional, msg.MessageType)
	assert.NotEmpty(t, msg.TextBody)
}

func TestBuildSyntheticMessageKeepsBareSenderResendCompatible(t *testing.T) {
	msg, err := BuildSyntheticMessage("noreply@payverge.io", "monitor@payverge.io")
	require.NoError(t, err)
	assert.Equal(t, "noreply@payverge.io", msg.From)
}

func TestBuildSyntheticMessageRejectsInvalidRecipient(t *testing.T) {
	_, err := BuildSyntheticMessage("noreply@payverge.io", "a@example.com,b@example.com")
	require.Error(t, err)
	_, err = BuildSyntheticMessage("noreply@payverge.io", "not-an-email")
	require.Error(t, err)
}
