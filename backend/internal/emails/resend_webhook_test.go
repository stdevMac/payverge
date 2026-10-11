package emails

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func signedResendWebhookRequest(t *testing.T, secret, webhookID string, payload []byte) *http.Request {
	t.Helper()
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	decoded, err := base64.StdEncoding.DecodeString(secret[len("whsec_"):])
	require.NoError(t, err)
	content := fmt.Sprintf("%s.%s.%s", webhookID, timestamp, payload)
	mac := hmac.New(sha256.New, decoded)
	_, _ = mac.Write([]byte(content))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/email/resend", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("svix-id", webhookID)
	req.Header.Set("svix-timestamp", timestamp)
	req.Header.Set("svix-signature", "v1,"+signature)
	return req
}

func TestResendWebhookRejectsUnsignedPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := suppressionTestDB(t)
	handler := NewResendWebhookHandler(db, "whsec_"+base64.StdEncoding.EncodeToString([]byte("webhook-secret")))
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/email/resend", bytes.NewBufferString(`{"type":"email.complained"}`))

	handler.Handle(ctx)
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	var count int64
	require.NoError(t, db.Model(&EmailSuppression{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestResendWebhookPersistsSuppressionsAndIsIdempotent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := suppressionTestDB(t)
	secret := "whsec_" + base64.StdEncoding.EncodeToString([]byte("webhook-secret"))
	handler := NewResendWebhookHandler(db, secret)

	events := []struct {
		webhookID string
		eventType string
		email     string
		extra     map[string]interface{}
		reason    SuppressionReason
	}{
		{"evt-bounce", "email.bounced", "bounce@example.test", map[string]interface{}{"bounce": map[string]interface{}{"type": "Permanent"}}, SuppressionReasonBounce},
		{"evt-complaint", "email.complained", "complaint@example.test", nil, SuppressionReasonComplaint},
		{"evt-suppressed", "email.suppressed", "suppressed@example.test", nil, SuppressionReasonProvider},
	}
	for _, event := range events {
		data := map[string]interface{}{"email_id": "email-" + event.webhookID, "to": []string{event.email}}
		for key, value := range event.extra {
			data[key] = value
		}
		payload, err := json.Marshal(map[string]interface{}{
			"type": event.eventType, "created_at": time.Now().UTC().Format(time.RFC3339Nano), "data": data,
		})
		require.NoError(t, err)

		for attempt := 0; attempt < 2; attempt++ {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = signedResendWebhookRequest(t, secret, event.webhookID, payload)
			handler.Handle(ctx)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		}

		stored, err := NewGormSuppressionStore(db).Lookup(t.Context(), event.email)
		require.NoError(t, err)
		require.NotNil(t, stored)
		require.Equal(t, event.reason, stored.Reason)
	}

	var deliveryEvents int64
	require.NoError(t, db.Model(&EmailDeliveryEvent{}).Count(&deliveryEvents).Error)
	require.EqualValues(t, len(events), deliveryEvents, "provider retries must not duplicate durable events")
}

func TestResendWebhookDoesNotSuppressDeliveredOrTemporaryBounce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := suppressionTestDB(t)
	secret := "whsec_" + base64.StdEncoding.EncodeToString([]byte("webhook-secret"))
	handler := NewResendWebhookHandler(db, secret)

	for i, event := range []map[string]interface{}{
		{"type": "email.delivered", "data": map[string]interface{}{"email_id": "delivered", "to": []string{"ok@example.test"}}},
		{"type": "email.bounced", "data": map[string]interface{}{"email_id": "temporary", "to": []string{"temporary@example.test"}, "bounce": map[string]interface{}{"type": "Temporary"}}},
	} {
		event["created_at"] = time.Now().UTC().Format(time.RFC3339Nano)
		payload, err := json.Marshal(event)
		require.NoError(t, err)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = signedResendWebhookRequest(t, secret, fmt.Sprintf("evt-safe-%d", i), payload)
		handler.Handle(ctx)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	}

	var suppressions int64
	require.NoError(t, db.Model(&EmailSuppression{}).Count(&suppressions).Error)
	require.Zero(t, suppressions)
}
