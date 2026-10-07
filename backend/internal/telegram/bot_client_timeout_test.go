package telegram_test

import (
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/telegram"
)

// TestNewBoundedBotHasTimeout verifies that BoundedTelegramClient returns an
// *http.Client whose Timeout is exactly 15 seconds. This guards against
// accidental regressions that would restore the unbounded http.DefaultClient
// (Timeout == 0), which can wedge bot.Send/MakeRequest calls indefinitely.
func TestNewBoundedBotHasTimeout(t *testing.T) {
	t.Parallel()

	client := telegram.BoundedTelegramClient()
	if client == nil {
		t.Fatal("BoundedTelegramClient() returned nil")
	}

	const want = 15 * time.Second
	if client.Timeout != want {
		t.Errorf("client.Timeout = %v; want %v", client.Timeout, want)
	}
}
