package handlers

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/plugins"
)

func webhookFailureCount(plugin, reason string) float64 {
	return testutil.ToFloat64(metrics.PaymentWebhookProcessingFailures.WithLabelValues(plugin, reason))
}

// A forged webhook is rejected before any row is claimed; it must still be
// counted under the closed reason signature_invalid, exactly once.
func TestHandlePaymentWebhook_BadSignatureCountsSignatureInvalid(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	enableTestMercadoPagoPlugin(t, business.ID)
	plugins.GlobalRegistry.RegisterPlugin(&testPaymentPlugin{name: "mercadopago", verifySignature: false})

	before := webhookFailureCount("mercadopago", metrics.WebhookFailureSignatureInvalid)
	beforeRejected := webhookFailureCount("mercadopago", metrics.WebhookFailureRejected)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/?bill_id=1&business_id=%d", business.ID),
		bytes.NewBufferString(`{"id":"evt-forged","action":"payment.updated","data":{"id":"forged-1"}}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("x-signature", "ts=1,v1=forged")

	NewPluginHandlers(nil, nil).handlePaymentWebhook(c, "mercadopago")

	require.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, before+1, webhookFailureCount("mercadopago", metrics.WebhookFailureSignatureInvalid))
	assert.Equal(t, beforeRejected, webhookFailureCount("mercadopago", metrics.WebhookFailureRejected),
		"a pre-claim rejection must not be counted twice")
}

// A captured payment that cannot be applied (and whose auto-refund failed)
// is a settlement failure: counted as settlement_failed and surfaced on the
// plugin health row as last_status=error.
func TestHandlePaymentWebhook_SettlementFailureMarksPluginHealthError(t *testing.T) {
	setupHandlerTestDB(t)

	business := createTestBusiness(t)
	enableTestMercadoPagoPlugin(t, business.ID)

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-settle-fail-%d", time.Now().UnixNano()),
		Status:      database.BillStatusPaid,
		Items:       "[]",
		TotalAmount: 2000,
		PaidAmount:  2000,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	before := webhookFailureCount("mercadopago", metrics.WebhookFailureSettlementFailed)

	w, _ := driveMercadoPagoCompletedWebhook(t, business.ID, bill.ID, "mp-settle-fail-1", 2000, 0, fmt.Errorf("psp refund unavailable"))

	require.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, before+1, webhookFailureCount("mercadopago", metrics.WebhookFailureSettlementFailed))
	health := mustLoadBusinessPluginHealth(t, business.ID, "mercadopago")
	assert.Equal(t, "error", health["last_status"])
}
