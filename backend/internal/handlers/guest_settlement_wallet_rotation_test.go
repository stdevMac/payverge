package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// A bill that still carries the pre-rotation wallet must not be quoted or
// settled: the guest would sign a transfer to a recipient the owner retired.
func TestCurrentGuestSettlementContract_RotatedWalletReturnsConflict(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(&database.Business{}).Where("id = ?", business.ID).
		Update("settlement_addr", "0x2222222222222222222222222222222222222222").Error)

	bill := &database.Bill{
		BusinessID:     business.ID,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	h := &PaymentHandler{db: database.GetDBWrapper()}
	_, ok := h.currentGuestSettlementContract(c, bill, guestPaymentMethodUSDC)

	require.False(t, ok)
	require.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "settlement_wallet_changed")
}

// The comparison is case-insensitive: a checksummed copy on the bill and a
// lowercase business row are the same wallet, so the check passes through to
// the next gate (here, the missing verifier) instead of returning 409.
func TestCurrentGuestSettlementContract_SameWalletDifferentCaseIsNotConflict(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(&database.Business{}).Where("id = ?", business.ID).
		Update("settlement_addr", "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd").Error)

	bill := &database.Bill{
		BusinessID:     business.ID,
		SettlementAddr: "0xABCDEFabcdefABCDEFabcdefABCDEFabcdefABCD",
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	h := &PaymentHandler{db: database.GetDBWrapper()}
	_, ok := h.currentGuestSettlementContract(c, bill, guestPaymentMethodUSDC)

	require.False(t, ok)
	assert.NotEqual(t, http.StatusConflict, w.Code)
	assert.NotContains(t, w.Body.String(), "settlement_wallet_changed")
}
