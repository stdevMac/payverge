package handlers

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaffSafeOrderStatus_InventoryHardBlock(t *testing.T) {
	err := errors.New("inventory hard block prevents approving order: flour does not have enough stock for Bread")
	status, code, msg := staffSafeOrderStatus(err)
	assert.Equal(t, http.StatusConflict, status)
	assert.Equal(t, server.ErrCodeInventoryInsufficientStock, code)
	assert.Contains(t, msg, "Insufficient stock")
}

func TestDeliveryValidationCode(t *testing.T) {
	cases := []struct {
		msg  string
		code string
	}{
		{"delivery fees and minimums cannot be negative", server.ErrCodeDeliveryFeeNegative},
		{"invalid payment_mode \"foo\"", server.ErrCodeDeliveryPaymentModeInvalid},
		{"invalid delivery_zones: boom", server.ErrCodeDeliveryZoneInvalid},
		{"zone 1: name required", server.ErrCodeDeliveryZoneNameRequired},
		{"duplicate active zone priority: 1", server.ErrCodeDeliveryZonePriorityDuplicate},
		{"delivery_start_time must be HH:MM (24-hour), got \"x\"", server.ErrCodeDeliveryHoursInvalid},
		{"delivery_start_time and delivery_end_time are required when delivery_hours_same_as_business is false", server.ErrCodeDeliveryHoursInvalid},
		{"in_house_delivery_enabled requires at least one active zone", server.ErrCodeDeliveryRequiresZoneOrPartner},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			got := deliveryValidationCode(errors.New(tc.msg))
			require.Equal(t, tc.code, got)
		})
	}
}
