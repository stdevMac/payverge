package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestReservationOccupancySnapshotUsesBatchedProjectedReads(t *testing.T) {
	recorder := &orderListSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	db, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())),
		&gorm.Config{Logger: recorder},
	)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Bill{}))

	tableIDs := make([]uint, 50)
	for i := range tableIDs {
		tableIDs[i] = uint(i + 1)
	}
	recorder.statements = nil

	_, err = GetReservationTableOccupancySnapshot(
		db,
		1,
		tableIDs,
		time.Now().UTC(),
		12*time.Hour,
	)
	require.NoError(t, err)
	require.Len(t, recorder.statements, 1)
	assert.Zero(t, recorder.selectStarCount("bills"))
	assert.False(t, recorder.selectMentionsColumn("bills", "items"))
}
