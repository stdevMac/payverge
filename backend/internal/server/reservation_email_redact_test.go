package server

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

// syncLogBuffer is a race-safe sink for capturing stdlib / logrus output
// while reservation sends finish on SafeGo goroutines.
type syncLogBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncLogBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncLogBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

const reservationGuestEmailProbe = "guest.alice.redact+test@example.com"

type reservationEmailProbeProvider struct {
	err error
}

func (p reservationEmailProbeProvider) Send(context.Context, emails.EmailMessage) error {
	return p.err
}

func reservationEmailTemplatesRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "email", "templates"))
	_, err := os.Stat(root)
	require.NoError(t, err, "templates root %s", root)
	return root
}

func installReservationEmailServer(t *testing.T, sendErr error) {
	t.Helper()
	prev := emails.EmailServerInstance
	t.Cleanup(func() { emails.EmailServerInstance = prev })
	_, err := emails.NewEmailServer(
		reservationEmailProbeProvider{err: sendErr},
		"noreply@example.com",
		"updates@example.com",
		reservationEmailTemplatesRoot(t),
	)
	require.NoError(t, err)
}

func captureReservationEmailLogs(t *testing.T) *syncLogBuffer {
	t.Helper()
	buf := &syncLogBuffer{}
	prevStd := log.Writer()
	prevLogger := logger.Logger.Out
	prevLogrus := logrus.StandardLogger().Out
	log.SetOutput(buf)
	logger.Logger.SetOutput(buf)
	logrus.SetOutput(buf)
	t.Cleanup(func() {
		log.SetOutput(prevStd)
		logger.Logger.SetOutput(prevLogger)
		logrus.SetOutput(prevLogrus)
	})
	return buf
}

func waitForReservationEmailLog(t *testing.T, buf *syncLogBuffer) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got := buf.String()
		if strings.Contains(got, "reservation") || strings.Contains(got, "Reservation") {
			time.Sleep(20 * time.Millisecond)
			return buf.String()
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for reservation email log; got %q", buf.String())
	return ""
}

func assertReservationLogRedactsGuestEmail(t *testing.T, logs, raw string) {
	t.Helper()
	if strings.Contains(logs, raw) {
		t.Fatalf("reservation email log leaked raw guest address %q:\n%s", raw, logs)
	}
	redacted := logger.RedactEmail(raw)
	if !strings.Contains(logs, redacted) {
		t.Fatalf("reservation email log missing redacted guest address %q:\n%s", redacted, logs)
	}
}

func reservationEmailProbeFixtures() (*database.TableReservation, *database.Business) {
	return &database.TableReservation{
			CustomerEmail:    reservationGuestEmailProbe,
			CustomerName:     "Alice Guest",
			ReservationTime:  time.Date(2026, 8, 20, 19, 0, 0, 0, time.UTC),
			PartySize:        2,
			ConfirmationCode: "REDCT1",
			Status:           "confirmed",
			Language:         "en",
		}, &database.Business{
			Name:            "Redact Bistro",
			Timezone:        "UTC",
			DefaultLanguage: "en",
			Phone:           "+15555550100",
		}
}

func TestReservationCancellationEmailLogRedactsGuestEmail(t *testing.T) {
	installReservationEmailServer(t, nil)
	buf := captureReservationEmailLogs(t)
	res, biz := reservationEmailProbeFixtures()

	sendReservationCancellationEmail(res, biz)
	logs := waitForReservationEmailLog(t, buf)
	assertReservationLogRedactsGuestEmail(t, logs, reservationGuestEmailProbe)
}

func TestReservationNoShowEmailLogRedactsGuestEmail(t *testing.T) {
	installReservationEmailServer(t, nil)
	buf := captureReservationEmailLogs(t)
	res, biz := reservationEmailProbeFixtures()

	sendReservationNoShowEmail(res, biz)
	logs := waitForReservationEmailLog(t, buf)
	assertReservationLogRedactsGuestEmail(t, logs, reservationGuestEmailProbe)
}

func TestReservationUpdatedEmailLogRedactsGuestEmail(t *testing.T) {
	installReservationEmailServer(t, nil)
	buf := captureReservationEmailLogs(t)
	res, biz := reservationEmailProbeFixtures()

	sendReservationUpdatedEmail(res, biz)
	logs := waitForReservationEmailLog(t, buf)
	assertReservationLogRedactsGuestEmail(t, logs, reservationGuestEmailProbe)
}

func TestReservationApprovalOutcomeEmailLogRedactsGuestEmail(t *testing.T) {
	installReservationEmailServer(t, nil)
	buf := captureReservationEmailLogs(t)
	res, biz := reservationEmailProbeFixtures()

	sendReservationApprovalOutcomeEmail("declined", res, biz)
	logs := waitForReservationEmailLog(t, buf)
	assertReservationLogRedactsGuestEmail(t, logs, reservationGuestEmailProbe)
}

func TestReservationEmailFailureLogRedactsGuestEmail(t *testing.T) {
	installReservationEmailServer(t, errors.New("provider down"))
	buf := captureReservationEmailLogs(t)
	res, biz := reservationEmailProbeFixtures()

	sendReservationCancellationEmail(res, biz)
	logs := waitForReservationEmailLog(t, buf)
	assertReservationLogRedactsGuestEmail(t, logs, reservationGuestEmailProbe)
}

func TestLogReservationGuestEmailRedactsAddress(t *testing.T) {
	buf := captureReservationEmailLogs(t)
	logReservationGuestEmail("confirmation", reservationGuestEmailProbe, errors.New("smtp timeout"))
	assertReservationLogRedactsGuestEmail(t, buf.String(), reservationGuestEmailProbe)
}

func TestCreatePublicReservationFailureLogUsesRedactEmail(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "reservation_handlers.go"))
	require.NoError(t, err)
	text := string(src)

	require.Contains(t, text, "logReservationGuestEmail(emailKind, reservation.CustomerEmail, emailErr)",
		"public reservation create must log send failures through the redacting helper")
	for i, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "log.Print") && strings.Contains(line, "CustomerEmail") {
			t.Fatalf("reservation_handlers.go:%d logs raw CustomerEmail: %s", i+1, strings.TrimSpace(line))
		}
	}
}
