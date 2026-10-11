package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupStaffLoginCodeServiceTestDB(t *testing.T) *StaffLoginCodeService {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	db = gormDB
	require.NoError(t, gormDB.AutoMigrate(
		&Business{},
		&Staff{},
		&StaffLoginCode{},
	))

	return NewStaffLoginCodeService()
}

func TestStaffLoginCodeService_ConsumeActiveCodesByStaffAndCode(t *testing.T) {
	service := setupStaffLoginCodeServiceTestDB(t)

	business := &Business{
		BusinessId:     "consume-staff-login-code",
		Name:           "Consume Staff Login Code",
		OwnerAddress:   "0xConsumeOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, db.Create(business).Error)

	staff := &Staff{
		BusinessID: business.ID,
		Email:      "consume@example.com",
		Name:       "Consume Staff",
		Role:       StaffRoleManager,
		InvitedBy:  "owner@example.com",
		IsActive:   true,
	}
	require.NoError(t, db.Create(staff).Error)

	loginCode := &StaffLoginCode{
		StaffID:   staff.ID,
		Code:      "777777",
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Used:      false,
	}
	require.NoError(t, db.Create(loginCode).Error)

	consumed, err := service.ConsumeActiveCodesByStaffAndCode(staff.ID, "777777")
	require.NoError(t, err)
	require.True(t, consumed)

	consumed, err = service.ConsumeActiveCodesByStaffAndCode(staff.ID, "777777")
	require.NoError(t, err)
	require.False(t, consumed)

	var persisted StaffLoginCode
	require.NoError(t, db.First(&persisted, loginCode.ID).Error)
	require.True(t, persisted.Used)
}

func TestStaffLoginCodeService_ActivateCodeForStaffReplacesPreviousActiveCode(t *testing.T) {
	service := setupStaffLoginCodeServiceTestDB(t)

	business := &Business{
		BusinessId:     "activate-staff-login-code",
		Name:           "Activate Staff Login Code",
		OwnerAddress:   "0xActivateOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, db.Create(business).Error)

	staff := &Staff{
		BusinessID: business.ID,
		Email:      "activate@example.com",
		Name:       "Activate Staff",
		Role:       StaffRoleManager,
		InvitedBy:  "owner@example.com",
		IsActive:   true,
	}
	require.NoError(t, db.Create(staff).Error)

	oldCode := &StaffLoginCode{
		StaffID:   staff.ID,
		Code:      "888888",
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Used:      false,
	}
	newCode := &StaffLoginCode{
		StaffID:   staff.ID,
		Code:      "999999",
		ExpiresAt: time.Now().Add(10 * time.Minute),
		Used:      true,
	}
	require.NoError(t, db.Create(oldCode).Error)
	require.NoError(t, db.Create(newCode).Error)

	require.NoError(t, service.ActivateCodeForStaff(staff.ID, newCode.ID))

	var persisted []StaffLoginCode
	require.NoError(t, db.Where("staff_id = ?", staff.ID).Order("id ASC").Find(&persisted).Error)
	require.Len(t, persisted, 2)
	require.True(t, persisted[0].Used)
	require.False(t, persisted[1].Used)
}
