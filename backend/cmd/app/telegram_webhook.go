package main

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type telegramWebhookRegistrar interface {
	MakeRequest(endpoint string, params tgbotapi.Params) (*tgbotapi.APIResponse, error)
}

func configureTelegramWebhook(registrar telegramWebhookRegistrar, apiBaseURL string, secret string) (string, error) {
	if registrar == nil {
		return "", fmt.Errorf("telegram bot is not initialized")
	}

	trimmedBaseURL := strings.TrimRight(strings.TrimSpace(apiBaseURL), "/")
	if trimmedBaseURL == "" {
		return "", fmt.Errorf("public API base URL is required")
	}

	trimmedSecret := strings.TrimSpace(secret)
	if trimmedSecret == "" {
		return "", fmt.Errorf("telegram webhook secret is required")
	}

	webhookURL := trimmedBaseURL + "/api/v1/webhooks/telegram"
	_, err := registrar.MakeRequest("setWebhook", tgbotapi.Params{
		"url":          webhookURL,
		"secret_token": trimmedSecret,
	})
	if err != nil {
		return "", err
	}

	return webhookURL, nil
}
