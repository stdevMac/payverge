package services

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

type fakeReportRenderer struct {
	mu       sync.Mutex
	calls    int
	failures []error
}

func (f *fakeReportRenderer) Render(_ context.Context, schedule database.ReportSchedule, delivery database.ReportDelivery) (ReportEmail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if len(f.failures) > 0 {
		err := f.failures[0]
		f.failures = f.failures[1:]
		if err != nil {
			return ReportEmail{}, err
		}
	}
	return ReportEmail{
		Frequency:   schedule.Frequency,
		Recipients:  []string{"owner@example.test"},
		OwnerName:   "Owner",
		WindowStart: delivery.WindowStart,
		WindowEnd:   delivery.WindowEnd,
	}, nil
}

type fakeReportSender struct {
	mu             sync.Mutex
	calls          int
	uniqueSends    map[string]int
	failures       []error
	idempotencyKey []string
}

func (f *fakeReportSender) Send(_ context.Context, _ ReportEmail, idempotencyKey string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.idempotencyKey = append(f.idempotencyKey, idempotencyKey)
	if len(f.failures) > 0 {
		err := f.failures[0]
		f.failures = f.failures[1:]
		if err != nil {
			return err
		}
	}
	if f.uniqueSends == nil {
		f.uniqueSends = make(map[string]int)
	}
	// Model a provider-side idempotency key: repeated accepted requests are one
	// externally delivered email.
	f.uniqueSends[idempotencyKey] = 1
	return nil
}

func (f *fakeReportSender) snapshot() (calls, unique int, keys []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls, len(f.uniqueSends), append([]string(nil), f.idempotencyKey...)
}

func setupReportStateMachine(t *testing.T, due time.Time) (*database.DB, database.ReportSchedule) {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gdb.AutoMigrate(&database.Business{}, &database.ReportSchedule{}, &database.ReportDelivery{}))
	previous := database.GetDB()
	database.SetTestDB(gdb)
	t.Cleanup(func() { database.SetTestDB(previous) })

	business := database.Business{Name: "Reports", Email: "owner@example.test", OwnerName: "Owner"}
	require.NoError(t, gdb.Create(&business).Error)
	schedule := database.ReportSchedule{
		BusinessID: business.ID,
		Frequency:  database.ReportFrequencyDaily,
		Hour:       due.Hour(),
		Minute:     due.Minute(),
		Timezone:   "UTC",
		IsActive:   true,
		NextSendAt: due,
	}
	require.NoError(t, gdb.Create(&schedule).Error)
	return database.GetDBWrapper(), schedule
}

func configuredReportWorker(db *database.DB, renderer ReportRenderer, sender ReportSender) *ReportScheduler {
	rs := NewReportScheduler(db, nil, nil)
	rs.renderer = renderer
	rs.sender = sender
	rs.pluginEnabled = func(database.ReportSchedule) (bool, error) { return true, nil }
	rs.leaseDuration = time.Minute
	rs.maxAttempts = 3
	rs.retryBase = time.Minute
	rs.retryMax = 2 * time.Minute
	return rs
}

func TestReportDeliveryTransientFailureRetriesWithBoundedBackoffThenAlerts(t *testing.T) {
	due := time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC)
	db, schedule := setupReportStateMachine(t, due)
	renderer := &fakeReportRenderer{}
	sender := &fakeReportSender{failures: []error{errors.New("transport unavailable"), errors.New("transport unavailable"), errors.New("transport unavailable")}}
	rs := configuredReportWorker(db, renderer, sender)
	var alerts []database.ReportDelivery
	rs.alertOperator = func(_ context.Context, delivery database.ReportDelivery) error {
		alerts = append(alerts, delivery)
		return nil
	}

	rs.runOnce(due)
	rows, err := db.GetReportDeliveriesForSchedule(schedule.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, database.ReportDeliveryStateRetryWait, rows[0].State)
	require.Equal(t, due.Add(time.Minute), rows[0].NextAttemptAt)

	rs.runOnce(due.Add(59 * time.Second))
	calls, _, _ := sender.snapshot()
	require.Equal(t, 1, calls, "backoff prevents an early retry")

	rs.runOnce(due.Add(time.Minute))
	rows, err = db.GetReportDeliveriesForSchedule(schedule.ID)
	require.NoError(t, err)
	require.Equal(t, due.Add(3*time.Minute), rows[0].NextAttemptAt, "second retry is capped at two minutes")

	rs.runOnce(due.Add(3 * time.Minute))
	rows, err = db.GetReportDeliveriesForSchedule(schedule.ID)
	require.NoError(t, err)
	require.Equal(t, database.ReportDeliveryStateDeadLetter, rows[0].State)
	require.Equal(t, 3, rows[0].AttemptCount)
	require.Len(t, alerts, 1, "retry exhaustion alerts an operator once")

	reloaded, err := db.GetReportScheduleByID(schedule.ID)
	require.NoError(t, err)
	require.Nil(t, reloaded.LastSentAt)
	require.Equal(t, due, reloaded.NextSendAt, "failed delivery never advances the regular schedule")
}

