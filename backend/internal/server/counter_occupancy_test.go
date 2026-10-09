package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

func TestCreateBillCounterConflictReturnsActiveBillID(t *testing.T) {
	business, counter, bundle := setupBusinessBillOrderabilityTest(t, database.InventoryAvailabilityModeWarn)
	// Bundle child is recipe-linked; give stock so warn-mode orderability allows create.
	require.NoError(t, database.GetDB().Model(&database.InventoryItem{}).
		Where("business_id = ?", business.ID).
		Update("current_quantity", 10).Error)

	first := performBusinessBillCreateWithBundle(t, business, counter.ID, bundle)
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())
	var firstBody struct {
		Bill database.Bill `json:"bill"`
	}
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &firstBody))
	require.NotZero(t, firstBody.Bill.ID)

	second := performBusinessBillCreateWithBundle(t, business, counter.ID, bundle)
	require.Equal(t, http.StatusConflict, second.Code, second.Body.String())
	var conflict struct {
		Code         string `json:"code"`
		ActiveBillID uint   `json:"active_bill_id"`
	}
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &conflict))
	require.Equal(t, "counter_occupied", conflict.Code)
	require.Equal(t, firstBody.Bill.ID, conflict.ActiveBillID)
}
