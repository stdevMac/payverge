package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOrderStatus_IsValid(t *testing.T) {
	assert.True(t, OrderStatusPending.IsValid())
	assert.True(t, OrderStatusApproved.IsValid())
	assert.True(t, OrderStatusInKitchen.IsValid())
	assert.True(t, OrderStatusOrderReady.IsValid())
	assert.True(t, OrderStatusOrderDelivered.IsValid())
	assert.True(t, OrderStatusOrderCancelled.IsValid())
	assert.False(t, OrderStatus("bogus").IsValid())
	assert.False(t, OrderStatus("").IsValid())
}