func TestReportDeliveryAnalyticsFailureRetriesAndPermanentFailureDeadLetters(t *testing.T) {
	due := time.Date(2026, 8, 2, 8, 0, 0, 0, time.UTC)
	db, schedule := setupReportStateMachine(t, due)
	renderer := &fakeReportRenderer{failures: []error{errors.New("analytics unavailable"), PermanentReportDeliveryError(errors.New("business email missing"))}}
	sender := &fakeReportSender{}
	rs := configuredReportWorker(db, renderer, sender)
	alerted := 0
	rs.alertOperator = func(context.Context, database.ReportDelivery) error { alerted++; return nil }

	rs.runOnce(due)
	rows, err := db.GetReportDeliveriesForSchedule(schedule.ID)
	require.NoError(t, err)
	require.Equal(t, database.ReportDeliveryStateRetryWait, rows[0].State)
	require.Equal(t, 1, rows[0].AttemptCount)

	rs.runOnce(due.Add(time.Minute))
	rows, err = db.GetReportDeliveriesForSchedule(schedule.ID)
	require.NoError(t, err)
	require.Equal(t, database.ReportDeliveryStateDeadLetter, rows[0].State)
	require.Equal(t, 2, rows[0].AttemptCount)
	require.Equal(t, 1, alerted)
	calls, _, _ := sender.snapshot()
	require.Zero(t, calls, "transport is not attempted when rendering fails")
}

func TestReportDeliveryCrashAfterSendReusesProviderIdempotencyKey(t *testing.T) {
	due := time.Date(2026, 8, 3, 8, 0, 0, 0, time.UTC)
	db, schedule := setupReportStateMachine(t, due)
	renderer := &fakeReportRenderer{}
	sender := &fakeReportSender{}
	crashing := configuredReportWorker(db, renderer, sender)
	crashing.afterTransport = func(database.ReportDelivery) error { return ErrReportWorkerCrash }

	crashing.runOnce(due)
	rows, err := db.GetReportDeliveriesForSchedule(schedule.ID)
	require.NoError(t, err)
	require.Equal(t, database.ReportDeliveryStateLeased, rows[0].State, "a process crash leaves the lease for expiry")
	reloaded, err := db.GetReportScheduleByID(schedule.ID)
	require.NoError(t, err)
	require.Nil(t, reloaded.LastSentAt)

	restarted := configuredReportWorker(db, renderer, sender)
	restarted.runOnce(due.Add(time.Minute))

	calls, unique, keys := sender.snapshot()
	require.Equal(t, 2, calls, "the restarted worker retries the provider request")
	require.Equal(t, 1, unique, "provider idempotency prevents duplicate delivery")
	require.Equal(t, keys[0], keys[1])
	rows, err = db.GetReportDeliveriesForSchedule(schedule.ID)
	require.NoError(t, err)
	require.Equal(t, database.ReportDeliveryStateSent, rows[0].State)
	reloaded, err = db.GetReportScheduleByID(schedule.ID)
	require.NoError(t, err)
	require.NotNil(t, reloaded.LastSentAt)
}

func TestReportDeliveryCrashBeforeSendIsRecoveredAfterLeaseExpiry(t *testing.T) {
	due := time.Date(2026, 8, 4, 8, 0, 0, 0, time.UTC)
	db, schedule := setupReportStateMachine(t, due)
	delivery, _, err := db.EnsureReportDelivery(&schedule, due)
	require.NoError(t, err)
	claimed, err := db.ClaimReportDelivery(delivery.ID, "dead-worker", due, time.Minute, 3)
	require.NoError(t, err)
	require.NotNil(t, claimed)

	sender := &fakeReportSender{}
	restarted := configuredReportWorker(db, &fakeReportRenderer{}, sender)
	restarted.runOnce(due.Add(30 * time.Second))
	calls, _, _ := sender.snapshot()
	require.Zero(t, calls)
	restarted.runOnce(due.Add(time.Minute))
	calls, unique, _ := sender.snapshot()
	require.Equal(t, 1, calls)
	require.Equal(t, 1, unique)
}

