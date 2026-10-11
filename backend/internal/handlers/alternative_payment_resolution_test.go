package handlers

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/metrics"
)

func TestRejectPendingAlternativePaymentAuthorizesPersistsAndReturnsTerminalState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)
	ownerID := uint(71)
	business := createPaymentRegressionBusiness(t, "alt-reject", &ownerID, "owner@example.com")
	bill := createPaymentRegressionBill(t, business.ID, 20)
	expiresAt := time.Now().Add(time.Minute)
	pending := &database.AlternativePayment{
		BillID: bill.ID, ParticipantAddr: "guest", ParticipantName: "Jane", Amount: 1_250,
		PaymentMethod: database.PaymentMethodCash, Status: database.AltPaymentStatusPending, ExpiresAt: &expiresAt,
	}
	require.NoError(t, database.GetDB().Create(pending).Error)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "user")
		c.Set("user_id", ownerID)
		c.Set("email", "owner@example.com")
		c.Next()
	})
	router.POST("/inside/bills/:bill_id/pending-alternative-payments/:request_id/reject", handler.RejectPendingAlternativePayment)
	rejectedBefore := metrics.CurrentAlternativePaymentRequestOutcome(metrics.AlternativePaymentRequestRejected)
	eventCh, _, cancelEvents := events.GetHub().SubscribeWithReplayTopic(business.ID, 0)
	defer cancelEvents()

	w := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/inside/bills/%d/pending-alternative-payments/%d/reject", bill.ID, pending.ID),
		map[string]any{"reason": "cash was not received"})

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"status":"rejected"`)
	require.Equal(t, rejectedBefore+1, metrics.CurrentAlternativePaymentRequestOutcome(metrics.AlternativePaymentRequestRejected))
	eventTypes, _ := collectHubEventTypes(eventCh, 100*time.Millisecond)
	require.Contains(t, eventTypes, "payment.request.resolved")
	require.NotContains(t, eventTypes, "payment.received", "rejecting a claim must never announce settled money")
	require.NoError(t, database.GetDB().First(pending, pending.ID).Error)
	require.Equal(t, database.AltPaymentStatusRejected, pending.Status)
	require.Equal(t, "owner@example.com", pending.ResolvedBy)
	require.Equal(t, "cash was not received", pending.ResolutionReason)

	// Exact retries are idempotent and keep the original audit fields.
	resolvedAt := *pending.ResolvedAt
	w = performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/inside/bills/%d/pending-alternative-payments/%d/reject", bill.ID, pending.ID),
		map[string]any{"reason": "changed retry text"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var replayed database.AlternativePayment
	require.NoError(t, database.GetDB().First(&replayed, pending.ID).Error)
	require.Equal(t, resolvedAt, *replayed.ResolvedAt)
	require.Equal(t, "cash was not received", replayed.ResolutionReason)
}

func TestCancelPendingAlternativePaymentRequiresBillPaymentAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPaymentRegressionDB(t)
	ownerID := uint(72)
	business := createPaymentRegressionBusiness(t, "alt-cancel-auth", &ownerID, "owner@example.com")
	bill := createPaymentRegressionBill(t, business.ID, 20)
	pending := &database.AlternativePayment{
		BillID: bill.ID, ParticipantAddr: "guest", Amount: 1_000,
		PaymentMethod: database.PaymentMethodCash, Status: database.AltPaymentStatusPending,
	}
	require.NoError(t, database.GetDB().Create(pending).Error)

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router := gin.New()
	router.POST("/inside/bills/:bill_id/pending-alternative-payments/:request_id/cancel", handler.CancelPendingAlternativePayment)

	w := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/inside/bills/%d/pending-alternative-payments/%d/cancel", bill.ID, pending.ID), nil)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	require.NoError(t, database.GetDB().First(pending, pending.ID).Error)
	require.Equal(t, database.AltPaymentStatusPending, pending.Status)
}
