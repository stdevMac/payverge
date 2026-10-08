package jobs

import (
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestStuckBillWatchdog_DedupsRepeatSweeps pins the emit contract for
// bill.stuck: a bill that was already reported stuck must NOT be re-published
// on every sweep tick (the frontend toasts on each event, so re-publishing the
// same stuck set every 5 minutes spams operators). A NEW bill crossing the
// threshold (count increase) must publish again.
func TestStuckBillWatchdog_DedupsRepeatSweeps(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&database.Bill{}))

	const businessID = uint(4242)
	stale := time.Now().Add(-3 * time.Hour)

	createStaleBill := func(number string) {
		bill := &database.Bill{
			BusinessID:  businessID,
			BillNumber:  number,
			Status:      database.BillStatusOpen,
			TotalAmount: 1000,
		}
		require.NoError(t, gormDB.Create(bill).Error)
		require.NoError(t, gormDB.Model(&database.Bill{}).Where("id = ?", bill.ID).
			UpdateColumn("updated_at", stale).Error)
	}
	createStaleBill("STUCK-1")

	w := NewStuckBillWatchdog(gormDB, StuckBillWatchdogConfig{Threshold: 2 * time.Hour})

	ch, _, cancel := events.GetHub().SubscribeWithReplayTopic(businessID, 0, "bill.stuck")
	defer cancel()

	drain := func() int {
		n := 0
		for {
			select {
			case <-ch:
				n++
			case <-time.After(200 * time.Millisecond):
				return n
			}
		}
	}

	w.RunOnce()
	require.Equal(t, 1, drain(), "first sweep must publish bill.stuck for the newly stuck bill")

	w.RunOnce()
	w.RunOnce()
	require.Equal(t, 0, drain(), "repeat sweeps over the same stuck set must not republish bill.stuck")

	createStaleBill("STUCK-2")
	w.RunOnce()
	require.Equal(t, 1, drain(), "a new bill crossing the threshold must publish bill.stuck again")
}
