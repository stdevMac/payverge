package activation

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openActivationMemoryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func TestDispatcherMarksUnknownEventFailed(t *testing.T) {
	db := openActivationMemoryDB(t)
	require.NoError(t, db.AutoMigrate(&OutboxEvent{}))

	now := time.Now().UTC()
	row := OutboxEvent{
		EventName:      Name("not_a_real_event"),
		Dimensions:     json.RawMessage(`{}`),
		DeliveryStatus: "processing",
		IdempotencyKey: "k1",
		OccurredAt:     now,
	}
	require.NoError(t, db.Create(&row).Error)
	require.NotZero(t, row.ID)

	called := false
	d := &Dispatcher{
		db: db,
		sink: func(distinctID, event string, properties map[string]interface{}) error {
			called = true
			return nil
		},
		now: time.Now,
	}

	var delivered int
	var err error
	require.NotPanics(t, func() {
		delivered, err = d.deliverRows(context.Background(), []OutboxEvent{row}, time.Now())
	})
	require.Equal(t, 0, delivered)
	require.NoError(t, err)
	require.False(t, called)

	var reloaded OutboxEvent
	require.NoError(t, db.First(&reloaded, row.ID).Error)
	require.Equal(t, "failed", reloaded.DeliveryStatus)
	require.Equal(t, "unknown activation event", reloaded.LastError)
}

func TestSchedulerStopCancelsPromptly(t *testing.T) {
	db := openActivationMemoryDB(t)
	s := NewScheduler(db)
	s.interval = time.Hour
	require.NoError(t, s.Start())

	stopped := make(chan struct{})
	go func() {
		s.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler Stop did not return within 2s")
	}
}
