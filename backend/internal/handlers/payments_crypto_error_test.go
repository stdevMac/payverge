package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/blockchain"
)

func TestRespondCryptoVerifyError_UnavailableIs425WithRetryCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	respondCryptoVerifyError(c, blockchain.ErrVerificationUnavailable)

	if rec.Code != http.StatusTooEarly {
		t.Fatalf("expected 425 for verification-unavailable, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "verification_unavailable") {
		t.Fatalf("expected code verification_unavailable in body, got %s", rec.Body.String())
	}
}
