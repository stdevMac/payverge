package handlers

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// drainHubEvents drains all events already queued on the subscription channel.
// Hub publishes are synchronous, so once the handler returns every emitted
// event is already buffered.
func drainHubEvents(ch <-chan events.BusinessEvent) []events.BusinessEvent {
	var out []events.BusinessEvent
	for {
		select {
		case ev := <-ch:
			out = append(out, ev)
		default:
			return out
		}
	}
}

func countHubEvents(evs []events.BusinessEvent, eventType string) int {
	n := 0
	for _, ev := range evs {
		if ev.Type == eventType {
			n++
		}
	}
	return n
}

func hubEventTypes(evs []events.BusinessEvent) []string {
	out := make([]string, 0, len(evs))
	for _, ev := range evs {
		out = append(out, ev.Type)
	}
	return out
}

// TestCancelDeliveryOrder_EmitsExactlyOneCancelled — MED dedup fix: the
// lifecycle helper (cancelDeliveryLinked → emitDeliveryCancelled) already
// publishes delivery.cancelled; the handler must NOT publish a second copy or
// guests/operators get duplicate toasts. The operational alert must still be
// resolved by the handler.
func TestCancelDeliveryOrder_EmitsExactlyOneCancelled(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)
	deliveryOrder := createTestDeliveryOrder(t, business.ID, bill.ID)
	alert := &database.OperationalAlert{
		BusinessID:   business.ID,
		AlertType:    database.OperationalAlertTypeDeliveryNew,
		ResourceType: database.OperationalAlertResourceTypeDelivery,
		ResourceID:   int64(deliveryOrder.ID),
		Status:       database.OperationalAlertStatusOpen,
		Priority:     database.OperationalAlertPriorityUrgent,
		Title:        "New delivery",
		LastEventAt:  time.Now(),
	}
	require.NoError(t, database.GetDB().Create(alert).Error)

	claimTestDelivery(t, business.ID, deliveryOrder.ID, 9)
	eventsCh, _, cancel := events.GetHub().SubscribeWithReplayTopic(business.ID, 0)
	defer cancel()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "delivery_id", Value: fmt.Sprintf("%d", deliveryOrder.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"reason":"Customer requested cancellation"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("staff_id", uint(9))
	c.Set("staff_name", "Test Staff")
	c.Set("staff_role", "server")

	handler := NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil))
	handler.CancelDeliveryOrder(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	evs := drainHubEvents(eventsCh)
	assert.Equal(t, 1, countHubEvents(evs, "delivery.cancelled"),
		"cancel must publish exactly ONE delivery.cancelled, got events: %v", hubEventTypes(evs))
	assert.Equal(t, 0, countHubEvents(evs, "delivery.updated"),
		"cancel must not also publish delivery.updated, got events: %v", hubEventTypes(evs))

	var refreshed database.OperationalAlert
	require.NoError(t, database.GetDB().First(&refreshed, alert.ID).Error)
	assert.Equal(t, database.OperationalAlertStatusResolved, refreshed.Status,
		"handler must still resolve the delivery operational alert after dedup")
}

// TestUpdateDeliveryStatus_FailedEmitsCancelledOnly — the failed/cancelled
// statuses route through the lifecycle helper, which publishes
// delivery.cancelled (payload status distinguishes failed). The handler must
// not stack an extra delivery.updated on top.
func TestUpdateDeliveryStatus_FailedEmitsCancelledOnly(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)
	deliveryOrder := createTestDeliveryOrder(t, business.ID, bill.ID)
	claimTestDelivery(t, business.ID, deliveryOrder.ID, 9)

	eventsCh, _, cancel := events.GetHub().SubscribeWithReplayTopic(business.ID, 0)
	defer cancel()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "delivery_id", Value: fmt.Sprintf("%d", deliveryOrder.ID)},
	}
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewBufferString(`{"status":"failed"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("staff_id", uint(9))
	c.Set("staff_name", "Test Staff")
	c.Set("staff_role", "server")

	handler := NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil))
	handler.UpdateDeliveryStatus(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	evs := drainHubEvents(eventsCh)
	assert.Equal(t, 1, countHubEvents(evs, "delivery.cancelled"),
		"failed must publish exactly ONE delivery.cancelled, got events: %v", hubEventTypes(evs))
	assert.Equal(t, 0, countHubEvents(evs, "delivery.updated"),
		"failed must not also publish delivery.updated, got events: %v", hubEventTypes(evs))
}
