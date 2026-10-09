package server

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/loyalty"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestMapLoyaltyRedeemError_KnownStates(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{
			name:   "insufficient points",
			err:    fmt.Errorf("%w: have 40, need 3000", loyalty.ErrInsufficientPoints),
			status: http.StatusUnprocessableEntity,
			code:   ErrCodeInsufficientPoints,
		},
		{
			name:   "already applied",
			err:    loyalty.ErrDiscountAlreadyApplied,
			status: http.StatusConflict,
			code:   ErrCodeLoyaltyAlreadyApplied,
		},
		{
			name:   "not connected",
			err:    loyalty.ErrNotConnected,
			status: http.StatusNotFound,
			code:   ErrCodeNoLoyaltyConnection,
		},
		{
			name:   "program disabled",
			err:    loyalty.ErrProgramNotEnabled,
			status: http.StatusBadRequest,
			code:   ErrCodeLoyaltyNotEnabled,
		},
		{
			name:   "rate unset",
			err:    loyalty.ErrRateNotConfigured,
			status: http.StatusBadRequest,
			code:   ErrCodeLoyaltyRateNotConfigured,
		},
		{
			name:   "too few points",
			err:    loyalty.ErrPointsTooSmall,
			status: http.StatusBadRequest,
			code:   ErrCodeLoyaltyPointsTooSmall,
		},
		{
			name:   "bill not open",
			err:    loyalty.ErrBillNotOpen,
			status: http.StatusNotFound,
			code:   ErrCodeNoActiveBill,
		},
		{
			name:   "record not found",
			err:    gorm.ErrRecordNotFound,
			status: http.StatusNotFound,
			code:   ErrCodeBusinessNotFound,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, code, _ := mapLoyaltyRedeemError(tc.err)
			assert.Equal(t, tc.status, status)
			assert.Equal(t, tc.code, code)
			assert.NotEqual(t, ErrCodeInvalidInput, code)
		})
	}
}

func TestMapLoyaltyRedeemError_DefaultIsNotFormValidation(t *testing.T) {
	t.Parallel()
	status, code, message := mapLoyaltyRedeemError(errors.New("something unexpected"))
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, "GENERIC_ERROR", code)
	assert.NotEqual(t, ErrCodeInvalidInput, code)
	assert.NotContains(t, message, "form")
}

func TestMapLoyaltyUndoError_KnownStates(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{
			name:   "not the redeemer",
			err:    loyalty.ErrUndoNotOwner,
			status: http.StatusForbidden,
			code:   ErrCodeLoyaltyUndoNotOwner,
		},
		{
			name:   "nothing to undo",
			err:    loyalty.ErrNoDiscountToUndo,
			status: http.StatusBadRequest,
			code:   ErrCodeNoLoyaltyDiscount,
		},
		{
			name:   "not connected",
			err:    loyalty.ErrNotConnected,
			status: http.StatusNotFound,
			code:   ErrCodeNoLoyaltyConnection,
		},
		{
			name:   "bill not open",
			err:    loyalty.ErrBillNotOpen,
			status: http.StatusNotFound,
			code:   ErrCodeNoActiveBill,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, code, _ := mapLoyaltyUndoError(tc.err)
			assert.Equal(t, tc.status, status)
			assert.Equal(t, tc.code, code)
			assert.NotEqual(t, ErrCodeInvalidInput, code)
		})
	}
}
