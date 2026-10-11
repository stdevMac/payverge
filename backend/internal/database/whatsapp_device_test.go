package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupWhatsAppDeviceDB(t *testing.T) *gorm.DB {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:wa-dev-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(&Business{}, &WhatsAppBusinessDevice{}))
	return gormDB
}

func seedWABusiness(t *testing.T, slug string) *Business {
	t.Helper()
	biz := &Business{
		BusinessId:     slug,
		Name:           "WA " + slug,
		OwnerAddress:   "0xwa",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, GetDB().Create(biz).Error)
	return biz
}

func TestUpsertWhatsAppBusinessDevice_UniqueBusinessAndJID(t *testing.T) {
	setupWhatsAppDeviceDB(t)
	a := seedWABusiness(t, "wa-a")
	b := seedWABusiness(t, "wa-b")

	require.NoError(t, UpsertWhatsAppBusinessDevice(&WhatsAppBusinessDevice{
		BusinessID: a.ID,
		DeviceJID:  "111@s.whatsapp.net",
		Status:     WhatsAppDeviceStatusConnected,
	}))
	// Same business upserts in place.
	require.NoError(t, UpsertWhatsAppBusinessDevice(&WhatsAppBusinessDevice{
		BusinessID: a.ID,
		DeviceJID:  "111@s.whatsapp.net",
		Status:     WhatsAppDeviceStatusDegraded,
	}))
	got, err := GetWhatsAppBusinessDevice(a.ID)
	require.NoError(t, err)
	assert.Equal(t, WhatsAppDeviceStatusDegraded, got.Status)

	// Duplicate JID on another business must fail uniqueness.
	err = UpsertWhatsAppBusinessDevice(&WhatsAppBusinessDevice{
		BusinessID: b.ID,
		DeviceJID:  "111@s.whatsapp.net",
		Status:     WhatsAppDeviceStatusConnected,
	})
	require.Error(t, err)

	// Blank JID rejected.
	err = UpsertWhatsAppBusinessDevice(&WhatsAppBusinessDevice{
		BusinessID: b.ID,
		DeviceJID:  "",
		Status:     WhatsAppDeviceStatusPairing,
	})
	require.Error(t, err)
}

func TestMarkAndDeleteWhatsAppBusinessDevice(t *testing.T) {
	setupWhatsAppDeviceDB(t)
	biz := seedWABusiness(t, "wa-mark")
	require.NoError(t, UpsertWhatsAppBusinessDevice(&WhatsAppBusinessDevice{
		BusinessID: biz.ID,
		DeviceJID:  "222@s.whatsapp.net",
		Status:     WhatsAppDeviceStatusConnecting,
	}))
	now := time.Now().UTC()
	attempts := int64(2)
	require.NoError(t, MarkWhatsAppDeviceStatus(biz.ID, WhatsAppDeviceStatusConnected, "", &now, nil, &attempts))
	got, err := GetWhatsAppBusinessDevice(biz.ID)
	require.NoError(t, err)
	assert.Equal(t, WhatsAppDeviceStatusConnected, got.Status)
	assert.Equal(t, int64(2), got.ReconnectAttempts)
	require.NotNil(t, got.LastConnectedAt)

	require.NoError(t, DeleteWhatsAppBusinessDevice(biz.ID))
	_, err = GetWhatsAppBusinessDevice(biz.ID)
	require.Error(t, err)
}

func TestListWhatsAppBusinessDevicesForRestore(t *testing.T) {
	setupWhatsAppDeviceDB(t)
	now := time.Now().UTC()
	due := now.Add(-time.Minute)
	future := now.Add(time.Hour)

	bizConnected := seedWABusiness(t, "wa-conn")
	bizRetry := seedWABusiness(t, "wa-retry")
	bizFuture := seedWABusiness(t, "wa-future")
	require.NoError(t, UpsertWhatsAppBusinessDevice(&WhatsAppBusinessDevice{
		BusinessID: bizConnected.ID, DeviceJID: "c@s.whatsapp.net", Status: WhatsAppDeviceStatusConnected,
	}))
	require.NoError(t, UpsertWhatsAppBusinessDevice(&WhatsAppBusinessDevice{
		BusinessID: bizRetry.ID, DeviceJID: "r@s.whatsapp.net", Status: WhatsAppDeviceStatusDisconnected, NextRetryAt: &due,
	}))
	require.NoError(t, UpsertWhatsAppBusinessDevice(&WhatsAppBusinessDevice{
		BusinessID: bizFuture.ID, DeviceJID: "f@s.whatsapp.net", Status: WhatsAppDeviceStatusDisconnected, NextRetryAt: &future,
	}))

	list, err := ListWhatsAppBusinessDevicesForRestore(now, 50)
	require.NoError(t, err)
	ids := map[uint]bool{}
	for _, d := range list {
		ids[d.BusinessID] = true
	}
	assert.True(t, ids[bizConnected.ID])
	assert.True(t, ids[bizRetry.ID])
	assert.False(t, ids[bizFuture.ID], "future retry must not restore yet")
}
