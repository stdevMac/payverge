package telegram

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stretchr/testify/require"
)

// blockingTelegramBotSender simulates a hung Telegram API call: Send blocks
// until released. Used to prove sendHTMLMessage honors context cancellation
// instead of stalling the delivery worker past the 2-minute reclaim window.
type blockingTelegramBotSender struct {
	release chan struct{}
	entered chan struct{}
}

func (s *blockingTelegramBotSender) Send(_ tgbotapi.Chattable) (tgbotapi.Message, error) {
	if s.entered != nil {
		close(s.entered)
	}
	<-s.release
	return tgbotapi.Message{MessageID: 1}, nil
}

// A context that is already cancelled must make sendHTMLMessage return
// promptly with an error — never enter/wait on a hung HTTP call indefinitely.
func TestSendHTMLMessage_ReturnsPromptlyOnCancelledContext(t *testing.T) {
	sender := &blockingTelegramBotSender{release: make(chan struct{})}
	defer close(sender.release)
	plugin := &TelegramPlugin{botSender: sender}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() {
		_, err := plugin.sendHTMLMessage(ctx, 12345, "hello")
		done <- err
	}()

	select {
	case err := <-done:
		require.Error(t, err, "cancelled context must surface an error")
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("sendHTMLMessage did not return promptly with a cancelled context")
	}
}

// A hung send with a live context must still be bounded by the hard timeout —
// but 30s is too slow for a unit test, so we only assert cancellation mid-send.
func TestSendHTMLMessage_UnblocksWhenContextCancelledMidSend(t *testing.T) {
	sender := &blockingTelegramBotSender{release: make(chan struct{}), entered: make(chan struct{})}
	defer close(sender.release)
	plugin := &TelegramPlugin{botSender: sender}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := plugin.sendHTMLMessage(ctx, 12345, "hello")
		done <- err
	}()

	select {
	case <-sender.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("fake sender never entered Send")
	}
	cancel()

	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("sendHTMLMessage stayed blocked after context cancellation")
	}
}

// Truncation must never split a UTF-8 rune: a byte-slice at 3900 inside a
// multi-byte character produces invalid UTF-8 → Telegram 400 → 5 futile
// retries → message dropped.
func TestTruncateTelegramMessage_RuneSafe(t *testing.T) {
	// 1 ASCII byte + 3-byte runes: byte offset 3900 lands mid-rune.
	message := "a" + strings.Repeat("€", 1500) // 1 + 4500 bytes > 4096
	out := truncateTelegramMessage(message)
	require.True(t, utf8.ValidString(out), "truncation must not split a rune")
	require.LessOrEqual(t, len(out), 4096)
	require.Contains(t, out, "[truncated]")
}

// Truncation must not cut inside an HTML entity (e.g. "&amp;") — a dangling
// "&a" fragment can break Telegram HTML parsing.
func TestTruncateTelegramMessage_DoesNotSplitHTMLEntity(t *testing.T) {
	message := strings.Repeat("a", 3898) + "&amp;" + strings.Repeat("b", 400)
	out := truncateTelegramMessage(message)
	require.LessOrEqual(t, len(out), 4096)
	body := strings.TrimSuffix(strings.TrimSpace(out), "[truncated]")
	body = strings.TrimSpace(body)
	if i := strings.LastIndexByte(body, '&'); i >= 0 {
		require.Contains(t, body[i:], ";", "no dangling entity fragment at the cut point")
	}
}

// Truncation must not cut inside an HTML tag: a dangling "<b" (or bare "<")
// is a Telegram HTML parse error.
func TestTruncateTelegramMessage_DoesNotSplitHTMLTag(t *testing.T) {
	message := strings.Repeat("a", 3899) + "<b>bold</b>" + strings.Repeat("c", 400)
	out := truncateTelegramMessage(message)
	require.LessOrEqual(t, len(out), 4096)
	body := strings.TrimSuffix(strings.TrimSpace(out), "[truncated]")
	body = strings.TrimSpace(body)
	if i := strings.LastIndexByte(body, '<'); i >= 0 {
		require.Contains(t, body[i:], ">", "no dangling tag fragment at the cut point")
	}
}

// Short messages pass through untouched.
func TestTruncateTelegramMessage_LeavesShortMessages(t *testing.T) {
	message := "hello <b>world</b> &amp; more"
	require.Equal(t, message, truncateTelegramMessage(message))
}
