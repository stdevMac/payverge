package database

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newStaffCompTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Error),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Business{}, &Staff{}))
	return db
}

func TestStaffCompensation_RoundTrip(t *testing.T) {
	db := newStaffCompTestDB(t)
	s := Staff{BusinessID: 1, Email: "a@b.co", Name: "A", Role: StaffRoleServer,
		EmploymentType: "hourly", HourlyRateCents: 1850, AnnualSalaryCents: 0}
	require.NoError(t, db.Create(&s).Error)
	var got Staff
	require.NoError(t, db.First(&got, s.ID).Error)
	assert.Equal(t, "hourly", got.EmploymentType)
	assert.Equal(t, int64(1850), got.HourlyRateCents)
}

func TestStaffJSON_OmitsCompensation(t *testing.T) {
	s := Staff{BusinessID: 1, Email: "a@b.co", Name: "A", Role: StaffRoleServer,
		EmploymentType: "hourly", HourlyRateCents: 1850, AnnualSalaryCents: 9000000}
	b, err := json.Marshal(s)
	require.NoError(t, err)
	body := strings.ToLower(string(b))
	assert.NotContains(t, body, "hourly_rate")
	assert.NotContains(t, body, "annual_salary")
	assert.NotContains(t, body, "employment_type")
	assert.NotContains(t, body, "1850")
}
