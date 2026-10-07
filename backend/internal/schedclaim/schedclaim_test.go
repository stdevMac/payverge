package schedclaim

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type job struct {
	ID     uint `gorm:"primaryKey"`
	DoneAt *time.Time
}

// dbSeq gives each setup() call a unique DSN so that named shared-cache
// in-memory databases do not bleed across test re-runs (-count > 1).
var dbSeq atomic.Int64

func setup(t *testing.T) *gorm.DB {
	t.Helper()
	// Named shared-cache in-memory DB: all connections within this call see
	// the same data, and the unique seq suffix prevents cross-run pollution.
	seq := dbSeq.Add(1)
	dsn := fmt.Sprintf("file:schedclaim_%d?mode=memory&cache=shared", seq)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&job{}))
	require.NoError(t, db.Create(&job{ID: 1}).Error)
	return db
}

func TestClaim_firstCallerWinsSecondLoses(t *testing.T) {
	db := setup(t)
	c := New(db)

	ok, err := c.Claim(context.Background(), "jobs", "done_at", "id", uint(1))
	require.NoError(t, err)
	require.True(t, ok, "first claim must win")

	ok, err = c.Claim(context.Background(), "jobs", "done_at", "id", uint(1))
	require.NoError(t, err)
	require.False(t, ok, "second claim must lose (done_at already set)")

	var got job
	require.NoError(t, db.First(&got, 1).Error)
	require.NotNil(t, got.DoneAt, "done_at must be stamped exactly once")
}

func TestClaim_missingRowReturnsFalseNoError(t *testing.T) {
	db := setup(t)
	c := New(db)
	ok, err := c.Claim(context.Background(), "jobs", "done_at", "id", uint(999))
	require.NoError(t, err)
	require.False(t, ok)
}
