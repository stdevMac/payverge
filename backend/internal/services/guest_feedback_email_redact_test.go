package services

import (
	"bytes"
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

const guestFeedbackEmailProbe = "guest.feedback.redact+test@example.com"

type guestFeedbackEmailProbeProvider struct{}

func (guestFeedbackEmailProbeProvider) Send(context.Context, emails.EmailMessage) error {
	return nil
}

func guestFeedbackTemplatesRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "email", "templates"))
	_, err := os.Stat(root)
	require.NoError(t, err, "templates root %s", root)
	return root
}

func TestGuestFeedbackEmailLogRedactsGuestEmail(t *testing.T) {
	prev := emails.EmailServerInstance
	t.Cleanup(func() { emails.EmailServerInstance = prev })
	_, err := emails.NewEmailServer(
		guestFeedbackEmailProbeProvider{},
		"noreply@example.com",
		"updates@example.com",
		guestFeedbackTemplatesRoot(t),
	)
	require.NoError(t, err)

	var buf bytes.Buffer
	prevStd := log.Writer()
	prevLogger := logger.Logger.Out
	prevLogrus := logrus.StandardLogger().Out
	mw := io.MultiWriter(&buf)
	log.SetOutput(mw)
	logger.Logger.SetOutput(mw)
	logrus.SetOutput(mw)
	t.Cleanup(func() {
		log.SetOutput(prevStd)
		logger.Logger.SetOutput(prevLogger)
		logrus.SetOutput(prevLogrus)
	})

	scheduler := NewGuestFeedbackScheduler(nil)
	bill := &database.Bill{
		ID: 4242,
		Business: database.Business{
			Name:            "Feedback Bistro",
			BusinessId:      "feedback-bistro",
			DefaultLanguage: "en",
		},
	}

	require.NoError(t, scheduler.sendGuestFeedbackEmail(bill, guestFeedbackEmailProbe))

	logs := buf.String()
	if !strings.Contains(logs, "Guest feedback") && !strings.Contains(logs, "guest feedback") {
		t.Fatalf("expected a guest feedback send log, got %q", logs)
	}
	if strings.Contains(logs, guestFeedbackEmailProbe) {
		t.Fatalf("guest feedback log leaked raw guest address %q:\n%s", guestFeedbackEmailProbe, logs)
	}
	redacted := logger.RedactEmail(guestFeedbackEmailProbe)
	if !strings.Contains(logs, redacted) {
		t.Fatalf("guest feedback log missing redacted address %q:\n%s", redacted, logs)
	}
}
