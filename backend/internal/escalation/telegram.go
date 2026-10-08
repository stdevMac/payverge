// Package escalation delivers support escalations raised by the Ops Assistant
// bot to a durable store plus out-of-band notifiers.
package escalation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// TelegramNotifier posts escalation messages to a DEDICATED support-escalation
// Telegram bot. It intentionally uses its own env vars
// (TELEGRAM_ESCALATION_BOT_TOKEN / TELEGRAM_ESCALATION_CHAT_ID), completely
// separate from the operator-notification bot (TELEGRAM_TOKEN / TELEGRAM_BOT_TOKEN),
// so support alerts never mix with operator order/payment pings. When either var
// is unset the notifier is a silent no-op (feature dark), never an error.
type TelegramNotifier struct {
	token  string
	chatID string
	client *http.Client
}

// NewTelegramNotifierFromEnv builds the notifier from the escalation-specific
// env vars. Returns a configured notifier (Enabled() reports readiness).
func NewTelegramNotifierFromEnv() *TelegramNotifier {
	return &TelegramNotifier{
		token:  strings.TrimSpace(os.Getenv("TELEGRAM_ESCALATION_BOT_TOKEN")),
		chatID: strings.TrimSpace(os.Getenv("TELEGRAM_ESCALATION_CHAT_ID")),
		client: &http.Client{Timeout: 8 * time.Second},
	}
}

// Enabled reports whether both the bot token and chat id are configured.
func (n *TelegramNotifier) Enabled() bool {
	return n != nil && n.token != "" && n.chatID != ""
}

// baseURL is overridable in tests to point at a fake Telegram API.
var baseURL = "https://api.telegram.org"

// Send posts a plain-text message. No-op (nil error) when disabled.
func (n *TelegramNotifier) Send(ctx context.Context, text string) error {
	if !n.Enabled() {
		return nil
	}
	payload, err := json.Marshal(map[string]any{
		"chat_id":                  n.chatID,
		"text":                     text,
		"disable_web_page_preview": true,
	})
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/bot%s/sendMessage", baseURL, n.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram escalation status %d", resp.StatusCode)
	}
	return nil
}
