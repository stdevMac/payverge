package database

import (
	"bytes"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestProductionGORMLoggerSilencesNotFoundButLogsSQLFailure(t *testing.T) {
	var output bytes.Buffer
	gormLogger := newGORMLogger(log.New(&output, "", 0), logger.Warn, time.Second)
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: gormLogger})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE probes (id integer primary key)").Error)

	var row struct{ ID int }
	require.ErrorIs(t, db.Table("probes").Where("id = ?", 99).First(&row).Error, gorm.ErrRecordNotFound)
	assert.NotContains(t, output.String(), "record not found")

	_ = db.Exec("SELECT * FROM table_that_does_not_exist").Error
	assert.True(t, strings.Contains(output.String(), "table_that_does_not_exist"), output.String())
}
