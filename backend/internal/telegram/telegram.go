package telegram

import (
	"net/http"
	"time"
)

// botClientTimeout is the dial-plus-response deadline for every outbound
// Telegram API call. The zero-value http.DefaultClient blocks indefinitely,
// which can wedge bot.Send / MakeRequest when api.telegram.org is slow or
// unreachable.
const botClientTimeout = 15 * time.Second

// boundedTelegramClient returns a fresh *http.Client with a hard 15-second
// timeout so Telegram API calls fail fast rather than blocking forever.
func boundedTelegramClient() *http.Client {
	return &http.Client{Timeout: botClientTimeout}
}

// BoundedTelegramClient is the exported entry point used by cmd/app/main.go
// when constructing the bot via tgbotapi.NewBotAPIWithClient. It returns the
// same bounded client as the unexported helper; keeping both lets internal
// callers stay concise while still allowing external packages (main, tests) to
// access the constructor.
func BoundedTelegramClient() *http.Client {
	return boundedTelegramClient()
}
