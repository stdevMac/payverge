package server

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/services"

	sentry "github.com/getsentry/sentry-go"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// imageAlertTestDSN is a non-network DSN so sentry-go accepts a client while
// MockTransport captures events in memory.
const imageAlertTestDSN = "https://public@example.com/1"

func readImageAlertCounter(t *testing.T) float64 {
	t.Helper()
	return testutil.ToFloat64(metrics.AIImageMonthlyAlerts)
}

// bindMockSentryTransport rebinds the process hub to a MockTransport client and
// restores the previous client on cleanup so other tests in this package do not
// inherit a test transport. sentry.CaptureMessage routes through CurrentHub.
func bindMockSentryTransport(t *testing.T) *sentry.MockTransport {
	t.Helper()
	previous := sentry.CurrentHub().Client()
	t.Cleanup(func() {
		sentry.CurrentHub().BindClient(previous)
	})

	transport := &sentry.MockTransport{}
	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:       imageAlertTestDSN,
		Transport: transport,
	})
	require.NoError(t, err)
	sentry.CurrentHub().BindClient(client)
	return transport
}

func TestNotifyImageUsageAlert_OnlyFiresWhenTriggered(t *testing.T) {
	transport := bindMockSentryTransport(t)
	before := readImageAlertCounter(t)

	notifyImageUsageAlert(database.ImageUsageReservation{
		BusinessID: 42, AlertTriggered: false, MonthlyUsed: 2500,
	})
	assert.Equal(t, before, readImageAlertCounter(t), "untriggered reservation must not alert")
	assert.Empty(t, transport.Events(), "untriggered reservation must not send a Sentry event")

	notifyImageUsageAlert(database.ImageUsageReservation{
		BusinessID: 42, AlertTriggered: true, MonthlyUsed: 2000,
	})
	assert.Equal(t, before+1, readImageAlertCounter(t), "the crossing reservation alerts once")
}

// TestNotifyImageUsageAlert_SendsSentryWhenTriggered pins the Sentry half of
// notifyImageUsageAlert. The Prometheus counter is covered elsewhere; without
// this assertion, neutering sentry.CaptureMessage left the suite green.
func TestNotifyImageUsageAlert_SendsSentryWhenTriggered(t *testing.T) {
	transport := bindMockSentryTransport(t)

	notifyImageUsageAlert(database.ImageUsageReservation{
		BusinessID: 42, AlertTriggered: true, MonthlyUsed: 2000,
	})

	events := transport.Events()
	require.Len(t, events, 1, "triggered reservation must send exactly one Sentry event")
	assert.True(t, strings.Contains(events[0].Message, "business 42"),
		"Sentry message must name the business; got %q", events[0].Message)
}

// TestReserveGenerateRefundImage_AlertsOnRealReservation is the mirror for the
// GeneratedImage wrapper, the other of the two reserve call sites. The delivery
// validator is stubbed because the alert, not the CDN round trip, is under test.
func TestReserveGenerateRefundImage_AlertsOnRealReservation(t *testing.T) {
	setupImageLimitsTestDB(t)
	writeImageLimitSetting(t, database.SettingImageMonthlyAlert, "1")
	business := seedImageLimitsBusiness(t, "monthly-alert-image", "0xmonthlyalertimage")

	originalValidator := validateGeneratedImageDelivery
	t.Cleanup(func() { validateGeneratedImageDelivery = originalValidator })
	validateGeneratedImageDelivery = func(context.Context, *services.GeneratedImage) error { return nil }

	before := readImageAlertCounter(t)

	_, err := reserveGenerateRefundImage(
		context.Background(),
		business,
		func() (*services.GeneratedImage, error) {
			return &services.GeneratedImage{
				URL: "https://images.payverge.io/x.png", MIMEType: "image/png",
			}, nil
		},
	)
	require.NoError(t, err)
	assert.Equal(t, before+1, readImageAlertCounter(t), "crossing the threshold must alert")
}

func TestReserveGenerateRefundImage_DoesNotAlertWhenTheCrossingFails(t *testing.T) {
	setupImageLimitsTestDB(t)
	writeImageLimitSetting(t, database.SettingImageMonthlyAlert, "1")
	business := seedImageLimitsBusiness(t, "alert-failed-image", "0xalertfailedimage")

	originalValidator := validateGeneratedImageDelivery
	t.Cleanup(func() { validateGeneratedImageDelivery = originalValidator })
	validateGeneratedImageDelivery = func(context.Context, *services.GeneratedImage) error { return nil }

	before := readImageAlertCounter(t)

	_, err := reserveGenerateRefundImage(
		context.Background(),
		business,
		func() (*services.GeneratedImage, error) { return nil, errors.New("provider exploded") },
	)
	require.Error(t, err)
	assert.Equal(t, before, readImageAlertCounter(t),
		"a refunded crossing never happened; it must not alert")

	_, err = reserveGenerateRefundImage(
		context.Background(),
		business,
		func() (*services.GeneratedImage, error) {
			return &services.GeneratedImage{
				URL: "https://images.payverge.io/x.png", MIMEType: "image/png",
			}, nil
		},
	)
	require.NoError(t, err)
	assert.Equal(t, before+1, readImageAlertCounter(t),
		"the first generation that sticks is the crossing that alerts")
}
