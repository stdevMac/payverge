package services

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestUpdateDeliveryStatus_RejectsTransitionOutOfTerminalState verifies that a
// delivered/cancelled/failed delivery cannot be moved to a different status
// (which would corrupt the lifecycle timeline and driver KPIs), while a
// redundant no-op and a normal forward transition both still work.
func TestUpdateDeliveryStatus_RejectsTransitionOutOfTerminalState(t *testing.T) {
	setupDeliveryTestDB(t)
	svc := NewDeliveryService(database.GetDB(), nil)

	delivered := &database.DeliveryOrder{
		ID: 1, BusinessID: 100, DeliveryNumber: "DEL-T1",
		CustomerName: "John", CustomerPhone: "+1", CustomerEmail: "j@x.com",
		Status: database.DeliveryStatusDelivered,
	}
	require.NoError(t, database.GetDB().Create(delivered).Error)

	err := svc.UpdateDeliveryStatus(delivered.ID, database.DeliveryStatusPending, nil, "staff")
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrDeliveryStatusTerminal), "reversing a delivered order must be rejected, got %v", err)

	// Redundant no-op (same status) is allowed.
	require.NoError(t, svc.UpdateDeliveryStatus(delivered.ID, database.DeliveryStatusDelivered, nil, "staff"))

	// A non-terminal delivery can still advance.
	active := &database.DeliveryOrder{
		ID: 2, BusinessID: 100, DeliveryNumber: "DEL-T2",
		CustomerName: "Jane", CustomerPhone: "+2", CustomerEmail: "ja@x.com",
		Status: database.DeliveryStatusInTransit,
	}
	require.NoError(t, database.GetDB().Create(active).Error)
	require.NoError(t, svc.UpdateDeliveryStatus(active.ID, database.DeliveryStatusDelivered, nil, "staff"))
}

// TestAssignDriver_RejectsTerminalDelivery verifies a driver cannot be assigned
// to a cancelled/failed/delivered delivery (which would resurrect it as
// "assigned"). The terminal check fires before any driver validation.
func TestAssignDriver_RejectsTerminalDelivery(t *testing.T) {
	setupDeliveryTestDB(t)
	svc := NewDeliveryService(database.GetDB(), nil)

	cancelled := &database.DeliveryOrder{
		ID: 3, BusinessID: 100, DeliveryNumber: "DEL-T3",
		CustomerName: "Cx", CustomerPhone: "+3", CustomerEmail: "c@x.com",
		Status: database.DeliveryStatusCancelled, // never assigned → DriverID nil
	}
	require.NoError(t, database.GetDB().Create(cancelled).Error)

	driver := &database.DeliveryDriver{ID: 10, Name: "D", Phone: "+9", Email: "d@x.com"}
	require.NoError(t, database.GetDB().Create(driver).Error)

	err := svc.AssignDriver(cancelled.ID, driver.ID)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrDeliveryStatusTerminal), "assigning to a cancelled delivery must be rejected, got %v", err)
}
