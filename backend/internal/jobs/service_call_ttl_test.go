package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	operational_alerts "github.com/stdevmac/payverge/backend/internal/services/operational_alerts"
)

func TestServiceCallTTLJanitorExpiresStaleCalls(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&database.Business{},
		&database.OperationalAlert{},
		&database.OperationalAlertEvent{},
	))
	business := &database.Business{
		BusinessId:     "svc-ttl-janitor",
		Name:           "TTL Lounge",
		OwnerAddress:   "0xowner",
		SettlementAddr: "0xsettle",
		TippingAddr:    "0xtip",
	}
	require.NoError(t, db.Create(business).Error)

	svc := operational_alerts.NewService(db)
	require.NoError(t, svc.CreateServiceCallAlert(context.Background(), business.ID, 1, "Table 1", "water"))
	stale := time.Now().Add(-2 * time.Hour)
	require.NoError(t, db.Model(&database.OperationalAlert{}).
		Where("resource_id = ?", 1).
		Update("last_event_at", stale).Error)

	janitor := NewServiceCallTTLJanitor(db, time.Minute)
	janitor.RunOnce()

	var alert database.OperationalAlert
	require.NoError(t, db.Where("resource_id = ?", 1).First(&alert).Error)
	require.Equal(t, database.OperationalAlertStatusResolved, alert.Status)
	require.NotNil(t, alert.ResolvedAt)
}

func TestServiceCallTTLJanitorExpiresEmptyTableCheckPlease(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&database.Business{},
		&database.OperationalAlert{},
		&database.OperationalAlertEvent{},
	))
	business := &database.Business{
		BusinessId:     "svc-ttl-check",
		Name:           "TTL Check",
		OwnerAddress:   "0xowner",
		SettlementAddr: "0xsettle",
		TippingAddr:    "0xtip",
	}
	require.NoError(t, db.Create(business).Error)

	svc := operational_alerts.NewService(db)
	require.NoError(t, svc.CreateServiceCallAlert(context.Background(), business.ID, 2, "Table 2", "check"))
	stale := time.Now().Add(-2 * time.Hour)
	require.NoError(t, db.Model(&database.OperationalAlert{}).
		Where("resource_id = ?", 2).
		Update("last_event_at", stale).Error)

	janitor := NewServiceCallTTLJanitor(db, time.Minute)
	janitor.RunOnce()

	var alert database.OperationalAlert
	require.NoError(t, db.Where("resource_id = ?", 2).First(&alert).Error)
	require.Equal(t, database.OperationalAlertStatusResolved, alert.Status)
}

func TestServiceCallTTLJanitorKeepsSameSeatingCheckPlease(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&database.Business{},
		&database.OperationalAlert{},
		&database.OperationalAlertEvent{},
		&database.Bill{},
	))
	business := &database.Business{
		BusinessId:     "svc-ttl-check-bill",
		Name:           "TTL Check Bill",
		OwnerAddress:   "0xowner",
		SettlementAddr: "0xsettle",
		TippingAddr:    "0xtip",
	}
	require.NoError(t, db.Create(business).Error)

	svc := operational_alerts.NewService(db)
	require.NoError(t, svc.CreateServiceCallAlert(context.Background(), business.ID, 3, "Table 3", "check"))
	stale := time.Now().Add(-2 * time.Hour)
	require.NoError(t, db.Model(&database.OperationalAlert{}).
		Where("resource_id = ?", 3).
		Update("last_event_at", stale).Error)
	require.NoError(t, db.Create(&database.Bill{
		BusinessID:     business.ID,
		TableID:        3,
		BillNumber:     "OPEN-3-" + t.Name(),
		Status:         database.BillStatusOpen,
		SettlementAddr: "settle",
		TippingAddr:    "tip",
		Items:          "[]",
		CreatedAt:      stale.Add(-20 * time.Minute),
	}).Error)

	janitor := NewServiceCallTTLJanitor(db, time.Minute)
	janitor.RunOnce()

	var alert database.OperationalAlert
	require.NoError(t, db.Where("resource_id = ?", 3).First(&alert).Error)
	require.Equal(t, database.OperationalAlertStatusOpen, alert.Status)
}

func TestServiceCallTTLJanitorStartStop(t *testing.T) {
	janitor := NewServiceCallTTLJanitor(nil, 10*time.Millisecond)
	janitor.Start()
	janitor.Start() // idempotent
	janitor.Stop()
}