func TestReportDeliveryDisabledProviderDeadLettersAndAlerts(t *testing.T) {
	due := time.Date(2026, 8, 5, 8, 0, 0, 0, time.UTC)
	db, schedule := setupReportStateMachine(t, due)
	rs := configuredReportWorker(db, &fakeReportRenderer{}, DisabledReportSender{})
	alerted := 0
	rs.alertOperator = func(context.Context, database.ReportDelivery) error { alerted++; return nil }

	rs.runOnce(due)
	rows, err := db.GetReportDeliveriesForSchedule(schedule.ID)
	require.NoError(t, err)
	require.Equal(t, database.ReportDeliveryStateDeadLetter, rows[0].State)
	require.Contains(t, rows[0].LastError, "disabled")
	require.Equal(t, 1, alerted)
}

func TestReportDeliveryProductionDisabledProviderDeadLettersAndAlerts(t *testing.T) {
	due := time.Date(2026, 8, 5, 9, 0, 0, 0, time.UTC)
	db, schedule := setupReportStateMachine(t, due)
	rs := configuredReportWorker(db, &fakeReportRenderer{}, &fakeReportSender{})
	rs.pluginEnabled = func(database.ReportSchedule) (bool, error) { return false, nil }
	alerted := 0
	rs.alertOperator = func(context.Context, database.ReportDelivery) error { alerted++; return nil }

	rs.runOnce(due)
	rows, err := db.GetReportDeliveriesForSchedule(schedule.ID)
	require.NoError(t, err)
	require.Len(t, rows, 1, "a disabled provider must leave an auditable terminal delivery")
	require.Equal(t, database.ReportDeliveryStateDeadLetter, rows[0].State)
	require.Contains(t, rows[0].LastError, "disabled")
	require.Equal(t, 1, alerted)
	reloaded, err := db.GetReportScheduleByID(schedule.ID)
	require.NoError(t, err)
	require.False(t, reloaded.IsActive)
}

func TestReportDeliveryCrashOnFinalAttemptExpiresToAmbiguousDeadLetter(t *testing.T) {
	due := time.Date(2026, 8, 7, 8, 0, 0, 0, time.UTC)
	db, schedule := setupReportStateMachine(t, due)
	sender := &fakeReportSender{}
	crashing := configuredReportWorker(db, &fakeReportRenderer{}, sender)
	crashing.maxAttempts = 1
	crashing.afterTransport = func(database.ReportDelivery) error { return ErrReportWorkerCrash }
	alerted := 0
	crashing.alertOperator = func(context.Context, database.ReportDelivery) error { alerted++; return nil }

	crashing.runOnce(due)
	crashing.runOnce(due.Add(time.Minute))
	rows, err := db.GetReportDeliveriesForSchedule(schedule.ID)
	require.NoError(t, err)
	require.Equal(t, database.ReportDeliveryStateDeadLetter, rows[0].State)
	require.Contains(t, rows[0].LastError, "outcome is unknown")
	require.Equal(t, 1, alerted)
	calls, unique, _ := sender.snapshot()
	require.Equal(t, 1, calls)
	require.Equal(t, 1, unique)
}

func TestReportDeliveryRetryExhaustionCreatesOperationalAlert(t *testing.T) {
	due := time.Date(2026, 8, 9, 8, 0, 0, 0, time.UTC)
	db, schedule := setupReportStateMachine(t, due)
	require.NoError(t, db.GetDB().AutoMigrate(&database.OperationalAlert{}, &database.OperationalAlertEvent{}))
	rs := configuredReportWorker(db, &fakeReportRenderer{}, &fakeReportSender{failures: []error{
		PermanentReportDeliveryError(errors.New("recipient rejected")),
	}})
	// Keep NewReportScheduler's production alert callback rather than replacing
	// it with a spy.

	rs.runOnce(due)
	var alert database.OperationalAlert
	require.NoError(t, db.GetDB().Where(
		"business_id = ? AND alert_type = ? AND resource_type = ?",
		schedule.BusinessID, "report_delivery_failed", "report_delivery",
	).First(&alert).Error)
	require.Equal(t, database.OperationalAlertPriorityHigh, alert.Priority)
	require.Equal(t, int64(1), alert.ResourceID)
	require.Contains(t, alert.Title, "Scheduled report")
}
