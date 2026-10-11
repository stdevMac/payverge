package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/stdevmac/payverge/backend/internal/blockchain"
)

// TestRequireTransferAfterQuote_RejectsTokenWithoutIat pins that a quote token
// without its mint time is invalid: every server-minted quote carries iat, and
// without it the transfer cannot be proven to postdate the quote.
func TestRequireTransferAfterQuote_RejectsTokenWithoutIat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	ok := requireTransferAfterQuote(c, cryptoQuoteClaims{BillID: 1, Exp: 1}, blockchain.USDCTransferEvidence{BlockTimestamp: 1})

	assert.False(t, ok)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "quote_invalid")
}
