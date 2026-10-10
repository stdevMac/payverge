package escalation

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestTelegramSendErrorOmitsBotToken: the bot token is in the request path, so
// a transport failure must not echo it into the error the service logs.
func TestTelegramSendErrorOmitsBotToken(t *testing.T) {
	// A closed server refuses the connection, which yields a *url.Error
	// carrying the full request URL.
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()

	oldBase := baseURL
	baseURL = srv.URL
	defer func() { baseURL = oldBase }()

	n := &TelegramNotifier{
		token:  "123456:SECRET-BOT-TOKEN",
		chatID: "12345",
		client: &http.Client{Timeout: 2 * time.Second},
	}
	err := n.Send(context.Background(), "hello")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "SECRET-BOT-TOKEN")
	require.Contains(t, err.Error(), srv.URL)
}
