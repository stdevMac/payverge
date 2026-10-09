package operational_alerts

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
)

// Fix 4 (end-to-end): closing a bill batch-resolves its order alerts inside
// the tx AND the SSE hub receives alert.resolved frames with the full alert
// row — same shape publishAlert produces — so the FE provider silences the
// urgent repeating alarm immediately instead of waiting for the next poll.
//
// The publish is wired through database.SetOperationalAlertResolvedPublisher,
// registered by this package's init() (database cannot import events: events
// imports database).
func TestCloseBillPublishesAlertResolvedFramesToHub(t *testing.T) {
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Table{},
		&database.Bill{},
		&database.Order{},
		&database.BillHistoryEvent{},
		&database.OperationalAlert{},
		&database.OperationalAlertEvent{},
	))
	database.InitTestDB(gormDB)

	business := &database.Business{
		BusinessId:     "ops-alert-close-publish",
		Name:           "Close Publish",
		OwnerAddress:   "0xowner",
		SettlementAddr: "0xsettle",
		TippingAddr:    "0xtip",
	}
	require.NoError(t, gormDB.Create(business).Error)
	bill := &database.Bill{
		BusinessID: business.ID,
		BillNumber: "B-close-publish",
		Status:     database.BillStatusOpen,
		Items:      "[]",
	}
	require.NoError(t, gormDB.Create(bill).Error)
	order := &database.Order{
		BusinessID:  business.ID,
		BillID:      bill.ID,
		OrderNumber: "O-close-publish",
		Status:      database.OrderStatusPending,
		CreatedBy:   "guest",
		Items:       `[{"id":"l1","menu_item_name":"Burger","quantity":1,"price":10,"subtotal":10}]`,
	}
	require.NoError(t, gormDB.Create(order).Error)
	alert := &database.OperationalAlert{
		BusinessID:   business.ID,
		AlertType:    database.OperationalAlertTypeOrderNew,
		ResourceType: database.OperationalAlertResourceTypeOrder,
		ResourceID:   int64(order.ID),
		Status:       database.OperationalAlertStatusOpen,
		Priority:     database.OperationalAlertPriorityUrgent,
		Title:        "New order #O-close-publish",
		LastEventAt:  time.Now(),
		Metadata:     database.JSONRawMessage(`{}`),
	}
	require.NoError(t, gormDB.Create(alert).Error)

	ch, _, cancel := events.GetHub().SubscribeWithReplayTopic(business.ID, 0)
	defer cancel()

	require.NoError(t, database.CloseBill(bill.ID))

	deadline := time.After(2 * time.Second)
	for {
		select {
		case frame := <-ch:
			if frame.Type != "alert.resolved" {
				continue
			}
			var payload database.OperationalAlert
			require.NoError(t, json.Unmarshal(frame.Data, &payload))
			require.Equal(t, alert.ID, payload.ID)
			require.Equal(t, business.ID, payload.BusinessID)
			require.Equal(t, database.OperationalAlertStatusResolved, payload.Status)
			require.NotNil(t, payload.ResolvedAt)
			return
		case <-deadline:
			t.Fatal("expected an alert.resolved frame on the hub after bill close")
		}
	}
}
