package main

import (
	"errors"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingTelegramWebhookRegistrar struct {
	endpoint string
	params   tgbotapi.Params
	err      error
}

func (r *recordingTelegramWebhookRegistrar) MakeRequest(endpoint string, params tgbotapi.Params) (*tgbotapi.APIResponse, error) {
	r.endpoint = endpoint
	r.params = params
	if r.err != nil {
		return nil, r.err
	}
	return &tgbotapi.APIResponse{Ok: true}, nil
}

func TestConfigureTelegramWebhookRegistersPublicURLAndSecret(t *testing.T) {
	registrar := &recordingTelegramWebhookRegistrar{}

	webhookURL, err := configureTelegramWebhook(registrar, "https://api.example.com/", " secret-token ")

	require.NoError(t, err)
	assert.Equal(t, "https://api.example.com/api/v1/webhooks/telegram", webhookURL)
	assert.Equal(t, "setWebhook", registrar.endpoint)
	assert.Equal(t, "https://api.example.com/api/v1/webhooks/telegram", registrar.params["url"])
	assert.Equal(t, "secret-token", registrar.params["secret_token"])
	assert.NotContains(t, webhookURL, "secret-token")
}

func TestConfigureTelegramWebhookReturnsProviderError(t *testing.T) {
	registrar := &recordingTelegramWebhookRegistrar{err: errors.New("telegram down")}

	_, err := configureTelegramWebhook(registrar, "https://api.example.com", "secret-token")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "telegram down")
}

func TestConfigureTelegramWebhookRejectsMissingInputs(t *testing.T) {
	registrar := &recordingTelegramWebhookRegistrar{}

	_, err := configureTelegramWebhook(nil, "https://api.example.com", "secret-token")
	require.Error(t, err)

	_, err = configureTelegramWebhook(registrar, "", "secret-token")
	require.Error(t, err)

	_, err = configureTelegramWebhook(registrar, "https://api.example.com", "")
	require.Error(t, err)
}
