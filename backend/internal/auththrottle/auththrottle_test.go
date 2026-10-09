package auththrottle

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// dbSeq gives each newThrottle call a unique DSN so named shared-cache
// in-memory databases do not bleed across test re-runs (-count > 1).
var dbSeq atomic.Int64

func newThrottle(t *testing.T) *Throttle {
	t.Helper()
	seq := dbSeq.Add(1)
	dsn := fmt.Sprintf("file:auththrottle_%d?mode=memory&cache=shared", seq)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&database.AuthAttempt{}))
	return New(db, Config{MaxAttempts: 3, Window: time.Minute, BaseLockout: time.Minute})
}

func TestRecord_locksAfterMaxAttempts(t *testing.T) {
	th := newThrottle(t)
	for i := 0; i < 2; i++ {
		require.NoError(t, th.Record("user@x.com", "password_login"))
		locked, _, err := th.IsLocked("user@x.com", "password_login")
		require.NoError(t, err)
		require.False(t, locked, "must not lock before MaxAttempts")
	}
	require.NoError(t, th.Record("user@x.com", "password_login")) // 3rd -> lock
	locked, until, err := th.IsLocked("user@x.com", "password_login")
	require.NoError(t, err)
	require.True(t, locked)
	require.True(t, until.After(time.Now()))
}

func TestClear_resetsCounter(t *testing.T) {
	th := newThrottle(t)
	for i := 0; i < 3; i++ {
		require.NoError(t, th.Record("user@x.com", "password_login"))
	}
	require.NoError(t, th.Clear("user@x.com", "password_login"))
	locked, _, err := th.IsLocked("user@x.com", "password_login")
	require.NoError(t, err)
	require.False(t, locked, "Clear must drop the lock")
}

func TestRecord_exponentialBackoffGrowsLockout(t *testing.T) {
	th := newThrottle(t)
	for i := 0; i < 3; i++ {
		require.NoError(t, th.Record("user@x.com", "password_login"))
	}
	_, until1, _ := th.IsLocked("user@x.com", "password_login")
	d1 := time.Until(until1)
	require.NoError(t, th.Record("user@x.com", "password_login"))
	_, until2, _ := th.IsLocked("user@x.com", "password_login")
	d2 := time.Until(until2)
	require.Greater(t, d2, d1, "lockout must grow with repeated failures")
}
